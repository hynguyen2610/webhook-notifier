package mockreceiver

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"webhook-notifier/internal/config"
	"webhook-notifier/internal/httpx"
)

type Application struct {
	config         config.MockReceiverConfig
	logger         *slog.Logger
	httpServer     *http.Server
	scenarioMutex  sync.RWMutex
	scenarios      map[string]Scenario
	receivedCount  atomic.Int64
	successCount   atomic.Int64
	failedCount    atomic.Int64
	totalLatencyMs atomic.Int64
	randomSource   *rand.Rand
}

type Scenario struct {
	Mode               string  `json:"mode"`
	DelayMilliseconds  int     `json:"delay"`
	FailureProbability float64 `json:"failureProbability,omitempty"`
}

type Statistics struct {
	Received         int64 `json:"received"`
	Success          int64 `json:"success"`
	Failed           int64 `json:"failed"`
	AverageLatencyMs int64 `json:"averageLatencyMs"`
}

func NewApplication(applicationConfig config.MockReceiverConfig, logger *slog.Logger) *Application {
	application := &Application{
		config:       applicationConfig,
		logger:       logger,
		scenarios:    make(map[string]Scenario),
		randomSource: rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /webhook/{customerId}", application.handleWebhook)
	mux.HandleFunc("POST /config/{customerId}", application.handleScenarioConfig)
	mux.HandleFunc("GET /stats", application.handleStats)
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

	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()

	return application.httpServer.Shutdown(shutdownContext)
}

func (application *Application) handleWebhook(responseWriter http.ResponseWriter, request *http.Request) {
	customerID := request.PathValue("customerId")
	if strings.TrimSpace(customerID) == "" {
		httpx.WriteError(responseWriter, http.StatusBadRequest, "customerId is required")
		return
	}

	startedAt := time.Now()
	application.receivedCount.Add(1)

	scenario := application.lookupScenario(customerID)
	if scenario.DelayMilliseconds > 0 {
		time.Sleep(time.Duration(scenario.DelayMilliseconds) * time.Millisecond)
	}

	statusCode := application.resolveStatusCode(scenario)
	latencyMilliseconds := time.Since(startedAt).Milliseconds()
	application.totalLatencyMs.Add(latencyMilliseconds)
	if statusCode >= 200 && statusCode < 300 {
		application.successCount.Add(1)
	} else {
		application.failedCount.Add(1)
	}

	httpx.WriteJSON(responseWriter, statusCode, map[string]any{
		"customerId": customerID,
		"mode":       scenario.Mode,
		"status":     statusCode,
	})
}

func (application *Application) handleScenarioConfig(responseWriter http.ResponseWriter, request *http.Request) {
	customerID := request.PathValue("customerId")
	if strings.TrimSpace(customerID) == "" {
		httpx.WriteError(responseWriter, http.StatusBadRequest, "customerId is required")
		return
	}

	var scenario Scenario
	if decodeError := json.NewDecoder(request.Body).Decode(&scenario); decodeError != nil {
		httpx.WriteError(responseWriter, http.StatusBadRequest, decodeError.Error())
		return
	}

	if scenario.Mode == "" {
		scenario.Mode = "success"
	}

	application.scenarioMutex.Lock()
	application.scenarios[customerID] = scenario
	application.scenarioMutex.Unlock()

	httpx.WriteJSON(responseWriter, http.StatusOK, map[string]any{
		"customerId": customerID,
		"scenario":   scenario,
	})
}

func (application *Application) handleStats(responseWriter http.ResponseWriter, _ *http.Request) {
	received := application.receivedCount.Load()
	averageLatency := int64(0)
	if received > 0 {
		averageLatency = application.totalLatencyMs.Load() / received
	}

	httpx.WriteJSON(responseWriter, http.StatusOK, Statistics{
		Received:         received,
		Success:          application.successCount.Load(),
		Failed:           application.failedCount.Load(),
		AverageLatencyMs: averageLatency,
	})
}

func (application *Application) handleResetStats(responseWriter http.ResponseWriter, _ *http.Request) {
	application.receivedCount.Store(0)
	application.successCount.Store(0)
	application.failedCount.Store(0)
	application.totalLatencyMs.Store(0)
	httpx.WriteJSON(responseWriter, http.StatusOK, map[string]string{
		"status": "reset",
	})
}

func (application *Application) handleHealth(responseWriter http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(responseWriter, http.StatusOK, map[string]string{
		"status": "ok",
	})
}

func (application *Application) lookupScenario(customerID string) Scenario {
	application.scenarioMutex.RLock()
	defer application.scenarioMutex.RUnlock()

	scenario, found := application.scenarios[customerID]
	if !found {
		return Scenario{Mode: "success"}
	}

	return scenario
}

func (application *Application) resolveStatusCode(scenario Scenario) int {
	switch strings.ToLower(strings.TrimSpace(scenario.Mode)) {
	case "timeout":
		return http.StatusGatewayTimeout
	case "error500":
		return http.StatusInternalServerError
	case "error400":
		return http.StatusBadRequest
	case "unauthorized":
		return http.StatusUnauthorized
	case "random":
		if application.randomSource.Float64() < scenario.FailureProbability {
			return http.StatusInternalServerError
		}
		return http.StatusOK
	default:
		return http.StatusOK
	}
}
