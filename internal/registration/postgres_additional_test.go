package registration

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestNewPostgresRegistryReturnsRegistry(t *testing.T) {
	// Input: a PostgreSQL connection string and registration queries.
	// Outcome: the constructor returns a usable registry wrapper.
	originalOpenConnection := openPostgresConnection
	originalCloseConnection := closePostgresConnection
	t.Cleanup(func() { openPostgresConnection = originalOpenConnection })
	t.Cleanup(func() { closePostgresConnection = originalCloseConnection })

	registry, createError := NewPostgresRegistry(
		"postgres://postgres:postgres@127.0.0.1:5432/webhook_notifier?sslmode=disable",
		"SELECT webhook_url",
		"SELECT customer_id, webhook_url",
	)
	if createError != nil {
		t.Fatalf("create postgres registry: %v", createError)
	}
	if registry == nil || registry.databaseConnection == nil {
		t.Fatalf("expected initialized registry, got %#v", registry)
	}
	_ = registry.Close()

	openPostgresConnection = func(string, string) (*sql.DB, error) {
		return nil, errors.New("open failed")
	}
	_, createError = NewPostgresRegistry("postgres://example", "SELECT webhook_url", "SELECT customer_id, webhook_url")
	if createError == nil || !strings.Contains(createError.Error(), "open postgres connection: open failed") {
		t.Fatalf("expected wrapped open error, got %v", createError)
	}
}

func TestPostgresRegistryResolveWebhookURLsReturnsQueryAndScanErrors(t *testing.T) {
	testCases := []struct {
		name      string
		setupMock func(sqlmock.Sqlmock)
		errorText string
	}{
		{
			name: "input query failure expects wrapped query error",
			setupMock: func(databaseMock sqlmock.Sqlmock) {
				databaseMock.ExpectQuery("SELECT webhook_url FROM webhook_registrations WHERE customer_id = \\$1").
					WithArgs("customer-a").
					WillReturnError(errors.New("query failed"))
			},
			errorText: "query webhook registrations: query failed",
		},
		{
			name: "input scan failure expects wrapped scan error",
			setupMock: func(databaseMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"webhook_url"}).AddRow(nil)
				databaseMock.ExpectQuery("SELECT webhook_url FROM webhook_registrations WHERE customer_id = \\$1").
					WithArgs("customer-a").
					WillReturnRows(rows)
			},
			errorText: "scan webhook registration",
		},
		{
			name: "input row iteration failure expects wrapped iterate error",
			setupMock: func(databaseMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"webhook_url"}).
					AddRow("https://example.com/a").
					RowError(0, errors.New("row failed"))
				databaseMock.ExpectQuery("SELECT webhook_url FROM webhook_registrations WHERE customer_id = \\$1").
					WithArgs("customer-a").
					WillReturnRows(rows)
			},
			errorText: "iterate webhook registrations: row failed",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			databaseConnection, databaseMock, mockError := sqlmock.New()
			if mockError != nil {
				t.Fatalf("create sql mock: %v", mockError)
			}
			defer databaseConnection.Close()

			registry := NewPostgresRegistryWithDatabaseConnection(
				databaseConnection,
				"SELECT webhook_url FROM webhook_registrations WHERE customer_id = $1",
				"SELECT customer_id, webhook_url FROM webhook_registrations",
			)
			testCase.setupMock(databaseMock)

			_, resolveError := registry.ResolveWebhookURLs(context.Background(), "customer-a")
			if resolveError == nil || !strings.Contains(resolveError.Error(), testCase.errorText) {
				t.Fatalf("expected error containing %q, got %v", testCase.errorText, resolveError)
			}
		})
	}
}

func TestPostgresRegistrySnapshotReturnsQueryScanAndIterateErrors(t *testing.T) {
	testCases := []struct {
		name      string
		setupMock func(sqlmock.Sqlmock)
		errorText string
	}{
		{
			name: "input snapshot query failure expects wrapped query error",
			setupMock: func(databaseMock sqlmock.Sqlmock) {
				databaseMock.ExpectQuery("SELECT customer_id, webhook_url FROM webhook_registrations").
					WillReturnError(errors.New("snapshot failed"))
			},
			errorText: "query webhook registration snapshot: snapshot failed",
		},
		{
			name: "input snapshot scan failure expects wrapped scan error",
			setupMock: func(databaseMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"customer_id", "webhook_url"}).AddRow("customer-a", nil)
				databaseMock.ExpectQuery("SELECT customer_id, webhook_url FROM webhook_registrations").
					WillReturnRows(rows)
			},
			errorText: "scan webhook registration snapshot",
		},
		{
			name: "input snapshot row failure expects wrapped iterate error",
			setupMock: func(databaseMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"customer_id", "webhook_url"}).
					AddRow("customer-a", "https://example.com/a").
					RowError(0, errors.New("snapshot row failed"))
				databaseMock.ExpectQuery("SELECT customer_id, webhook_url FROM webhook_registrations").
					WillReturnRows(rows)
			},
			errorText: "iterate webhook registration snapshot: snapshot row failed",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			databaseConnection, databaseMock, mockError := sqlmock.New()
			if mockError != nil {
				t.Fatalf("create sql mock: %v", mockError)
			}
			defer databaseConnection.Close()

			registry := NewPostgresRegistryWithDatabaseConnection(
				databaseConnection,
				"SELECT webhook_url FROM webhook_registrations WHERE customer_id = $1",
				"SELECT customer_id, webhook_url FROM webhook_registrations",
			)
			testCase.setupMock(databaseMock)

			_, snapshotError := registry.Snapshot(context.Background())
			if snapshotError == nil || !strings.Contains(snapshotError.Error(), testCase.errorText) {
				t.Fatalf("expected error containing %q, got %v", testCase.errorText, snapshotError)
			}
		})
	}
}

func TestPostgresRegistryPingAndCloseCoverSuccessAndFailurePaths(t *testing.T) {
	// Input: one registry with a ping failure, one with a ping success, and close calls for nil and non-nil connections.
	// Outcome: ping errors are wrapped, successful ping passes, and close handles both nil and active database handles.
	failingDatabase, failingMock, mockError := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if mockError != nil {
		t.Fatalf("create failing sql mock: %v", mockError)
	}
	failingMock.ExpectPing().WillReturnError(errors.New("ping failed"))

	failingRegistry := NewPostgresRegistryWithDatabaseConnection(failingDatabase, "", "")
	pingError := failingRegistry.Ping(context.Background())
	if pingError == nil || !strings.Contains(pingError.Error(), "ping postgres connection: ping failed") {
		t.Fatalf("expected wrapped ping error, got %v", pingError)
	}
	_ = failingDatabase.Close()

	successDatabase, successMock, successMockError := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if successMockError != nil {
		t.Fatalf("create success sql mock: %v", successMockError)
	}
	successMock.ExpectPing()

	successRegistry := NewPostgresRegistryWithDatabaseConnection(successDatabase, "", "")
	if pingError = successRegistry.Ping(context.Background()); pingError != nil {
		t.Fatalf("expected successful ping, got %v", pingError)
	}
	successMock.ExpectClose()
	if closeError := successRegistry.Close(); closeError != nil {
		t.Fatalf("close registry: %v", closeError)
	}

	nilRegistry := &PostgresRegistry{databaseConnection: nil}
	if closeError := nilRegistry.Close(); closeError != nil {
		t.Fatalf("close nil registry: %v", closeError)
	}

	closePostgresConnection = func(*sql.DB) error {
		return errors.New("close failed")
	}
	errorRegistry := &PostgresRegistry{databaseConnection: successDatabase}
	if closeError := errorRegistry.Close(); closeError == nil || closeError.Error() != "close failed" {
		t.Fatalf("expected close failure, got %v", closeError)
	}
}

var _ = sql.ErrConnDone
