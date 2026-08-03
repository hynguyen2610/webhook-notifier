package notifier

import (
	"context"
	"time"

	"webhook-notifier/internal/events"
)

func (application *Application) runWorker(requestContext context.Context, workerID int, scheduledJobs <-chan events.DeliveryJob) {
	for {
		select {
		case <-requestContext.Done():
			return
		case job, channelOpen := <-scheduledJobs:
			if !channelOpen {
				return
			}

			result := application.deliveryClient.Deliver(requestContext, job)
			application.logDeliveryResult(workerID, result)
			application.recordDeliveryMetrics(result)

			if result.StatusCode >= 200 && result.StatusCode < 300 {
				application.deliveredEvents.Add(1)
				if markDeliveredError := application.workQueue.MarkDelivered(requestContext, job.QueueItemID, result.CompletedAt); markDeliveredError != nil {
					application.logger.Error("mark queue item delivered", "queueItemID", job.QueueItemID, "error", markDeliveredError)
				}
				continue
			}

			application.failedDeliveries.Add(1)
			if result.ShouldRetry && application.retryPolicy.CanRetry(job.Attempt) {
				application.retriedDeliveries.Add(1)
				delay := application.retryPolicy.NextDelay(job.Attempt)
				nextAvailableAt := time.Now().UTC().Add(delay)
				if retryError := application.workQueue.MarkRetryPending(requestContext, job.QueueItemID, result.FailureReason, nextAvailableAt, time.Now().UTC()); retryError != nil {
					application.logger.Error("mark queue item retry pending", "queueItemID", job.QueueItemID, "error", retryError)
				}
				continue
			}

			application.recordDeadLetter(job, result.FailureReason)
		}
	}
}

func (application *Application) recordDeadLetter(job events.DeliveryJob, failureReason string) {
	application.deadLetterCount.Add(1)
	application.notifierMetrics.DeadLetterCounter.Inc()
	application.deadLetterMutex.Lock()
	deadLetterMessage := events.DeadLetterMessage{
		Job:           job,
		FailureReason: failureReason,
		ExhaustedAt:   time.Now(),
	}
	application.deadLetters = append(application.deadLetters, deadLetterMessage)
	application.deadLetterMutex.Unlock()
	if queueError := application.workQueue.MarkDeadLetter(context.Background(), job.QueueItemID, failureReason, deadLetterMessage.ExhaustedAt); queueError != nil {
		application.logger.Error("mark queue item dead lettered", "queueItemID", job.QueueItemID, "error", queueError)
	}
}
