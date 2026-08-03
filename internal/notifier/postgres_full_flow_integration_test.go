package notifier

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"webhook-notifier/internal/config"
	"webhook-notifier/internal/events"
	"webhook-notifier/internal/testsupport"
)

func TestNotifierIntegrationAcceptsHTTPEventAndDeliversWebhook(t *testing.T) {
	// Input: one valid event posted to the notifier HTTP API for a customer registered in PostgreSQL.
	// Outcome: the notifier accepts the request, writes one queue row, polls PostgreSQL, delivers the webhook, and completes the queue row.
	postgresDSN, cleanupPostgres := testsupport.PostgresDSN(t)
	defer cleanupPostgres()

	databaseConnection, openError := sql.Open("pgx", postgresDSN)
	if openError != nil {
		t.Fatalf("open postgres connection: %v", openError)
	}
	defer databaseConnection.Close()

	registrationTableName := fmt.Sprintf("webhook_registrations_full_flow_it_%d", time.Now().UnixNano())
	requestContext := context.Background()
	if _, createError := databaseConnection.ExecContext(requestContext, fmt.Sprintf(`
		CREATE TABLE %s (
			customer_id TEXT NOT NULL,
			webhook_url TEXT NOT NULL,
			is_active BOOLEAN NOT NULL DEFAULT TRUE
		)
	`, registrationTableName)); createError != nil {
		t.Fatalf("create full-flow registration table: %v", createError)
	}
	defer func() {
		if _, dropError := databaseConnection.ExecContext(context.Background(), fmt.Sprintf("DROP TABLE IF EXISTS %s", registrationTableName)); dropError != nil {
			t.Fatalf("drop full-flow registration table: %v", dropError)
		}
	}()

	webhookRequests := make(chan events.SubscriberEvent, 1)
	webhookServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Fatalf("expected POST request, got %s", request.Method)
		}

		var deliveredEvent events.SubscriberEvent
		if decodeError := json.NewDecoder(request.Body).Decode(&deliveredEvent); decodeError != nil {
			t.Fatalf("decode delivered event: %v", decodeError)
		}

		webhookRequests <- deliveredEvent
		responseWriter.WriteHeader(http.StatusOK)
	}))
	defer webhookServer.Close()

	if _, insertError := databaseConnection.ExecContext(requestContext, fmt.Sprintf(`
		INSERT INTO %s (customer_id, webhook_url, is_active)
		VALUES ('customer-a', $1, TRUE)
	`, registrationTableName), webhookServer.URL); insertError != nil {
		t.Fatalf("insert full-flow registration row: %v", insertError)
	}

	application, applicationError := NewApplication(config.NotifierConfig{
		HTTPAddress:               ":0",
		WorkerCount:               1,
		RequestTimeout:            2 * time.Second,
		MaxRetryAttempts:          1,
		InitialRetryDelay:         10 * time.Millisecond,
		QueueClaimBatchSize:       32,
		QueuePollInterval:         10 * time.Millisecond,
		PostgresConnection:        postgresDSN,
		RegistrationResolveQuery:  fmt.Sprintf("SELECT webhook_url FROM %s WHERE customer_id = $1 AND is_active = TRUE ORDER BY webhook_url", registrationTableName),
		RegistrationSnapshotQuery: fmt.Sprintf("SELECT customer_id, webhook_url FROM %s WHERE is_active = TRUE ORDER BY customer_id, webhook_url", registrationTableName),
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if applicationError != nil {
		t.Fatalf("initialize notifier application: %v", applicationError)
	}

	if _, truncateError := databaseConnection.ExecContext(requestContext, "TRUNCATE TABLE webhook_delivery_queue RESTART IDENTITY"); truncateError != nil {
		t.Fatalf("truncate queue table before full-flow test: %v", truncateError)
	}
	defer func() {
		if _, cleanupError := databaseConnection.ExecContext(context.Background(), "TRUNCATE TABLE webhook_delivery_queue RESTART IDENTITY"); cleanupError != nil {
			t.Fatalf("truncate queue table after full-flow test: %v", cleanupError)
		}
		if closeError := application.registry.Close(); closeError != nil {
			t.Fatalf("close application registry: %v", closeError)
		}
		if closeError := application.workQueue.Close(); closeError != nil {
			t.Fatalf("close application work queue: %v", closeError)
		}
		application.scheduler.Close()
	}()

	runContext, cancelRun := context.WithCancel(context.Background())
	workers := startTestWorkers(runContext, application, application.config.WorkerCount)
	defer func() {
		cancelRun()
		workers.Wait()
	}()

	notifierServer := httptest.NewServer(application.httpServer.Handler)
	defer notifierServer.Close()

	testEvent := newTestEvent("customer-a", "event-http-full-flow")
	requestBody, marshalError := json.Marshal(testEvent)
	if marshalError != nil {
		t.Fatalf("marshal notifier event request: %v", marshalError)
	}

	response, requestError := http.Post(notifierServer.URL+"/events", "application/json", bytes.NewReader(requestBody))
	if requestError != nil {
		t.Fatalf("post notifier event request: %v", requestError)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusAccepted {
		responseBody, _ := io.ReadAll(response.Body)
		t.Fatalf("expected status 202, got %d: %s", response.StatusCode, string(responseBody))
	}

	var ingestResult ingestResponse
	if decodeError := json.NewDecoder(response.Body).Decode(&ingestResult); decodeError != nil {
		t.Fatalf("decode ingest response: %v", decodeError)
	}
	if ingestResult.AcceptedEvents != 1 {
		t.Fatalf("expected acceptedEvents 1, got %d", ingestResult.AcceptedEvents)
	}
	if ingestResult.CreatedJobs != 1 {
		t.Fatalf("expected createdJobs 1, got %d", ingestResult.CreatedJobs)
	}

	queueRowCount := countQueueRowsForEvent(t, databaseConnection, testEvent.EventID)
	if queueRowCount != 1 {
		t.Fatalf("expected 1 queue row for %s, got %d", testEvent.EventID, queueRowCount)
	}

	select {
	case deliveredEvent := <-webhookRequests:
		assertDeliveredEventMatches(t, testEvent, deliveredEvent)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for full-flow webhook delivery")
	}

	waitForNotifierCount(t, func() int64 { return application.receivedEvents.Load() }, 1, "receivedEvents")
	waitForNotifierCount(t, func() int64 { return application.deliveredEvents.Load() }, 1, "deliveredEvents")
	waitForCondition(t, "full-flow queue row to be completed", func() bool {
		return queueStatusForEvent(t, databaseConnection, testEvent.EventID) == "completed"
	})
}
