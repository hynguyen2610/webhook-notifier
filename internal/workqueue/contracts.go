package workqueue

import (
	"context"
	"time"

	"webhook-notifier/internal/events"
)

type Repository interface {
	EnsureSchema(requestContext context.Context) error
	EnqueueDeliveries(requestContext context.Context, subscriberEvent events.SubscriberEvent, webhookURLs []string, availableAt time.Time) (int, error)
	ClaimAvailableDeliveries(requestContext context.Context, claimOwner string, limit int, claimedAt time.Time) ([]QueuedDelivery, error)
	MarkDelivered(requestContext context.Context, queueItemID int64, completedAt time.Time) error
	MarkRetryPending(requestContext context.Context, queueItemID int64, lastError string, nextAvailableAt time.Time, updatedAt time.Time) error
	MarkDeadLetter(requestContext context.Context, queueItemID int64, lastError string, deadLetteredAt time.Time) error
	SnapshotDeadLetters(requestContext context.Context) ([]events.DeadLetterMessage, error)
	Close() error
}

type QueuedDelivery struct {
	QueueItemID int64
	Job         events.DeliveryJob
}
