package notifier

import (
	"context"

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

			deliveryResult := application.performDeliveryAttempt(requestContext, workerID, job)
			if application.handleSuccessfulDelivery(requestContext, deliveryResult) {
				continue
			}

			if application.handleRetryableFailure(requestContext, deliveryResult) {
				continue
			}

			application.handlePermanentFailure(deliveryResult)
		}
	}
}
