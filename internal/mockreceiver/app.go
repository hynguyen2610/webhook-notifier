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
	"webhook-notifier/internal/events"
	"webhook-notifier/internal/httpx"
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
	subscriberEvent, decodeError := decodeSubscriberEvent(request)
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
	application.recordCustomerStatistic(customerID, subscriberEvent, decodeError, statusCode)

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
		Customers:        application.snapshotCustomerStatistics(),
	})
}

func (application *Application) handleCustomerStats(responseWriter http.ResponseWriter, request *http.Request) {
	customerID := request.PathValue("customerId")
	if strings.TrimSpace(customerID) == "" {
		httpx.WriteError(responseWriter, http.StatusBadRequest, "customerId is required")
		return
	}

	httpx.WriteJSON(responseWriter, http.StatusOK, application.snapshotCustomerStatistic(customerID))
}

func (application *Application) handleResetStats(responseWriter http.ResponseWriter, _ *http.Request) {
	application.receivedCount.Store(0)
	application.successCount.Store(0)
	application.failedCount.Store(0)
	application.totalLatencyMs.Store(0)
	application.statisticsMutex.Lock()
	application.customerStats = make(map[string]*CustomerStatistics)
	application.statisticsMutex.Unlock()
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

func decodeSubscriberEvent(request *http.Request) (*events.SubscriberEvent, error) {
	defer request.Body.Close()

	var subscriberEvent events.SubscriberEvent
	if decodeError := json.NewDecoder(request.Body).Decode(&subscriberEvent); decodeError != nil {
		return nil, decodeError
	}

	return &subscriberEvent, nil
}

func (application *Application) recordCustomerStatistic(customerID string, subscriberEvent *events.SubscriberEvent, decodeError error, statusCode int) {
	application.statisticsMutex.Lock()
	defer application.statisticsMutex.Unlock()

	customerStatistic, found := application.customerStats[customerID]
	if !found {
		customerStatistic = &CustomerStatistics{
			CustomerID:      customerID,
			EventTypeCounts: make(map[string]int64),
		}
		application.customerStats[customerID] = customerStatistic
	}

	customerStatistic.Received++
	if statusCode >= 200 && statusCode < 300 {
		customerStatistic.Success++
	} else {
		customerStatistic.Failed++
	}

	if decodeError != nil {
		customerStatistic.PayloadDecodeFailures++
		return
	}

	customerStatistic.LastEvent = subscriberEvent
	customerStatistic.EventTypeCounts[subscriberEvent.EventType]++
	if subscriberEvent.CustomerID != customerID {
		customerStatistic.PathPayloadCustomerMismatches++
	}
}

func (application *Application) snapshotCustomerStatistics() map[string]CustomerStatistics {
	application.statisticsMutex.RLock()
	defer application.statisticsMutex.RUnlock()

	customerStatistics := make(map[string]CustomerStatistics, len(application.customerStats))
	for customerID, customerStatistic := range application.customerStats {
		customerStatistics[customerID] = cloneCustomerStatistics(customerStatistic)
	}

	return customerStatistics
}

func (application *Application) snapshotCustomerStatistic(customerID string) CustomerStatistics {
	application.statisticsMutex.RLock()
	defer application.statisticsMutex.RUnlock()

	customerStatistic, found := application.customerStats[customerID]
	if !found {
		return CustomerStatistics{
			CustomerID:      customerID,
			EventTypeCounts: make(map[string]int64),
		}
	}

	return cloneCustomerStatistics(customerStatistic)
}

func cloneCustomerStatistics(customerStatistic *CustomerStatistics) CustomerStatistics {
	clonedStatistic := CustomerStatistics{
		CustomerID:                    customerStatistic.CustomerID,
		Received:                      customerStatistic.Received,
		Success:                       customerStatistic.Success,
		Failed:                        customerStatistic.Failed,
		PayloadDecodeFailures:         customerStatistic.PayloadDecodeFailures,
		PathPayloadCustomerMismatches: customerStatistic.PathPayloadCustomerMismatches,
		EventTypeCounts:               make(map[string]int64, len(customerStatistic.EventTypeCounts)),
	}

	if customerStatistic.LastEvent != nil {
		lastEvent := *customerStatistic.LastEvent
		clonedStatistic.LastEvent = &lastEvent
	}

	for eventType, count := range customerStatistic.EventTypeCounts {
		clonedStatistic.EventTypeCounts[eventType] = count
	}

	return clonedStatistic
}
