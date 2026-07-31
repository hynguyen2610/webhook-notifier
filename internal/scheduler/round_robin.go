package scheduler

import (
	"context"
	"sync"

	"webhook-notifier/internal/events"
)

type RoundRobinScheduler struct {
	mutex             sync.Mutex
	condition         *sync.Cond
	customerQueues    map[string][]events.DeliveryJob
	customerOrder     []string
	nextCustomerIndex int
	output            chan events.DeliveryJob
	closed            bool
	queuedJobCount    int
}

func NewRoundRobinScheduler(bufferSize int) *RoundRobinScheduler {
	roundRobinScheduler := &RoundRobinScheduler{
		customerQueues: make(map[string][]events.DeliveryJob),
		output:         make(chan events.DeliveryJob, bufferSize),
	}
	roundRobinScheduler.condition = sync.NewCond(&roundRobinScheduler.mutex)
	return roundRobinScheduler
}

func (scheduler *RoundRobinScheduler) Start(requestContext context.Context) <-chan events.DeliveryJob {
	go func() {
		go func() {
			<-requestContext.Done()
			scheduler.condition.Broadcast()
		}()

		defer close(scheduler.output)

		for {
			nextJob, found := scheduler.waitForNextJob(requestContext)
			if !found {
				return
			}

			select {
			case <-requestContext.Done():
				return
			case scheduler.output <- nextJob:
			}
		}
	}()

	return scheduler.output
}

func (scheduler *RoundRobinScheduler) Enqueue(job events.DeliveryJob) {
	scheduler.mutex.Lock()
	defer scheduler.mutex.Unlock()

	customerID := job.Event.CustomerID
	if _, found := scheduler.customerQueues[customerID]; !found {
		scheduler.customerOrder = append(scheduler.customerOrder, customerID)
	}

	scheduler.customerQueues[customerID] = append(scheduler.customerQueues[customerID], job)
	scheduler.queuedJobCount++
	scheduler.condition.Signal()
}

func (scheduler *RoundRobinScheduler) Close() {
	scheduler.mutex.Lock()
	defer scheduler.mutex.Unlock()

	scheduler.closed = true
	scheduler.condition.Broadcast()
}

func (scheduler *RoundRobinScheduler) waitForNextJob(requestContext context.Context) (events.DeliveryJob, bool) {
	scheduler.mutex.Lock()
	defer scheduler.mutex.Unlock()

	for {
		if scheduler.closed || requestContext.Err() != nil {
			return events.DeliveryJob{}, false
		}

		nextJob, found := scheduler.dequeueNextJobLocked()
		if found {
			scheduler.queuedJobCount--
			return nextJob, true
		}

		scheduler.condition.Wait()
	}
}

func (scheduler *RoundRobinScheduler) QueueDepth() int {
	scheduler.mutex.Lock()
	defer scheduler.mutex.Unlock()

	return scheduler.queuedJobCount
}

func (scheduler *RoundRobinScheduler) dequeueNextJobLocked() (events.DeliveryJob, bool) {
	if len(scheduler.customerOrder) == 0 {
		return events.DeliveryJob{}, false
	}

	customerCount := len(scheduler.customerOrder)
	for customerOffset := 0; customerOffset < customerCount; customerOffset++ {
		customerIndex := (scheduler.nextCustomerIndex + customerOffset) % customerCount
		customerID := scheduler.customerOrder[customerIndex]
		queue := scheduler.customerQueues[customerID]
		if len(queue) == 0 {
			continue
		}

		nextJob := queue[0]
		remainingQueue := queue[1:]
		if len(remainingQueue) == 0 {
			scheduler.customerQueues[customerID] = nil
		} else {
			scheduler.customerQueues[customerID] = remainingQueue
		}

		scheduler.nextCustomerIndex = (customerIndex + 1) % customerCount
		return nextJob, true
	}

	return events.DeliveryJob{}, false
}
