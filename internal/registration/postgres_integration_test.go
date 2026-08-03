package registration

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestPostgresRegistryIntegrationReadsRealRegistrations(t *testing.T) {
	// Input: a real PostgreSQL table containing active and inactive webhook registrations for multiple customers.
	// Outcome: registry ping, resolve, and snapshot calls return only active registrations from the live database.
	postgresDSN := os.Getenv("TEST_POSTGRES_DSN")
	if strings.TrimSpace(postgresDSN) == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}

	databaseConnection, openError := sql.Open("pgx", postgresDSN)
	if openError != nil {
		t.Fatalf("open postgres connection: %v", openError)
	}
	defer databaseConnection.Close()

	tableName := fmt.Sprintf("webhook_registrations_it_%d", time.Now().UnixNano())
	ctx := context.Background()

	if _, createError := databaseConnection.ExecContext(ctx, fmt.Sprintf(`
		CREATE TABLE %s (
			customer_id TEXT NOT NULL,
			webhook_url TEXT NOT NULL,
			is_active BOOLEAN NOT NULL DEFAULT TRUE
		)
	`, tableName)); createError != nil {
		t.Fatalf("create integration table: %v", createError)
	}
	defer func() {
		if _, dropError := databaseConnection.ExecContext(context.Background(), fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName)); dropError != nil {
			t.Fatalf("drop integration table: %v", dropError)
		}
	}()

	if _, insertError := databaseConnection.ExecContext(ctx, fmt.Sprintf(`
		INSERT INTO %s (customer_id, webhook_url, is_active)
		VALUES
			('customer-a', 'https://example.com/a-primary', TRUE),
			('customer-a', 'https://example.com/a-disabled', FALSE),
			('customer-b', 'https://example.com/b-primary', TRUE),
			('customer-b', 'https://example.com/b-secondary', TRUE)
	`, tableName)); insertError != nil {
		t.Fatalf("insert integration rows: %v", insertError)
	}

	registry := NewPostgresRegistryWithDatabaseConnection(
		databaseConnection,
		fmt.Sprintf("SELECT webhook_url FROM %s WHERE customer_id = $1 AND is_active = TRUE ORDER BY webhook_url", tableName),
		fmt.Sprintf("SELECT customer_id, webhook_url FROM %s WHERE is_active = TRUE ORDER BY customer_id, webhook_url", tableName),
	)

	if pingError := registry.Ping(ctx); pingError != nil {
		t.Fatalf("ping registry: %v", pingError)
	}

	customerAWebhookURLs, resolveError := registry.ResolveWebhookURLs(ctx, "customer-a")
	if resolveError != nil {
		t.Fatalf("resolve customer-a webhook URLs: %v", resolveError)
	}
	if len(customerAWebhookURLs) != 1 || customerAWebhookURLs[0] != "https://example.com/a-primary" {
		t.Fatalf("unexpected customer-a webhook URLs: %#v", customerAWebhookURLs)
	}

	customerSnapshot, snapshotError := registry.Snapshot(ctx)
	if snapshotError != nil {
		t.Fatalf("snapshot registrations: %v", snapshotError)
	}

	expectedSnapshot := map[string][]string{
		"customer-a": {"https://example.com/a-primary"},
		"customer-b": {"https://example.com/b-primary", "https://example.com/b-secondary"},
	}
	if fmt.Sprintf("%#v", customerSnapshot) != fmt.Sprintf("%#v", expectedSnapshot) {
		t.Fatalf("unexpected snapshot: got %#v want %#v", customerSnapshot, expectedSnapshot)
	}
}
