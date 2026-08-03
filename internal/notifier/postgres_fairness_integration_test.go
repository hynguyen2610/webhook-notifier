package notifier

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"webhook-notifier/internal/config"
	"webhook-notifier/internal/delivery"
	"webhook-notifier/internal/events"
	"webhook-notifier/internal/registration"
	"webhook-notifier/internal/retry"
	"webhook-notifier/internal/scheduler"
	"webhook-notifier/internal/testsupport"
	"webhook-notifier/internal/workqueue"
)

func TestNotifierIntegrationPreservesFairnessWithPostgresQueue(t *testing.T) {
	// Input: ten events for customer-a and two events each for customer-b and customer-c enqueued into the real PostgreSQL queue before polling starts.
	// Outcome: after PostgreSQL claim and scheduler handoff, the first six deliveries alternate across customers so smaller customers make progress before the whale drains.
	postgresDSN, cleanupPostgres := testsupport.PostgresDSN(t)
	defer cleanupPostgres()

	databaseConnection, openError := sql.Open("pgx", postgresDSN)
	if openError != nil {
		t.Fatalf("open postgres connection: %v", openError)
	}

	registrationTableName := fmt.Sprintf("webhook_registrations_fairness_it_%d", time.Now().UnixNano())
	requestContext := context.Background()
	if _, createError := databaseConnection.ExecContext(requestContext, fmt.Sprintf(`
		CREATE TABLE %s (
			customer_id TEXT NOT NULL,
			webhook_url TEXT NOT NULL,
			is_active BOOLEAN NOT NULL DEFAULT TRUE
		)
	`, registrationTableName)); createError != nil {
		t.Fatalf("create fairness registration table: %v", createError)
	}
	deliveryOrder := make(chan string, 14)
	webhookServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		var deliveredEvent events.SubscriberEvent
		if decodeError := json.NewDecoder(request.Body).Decode(&deliveredEvent); decodeError != nil {
			t.Fatalf("decode delivered event: %v", decodeError)
		}

		deliveryOrder <- deliveredEvent.CustomerID
		responseWriter.WriteHeader(http.StatusOK)
	}))
	defer webhookServer.Close()

	for _, customerID := range []string{"customer-a", "customer-b", "customer-c"} {
		if _, insertError := databaseConnection.ExecContext(requestContext, fmt.Sprintf(`
			INSERT INTO %s (customer_id, webhook_url, is_active)
			VALUES ($1, $2, TRUE)
		`, registrationTableName), customerID, webhookServer.URL); insertError != nil {
			t.Fatalf("insert fairness registration row for %s: %v", customerID, insertError)
		}
	}

	registry := registration.NewPostgresRegistryWithDatabaseConnection(
		databaseConnection,
		fmt.Sprintf("SELECT webhook_url FROM %s WHERE customer_id = $1 AND is_active = TRUE ORDER BY webhook_url", registrationTableName),
		fmt.Sprintf("SELECT customer_id, webhook_url FROM %s WHERE is_active = TRUE ORDER BY customer_id, webhook_url", registrationTableName),
	)

	queueRepository, repositoryError := workqueue.NewPostgresRepository(postgresDSN)
	if repositoryError != nil {
		t.Fatalf("open postgres queue repository: %v", repositoryError)
	}
	if ensureSchemaError := queueRepository.EnsureSchema(requestContext); ensureSchemaError != nil {
		t.Fatalf("ensure queue schema: %v", ensureSchemaError)
	}
	if _, truncateError := databaseConnection.ExecContext(requestContext, "TRUNCATE TABLE webhook_delivery_queue RESTART IDENTITY"); truncateError != nil {
		t.Fatalf("truncate queue table before fairness integration test: %v", truncateError)
	}
	defer func() {
		if _, cleanupError := databaseConnection.ExecContext(context.Background(), "TRUNCATE TABLE webhook_delivery_queue RESTART IDENTITY"); cleanupError != nil {
			t.Fatalf("truncate queue table after fairness integration test: %v", cleanupError)
		}
		if _, dropError := databaseConnection.ExecContext(context.Background(), fmt.Sprintf("DROP TABLE IF EXISTS %s", registrationTableName)); dropError != nil {
			t.Fatalf("drop fairness registration table: %v", dropError)
		}
		if closeError := queueRepository.Close(); closeError != nil {
			t.Fatalf("close postgres queue repository: %v", closeError)
		}
		if closeError := registry.Close(); closeError != nil {
			t.Fatalf("close postgres registry: %v", closeError)
		}
		if closeError := databaseConnection.Close(); closeError != nil {
			t.Fatalf("close postgres connection: %v", closeError)
		}
	}()

	application := &Application{
		config: config.NotifierConfig{
			WorkerCount:         1,
			RequestTimeout:      200 * time.Millisecond,
			MaxRetryAttempts:    0,
			InitialRetryDelay:   10 * time.Millisecond,
			QueueClaimBatchSize: 32,
			QueuePollInterval:   10 * time.Millisecond,
		},
		logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		registry:       registry,
		workQueue:      queueRepository,
		scheduler:      scheduler.NewRoundRobinScheduler(4),
		deliveryClient: delivery.NewHTTPClient(200 * time.Millisecond),
		retryPolicy: retry.ExponentialBackoffPolicy{
			InitialDelay:    10 * time.Millisecond,
			MaxRetryAttempt: 0,
		},
		notifierMetrics: newTestNotifierMetrics(),
	}

	whaleEvents := make([]events.SubscriberEvent, 0, 14)
	for eventIndex := 0; eventIndex < 10; eventIndex++ {
		whaleEvents = append(whaleEvents, newTestEvent("customer-a", "postgres-event-a-"+strconv.Itoa(eventIndex)))
	}
	for eventIndex := 0; eventIndex < 2; eventIndex++ {
		whaleEvents = append(whaleEvents, newTestEvent("customer-b", "postgres-event-b-"+strconv.Itoa(eventIndex)))
		whaleEvents = append(whaleEvents, newTestEvent("customer-c", "postgres-event-c-"+strconv.Itoa(eventIndex)))
	}

	createdJobs, enqueueError := application.enqueueEvents(whaleEvents)
	if enqueueError != nil {
		t.Fatalf("enqueue postgres fairness events: %v", enqueueError)
	}
	if createdJobs != len(whaleEvents) {
		t.Fatalf("expected %d created jobs, got %d", len(whaleEvents), createdJobs)
	}

	runContext, cancelRun := context.WithCancel(context.Background())
	workers := startTestWorkers(runContext, application, 1)
	defer func() {
		cancelRun()
		application.scheduler.Close()
		workers.Wait()
	}()

	actualOrder := make([]string, 0, 6)
	for len(actualOrder) < 6 {
		select {
		case customerID := <-deliveryOrder:
			actualOrder = append(actualOrder, customerID)
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for postgres fairness delivery order, got %#v", actualOrder)
		}
	}

	assertBatchContainsCustomers(t, actualOrder[:3], []string{"customer-a", "customer-b", "customer-c"})
	assertBatchContainsCustomers(t, actualOrder[3:6], []string{"customer-a", "customer-b", "customer-c"})
}

func assertBatchContainsCustomers(t *testing.T, actualCustomers []string, expectedCustomers []string) {
	t.Helper()

	if len(actualCustomers) != len(expectedCustomers) {
		t.Fatalf("expected %d customers in batch, got %d", len(expectedCustomers), len(actualCustomers))
	}

	actualCounts := make(map[string]int, len(actualCustomers))
	for _, customerID := range actualCustomers {
		actualCounts[customerID]++
	}

	for _, expectedCustomerID := range expectedCustomers {
		if actualCounts[expectedCustomerID] != 1 {
			t.Fatalf("unexpected customer mix in batch: got %#v want %#v", actualCustomers, expectedCustomers)
		}
	}
}
