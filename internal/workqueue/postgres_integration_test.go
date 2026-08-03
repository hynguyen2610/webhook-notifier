package workqueue

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"webhook-notifier/internal/events"
)

func TestPostgresRepositoryIntegrationRetriesThenSucceeds(t *testing.T) {
	// Input: one event enqueued for two webhook URLs where one queue item is retried once before final success.
	// Outcome: claim, retry, and success transitions persist in PostgreSQL with the updated attempt count and last error.
	repository, cleanup := newIntegrationRepository(t)
	defer cleanup()

	requestContext := context.Background()
	availableAt := time.Date(2026, time.August, 3, 9, 0, 0, 0, time.UTC)
	webhookURLs := []string{"https://example.com/a", "https://example.com/b"}

	createdJobs, enqueueError := repository.EnqueueDeliveries(requestContext, newIntegrationEvent("customer-a", "event-retry-success"), webhookURLs, availableAt)
	if enqueueError != nil {
		t.Fatalf("enqueue deliveries: %v", enqueueError)
	}
	if createdJobs != len(webhookURLs) {
		t.Fatalf("expected %d created jobs, got %d", len(webhookURLs), createdJobs)
	}

	claimedDeliveries, claimError := repository.ClaimAvailableDeliveries(requestContext, "worker-a", 2, availableAt.Add(time.Second))
	if claimError != nil {
		t.Fatalf("claim available deliveries: %v", claimError)
	}
	if len(claimedDeliveries) != 2 {
		t.Fatalf("expected 2 claimed deliveries, got %d", len(claimedDeliveries))
	}

	sort.Slice(claimedDeliveries, func(leftIndex int, rightIndex int) bool {
		return claimedDeliveries[leftIndex].Job.WebhookURL < claimedDeliveries[rightIndex].Job.WebhookURL
	})

	if markDeliveredError := repository.MarkDelivered(requestContext, claimedDeliveries[0].QueueItemID, availableAt.Add(2*time.Second)); markDeliveredError != nil {
		t.Fatalf("mark first delivery completed: %v", markDeliveredError)
	}

	retryAvailableAt := availableAt.Add(3 * time.Second)
	if markRetryError := repository.MarkRetryPending(
		requestContext,
		claimedDeliveries[1].QueueItemID,
		"temporary webhook failure",
		retryAvailableAt,
		retryAvailableAt,
	); markRetryError != nil {
		t.Fatalf("mark second delivery retry pending: %v", markRetryError)
	}

	earlyClaim, earlyClaimError := repository.ClaimAvailableDeliveries(requestContext, "worker-b", 2, retryAvailableAt.Add(-time.Millisecond))
	if earlyClaimError != nil {
		t.Fatalf("claim before retry availability: %v", earlyClaimError)
	}
	if len(earlyClaim) != 0 {
		t.Fatalf("expected 0 early claimed deliveries, got %d", len(earlyClaim))
	}

	retriedClaim, retriedClaimError := repository.ClaimAvailableDeliveries(requestContext, "worker-b", 2, retryAvailableAt.Add(time.Millisecond))
	if retriedClaimError != nil {
		t.Fatalf("claim retried delivery: %v", retriedClaimError)
	}
	if len(retriedClaim) != 1 {
		t.Fatalf("expected 1 retried delivery, got %d", len(retriedClaim))
	}
	if retriedClaim[0].Job.Attempt != 1 {
		t.Fatalf("expected retry attempt 1, got %d", retriedClaim[0].Job.Attempt)
	}
	if retriedClaim[0].Job.LastError != "temporary webhook failure" {
		t.Fatalf("expected retry last error to persist, got %q", retriedClaim[0].Job.LastError)
	}

	if markDeliveredError := repository.MarkDelivered(requestContext, retriedClaim[0].QueueItemID, retryAvailableAt.Add(time.Second)); markDeliveredError != nil {
		t.Fatalf("mark retried delivery completed: %v", markDeliveredError)
	}
}

func TestPostgresRepositoryIntegrationExhaustsRetryToDeadLetter(t *testing.T) {
	// Input: one event enqueued for one webhook URL and moved from pending to retryable to dead-lettered.
	// Outcome: PostgreSQL stores the dead-letter record with the final retry count and failure reason available for inspection.
	repository, cleanup := newIntegrationRepository(t)
	defer cleanup()

	requestContext := context.Background()
	availableAt := time.Date(2026, time.August, 3, 10, 0, 0, 0, time.UTC)

	createdJobs, enqueueError := repository.EnqueueDeliveries(
		requestContext,
		newIntegrationEvent("customer-b", "event-dead-letter"),
		[]string{"https://example.com/dead-letter"},
		availableAt,
	)
	if enqueueError != nil {
		t.Fatalf("enqueue dead-letter delivery: %v", enqueueError)
	}
	if createdJobs != 1 {
		t.Fatalf("expected 1 created job, got %d", createdJobs)
	}

	firstClaim, firstClaimError := repository.ClaimAvailableDeliveries(requestContext, "worker-a", 1, availableAt.Add(time.Second))
	if firstClaimError != nil {
		t.Fatalf("claim initial delivery: %v", firstClaimError)
	}
	if len(firstClaim) != 1 {
		t.Fatalf("expected 1 initial claim, got %d", len(firstClaim))
	}

	retryAvailableAt := availableAt.Add(2 * time.Second)
	if markRetryError := repository.MarkRetryPending(
		requestContext,
		firstClaim[0].QueueItemID,
		"persistent webhook failure",
		retryAvailableAt,
		retryAvailableAt,
	); markRetryError != nil {
		t.Fatalf("mark retry pending before dead letter: %v", markRetryError)
	}

	secondClaim, secondClaimError := repository.ClaimAvailableDeliveries(requestContext, "worker-b", 1, retryAvailableAt.Add(time.Millisecond))
	if secondClaimError != nil {
		t.Fatalf("claim retried delivery before dead letter: %v", secondClaimError)
	}
	if len(secondClaim) != 1 {
		t.Fatalf("expected 1 retried claim, got %d", len(secondClaim))
	}

	deadLetteredAt := retryAvailableAt.Add(time.Second)
	if markDeadLetterError := repository.MarkDeadLetter(requestContext, secondClaim[0].QueueItemID, "persistent webhook failure", deadLetteredAt); markDeadLetterError != nil {
		t.Fatalf("mark dead letter: %v", markDeadLetterError)
	}

	deadLetterMessages, snapshotError := repository.SnapshotDeadLetters(requestContext)
	if snapshotError != nil {
		t.Fatalf("snapshot dead letters: %v", snapshotError)
	}
	if len(deadLetterMessages) != 1 {
		t.Fatalf("expected 1 dead letter message, got %d", len(deadLetterMessages))
	}
	if deadLetterMessages[0].Job.Attempt != 1 {
		t.Fatalf("expected dead letter retry attempt 1, got %d", deadLetterMessages[0].Job.Attempt)
	}
	if deadLetterMessages[0].FailureReason != "persistent webhook failure" {
		t.Fatalf("expected dead letter failure reason to persist, got %q", deadLetterMessages[0].FailureReason)
	}
}

func TestPostgresRepositoryIntegrationClaimsDoNotOverlapAcrossWorkers(t *testing.T) {
	// Input: ten pending queue rows claimed concurrently by two workers with the same batch size.
	// Outcome: the workers receive disjoint queue item IDs because PostgreSQL row claiming uses skip-locked semantics.
	repository, cleanup := newIntegrationRepository(t)
	defer cleanup()

	requestContext := context.Background()
	availableAt := time.Date(2026, time.August, 3, 11, 0, 0, 0, time.UTC)
	webhookURLs := make([]string, 0, 10)
	for queueItemIndex := 0; queueItemIndex < 10; queueItemIndex++ {
		webhookURLs = append(webhookURLs, fmt.Sprintf("https://example.com/%02d", queueItemIndex))
	}

	createdJobs, enqueueError := repository.EnqueueDeliveries(requestContext, newIntegrationEvent("customer-c", "event-concurrent-claim"), webhookURLs, availableAt)
	if enqueueError != nil {
		t.Fatalf("enqueue concurrent claim deliveries: %v", enqueueError)
	}
	if createdJobs != len(webhookURLs) {
		t.Fatalf("expected %d created jobs, got %d", len(webhookURLs), createdJobs)
	}

	claimedBatches := make(chan []QueuedDelivery, 2)
	claimErrors := make(chan error, 2)
	startClaims := make(chan struct{})
	var claimGroup sync.WaitGroup

	for workerIndex := 0; workerIndex < 2; workerIndex++ {
		claimGroup.Add(1)
		go func(workerID int) {
			defer claimGroup.Done()
			<-startClaims
			claimedDeliveries, claimError := repository.ClaimAvailableDeliveries(
				requestContext,
				fmt.Sprintf("worker-%d", workerID),
				5,
				availableAt.Add(time.Second),
			)
			if claimError != nil {
				claimErrors <- claimError
				return
			}
			claimedBatches <- claimedDeliveries
		}(workerIndex + 1)
	}

	close(startClaims)
	claimGroup.Wait()
	close(claimErrors)
	close(claimedBatches)

	for claimError := range claimErrors {
		if claimError != nil {
			t.Fatalf("concurrent claim error: %v", claimError)
		}
	}

	seenQueueItemIDs := make(map[int64]struct{}, 10)
	totalClaimed := 0
	for claimedBatch := range claimedBatches {
		totalClaimed += len(claimedBatch)
		for _, claimedDelivery := range claimedBatch {
			if _, alreadyClaimed := seenQueueItemIDs[claimedDelivery.QueueItemID]; alreadyClaimed {
				t.Fatalf("queue item %d was claimed more than once", claimedDelivery.QueueItemID)
			}
			seenQueueItemIDs[claimedDelivery.QueueItemID] = struct{}{}
		}
	}

	if totalClaimed != 10 {
		t.Fatalf("expected 10 total claimed deliveries, got %d", totalClaimed)
	}
}

func newIntegrationRepository(t *testing.T) (*PostgresRepository, func()) {
	t.Helper()

	postgresDSN := os.Getenv("TEST_POSTGRES_DSN")
	if strings.TrimSpace(postgresDSN) == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}

	repository, repositoryError := NewPostgresRepository(postgresDSN)
	if repositoryError != nil {
		t.Fatalf("open postgres queue repository: %v", repositoryError)
	}

	requestContext := context.Background()
	if ensureSchemaError := repository.EnsureSchema(requestContext); ensureSchemaError != nil {
		t.Fatalf("ensure queue schema: %v", ensureSchemaError)
	}
	if _, cleanupError := repository.databaseConnection.ExecContext(requestContext, "TRUNCATE TABLE webhook_delivery_queue RESTART IDENTITY"); cleanupError != nil {
		t.Fatalf("truncate queue table before test: %v", cleanupError)
	}

	return repository, func() {
		if _, cleanupError := repository.databaseConnection.ExecContext(context.Background(), "TRUNCATE TABLE webhook_delivery_queue RESTART IDENTITY"); cleanupError != nil {
			t.Fatalf("truncate queue table after test: %v", cleanupError)
		}
		if closeError := repository.Close(); closeError != nil {
			t.Fatalf("close postgres queue repository: %v", closeError)
		}
	}
}

func newIntegrationEvent(customerID string, eventID string) events.SubscriberEvent {
	return events.SubscriberEvent{
		EventID:      eventID,
		CustomerID:   customerID,
		SubscriberID: "subscriber-" + eventID,
		EventType:    "subscriber.created",
		OccurredAt:   time.Date(2026, time.August, 3, 8, 30, 0, 0, time.UTC),
	}
}
