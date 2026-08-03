package mockreceiver

import (
	"encoding/json"
	"net/http"
	"strings"

	"webhook-notifier/internal/events"
)

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
