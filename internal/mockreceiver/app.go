package mockreceiver

import (
	"context"
	"errors"
	"log/slog"
	"math/rand"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"webhook-notifier/internal/config"
	"webhook-notifier/internal/events"
)

type Application struct {
	config          config.MockReceiverConfig
	logger          *slog.Logger
	httpServer      *http.Server
	scenarioMutex   sync.RWMutex
	scenarios       map[string]Scenario
	receivedCount   atomic.Int64
	successCount    atomic.Int64
	failedCount     atomic.Int64
	totalLatencyMs  atomic.Int64
	statisticsMutex sync.RWMutex
	customerStats   map[string]*CustomerStatistics
	randomSource    *rand.Rand
}

type Scenario struct {
	Mode               string  `json:"mode"`
	DelayMilliseconds  int     `json:"delay"`
	FailureProbability float64 `json:"failureProbability,omitempty"`
}

type Statistics struct {
	Received         int64                         `json:"received"`
	Success          int64                         `json:"success"`
	Failed           int64                         `json:"failed"`
	AverageLatencyMs int64                         `json:"averageLatencyMs"`
	Customers        map[string]CustomerStatistics `json:"customers"`
}

type CustomerStatistics struct {
	CustomerID                    string                  `json:"customerId"`
	Received                      int64                   `json:"received"`
	Success                       int64                   `json:"success"`
	Failed                        int64                   `json:"failed"`
	PayloadDecodeFailures         int64                   `json:"payloadDecodeFailures"`
	PathPayloadCustomerMismatches int64                   `json:"pathPayloadCustomerMismatches"`
	EventTypeCounts               map[string]int64        `json:"eventTypeCounts"`
	LastEvent                     *events.SubscriberEvent `json:"lastEvent,omitempty"`
}

func NewApplication(applicationConfig config.MockReceiverConfig, logger *slog.Logger) *Application {
	application := &Application{
		config:        applicationConfig,
		logger:        logger,
		scenarios:     make(map[string]Scenario),
		customerStats: make(map[string]*CustomerStatistics),
		randomSource:  rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /webhook/{customerId}", application.handleWebhook)
	mux.HandleFunc("POST /config/{customerId}", application.handleScenarioConfig)
	mux.HandleFunc("GET /stats", application.handleStats)
	mux.HandleFunc("GET /stats/customer/{customerId}", application.handleCustomerStats)
	mux.HandleFunc("POST /stats/reset", application.handleResetStats)
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
		application.logger.Info("starting mock receiver", "address", application.config.HTTPAddress)
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
