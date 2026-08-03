package notifier

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"webhook-notifier/internal/config"
	"webhook-notifier/internal/delivery"
	"webhook-notifier/internal/events"
	"webhook-notifier/internal/metrics"
	"webhook-notifier/internal/registration"
	"webhook-notifier/internal/retry"
	"webhook-notifier/internal/scheduler"
	"webhook-notifier/internal/workqueue"
)

type Application struct {
	config            config.NotifierConfig
	logger            *slog.Logger
	registry          registration.Registry
	workQueue         workqueue.Repository
	scheduler         *scheduler.RoundRobinScheduler
	deliveryClient    deliveryClient
	retryPolicy       retry.ExponentialBackoffPolicy
	notifierMetrics   *metrics.NotifierMetrics
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

func NewApplication(applicationConfig config.NotifierConfig, logger *slog.Logger) (*Application, error) {
	registryStore, registryError := registration.NewPostgresRegistry(
		applicationConfig.PostgresConnection,
		applicationConfig.RegistrationResolveQuery,
		applicationConfig.RegistrationSnapshotQuery,
	)
	if registryError != nil {
		return nil, registryError
	}

	if pingError := registryStore.Ping(context.Background()); pingError != nil {
		return nil, pingError
	}

	queueRepository, queueError := workqueue.NewPostgresRepository(applicationConfig.PostgresConnection)
	if queueError != nil {
		return nil, queueError
	}
	if schemaError := queueRepository.EnsureSchema(context.Background()); schemaError != nil {
		return nil, schemaError
	}

	application := &Application{
		config:          applicationConfig,
		logger:          logger,
		registry:        registryStore,
		workQueue:       queueRepository,
		scheduler:       scheduler.NewRoundRobinScheduler(applicationConfig.WorkerCount * 4),
		deliveryClient:  delivery.NewHTTPClient(applicationConfig.RequestTimeout),
		notifierMetrics: metrics.NewNotifierMetrics(),
		retryPolicy: retry.ExponentialBackoffPolicy{
			InitialDelay:    applicationConfig.InitialRetryDelay,
			MaxRetryAttempt: applicationConfig.MaxRetryAttempts,
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", application.handleHealth)
	mux.Handle("GET /metrics", metrics.Handler())
	mux.HandleFunc("GET /stats", application.handleStats)
	mux.HandleFunc("GET /registrations", application.handleRegistrations)
	mux.HandleFunc("GET /dlq", application.handleDeadLetters)
	mux.HandleFunc("POST /events", application.handleSingleEvent)
	mux.HandleFunc("POST /events/batch", application.handleBatchEvents)

	application.httpServer = &http.Server{
		Addr:    applicationConfig.HTTPAddress,
		Handler: mux,
	}

	return application, nil
}

func (application *Application) Run(requestContext context.Context) error {
	scheduledJobs := application.scheduler.Start(requestContext)
	application.startMetricsReporter(requestContext)

	var workerGroup sync.WaitGroup
	for workerIndex := 0; workerIndex < application.config.WorkerCount; workerIndex++ {
		workerGroup.Add(1)
		go func(workerID int) {
			defer workerGroup.Done()
			application.runWorker(requestContext, workerID, scheduledJobs)
		}(workerIndex + 1)
	}

	serverErrors := make(chan error, 1)
	queueErrors := make(chan error, 1)
	go func() {
		application.logger.Info("starting notifier", "address", application.config.HTTPAddress, "workerCount", application.config.WorkerCount)
		listenError := application.httpServer.ListenAndServe()
		if listenError != nil && !errors.Is(listenError, http.ErrServerClosed) {
			serverErrors <- listenError
			return
		}
		serverErrors <- nil
	}()

	go func() {
		queueErrors <- application.runQueuePoller(requestContext)
	}()

	select {
	case <-requestContext.Done():
	case serverError := <-serverErrors:
		if serverError != nil {
			application.scheduler.Close()
			return serverError
		}
	case queueError := <-queueErrors:
		if queueError != nil {
			application.scheduler.Close()
			return queueError
		}
	}

	application.scheduler.Close()

	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()

	shutdownError := application.httpServer.Shutdown(shutdownContext)
	registryCloseError := application.registry.Close()
	queueCloseError := application.workQueue.Close()
	workerGroup.Wait()
	if shutdownError != nil {
		return shutdownError
	}
	if registryCloseError != nil {
		return registryCloseError
	}
	if queueCloseError != nil {
		return queueCloseError
	}

	return nil
}

func (application *Application) runQueuePoller(requestContext context.Context) error {
	claimOwner := fmt.Sprintf("notifier-%d", time.Now().UnixNano())
	application.logger.Info(
		"starting postgres work queue poller",
		"claimBatchSize", application.config.QueueClaimBatchSize,
		"pollInterval", application.config.QueuePollInterval.String(),
		"claimOwner", claimOwner,
	)

	pollTicker := time.NewTicker(application.config.QueuePollInterval)
	defer pollTicker.Stop()

	for {
		if queueError := application.claimAndSchedule(requestContext, claimOwner); queueError != nil {
			return queueError
		}

		select {
		case <-requestContext.Done():
			return nil
		case <-pollTicker.C:
		}
	}
}

func (application *Application) claimAndSchedule(requestContext context.Context, claimOwner string) error {
	queuedDeliveries, claimError := application.workQueue.ClaimAvailableDeliveries(
		requestContext,
		claimOwner,
		application.config.QueueClaimBatchSize,
		time.Now().UTC(),
	)
	if claimError != nil {
		return claimError
	}

	for _, queuedDelivery := range queuedDeliveries {
		application.scheduler.Enqueue(queuedDelivery.Job)
	}

	return nil
}
