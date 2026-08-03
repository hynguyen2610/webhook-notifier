package notifier

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"webhook-notifier/internal/config"
	"webhook-notifier/internal/delivery"
	"webhook-notifier/internal/events"
	"webhook-notifier/internal/kafka"
	"webhook-notifier/internal/metrics"
	"webhook-notifier/internal/registration"
	"webhook-notifier/internal/retry"
	"webhook-notifier/internal/scheduler"
)

type Application struct {
	config            config.NotifierConfig
	logger            *slog.Logger
	registry          registration.Registry
	scheduler         *scheduler.RoundRobinScheduler
	deliveryClient    *delivery.HTTPClient
	retryPolicy       retry.ExponentialBackoffPolicy
	kafkaConsumer     kafka.Consumer
	deadLetterWriter  kafka.DeadLetterWriter
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

	application := &Application{
		config:          applicationConfig,
		logger:          logger,
		registry:        registryStore,
		scheduler:       scheduler.NewRoundRobinScheduler(applicationConfig.WorkerCount * 4),
		deliveryClient:  delivery.NewHTTPClient(applicationConfig.RequestTimeout),
		notifierMetrics: metrics.NewNotifierMetrics(),
		retryPolicy: retry.ExponentialBackoffPolicy{
			InitialDelay:    applicationConfig.InitialRetryDelay,
			MaxRetryAttempt: applicationConfig.MaxRetryAttempts,
		},
	}

	if len(applicationConfig.KafkaBrokers) > 0 {
		application.kafkaConsumer = kafka.NewEventConsumer(
			applicationConfig.KafkaBrokers,
			applicationConfig.KafkaHostOverrides,
			applicationConfig.KafkaTopic,
			applicationConfig.KafkaConsumerGroup,
		)
		application.deadLetterWriter = kafka.NewDeadLetterPublisher(
			applicationConfig.KafkaBrokers,
			applicationConfig.KafkaHostOverrides,
			applicationConfig.KafkaDLQTopic,
		)
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
	kafkaErrors := make(chan error, 1)
	go func() {
		application.logger.Info("starting notifier", "address", application.config.HTTPAddress, "workerCount", application.config.WorkerCount)
		listenError := application.httpServer.ListenAndServe()
		if listenError != nil && !errors.Is(listenError, http.ErrServerClosed) {
			serverErrors <- listenError
			return
		}
		serverErrors <- nil
	}()

	if application.kafkaConsumer != nil {
		go func() {
			application.logger.Info(
				"starting kafka consumer",
				"brokers", strings.Join(application.config.KafkaBrokers, ","),
				"topic", application.config.KafkaTopic,
				"consumerGroup", application.config.KafkaConsumerGroup,
			)

			consumeError := application.kafkaConsumer.Start(requestContext, func(subscriberEvent events.SubscriberEvent) error {
				_, ingestError := application.ingestEvents([]events.SubscriberEvent{subscriberEvent})
				return ingestError
			})
			kafkaErrors <- consumeError
		}()
	}

	select {
	case <-requestContext.Done():
	case serverError := <-serverErrors:
		if serverError != nil {
			application.scheduler.Close()
			return serverError
		}
	case kafkaError := <-kafkaErrors:
		if kafkaError != nil {
			application.scheduler.Close()
			return kafkaError
		}
	}

	application.scheduler.Close()

	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()

	shutdownError := application.httpServer.Shutdown(shutdownContext)
	closeError := application.registry.Close()
	workerGroup.Wait()
	if shutdownError != nil {
		return shutdownError
	}

	return closeError
}
