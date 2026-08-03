package mockreceiver

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"webhook-notifier/internal/httpx"
)

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
