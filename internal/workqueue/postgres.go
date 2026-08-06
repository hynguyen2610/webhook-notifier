package workqueue

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"webhook-notifier/internal/events"
)

const queueTableName = "webhook_delivery_queue"

type PostgresRepository struct {
	databaseConnection *sql.DB
}

var openQueueConnection = sql.Open
var closeQueueConnection = func(databaseConnection *sql.DB) error {
	return databaseConnection.Close()
}

func NewPostgresRepository(connectionString string) (*PostgresRepository, error) {
	databaseConnection, openError := openQueueConnection("pgx", connectionString)
	if openError != nil {
		return nil, fmt.Errorf("open postgres queue connection: %w", openError)
	}

	return &PostgresRepository{databaseConnection: databaseConnection}, nil
}

func NewPostgresRepositoryWithDatabaseConnection(databaseConnection *sql.DB) *PostgresRepository {
	return &PostgresRepository{databaseConnection: databaseConnection}
}

func (repository *PostgresRepository) EnsureSchema(requestContext context.Context) error {
	_, executionError := repository.databaseConnection.ExecContext(requestContext, `
CREATE TABLE IF NOT EXISTS webhook_delivery_queue (
  id BIGSERIAL PRIMARY KEY,
  event_id TEXT NOT NULL,
  customer_id TEXT NOT NULL,
  subscriber_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  occurred_at TIMESTAMPTZ NOT NULL,
  webhook_url TEXT NOT NULL,
  status TEXT NOT NULL,
  available_at TIMESTAMPTZ NOT NULL,
  claimed_at TIMESTAMPTZ NULL,
  claim_owner TEXT NULL,
  retry_count INTEGER NOT NULL DEFAULT 0,
  last_error TEXT NULL,
  dead_lettered_at TIMESTAMPTZ NULL,
  completed_at TIMESTAMPTZ NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS webhook_delivery_queue_pending_idx
  ON webhook_delivery_queue (status, available_at, id);

CREATE INDEX IF NOT EXISTS webhook_delivery_queue_dead_letter_idx
  ON webhook_delivery_queue (status, dead_lettered_at DESC, id DESC);
`)
	if executionError != nil {
		return fmt.Errorf("ensure queue schema: %w", executionError)
	}

	return nil
}

/*
Job enqueueing also include the job enrichment in database
This system use database as the source of truth
*/
func (repository *PostgresRepository) EnqueueDeliveries(requestContext context.Context, subscriberEvent events.SubscriberEvent, webhookURLs []string, availableAt time.Time) (int, error) {
	if len(webhookURLs) == 0 {
		return 0, nil
	}

	transaction, beginError := repository.databaseConnection.BeginTx(requestContext, nil)
	if beginError != nil {
		return 0, fmt.Errorf("begin enqueue transaction: %w", beginError)
	}
	defer transaction.Rollback()

	createdAt := time.Now().UTC()
	for _, webhookURL := range webhookURLs {
		_, executionError := transaction.ExecContext(
			requestContext,
			`INSERT INTO webhook_delivery_queue (
  event_id, customer_id, subscriber_id, event_type, occurred_at, webhook_url,
  status, available_at, retry_count, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, 'pending', $7, 0, $8, $8)`,
			subscriberEvent.EventID,
			subscriberEvent.CustomerID,
			subscriberEvent.SubscriberID,
			subscriberEvent.EventType,
			subscriberEvent.OccurredAt,
			webhookURL,
			availableAt.UTC(),
			createdAt,
		)
		if executionError != nil {
			return 0, fmt.Errorf("insert queued delivery: %w", executionError)
		}
	}

	if commitError := transaction.Commit(); commitError != nil {
		return 0, fmt.Errorf("commit enqueue transaction: %w", commitError)
	}

	return len(webhookURLs), nil
}

func (repository *PostgresRepository) ClaimAvailableDeliveries(requestContext context.Context, claimOwner string, limit int, claimedAt time.Time) ([]QueuedDelivery, error) {
	transaction, beginError := repository.databaseConnection.BeginTx(requestContext, nil)
	if beginError != nil {
		return nil, fmt.Errorf("begin claim transaction: %w", beginError)
	}
	defer transaction.Rollback()

	rows, queryError := transaction.QueryContext(
		requestContext,
		`WITH candidate_rows AS (
  SELECT id
  FROM webhook_delivery_queue
  WHERE status = 'pending'
    AND available_at <= $1
  ORDER BY available_at, id
  LIMIT $2
  FOR UPDATE SKIP LOCKED
)
UPDATE webhook_delivery_queue
SET status = 'claimed',
    claimed_at = $1,
    claim_owner = $3,
    updated_at = $1
WHERE id IN (SELECT id FROM candidate_rows)
RETURNING id, event_id, customer_id, subscriber_id, event_type, occurred_at, webhook_url, retry_count, created_at, COALESCE(last_error, '')`,
		claimedAt.UTC(),
		limit,
		claimOwner,
	)
	if queryError != nil {
		return nil, fmt.Errorf("claim queued deliveries: %w", queryError)
	}
	defer rows.Close()

	queuedDeliveries := make([]QueuedDelivery, 0, limit)
	for rows.Next() {
		var queuedDelivery QueuedDelivery
		if scanError := rows.Scan(
			&queuedDelivery.QueueItemID,
			&queuedDelivery.Job.Event.EventID,
			&queuedDelivery.Job.Event.CustomerID,
			&queuedDelivery.Job.Event.SubscriberID,
			&queuedDelivery.Job.Event.EventType,
			&queuedDelivery.Job.Event.OccurredAt,
			&queuedDelivery.Job.WebhookURL,
			&queuedDelivery.Job.Attempt,
			&queuedDelivery.Job.EnqueuedAt,
			&queuedDelivery.Job.LastError,
		); scanError != nil {
			return nil, fmt.Errorf("scan claimed queued delivery: %w", scanError)
		}
		queuedDelivery.Job.QueueItemID = queuedDelivery.QueueItemID
		queuedDelivery.Job.TraceID = queuedDelivery.Job.Event.EventID
		queuedDeliveries = append(queuedDeliveries, queuedDelivery)
	}

	if rowsError := rows.Err(); rowsError != nil {
		return nil, fmt.Errorf("iterate claimed queued deliveries: %w", rowsError)
	}

	if commitError := transaction.Commit(); commitError != nil {
		return nil, fmt.Errorf("commit claim transaction: %w", commitError)
	}

	return queuedDeliveries, nil
}

func (repository *PostgresRepository) MarkDelivered(requestContext context.Context, queueItemID int64, completedAt time.Time) error {
	return repository.updateQueueItem(
		requestContext,
		`UPDATE webhook_delivery_queue
SET status = 'completed',
    completed_at = $2,
    updated_at = $2
WHERE id = $1`,
		queueItemID,
		completedAt.UTC(),
	)
}

func (repository *PostgresRepository) MarkRetryPending(requestContext context.Context, queueItemID int64, lastError string, nextAvailableAt time.Time, updatedAt time.Time) error {
	return repository.updateQueueItem(
		requestContext,
		`UPDATE webhook_delivery_queue
SET status = 'pending',
    available_at = $2,
    retry_count = retry_count + 1,
    last_error = $3,
    claimed_at = NULL,
    claim_owner = NULL,
    updated_at = $4
WHERE id = $1`,
		queueItemID,
		nextAvailableAt.UTC(),
		lastError,
		updatedAt.UTC(),
	)
}

func (repository *PostgresRepository) MarkDeadLetter(requestContext context.Context, queueItemID int64, lastError string, deadLetteredAt time.Time) error {
	return repository.updateQueueItem(
		requestContext,
		`UPDATE webhook_delivery_queue
SET status = 'dead_lettered',
    dead_lettered_at = $2,
    last_error = $3,
    claimed_at = NULL,
    claim_owner = NULL,
    updated_at = $2
WHERE id = $1`,
		queueItemID,
		deadLetteredAt.UTC(),
		lastError,
	)
}

func (repository *PostgresRepository) SnapshotQueueState(requestContext context.Context) (QueueStateSnapshot, error) {
	var queueState QueueStateSnapshot
	var oldestPendingCreatedAt sql.NullTime

	queryError := repository.databaseConnection.QueryRowContext(
		requestContext,
		`SELECT COUNT(*), MIN(created_at)
FROM webhook_delivery_queue
WHERE status = 'pending'`,
	).Scan(&queueState.PendingDeliveryCount, &oldestPendingCreatedAt)
	if queryError != nil {
		return QueueStateSnapshot{}, fmt.Errorf("query queue state snapshot: %w", queryError)
	}

	if oldestPendingCreatedAt.Valid {
		queueState.OldestPendingCreatedAt = oldestPendingCreatedAt.Time.UTC()
	}

	return queueState, nil
}

func (repository *PostgresRepository) SnapshotDeadLetters(requestContext context.Context) ([]events.DeadLetterMessage, error) {
	rows, queryError := repository.databaseConnection.QueryContext(
		requestContext,
		`SELECT event_id, customer_id, subscriber_id, event_type, occurred_at, webhook_url, retry_count, created_at, COALESCE(last_error, ''), dead_lettered_at
FROM webhook_delivery_queue
WHERE status = 'dead_lettered'
ORDER BY dead_lettered_at DESC, id DESC`,
	)
	if queryError != nil {
		return nil, fmt.Errorf("query dead-letter deliveries: %w", queryError)
	}
	defer rows.Close()

	deadLetterMessages := make([]events.DeadLetterMessage, 0)
	for rows.Next() {
		var deadLetterMessage events.DeadLetterMessage
		if scanError := rows.Scan(
			&deadLetterMessage.Job.Event.EventID,
			&deadLetterMessage.Job.Event.CustomerID,
			&deadLetterMessage.Job.Event.SubscriberID,
			&deadLetterMessage.Job.Event.EventType,
			&deadLetterMessage.Job.Event.OccurredAt,
			&deadLetterMessage.Job.WebhookURL,
			&deadLetterMessage.Job.Attempt,
			&deadLetterMessage.Job.EnqueuedAt,
			&deadLetterMessage.FailureReason,
			&deadLetterMessage.ExhaustedAt,
		); scanError != nil {
			return nil, fmt.Errorf("scan dead-letter delivery: %w", scanError)
		}
		deadLetterMessage.Job.TraceID = deadLetterMessage.Job.Event.EventID
		deadLetterMessages = append(deadLetterMessages, deadLetterMessage)
	}

	if rowsError := rows.Err(); rowsError != nil {
		return nil, fmt.Errorf("iterate dead-letter deliveries: %w", rowsError)
	}

	return deadLetterMessages, nil
}

func (repository *PostgresRepository) Close() error {
	if repository.databaseConnection == nil {
		return nil
	}

	closeError := closeQueueConnection(repository.databaseConnection)
	if closeError != nil && !errors.Is(closeError, sql.ErrConnDone) {
		return closeError
	}

	return nil
}

func (repository *PostgresRepository) updateQueueItem(requestContext context.Context, query string, arguments ...any) error {
	result, executionError := repository.databaseConnection.ExecContext(requestContext, query, arguments...)
	if executionError != nil {
		return executionError
	}

	affectedRows, rowsError := result.RowsAffected()
	if rowsError != nil {
		return rowsError
	}
	if affectedRows != 1 {
		return fmt.Errorf("expected 1 affected queue row, got %d", affectedRows)
	}

	return nil
}
