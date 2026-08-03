package notifier

import (
	"context"
	"io"
	"log/slog"
	"sort"
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
	"webhook-notifier/internal/workqueue"
)

func newTestApplication(webhookURLsByCustomerID map[string][]string, workerCount int, requestTimeout time.Duration, initialRetryDelay time.Duration, maxRetryAttempts int) *Application {
	return &Application{
		config: config.NotifierConfig{
			WorkerCount:         workerCount,
			RequestTimeout:      requestTimeout,
			MaxRetryAttempts:    maxRetryAttempts,
			QueueClaimBatchSize: 32,
			QueuePollInterval:   10 * time.Millisecond,
		},
		logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		registry:       testRegistry{webhookURLsByCustomerID: webhookURLsByCustomerID},
		workQueue:      newTestQueue(),
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

	workers.waitGroup.Add(1)
	go func() {
		defer workers.waitGroup.Done()
		_ = application.runQueuePoller(requestContext)
	}()

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

type testQueue struct {
	mutex      sync.Mutex
	nextID     int64
	queueItems []testQueueItem
}

type testQueueItem struct {
	queueItemID    int64
	job            events.DeliveryJob
	status         string
	availableAt    time.Time
	deadLetteredAt time.Time
}

func newTestQueue() *testQueue {
	return &testQueue{nextID: 1}
}

func (queue *testQueue) EnsureSchema(context.Context) error {
	return nil
}

func (queue *testQueue) EnqueueDeliveries(_ context.Context, subscriberEvent events.SubscriberEvent, webhookURLs []string, availableAt time.Time) (int, error) {
	queue.mutex.Lock()
	defer queue.mutex.Unlock()

	for _, webhookURL := range webhookURLs {
		queue.queueItems = append(queue.queueItems, testQueueItem{
			queueItemID: queue.nextID,
			job: events.DeliveryJob{
				QueueItemID: queue.nextID,
				Event:       subscriberEvent,
				WebhookURL:  webhookURL,
				Attempt:     0,
				EnqueuedAt:  time.Now().UTC(),
				TraceID:     subscriberEvent.EventID,
			},
			status:      "pending",
			availableAt: availableAt.UTC(),
		})
		queue.nextID++
	}

	return len(webhookURLs), nil
}

func (queue *testQueue) ClaimAvailableDeliveries(_ context.Context, _ string, limit int, claimedAt time.Time) ([]workqueue.QueuedDelivery, error) {
	queue.mutex.Lock()
	defer queue.mutex.Unlock()

	sort.SliceStable(queue.queueItems, func(leftIndex int, rightIndex int) bool {
		if queue.queueItems[leftIndex].availableAt.Equal(queue.queueItems[rightIndex].availableAt) {
			return queue.queueItems[leftIndex].queueItemID < queue.queueItems[rightIndex].queueItemID
		}
		return queue.queueItems[leftIndex].availableAt.Before(queue.queueItems[rightIndex].availableAt)
	})

	queuedDeliveries := make([]workqueue.QueuedDelivery, 0, limit)
	for queueItemIndex := range queue.queueItems {
		queueItem := &queue.queueItems[queueItemIndex]
		if queueItem.status != "pending" || queueItem.availableAt.After(claimedAt.UTC()) {
			continue
		}

		queueItem.status = "claimed"
		queuedDeliveries = append(queuedDeliveries, workqueue.QueuedDelivery{
			QueueItemID: queueItem.queueItemID,
			Job:         queueItem.job,
		})
		if len(queuedDeliveries) == limit {
			break
		}
	}

	return queuedDeliveries, nil
}

func (queue *testQueue) MarkDelivered(_ context.Context, queueItemID int64, completedAt time.Time) error {
	queue.mutex.Lock()
	defer queue.mutex.Unlock()

	for queueItemIndex := range queue.queueItems {
		if queue.queueItems[queueItemIndex].queueItemID == queueItemID {
			queue.queueItems[queueItemIndex].status = "completed"
			queue.queueItems[queueItemIndex].job.EnqueuedAt = queue.queueItems[queueItemIndex].job.EnqueuedAt
			_ = completedAt
			return nil
		}
	}

	return nil
}

func (queue *testQueue) MarkRetryPending(_ context.Context, queueItemID int64, lastError string, nextAvailableAt time.Time, _ time.Time) error {
	queue.mutex.Lock()
	defer queue.mutex.Unlock()

	for queueItemIndex := range queue.queueItems {
		if queue.queueItems[queueItemIndex].queueItemID == queueItemID {
			queue.queueItems[queueItemIndex].status = "pending"
			queue.queueItems[queueItemIndex].availableAt = nextAvailableAt.UTC()
			queue.queueItems[queueItemIndex].job.Attempt++
			queue.queueItems[queueItemIndex].job.LastError = lastError
			return nil
		}
	}

	return nil
}

func (queue *testQueue) MarkDeadLetter(_ context.Context, queueItemID int64, lastError string, deadLetteredAt time.Time) error {
	queue.mutex.Lock()
	defer queue.mutex.Unlock()

	for queueItemIndex := range queue.queueItems {
		if queue.queueItems[queueItemIndex].queueItemID == queueItemID {
			queue.queueItems[queueItemIndex].status = "dead_lettered"
			queue.queueItems[queueItemIndex].job.LastError = lastError
			queue.queueItems[queueItemIndex].deadLetteredAt = deadLetteredAt.UTC()
			return nil
		}
	}

	return nil
}

func (queue *testQueue) SnapshotDeadLetters(_ context.Context) ([]events.DeadLetterMessage, error) {
	queue.mutex.Lock()
	defer queue.mutex.Unlock()

	deadLetterMessages := make([]events.DeadLetterMessage, 0)
	for _, queueItem := range queue.queueItems {
		if queueItem.status != "dead_lettered" {
			continue
		}
		deadLetterMessages = append(deadLetterMessages, events.DeadLetterMessage{
			Job:           queueItem.job,
			FailureReason: queueItem.job.LastError,
			ExhaustedAt:   queueItem.deadLetteredAt,
		})
	}

	return deadLetterMessages, nil
}

func (queue *testQueue) Close() error {
	return nil
}
