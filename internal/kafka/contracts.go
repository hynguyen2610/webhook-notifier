package kafka

import "webhook-notifier/internal/events"

type Publisher interface {
	Publish(subscriberEvents []events.SubscriberEvent) error
}

type Consumer interface {
	Start(handler func(events.SubscriberEvent) error) error
}
