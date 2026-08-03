package events

import "time"

type SubscriberEvent struct {
	EventID      string    `json:"eventId"`
	CustomerID   string    `json:"customerId"`
	SubscriberID string    `json:"subscriberId"`
	EventType    string    `json:"eventType"`
	OccurredAt   time.Time `json:"occurredAt"`
}

type DeliveryJob struct {
	QueueItemID int64           `json:"queueItemId,omitempty"`
	Event       SubscriberEvent `json:"event"`
	WebhookURL  string          `json:"webhookUrl"`
	Attempt     int             `json:"attempt"`
	EnqueuedAt  time.Time       `json:"enqueuedAt"`
	LastError   string          `json:"lastError,omitempty"`
	TraceID     string          `json:"traceId,omitempty"`
}

type DeliveryResult struct {
	Job           DeliveryJob
	StatusCode    int
	Duration      time.Duration
	ShouldRetry   bool
	FailureReason string
	ResponseBody  string
	CompletedAt   time.Time
}

type DeadLetterMessage struct {
	Job           DeliveryJob `json:"job"`
	FailureReason string      `json:"failureReason"`
	ExhaustedAt   time.Time   `json:"exhaustedAt"`
}
