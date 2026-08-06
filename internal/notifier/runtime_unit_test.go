package notifier

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"webhook-notifier/internal/config"
	"webhook-notifier/internal/events"
	"webhook-notifier/internal/registration"
	"webhook-notifier/internal/retry"
	"webhook-notifier/internal/scheduler"
	"webhook-notifier/internal/workqueue"
)

func TestValidateEventAndDeliveryMetricsCoverCoreBranches(t *testing.T) {
	testCases := []struct {
		name      string
		event      events.SubscriberEvent
		expectedErr string
	}{
		{name: "input missing event id expects validation error", event: events.SubscriberEvent{}, expectedErr: "eventId is required"},
		{name: "input missing customer id expects validation error", event: events.SubscriberEvent{EventID: "1"}, expectedErr: "customerId is required"},
		{name: "input missing subscriber id expects validation error", event: events.SubscriberEvent{EventID: "1", CustomerID: "c"}, expectedErr: "subscriberId is required"},
		{name: "input missing event type expects validation error", event: events.SubscriberEvent{EventID: "1", CustomerID: "c", SubscriberID: "s"}, expectedErr: "eventType is required"},
		{name: "input missing occurred at expects validation error", event: events.SubscriberEvent{EventID: "1", CustomerID: "c", SubscriberID: "s", EventType: "t"}, expectedErr: "occurredAt is required"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			validationError := validateEvent(testCase.event)
			if validationError == nil || validationError.Error() != testCase.expectedErr {
				t.Fatalf("expected %q, got %v", testCase.expectedErr, validationError)
			}
		})
	}
	if validationError := validateEvent(newTestEvent("customer-a", "event-valid")); validationError != nil {
		t.Fatalf("expected valid event, got %v", validationError)
	}

	application := newUnitTestApplication()
	successResult := events.DeliveryResult{Job: events.DeliveryJob{Event: newTestEvent("customer-a", "event-1")}, StatusCode: 200, Duration: time.Second}
	retryResult := events.DeliveryResult{Job: events.DeliveryJob{Event: newTestEvent("customer-a", "event-2"), Attempt: 0}, StatusCode: 500, Duration: time.Second, ShouldRetry: true}
	transportResult := events.DeliveryResult{Job: events.DeliveryJob{Event: newTestEvent("customer-a", "event-3")}, Duration: time.Second, ShouldRetry: false}
	application.recordDeliveryMetrics(successResult)
	application.recordDeliveryMetrics(retryResult)
	application.recordDeliveryMetrics(transportResult)
	application.logDeliveryResult(1, successResult)
	application.logDeliveryResult(2, retryResult)

	if delivered := readCollectorValue(t, application.notifierMetrics.DeliveredEventsCounter.WithLabelValues("customer-a")); delivered != 1 {
		t.Fatalf("expected delivered counter 1, got %f", delivered)
	}
	if failed := readCollectorValue(t, application.notifierMetrics.FailedDeliveriesCounter.WithLabelValues("customer-a", "true")); failed != 1 {
		t.Fatalf("expected retryable failed counter 1, got %f", failed)
	}
	if failed := readCollectorValue(t, application.notifierMetrics.FailedDeliveriesCounter.WithLabelValues("customer-a", "false")); failed != 1 {
		t.Fatalf("expected non-retryable failed counter 1, got %f", failed)
	}
	if retried := readCollectorValue(t, application.notifierMetrics.RetriedDeliveriesCounter); retried != 1 {
		t.Fatalf("expected retried counter 1, got %f", retried)
	}
}

func TestQueuePollingWorkerAndOutcomeHandlersCoverBranches(t *testing.T) {
	application := newUnitTestApplication()
	unitQueue := application.workQueue.(*unitQueue)
	unitQueue.enqueueDeliveredJob(newTestEvent("customer-a", "event-1"), "https://example.com/a", time.Now().UTC())

	if claimError := application.claimAndSchedule(context.Background(), "worker-a"); claimError != nil {
		t.Fatalf("claim and schedule: %v", claimError)
	}
	if application.scheduler.QueueDepth() != 1 {
		t.Fatalf("expected scheduled queue depth 1, got %d", application.scheduler.QueueDepth())
	}

	unitQueue.claimError = errors.New("claim failed")
	if claimError := application.claimAndSchedule(context.Background(), "worker-a"); claimError == nil || claimError.Error() != "claim failed" {
		t.Fatalf("expected claim failure, got %v", claimError)
	}
	unitQueue.claimError = nil

	successResult := events.DeliveryResult{Job: events.DeliveryJob{QueueItemID: 1, Event: newTestEvent("customer-a", "event-1")}, StatusCode: 200, CompletedAt: time.Now()}
	if !application.handleSuccessfulDelivery(context.Background(), successResult) {
		t.Fatal("expected successful delivery")
	}
	if application.deliveredEvents.Load() != 1 {
		t.Fatalf("expected delivered events count 1, got %d", application.deliveredEvents.Load())
	}
	if application.handleSuccessfulDelivery(context.Background(), events.DeliveryResult{StatusCode: 500}) {
		t.Fatal("expected unsuccessful delivery branch")
	}

	unitQueue.markDeliveredError = errors.New("mark delivered failed")
	_ = application.handleSuccessfulDelivery(context.Background(), successResult)
	unitQueue.markDeliveredError = nil

	retryResult := events.DeliveryResult{Job: events.DeliveryJob{QueueItemID: 1, Event: newTestEvent("customer-a", "event-2"), Attempt: 0}, ShouldRetry: true, FailureReason: "retry later"}
	if !application.handleRetryableFailure(context.Background(), retryResult) {
		t.Fatal("expected retryable failure")
	}
	if application.retriedDeliveries.Load() != 1 || application.failedDeliveries.Load() == 0 {
		t.Fatalf("unexpected retry counters: retried=%d failed=%d", application.retriedDeliveries.Load(), application.failedDeliveries.Load())
	}
	if application.handleRetryableFailure(context.Background(), events.DeliveryResult{Job: events.DeliveryJob{Attempt: 9}, ShouldRetry: false}) {
		t.Fatal("expected non-retry branch")
	}

	unitQueue.markRetryError = errors.New("mark retry failed")
	_ = application.handleRetryableFailure(context.Background(), retryResult)
	unitQueue.markRetryError = nil

	application.handlePermanentFailure(events.DeliveryResult{Job: events.DeliveryJob{QueueItemID: 1, Event: newTestEvent("customer-a", "event-3")}, FailureReason: "failed forever"})
	if application.deadLetterCount.Load() != 1 {
		t.Fatalf("expected dead letter count 1, got %d", application.deadLetterCount.Load())
	}
	unitQueue.markDeadLetterError = errors.New("mark dead letter failed")
	application.recordDeadLetter(events.DeliveryJob{QueueItemID: 1, Event: newTestEvent("customer-a", "event-4")}, "failed again")
}

func TestClaimAndScheduleSkipsClaimsWhenScheduledQueueIsFullAndResumesAfterDrain(t *testing.T) {
	application := newUnitTestApplication()
	application.config.WorkerCount = 1
	application.config.ScheduledQueueLimitFactor = 2

	unitQueue := application.workQueue.(*unitQueue)
	unitQueue.enqueueDeliveredJob(newTestEvent("customer-a", "event-1"), "https://example.com/a", time.Now().UTC())
	unitQueue.enqueueDeliveredJob(newTestEvent("customer-a", "event-2"), "https://example.com/a", time.Now().UTC())

	application.scheduler.Enqueue(events.DeliveryJob{Event: newTestEvent("customer-a", "scheduled-1")})
	application.scheduler.Enqueue(events.DeliveryJob{Event: newTestEvent("customer-b", "scheduled-2")})
	if application.scheduler.QueueDepth() != 2 {
		t.Fatalf("expected scheduled queue depth 2 before bounded claim, got %d", application.scheduler.QueueDepth())
	}

	if claimError := application.claimAndSchedule(context.Background(), "worker-a"); claimError != nil {
		t.Fatalf("claim and schedule with full queue: %v", claimError)
	}
	if unitQueue.claimCallCount() != 0 {
		t.Fatalf("expected zero queue claims while scheduled queue is full, got %d", unitQueue.claimCallCount())
	}
	if application.scheduler.QueueDepth() != 2 {
		t.Fatalf("expected scheduled queue depth to remain 2, got %d", application.scheduler.QueueDepth())
	}

	scheduledJobs := application.scheduler.Start(context.Background())
	<-scheduledJobs

	if claimError := application.claimAndSchedule(context.Background(), "worker-a"); claimError != nil {
		t.Fatalf("claim and schedule after drain: %v", claimError)
	}
	if unitQueue.claimCallCount() != 1 {
		t.Fatalf("expected one queue claim after scheduled queue drained, got %d", unitQueue.claimCallCount())
	}
}

func TestPerformDeliveryAttemptAndRunWorkerCoverFlowBranches(t *testing.T) {
	application := newUnitTestApplication()
	application.deliveryClient = fakeDeliveryClient{
		result: events.DeliveryResult{
			Job:         events.DeliveryJob{QueueItemID: 1, Event: newTestEvent("customer-a", "event-1")},
			StatusCode:  200,
			CompletedAt: time.Now(),
			Duration:    time.Millisecond,
		},
	}

	deliveryResult := application.performDeliveryAttempt(context.Background(), 1, events.DeliveryJob{QueueItemID: 1, Event: newTestEvent("customer-a", "event-1")})
	if deliveryResult.StatusCode != 200 {
		t.Fatalf("expected delivery result, got %#v", deliveryResult)
	}

	requestContext, cancelRequest := context.WithCancel(context.Background())
	cancelRequest()
	application.runWorker(requestContext, 1, make(chan events.DeliveryJob))

	closedJobs := make(chan events.DeliveryJob)
	close(closedJobs)
	application.runWorker(context.Background(), 1, closedJobs)

	application.deliveryClient = fakeDeliveryClient{
		result: events.DeliveryResult{
			Job:           events.DeliveryJob{QueueItemID: 2, Event: newTestEvent("customer-a", "event-2")},
			StatusCode:    500,
			ShouldRetry:   false,
			FailureReason: "permanent failure",
			CompletedAt:   time.Now(),
		},
	}
	jobChannel := make(chan events.DeliveryJob, 1)
	jobChannel <- events.DeliveryJob{QueueItemID: 2, Event: newTestEvent("customer-a", "event-2")}
	close(jobChannel)
	application.runWorker(context.Background(), 1, jobChannel)
}

func TestMetricsReporterAndRuntimeHelpersCoverBranches(t *testing.T) {
	application := newUnitTestApplication()
	unitQueue := application.workQueue.(*unitQueue)
	application.config.MetricsReportInterval = 5 * time.Millisecond

	requestContext, cancelRequest := context.WithCancel(context.Background())
	application.startMetricsReporter(requestContext)
	waitForCondition(t, "metrics reporter zero oldest branch", func() bool {
		return readCollectorValue(t, application.notifierMetrics.OldestPendingAgeGauge) == 0
	})

	unitQueue.queueState = workqueue.QueueStateSnapshot{
		PendingDeliveryCount:   2,
		OldestPendingCreatedAt: time.Now().Add(2 * time.Second),
	}
	waitForCondition(t, "metrics reporter future oldest branch", func() bool {
		return readCollectorValue(t, application.notifierMetrics.PendingQueueDepthGauge) == 2 &&
			readCollectorValue(t, application.notifierMetrics.OldestPendingAgeGauge) == 0
	})

	unitQueue.snapshotQueueStateError = errors.New("snapshot failed")
	time.Sleep(15 * time.Millisecond)
	cancelRequest()

	serverErrors := make(chan error, 1)
	queueErrors := make(chan error, 1)
	serverErrors <- errors.New("server failed")
	if runtimeError := application.waitForRuntimeCompletion(context.Background(), serverErrors, queueErrors); runtimeError == nil || runtimeError.Error() != "server failed" {
		t.Fatalf("expected server error, got %v", runtimeError)
	}

	serverErrors = make(chan error, 1)
	queueErrors = make(chan error, 1)
	queueErrors <- errors.New("queue failed")
	if runtimeError := application.waitForRuntimeCompletion(context.Background(), serverErrors, queueErrors); runtimeError == nil || runtimeError.Error() != "queue failed" {
		t.Fatalf("expected queue error, got %v", runtimeError)
	}

	cancelledContext, cancel := context.WithCancel(context.Background())
	cancel()
	if runtimeError := application.waitForRuntimeCompletion(cancelledContext, make(chan error, 1), make(chan error, 1)); runtimeError != nil {
		t.Fatalf("expected nil runtime error after cancel, got %v", runtimeError)
	}
}

func TestServeHTTPShutdownAndRunCoverSuccessAndErrorPaths(t *testing.T) {
	application := newUnitTestApplication()
	application.httpServer = &http.Server{Addr: "bad-address", Handler: http.NewServeMux()}
	serverErrors := make(chan error, 1)
	application.serveHTTP(serverErrors)
	if serveError := <-serverErrors; serveError == nil {
		t.Fatal("expected serve error for invalid address")
	}

	listener, listenError := net.Listen("tcp", "127.0.0.1:0")
	if listenError != nil {
		t.Fatalf("listen: %v", listenError)
	}
	successApplication := newUnitTestApplication()
	successApplication.httpServer = &http.Server{Handler: http.NewServeMux()}
	go func() { _ = successApplication.httpServer.Serve(listener) }()
	if shutdownError := successApplication.shutdown(sync.WaitGroup{}); shutdownError != nil {
		t.Fatalf("shutdown application: %v", shutdownError)
	}

	errorApplication := newUnitTestApplication()
	errorApplication.httpServer = &http.Server{Handler: http.NewServeMux()}
	errorApplication.registry.(*unitRegistry).closeError = errors.New("registry close failed")
	if shutdownError := errorApplication.shutdown(sync.WaitGroup{}); shutdownError == nil || shutdownError.Error() != "registry close failed" {
		t.Fatalf("expected registry close error, got %v", shutdownError)
	}

	queueErrorApplication := newUnitTestApplication()
	queueErrorApplication.httpServer = &http.Server{Handler: http.NewServeMux()}
	queueErrorApplication.workQueue.(*unitQueue).closeError = errors.New("queue close failed")
	if shutdownError := queueErrorApplication.shutdown(sync.WaitGroup{}); shutdownError == nil || shutdownError.Error() != "queue close failed" {
		t.Fatalf("expected queue close error, got %v", shutdownError)
	}

	runApplication := newUnitTestApplication()
	runApplication.httpServer = &http.Server{Addr: "127.0.0.1:0", Handler: http.NewServeMux()}
	runContext, cancelRun := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancelRun()
	}()
	if runError := runApplication.Run(runContext); runError != nil {
		t.Fatalf("expected nil run error on cancel, got %v", runError)
	}

	errorRunApplication := newUnitTestApplication()
	errorRunApplication.httpServer = &http.Server{Addr: "127.0.0.1:0", Handler: http.NewServeMux()}
	errorRunApplication.workQueue.(*unitQueue).claimError = errors.New("claim failed")
	if runError := errorRunApplication.Run(context.Background()); runError == nil || runError.Error() != "claim failed" {
		t.Fatalf("expected queue poller error, got %v", runError)
	}
}

type fakeDeliveryClient struct {
	result events.DeliveryResult
}

func (client fakeDeliveryClient) Deliver(_ context.Context, _ events.DeliveryJob) events.DeliveryResult {
	return client.result
}

type unitRegistry struct {
	webhookURLsByCustomerID map[string][]string
	resolveError            error
	snapshotError           error
	closeError              error
}

func (registry *unitRegistry) ResolveWebhookURLs(_ context.Context, customerID string) ([]string, error) {
	if registry.resolveError != nil {
		return nil, registry.resolveError
	}
	webhookURLs, found := registry.webhookURLsByCustomerID[customerID]
	if !found {
		return nil, registration.ErrCustomerNotRegistered
	}
	return webhookURLs, nil
}

func (registry *unitRegistry) Snapshot(_ context.Context) (map[string][]string, error) {
	if registry.snapshotError != nil {
		return nil, registry.snapshotError
	}
	return registry.webhookURLsByCustomerID, nil
}

func (registry *unitRegistry) Close() error { return registry.closeError }

type unitQueue struct {
	*testQueue
	enqueueError              error
	claimError                error
	markDeliveredError        error
	markRetryError            error
	markDeadLetterError       error
	snapshotQueueStateError   error
	snapshotDeadLettersError  error
	queueState                workqueue.QueueStateSnapshot
	deadLetters               []events.DeadLetterMessage
	closeError                error
	queueClaimCount           int
}

func (queue *unitQueue) EnqueueDeliveries(requestContext context.Context, subscriberEvent events.SubscriberEvent, webhookURLs []string, availableAt time.Time) (int, error) {
	if queue.enqueueError != nil {
		return 0, queue.enqueueError
	}
	return queue.testQueue.EnqueueDeliveries(requestContext, subscriberEvent, webhookURLs, availableAt)
}

func (queue *unitQueue) ClaimAvailableDeliveries(requestContext context.Context, claimOwner string, limit int, claimedAt time.Time) ([]workqueue.QueuedDelivery, error) {
	if queue.claimError != nil {
		return nil, queue.claimError
	}
	queue.queueClaimCount++
	return queue.testQueue.ClaimAvailableDeliveries(requestContext, claimOwner, limit, claimedAt)
}

func (queue *unitQueue) MarkDelivered(requestContext context.Context, queueItemID int64, completedAt time.Time) error {
	if queue.markDeliveredError != nil {
		return queue.markDeliveredError
	}
	return queue.testQueue.MarkDelivered(requestContext, queueItemID, completedAt)
}

func (queue *unitQueue) MarkRetryPending(requestContext context.Context, queueItemID int64, lastError string, nextAvailableAt time.Time, updatedAt time.Time) error {
	if queue.markRetryError != nil {
		return queue.markRetryError
	}
	return queue.testQueue.MarkRetryPending(requestContext, queueItemID, lastError, nextAvailableAt, updatedAt)
}

func (queue *unitQueue) MarkDeadLetter(requestContext context.Context, queueItemID int64, lastError string, deadLetteredAt time.Time) error {
	if queue.markDeadLetterError != nil {
		return queue.markDeadLetterError
	}
	return queue.testQueue.MarkDeadLetter(requestContext, queueItemID, lastError, deadLetteredAt)
}

func (queue *unitQueue) SnapshotQueueState(_ context.Context) (workqueue.QueueStateSnapshot, error) {
	if queue.snapshotQueueStateError != nil {
		return workqueue.QueueStateSnapshot{}, queue.snapshotQueueStateError
	}
	return queue.queueState, nil
}

func (queue *unitQueue) SnapshotDeadLetters(_ context.Context) ([]events.DeadLetterMessage, error) {
	if queue.snapshotDeadLettersError != nil {
		return nil, queue.snapshotDeadLettersError
	}
	return queue.deadLetters, nil
}

func (queue *unitQueue) Close() error { return queue.closeError }

func (queue *unitQueue) claimCallCount() int {
	return queue.queueClaimCount
}

func (queue *unitQueue) enqueueDeliveredJob(event events.SubscriberEvent, webhookURL string, availableAt time.Time) {
	_, _ = queue.testQueue.EnqueueDeliveries(context.Background(), event, []string{webhookURL}, availableAt)
}

func newUnitTestApplication() *Application {
	unitRegistry := &unitRegistry{
		webhookURLsByCustomerID: map[string][]string{"customer-a": {"https://example.com/a"}},
	}
	unitQueue := &unitQueue{testQueue: newTestQueue()}
	return &Application{
		config: config.NotifierConfig{
			HTTPAddress:               "127.0.0.1:0",
			WorkerCount:               1,
			RequestTimeout:            time.Second,
			MaxRetryAttempts:          2,
			InitialRetryDelay:         time.Millisecond,
			QueueClaimBatchSize:       2,
			QueuePollInterval:         5 * time.Millisecond,
			SchedulerBufferMultiplier: 2,
			ScheduledQueueLimitFactor: 10,
			MetricsReportInterval:     5 * time.Millisecond,
			ShutdownTimeout:           time.Second,
		},
		logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		registry:       unitRegistry,
		workQueue:      unitQueue,
		scheduler:      scheduler.NewRoundRobinScheduler(4),
		deliveryClient: fakeDeliveryClient{},
		retryPolicy: retry.ExponentialBackoffPolicy{
			InitialDelay:    time.Millisecond,
			MaxRetryAttempt: 2,
		},
		notifierMetrics: newTestNotifierMetrics(),
		httpServer:      &http.Server{Handler: http.NewServeMux()},
	}
}

func readCollectorValue(t *testing.T, collector interface{ Collect(chan<- prometheus.Metric) }) float64 {
	t.Helper()

	metricChannel := make(chan prometheus.Metric, 1)
	collector.Collect(metricChannel)
	metric := <-metricChannel

	var metricDTO dto.Metric
	if writeError := metric.Write(&metricDTO); writeError != nil {
		t.Fatalf("write metric dto: %v", writeError)
	}
	if metricDTO.Counter != nil {
		return metricDTO.Counter.GetValue()
	}
	if metricDTO.Gauge != nil {
		return metricDTO.Gauge.GetValue()
	}
	return 0
}
