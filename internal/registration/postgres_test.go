package registration

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestPostgresRegistryResolveWebhookURLsReturnsURLs(t *testing.T) {
	// Input: customer ID "customer-a" with two active webhook rows in PostgreSQL.
	// Outcome: registry returns both webhook URLs in query order with no error.
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

	rows := sqlmock.NewRows([]string{"webhook_url"}).
		AddRow("https://example.com/a").
		AddRow("https://example.com/b")
	databaseMock.ExpectQuery("SELECT webhook_url FROM webhook_registrations WHERE customer_id = \\$1").
		WithArgs("customer-a").
		WillReturnRows(rows)

	webhookURLs, resolveError := registry.ResolveWebhookURLs(context.Background(), "customer-a")
	if resolveError != nil {
		t.Fatalf("resolve webhook URLs: %v", resolveError)
	}

	if len(webhookURLs) != 2 {
		t.Fatalf("expected 2 webhook URLs, got %d", len(webhookURLs))
	}
	if webhookURLs[0] != "https://example.com/a" || webhookURLs[1] != "https://example.com/b" {
		t.Fatalf("unexpected webhook URLs: %#v", webhookURLs)
	}

	if expectationError := databaseMock.ExpectationsWereMet(); expectationError != nil {
		t.Fatalf("sql expectations: %v", expectationError)
	}
}

func TestPostgresRegistryResolveWebhookURLsReturnsNotRegistered(t *testing.T) {
	// Input: customer ID "missing-customer" with no matching webhook rows.
	// Outcome: registry returns ErrCustomerNotRegistered.
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

	rows := sqlmock.NewRows([]string{"webhook_url"})
	databaseMock.ExpectQuery("SELECT webhook_url FROM webhook_registrations WHERE customer_id = \\$1").
		WithArgs("missing-customer").
		WillReturnRows(rows)

	_, resolveError := registry.ResolveWebhookURLs(context.Background(), "missing-customer")
	if resolveError != ErrCustomerNotRegistered {
		t.Fatalf("expected ErrCustomerNotRegistered, got %v", resolveError)
	}

	if expectationError := databaseMock.ExpectationsWereMet(); expectationError != nil {
		t.Fatalf("sql expectations: %v", expectationError)
	}
}

func TestPostgresRegistrySnapshotGroupsWebhookURLsByCustomer(t *testing.T) {
	// Input: snapshot query rows for two customers, with two webhook URLs for customer-a.
	// Outcome: registry groups webhook URLs by customer ID in the returned snapshot map.
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

	rows := sqlmock.NewRows([]string{"customer_id", "webhook_url"}).
		AddRow("customer-a", "https://example.com/a-1").
		AddRow("customer-a", "https://example.com/a-2").
		AddRow("customer-b", "https://example.com/b-1")
	databaseMock.ExpectQuery("SELECT customer_id, webhook_url FROM webhook_registrations").
		WillReturnRows(rows)

	snapshot, snapshotError := registry.Snapshot(context.Background())
	if snapshotError != nil {
		t.Fatalf("snapshot registrations: %v", snapshotError)
	}

	if len(snapshot["customer-a"]) != 2 {
		t.Fatalf("expected customer-a to have 2 URLs, got %#v", snapshot["customer-a"])
	}
	if len(snapshot["customer-b"]) != 1 {
		t.Fatalf("expected customer-b to have 1 URL, got %#v", snapshot["customer-b"])
	}

	if expectationError := databaseMock.ExpectationsWereMet(); expectationError != nil {
		t.Fatalf("sql expectations: %v", expectationError)
	}
}
