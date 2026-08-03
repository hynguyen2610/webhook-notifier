package mockgenerator

import (
	"fmt"
	"net/http"

	"webhook-notifier/internal/events"
	"webhook-notifier/internal/httpx"
)

func (application *Application) handleGenerate(responseWriter http.ResponseWriter, request *http.Request) {
	var generateRequest GenerateRequest
	if decodeError := decodeJSONRequest(request, &generateRequest); decodeError != nil {
		httpx.WriteError(responseWriter, http.StatusBadRequest, decodeError.Error())
		return
	}
	if validationError := validateGenerateRequest(generateRequest); validationError != nil {
		httpx.WriteError(responseWriter, http.StatusBadRequest, validationError.Error())
		return
	}

	if generateRequest.Count <= 0 {
		generateRequest.Count = 1
	}
	if generateRequest.EventType == "" {
		generateRequest.EventType = "subscriber.created"
	}

	eventsBatch := make([]events.SubscriberEvent, 0, generateRequest.Count)
	for eventIndex := 0; eventIndex < generateRequest.Count; eventIndex++ {
		eventsBatch = append(eventsBatch, application.newEvent(generateRequest.CustomerID, generateRequest.EventType, eventIndex))
	}

	if publishError := application.publishEvents(request.Context(), eventsBatch); publishError != nil {
		httpx.WriteError(responseWriter, http.StatusBadGateway, publishError.Error())
		return
	}

	httpx.WriteJSON(responseWriter, http.StatusAccepted, PublishResponse{Generated: len(eventsBatch)})
}

func (application *Application) handleGenerateBulk(responseWriter http.ResponseWriter, request *http.Request) {
	var bulkRequest BulkGenerateRequest
	if decodeError := decodeJSONRequest(request, &bulkRequest); decodeError != nil {
		httpx.WriteError(responseWriter, http.StatusBadRequest, decodeError.Error())
		return
	}
	if validationError := validateBulkGenerateRequest(bulkRequest); validationError != nil {
		httpx.WriteError(responseWriter, http.StatusBadRequest, validationError.Error())
		return
	}

	if bulkRequest.Customers <= 0 {
		bulkRequest.Customers = application.config.DefaultCustomerCount
	}
	if bulkRequest.EventsPerCustomer <= 0 {
		bulkRequest.EventsPerCustomer = 100
	}

	eventsBatch := make([]events.SubscriberEvent, 0, bulkRequest.Customers*bulkRequest.EventsPerCustomer)
	for customerIndex := 0; customerIndex < bulkRequest.Customers; customerIndex++ {
		customerID := fmt.Sprintf("customer-%02d", customerIndex+1)
		for eventIndex := 0; eventIndex < bulkRequest.EventsPerCustomer; eventIndex++ {
			eventsBatch = append(eventsBatch, application.newEvent(customerID, "subscriber.updated", eventIndex))
		}
	}

	if publishError := application.publishEvents(request.Context(), eventsBatch); publishError != nil {
		httpx.WriteError(responseWriter, http.StatusBadGateway, publishError.Error())
		return
	}

	httpx.WriteJSON(responseWriter, http.StatusAccepted, PublishResponse{Generated: len(eventsBatch)})
}

func (application *Application) handleWhaleScenario(responseWriter http.ResponseWriter, request *http.Request) {
	eventsBatch := make([]events.SubscriberEvent, 0, 2200)
	for eventIndex := 0; eventIndex < 2000; eventIndex++ {
		eventsBatch = append(eventsBatch, application.newEvent("customer-a", "subscriber.updated", eventIndex))
	}
	for customerIndex := 0; customerIndex < 2; customerIndex++ {
		customerID := fmt.Sprintf("customer-%c", 'b'+customerIndex)
		for eventIndex := 0; eventIndex < 100; eventIndex++ {
			eventsBatch = append(eventsBatch, application.newEvent(customerID, "subscriber.updated", eventIndex))
		}
	}

	if publishError := application.publishEvents(request.Context(), eventsBatch); publishError != nil {
		httpx.WriteError(responseWriter, http.StatusBadGateway, publishError.Error())
		return
	}

	httpx.WriteJSON(responseWriter, http.StatusAccepted, PublishResponse{Generated: len(eventsBatch)})
}

func (application *Application) handleMixedScenario(responseWriter http.ResponseWriter, request *http.Request) {
	eventsBatch := make([]events.SubscriberEvent, 0, application.config.DefaultCustomerCount*75)
	for customerIndex := 0; customerIndex < application.config.DefaultCustomerCount; customerIndex++ {
		customerID := fmt.Sprintf("customer-%02d", customerIndex+1)
		eventCount := 25 + application.randomSource.Intn(75)
		for eventIndex := 0; eventIndex < eventCount; eventIndex++ {
			eventTypes := []string{"subscriber.created", "subscriber.updated", "subscriber.deleted", "subscriber.unsubscribed"}
			eventType := eventTypes[application.randomSource.Intn(len(eventTypes))]
			eventsBatch = append(eventsBatch, application.newEvent(customerID, eventType, eventIndex))
		}
	}

	if publishError := application.publishEvents(request.Context(), eventsBatch); publishError != nil {
		httpx.WriteError(responseWriter, http.StatusBadGateway, publishError.Error())
		return
	}

	httpx.WriteJSON(responseWriter, http.StatusAccepted, PublishResponse{Generated: len(eventsBatch)})
}

func (application *Application) handleHealth(responseWriter http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(responseWriter, http.StatusOK, map[string]string{
		"status": "ok",
	})
}
