package scheduler

import (
	"context"
	"testing"
	"time"

	"webhook-notifier/internal/events"
)

func TestRoundRobinSchedulerCloseStopsStartedOutputChannel(t *testing.T) {
	// Input: a started scheduler with no queued jobs that is closed explicitly.
	// Outcome: the output channel closes cleanly instead of blocking forever.
	roundRobinScheduler := NewRoundRobinScheduler(1)
	requestContext, cancelRequest := context.WithCancel(context.Background())
	defer cancelRequest()

	scheduledJobs := roundRobinScheduler.Start(requestContext)
	roundRobinScheduler.Close()

	select {
	case _, channelOpen := <-scheduledJobs:
		if channelOpen {
			t.Fatal("expected closed scheduler output channel")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for scheduler output to close")
	}
}

func TestRoundRobinSchedulerStartStopsWhenContextIsCancelled(t *testing.T) {
	// Input: a started scheduler whose context is canceled before any job arrives.
	// Outcome: the output channel closes after context cancellation.
	roundRobinScheduler := NewRoundRobinScheduler(1)
	requestContext, cancelRequest := context.WithCancel(context.Background())
	scheduledJobs := roundRobinScheduler.Start(requestContext)

	cancelRequest()

	select {
	case _, channelOpen := <-scheduledJobs:
		if channelOpen {
			t.Fatal("expected closed scheduler output channel after cancel")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for scheduler output to close after cancel")
	}
}

func TestRoundRobinSchedulerDequeueSkipsEmptyCustomerQueues(t *testing.T) {
	// Input: one scheduler with an empty customer queue slot followed by a populated queue.
	// Outcome: dequeue skips the empty slot and returns the available job.
	roundRobinScheduler := NewRoundRobinScheduler(1)
	roundRobinScheduler.customerOrder = []string{"customer-empty", "customer-a"}
	roundRobinScheduler.customerQueues["customer-empty"] = nil
	roundRobinScheduler.customerQueues["customer-a"] = []events.DeliveryJob{newJob("customer-a", "event-1")}

	nextJob, found := roundRobinScheduler.dequeueNextJobLocked()
	if !found {
		t.Fatal("expected job to be found")
	}
	if nextJob.Event.CustomerID != "customer-a" || roundRobinScheduler.nextCustomerIndex != 0 {
		t.Fatalf("unexpected dequeued job %#v with next index %d", nextJob, roundRobinScheduler.nextCustomerIndex)
	}
}

func TestRoundRobinSchedulerDequeueReturnsFalseWhenNoCustomersHaveJobs(t *testing.T) {
	// Input: one scheduler with no customer order and one with only empty queues.
	// Outcome: dequeue reports no job available in both cases.
	emptyScheduler := NewRoundRobinScheduler(1)
	if nextJob, found := emptyScheduler.dequeueNextJobLocked(); found || nextJob != (events.DeliveryJob{}) {
		t.Fatalf("expected no job from empty scheduler, got %#v found=%t", nextJob, found)
	}

	orderedScheduler := NewRoundRobinScheduler(1)
	orderedScheduler.customerOrder = []string{"customer-a", "customer-b"}
	orderedScheduler.customerQueues["customer-a"] = nil
	orderedScheduler.customerQueues["customer-b"] = nil
	if nextJob, found := orderedScheduler.dequeueNextJobLocked(); found || nextJob != (events.DeliveryJob{}) {
		t.Fatalf("expected no job from empty ordered scheduler, got %#v found=%t", nextJob, found)
	}
}

func TestRoundRobinSchedulerStartStopsWhenCancelledAfterDequeue(t *testing.T) {
	// Input: one started scheduler with a queued job and a context canceled immediately after enqueue.
	// Outcome: the scheduler exits cleanly even if cancellation races with channel delivery.
	roundRobinScheduler := NewRoundRobinScheduler(1)
	requestContext, cancelRequest := context.WithCancel(context.Background())
	scheduledJobs := roundRobinScheduler.Start(requestContext)
	roundRobinScheduler.Enqueue(newJob("customer-a", "event-1"))
	cancelRequest()

	select {
	case <-scheduledJobs:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for scheduler delivery or closure after cancel")
	}
}

func TestRoundRobinSchedulerStartStopsWhenCancelWinsBeforeChannelSend(t *testing.T) {
	// Input: a scheduler whose buffered output is already full when a second job is ready to send.
	// Outcome: cancelling the context drives the Start loop through the requestContext.Done branch in the send select.
	roundRobinScheduler := NewRoundRobinScheduler(1)
	requestContext, cancelRequest := context.WithCancel(context.Background())
	scheduledJobs := roundRobinScheduler.Start(requestContext)
	roundRobinScheduler.Enqueue(newJob("customer-a", "event-1"))
	roundRobinScheduler.Enqueue(newJob("customer-b", "event-2"))

	time.Sleep(20 * time.Millisecond)
	cancelRequest()

	select {
	case _, channelOpen := <-scheduledJobs:
		if channelOpen {
			select {
			case _, channelOpen = <-scheduledJobs:
				if channelOpen {
					t.Fatal("expected scheduler output channel to close after draining buffered work")
				}
			case <-time.After(2 * time.Second):
				t.Fatal("timed out waiting for scheduler output to close after draining buffered work")
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for scheduler output to close")
	}
}
