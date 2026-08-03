package notifier

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"webhook-notifier/internal/config"
	"webhook-notifier/internal/delivery"
	"webhook-notifier/internal/events"
	"webhook-notifier/internal/registration"
	"webhook-notifier/internal/retry"
	"webhook-notifier/internal/scheduler"
	"webhook-notifier/internal/testsupport"
	"webhook-notifier/internal/workqueue"
)

func TestNotifierIntegrationPollsPostgresQueueAndDeliversWebhook(t *testing.T) {
	// Input: one valid subscriber event for a registered customer with a real PostgreSQL-backed registry and queue.
	// Outcome: notifier writes one queue row to PostgreSQL, polls it, delivers the webhook, and marks the row completed.
	postgresDSN, cleanupPostgres := testsupport.PostgresDSN(t)
	defer cleanupPostgres()

	databaseConnection, openError := sql.Open("pgx", postgresDSN)
	if openError != nil {
		t.Fatalf("open postgres connection: %v", openError)
	}

	webhookRequests := make(chan events.SubscriberEvent, 1)
	webhookServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Fatalf("expected POST request, got %s", request.Method)
		}

		var deliveredEvent events.SubscriberEvent
		if decodeError := json.NewDecoder(request.Body).Decode(&deliveredEvent); decodeError != nil {
			t.Fatalf("decode delivered event: %v", decodeError)
		}

		webhookRequests <- deliveredEvent
		responseWriter.WriteHeader(http.StatusOK)
	}))
	defer webhookServer.Close()

	registrationTableName := fmt.Sprintf("webhook_registrations_notifier_it_%d", time.Now().UnixNano())
	requestContext := context.Background()
	if _, createError := databaseConnection.ExecContext(requestContext, fmt.Sprintf(`
		CREATE TABLE %s (
			customer_id TEXT NOT NULL,
			webhook_url TEXT NOT NULL,
			is_active BOOLEAN NOT NULL DEFAULT TRUE
		)
	`, registrationTableName)); createError != nil {
		t.Fatalf("create notifier integration registration table: %v", createError)
	}

	registry := registration.NewPostgresRegistryWithDatabaseConnection(
		databaseConnection,
		fmt.Sprintf("SELECT webhook_url FROM %s WHERE customer_id = $1 AND is_active = TRUE ORDER BY webhook_url", registrationTableName),
		fmt.Sprintf("SELECT customer_id, webhook_url FROM %s WHERE is_active = TRUE ORDER BY customer_id, webhook_url", registrationTableName),
	)

	queueRepository, repositoryError := workqueue.NewPostgresRepository(postgresDSN)
	if repositoryError != nil {
		t.Fatalf("open postgres queue repository: %v", repositoryError)
	}
	if ensureSchemaError := queueRepository.EnsureSchema(requestContext); ensureSchemaError != nil {
		t.Fatalf("ensure queue schema: %v", ensureSchemaError)
	}
	if _, truncateError := databaseConnection.ExecContext(requestContext, "TRUNCATE TABLE webhook_delivery_queue RESTART IDENTITY"); truncateError != nil {
		t.Fatalf("truncate queue table before notifier integration test: %v", truncateError)
	}

	defer func() {
		if _, cleanupError := databaseConnection.ExecContext(context.Background(), "TRUNCATE TABLE webhook_delivery_queue RESTART IDENTITY"); cleanupError != nil {
			t.Fatalf("truncate queue table after notifier integration test: %v", cleanupError)
		}
		if _, dropError := databaseConnection.ExecContext(context.Background(), fmt.Sprintf("DROP TABLE IF EXISTS %s", registrationTableName)); dropError != nil {
			t.Fatalf("drop notifier integration registration table: %v", dropError)
		}
		if closeError := queueRepository.Close(); closeError != nil {
			t.Fatalf("close postgres queue repository: %v", closeError)
		}
		if closeError := registry.Close(); closeError != nil {
			t.Fatalf("close postgres registry: %v", closeError)
		}
	}()

	if _, insertError := databaseConnection.ExecContext(requestContext, fmt.Sprintf(`
		INSERT INTO %s (customer_id, webhook_url, is_active)
		VALUES ('customer-a', $1, TRUE)
	`, registrationTableName), webhookServer.URL); insertError != nil {
		t.Fatalf("insert notifier integration registration row: %v", insertError)
	}

	application := &Application{
		config: config.NotifierConfig{
			WorkerCount:         1,
			RequestTimeout:      2 * time.Second,
			MaxRetryAttempts:    1,
			InitialRetryDelay:   10 * time.Millisecond,
			QueueClaimBatchSize: 32,
			QueuePollInterval:   10 * time.Millisecond,
		},
		logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		registry:       registry,
		workQueue:      queueRepository,
		scheduler:      scheduler.NewRoundRobinScheduler(4),
		deliveryClient: delivery.NewHTTPClient(2 * time.Second),
		retryPolicy: retry.ExponentialBackoffPolicy{
			InitialDelay:    10 * time.Millisecond,
			MaxRetryAttempt: 1,
		},
		notifierMetrics: newTestNotifierMetrics(),
	}

	runContext, cancelRun := context.WithCancel(context.Background())
	workers := startTestWorkers(runContext, application, 1)
	defer func() {
		cancelRun()
		application.scheduler.Close()
		workers.Wait()
	}()

	testEvent := newTestEvent("customer-a", "event-postgres-backed-delivery")
	createdJobs, enqueueError := application.enqueueEvents([]events.SubscriberEvent{testEvent})
	if enqueueError != nil {
		t.Fatalf("enqueue postgres-backed notifier event: %v", enqueueError)
	}
	if createdJobs != 1 {
		t.Fatalf("expected 1 created job, got %d", createdJobs)
	}

	queueRowCount := countQueueRowsForEvent(t, databaseConnection, testEvent.EventID)
	if queueRowCount != 1 {
		t.Fatalf("expected 1 queue row for %s, got %d", testEvent.EventID, queueRowCount)
	}

	select {
	case deliveredEvent := <-webhookRequests:
		assertDeliveredEventMatches(t, testEvent, deliveredEvent)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for postgres-backed webhook delivery")
	}

	waitForNotifierCount(t, func() int64 { return application.deliveredEvents.Load() }, 1, "deliveredEvents")
	waitForCondition(t, "postgres queue row to be completed", func() bool {
		return queueStatusForEvent(t, databaseConnection, testEvent.EventID) == "completed"
	})

	if application.receivedEvents.Load() != 1 {
		t.Fatalf("expected receivedEvents 1, got %d", application.receivedEvents.Load())
	}
	if application.failedDeliveries.Load() != 0 {
		t.Fatalf("expected failedDeliveries 0, got %d", application.failedDeliveries.Load())
	}
	if application.deadLetterCount.Load() != 0 {
		t.Fatalf("expected deadLetterCount 0, got %d", application.deadLetterCount.Load())
	}
}

func TestNotifierIntegrationRetriesPostgresQueueDeliveryThenSucceeds(t *testing.T) {
	// Input: one valid subscriber event whose registered webhook returns HTTP 500 once and HTTP 200 on retry.
	// Outcome: notifier retries the PostgreSQL-backed queue item once, delivers successfully, and persists the retry state.
	postgresDSN, cleanupPostgres := testsupport.PostgresDSN(t)
	defer cleanupPostgres()

	databaseConnection, openError := sql.Open("pgx", postgresDSN)
	if openError != nil {
		t.Fatalf("open postgres connection: %v", openError)
	}

	var attemptCount atomic.Int64
	webhookRequests := make(chan events.SubscriberEvent, 2)
	webhookServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		var deliveredEvent events.SubscriberEvent
		if decodeError := json.NewDecoder(request.Body).Decode(&deliveredEvent); decodeError != nil {
			t.Fatalf("decode delivered event: %v", decodeError)
		}

		webhookRequests <- deliveredEvent
		if attemptCount.Add(1) == 1 {
			http.Error(responseWriter, "temporary failure", http.StatusInternalServerError)
			return
		}

		responseWriter.WriteHeader(http.StatusOK)
	}))
	defer webhookServer.Close()

	registrationTableName := fmt.Sprintf("webhook_registrations_notifier_retry_it_%d", time.Now().UnixNano())
	requestContext := context.Background()
	if _, createError := databaseConnection.ExecContext(requestContext, fmt.Sprintf(`
		CREATE TABLE %s (
			customer_id TEXT NOT NULL,
			webhook_url TEXT NOT NULL,
			is_active BOOLEAN NOT NULL DEFAULT TRUE
		)
	`, registrationTableName)); createError != nil {
		t.Fatalf("create notifier retry integration registration table: %v", createError)
	}

	registry := registration.NewPostgresRegistryWithDatabaseConnection(
		databaseConnection,
		fmt.Sprintf("SELECT webhook_url FROM %s WHERE customer_id = $1 AND is_active = TRUE ORDER BY webhook_url", registrationTableName),
		fmt.Sprintf("SELECT customer_id, webhook_url FROM %s WHERE is_active = TRUE ORDER BY customer_id, webhook_url", registrationTableName),
	)

	queueRepository, repositoryError := workqueue.NewPostgresRepository(postgresDSN)
	if repositoryError != nil {
		t.Fatalf("open postgres queue repository: %v", repositoryError)
	}
	if ensureSchemaError := queueRepository.EnsureSchema(requestContext); ensureSchemaError != nil {
		t.Fatalf("ensure queue schema: %v", ensureSchemaError)
	}
	if _, truncateError := databaseConnection.ExecContext(requestContext, "TRUNCATE TABLE webhook_delivery_queue RESTART IDENTITY"); truncateError != nil {
		t.Fatalf("truncate queue table before notifier retry integration test: %v", truncateError)
	}

	defer func() {
		if _, cleanupError := databaseConnection.ExecContext(context.Background(), "TRUNCATE TABLE webhook_delivery_queue RESTART IDENTITY"); cleanupError != nil {
			t.Fatalf("truncate queue table after notifier retry integration test: %v", cleanupError)
		}
		if _, dropError := databaseConnection.ExecContext(context.Background(), fmt.Sprintf("DROP TABLE IF EXISTS %s", registrationTableName)); dropError != nil {
			t.Fatalf("drop notifier retry integration registration table: %v", dropError)
		}
		if closeError := queueRepository.Close(); closeError != nil {
			t.Fatalf("close postgres queue repository: %v", closeError)
		}
		if closeError := registry.Close(); closeError != nil {
			t.Fatalf("close postgres registry: %v", closeError)
		}
	}()

	if _, insertError := databaseConnection.ExecContext(requestContext, fmt.Sprintf(`
		INSERT INTO %s (customer_id, webhook_url, is_active)
		VALUES ('customer-a', $1, TRUE)
	`, registrationTableName), webhookServer.URL); insertError != nil {
		t.Fatalf("insert notifier retry integration registration row: %v", insertError)
	}

	application := &Application{
		config: config.NotifierConfig{
			WorkerCount:         1,
			RequestTimeout:      200 * time.Millisecond,
			MaxRetryAttempts:    2,
			InitialRetryDelay:   10 * time.Millisecond,
			QueueClaimBatchSize: 32,
			QueuePollInterval:   10 * time.Millisecond,
		},
		logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		registry:       registry,
		workQueue:      queueRepository,
		scheduler:      scheduler.NewRoundRobinScheduler(4),
		deliveryClient: delivery.NewHTTPClient(200 * time.Millisecond),
		retryPolicy: retry.ExponentialBackoffPolicy{
			InitialDelay:    10 * time.Millisecond,
			MaxRetryAttempt: 2,
		},
		notifierMetrics: newTestNotifierMetrics(),
	}

	runContext, cancelRun := context.WithCancel(context.Background())
	workers := startTestWorkers(runContext, application, 1)
	defer func() {
		cancelRun()
		application.scheduler.Close()
		workers.Wait()
	}()

	testEvent := newTestEvent("customer-a", "event-postgres-backed-retry")
	createdJobs, enqueueError := application.enqueueEvents([]events.SubscriberEvent{testEvent})
	if enqueueError != nil {
		t.Fatalf("enqueue postgres-backed retry notifier event: %v", enqueueError)
	}
	if createdJobs != 1 {
		t.Fatalf("expected 1 created job, got %d", createdJobs)
	}

	waitForNotifierCount(t, func() int64 { return attemptCount.Load() }, 2, "attemptCount")
	waitForNotifierCount(t, func() int64 { return application.deliveredEvents.Load() }, 1, "deliveredEvents")
	waitForNotifierCount(t, func() int64 { return application.failedDeliveries.Load() }, 1, "failedDeliveries")
	waitForNotifierCount(t, func() int64 { return application.retriedDeliveries.Load() }, 1, "retriedDeliveries")
	waitForCondition(t, "postgres queue row to be completed after retry", func() bool {
		return queueRowStateForEvent(t, databaseConnection, testEvent.EventID).Status == "completed"
	})

	if application.deadLetterCount.Load() != 0 {
		t.Fatalf("expected deadLetterCount 0, got %d", application.deadLetterCount.Load())
	}
	if len(drainDeliveredEvents(webhookRequests)) != 2 {
		t.Fatalf("expected 2 delivery attempts to reach the webhook")
	}

	queueRowState := queueRowStateForEvent(t, databaseConnection, testEvent.EventID)
	if queueRowState.RetryCount != 1 {
		t.Fatalf("expected retry_count 1, got %d", queueRowState.RetryCount)
	}
	if queueRowState.LastError != "webhook returned status 500" {
		t.Fatalf("expected last_error to persist retry failure, got %q", queueRowState.LastError)
	}
}

func countQueueRowsForEvent(t *testing.T, databaseConnection *sql.DB, eventID string) int {
	t.Helper()

	var queueRowCount int
	if queryError := databaseConnection.QueryRowContext(
		context.Background(),
		"SELECT COUNT(*) FROM webhook_delivery_queue WHERE event_id = $1",
		eventID,
	).Scan(&queueRowCount); queryError != nil {
		t.Fatalf("count queue rows for event %s: %v", eventID, queryError)
	}

	return queueRowCount
}

func queueStatusForEvent(t *testing.T, databaseConnection *sql.DB, eventID string) string {
	t.Helper()

	return queueRowStateForEvent(t, databaseConnection, eventID).Status
}

type queueRowState struct {
	Status     string
	RetryCount int
	LastError  string
}

func queueRowStateForEvent(t *testing.T, databaseConnection *sql.DB, eventID string) queueRowState {
	t.Helper()

	var queueState queueRowState
	if queryError := databaseConnection.QueryRowContext(
		context.Background(),
		"SELECT status, retry_count, COALESCE(last_error, '') FROM webhook_delivery_queue WHERE event_id = $1 ORDER BY id DESC LIMIT 1",
		eventID,
	).Scan(&queueState.Status, &queueState.RetryCount, &queueState.LastError); queryError != nil {
		t.Fatalf("read queue row state for event %s: %v", eventID, queryError)
	}

	return queueState
}

func assertDeliveredEventMatches(t *testing.T, expectedEvent events.SubscriberEvent, actualEvent events.SubscriberEvent) {
	t.Helper()

	if actualEvent.EventID != expectedEvent.EventID {
		t.Fatalf("expected delivered event ID %s, got %s", expectedEvent.EventID, actualEvent.EventID)
	}
	if actualEvent.CustomerID != expectedEvent.CustomerID {
		t.Fatalf("expected delivered customer ID %s, got %s", expectedEvent.CustomerID, actualEvent.CustomerID)
	}
	if actualEvent.SubscriberID != expectedEvent.SubscriberID {
		t.Fatalf("expected delivered subscriber ID %s, got %s", expectedEvent.SubscriberID, actualEvent.SubscriberID)
	}
	if actualEvent.EventType != expectedEvent.EventType {
		t.Fatalf("expected delivered event type %s, got %s", expectedEvent.EventType, actualEvent.EventType)
	}
	if !actualEvent.OccurredAt.Equal(expectedEvent.OccurredAt) {
		t.Fatalf("expected delivered occurred_at %s, got %s", expectedEvent.OccurredAt.UTC(), actualEvent.OccurredAt.UTC())
	}
}
