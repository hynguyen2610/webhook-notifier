package mockreceiver

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"webhook-notifier/internal/config"
	"webhook-notifier/internal/events"
)

func TestHandleWebhookTracksPerCustomerStatisticsForSupportedEventTypes(t *testing.T) {
	testCases := []struct {
		name      string
		eventType string
	}{
		{
			name:      "input subscriber.created expects stats count for subscriber.created",
			eventType: events.SubscriberCreatedEventType,
		},
		{
			name:      "input subscriber.added_to_segment expects stats count for subscriber.added_to_segment",
			eventType: events.SubscriberAddedToSegmentEventType,
		},
		{
			name:      "input subscriber.unsubscribed expects stats count for subscriber.unsubscribed",
			eventType: events.SubscriberUnsubscribedEventType,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			// Input: a webhook request for customer-a with one supported event type payload.
			// Outcome: receiver stats show one successful delivery, one eventType count, and the same event type in the last event snapshot.
			application := NewApplication(config.MockReceiverConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil)))

			subscriberEvent := events.SubscriberEvent{
				EventID:      "event-001",
				CustomerID:   "customer-a",
				SubscriberID: "subscriber-001",
				EventType:    testCase.eventType,
				OccurredAt:   time.Date(2026, time.August, 2, 10, 0, 0, 0, time.UTC),
			}
			requestBody, marshalError := json.Marshal(subscriberEvent)
			if marshalError != nil {
				t.Fatalf("marshal request body: %v", marshalError)
			}

			request := httptest.NewRequest(http.MethodPost, "/webhook/customer-a", bytes.NewReader(requestBody))
			request.SetPathValue("customerId", "customer-a")
			responseRecorder := httptest.NewRecorder()

			application.handleWebhook(responseRecorder, request)

			if responseRecorder.Code != http.StatusOK {
				t.Fatalf("expected status 200, got %d", responseRecorder.Code)
			}

			customerStatistic := application.snapshotCustomerStatistic("customer-a")
			if customerStatistic.Received != 1 {
				t.Fatalf("expected received count 1, got %d", customerStatistic.Received)
			}
			if customerStatistic.Success != 1 {
				t.Fatalf("expected success count 1, got %d", customerStatistic.Success)
			}
			if customerStatistic.PayloadDecodeFailures != 0 {
				t.Fatalf("expected zero payload decode failures, got %d", customerStatistic.PayloadDecodeFailures)
			}
			if customerStatistic.PathPayloadCustomerMismatches != 0 {
				t.Fatalf("expected zero customer mismatches, got %d", customerStatistic.PathPayloadCustomerMismatches)
			}
			if customerStatistic.EventTypeCounts[testCase.eventType] != 1 {
				t.Fatalf("expected %s count 1, got %d", testCase.eventType, customerStatistic.EventTypeCounts[testCase.eventType])
			}
			if customerStatistic.LastEvent == nil || customerStatistic.LastEvent.EventType != testCase.eventType {
				t.Fatalf("expected last event type %s, got %#v", testCase.eventType, customerStatistic.LastEvent)
			}
		})
	}
}

func TestHandleWebhookTracksDecodeFailuresAndCustomerMismatches(t *testing.T) {
	// Input: one invalid JSON request and one valid payload routed to the wrong customer path.
	// Outcome: receiver stats record one decode failure and one path-to-payload customer mismatch.
	application := NewApplication(config.MockReceiverConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	invalidRequest := httptest.NewRequest(http.MethodPost, "/webhook/customer-a", bytes.NewBufferString("{"))
	invalidRequest.SetPathValue("customerId", "customer-a")
	invalidResponseRecorder := httptest.NewRecorder()
	application.handleWebhook(invalidResponseRecorder, invalidRequest)

	mismatchedEvent := events.SubscriberEvent{
		EventID:      "event-002",
		CustomerID:   "customer-b",
		SubscriberID: "subscriber-002",
		EventType:    "subscriber.updated",
		OccurredAt:   time.Date(2026, time.August, 2, 10, 5, 0, 0, time.UTC),
	}
	requestBody, marshalError := json.Marshal(mismatchedEvent)
	if marshalError != nil {
		t.Fatalf("marshal mismatched event: %v", marshalError)
	}

	mismatchedRequest := httptest.NewRequest(http.MethodPost, "/webhook/customer-a", bytes.NewReader(requestBody))
	mismatchedRequest.SetPathValue("customerId", "customer-a")
	mismatchedResponseRecorder := httptest.NewRecorder()
	application.handleWebhook(mismatchedResponseRecorder, mismatchedRequest)

	customerStatistic := application.snapshotCustomerStatistic("customer-a")
	if customerStatistic.Received != 2 {
		t.Fatalf("expected received count 2, got %d", customerStatistic.Received)
	}
	if customerStatistic.PayloadDecodeFailures != 1 {
		t.Fatalf("expected payload decode failures 1, got %d", customerStatistic.PayloadDecodeFailures)
	}
	if customerStatistic.PathPayloadCustomerMismatches != 1 {
		t.Fatalf("expected customer mismatches 1, got %d", customerStatistic.PathPayloadCustomerMismatches)
	}
	if customerStatistic.EventTypeCounts["subscriber.updated"] != 1 {
		t.Fatalf("expected subscriber.updated count 1, got %d", customerStatistic.EventTypeCounts["subscriber.updated"])
	}
}
