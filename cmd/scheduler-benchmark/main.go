package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"webhook-notifier/internal/events"
	"webhook-notifier/internal/scheduler"
)

type benchmarkScenario struct {
	name string
	jobs []events.DeliveryJob
}

type benchmarkSummary struct {
	name          string
	jobCount      int
	nsPerOp       int64
	allocsPerOp   int64
	bytesPerOp    int64
	throughputOps float64
	jobsPerSecond float64
}

func main() {
	benchmarkOptions, optionsError := parseBenchmarkOptions()
	if optionsError != nil {
		fmt.Fprintf(os.Stderr, "parse benchmark options: %v\n", optionsError)
		os.Exit(1)
	}

	reportTime := time.Now().UTC()
	schedulerScenarios := []benchmarkScenario{
		newScenario("single-customer-burst", map[string]int{
			"customer-a": 1024,
		}),
		newScenario("balanced-three-customers", map[string]int{
			"customer-a": 512,
			"customer-b": 512,
			"customer-c": 512,
		}),
		newScenario("whale-scenario", map[string]int{
			"customer-a": 1024,
			"customer-b": 128,
			"customer-c": 128,
			"customer-d": 128,
		}),
		newScenario("huge-mixed-customer-load", map[string]int{
			"customer-a": 16384,
			"customer-b": 4096,
			"customer-c": 4096,
			"customer-d": 4096,
			"customer-e": 4096,
			"customer-f": 4096,
			"customer-g": 4096,
			"customer-h": 4096,
		}),
	}

	schedulerSummaries := make([]benchmarkSummary, 0, len(schedulerScenarios))
	for _, scenario := range schedulerScenarios {
		schedulerSummaries = append(schedulerSummaries, runScenarioBenchmark(scenario))
	}

	fairnessScenarioSummaries, fairnessScenarioError := runFairnessScenarios(benchmarkOptions)
	if fairnessScenarioError != nil {
		fmt.Fprintf(os.Stderr, "run fairness scenarios: %v\n", fairnessScenarioError)
		os.Exit(1)
	}

	consoleReportContent := buildConsoleReport(reportTime, benchmarkOptions, schedulerSummaries, fairnessScenarioSummaries)
	fmt.Print(consoleReportContent)

	reportContent := buildHTMLReport(reportTime, benchmarkOptions, schedulerSummaries, fairnessScenarioSummaries)

	reportPath, writeError := writeReport(reportTime, benchmarkOptions.mode, reportContent)
	if writeError != nil {
		fmt.Fprintf(os.Stderr, "write scheduler benchmark report: %v\n", writeError)
		os.Exit(1)
	}

	fmt.Printf("\nSaved report to %s\n", buildTerminalFileLink(reportPath))
}

func newScenario(name string, eventsPerCustomer map[string]int) benchmarkScenario {
	jobs := make([]events.DeliveryJob, 0, totalJobs(eventsPerCustomer))
	occurredAt := time.Date(2026, time.August, 3, 9, 0, 0, 0, time.UTC)
	customerIDs := make([]string, 0, len(eventsPerCustomer))
	for customerID := range eventsPerCustomer {
		customerIDs = append(customerIDs, customerID)
	}
	sort.Strings(customerIDs)

	for _, customerID := range customerIDs {
		eventCount := eventsPerCustomer[customerID]
		for eventIndex := 0; eventIndex < eventCount; eventIndex++ {
			eventID := fmt.Sprintf("%s-event-%04d", customerID, eventIndex)
			jobs = append(jobs, events.DeliveryJob{
				QueueItemID: int64(len(jobs) + 1),
				Event: events.SubscriberEvent{
					EventID:      eventID,
					CustomerID:   customerID,
					SubscriberID: "subscriber-" + eventID,
					EventType:    "subscriber.created",
					OccurredAt:   occurredAt,
				},
				WebhookURL: "https://example.com/webhook/" + customerID,
				Attempt:    0,
				EnqueuedAt: occurredAt,
				TraceID:    eventID,
			})
		}
	}

	return benchmarkScenario{
		name: name,
		jobs: jobs,
	}
}

func totalJobs(eventsPerCustomer map[string]int) int {
	totalJobCount := 0
	for _, eventCount := range eventsPerCustomer {
		totalJobCount += eventCount
	}
	return totalJobCount
}

func runScenarioBenchmark(scenario benchmarkScenario) benchmarkSummary {
	benchmarkResult := testing.Benchmark(func(benchmark *testing.B) {
		benchmark.ReportAllocs()
		for iterationIndex := 0; iterationIndex < benchmark.N; iterationIndex++ {
			requestContext, cancelRequest := context.WithCancel(context.Background())
			roundRobinScheduler := scheduler.NewRoundRobinScheduler(len(scenario.jobs))
			scheduledJobs := roundRobinScheduler.Start(requestContext)

			for _, job := range scenario.jobs {
				roundRobinScheduler.Enqueue(job)
			}

			for range scenario.jobs {
				<-scheduledJobs
			}

			roundRobinScheduler.Close()
			cancelRequest()

			for range scheduledJobs {
			}
		}
	})

	nsPerOp := benchmarkResult.NsPerOp()
	throughputOps := 0.0
	if nsPerOp > 0 {
		throughputOps = float64(time.Second) / float64(nsPerOp)
	}

	return benchmarkSummary{
		name:          scenario.name,
		jobCount:      len(scenario.jobs),
		nsPerOp:       nsPerOp,
		allocsPerOp:   benchmarkResult.AllocsPerOp(),
		bytesPerOp:    benchmarkResult.AllocedBytesPerOp(),
		throughputOps: throughputOps,
		jobsPerSecond: throughputOps * float64(len(scenario.jobs)),
	}
}

func buildConsoleReport(reportTime time.Time, benchmarkOptions benchmarkOptions, schedulerSummaries []benchmarkSummary, fairnessScenarioSummaries []fairnessScenarioSummary) string {
	var builder strings.Builder

	builder.WriteString("Scheduler Benchmark Report\n\n")
	builder.WriteString(fmt.Sprintf("Generated at %s UTC.\n\n", reportTime.Format("2006-01-02 15:04:05")))
	builder.WriteString(fmt.Sprintf("Mode: %s\n", benchmarkOptions.mode))
	if !benchmarkOptions.includeLargeScenario {
		builder.WriteString("Large fairness scenario: skipped by default for faster day-to-day runs\n")
	}
	builder.WriteString("\n")
	builder.WriteString("How to read this report\n")
	for _, line := range buildHowToReadReport(benchmarkOptions.mode) {
		builder.WriteString(fmt.Sprintf("- %s\n", line))
	}
	builder.WriteString("\n")
	builder.WriteString("Throughput benchmark scope\n")
	builder.WriteString("This benchmark measures the round-robin scheduler end-to-end within one benchmark iteration: enqueue jobs, emit scheduled jobs, and drain the scheduler output channel.\n\n")
	builder.WriteString(fmt.Sprintf("%-28s %14s %12s %12s %12s %12s %14s\n", "Scenario", "Jobs/iter", "ns/op", "allocs/op", "bytes/op", "ops/sec", "jobs/sec"))
	builder.WriteString(fmt.Sprintf("%-28s %14s %12s %12s %12s %12s %14s\n", strings.Repeat("-", 28), strings.Repeat("-", 14), strings.Repeat("-", 12), strings.Repeat("-", 12), strings.Repeat("-", 12), strings.Repeat("-", 12), strings.Repeat("-", 14)))

	for _, summary := range schedulerSummaries {
		builder.WriteString(fmt.Sprintf(
			"%-28s %14d %12d %12d %12d %12.2f %14.2f\n",
			summary.name,
			summary.jobCount,
			summary.nsPerOp,
			summary.allocsPerOp,
			summary.bytesPerOp,
			summary.throughputOps,
			summary.jobsPerSecond,
		))
	}

	if benchmarkOptions.mode == benchmarkModeApp {
		builder.WriteString("\nApp-mode confidence notes\n")
		for _, line := range appModeConfidenceSummary() {
			builder.WriteString(fmt.Sprintf("- %s\n", line))
		}
	}

	builder.WriteString(buildConsoleFairnessReport(benchmarkOptions.mode, fairnessScenarioSummaries))

	return builder.String()
}

func writeReport(reportTime time.Time, mode benchmarkMode, reportContent string) (string, error) {
	reportDirectory := filepath.Join("loadtest", "reports")
	if mkdirError := os.MkdirAll(reportDirectory, 0o755); mkdirError != nil {
		return "", mkdirError
	}

	reportPath := filepath.Join(
		reportDirectory,
		fmt.Sprintf("scheduler-benchmark-%s-%s.html", mode, reportTime.Format("20060102-150405")),
	)
	if writeError := os.WriteFile(reportPath, []byte(reportContent), 0o644); writeError != nil {
		return "", writeError
	}

	return reportPath, nil
}

func buildTerminalFileLink(reportPath string) string {
	absoluteReportPath, absolutePathError := filepath.Abs(reportPath)
	if absolutePathError != nil {
		return reportPath
	}

	reportURL := (&url.URL{Scheme: "file", Path: absoluteReportPath}).String()
	return fmt.Sprintf("\033]8;;%s\033\\%s\033]8;;\033\\", reportURL, absoluteReportPath)
}
