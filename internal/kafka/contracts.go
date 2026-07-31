package kafka

import (
	"context"

	"webhook-notifier/internal/events"
)

type Publisher interface {
	Publish(subscriberEvents []events.SubscriberEvent) error
}

type Consumer interface {
	Start(requestContext context.Context, handler func(events.SubscriberEvent) error) error
}

type DeadLetterWriter interface {
	Publish(deadLetterMessage events.DeadLetterMessage) error
}
