package scheduler

import (
	"context"
	"testing"
	"time"

	"webhook-notifier/internal/events"
)

func TestRoundRobinSchedulerAlternatesAcrossCustomers(t *testing.T) {
	// Input: two queued jobs for customer-a and two queued jobs for customer-b.
	// Outcome: scheduler alternates between customers instead of draining one customer first.
	roundRobinScheduler := NewRoundRobinScheduler(10)
	requestContext, cancelRequest := context.WithCancel(context.Background())
	defer cancelRequest()

	scheduledJobs := roundRobinScheduler.Start(requestContext)

	roundRobinScheduler.Enqueue(newJob("customer-a", "event-1"))
	roundRobinScheduler.Enqueue(newJob("customer-a", "event-2"))
	roundRobinScheduler.Enqueue(newJob("customer-b", "event-3"))
	roundRobinScheduler.Enqueue(newJob("customer-b", "event-4"))

	firstJob := readJob(t, scheduledJobs)
	secondJob := readJob(t, scheduledJobs)
	thirdJob := readJob(t, scheduledJobs)
	fourthJob := readJob(t, scheduledJobs)

	actualOrder := []string{
		firstJob.Event.CustomerID + ":" + firstJob.Event.EventID,
		secondJob.Event.CustomerID + ":" + secondJob.Event.EventID,
		thirdJob.Event.CustomerID + ":" + thirdJob.Event.EventID,
		fourthJob.Event.CustomerID + ":" + fourthJob.Event.EventID,
	}
	expectedOrder := []string{
		"customer-a:event-1",
		"customer-b:event-3",
		"customer-a:event-2",
		"customer-b:event-4",
	}

	for index := range expectedOrder {
		if actualOrder[index] != expectedOrder[index] {
			t.Fatalf("unexpected order: got %#v want %#v", actualOrder, expectedOrder)
		}
	}

	if roundRobinScheduler.QueueDepth() != 0 {
		t.Fatalf("expected queue depth 0, got %d", roundRobinScheduler.QueueDepth())
	}
}

func TestRoundRobinSchedulerTracksQueuedDepth(t *testing.T) {
	// Input: two jobs enqueued before any worker consumes them.
	// Outcome: queue depth reports 2.
	roundRobinScheduler := NewRoundRobinScheduler(2)
	roundRobinScheduler.Enqueue(newJob("customer-a", "event-1"))
	roundRobinScheduler.Enqueue(newJob("customer-b", "event-2"))

	if roundRobinScheduler.QueueDepth() != 2 {
		t.Fatalf("expected queue depth 2, got %d", roundRobinScheduler.QueueDepth())
	}
}

func newJob(customerID string, eventID string) events.DeliveryJob {
	return events.DeliveryJob{
		Event: events.SubscriberEvent{
			EventID:      eventID,
			CustomerID:   customerID,
			SubscriberID: "subscriber-1",
			EventType:    events.SubscriberCreatedEventType,
			OccurredAt:   time.Now().UTC(),
		},
		WebhookURL: "https://example.com/webhook",
	}
}

func readJob(t *testing.T, scheduledJobs <-chan events.DeliveryJob) events.DeliveryJob {
	t.Helper()

	select {
	case job := <-scheduledJobs:
		return job
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for scheduled job")
		return events.DeliveryJob{}
	}
}
