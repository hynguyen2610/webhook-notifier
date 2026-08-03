package notifier

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"webhook-notifier/internal/config"
	"webhook-notifier/internal/events"
	"webhook-notifier/internal/metrics"
	"webhook-notifier/internal/retry"
	"webhook-notifier/internal/scheduler"
)

type BenchmarkFairnessScenario struct {
	EventsPerCustomer       map[string]int
	CustomerSegments        map[string]string
	WorkerCount             int
	SyntheticWorkIterations int
	EarlyCompletionWindow   int
}

type BenchmarkFairnessRunSummary struct {
	WorkerCount           int
	TotalJobCount         int
	EarlyCompletionWindow int
	TotalDuration         time.Duration
	TotalJobsPerSecond    float64
	CustomerSummaries     []BenchmarkFairnessCustomerSummary
}

type BenchmarkFairnessCustomerSummary struct {
	CustomerSegment      string
	CustomerID           string
	JobCount             int
	FirstFinishDuration  time.Duration
	FinishDuration       time.Duration
	EarlyCompletionCount int
	EarlyCompletionShare float64
}

func RunInMemoryFairnessScenario(scenario BenchmarkFairnessScenario) (BenchmarkFairnessRunSummary, error) {
	customerOrder := benchmarkCustomerOrder(scenario.EventsPerCustomer)
	totalJobCount := benchmarkTotalJobCount(scenario.EventsPerCustomer)
	earlyCompletionWindow := scenario.EarlyCompletionWindow
	if earlyCompletionWindow <= 0 || earlyCompletionWindow > totalJobCount {
		earlyCompletionWindow = totalJobCount
	}

	completionCounts := make(map[string]int, len(customerOrder))
	firstCompletionTimes := make(map[string]time.Time, len(customerOrder))
	finishCompletionTimes := make(map[string]time.Time, len(customerOrder))
	earlyCompletionCounts := make(map[string]int, len(customerOrder))
	totalCompleted := 0
	var completionMutex sync.Mutex
	var deliveryGroup sync.WaitGroup
	deliveryGroup.Add(totalJobCount)

	application := &Application{
		config: config.NotifierConfig{
			WorkerCount:         scenario.WorkerCount,
			RequestTimeout:      2 * time.Second,
			MaxRetryAttempts:    0,
			InitialRetryDelay:   10 * time.Millisecond,
			QueueClaimBatchSize: max(128, scenario.WorkerCount*64),
			QueuePollInterval:   time.Millisecond,
		},
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		registry:  newBenchmarkRegistry(scenario.EventsPerCustomer),
		workQueue: newBenchmarkQueue(),
		scheduler: scheduler.NewRoundRobinScheduler(max(128, scenario.WorkerCount*64)),
		deliveryClient: &benchmarkDeliveryClient{
			syntheticWorkIterations: scenario.SyntheticWorkIterations,
			onCompletion: func(customerID string, completedAt time.Time) {
				completionMutex.Lock()
				totalCompleted++
				completionCounts[customerID]++
				if completionCounts[customerID] == 1 {
					firstCompletionTimes[customerID] = completedAt
				}
				if totalCompleted <= earlyCompletionWindow {
					earlyCompletionCounts[customerID]++
				}
				if completionCounts[customerID] == scenario.EventsPerCustomer[customerID] {
					finishCompletionTimes[customerID] = completedAt
				}
				completionMutex.Unlock()
				deliveryGroup.Done()
			},
		},
		retryPolicy: retry.ExponentialBackoffPolicy{
			InitialDelay:    10 * time.Millisecond,
			MaxRetryAttempt: 0,
		},
		notifierMetrics: newBenchmarkNotifierMetrics(),
	}

	subscriberEvents := buildBenchmarkEvents(scenario.EventsPerCustomer)
	createdJobs, enqueueError := application.enqueueEvents(subscriberEvents)
	if enqueueError != nil {
		return BenchmarkFairnessRunSummary{}, enqueueError
	}
	if createdJobs != totalJobCount {
		return BenchmarkFairnessRunSummary{}, fmt.Errorf("expected %d created jobs, got %d", totalJobCount, createdJobs)
	}

	requestContext, cancelRequest := context.WithCancel(context.Background())
	defer cancelRequest()
	scheduledJobs := application.scheduler.Start(requestContext)
	startedAt := time.Now()

	var workerGroup sync.WaitGroup
	workerGroup.Add(1)
	go func() {
		defer workerGroup.Done()
		_ = application.runQueuePoller(requestContext)
	}()

	for workerIndex := 0; workerIndex < scenario.WorkerCount; workerIndex++ {
		workerGroup.Add(1)
		go func(workerID int) {
			defer workerGroup.Done()
			application.runWorker(requestContext, workerID, scheduledJobs)
		}(workerIndex + 1)
	}

	deliveryGroup.Wait()
	totalDuration := time.Since(startedAt)
	if totalDuration <= 0 {
		totalDuration = time.Nanosecond
	}

	cancelRequest()
	application.scheduler.Close()
	workerGroup.Wait()

	customerSummaries := make([]BenchmarkFairnessCustomerSummary, 0, len(customerOrder))
	for _, customerID := range customerOrder {
		firstDuration := firstCompletionTimes[customerID].Sub(startedAt)
		if firstDuration <= 0 {
			firstDuration = time.Nanosecond
		}
		finishDuration := finishCompletionTimes[customerID].Sub(startedAt)
		if finishDuration <= 0 {
			finishDuration = time.Nanosecond
		}
		customerSummaries = append(customerSummaries, BenchmarkFairnessCustomerSummary{
			CustomerSegment:      scenario.CustomerSegments[customerID],
			CustomerID:           customerID,
			JobCount:             scenario.EventsPerCustomer[customerID],
			FirstFinishDuration:  firstDuration,
			FinishDuration:       finishDuration,
			EarlyCompletionCount: earlyCompletionCounts[customerID],
			EarlyCompletionShare: float64(earlyCompletionCounts[customerID]) / float64(earlyCompletionWindow),
		})
	}

	return BenchmarkFairnessRunSummary{
		WorkerCount:           scenario.WorkerCount,
		TotalJobCount:         totalJobCount,
		EarlyCompletionWindow: earlyCompletionWindow,
		TotalDuration:         totalDuration,
		TotalJobsPerSecond:    float64(totalJobCount) / totalDuration.Seconds(),
		CustomerSummaries:     customerSummaries,
	}, nil
}

func buildBenchmarkEvents(eventsPerCustomer map[string]int) []events.SubscriberEvent {
	customerOrder := benchmarkCustomerOrder(eventsPerCustomer)
	subscriberEvents := make([]events.SubscriberEvent, 0, benchmarkTotalJobCount(eventsPerCustomer))
	occurredAt := time.Date(2026, time.August, 3, 9, 0, 0, 0, time.UTC)
	for _, customerID := range customerOrder {
		for eventIndex := 0; eventIndex < eventsPerCustomer[customerID]; eventIndex++ {
			eventID := benchmarkEventID(customerID, eventIndex)
			subscriberEvents = append(subscriberEvents, events.SubscriberEvent{
				EventID:      eventID,
				CustomerID:   customerID,
				SubscriberID: "subscriber-" + eventID,
				EventType:    "subscriber.created",
				OccurredAt:   occurredAt,
			})
		}
	}
	return subscriberEvents
}

func benchmarkCustomerOrder(eventsPerCustomer map[string]int) []string {
	customerOrder := make([]string, 0, len(eventsPerCustomer))
	for customerID := range eventsPerCustomer {
		customerOrder = append(customerOrder, customerID)
	}
	sort.Strings(customerOrder)
	return customerOrder
}

func benchmarkTotalJobCount(eventsPerCustomer map[string]int) int {
	totalJobCount := 0
	for _, eventCount := range eventsPerCustomer {
		totalJobCount += eventCount
	}
	return totalJobCount
}

func benchmarkEventID(customerID string, eventIndex int) string {
	return customerID + "-event-" + fmt.Sprintf("%06d", eventIndex)
}

func newBenchmarkNotifierMetrics() *metrics.NotifierMetrics {
	return &metrics.NotifierMetrics{
		ReceivedEventsCounter:     prometheus.NewCounter(prometheus.CounterOpts{Name: "benchmark_received_events_total", Help: "Benchmark counter."}),
		DeliveredEventsCounter:    prometheus.NewCounterVec(prometheus.CounterOpts{Name: "benchmark_delivered_events_total", Help: "Benchmark counter."}, []string{"customer_id"}),
		FailedDeliveriesCounter:   prometheus.NewCounterVec(prometheus.CounterOpts{Name: "benchmark_failed_deliveries_total", Help: "Benchmark counter."}, []string{"customer_id", "retryable"}),
		RetriedDeliveriesCounter:  prometheus.NewCounter(prometheus.CounterOpts{Name: "benchmark_retried_deliveries_total", Help: "Benchmark counter."}),
		DeadLetterCounter:         prometheus.NewCounter(prometheus.CounterOpts{Name: "benchmark_dead_letter_total", Help: "Benchmark counter."}),
		DeliveryDurationHistogram: prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "benchmark_delivery_duration_seconds", Help: "Benchmark histogram."}, []string{"customer_id", "status_family"}),
		ScheduledQueueDepthGauge:  prometheus.NewGauge(prometheus.GaugeOpts{Name: "benchmark_scheduled_queue_depth", Help: "Benchmark gauge."}),
	}
}

func max(leftValue int, rightValue int) int {
	if leftValue > rightValue {
		return leftValue
	}
	return rightValue
}
