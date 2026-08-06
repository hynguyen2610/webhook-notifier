package notifier

import (
	"context"
	"time"

	"webhook-notifier/internal/events"
)

func (application *Application) handlePermanentFailure(deliveryResult events.DeliveryResult) {
	application.recordDeadLetter(deliveryResult.Job, deliveryResult.FailureReason)
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
