package notifier

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"webhook-notifier/internal/config"
	"webhook-notifier/internal/events"
	"webhook-notifier/internal/registration"
	"webhook-notifier/internal/workqueue"
)

func TestNewApplicationBuildsDependenciesAndRoutes(t *testing.T) {
	// Input: successful registry and queue constructors backed by mocked dependencies.
	// Outcome: the notifier application builds its runtime dependencies and exposes the configured routes.
	restoreFactories := swapApplicationFactories(t)
	defer restoreFactories()

	registryDatabase, registryMock, registryMockError := newPingMock(t, nil)
	queueDatabase, queueMock, queueMockError := sqlExecMock(t)
	if registryMockError != nil || queueMockError != nil {
		t.Fatalf("unexpected sql mock creation errors: %v %v", registryMockError, queueMockError)
	}
	defer registryDatabase.Close()
	defer queueDatabase.Close()

	registryMock.ExpectPing()
	queueMock.ExpectExec("CREATE TABLE IF NOT EXISTS webhook_delivery_queue").WillReturnResult(sqlmock.NewResult(0, 0))

	newRegistryStore = func(string, string, string) (*registration.PostgresRegistry, error) {
		return registration.NewPostgresRegistryWithDatabaseConnection(registryDatabase, "SELECT webhook_url", "SELECT customer_id, webhook_url"), nil
	}
	newQueueRepository = func(string) (*workqueue.PostgresRepository, error) {
		return workqueue.NewPostgresRepositoryWithDatabaseConnection(queueDatabase), nil
	}
	newNotifierMetrics = newTestNotifierMetrics

	application, createError := NewApplication(newNotifierConfig(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if createError != nil {
		t.Fatalf("create application: %v", createError)
	}

	responseRecorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	application.httpServer.Handler.ServeHTTP(responseRecorder, request)
	if responseRecorder.Code != http.StatusOK {
		t.Fatalf("expected health route to return 200, got %d", responseRecorder.Code)
	}
}

func TestNewApplicationReturnsDependencyFailures(t *testing.T) {
	testCases := []struct {
		name        string
		setup       func(*testing.T)
		expectedErr string
	}{
		{
			name: "input registry constructor failure expects returned error",
			setup: func(t *testing.T) {
				newRegistryStore = func(string, string, string) (*registration.PostgresRegistry, error) {
					return nil, errors.New("registry failed")
				}
			},
			expectedErr: "registry failed",
		},
		{
			name: "input registry ping failure expects returned error",
			setup: func(t *testing.T) {
				registryDatabase, registryMock, _ := newPingMock(t, errors.New("ping failed"))
				t.Cleanup(func() { _ = registryDatabase.Close() })
				registryMock.ExpectPing().WillReturnError(errors.New("ping failed"))
				newRegistryStore = func(string, string, string) (*registration.PostgresRegistry, error) {
					return registration.NewPostgresRegistryWithDatabaseConnection(registryDatabase, "", ""), nil
				}
			},
			expectedErr: "ping postgres connection: ping failed",
		},
		{
			name: "input queue constructor failure expects returned error",
			setup: func(t *testing.T) {
				registryDatabase, registryMock, _ := newPingMock(t, nil)
				t.Cleanup(func() { _ = registryDatabase.Close() })
				registryMock.ExpectPing()
				newRegistryStore = func(string, string, string) (*registration.PostgresRegistry, error) {
					return registration.NewPostgresRegistryWithDatabaseConnection(registryDatabase, "", ""), nil
				}
				newQueueRepository = func(string) (*workqueue.PostgresRepository, error) {
					return nil, errors.New("queue failed")
				}
			},
			expectedErr: "queue failed",
		},
		{
			name: "input schema failure expects returned error",
			setup: func(t *testing.T) {
				registryDatabase, registryMock, _ := newPingMock(t, nil)
				queueDatabase, queueMock, _ := sqlExecMock(t)
				t.Cleanup(func() { _ = registryDatabase.Close() })
				t.Cleanup(func() { _ = queueDatabase.Close() })
				registryMock.ExpectPing()
				queueMock.ExpectExec("CREATE TABLE IF NOT EXISTS webhook_delivery_queue").WillReturnError(errors.New("schema failed"))
				newRegistryStore = func(string, string, string) (*registration.PostgresRegistry, error) {
					return registration.NewPostgresRegistryWithDatabaseConnection(registryDatabase, "", ""), nil
				}
				newQueueRepository = func(string) (*workqueue.PostgresRepository, error) {
					return workqueue.NewPostgresRepositoryWithDatabaseConnection(queueDatabase), nil
				}
			},
			expectedErr: "ensure queue schema: schema failed",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			restoreFactories := swapApplicationFactories(t)
			defer restoreFactories()
			testCase.setup(t)
			newNotifierMetrics = newTestNotifierMetrics

			application, createError := NewApplication(newNotifierConfig(), slog.New(slog.NewTextHandler(io.Discard, nil)))
			if application != nil {
				t.Fatalf("expected nil application on failure, got %#v", application)
			}
			if createError == nil || !strings.Contains(createError.Error(), testCase.expectedErr) {
				t.Fatalf("expected error containing %q, got %v", testCase.expectedErr, createError)
			}
		})
	}
}

func TestNotifierHandlersCoverSuccessAndErrorResponses(t *testing.T) {
	application := newUnitTestApplication()
	unitRegistry := application.registry.(*unitRegistry)
	unitQueue := application.workQueue.(*unitQueue)

	application.receivedEvents.Store(4)
	application.deliveredEvents.Store(3)
	application.failedDeliveries.Store(2)
	application.retriedDeliveries.Store(1)
	application.deadLetterCount.Store(5)

	t.Run("input health request expects ok payload", func(t *testing.T) {
		responseRecorder := httptest.NewRecorder()
		application.handleHealth(responseRecorder, httptest.NewRequest(http.MethodGet, "/health", nil))
		assertStatusAndBodyContains(t, responseRecorder, http.StatusOK, `"status":"ok"`)
	})

	t.Run("input stats request expects current counters", func(t *testing.T) {
		responseRecorder := httptest.NewRecorder()
		application.handleStats(responseRecorder, httptest.NewRequest(http.MethodGet, "/stats", nil))
		assertStatusAndBodyContains(t, responseRecorder, http.StatusOK, `"deadLetterCount":5`)
	})

	t.Run("input registrations success expects snapshot payload", func(t *testing.T) {
		responseRecorder := httptest.NewRecorder()
		application.handleRegistrations(responseRecorder, httptest.NewRequest(http.MethodGet, "/registrations", nil))
		assertStatusAndBodyContains(t, responseRecorder, http.StatusOK, `customer-a`)
	})

	t.Run("input registrations failure expects server error", func(t *testing.T) {
		unitRegistry.snapshotError = errors.New("snapshot failed")
		defer func() { unitRegistry.snapshotError = nil }()
		responseRecorder := httptest.NewRecorder()
		application.handleRegistrations(responseRecorder, httptest.NewRequest(http.MethodGet, "/registrations", nil))
		assertStatusAndBodyContains(t, responseRecorder, http.StatusInternalServerError, `snapshot failed`)
	})

	t.Run("input dead letter success expects queue snapshot payload", func(t *testing.T) {
		unitQueue.deadLetters = []events.DeadLetterMessage{{FailureReason: "failed"}}
		responseRecorder := httptest.NewRecorder()
		application.handleDeadLetters(responseRecorder, httptest.NewRequest(http.MethodGet, "/dlq", nil))
		assertStatusAndBodyContains(t, responseRecorder, http.StatusOK, `failed`)
	})

	t.Run("input dead letter failure expects server error", func(t *testing.T) {
		unitQueue.snapshotDeadLettersError = errors.New("dlq failed")
		defer func() { unitQueue.snapshotDeadLettersError = nil }()
		responseRecorder := httptest.NewRecorder()
		application.handleDeadLetters(responseRecorder, httptest.NewRequest(http.MethodGet, "/dlq", nil))
		assertStatusAndBodyContains(t, responseRecorder, http.StatusInternalServerError, `dlq failed`)
	})
}

func TestSingleAndBatchEventHandlersCoverDecodeAndEnqueueOutcomes(t *testing.T) {
	application := newUnitTestApplication()
	unitRegistry := application.registry.(*unitRegistry)
	unitQueue := application.workQueue.(*unitQueue)

	t.Run("input invalid single-event json expects bad request", func(t *testing.T) {
		responseRecorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/events", bytes.NewBufferString("{"))
		application.handleSingleEvent(responseRecorder, request)
		assertStatusAndBodyContains(t, responseRecorder, http.StatusBadRequest, `decode event`)
	})

	t.Run("input missing customer registration expects not found", func(t *testing.T) {
		responseRecorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/events", bytes.NewBufferString(marshalEventJSON(newTestEvent("missing", "event-1"))))
		application.handleSingleEvent(responseRecorder, request)
		assertStatusAndBodyContains(t, responseRecorder, http.StatusNotFound, `customer not registered`)
	})

	t.Run("input validation failure expects bad request", func(t *testing.T) {
		invalidEvent := newTestEvent("customer-a", "event-1")
		invalidEvent.EventID = ""
		responseRecorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/events", bytes.NewBufferString(marshalEventJSON(invalidEvent)))
		application.handleSingleEvent(responseRecorder, request)
		assertStatusAndBodyContains(t, responseRecorder, http.StatusBadRequest, `eventId is required`)
	})

	t.Run("input queue enqueue error expects bad request", func(t *testing.T) {
		unitQueue.enqueueError = errors.New("enqueue failed")
		defer func() { unitQueue.enqueueError = nil }()
		responseRecorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/events", bytes.NewBufferString(marshalEventJSON(newTestEvent("customer-a", "event-1"))))
		application.handleSingleEvent(responseRecorder, request)
		assertStatusAndBodyContains(t, responseRecorder, http.StatusBadRequest, `enqueue failed`)
	})

	t.Run("input valid single event expects accepted response and metric increments", func(t *testing.T) {
		beforeCounter := readCollectorValue(t, application.notifierMetrics.ReceivedEventsCounter)
		responseRecorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/events", bytes.NewBufferString(marshalEventJSON(newTestEvent("customer-a", "event-accepted"))))
		application.handleSingleEvent(responseRecorder, request)
		assertStatusAndBodyContains(t, responseRecorder, http.StatusAccepted, `"acceptedEvents":1`)
		if afterCounter := readCollectorValue(t, application.notifierMetrics.ReceivedEventsCounter); afterCounter != beforeCounter+1 {
			t.Fatalf("expected received counter increment, got before=%f after=%f", beforeCounter, afterCounter)
		}
	})

	t.Run("input invalid batch json expects bad request", func(t *testing.T) {
		responseRecorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/events/batch", bytes.NewBufferString("{"))
		application.handleBatchEvents(responseRecorder, request)
		assertStatusAndBodyContains(t, responseRecorder, http.StatusBadRequest, `decode events`)
	})

	t.Run("input batch with missing registration expects not found", func(t *testing.T) {
		eventsBatch := []events.SubscriberEvent{newTestEvent("customer-a", "event-1"), newTestEvent("missing", "event-2")}
		responseRecorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/events/batch", bytes.NewBufferString(marshalJSON(eventsBatch)))
		application.handleBatchEvents(responseRecorder, request)
		assertStatusAndBodyContains(t, responseRecorder, http.StatusNotFound, `customer not registered`)
	})

	t.Run("input valid batch expects accepted response", func(t *testing.T) {
		unitRegistry.webhookURLsByCustomerID["customer-b"] = []string{"https://example.com/b"}
		eventsBatch := []events.SubscriberEvent{newTestEvent("customer-a", "event-10"), newTestEvent("customer-b", "event-11")}
		responseRecorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/events/batch", bytes.NewBufferString(marshalJSON(eventsBatch)))
		application.handleBatchEvents(responseRecorder, request)
		assertStatusAndBodyContains(t, responseRecorder, http.StatusAccepted, `"acceptedEvents":2`)
	})
}

func TestEnqueueEventsCoversResolveAndValidationFailures(t *testing.T) {
	application := newUnitTestApplication()
	unitRegistry := application.registry.(*unitRegistry)
	unitQueue := application.workQueue.(*unitQueue)

	_, enqueueError := application.enqueueEvents([]events.SubscriberEvent{{}})
	if enqueueError == nil || enqueueError.Error() != "eventId is required" {
		t.Fatalf("expected event validation error, got %v", enqueueError)
	}

	unitRegistry.resolveError = errors.New("resolve failed")
	_, enqueueError = application.enqueueEvents([]events.SubscriberEvent{newTestEvent("customer-a", "event-1")})
	if enqueueError == nil || enqueueError.Error() != "resolve failed" {
		t.Fatalf("expected resolve error, got %v", enqueueError)
	}
	unitRegistry.resolveError = nil

	unitQueue.enqueueError = errors.New("enqueue failed")
	_, enqueueError = application.enqueueEvents([]events.SubscriberEvent{newTestEvent("customer-a", "event-2")})
	if enqueueError == nil || enqueueError.Error() != "enqueue failed" {
		t.Fatalf("expected enqueue error, got %v", enqueueError)
	}
}

func marshalEventJSON(event events.SubscriberEvent) string {
	return marshalJSON(event)
}

func marshalJSON(value any) string {
	bodyBytes, marshalError := json.Marshal(value)
	if marshalError != nil {
		panic(marshalError)
	}
	return string(bodyBytes)
}

func assertStatusAndBodyContains(t *testing.T, responseRecorder *httptest.ResponseRecorder, expectedStatus int, bodyFragment string) {
	t.Helper()
	if responseRecorder.Code != expectedStatus {
		t.Fatalf("expected status %d, got %d with body %s", expectedStatus, responseRecorder.Code, responseRecorder.Body.String())
	}
	if !strings.Contains(responseRecorder.Body.String(), bodyFragment) {
		t.Fatalf("expected body to contain %q, got %s", bodyFragment, responseRecorder.Body.String())
	}
}

func newNotifierConfig() config.NotifierConfig {
	return config.NotifierConfig{
		HTTPAddress:               "127.0.0.1:0",
		WorkerCount:               2,
		RequestTimeout:            time.Second,
		MaxRetryAttempts:          2,
		InitialRetryDelay:         10 * time.Millisecond,
		QueueClaimBatchSize:       4,
		QueuePollInterval:         5 * time.Millisecond,
		SchedulerBufferMultiplier: 4,
		MetricsReportInterval:     5 * time.Millisecond,
		ShutdownTimeout:           time.Second,
		LogLevel:                  "INFO",
		PostgresConnection:        "postgres://postgres:postgres@127.0.0.1:5432/webhook_notifier?sslmode=disable",
		RegistrationResolveQuery:  "SELECT webhook_url",
		RegistrationSnapshotQuery: "SELECT customer_id, webhook_url",
	}
}
