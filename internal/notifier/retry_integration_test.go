package notifier

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"webhook-notifier/internal/events"
)

func TestNotifierIntegrationRetriesThenSucceeds(t *testing.T) {
	// Input: one valid event whose webhook returns HTTP 500 on the first attempt and HTTP 200 on the second.
	// Outcome: notifier retries once, delivers successfully on retry, and records one retry without dead-lettering.
	var attemptCount atomic.Int64
	deliveryRequests := make(chan events.SubscriberEvent, 2)

	webhookServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		var deliveredEvent events.SubscriberEvent
		if decodeError := json.NewDecoder(request.Body).Decode(&deliveredEvent); decodeError != nil {
			t.Fatalf("decode delivered event: %v", decodeError)
		}

		deliveryRequests <- deliveredEvent
		if attemptCount.Add(1) == 1 {
			http.Error(responseWriter, "temporary failure", http.StatusInternalServerError)
			return
		}

		responseWriter.WriteHeader(http.StatusOK)
	}))
	defer webhookServer.Close()

	application := newTestApplication(
		map[string][]string{"customer-a": {webhookServer.URL}},
		1,
		200*time.Millisecond,
		10*time.Millisecond,
		2,
	)

	requestContext, cancelRequest := context.WithCancel(context.Background())
	defer cancelRequest()
	workers := startTestWorkers(requestContext, application, 1)

	createdJobs, enqueueError := application.enqueueEvents([]events.SubscriberEvent{newTestEvent("customer-a", "event-retry-success")})
	if enqueueError != nil {
		t.Fatalf("enqueue events: %v", enqueueError)
	}
	if createdJobs != 1 {
		t.Fatalf("expected 1 created job, got %d", createdJobs)
	}

	waitForNotifierCount(t, func() int64 { return attemptCount.Load() }, 2, "attemptCount")
	waitForNotifierCount(t, func() int64 { return application.deliveredEvents.Load() }, 1, "deliveredEvents")
	waitForNotifierCount(t, func() int64 { return application.failedDeliveries.Load() }, 1, "failedDeliveries")
	waitForNotifierCount(t, func() int64 { return application.retriedDeliveries.Load() }, 1, "retriedDeliveries")

	if application.deadLetterCount.Load() != 0 {
		t.Fatalf("expected deadLetterCount 0, got %d", application.deadLetterCount.Load())
	}
	if len(drainDeliveredEvents(deliveryRequests)) != 2 {
		t.Fatalf("expected 2 delivery attempts to reach the webhook")
	}

	cancelRequest()
	workers.Wait()
}

func TestNotifierIntegrationRetriesTooManyRequestsThenSucceeds(t *testing.T) {
	// Input: one valid event whose webhook returns HTTP 429 on the first attempt and HTTP 200 on the second.
	// Outcome: notifier treats HTTP 429 as retryable, retries once, and delivers successfully without dead-lettering.
	var attemptCount atomic.Int64
	deliveryRequests := make(chan events.SubscriberEvent, 2)

	webhookServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		var deliveredEvent events.SubscriberEvent
		if decodeError := json.NewDecoder(request.Body).Decode(&deliveredEvent); decodeError != nil {
			t.Fatalf("decode delivered event: %v", decodeError)
		}

		deliveryRequests <- deliveredEvent
		if attemptCount.Add(1) == 1 {
			http.Error(responseWriter, "slow down", http.StatusTooManyRequests)
			return
		}

		responseWriter.WriteHeader(http.StatusOK)
	}))
	defer webhookServer.Close()

	application := newTestApplication(
		map[string][]string{"customer-a": {webhookServer.URL}},
		1,
		200*time.Millisecond,
		10*time.Millisecond,
		2,
	)

	requestContext, cancelRequest := context.WithCancel(context.Background())
	defer cancelRequest()
	workers := startTestWorkers(requestContext, application, 1)

	createdJobs, enqueueError := application.enqueueEvents([]events.SubscriberEvent{newTestEvent("customer-a", "event-retry-429-success")})
	if enqueueError != nil {
		t.Fatalf("enqueue events: %v", enqueueError)
	}
	if createdJobs != 1 {
		t.Fatalf("expected 1 created job, got %d", createdJobs)
	}

	waitForNotifierCount(t, func() int64 { return attemptCount.Load() }, 2, "attemptCount")
	waitForNotifierCount(t, func() int64 { return application.deliveredEvents.Load() }, 1, "deliveredEvents")
	waitForNotifierCount(t, func() int64 { return application.failedDeliveries.Load() }, 1, "failedDeliveries")
	waitForNotifierCount(t, func() int64 { return application.retriedDeliveries.Load() }, 1, "retriedDeliveries")

	if application.deadLetterCount.Load() != 0 {
		t.Fatalf("expected deadLetterCount 0, got %d", application.deadLetterCount.Load())
	}
	if len(drainDeliveredEvents(deliveryRequests)) != 2 {
		t.Fatalf("expected 2 delivery attempts to reach the webhook")
	}

	cancelRequest()
	workers.Wait()
}

func TestNotifierIntegrationRoutesExhaustedFailuresToDeadLetter(t *testing.T) {
	// Input: one valid event whose webhook always returns HTTP 500 with one allowed retry attempt.
	// Outcome: notifier retries once, exhausts retries, and records one dead-lettered delivery.
	var attemptCount atomic.Int64
	webhookServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		attemptCount.Add(1)
		http.Error(responseWriter, "persistent failure", http.StatusInternalServerError)
	}))
	defer webhookServer.Close()

	application := newTestApplication(
		map[string][]string{"customer-a": {webhookServer.URL}},
		1,
		200*time.Millisecond,
		10*time.Millisecond,
		1,
	)

	requestContext, cancelRequest := context.WithCancel(context.Background())
	defer cancelRequest()
	workers := startTestWorkers(requestContext, application, 1)

	createdJobs, enqueueError := application.enqueueEvents([]events.SubscriberEvent{newTestEvent("customer-a", "event-dead-letter")})
	if enqueueError != nil {
		t.Fatalf("enqueue events: %v", enqueueError)
	}
	if createdJobs != 1 {
		t.Fatalf("expected 1 created job, got %d", createdJobs)
	}

	waitForNotifierCount(t, func() int64 { return attemptCount.Load() }, 2, "attemptCount")
	waitForNotifierCount(t, func() int64 { return application.failedDeliveries.Load() }, 2, "failedDeliveries")
	waitForNotifierCount(t, func() int64 { return application.retriedDeliveries.Load() }, 1, "retriedDeliveries")
	waitForNotifierCount(t, func() int64 { return application.deadLetterCount.Load() }, 1, "deadLetterCount")

	application.deadLetterMutex.Lock()
	if len(application.deadLetters) != 1 {
		application.deadLetterMutex.Unlock()
		t.Fatalf("expected 1 dead letter entry, got %d", len(application.deadLetters))
	}
	deadLetterMessage := application.deadLetters[0]
	application.deadLetterMutex.Unlock()

	if deadLetterMessage.Job.Event.EventID != "event-dead-letter" {
		t.Fatalf("expected dead letter for event-dead-letter, got %s", deadLetterMessage.Job.Event.EventID)
	}
	if deadLetterMessage.FailureReason != "webhook returned status 500" {
		t.Fatalf("expected HTTP 500 dead letter reason, got %s", deadLetterMessage.FailureReason)
	}

	cancelRequest()
	workers.Wait()
}

func TestNotifierIntegrationDoesNotRetryNonRetryableClientError(t *testing.T) {
	// Input: one valid event whose webhook returns HTTP 404 on the first attempt.
	// Outcome: notifier treats HTTP 404 as a permanent failure, skips retrying, and records one dead-lettered delivery.
	var attemptCount atomic.Int64
	webhookServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		attemptCount.Add(1)
		http.Error(responseWriter, "not found", http.StatusNotFound)
	}))
	defer webhookServer.Close()

	application := newTestApplication(
		map[string][]string{"customer-a": {webhookServer.URL}},
		1,
		200*time.Millisecond,
		10*time.Millisecond,
		2,
	)

	requestContext, cancelRequest := context.WithCancel(context.Background())
	defer cancelRequest()
	workers := startTestWorkers(requestContext, application, 1)

	createdJobs, enqueueError := application.enqueueEvents([]events.SubscriberEvent{newTestEvent("customer-a", "event-no-retry-404")})
	if enqueueError != nil {
		t.Fatalf("enqueue events: %v", enqueueError)
	}
	if createdJobs != 1 {
		t.Fatalf("expected 1 created job, got %d", createdJobs)
	}

	waitForNotifierCount(t, func() int64 { return attemptCount.Load() }, 1, "attemptCount")
	waitForNotifierCount(t, func() int64 { return application.failedDeliveries.Load() }, 1, "failedDeliveries")
	waitForNotifierCount(t, func() int64 { return application.deadLetterCount.Load() }, 1, "deadLetterCount")

	if application.retriedDeliveries.Load() != 0 {
		t.Fatalf("expected retriedDeliveries 0, got %d", application.retriedDeliveries.Load())
	}

	application.deadLetterMutex.Lock()
	if len(application.deadLetters) != 1 {
		application.deadLetterMutex.Unlock()
		t.Fatalf("expected 1 dead letter entry, got %d", len(application.deadLetters))
	}
	deadLetterMessage := application.deadLetters[0]
	application.deadLetterMutex.Unlock()

	if deadLetterMessage.Job.Event.EventID != "event-no-retry-404" {
		t.Fatalf("expected dead letter for event-no-retry-404, got %s", deadLetterMessage.Job.Event.EventID)
	}
	if deadLetterMessage.FailureReason != "webhook returned status 404" {
		t.Fatalf("expected HTTP 404 dead letter reason, got %s", deadLetterMessage.FailureReason)
	}

	cancelRequest()
	workers.Wait()
}

func TestNotifierIntegrationDeadLettersSlowReceiverTimeout(t *testing.T) {
	// Input: one valid event whose webhook takes longer to respond than the notifier request timeout.
	// Outcome: notifier treats the timeout as a failure, skips retries when configured to do so, and records a dead letter.
	webhookServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		time.Sleep(150 * time.Millisecond)
		responseWriter.WriteHeader(http.StatusOK)
	}))
	defer webhookServer.Close()

	application := newTestApplication(
		map[string][]string{"customer-a": {webhookServer.URL}},
		1,
		50*time.Millisecond,
		10*time.Millisecond,
		0,
	)

	requestContext, cancelRequest := context.WithCancel(context.Background())
	defer cancelRequest()
	workers := startTestWorkers(requestContext, application, 1)

	createdJobs, enqueueError := application.enqueueEvents([]events.SubscriberEvent{newTestEvent("customer-a", "event-timeout")})
	if enqueueError != nil {
		t.Fatalf("enqueue events: %v", enqueueError)
	}
	if createdJobs != 1 {
		t.Fatalf("expected 1 created job, got %d", createdJobs)
	}

	waitForNotifierCount(t, func() int64 { return application.failedDeliveries.Load() }, 1, "failedDeliveries")
	waitForNotifierCount(t, func() int64 { return application.deadLetterCount.Load() }, 1, "deadLetterCount")

	application.deadLetterMutex.Lock()
	if len(application.deadLetters) != 1 {
		application.deadLetterMutex.Unlock()
		t.Fatalf("expected 1 dead letter entry, got %d", len(application.deadLetters))
	}
	failureReason := application.deadLetters[0].FailureReason
	application.deadLetterMutex.Unlock()

	if !strings.Contains(strings.ToLower(failureReason), "timeout") {
		t.Fatalf("expected timeout failure reason, got %s", failureReason)
	}
	if application.retriedDeliveries.Load() != 0 {
		t.Fatalf("expected retriedDeliveries 0, got %d", application.retriedDeliveries.Load())
	}

	cancelRequest()
	workers.Wait()
}

func drainDeliveredEvents(deliveryRequests <-chan events.SubscriberEvent) []events.SubscriberEvent {
	deliveredEvents := make([]events.SubscriberEvent, 0)
	for {
		select {
		case deliveredEvent := <-deliveryRequests:
			deliveredEvents = append(deliveredEvents, deliveredEvent)
		default:
			return deliveredEvents
		}
	}
}
