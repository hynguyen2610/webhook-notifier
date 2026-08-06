package notifier

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"

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

var (
	newRegistryStore         = registration.NewPostgresRegistry
	newQueueRepository       = workqueue.NewPostgresRepository
	newScheduler             = scheduler.NewRoundRobinScheduler
	newDeliveryClientFactory = delivery.NewHTTPClient
	newNotifierMetrics       = metrics.NewNotifierMetrics
)

func NewApplication(applicationConfig config.NotifierConfig, logger *slog.Logger) (*Application, error) {
	registryStore, registryError := newRegistryStore(
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

	queueRepository, queueError := newQueueRepository(applicationConfig.PostgresConnection)
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
		scheduler:       newScheduler(applicationConfig.WorkerCount * applicationConfig.SchedulerBufferMultiplier),
		deliveryClient:  newDeliveryClientFactory(applicationConfig.RequestTimeout),
		notifierMetrics: newNotifierMetrics(),
		retryPolicy: retry.ExponentialBackoffPolicy{
			InitialDelay:    applicationConfig.InitialRetryDelay,
			MaxRetryAttempt: applicationConfig.MaxRetryAttempts,
		},
	}

	application.httpServer = application.newHTTPServer()

	return application, nil
}
