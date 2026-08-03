package notifier

import (
	"context"
	"sort"
	"sync"
	"time"

	"webhook-notifier/internal/events"
	"webhook-notifier/internal/registration"
	"webhook-notifier/internal/workqueue"
)

type benchmarkRegistry struct {
	webhookURLsByCustomerID map[string][]string
}

func newBenchmarkRegistry(eventsPerCustomer map[string]int) benchmarkRegistry {
	webhookURLsByCustomerID := make(map[string][]string, len(eventsPerCustomer))
	for customerID := range eventsPerCustomer {
		webhookURLsByCustomerID[customerID] = []string{"benchmark://" + customerID}
	}
	return benchmarkRegistry{webhookURLsByCustomerID: webhookURLsByCustomerID}
}

func (registry benchmarkRegistry) ResolveWebhookURLs(_ context.Context, customerID string) ([]string, error) {
	webhookURLs, found := registry.webhookURLsByCustomerID[customerID]
	if !found {
		return nil, registration.ErrCustomerNotRegistered
	}
	return webhookURLs, nil
}

func (registry benchmarkRegistry) Snapshot(_ context.Context) (map[string][]string, error) {
	snapshot := make(map[string][]string, len(registry.webhookURLsByCustomerID))
	for customerID, webhookURLs := range registry.webhookURLsByCustomerID {
		snapshot[customerID] = append([]string(nil), webhookURLs...)
	}
	return snapshot, nil
}

func (registry benchmarkRegistry) Close() error {
	return nil
}

type benchmarkQueue struct {
	mutex      sync.Mutex
	nextID     int64
	queueItems []benchmarkQueueItem
}

type benchmarkQueueItem struct {
	queueItemID    int64
	job            events.DeliveryJob
	status         string
	availableAt    time.Time
	deadLetteredAt time.Time
}

func newBenchmarkQueue() *benchmarkQueue {
	return &benchmarkQueue{nextID: 1}
}

func (queue *benchmarkQueue) EnsureSchema(context.Context) error { return nil }

func (queue *benchmarkQueue) EnqueueDeliveries(_ context.Context, subscriberEvent events.SubscriberEvent, webhookURLs []string, availableAt time.Time) (int, error) {
	queue.mutex.Lock()
	defer queue.mutex.Unlock()

	for _, webhookURL := range webhookURLs {
		queue.queueItems = append(queue.queueItems, benchmarkQueueItem{
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

func (queue *benchmarkQueue) ClaimAvailableDeliveries(_ context.Context, _ string, limit int, claimedAt time.Time) ([]workqueue.QueuedDelivery, error) {
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

func (queue *benchmarkQueue) MarkDelivered(_ context.Context, queueItemID int64, _ time.Time) error {
	queue.mutex.Lock()
	defer queue.mutex.Unlock()
	for queueItemIndex := range queue.queueItems {
		if queue.queueItems[queueItemIndex].queueItemID == queueItemID {
			queue.queueItems[queueItemIndex].status = "completed"
			return nil
		}
	}
	return nil
}

func (queue *benchmarkQueue) MarkRetryPending(_ context.Context, queueItemID int64, lastError string, nextAvailableAt time.Time, _ time.Time) error {
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

func (queue *benchmarkQueue) MarkDeadLetter(_ context.Context, queueItemID int64, lastError string, deadLetteredAt time.Time) error {
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

func (queue *benchmarkQueue) SnapshotDeadLetters(_ context.Context) ([]events.DeadLetterMessage, error) {
	queue.mutex.Lock()
	defer queue.mutex.Unlock()

	deadLetters := make([]events.DeadLetterMessage, 0)
	for _, queueItem := range queue.queueItems {
		if queueItem.status != "dead_lettered" {
			continue
		}
		deadLetters = append(deadLetters, events.DeadLetterMessage{
			Job:           queueItem.job,
			FailureReason: queueItem.job.LastError,
			ExhaustedAt:   queueItem.deadLetteredAt,
		})
	}
	return deadLetters, nil
}

func (queue *benchmarkQueue) Close() error { return nil }

type benchmarkDeliveryClient struct {
	syntheticWorkIterations int
	onCompletion            func(customerID string, completedAt time.Time)
}

var benchmarkSyntheticDeliverySink int

func (client *benchmarkDeliveryClient) Deliver(_ context.Context, job events.DeliveryJob) events.DeliveryResult {
	startedAt := time.Now()
	payload := job.Event.EventID + job.Event.CustomerID + job.Event.SubscriberID
	if payload == "" {
		payload = "event"
	}

	checksum := 0
	for iterationIndex := 0; iterationIndex < client.syntheticWorkIterations; iterationIndex++ {
		payloadIndex := iterationIndex % len(payload)
		checksum += int(payload[payloadIndex]) + iterationIndex
	}
	benchmarkSyntheticDeliverySink += checksum

	completedAt := time.Now()
	if client.onCompletion != nil {
		client.onCompletion(job.Event.CustomerID, completedAt)
	}

	return events.DeliveryResult{
		Job:         job,
		StatusCode:  202,
		Duration:    completedAt.Sub(startedAt),
		CompletedAt: completedAt,
	}
}
