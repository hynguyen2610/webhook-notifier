package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestNewNotifierMetricsRegistersCollectorsOnFreshDefaultRegistry(t *testing.T) {
	// Input: a fresh Prometheus default registry and a new notifier metrics instance.
	// Outcome: all notifier collectors are registered and can be gathered successfully.
	restoreDefaults := swapPrometheusDefaults()
	defer restoreDefaults()

	notifierMetrics := NewNotifierMetrics()
	notifierMetrics.ReceivedEventsCounter.Inc()
	notifierMetrics.DeliveredEventsCounter.WithLabelValues("customer-a").Inc()
	notifierMetrics.FailedDeliveriesCounter.WithLabelValues("customer-a", "true").Inc()
	notifierMetrics.RetriedDeliveriesCounter.Inc()
	notifierMetrics.DeadLetterCounter.Inc()
	notifierMetrics.DeliveryDurationHistogram.WithLabelValues("customer-a", "2xx").Observe(0.5)
	notifierMetrics.ScheduledQueueDepthGauge.Set(3)
	notifierMetrics.PendingQueueDepthGauge.Set(5)
	notifierMetrics.OldestPendingAgeGauge.Set(7)

	metricFamilies, gatherError := prometheus.DefaultGatherer.Gather()
	if gatherError != nil {
		t.Fatalf("gather metrics: %v", gatherError)
	}
	if len(metricFamilies) != 9 {
		t.Fatalf("expected 9 metric families, got %d", len(metricFamilies))
	}
}

func TestHandlerServesMetricsFromDefaultGatherer(t *testing.T) {
	// Input: the notifier metrics handler invoked against a registry with collected metrics.
	// Outcome: the handler returns Prometheus exposition text containing notifier metric names.
	restoreDefaults := swapPrometheusDefaults()
	defer restoreDefaults()

	notifierMetrics := NewNotifierMetrics()
	notifierMetrics.ReceivedEventsCounter.Inc()

	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	responseRecorder := httptest.NewRecorder()

	Handler().ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", responseRecorder.Code)
	}
	if !strings.Contains(responseRecorder.Body.String(), "webhook_notifier_received_events_total") {
		t.Fatalf("expected metrics output to include received events counter, got %s", responseRecorder.Body.String())
	}
}

func swapPrometheusDefaults() func() {
	originalRegisterer := prometheus.DefaultRegisterer
	originalGatherer := prometheus.DefaultGatherer
	registry := prometheus.NewRegistry()
	prometheus.DefaultRegisterer = registry
	prometheus.DefaultGatherer = registry

	return func() {
		prometheus.DefaultRegisterer = originalRegisterer
		prometheus.DefaultGatherer = originalGatherer
	}
}
