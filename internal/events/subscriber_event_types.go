package events

const (
	SubscriberCreatedEventType        = "subscriber.created"
	SubscriberAddedToSegmentEventType = "subscriber.added_to_segment"
	SubscriberUnsubscribedEventType   = "subscriber.unsubscribed"
)

var SupportedSubscriberEventTypes = []string{
	SubscriberCreatedEventType,
	SubscriberAddedToSegmentEventType,
	SubscriberUnsubscribedEventType,
}
