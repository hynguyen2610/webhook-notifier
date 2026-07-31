package notifier

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"webhook-notifier/internal/config"
	"webhook-notifier/internal/delivery"
	"webhook-notifier/internal/events"
	"webhook-notifier/internal/httpx"
	"webhook-notifier/internal/registration"
	"webhook-notifier/internal/retry"
	"webhook-notifier/internal/scheduler"
)

type Application struct {
	config            config.NotifierConfig
	logger            *slog.Logger
	registry          *registration.MemoryRegistry
	scheduler         *scheduler.RoundRobinScheduler
	deliveryClient    *delivery.HTTPClient
	retryPolicy       retry.ExponentialBackoffPolicy
	httpServer        *http.Server
	deadLetterMutex   sync.Mutex
	deadLetters       []events.DeadLetterMessage
	receivedEvents    atomic.Int64
	deliveredEvents   atomic.Int64
	failedDeliveries  atomic.Int64
	retriedDeliveries atomic.Int64
	deadLetterCount   atomic.Int64
}

type ingestResponse struct {
	AcceptedEvents int `json:"acceptedEvents"`
	CreatedJobs    int `json:"createdJobs"`
}

func NewApplication(applicationConfig config.NotifierConfig, logger *slog.Logger) *Application {
	application := &Application{
		config:         applicationConfig,
		logger:         logger,
		registry:       registration.NewMemoryRegistry(applicationConfig.WebhookRegistrations),
		scheduler:      scheduler.NewRoundRobinScheduler(applicationConfig.WorkerCount * 4),
		deliveryClient: delivery.NewHTTPClient(applicationConfig.RequestTimeout),
		retryPolicy: retry.ExponentialBackoffPolicy{
			InitialDelay:    applicationConfig.InitialRetryDelay,
			MaxRetryAttempt: applicationConfig.MaxRetryAttempts,
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", application.handleHealth)
	mux.HandleFunc("GET /stats", application.handleStats)
	mux.HandleFunc("GET /registrations", application.handleRegistrations)
	mux.HandleFunc("GET /dlq", application.handleDeadLetters)
	mux.HandleFunc("POST /events", application.handleSingleEvent)
	mux.HandleFunc("POST /events/batch", application.handleBatchEvents)

	application.httpServer = &http.Server{
		Addr:    applicationConfig.HTTPAddress,
		Handler: mux,
	}

	return application
}

func (application *Application) Run(requestContext context.Context) error {
	scheduledJobs := application.scheduler.Start(requestContext)

	var workerGroup sync.WaitGroup
	for workerIndex := 0; workerIndex < application.config.WorkerCount; workerIndex++ {
		workerGroup.Add(1)
		go func(workerID int) {
			defer workerGroup.Done()
			application.runWorker(requestContext, workerID, scheduledJobs)
		}(workerIndex + 1)
	}

	serverErrors := make(chan error, 1)
	go func() {
		application.logger.Info("starting notifier", "address", application.config.HTTPAddress, "workerCount", application.config.WorkerCount)
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
			application.scheduler.Close()
			return serverError
		}
	}

	application.scheduler.Close()

	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()

	shutdownError := application.httpServer.Shutdown(shutdownContext)
	workerGroup.Wait()
	return shutdownError
}

func (application *Application) runWorker(requestContext context.Context, workerID int, scheduledJobs <-chan events.DeliveryJob) {
	for {
		select {
		case <-requestContext.Done():
			return
		case job, channelOpen := <-scheduledJobs:
			if !channelOpen {
				return
			}

			result := application.deliveryClient.Deliver(requestContext, job)
			application.logDeliveryResult(workerID, result)

			if result.StatusCode >= 200 && result.StatusCode < 300 {
				application.deliveredEvents.Add(1)
				continue
			}

			application.failedDeliveries.Add(1)
			if result.ShouldRetry && application.retryPolicy.CanRetry(job.Attempt) {
				application.retriedDeliveries.Add(1)
				nextJob := job
				nextJob.Attempt++
				nextJob.LastError = result.FailureReason
				delay := application.retryPolicy.NextDelay(nextJob.Attempt - 1)
				time.AfterFunc(delay, func() {
					application.scheduler.Enqueue(nextJob)
				})
				continue
			}

			application.deadLetterCount.Add(1)
			application.deadLetterMutex.Lock()
			application.deadLetters = append(application.deadLetters, events.DeadLetterMessage{
				Job:           job,
				FailureReason: result.FailureReason,
				ExhaustedAt:   time.Now(),
			})
			application.deadLetterMutex.Unlock()
		}
	}
}

func (application *Application) logDeliveryResult(workerID int, result events.DeliveryResult) {
	logValues := []any{
		"workerID", workerID,
		"eventID", result.Job.Event.EventID,
		"customerID", result.Job.Event.CustomerID,
		"webhookURL", result.Job.WebhookURL,
		"retryCount", result.Job.Attempt,
		"duration", result.Duration.String(),
		"statusCode", result.StatusCode,
	}

	if result.StatusCode >= 200 && result.StatusCode < 300 {
		application.logger.Info("delivered webhook", logValues...)
		return
	}

	logValues = append(logValues, "failureReason", result.FailureReason)
	application.logger.Warn("webhook delivery failed", logValues...)
}

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
	httpx.WriteJSON(responseWriter, http.StatusOK, application.registry.Snapshot())
}

func (application *Application) handleDeadLetters(responseWriter http.ResponseWriter, _ *http.Request) {
	application.deadLetterMutex.Lock()
	defer application.deadLetterMutex.Unlock()

	deadLetterSnapshot := append([]events.DeadLetterMessage(nil), application.deadLetters...)
	httpx.WriteJSON(responseWriter, http.StatusOK, deadLetterSnapshot)
}

func (application *Application) handleSingleEvent(responseWriter http.ResponseWriter, request *http.Request) {
	var event events.SubscriberEvent
	if decodeError := json.NewDecoder(request.Body).Decode(&event); decodeError != nil {
		httpx.WriteError(responseWriter, http.StatusBadRequest, fmt.Sprintf("decode event: %v", decodeError))
		return
	}

	createdJobs, ingestError := application.ingestEvents([]events.SubscriberEvent{event})
	if ingestError != nil {
		statusCode := http.StatusBadRequest
		if errors.Is(ingestError, registration.ErrCustomerNotRegistered) {
			statusCode = http.StatusNotFound
		}
		httpx.WriteError(responseWriter, statusCode, ingestError.Error())
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

	createdJobs, ingestError := application.ingestEvents(eventsBatch)
	if ingestError != nil {
		statusCode := http.StatusBadRequest
		if errors.Is(ingestError, registration.ErrCustomerNotRegistered) {
			statusCode = http.StatusNotFound
		}
		httpx.WriteError(responseWriter, statusCode, ingestError.Error())
		return
	}

	httpx.WriteJSON(responseWriter, http.StatusAccepted, ingestResponse{
		AcceptedEvents: len(eventsBatch),
		CreatedJobs:    createdJobs,
	})
}

func (application *Application) ingestEvents(subscriberEvents []events.SubscriberEvent) (int, error) {
	createdJobs := 0
	for _, subscriberEvent := range subscriberEvents {
		if validationError := validateEvent(subscriberEvent); validationError != nil {
			return createdJobs, validationError
		}

		webhookURLs, resolveError := application.registry.ResolveWebhookURLs(subscriberEvent.CustomerID)
		if resolveError != nil {
			return createdJobs, resolveError
		}

		application.receivedEvents.Add(1)
		for _, webhookURL := range webhookURLs {
			application.scheduler.Enqueue(events.DeliveryJob{
				Event:      subscriberEvent,
				WebhookURL: webhookURL,
				EnqueuedAt: time.Now(),
				TraceID:    subscriberEvent.EventID,
			})
			createdJobs++
		}
	}

	return createdJobs, nil
}

func validateEvent(subscriberEvent events.SubscriberEvent) error {
	if strings.TrimSpace(subscriberEvent.EventID) == "" {
		return errors.New("eventId is required")
	}
	if strings.TrimSpace(subscriberEvent.CustomerID) == "" {
		return errors.New("customerId is required")
	}
	if strings.TrimSpace(subscriberEvent.SubscriberID) == "" {
		return errors.New("subscriberId is required")
	}
	if strings.TrimSpace(subscriberEvent.EventType) == "" {
		return errors.New("eventType is required")
	}
	if subscriberEvent.OccurredAt.IsZero() {
		return errors.New("occurredAt is required")
	}

	return nil
}
