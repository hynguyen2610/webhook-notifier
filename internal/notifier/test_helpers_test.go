package notifier

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"webhook-notifier/internal/config"
	"webhook-notifier/internal/delivery"
	"webhook-notifier/internal/events"
	"webhook-notifier/internal/metrics"
	"webhook-notifier/internal/registration"
	"webhook-notifier/internal/retry"
	"webhook-notifier/internal/scheduler"
)

func newTestApplication(webhookURLsByCustomerID map[string][]string, workerCount int, requestTimeout time.Duration, initialRetryDelay time.Duration, maxRetryAttempts int) *Application {
	return &Application{
		config: config.NotifierConfig{
			WorkerCount:      workerCount,
			RequestTimeout:   requestTimeout,
			MaxRetryAttempts: maxRetryAttempts,
		},
		logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		registry:       testRegistry{webhookURLsByCustomerID: webhookURLsByCustomerID},
		scheduler:      scheduler.NewRoundRobinScheduler(workerCount * 4),
		deliveryClient: delivery.NewHTTPClient(requestTimeout),
		retryPolicy: retry.ExponentialBackoffPolicy{
			InitialDelay:    initialRetryDelay,
			MaxRetryAttempt: maxRetryAttempts,
		},
		notifierMetrics: newTestNotifierMetrics(),
	}
}

func startTestWorkers(requestContext context.Context, application *Application, workerCount int) *testWorkers {
	scheduledJobs := application.scheduler.Start(requestContext)
	workers := &testWorkers{}

	for workerIndex := 0; workerIndex < workerCount; workerIndex++ {
		workers.waitGroup.Add(1)
		go func(workerID int) {
			defer workers.waitGroup.Done()
			application.runWorker(requestContext, workerID, scheduledJobs)
		}(workerIndex + 1)
	}

	return workers
}

type testWorkers struct {
	waitGroup sync.WaitGroup
}

func (workers *testWorkers) Wait() {
	workers.waitGroup.Wait()
}

type testRegistry struct {
	webhookURLsByCustomerID map[string][]string
}

func (registry testRegistry) ResolveWebhookURLs(_ context.Context, customerID string) ([]string, error) {
	webhookURLs, found := registry.webhookURLsByCustomerID[customerID]
	if !found {
		return nil, registration.ErrCustomerNotRegistered
	}

	return webhookURLs, nil
}

func (registry testRegistry) Snapshot(_ context.Context) (map[string][]string, error) {
	clonedSnapshot := make(map[string][]string, len(registry.webhookURLsByCustomerID))
	for customerID, webhookURLs := range registry.webhookURLsByCustomerID {
		clonedSnapshot[customerID] = append([]string(nil), webhookURLs...)
	}

	return clonedSnapshot, nil
}

func (registry testRegistry) Close() error {
	return nil
}

func newTestNotifierMetrics() *metrics.NotifierMetrics {
	return &metrics.NotifierMetrics{
		ReceivedEventsCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "test_webhook_notifier_received_events_total",
			Help: "Test counter for received events.",
		}),
		DeliveredEventsCounter: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "test_webhook_notifier_delivered_events_total",
			Help: "Test counter for delivered events.",
		}, []string{"customer_id"}),
		FailedDeliveriesCounter: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "test_webhook_notifier_failed_deliveries_total",
			Help: "Test counter for failed deliveries.",
		}, []string{"customer_id", "retryable"}),
		RetriedDeliveriesCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "test_webhook_notifier_retried_deliveries_total",
			Help: "Test counter for retried deliveries.",
		}),
		DeadLetterCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "test_webhook_notifier_dead_letter_total",
			Help: "Test counter for dead letter deliveries.",
		}),
		DeliveryDurationHistogram: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "test_webhook_notifier_delivery_duration_seconds",
			Help: "Test histogram for delivery durations.",
		}, []string{"customer_id", "status_family"}),
		ScheduledQueueDepthGauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "test_webhook_notifier_scheduled_queue_depth",
			Help: "Test gauge for scheduled queue depth.",
		}),
	}
}

func waitForNotifierCount(t *testing.T, readCount func() int64, expectedCount int64, countName string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if readCount() == expectedCount {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("expected %s %d, got %d", countName, expectedCount, readCount())
}

func waitForCondition(t *testing.T, description string, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for %s", description)
}

func newTestEvent(customerID string, eventID string) events.SubscriberEvent {
	return events.SubscriberEvent{
		EventID:      eventID,
		CustomerID:   customerID,
		SubscriberID: "subscriber-" + eventID,
		EventType:    "subscriber.created",
		OccurredAt:   time.Date(2026, time.August, 3, 9, 0, 0, 0, time.UTC),
	}
}
