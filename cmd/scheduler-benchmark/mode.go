package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

type benchmarkMode string

const (
	benchmarkModeScheduler benchmarkMode = "scheduler"
	benchmarkModeApp       benchmarkMode = "app"
)

type benchmarkOptions struct {
	mode                 benchmarkMode
	includeLargeScenario bool
}

func parseBenchmarkOptions() (benchmarkOptions, error) {
	defaultMode := strings.TrimSpace(os.Getenv("SCHEDULER_BENCHMARK_MODE"))
	if defaultMode == "" {
		defaultMode = string(benchmarkModeScheduler)
	}

	defaultIncludeLargeScenario := true
	if strings.EqualFold(strings.TrimSpace(os.Getenv("SCHEDULER_BENCHMARK_SKIP_LARGE_FAIRNESS_CASE")), "true") {
		defaultIncludeLargeScenario = false
	}

	modeFlag := flag.String("mode", defaultMode, "benchmark mode: scheduler or app")
	includeLargeScenarioFlag := flag.Bool("include-large-fairness-case", defaultIncludeLargeScenario, "include the large fairness scenarios, including the 200000-message whale cases")
	flag.Parse()

	selectedMode := benchmarkMode(strings.ToLower(strings.TrimSpace(*modeFlag)))
	switch selectedMode {
	case benchmarkModeScheduler, benchmarkModeApp:
	default:
		return benchmarkOptions{}, fmt.Errorf("invalid benchmark mode %q: expected scheduler or app", *modeFlag)
	}

	return benchmarkOptions{
		mode:                 selectedMode,
		includeLargeScenario: *includeLargeScenarioFlag,
	}, nil
}
