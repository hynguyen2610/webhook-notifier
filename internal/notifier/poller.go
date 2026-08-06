package notifier

import (
	"context"
	"fmt"
	"time"
)

func (application *Application) runQueuePoller(requestContext context.Context) error {
	claimOwner := fmt.Sprintf("notifier-%d", time.Now().UnixNano())
	application.logger.Info(
		"starting postgres work queue poller",
		"claimBatchSize", application.config.QueueClaimBatchSize,
		"pollInterval", application.config.QueuePollInterval.String(),
		"claimOwner", claimOwner,
	)

	pollTicker := time.NewTicker(application.config.QueuePollInterval)
	defer pollTicker.Stop()

	for {
		if queueError := application.claimAndSchedule(requestContext, claimOwner); queueError != nil {
			return queueError
		}

		select {
		case <-requestContext.Done():
			return nil
		case <-pollTicker.C:
		}
	}
}

func (application *Application) claimAndSchedule(requestContext context.Context, claimOwner string) error {
	if application.scheduledQueueIsFull() {
		return nil
	}

	queuedDeliveries, claimError := application.workQueue.ClaimAvailableDeliveries(
		requestContext,
		claimOwner,
		application.config.QueueClaimBatchSize,
		time.Now().UTC(),
	)
	if claimError != nil {
		return claimError
	}

	for _, queuedDelivery := range queuedDeliveries {
		application.scheduler.Enqueue(queuedDelivery.Job)
	}

	return nil
}

func (application *Application) scheduledQueueIsFull() bool {
	return application.scheduler.QueueDepth() >= application.scheduledQueueLimit()
}

func (application *Application) scheduledQueueLimit() int {
	return application.config.ScheduledQueueLimit()
}
