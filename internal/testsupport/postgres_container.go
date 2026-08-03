package testsupport

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	postgresmodule "github.com/testcontainers/testcontainers-go/modules/postgres"
)

func PostgresDSN(t *testing.T) (string, func()) {
	t.Helper()

	postgresDSN := strings.TrimSpace(os.Getenv("TEST_POSTGRES_DSN"))
	if postgresDSN != "" {
		return postgresDSN, func() {}
	}

	requestContext, cancelRequest := context.WithTimeout(context.Background(), 2*time.Minute)
	container, startError := postgresmodule.Run(
		requestContext,
		"postgres:16-alpine",
		postgresmodule.WithDatabase("webhook_notifier_test"),
		postgresmodule.WithUsername("postgres"),
		postgresmodule.WithPassword("postgres"),
	)
	cancelRequest()
	if startError != nil {
		t.Fatalf("start postgres test container: %v", startError)
	}

	connectionStringContext, cancelConnectionString := context.WithTimeout(context.Background(), 30*time.Second)
	postgresDSN, connectionStringError := container.ConnectionString(connectionStringContext, "sslmode=disable")
	cancelConnectionString()
	if connectionStringError != nil {
		terminateContainer(t, container)
		t.Fatalf("build postgres test container connection string: %v", connectionStringError)
	}
	waitForPostgresReady(t, postgresDSN)

	return postgresDSN, func() {
		terminateContainer(t, container)
	}
}

func waitForPostgresReady(t *testing.T, postgresDSN string) {
	t.Helper()

	databaseConnection, openError := sql.Open("pgx", postgresDSN)
	if openError != nil {
		t.Fatalf("open postgres test container connection: %v", openError)
	}
	defer databaseConnection.Close()

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		requestContext, cancelRequest := context.WithTimeout(context.Background(), 2*time.Second)
		pingError := databaseConnection.PingContext(requestContext)
		cancelRequest()
		if pingError == nil {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}

	t.Fatal("timed out waiting for postgres test container to accept connections")
}

func terminateContainer(t *testing.T, container *postgresmodule.PostgresContainer) {
	t.Helper()

	terminationContext, cancelTermination := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelTermination()
	if terminateError := container.Terminate(terminationContext); terminateError != nil {
		t.Fatalf("terminate postgres test container: %v", terminateError)
	}
}
