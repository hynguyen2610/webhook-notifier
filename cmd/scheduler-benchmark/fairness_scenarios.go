package main

import (
	"context"
	"sync"
	"time"

	"webhook-notifier/internal/notifier"
	"webhook-notifier/internal/scheduler"
)

type fairnessScenarioDefinition struct {
	name                    string
	description             string
	eventsPerCustomer       map[string]int
	customerSegments        map[string]string
	workerCounts            []int
	syntheticWorkIterations int
}

type fairnessScenarioSummary struct {
	name               string
	description        string
	totalJobCount      int
	earlyWindowReason  string
	workerRunSummaries []fairnessWorkerRunSummary
}

type fairnessWorkerRunSummary struct {
	workerCount           int
	totalJobCount         int
	earlyCompletionWindow int
	totalDuration         time.Duration
	totalJobsPerSecond    float64
	customerSummaries     []customerFairnessSummary
}

type customerFairnessSummary struct {
	customerSegment      string
	customerID           string
	jobCount             int
	firstFinishDuration  time.Duration
	finishDuration       time.Duration
	earlyCompletionCount int
	earlyCompletionShare float64
	jobsPerSecond        float64
}

func runFairnessScenarios(options benchmarkOptions) ([]fairnessScenarioSummary, error) {
	definitions := fairnessScenarioDefinitions(options)

	summaries := make([]fairnessScenarioSummary, 0, len(definitions))
	for _, definition := range definitions {
		scenarioSummary, scenarioError := runFairnessScenario(definition, options.mode)
		if scenarioError != nil {
			return nil, scenarioError
		}
		summaries = append(summaries, scenarioSummary)
	}

	return summaries, nil
}

func fairnessScenarioDefinitions(options benchmarkOptions) []fairnessScenarioDefinition {
	definitions := []fairnessScenarioDefinition{
		{
			name:        "two-whales-100-two-normals-2",
			description: "Two whale customers with 100 messages each and two normal customers with 2 messages each.",
			eventsPerCustomer: map[string]int{
				"customer-a": 100,
				"customer-b": 100,
				"customer-c": 2,
				"customer-d": 2,
			},
			customerSegments: map[string]string{
				"customer-a": "whale",
				"customer-b": "whale",
				"customer-c": "non-whale",
				"customer-d": "non-whale",
			},
			workerCounts:            []int{1, 4, 8},
			syntheticWorkIterations: 60000,
		},
	}
	if options.includeLargeScenario {
		definitions = append(
			definitions,
			fairnessScenarioDefinition{
				name:        "two-whales-200000-two-normals-2",
				description: "Two whale customers with 200000 messages each and two normal customers with 2 messages each.",
				eventsPerCustomer: map[string]int{
					"customer-a": 200000,
					"customer-b": 200000,
					"customer-c": 2,
					"customer-d": 2,
				},
				customerSegments: map[string]string{
					"customer-a": "whale",
					"customer-b": "whale",
					"customer-c": "non-whale",
					"customer-d": "non-whale",
				},
				workerCounts:            []int{1, 4, 8},
				syntheticWorkIterations: 64,
			},
			fairnessScenarioDefinition{
				name:        "two-whales-200000-two-non-whales-1000",
				description: "Two whale customers with 200000 messages each and two non-whale customers with 1000 messages each.",
				eventsPerCustomer: map[string]int{
					"customer-a": 200000,
					"customer-b": 200000,
					"customer-c": 1000,
					"customer-d": 1000,
				},
				customerSegments: map[string]string{
					"customer-a": "whale",
					"customer-b": "whale",
					"customer-c": "non-whale",
					"customer-d": "non-whale",
				},
				workerCounts:            []int{1, 4, 8},
				syntheticWorkIterations: 64,
			},
		)
	}
	return definitions
}

func runFairnessScenario(definition fairnessScenarioDefinition, mode benchmarkMode) (fairnessScenarioSummary, error) {
	scenario := newScenario(definition.name, definition.eventsPerCustomer)
	customerOrder := sortedCustomerIDs(scenario.jobs)
	workerRunSummaries := make([]fairnessWorkerRunSummary, 0, len(definition.workerCounts))

	for _, workerCount := range definition.workerCounts {
		var runSummary fairnessWorkerRunSummary
		var runError error
		switch mode {
		case benchmarkModeApp:
			runSummary, runError = runAppFairnessWorkerRun(definition, workerCount)
		default:
			runSummary, runError = runSchedulerFairnessWorkerRun(
				scenario,
				customerOrder,
				definition.customerSegments,
				workerCount,
				definition.syntheticWorkIterations,
			)
		}
		if runError != nil {
			return fairnessScenarioSummary{}, runError
		}
		workerRunSummaries = append(workerRunSummaries, runSummary)
	}

	return fairnessScenarioSummary{
		name:               definition.name,
		description:        definition.description,
		totalJobCount:      len(scenario.jobs),
		earlyWindowReason:  earlyCompletionWindowExplanation(defaultEarlyCompletionWindow),
		workerRunSummaries: workerRunSummaries,
	}, nil
}

func runAppFairnessWorkerRun(definition fairnessScenarioDefinition, workerCount int) (fairnessWorkerRunSummary, error) {
	runSummary, runError := notifier.RunInMemoryFairnessScenario(notifier.BenchmarkFairnessScenario{
		EventsPerCustomer:       definition.eventsPerCustomer,
		CustomerSegments:        definition.customerSegments,
		WorkerCount:             workerCount,
		SyntheticWorkIterations: definition.syntheticWorkIterations,
		EarlyCompletionWindow:   defaultEarlyCompletionWindow,
	})
	if runError != nil {
		return fairnessWorkerRunSummary{}, runError
	}

	customerSummaries := make([]customerFairnessSummary, 0, len(runSummary.CustomerSummaries))
	for _, customerSummary := range runSummary.CustomerSummaries {
		customerSummaries = append(customerSummaries, customerFairnessSummary{
			customerSegment:      customerSummary.CustomerSegment,
			customerID:           customerSummary.CustomerID,
			jobCount:             customerSummary.JobCount,
			firstFinishDuration:  customerSummary.FirstFinishDuration,
			finishDuration:       customerSummary.FinishDuration,
			earlyCompletionCount: customerSummary.EarlyCompletionCount,
			earlyCompletionShare: customerSummary.EarlyCompletionShare,
		})
	}

	return fairnessWorkerRunSummary{
		workerCount:           runSummary.WorkerCount,
		totalJobCount:         runSummary.TotalJobCount,
		earlyCompletionWindow: runSummary.EarlyCompletionWindow,
		totalDuration:         runSummary.TotalDuration,
		totalJobsPerSecond:    runSummary.TotalJobsPerSecond,
		customerSummaries:     customerSummaries,
	}, nil
}

func runSchedulerFairnessWorkerRun(
	scenario benchmarkScenario,
	customerOrder []string,
	customerSegments map[string]string,
	workerCount int,
	syntheticWorkIterations int,
) (fairnessWorkerRunSummary, error) {
	requestContext, cancelRequest := context.WithCancel(context.Background())
	defer cancelRequest()

	roundRobinScheduler := scheduler.NewRoundRobinScheduler(len(scenario.jobs))
	scheduledJobs := roundRobinScheduler.Start(requestContext)
	targetCounts := customerJobCounts(scenario.jobs)
	completionCounts := make(map[string]int, len(customerOrder))
	firstTimes := make(map[string]time.Time, len(customerOrder))
	finishTimes := make(map[string]time.Time, len(customerOrder))
	earlyCompletionCounts := make(map[string]int, len(customerOrder))
	earlyCompletionWindow := defaultEarlyCompletionWindow
	if len(scenario.jobs) < earlyCompletionWindow {
		earlyCompletionWindow = len(scenario.jobs)
	}
	totalCompleted := 0
	var completionMutex sync.Mutex

	startedAt := time.Now()

	var workerGroup sync.WaitGroup
	var deliveryGroup sync.WaitGroup
	deliveryGroup.Add(len(scenario.jobs))
	for workerIndex := 0; workerIndex < workerCount; workerIndex++ {
		workerGroup.Add(1)
		go func() {
			defer workerGroup.Done()
			for {
				select {
				case <-requestContext.Done():
					return
				case job, channelOpen := <-scheduledJobs:
					if !channelOpen {
						return
					}

					runSyntheticDelivery(job, syntheticWorkIterations)

					completionMutex.Lock()
					customerID := job.Event.CustomerID
					totalCompleted++
					completionCounts[customerID]++
					if completionCounts[customerID] == 1 {
						firstTimes[customerID] = time.Now()
					}
					if totalCompleted <= earlyCompletionWindow {
						earlyCompletionCounts[customerID]++
					}
					if completionCounts[customerID] == targetCounts[customerID] {
						finishTimes[customerID] = time.Now()
					}
					completionMutex.Unlock()
					deliveryGroup.Done()
				}
			}
		}()
	}

	for _, job := range scenario.jobs {
		roundRobinScheduler.Enqueue(job)
	}

	deliveryGroup.Wait()
	roundRobinScheduler.Close()
	cancelRequest()
	workerGroup.Wait()

	totalDuration := time.Since(startedAt)
	if totalDuration <= 0 {
		totalDuration = time.Nanosecond
	}

	customerSummaries := make([]customerFairnessSummary, 0, len(customerOrder))
	for _, customerID := range customerOrder {
		firstFinishDuration := firstTimes[customerID].Sub(startedAt)
		if firstFinishDuration <= 0 {
			firstFinishDuration = time.Nanosecond
		}
		finishDuration := finishTimes[customerID].Sub(startedAt)
		if finishDuration <= 0 {
			finishDuration = time.Nanosecond
		}
		customerSummaries = append(customerSummaries, customerFairnessSummary{
			customerSegment:      customerSegments[customerID],
			customerID:           customerID,
			jobCount:             targetCounts[customerID],
			firstFinishDuration:  firstFinishDuration,
			finishDuration:       finishDuration,
			earlyCompletionCount: earlyCompletionCounts[customerID],
			earlyCompletionShare: float64(earlyCompletionCounts[customerID]) / float64(earlyCompletionWindow),
			jobsPerSecond:        float64(targetCounts[customerID]) / finishDuration.Seconds(),
		})
	}

	return fairnessWorkerRunSummary{
		workerCount:           workerCount,
		totalJobCount:         len(scenario.jobs),
		earlyCompletionWindow: earlyCompletionWindow,
		totalDuration:         totalDuration,
		totalJobsPerSecond:    float64(len(scenario.jobs)) / totalDuration.Seconds(),
		customerSummaries:     customerSummaries,
	}, nil
}
