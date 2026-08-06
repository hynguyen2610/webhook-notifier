package notifier

import (
	"context"
	"testing"
	"time"

	"webhook-notifier/internal/events"
)

func TestRunInMemoryFairnessScenarioBuildsReasonableSummary(t *testing.T) {
	// Input: a small fairness scenario with two customers, bounded synthetic work, and an oversized early-completion window.
	// Outcome: the run summary clamps the early window, completes all jobs, and reports customer-level progress data.
	summary, runError := RunInMemoryFairnessScenario(BenchmarkFairnessScenario{
		EventsPerCustomer: map[string]int{
			"customer-a": 2,
			"customer-b": 1,
		},
		CustomerSegments: map[string]string{
			"customer-a": "whale",
			"customer-b": "normal",
		},
		WorkerCount:             2,
		SyntheticWorkIterations: 5,
		EarlyCompletionWindow:   99,
	})
	if runError != nil {
		t.Fatalf("run fairness scenario: %v", runError)
	}
	if summary.TotalJobCount != 3 || summary.EarlyCompletionWindow != 3 || summary.WorkerCount != 2 {
		t.Fatalf("unexpected summary header: %#v", summary)
	}
	if summary.TotalDuration <= 0 || summary.TotalJobsPerSecond <= 0 {
		t.Fatalf("expected positive throughput metrics, got %#v", summary)
	}
	if len(summary.CustomerSummaries) != 2 {
		t.Fatalf("expected 2 customer summaries, got %#v", summary.CustomerSummaries)
	}
}

func TestBenchmarkHelpersCoverOrderingAndSyntheticDelivery(t *testing.T) {
	// Input: benchmark helper calls for customer ordering, event generation, queue state, and synthetic delivery completion.
	// Outcome: helpers return sorted customer IDs, generated events, queue snapshots, and successful synthetic delivery results.
	customerOrder := benchmarkCustomerOrder(map[string]int{"customer-b": 1, "customer-a": 2})
	if len(customerOrder) != 2 || customerOrder[0] != "customer-a" || customerOrder[1] != "customer-b" {
		t.Fatalf("unexpected customer order: %#v", customerOrder)
	}
	if benchmarkTotalJobCount(map[string]int{"customer-a": 2, "customer-b": 1}) != 3 {
		t.Fatal("expected total job count 3")
	}
	if benchmarkEventID("customer-a", 7) != "customer-a-event-000007" {
		t.Fatalf("unexpected benchmark event ID")
	}
	if max(5, 3) != 5 || max(2, 4) != 4 {
		t.Fatal("expected max helper to choose larger value")
	}

	subscriberEvents := buildBenchmarkEvents(map[string]int{"customer-b": 1, "customer-a": 2})
	if len(subscriberEvents) != 3 || subscriberEvents[0].CustomerID != "customer-a" || subscriberEvents[2].CustomerID != "customer-b" {
		t.Fatalf("unexpected benchmark events: %#v", subscriberEvents)
	}

	registry := newBenchmarkRegistry(map[string]int{"customer-a": 2})
	webhookURLs, resolveError := registry.ResolveWebhookURLs(context.Background(), "customer-a")
	if resolveError != nil || len(webhookURLs) != 1 || webhookURLs[0] != "benchmark://customer-a" {
		t.Fatalf("unexpected benchmark registry resolve result: %#v error %v", webhookURLs, resolveError)
	}
	if _, resolveError = registry.ResolveWebhookURLs(context.Background(), "missing"); resolveError == nil {
		t.Fatal("expected missing benchmark registry customer error")
	}
	snapshot, snapshotError := registry.Snapshot(context.Background())
	if snapshotError != nil || len(snapshot["customer-a"]) != 1 {
		t.Fatalf("unexpected benchmark registry snapshot: %#v error %v", snapshot, snapshotError)
	}
	snapshot["customer-a"][0] = "changed"
	originalSnapshot, _ := registry.Snapshot(context.Background())
	if originalSnapshot["customer-a"][0] != "benchmark://customer-a" {
		t.Fatal("expected benchmark snapshot copy to remain unchanged")
	}
	if closeError := registry.Close(); closeError != nil {
		t.Fatalf("close benchmark registry: %v", closeError)
	}

	queue := newBenchmarkQueue()
	if ensureError := queue.EnsureSchema(context.Background()); ensureError != nil {
		t.Fatalf("ensure benchmark queue schema: %v", ensureError)
	}
	createdJobs, enqueueError := queue.EnqueueDeliveries(
		context.Background(),
		newTestEvent("customer-a", "event-1"),
		[]string{"benchmark://customer-a"},
		time.Now().UTC(),
	)
	if enqueueError != nil || createdJobs != 1 {
		t.Fatalf("unexpected benchmark queue enqueue result jobs=%d err=%v", createdJobs, enqueueError)
	}
	queuedDeliveries, claimError := queue.ClaimAvailableDeliveries(context.Background(), "worker-a", 1, time.Now().UTC().Add(time.Second))
	if claimError != nil || len(queuedDeliveries) != 1 {
		t.Fatalf("unexpected benchmark queue claim result %#v err %v", queuedDeliveries, claimError)
	}
	if markError := queue.MarkRetryPending(context.Background(), queuedDeliveries[0].QueueItemID, "temporary", time.Now().UTC(), time.Now().UTC()); markError != nil {
		t.Fatalf("mark benchmark retry pending: %v", markError)
	}
	if markError := queue.MarkDelivered(context.Background(), queuedDeliveries[0].QueueItemID, time.Now().UTC()); markError != nil {
		t.Fatalf("mark benchmark delivered: %v", markError)
	}
	if markError := queue.MarkDeadLetter(context.Background(), queuedDeliveries[0].QueueItemID, "failed", time.Now().UTC()); markError != nil {
		t.Fatalf("mark benchmark dead letter: %v", markError)
	}
	queueState, queueStateError := queue.SnapshotQueueState(context.Background())
	if queueStateError != nil || queueState.PendingDeliveryCount != 0 {
		t.Fatalf("unexpected benchmark queue state %#v err %v", queueState, queueStateError)
	}
	deadLetters, deadLetterError := queue.SnapshotDeadLetters(context.Background())
	if deadLetterError != nil || len(deadLetters) != 1 {
		t.Fatalf("unexpected benchmark dead letters %#v err %v", deadLetters, deadLetterError)
	}
	if closeError := queue.Close(); closeError != nil {
		t.Fatalf("close benchmark queue: %v", closeError)
	}

	completionCalled := false
	client := &benchmarkDeliveryClient{
		syntheticWorkIterations: 4,
		onCompletion: func(customerID string, completedAt time.Time) {
			completionCalled = customerID == "customer-a" && !completedAt.IsZero()
		},
	}
	deliveryResult := client.Deliver(context.Background(), events.DeliveryJob{Event: newTestEvent("customer-a", "event-2")})
	if deliveryResult.StatusCode != 202 || deliveryResult.CompletedAt.IsZero() || !completionCalled {
		t.Fatalf("unexpected synthetic delivery result %#v completionCalled=%t", deliveryResult, completionCalled)
	}

	metrics := newBenchmarkNotifierMetrics()
	if metrics == nil || metrics.ReceivedEventsCounter == nil || metrics.ScheduledQueueDepthGauge == nil {
		t.Fatalf("expected benchmark metrics to be initialized")
	}
}
