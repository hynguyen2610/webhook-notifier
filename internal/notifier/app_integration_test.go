package notifier

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"webhook-notifier/internal/events"
)

func TestNotifierIntegrationDeliversSupportedEventTypesToWebhook(t *testing.T) {
	testCases := []struct {
		name      string
		eventType string
	}{
		{
			name:      "input subscriber.created expects webhook delivery",
			eventType: events.SubscriberCreatedEventType,
		},
		{
			name:      "input subscriber.added_to_segment expects webhook delivery",
			eventType: events.SubscriberAddedToSegmentEventType,
		},
		{
			name:      "input subscriber.unsubscribed expects webhook delivery",
			eventType: events.SubscriberUnsubscribedEventType,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			// Input: one valid subscriber event for a registered customer with one supported event type and a reachable webhook endpoint.
			// Outcome: notifier creates one delivery, posts the same event type to the webhook, and records one successful delivery.
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

			application := newTestApplication(
				map[string][]string{"customer-a": {webhookServer.URL}},
				1,
				2*time.Second,
				10*time.Millisecond,
				3,
			)

			requestContext, cancelRequest := context.WithCancel(context.Background())
			workers := startTestWorkers(requestContext, application, 1)

			testEvent := newTestEventWithType("customer-a", "event-001", testCase.eventType)

			createdJobs, enqueueError := application.enqueueEvents([]events.SubscriberEvent{testEvent})
			if enqueueError != nil {
				t.Fatalf("enqueue events: %v", enqueueError)
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
			workers.Wait()
		})
	}
}
