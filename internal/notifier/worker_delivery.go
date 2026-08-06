package notifier

import (
	"context"
	"time"

	"webhook-notifier/internal/events"
)

func (application *Application) performDeliveryAttempt(requestContext context.Context, workerID int, job events.DeliveryJob) events.DeliveryResult {
	deliveryResult := application.deliveryClient.Deliver(requestContext, job)
	application.logDeliveryResult(workerID, deliveryResult)
	application.recordDeliveryMetrics(deliveryResult)
	return deliveryResult
}

func (application *Application) handleSuccessfulDelivery(requestContext context.Context, deliveryResult events.DeliveryResult) bool {
	if deliveryResult.StatusCode < 200 || deliveryResult.StatusCode >= 300 {
		return false
	}

	application.deliveredEvents.Add(1)
	if markDeliveredError := application.workQueue.MarkDelivered(requestContext, deliveryResult.Job.QueueItemID, deliveryResult.CompletedAt); markDeliveredError != nil {
		application.logger.Error("mark queue item delivered", "queueItemID", deliveryResult.Job.QueueItemID, "error", markDeliveredError)
	}

	return true
}

func (application *Application) handleRetryableFailure(requestContext context.Context, deliveryResult events.DeliveryResult) bool {
	application.failedDeliveries.Add(1)
	if !deliveryResult.ShouldRetry || !application.retryPolicy.CanRetry(deliveryResult.Job.Attempt) {
		return false
	}

	application.retriedDeliveries.Add(1)
	delay := application.retryPolicy.NextDelay(deliveryResult.Job.Attempt)
	nextAvailableAt := time.Now().UTC().Add(delay)
	if retryError := application.workQueue.MarkRetryPending(requestContext, deliveryResult.Job.QueueItemID, deliveryResult.FailureReason, nextAvailableAt, time.Now().UTC()); retryError != nil {
		application.logger.Error("mark queue item retry pending", "queueItemID", deliveryResult.Job.QueueItemID, "error", retryError)
	}

	return true
}
