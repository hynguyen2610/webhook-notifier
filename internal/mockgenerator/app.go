package mockgenerator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"time"

	"webhook-notifier/internal/config"
	"webhook-notifier/internal/events"
	"webhook-notifier/internal/httpx"
	"webhook-notifier/internal/kafka"
)

type Application struct {
	config       config.MockGeneratorConfig
	logger       *slog.Logger
	httpServer   *http.Server
	httpClient   *http.Client
	randomSource *rand.Rand
	publisher    kafka.Publisher
}

type GenerateRequest struct {
	CustomerID string `json:"customerId"`
	EventType  string `json:"eventType"`
	Count      int    `json:"count"`
}

type BulkGenerateRequest struct {
	Customers         int `json:"customers"`
	EventsPerCustomer int `json:"eventsPerCustomer"`
}

type PublishResponse struct {
	Generated int `json:"generated"`
}

func NewApplication(applicationConfig config.MockGeneratorConfig, logger *slog.Logger) *Application {
	application := &Application{
		config:       applicationConfig,
		logger:       logger,
		httpClient:   &http.Client{Timeout: 15 * time.Second},
		randomSource: rand.New(rand.NewSource(applicationConfig.RandomSeed)),
	}

	if len(applicationConfig.KafkaBrokers) > 0 {
		application.publisher = kafka.NewEventPublisher(
			applicationConfig.KafkaBrokers,
			applicationConfig.KafkaHostOverrides,
			applicationConfig.KafkaTopic,
		)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /generate", application.handleGenerate)
	mux.HandleFunc("POST /generate/bulk", application.handleGenerateBulk)
	mux.HandleFunc("POST /scenario/whale", application.handleWhaleScenario)
	mux.HandleFunc("POST /scenario/mixed", application.handleMixedScenario)
	mux.HandleFunc("GET /health", application.handleHealth)

	application.httpServer = &http.Server{
		Addr:    applicationConfig.HTTPAddress,
		Handler: mux,
	}

	return application
}

func (application *Application) Run(requestContext context.Context) error {
	serverErrors := make(chan error, 1)
	go func() {
		application.logger.Info("starting mock generator", "address", application.config.HTTPAddress, "notifierBaseURL", application.config.NotifierBaseURL)
		listenError := application.httpServer.ListenAndServe()
		if listenError != nil && !errors.Is(listenError, http.ErrServerClosed) {
			serverErrors <- listenError
			return
		}
		serverErrors <- nil
	}()

	select {
	case <-requestContext.Done():
	case serverError := <-serverErrors:
		if serverError != nil {
			return serverError
		}
	}

	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()

	return application.httpServer.Shutdown(shutdownContext)
}

func (application *Application) handleGenerate(responseWriter http.ResponseWriter, request *http.Request) {
	var generateRequest GenerateRequest
	if decodeError := json.NewDecoder(request.Body).Decode(&generateRequest); decodeError != nil {
		httpx.WriteError(responseWriter, http.StatusBadRequest, decodeError.Error())
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
	if decodeError := json.NewDecoder(request.Body).Decode(&bulkRequest); decodeError != nil {
		httpx.WriteError(responseWriter, http.StatusBadRequest, decodeError.Error())
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

func (application *Application) newEvent(customerID string, eventType string, eventIndex int) events.SubscriberEvent {
	return events.SubscriberEvent{
		EventID:      fmt.Sprintf("%d-%06d", time.Now().UnixNano(), eventIndex),
		CustomerID:   customerID,
		SubscriberID: fmt.Sprintf("subscriber-%06d", application.randomSource.Intn(999999)),
		EventType:    eventType,
		OccurredAt:   time.Now().UTC(),
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
