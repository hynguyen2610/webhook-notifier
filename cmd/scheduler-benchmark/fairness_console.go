package main

import (
	"fmt"
	"strings"
	"time"
)

func buildConsoleFairnessReport(mode benchmarkMode, fairnessScenarioSummaries []fairnessScenarioSummary) string {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("\nWorker Fairness Scenarios (%s mode)\n", mode))
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
