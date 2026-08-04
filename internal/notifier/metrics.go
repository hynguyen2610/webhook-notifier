package notifier

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"webhook-notifier/internal/events"
)

func (application *Application) logDeliveryResult(workerID int, result events.DeliveryResult) {
	logValues := []any{
		"workerID", workerID,
		"eventID", result.Job.Event.EventID,
		"customerID", result.Job.Event.CustomerID,
		"webhookURL", result.Job.WebhookURL,
		"retryCount", result.Job.Attempt,
		"duration", result.Duration.String(),
		"statusCode", result.StatusCode,
	}

	if result.StatusCode >= 200 && result.StatusCode < 300 {
		application.logger.Info("delivered webhook", logValues...)
		return
	}

	logValues = append(logValues, "failureReason", result.FailureReason)
	application.logger.Warn("webhook delivery failed", logValues...)
}

func (application *Application) startMetricsReporter(requestContext context.Context) {
	go func() {
		reportTicker := time.NewTicker(application.config.MetricsReportInterval)
		defer reportTicker.Stop()

		for {
			select {
			case <-requestContext.Done():
				return
			case <-reportTicker.C:
				application.notifierMetrics.ScheduledQueueDepthGauge.Set(float64(application.scheduler.QueueDepth()))
			}
		}
	}()
}

func (application *Application) recordDeliveryMetrics(result events.DeliveryResult) {
	statusFamily := fmt.Sprintf("%dxx", result.StatusCode/100)
	if result.StatusCode == 0 {
		statusFamily = "transport"
	}

	application.notifierMetrics.DeliveryDurationHistogram.
		WithLabelValues(result.Job.Event.CustomerID, statusFamily).
		Observe(result.Duration.Seconds())

	if result.StatusCode >= 200 && result.StatusCode < 300 {
		application.notifierMetrics.DeliveredEventsCounter.WithLabelValues(result.Job.Event.CustomerID).Inc()
		return
	}

	application.notifierMetrics.FailedDeliveriesCounter.WithLabelValues(
		result.Job.Event.CustomerID,
		fmt.Sprintf("%t", result.ShouldRetry),
	).Inc()

	if result.ShouldRetry && application.retryPolicy.CanRetry(result.Job.Attempt) {
		application.notifierMetrics.RetriedDeliveriesCounter.Inc()
	}
}

func validateEvent(subscriberEvent events.SubscriberEvent) error {
	if strings.TrimSpace(subscriberEvent.EventID) == "" {
		return errors.New("eventId is required")
	}
	if strings.TrimSpace(subscriberEvent.CustomerID) == "" {
		return errors.New("customerId is required")
	}
	if strings.TrimSpace(subscriberEvent.SubscriberID) == "" {
		return errors.New("subscriberId is required")
	}
	if strings.TrimSpace(subscriberEvent.EventType) == "" {
		return errors.New("eventType is required")
	}
	if subscriberEvent.OccurredAt.IsZero() {
		return errors.New("occurredAt is required")
	}

	return nil
}
