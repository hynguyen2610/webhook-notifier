package notifier

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"webhook-notifier/internal/events"
	"webhook-notifier/internal/httpx"
	"webhook-notifier/internal/registration"
)

func (application *Application) handleHealth(responseWriter http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(responseWriter, http.StatusOK, map[string]string{
		"status": "ok",
	})
}

func (application *Application) handleStats(responseWriter http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(responseWriter, http.StatusOK, map[string]int64{
		"receivedEvents":    application.receivedEvents.Load(),
		"deliveredEvents":   application.deliveredEvents.Load(),
		"failedDeliveries":  application.failedDeliveries.Load(),
		"retriedDeliveries": application.retriedDeliveries.Load(),
		"deadLetterCount":   application.deadLetterCount.Load(),
	})
}

func (application *Application) handleRegistrations(responseWriter http.ResponseWriter, _ *http.Request) {
	registrySnapshot, snapshotError := application.registry.Snapshot(context.Background())
	if snapshotError != nil {
		httpx.WriteError(responseWriter, http.StatusInternalServerError, snapshotError.Error())
		return
	}

	httpx.WriteJSON(responseWriter, http.StatusOK, registrySnapshot)
}

func (application *Application) handleDeadLetters(responseWriter http.ResponseWriter, _ *http.Request) {
	deadLetterSnapshot, snapshotError := application.workQueue.SnapshotDeadLetters(context.Background())
	if snapshotError != nil {
		httpx.WriteError(responseWriter, http.StatusInternalServerError, snapshotError.Error())
		return
	}
	httpx.WriteJSON(responseWriter, http.StatusOK, deadLetterSnapshot)
}

func (application *Application) handleSingleEvent(responseWriter http.ResponseWriter, request *http.Request) {
	var event events.SubscriberEvent
	if decodeError := json.NewDecoder(request.Body).Decode(&event); decodeError != nil {
		httpx.WriteError(responseWriter, http.StatusBadRequest, fmt.Sprintf("decode event: %v", decodeError))
		return
	}

	createdJobs, enqueueError := application.enqueueEvents([]events.SubscriberEvent{event})
	if enqueueError != nil {
		statusCode := http.StatusBadRequest
		if errors.Is(enqueueError, registration.ErrCustomerNotRegistered) {
			statusCode = http.StatusNotFound
		}
		httpx.WriteError(responseWriter, statusCode, enqueueError.Error())
		return
	}

	httpx.WriteJSON(responseWriter, http.StatusAccepted, ingestResponse{
		AcceptedEvents: 1,
		CreatedJobs:    createdJobs,
	})
}

func (application *Application) handleBatchEvents(responseWriter http.ResponseWriter, request *http.Request) {
	var eventsBatch []events.SubscriberEvent
	if decodeError := json.NewDecoder(request.Body).Decode(&eventsBatch); decodeError != nil {
		httpx.WriteError(responseWriter, http.StatusBadRequest, fmt.Sprintf("decode events: %v", decodeError))
		return
	}

	createdJobs, enqueueError := application.enqueueEvents(eventsBatch)
	if enqueueError != nil {
		statusCode := http.StatusBadRequest
		if errors.Is(enqueueError, registration.ErrCustomerNotRegistered) {
			statusCode = http.StatusNotFound
		}
		httpx.WriteError(responseWriter, statusCode, enqueueError.Error())
		return
	}

	httpx.WriteJSON(responseWriter, http.StatusAccepted, ingestResponse{
		AcceptedEvents: len(eventsBatch),
		CreatedJobs:    createdJobs,
	})
}

func (application *Application) enqueueEvents(subscriberEvents []events.SubscriberEvent) (int, error) {
	createdJobs := 0
	for _, subscriberEvent := range subscriberEvents {
		if validationError := validateEvent(subscriberEvent); validationError != nil {
			return createdJobs, validationError
		}

		webhookURLs, resolveError := application.registry.ResolveWebhookURLs(context.Background(), subscriberEvent.CustomerID)
		if resolveError != nil {
			return createdJobs, resolveError
		}

		application.logger.Info(
			"received subscriber event",
			"eventID", subscriberEvent.EventID,
			"customerID", subscriberEvent.CustomerID,
			"subscriberID", subscriberEvent.SubscriberID,
			"eventType", subscriberEvent.EventType,
			"occurredAt", subscriberEvent.OccurredAt,
			"webhookEndpointCount", len(webhookURLs),
		)
		application.receivedEvents.Add(1)
		application.notifierMetrics.ReceivedEventsCounter.Inc()
		enqueuedCount, enqueueError := application.workQueue.EnqueueDeliveries(
			context.Background(),
			subscriberEvent,
			webhookURLs,
			time.Now().UTC(),
		)
		if enqueueError != nil {
			return createdJobs, enqueueError
		}
		createdJobs += enqueuedCount
	}

	return createdJobs, nil
}
