package main

import (
	"fmt"
	"time"
)

const defaultEarlyCompletionWindow = 20

type fairnessRunInsights struct {
	averageWhaleFirstCompletion    time.Duration
	averageNonWhaleFirstCompletion time.Duration
	averageWhaleFullCompletion     time.Duration
	averageNonWhaleFullCompletion  time.Duration
	whaleVsNonWhaleCompletionGap   time.Duration
	nonWhalesFinishedBeforeWhales  bool
}

func buildHowToReadReport(mode benchmarkMode) []string {
	modeIncludes, modeExcludes := describeBenchmarkMode(mode)
	return []string{
		"The throughput tab is a scheduler microbenchmark. Use it to compare scheduler overhead growth, not to claim end-to-end app behavior.",
		"The fairness tab is the customer-facing view. Focus on first completion, full completion, and whether non-whales finish ahead of whales.",
		fmt.Sprintf("%s mode includes: %s.", mode, modeIncludes),
		fmt.Sprintf("%s mode excludes: %s.", mode, modeExcludes),
	}
}

func describeBenchmarkMode(mode benchmarkMode) (string, string) {
	switch mode {
	case benchmarkModeApp:
		return "notifier enqueue, in-memory queue claim, scheduler handoff, worker execution, and synthetic delivery work",
			"PostgreSQL queue claiming, notifier HTTP ingest, real outbound webhook network cost, and failure-path retry or DLQ behavior"
	default:
		return "the round-robin scheduler plus a synthetic worker harness that drains scheduled jobs",
			"queue claiming, notifier app flow, HTTP ingest, and real delivery behavior"
	}
}

func earlyCompletionWindowExplanation(window int) string {
	return fmt.Sprintf("The early window uses the first %d completed jobs. That is large enough to capture both non-whale customers finishing their two messages while still reflecting the first wave of shared progress before whale volume dominates the totals.", window)
}

func appModeConfidenceSummary() []string {
	return []string{
		"App mode is an in-memory notifier-flow confidence step, not a full deployment benchmark.",
		"At low worker counts, first completions can lag scheduler mode because the queue poller claims batches before the scheduler redistributes work.",
		"The next stronger evidence step is a PostgreSQL-backed app benchmark path that includes real queue claim order and poll behavior.",
		"Before making stronger horizontal-scale claims, we still need matching PostgreSQL-backed fairness runs plus at least one full end-to-end deployment measurement with real HTTP delivery cost.",
	}
}

func analyzeFairnessRun(customerSummaries []customerFairnessSummary) fairnessRunInsights {
	whaleCount := 0
	nonWhaleCount := 0
	var whaleFirstTotal time.Duration
	var nonWhaleFirstTotal time.Duration
	var whaleFullTotal time.Duration
	var nonWhaleFullTotal time.Duration
	earliestWhaleFullCompletion := time.Duration(1<<63 - 1)
	latestNonWhaleFullCompletion := time.Duration(0)

	for _, customerSummary := range customerSummaries {
		if customerSummary.customerSegment == "whale" {
			whaleCount++
			whaleFirstTotal += customerSummary.firstFinishDuration
			whaleFullTotal += customerSummary.finishDuration
			if customerSummary.finishDuration < earliestWhaleFullCompletion {
				earliestWhaleFullCompletion = customerSummary.finishDuration
			}
			continue
		}

		nonWhaleCount++
		nonWhaleFirstTotal += customerSummary.firstFinishDuration
		nonWhaleFullTotal += customerSummary.finishDuration
		if customerSummary.finishDuration > latestNonWhaleFullCompletion {
			latestNonWhaleFullCompletion = customerSummary.finishDuration
		}
	}

	insights := fairnessRunInsights{}
	if whaleCount > 0 {
		insights.averageWhaleFirstCompletion = whaleFirstTotal / time.Duration(whaleCount)
		insights.averageWhaleFullCompletion = whaleFullTotal / time.Duration(whaleCount)
	}
	if nonWhaleCount > 0 {
		insights.averageNonWhaleFirstCompletion = nonWhaleFirstTotal / time.Duration(nonWhaleCount)
		insights.averageNonWhaleFullCompletion = nonWhaleFullTotal / time.Duration(nonWhaleCount)
	}
	if whaleCount > 0 && nonWhaleCount > 0 {
		insights.whaleVsNonWhaleCompletionGap = insights.averageWhaleFullCompletion - insights.averageNonWhaleFullCompletion
		insights.nonWhalesFinishedBeforeWhales = latestNonWhaleFullCompletion < earliestWhaleFullCompletion
	}

	return insights
}

func buildFairnessConclusion(mode benchmarkMode, scenarioName string, runSummary fairnessWorkerRunSummary) string {
	insights := analyzeFairnessRun(runSummary.customerSummaries)
	result := "non-whales did not fully finish before whales"
	if insights.nonWhalesFinishedBeforeWhales {
		result = "non-whales fully finished before the first whale finished"
	}

	return fmt.Sprintf(
		"%s in %s mode at %d workers: average non-whale full completion %s, average whale full completion %s, completion gap %s, and %s.",
		scenarioName,
		mode,
		runSummary.workerCount,
		insights.averageNonWhaleFullCompletion.Round(time.Millisecond),
		insights.averageWhaleFullCompletion.Round(time.Millisecond),
		insights.whaleVsNonWhaleCompletionGap.Round(time.Millisecond),
		result,
	)
}

func fairnessConclusionBlock(mode benchmarkMode, scenarioSummary fairnessScenarioSummary) []string {
	lines := make([]string, 0, len(scenarioSummary.workerRunSummaries))
	for _, runSummary := range scenarioSummary.workerRunSummaries {
		lines = append(lines, buildFairnessConclusion(mode, scenarioSummary.name, runSummary))
	}
	return lines
}

func formatDurationGap(duration time.Duration) string {
	if duration == 0 {
		return "0s"
	}
	return duration.Round(time.Millisecond).String()
}
