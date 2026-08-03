package notifier

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"webhook-notifier/internal/events"
)

func TestNotifierIntegrationPreservesProgressDuringWhaleScenario(t *testing.T) {
	// Input: ten events for customer-a and two events each for customer-b and customer-c queued before worker start.
	// Outcome: the first six deliveries alternate across customers so smaller customers make progress before the whale drains.
	deliveryOrder := make(chan string, 14)
	webhookServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		var deliveredEvent events.SubscriberEvent
		if decodeError := json.NewDecoder(request.Body).Decode(&deliveredEvent); decodeError != nil {
			t.Fatalf("decode delivered event: %v", decodeError)
		}

		deliveryOrder <- deliveredEvent.CustomerID
		responseWriter.WriteHeader(http.StatusOK)
	}))
	defer webhookServer.Close()

	application := newTestApplication(
		map[string][]string{
			"customer-a": {webhookServer.URL},
			"customer-b": {webhookServer.URL},
			"customer-c": {webhookServer.URL},
		},
		1,
		200*time.Millisecond,
		10*time.Millisecond,
		0,
	)

	whaleEvents := make([]events.SubscriberEvent, 0, 14)
	for eventIndex := 0; eventIndex < 10; eventIndex++ {
		whaleEvents = append(whaleEvents, newTestEvent("customer-a", "event-a-"+strconv.Itoa(eventIndex)))
	}
	for eventIndex := 0; eventIndex < 2; eventIndex++ {
		whaleEvents = append(whaleEvents, newTestEvent("customer-b", "event-b-"+strconv.Itoa(eventIndex)))
		whaleEvents = append(whaleEvents, newTestEvent("customer-c", "event-c-"+strconv.Itoa(eventIndex)))
	}

	createdJobs, ingestError := application.ingestEvents(whaleEvents)
	if ingestError != nil {
		t.Fatalf("ingest whale events: %v", ingestError)
	}
	if createdJobs != len(whaleEvents) {
		t.Fatalf("expected %d created jobs, got %d", len(whaleEvents), createdJobs)
	}

	requestContext, cancelRequest := context.WithCancel(context.Background())
	defer cancelRequest()
	workers := startTestWorkers(requestContext, application, 1)

	actualOrder := make([]string, 0, 6)
	for len(actualOrder) < 6 {
		select {
		case customerID := <-deliveryOrder:
			actualOrder = append(actualOrder, customerID)
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for delivery order, got %#v", actualOrder)
		}
	}

	expectedOrder := []string{
		"customer-a",
		"customer-b",
		"customer-c",
		"customer-a",
		"customer-b",
		"customer-c",
	}
	for orderIndex := range expectedOrder {
		if actualOrder[orderIndex] != expectedOrder[orderIndex] {
			t.Fatalf("unexpected delivery order: got %#v want %#v", actualOrder, expectedOrder)
		}
	}

	cancelRequest()
	workers.Wait()
}
