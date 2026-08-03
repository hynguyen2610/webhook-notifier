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
		builder.WriteString("Conclusion\n")
		for _, line := range fairnessConclusionBlock(mode, scenarioSummary) {
			builder.WriteString(fmt.Sprintf("- %s\n", line))
		}
		builder.WriteString("\n")
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
		builder.WriteString(fmt.Sprintf("%-8s %-11s %10s %-12s %17s %17s %17s %18s %17s %22s\n", "Workers", "Segment", "Messages", "Customer", "First completion", "Full completion", "Early completions", "Early-window share", "Full gap", "Normals before whales?"))
		builder.WriteString(fmt.Sprintf("%-8s %-11s %10s %-12s %17s %17s %17s %18s %17s %22s\n", strings.Repeat("-", 8), strings.Repeat("-", 11), strings.Repeat("-", 10), strings.Repeat("-", 12), strings.Repeat("-", 17), strings.Repeat("-", 17), strings.Repeat("-", 17), strings.Repeat("-", 18), strings.Repeat("-", 17), strings.Repeat("-", 22)))
		for _, runSummary := range scenarioSummary.workerRunSummaries {
			runInsights := analyzeFairnessRun(runSummary.customerSummaries)
			nonWhalesFinishedBeforeWhales := "no"
			if runInsights.nonWhalesFinishedBeforeWhales {
				nonWhalesFinishedBeforeWhales = "yes"
			}
			for _, customerSummary := range runSummary.customerSummaries {
				builder.WriteString(fmt.Sprintf(
					"%-8d %-11s %10d %-12s %17s %17s %17d %17.1f%% %17s %22s\n",
					runSummary.workerCount,
					customerSummary.customerSegment,
					customerSummary.jobCount,
					customerSummary.customerID,
					customerSummary.firstFinishDuration.Round(time.Millisecond).String(),
					customerSummary.finishDuration.Round(time.Millisecond).String(),
					customerSummary.earlyCompletionCount,
					customerSummary.earlyCompletionShare*100,
					formatDurationGap(runInsights.whaleVsNonWhaleCompletionGap),
					nonWhalesFinishedBeforeWhales,
				))
			}
		}
		builder.WriteString(fmt.Sprintf("\n%s\n", scenarioSummary.earlyWindowReason))
	}

	return builder.String()
}
