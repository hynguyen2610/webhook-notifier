package registration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type PostgresRegistry struct {
	databaseConnection *sql.DB
	resolveQuery       string
	snapshotQuery      string
}

func NewPostgresRegistry(connectionString string, resolveQuery string, snapshotQuery string) (*PostgresRegistry, error) {
	databaseConnection, openError := sql.Open("pgx", connectionString)
	if openError != nil {

		return nil, fmt.Errorf("open postgres connection: %w", openError)
	}

	return NewPostgresRegistryWithDatabaseConnection(databaseConnection, resolveQuery, snapshotQuery), nil
}

func NewPostgresRegistryWithDatabaseConnection(databaseConnection *sql.DB, resolveQuery string, snapshotQuery string) *PostgresRegistry {
	return &PostgresRegistry{
		databaseConnection: databaseConnection,
		resolveQuery:       resolveQuery,
		snapshotQuery:      snapshotQuery,
	}
}

func (registry *PostgresRegistry) ResolveWebhookURLs(requestContext context.Context, customerID string) ([]string, error) {
	rows, queryError := registry.databaseConnection.QueryContext(requestContext, registry.resolveQuery, customerID)
	if queryError != nil {
		return nil, fmt.Errorf("query webhook registrations: %w", queryError)
	}
	defer rows.Close()

	webhookURLs := make([]string, 0, 1)
	for rows.Next() {
		var webhookURL string
		if scanError := rows.Scan(&webhookURL); scanError != nil {
			return nil, fmt.Errorf("scan webhook registration: %w", scanError)
		}
		webhookURLs = append(webhookURLs, webhookURL)
	}

	if rowsError := rows.Err(); rowsError != nil {
		return nil, fmt.Errorf("iterate webhook registrations: %w", rowsError)
	}

	if len(webhookURLs) == 0 {
		return nil, ErrCustomerNotRegistered
	}

	return webhookURLs, nil
}

func (registry *PostgresRegistry) Snapshot(requestContext context.Context) (map[string][]string, error) {
	rows, queryError := registry.databaseConnection.QueryContext(requestContext, registry.snapshotQuery)
	if queryError != nil {
		return nil, fmt.Errorf("query webhook registration snapshot: %w", queryError)
	}
	defer rows.Close()

	snapshot := make(map[string][]string)
	for rows.Next() {
		var customerID string
		var webhookURL string
		if scanError := rows.Scan(&customerID, &webhookURL); scanError != nil {
			return nil, fmt.Errorf("scan webhook registration snapshot: %w", scanError)
		}
		snapshot[customerID] = append(snapshot[customerID], webhookURL)
	}

	if rowsError := rows.Err(); rowsError != nil {
		return nil, fmt.Errorf("iterate webhook registration snapshot: %w", rowsError)
	}

	return snapshot, nil
}

func (registry *PostgresRegistry) Ping(requestContext context.Context) error {
	if pingError := registry.databaseConnection.PingContext(requestContext); pingError != nil {
		return fmt.Errorf("ping postgres connection: %w", pingError)
	}

	return nil
}

func (registry *PostgresRegistry) Close() error {
	if registry.databaseConnection == nil {
		return nil
	}

	closeError := registry.databaseConnection.Close()
	if closeError != nil && !errors.Is(closeError, sql.ErrConnDone) {
		return closeError
	}

	return nil
}
