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
				continue
			}

			application.failedDeliveries.Add(1)
			if result.ShouldRetry && application.retryPolicy.CanRetry(job.Attempt) {
				application.retriedDeliveries.Add(1)
				nextJob := job
				nextJob.Attempt++
				nextJob.LastError = result.FailureReason
				delay := application.retryPolicy.NextDelay(nextJob.Attempt - 1)
				time.AfterFunc(delay, func() {
					application.scheduler.Enqueue(nextJob)
				})
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

	if application.deadLetterWriter != nil {
		if publishError := application.deadLetterWriter.Publish(deadLetterMessage); publishError != nil {
			application.logger.Error("publish dead letter message", "error", publishError, "eventID", job.Event.EventID)
		}
	}
}
