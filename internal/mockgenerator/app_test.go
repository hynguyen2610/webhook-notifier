package mockgenerator

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

func TestNewEventUsesDeterministicSeededData(t *testing.T) {
	// Input: two generator applications created with the same seed and the same event request.
	// Outcome: both applications emit the same deterministic event payload and timestamp.
	firstApplication := NewApplication(
		config.MockGeneratorConfig{RandomSeed: 12345},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	secondApplication := NewApplication(
		config.MockGeneratorConfig{RandomSeed: 12345},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	firstEvent := firstApplication.newEvent("customer-a", "subscriber.created", 0)
	secondEvent := secondApplication.newEvent("customer-a", "subscriber.created", 0)

	if firstEvent != secondEvent {
		t.Fatalf("expected deterministic event output, got %#v and %#v", firstEvent, secondEvent)
	}
	if !firstEvent.OccurredAt.Equal(time.Unix(0, 12345).UTC()) {
		t.Fatalf("expected occurredAt %s, got %s", time.Unix(0, 12345).UTC(), firstEvent.OccurredAt)
	}
}

func TestHandleGenerateRejectsMissingCustomerID(t *testing.T) {
	// Input: a generate request with no customerId.
	// Outcome: the handler returns 400 with a useful validation error.
	application := NewApplication(config.MockGeneratorConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	request := httptest.NewRequest(http.MethodPost, "/generate", bytes.NewBufferString(`{"count":1}`))
	responseRecorder := httptest.NewRecorder()

	application.handleGenerate(responseRecorder, request)

	if responseRecorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", responseRecorder.Code)
	}

	var errorResponse map[string]any
	if decodeError := json.Unmarshal(responseRecorder.Body.Bytes(), &errorResponse); decodeError != nil {
		t.Fatalf("decode error response: %v", decodeError)
	}
	if errorResponse["error"] != "customerId is required" {
		t.Fatalf("expected customerId validation error, got %#v", errorResponse["error"])
	}
}

func TestHandleGenerateBulkRejectsNegativeEventsPerCustomer(t *testing.T) {
	// Input: a bulk generate request with a negative eventsPerCustomer value.
	// Outcome: the handler returns 400 and does not silently coerce the invalid value.
	application := NewApplication(config.MockGeneratorConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	request := httptest.NewRequest(http.MethodPost, "/generate/bulk", bytes.NewBufferString(`{"customers":2,"eventsPerCustomer":-1}`))
	responseRecorder := httptest.NewRecorder()

	application.handleGenerateBulk(responseRecorder, request)

	if responseRecorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", responseRecorder.Code)
	}

	var errorResponse map[string]any
	if decodeError := json.Unmarshal(responseRecorder.Body.Bytes(), &errorResponse); decodeError != nil {
		t.Fatalf("decode error response: %v", decodeError)
	}
	if errorResponse["error"] != "eventsPerCustomer must be zero or greater" {
		t.Fatalf("expected eventsPerCustomer validation error, got %#v", errorResponse["error"])
	}
}

func TestHandleGeneratePublishesDeterministicEventsToNotifier(t *testing.T) {
	// Input: a seeded generate request for customer-a with count 2 and a notifier batch endpoint.
	// Outcome: the handler forwards two deterministic events to the notifier batch endpoint and returns 202.
	var publishedEvents []events.SubscriberEvent
	notifierServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/events/batch" {
			t.Fatalf("expected /events/batch path, got %s", request.URL.Path)
		}
		if decodeError := json.NewDecoder(request.Body).Decode(&publishedEvents); decodeError != nil {
			t.Fatalf("decode published events: %v", decodeError)
		}
		responseWriter.WriteHeader(http.StatusAccepted)
	}))
	defer notifierServer.Close()

	application := NewApplication(
		config.MockGeneratorConfig{
			NotifierBaseURL: notifierServer.URL,
			RandomSeed:      7,
		},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	request := httptest.NewRequest(http.MethodPost, "/generate", bytes.NewBufferString(`{"customerId":"customer-a","eventType":"subscriber.created","count":2}`))
	responseRecorder := httptest.NewRecorder()

	application.handleGenerate(responseRecorder, request)

	if responseRecorder.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d", responseRecorder.Code)
	}
	if len(publishedEvents) != 2 {
		t.Fatalf("expected 2 published events, got %d", len(publishedEvents))
	}
	if publishedEvents[0].EventID != "seed-7-000000-000000" {
		t.Fatalf("expected first deterministic event ID, got %s", publishedEvents[0].EventID)
	}
	if publishedEvents[1].EventID != "seed-7-000001-000001" {
		t.Fatalf("expected second deterministic event ID, got %s", publishedEvents[1].EventID)
	}
	if !publishedEvents[1].OccurredAt.After(publishedEvents[0].OccurredAt) {
		t.Fatalf("expected second event timestamp after first, got %s and %s", publishedEvents[0].OccurredAt, publishedEvents[1].OccurredAt)
	}
}
