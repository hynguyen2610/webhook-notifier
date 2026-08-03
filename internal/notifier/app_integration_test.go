package notifier

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"webhook-notifier/internal/config"
	"webhook-notifier/internal/delivery"
	"webhook-notifier/internal/events"
	"webhook-notifier/internal/retry"
	"webhook-notifier/internal/scheduler"
)

func TestNotifierIntegrationDeliversRegisteredEventToWebhook(t *testing.T) {
	// Input: one valid subscriber event for a registered customer with a reachable webhook endpoint.
	// Outcome: notifier creates one delivery, posts the event to the webhook, and records one successful delivery.
	deliveryRequests := make(chan events.SubscriberEvent, 1)
	webhookServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Fatalf("expected POST request, got %s", request.Method)
		}

		var deliveredEvent events.SubscriberEvent
		if decodeError := json.NewDecoder(request.Body).Decode(&deliveredEvent); decodeError != nil {
			t.Fatalf("decode delivered event: %v", decodeError)
		}

		deliveryRequests <- deliveredEvent
		responseWriter.WriteHeader(http.StatusOK)
	}))
	defer webhookServer.Close()

	application := &Application{
		config: config.NotifierConfig{
			WorkerCount:      1,
			RequestTimeout:   2 * time.Second,
			MaxRetryAttempts: 3,
		},
		logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		registry:       testRegistry{webhookURLsByCustomerID: map[string][]string{"customer-a": {webhookServer.URL}}},
		scheduler:      scheduler.NewRoundRobinScheduler(4),
		deliveryClient: delivery.NewHTTPClient(2 * time.Second),
		retryPolicy: retry.ExponentialBackoffPolicy{
			InitialDelay:    10 * time.Millisecond,
			MaxRetryAttempt: 3,
		},
		notifierMetrics: newTestNotifierMetrics(),
	}

	requestContext, cancelRequest := context.WithCancel(context.Background())
	defer cancelRequest()

	scheduledJobs := application.scheduler.Start(requestContext)

	var workerGroup sync.WaitGroup
	workerGroup.Add(1)
	go func() {
		defer workerGroup.Done()
		application.runWorker(requestContext, 1, scheduledJobs)
	}()

	testEvent := events.SubscriberEvent{
		EventID:      "event-001",
		CustomerID:   "customer-a",
		SubscriberID: "subscriber-001",
		EventType:    "subscriber.created",
		OccurredAt:   time.Date(2026, time.August, 3, 9, 0, 0, 0, time.UTC),
	}

	createdJobs, ingestError := application.ingestEvents([]events.SubscriberEvent{testEvent})
	if ingestError != nil {
		t.Fatalf("ingest events: %v", ingestError)
	}
	if createdJobs != 1 {
		t.Fatalf("expected 1 created job, got %d", createdJobs)
	}

	select {
	case deliveredEvent := <-deliveryRequests:
		if deliveredEvent != testEvent {
			t.Fatalf("expected delivered event %#v, got %#v", testEvent, deliveredEvent)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for webhook delivery")
	}

	waitForNotifierCount(t, func() int64 { return application.deliveredEvents.Load() }, 1, "deliveredEvents")
	if application.receivedEvents.Load() != 1 {
		t.Fatalf("expected receivedEvents 1, got %d", application.receivedEvents.Load())
	}
	if application.failedDeliveries.Load() != 0 {
		t.Fatalf("expected failedDeliveries 0, got %d", application.failedDeliveries.Load())
	}
	if application.deadLetterCount.Load() != 0 {
		t.Fatalf("expected deadLetterCount 0, got %d", application.deadLetterCount.Load())
	}

	cancelRequest()
	workerGroup.Wait()
}
