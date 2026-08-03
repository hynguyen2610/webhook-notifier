package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"webhook-notifier/internal/events"
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

var syntheticDeliverySink int

func runFairnessScenarios() ([]fairnessScenarioSummary, error) {
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
		{
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
	}

	summaries := make([]fairnessScenarioSummary, 0, len(definitions))
	for _, definition := range definitions {
		scenarioSummary, scenarioError := runFairnessScenario(definition)
		if scenarioError != nil {
			return nil, scenarioError
		}
		summaries = append(summaries, scenarioSummary)
	}

	return summaries, nil
}

func runFairnessScenario(definition fairnessScenarioDefinition) (fairnessScenarioSummary, error) {
	scenario := newScenario(definition.name, definition.eventsPerCustomer)
	customerOrder := sortedCustomerIDs(scenario.jobs)
	workerRunSummaries := make([]fairnessWorkerRunSummary, 0, len(definition.workerCounts))

	for _, workerCount := range definition.workerCounts {
		runSummary, runError := runFairnessWorkerRun(
			scenario,
			customerOrder,
			definition.customerSegments,
			workerCount,
			definition.syntheticWorkIterations,
		)
		if runError != nil {
			return fairnessScenarioSummary{}, runError
		}
		workerRunSummaries = append(workerRunSummaries, runSummary)
	}

	return fairnessScenarioSummary{
		name:               definition.name,
		description:        definition.description,
		totalJobCount:      len(scenario.jobs),
		workerRunSummaries: workerRunSummaries,
	}, nil
}

func runFairnessWorkerRun(
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
	earlyCompletionWindow := 20
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

func sortedCustomerIDs(jobs []events.DeliveryJob) []string {
	customerSet := make(map[string]struct{}, len(jobs))
	for _, job := range jobs {
		customerSet[job.Event.CustomerID] = struct{}{}
	}

	customerIDs := make([]string, 0, len(customerSet))
	for customerID := range customerSet {
		customerIDs = append(customerIDs, customerID)
	}
	sort.Strings(customerIDs)
	return customerIDs
}

func customerJobCounts(jobs []events.DeliveryJob) map[string]int {
	counts := make(map[string]int)
	for _, job := range jobs {
		counts[job.Event.CustomerID]++
	}
	return counts
}

func runSyntheticDelivery(job events.DeliveryJob, syntheticWorkIterations int) {
	payload := job.Event.EventID + job.Event.CustomerID + job.Event.SubscriberID
	if payload == "" {
		payload = "event"
	}

	checksum := 0
	for iterationIndex := 0; iterationIndex < syntheticWorkIterations; iterationIndex++ {
		payloadIndex := iterationIndex % len(payload)
		checksum += int(payload[payloadIndex]) + iterationIndex
	}

	syntheticDeliverySink += checksum
}

func buildConsoleFairnessReport(fairnessScenarioSummaries []fairnessScenarioSummary) string {
	var builder strings.Builder

	builder.WriteString("\nWorker Fairness Scenarios\n")
	for _, scenarioSummary := range fairnessScenarioSummaries {
		builder.WriteString("\n")
		builder.WriteString(fmt.Sprintf("%s\n", scenarioSummary.name))
		builder.WriteString(fmt.Sprintf("%s\n\n", scenarioSummary.description))
		builder.WriteString(fmt.Sprintf("%-8s %12s %12s %14s\n", "Workers", "Jobs", "Duration", "jobs/sec"))
		builder.WriteString(fmt.Sprintf("%-8s %12s %12s %14s\n", strings.Repeat("-", 8), strings.Repeat("-", 12), strings.Repeat("-", 12), strings.Repeat("-", 14)))
		for _, runSummary := range scenarioSummary.workerRunSummaries {
			builder.WriteString(fmt.Sprintf(
				"%-8d %12d %12s %14.2f\n",
				runSummary.workerCount,
				runSummary.totalJobCount,
				runSummary.totalDuration.Round(time.Millisecond).String(),
				runSummary.totalJobsPerSecond,
			))
		}

		builder.WriteString("\n")
		builder.WriteString(fmt.Sprintf("%-8s %-11s %10s %-12s %12s %12s %10s %8s\n", "Workers", "Segment", "Messages", "Customer", "First", "Finish", "Early", "Share"))
		builder.WriteString(fmt.Sprintf("%-8s %-11s %10s %-12s %12s %12s %10s %8s\n", strings.Repeat("-", 8), strings.Repeat("-", 11), strings.Repeat("-", 10), strings.Repeat("-", 12), strings.Repeat("-", 12), strings.Repeat("-", 12), strings.Repeat("-", 10), strings.Repeat("-", 8)))
		for _, runSummary := range scenarioSummary.workerRunSummaries {
			for _, customerSummary := range runSummary.customerSummaries {
				builder.WriteString(fmt.Sprintf(
					"%-8d %-11s %10d %-12s %12s %12s %10d %7.1f%%\n",
					runSummary.workerCount,
					customerSummary.customerSegment,
					customerSummary.jobCount,
					customerSummary.customerID,
					customerSummary.firstFinishDuration.Round(time.Millisecond).String(),
					customerSummary.finishDuration.Round(time.Millisecond).String(),
					customerSummary.earlyCompletionCount,
					customerSummary.earlyCompletionShare*100,
				))
			}
		}
		builder.WriteString(fmt.Sprintf("\nEarly share is measured over the first %d completed jobs in each worker run.\n", scenarioSummary.workerRunSummaries[0].earlyCompletionWindow))
	}

	return builder.String()
}
