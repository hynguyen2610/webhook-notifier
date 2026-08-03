package mockgenerator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"webhook-notifier/internal/events"
)

func (application *Application) newEvent(customerID string, eventType string, eventIndex int) events.SubscriberEvent {
	application.generationStateMutex.Lock()
	subscriberID := fmt.Sprintf("subscriber-%06d", application.randomSource.Intn(999999))
	eventSequence := application.generatedEventCount
	occurredAt := application.baseOccurredAt.Add(time.Duration(eventSequence) * time.Millisecond)
	eventID := fmt.Sprintf("seed-%d-%06d-%06d", application.config.RandomSeed, eventSequence, eventIndex)
	application.generatedEventCount++
	application.generationStateMutex.Unlock()

	return events.SubscriberEvent{
		EventID:      eventID,
		CustomerID:   customerID,
		SubscriberID: subscriberID,
		EventType:    eventType,
		OccurredAt:   occurredAt,
	}
}

func (application *Application) publishEvents(requestContext context.Context, eventsBatch []events.SubscriberEvent) error {
	if application.publisher != nil {
		return application.publisher.Publish(eventsBatch)
	}

	requestBody, marshalError := json.Marshal(eventsBatch)
	if marshalError != nil {
		return marshalError
	}

	request, requestError := http.NewRequestWithContext(requestContext, http.MethodPost, application.config.NotifierBaseURL+"/events/batch", bytes.NewReader(requestBody))
	if requestError != nil {
		return requestError
	}
	request.Header.Set("Content-Type", "application/json")

	response, responseError := application.httpClient.Do(request)
	if responseError != nil {
		return responseError
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("notifier returned status %d", response.StatusCode)
	}

	return nil
}
