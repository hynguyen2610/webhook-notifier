package workqueue

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"webhook-notifier/internal/events"
)

func TestPostgresRepositoryConstructorAndSchema(t *testing.T) {
	// Input: a PostgreSQL DSN for constructor coverage and a mocked database for schema creation success and failure.
	// Outcome: the constructor returns a repository, successful schema setup passes, and execution errors are wrapped.
	originalOpenConnection := openQueueConnection
	originalCloseConnection := closeQueueConnection
	t.Cleanup(func() { openQueueConnection = originalOpenConnection })
	t.Cleanup(func() { closeQueueConnection = originalCloseConnection })

	repository, createError := NewPostgresRepository("postgres://postgres:postgres@127.0.0.1:5432/webhook_notifier?sslmode=disable")
	if createError != nil {
		t.Fatalf("create postgres repository: %v", createError)
	}
	if repository == nil || repository.databaseConnection == nil {
		t.Fatalf("expected initialized repository, got %#v", repository)
	}
	_ = repository.Close()

	databaseConnection, databaseMock, mockError := sqlmock.New()
	if mockError != nil {
		t.Fatalf("create sql mock: %v", mockError)
	}
	defer databaseConnection.Close()

	mockRepository := NewPostgresRepositoryWithDatabaseConnection(databaseConnection)
	databaseMock.ExpectExec("CREATE TABLE IF NOT EXISTS webhook_delivery_queue").
		WillReturnResult(sqlmock.NewResult(0, 0))
	if ensureError := mockRepository.EnsureSchema(context.Background()); ensureError != nil {
		t.Fatalf("ensure schema: %v", ensureError)
	}

	failingDatabase, failingMock, failingMockError := sqlmock.New()
	if failingMockError != nil {
		t.Fatalf("create failing sql mock: %v", failingMockError)
	}
	defer failingDatabase.Close()
	failingRepository := NewPostgresRepositoryWithDatabaseConnection(failingDatabase)
	failingMock.ExpectExec("CREATE TABLE IF NOT EXISTS webhook_delivery_queue").
		WillReturnError(errors.New("schema failed"))
	ensureError := failingRepository.EnsureSchema(context.Background())
	if ensureError == nil || !strings.Contains(ensureError.Error(), "ensure queue schema: schema failed") {
		t.Fatalf("expected wrapped schema error, got %v", ensureError)
	}

	openQueueConnection = func(string, string) (*sql.DB, error) {
		return nil, errors.New("open failed")
	}
	_, createError = NewPostgresRepository("postgres://example")
	if createError == nil || !strings.Contains(createError.Error(), "open postgres queue connection: open failed") {
		t.Fatalf("expected wrapped open error, got %v", createError)
	}
}

func TestPostgresRepositoryEnqueueDeliveriesCoversSuccessAndTransactionErrors(t *testing.T) {
	testEvent := newQueueTestEvent()
	availableAt := time.Date(2026, time.August, 6, 12, 0, 0, 0, time.UTC)

	t.Run("input empty webhook list expects zero created jobs", func(t *testing.T) {
		repository := NewPostgresRepositoryWithDatabaseConnection(nil)
		createdJobs, enqueueError := repository.EnqueueDeliveries(context.Background(), testEvent, nil, availableAt)
		if enqueueError != nil || createdJobs != 0 {
			t.Fatalf("expected zero jobs with no error, got jobs=%d err=%v", createdJobs, enqueueError)
		}
	})

	t.Run("input begin failure expects wrapped transaction error", func(t *testing.T) {
		databaseConnection, databaseMock, mockError := sqlmock.New()
		if mockError != nil {
			t.Fatalf("create sql mock: %v", mockError)
		}
		defer databaseConnection.Close()

		repository := NewPostgresRepositoryWithDatabaseConnection(databaseConnection)
		databaseMock.ExpectBegin().WillReturnError(errors.New("begin failed"))

		_, enqueueError := repository.EnqueueDeliveries(context.Background(), testEvent, []string{"https://example.com/a"}, availableAt)
		if enqueueError == nil || !strings.Contains(enqueueError.Error(), "begin enqueue transaction: begin failed") {
			t.Fatalf("expected begin error, got %v", enqueueError)
		}
	})

	t.Run("input exec and commit paths expect wrapped errors and success", func(t *testing.T) {
		databaseConnection, databaseMock, mockError := sqlmock.New()
		if mockError != nil {
			t.Fatalf("create sql mock: %v", mockError)
		}
		defer databaseConnection.Close()

		repository := NewPostgresRepositoryWithDatabaseConnection(databaseConnection)
		databaseMock.ExpectBegin()
		databaseMock.ExpectExec("INSERT INTO webhook_delivery_queue").
			WillReturnError(errors.New("insert failed"))
		databaseMock.ExpectRollback()
		_, enqueueError := repository.EnqueueDeliveries(context.Background(), testEvent, []string{"https://example.com/a"}, availableAt)
		if enqueueError == nil || !strings.Contains(enqueueError.Error(), "insert queued delivery: insert failed") {
			t.Fatalf("expected insert error, got %v", enqueueError)
		}

		successDatabase, successMock, successMockError := sqlmock.New()
		if successMockError != nil {
			t.Fatalf("create success sql mock: %v", successMockError)
		}
		defer successDatabase.Close()

		successRepository := NewPostgresRepositoryWithDatabaseConnection(successDatabase)
		successMock.ExpectBegin()
		successMock.ExpectExec("INSERT INTO webhook_delivery_queue").
			WillReturnResult(sqlmock.NewResult(1, 1))
		successMock.ExpectCommit().WillReturnError(errors.New("commit failed"))
		_, enqueueError = successRepository.EnqueueDeliveries(context.Background(), testEvent, []string{"https://example.com/a"}, availableAt)
		if enqueueError == nil || !strings.Contains(enqueueError.Error(), "commit enqueue transaction: commit failed") {
			t.Fatalf("expected commit error, got %v", enqueueError)
		}
	})
}

func TestPostgresRepositoryClaimAvailableDeliveriesCoversSuccessAndFailurePaths(t *testing.T) {
	claimedAt := time.Date(2026, time.August, 6, 12, 0, 1, 0, time.UTC)

	t.Run("input successful claim expects queued delivery output", func(t *testing.T) {
		databaseConnection, databaseMock, mockError := sqlmock.New()
		if mockError != nil {
			t.Fatalf("create sql mock: %v", mockError)
		}
		defer databaseConnection.Close()

		repository := NewPostgresRepositoryWithDatabaseConnection(databaseConnection)
		rows := sqlmock.NewRows([]string{"id", "event_id", "customer_id", "subscriber_id", "event_type", "occurred_at", "webhook_url", "retry_count", "created_at", "coalesce"}).
			AddRow(int64(10), "event-1", "customer-a", "subscriber-1", events.SubscriberCreatedEventType, claimedAt, "https://example.com/a", 1, claimedAt.Add(-time.Minute), "temporary failure")
		databaseMock.ExpectBegin()
		databaseMock.ExpectQuery("WITH candidate_rows AS").
			WithArgs(claimedAt, 2, "worker-a").
			WillReturnRows(rows)
		databaseMock.ExpectCommit()

		queuedDeliveries, claimError := repository.ClaimAvailableDeliveries(context.Background(), "worker-a", 2, claimedAt)
		if claimError != nil {
			t.Fatalf("claim queued deliveries: %v", claimError)
		}
		if len(queuedDeliveries) != 1 || queuedDeliveries[0].Job.TraceID != "event-1" || queuedDeliveries[0].Job.Attempt != 1 {
			t.Fatalf("unexpected queued deliveries: %#v", queuedDeliveries)
		}
	})

	testCases := []struct {
		name      string
		setupMock func(sqlmock.Sqlmock)
		errorText string
	}{
		{
			name: "input begin failure expects wrapped transaction error",
			setupMock: func(databaseMock sqlmock.Sqlmock) {
				databaseMock.ExpectBegin().WillReturnError(errors.New("begin failed"))
			},
			errorText: "begin claim transaction: begin failed",
		},
		{
			name: "input query failure expects wrapped claim error",
			setupMock: func(databaseMock sqlmock.Sqlmock) {
				databaseMock.ExpectBegin()
				databaseMock.ExpectQuery("WITH candidate_rows AS").
					WillReturnError(errors.New("claim query failed"))
				databaseMock.ExpectRollback()
			},
			errorText: "claim queued deliveries: claim query failed",
		},
		{
			name: "input scan failure expects wrapped scan error",
			setupMock: func(databaseMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "event_id", "customer_id", "subscriber_id", "event_type", "occurred_at", "webhook_url", "retry_count", "created_at", "coalesce"}).
					AddRow("bad-id", "event-1", "customer-a", "subscriber-1", events.SubscriberCreatedEventType, claimedAt, "https://example.com/a", 1, claimedAt, "")
				databaseMock.ExpectBegin()
				databaseMock.ExpectQuery("WITH candidate_rows AS").WillReturnRows(rows)
				databaseMock.ExpectRollback()
			},
			errorText: "scan claimed queued delivery",
		},
		{
			name: "input row iteration failure expects wrapped iterate error",
			setupMock: func(databaseMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "event_id", "customer_id", "subscriber_id", "event_type", "occurred_at", "webhook_url", "retry_count", "created_at", "coalesce"}).
					AddRow(int64(10), "event-1", "customer-a", "subscriber-1", events.SubscriberCreatedEventType, claimedAt, "https://example.com/a", 1, claimedAt, "").
					RowError(0, errors.New("row failed"))
				databaseMock.ExpectBegin()
				databaseMock.ExpectQuery("WITH candidate_rows AS").WillReturnRows(rows)
				databaseMock.ExpectRollback()
			},
			errorText: "iterate claimed queued deliveries: row failed",
		},
		{
			name: "input commit failure expects wrapped commit error",
			setupMock: func(databaseMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "event_id", "customer_id", "subscriber_id", "event_type", "occurred_at", "webhook_url", "retry_count", "created_at", "coalesce"})
				databaseMock.ExpectBegin()
				databaseMock.ExpectQuery("WITH candidate_rows AS").WillReturnRows(rows)
				databaseMock.ExpectCommit().WillReturnError(errors.New("commit failed"))
			},
			errorText: "commit claim transaction: commit failed",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			databaseConnection, databaseMock, mockError := sqlmock.New()
			if mockError != nil {
				t.Fatalf("create sql mock: %v", mockError)
			}
			defer databaseConnection.Close()

			repository := NewPostgresRepositoryWithDatabaseConnection(databaseConnection)
			testCase.setupMock(databaseMock)
			_, claimError := repository.ClaimAvailableDeliveries(context.Background(), "worker-a", 2, claimedAt)
			if claimError == nil || !strings.Contains(claimError.Error(), testCase.errorText) {
				t.Fatalf("expected error containing %q, got %v", testCase.errorText, claimError)
			}
		})
	}
}

func TestPostgresRepositoryUpdateAndSnapshotMethods(t *testing.T) {
	// Input: mocked update and snapshot queries across success and failure paths.
	// Outcome: queue updates wrap database errors correctly and snapshots return expected state.
	databaseConnection, databaseMock, mockError := sqlmock.New()
	if mockError != nil {
		t.Fatalf("create sql mock: %v", mockError)
	}
	defer databaseConnection.Close()

	repository := NewPostgresRepositoryWithDatabaseConnection(databaseConnection)
	now := time.Date(2026, time.August, 6, 12, 0, 0, 0, time.UTC)
	databaseMock.ExpectExec("UPDATE webhook_delivery_queue").WillReturnResult(sqlmock.NewResult(0, 1))
	if updateError := repository.MarkDelivered(context.Background(), 1, now); updateError != nil {
		t.Fatalf("mark delivered: %v", updateError)
	}
	databaseMock.ExpectExec("UPDATE webhook_delivery_queue").WillReturnError(errors.New("exec failed"))
	if updateError := repository.MarkDelivered(context.Background(), 1, now); updateError == nil || !strings.Contains(updateError.Error(), "exec failed") {
		t.Fatalf("expected exec error, got %v", updateError)
	}
	databaseMock.ExpectExec("UPDATE webhook_delivery_queue").WillReturnResult(sqlmock.NewErrorResult(errors.New("rows failed")))
	if updateError := repository.MarkRetryPending(context.Background(), 1, "failed", now, now); updateError == nil || !strings.Contains(updateError.Error(), "rows failed") {
		t.Fatalf("expected rows affected error, got %v", updateError)
	}
	databaseMock.ExpectExec("UPDATE webhook_delivery_queue").WillReturnResult(sqlmock.NewResult(0, 0))
	if updateError := repository.MarkDeadLetter(context.Background(), 1, "failed", now); updateError == nil || !strings.Contains(updateError.Error(), "expected 1 affected queue row, got 0") {
		t.Fatalf("expected affected rows error, got %v", updateError)
	}

	snapshotRows := sqlmock.NewRows([]string{"count", "min"}).AddRow(2, now.Add(-time.Minute))
	databaseMock.ExpectQuery("SELECT COUNT\\(\\*\\), MIN\\(created_at\\)").WillReturnRows(snapshotRows)
	queueState, snapshotError := repository.SnapshotQueueState(context.Background())
	if snapshotError != nil || queueState.PendingDeliveryCount != 2 || !queueState.OldestPendingCreatedAt.Equal(now.Add(-time.Minute)) {
		t.Fatalf("unexpected queue state %#v error %v", queueState, snapshotError)
	}

	nullSnapshotRows := sqlmock.NewRows([]string{"count", "min"}).AddRow(0, nil)
	databaseMock.ExpectQuery("SELECT COUNT\\(\\*\\), MIN\\(created_at\\)").WillReturnRows(nullSnapshotRows)
	queueState, snapshotError = repository.SnapshotQueueState(context.Background())
	if snapshotError != nil || queueState.PendingDeliveryCount != 0 || !queueState.OldestPendingCreatedAt.IsZero() {
		t.Fatalf("expected zero queue state, got %#v error %v", queueState, snapshotError)
	}

	databaseMock.ExpectQuery("SELECT COUNT\\(\\*\\), MIN\\(created_at\\)").WillReturnError(errors.New("snapshot failed"))
	_, snapshotError = repository.SnapshotQueueState(context.Background())
	if snapshotError == nil || !strings.Contains(snapshotError.Error(), "query queue state snapshot: snapshot failed") {
		t.Fatalf("expected queue snapshot error, got %v", snapshotError)
	}

	deadLetterRows := sqlmock.NewRows([]string{"event_id", "customer_id", "subscriber_id", "event_type", "occurred_at", "webhook_url", "retry_count", "created_at", "coalesce", "dead_lettered_at"}).
		AddRow("event-1", "customer-a", "subscriber-1", events.SubscriberCreatedEventType, now, "https://example.com/a", 2, now.Add(-time.Minute), "failed", now)
	databaseMock.ExpectQuery("SELECT event_id, customer_id, subscriber_id").WillReturnRows(deadLetterRows)
	deadLetters, deadLetterError := repository.SnapshotDeadLetters(context.Background())
	if deadLetterError != nil || len(deadLetters) != 1 || deadLetters[0].Job.TraceID != "event-1" {
		t.Fatalf("unexpected dead letters %#v error %v", deadLetters, deadLetterError)
	}
}

func TestPostgresRepositoryDeadLetterSnapshotAndCloseErrors(t *testing.T) {
	testCases := []struct {
		name      string
		setupMock func(sqlmock.Sqlmock)
		errorText string
	}{
		{
			name: "input dead letter query failure expects wrapped query error",
			setupMock: func(databaseMock sqlmock.Sqlmock) {
				databaseMock.ExpectQuery("SELECT event_id, customer_id, subscriber_id").WillReturnError(errors.New("dead letter query failed"))
			},
			errorText: "query dead-letter deliveries: dead letter query failed",
		},
		{
			name: "input dead letter scan failure expects wrapped scan error",
			setupMock: func(databaseMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"event_id", "customer_id", "subscriber_id", "event_type", "occurred_at", "webhook_url", "retry_count", "created_at", "coalesce", "dead_lettered_at"}).
					AddRow("event-1", "customer-a", "subscriber-1", events.SubscriberCreatedEventType, "bad-time", "https://example.com/a", 1, time.Now(), "", time.Now())
				databaseMock.ExpectQuery("SELECT event_id, customer_id, subscriber_id").WillReturnRows(rows)
			},
			errorText: "scan dead-letter delivery",
		},
		{
			name: "input dead letter row failure expects wrapped iterate error",
			setupMock: func(databaseMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"event_id", "customer_id", "subscriber_id", "event_type", "occurred_at", "webhook_url", "retry_count", "created_at", "coalesce", "dead_lettered_at"}).
					AddRow("event-1", "customer-a", "subscriber-1", events.SubscriberCreatedEventType, time.Now(), "https://example.com/a", 1, time.Now(), "", time.Now()).
					RowError(0, errors.New("dead letter row failed"))
				databaseMock.ExpectQuery("SELECT event_id, customer_id, subscriber_id").WillReturnRows(rows)
			},
			errorText: "iterate dead-letter deliveries: dead letter row failed",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			databaseConnection, databaseMock, mockError := sqlmock.New()
			if mockError != nil {
				t.Fatalf("create sql mock: %v", mockError)
			}
			defer databaseConnection.Close()

			repository := NewPostgresRepositoryWithDatabaseConnection(databaseConnection)
			testCase.setupMock(databaseMock)
			_, deadLetterError := repository.SnapshotDeadLetters(context.Background())
			if deadLetterError == nil || !strings.Contains(deadLetterError.Error(), testCase.errorText) {
				t.Fatalf("expected error containing %q, got %v", testCase.errorText, deadLetterError)
			}
		})
	}

	nilRepository := &PostgresRepository{databaseConnection: nil}
	if closeError := nilRepository.Close(); closeError != nil {
		t.Fatalf("close nil repository: %v", closeError)
	}

	activeDatabase, activeMock, activeMockError := sqlmock.New()
	if activeMockError != nil {
		t.Fatalf("create active sql mock: %v", activeMockError)
	}
	activeMock.ExpectClose()
	repository := NewPostgresRepositoryWithDatabaseConnection(activeDatabase)
	if closeError := repository.Close(); closeError != nil && !errors.Is(closeError, sql.ErrConnDone) {
		t.Fatalf("close active repository: %v", closeError)
	}

	closeQueueConnection = func(*sql.DB) error {
		return errors.New("close failed")
	}
	if closeError := repository.Close(); closeError == nil || closeError.Error() != "close failed" {
		t.Fatalf("expected close failure, got %v", closeError)
	}
}

func newQueueTestEvent() events.SubscriberEvent {
	return events.SubscriberEvent{
		EventID:      "event-1",
		CustomerID:   "customer-a",
		SubscriberID: "subscriber-1",
		EventType:    events.SubscriberCreatedEventType,
		OccurredAt:   time.Date(2026, time.August, 6, 11, 59, 0, 0, time.UTC),
	}
}
