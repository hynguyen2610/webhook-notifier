package notifier

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"webhook-notifier/internal/events"
)

func TestStartMetricsReporterPublishesQueueMetrics(t *testing.T) {
	// Input: one notifier with two scheduled jobs and two pending queue rows, the oldest created three seconds ago.
	// Outcome: the metrics reporter publishes scheduled depth, pending depth, and a positive oldest-pending age.
	application := newTestApplication(map[string][]string{
		"customer-a": []string{"https://example.com/a"},
	}, 1, time.Second, time.Second, 1)
	application.config.MetricsReportInterval = 5 * time.Millisecond

	registry := prometheus.NewRegistry()
	registry.MustRegister(
		application.notifierMetrics.ScheduledQueueDepthGauge,
		application.notifierMetrics.PendingQueueDepthGauge,
		application.notifierMetrics.OldestPendingAgeGauge,
	)

	application.scheduler.Enqueue(events.DeliveryJob{Event: events.SubscriberEvent{CustomerID: "customer-a"}})
	application.scheduler.Enqueue(events.DeliveryJob{Event: events.SubscriberEvent{CustomerID: "customer-b"}})

	queue := application.workQueue.(*testQueue)
	_, enqueueError := queue.EnqueueDeliveries(
		context.Background(),
		newTestEvent("customer-a", "event-001"),
		[]string{"https://example.com/a", "https://example.com/b"},
		time.Now().UTC(),
	)
	if enqueueError != nil {
		t.Fatalf("enqueue deliveries for metrics test: %v", enqueueError)
	}
	queue.queueItems[0].job.EnqueuedAt = time.Now().UTC().Add(-3 * time.Second)
	queue.queueItems[1].job.EnqueuedAt = time.Now().UTC().Add(-1 * time.Second)

	requestContext, cancelRequest := context.WithCancel(context.Background())
	defer cancelRequest()
	application.startMetricsReporter(requestContext)

	waitForCondition(t, "queue metrics to be reported", func() bool {
		scheduledDepth, scheduledDepthError := gatherGaugeValue(registry, "test_webhook_notifier_scheduled_queue_depth")
		pendingDepth, pendingDepthError := gatherGaugeValue(registry, "test_webhook_notifier_pending_queue_depth")
		oldestPendingAge, oldestPendingAgeError := gatherGaugeValue(registry, "test_webhook_notifier_oldest_pending_event_age_seconds")
		if scheduledDepthError != nil || pendingDepthError != nil || oldestPendingAgeError != nil {
			return false
		}

		return scheduledDepth == 2 && pendingDepth == 2 && oldestPendingAge >= 2
	})
}

func gatherGaugeValue(registry *prometheus.Registry, metricName string) (float64, error) {
	metricFamilies, gatherError := registry.Gather()
	if gatherError != nil {
		return 0, gatherError
	}

	for _, metricFamily := range metricFamilies {
		if metricFamily.GetName() != metricName || len(metricFamily.GetMetric()) == 0 {
			continue
		}
		return metricFamily.GetMetric()[0].GetGauge().GetValue(), nil
	}

	return 0, fmt.Errorf("metric %s not found", metricName)
}
