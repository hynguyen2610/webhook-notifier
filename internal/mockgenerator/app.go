package mockgenerator

import (
	"context"
	"errors"
	"log/slog"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"webhook-notifier/internal/config"
)

type Application struct {
	config       config.MockGeneratorConfig
	logger       *slog.Logger
	httpServer   *http.Server
	httpClient   *http.Client
	randomSource *rand.Rand

	generationStateMutex sync.Mutex
	generatedEventCount  int64
	baseOccurredAt       time.Time
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
		config:         applicationConfig,
		logger:         logger,
		httpClient:     &http.Client{Timeout: applicationConfig.HTTPRequestTimeout},
		randomSource:   rand.New(rand.NewSource(applicationConfig.RandomSeed)),
		baseOccurredAt: time.Unix(0, applicationConfig.RandomSeed).UTC(),
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

	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), application.config.ShutdownTimeout)
	defer cancelShutdown()

	return application.httpServer.Shutdown(shutdownContext)
}
