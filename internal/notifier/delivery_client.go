package notifier

import (
	"context"

	"webhook-notifier/internal/events"
)

type deliveryClient interface {
	Deliver(requestContext context.Context, job events.DeliveryJob) events.DeliveryResult
}
