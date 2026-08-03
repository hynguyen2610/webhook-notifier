package main

import "testing"

func TestFairnessScenarioDefinitionsSmokePathStaysSmall(t *testing.T) {
	// Input: benchmark options with large fairness scenarios disabled.
	// Outcome: only the fast smoke fairness scenario is included.
	definitions := fairnessScenarioDefinitions(benchmarkOptions{
		mode:                 benchmarkModeScheduler,
		includeLargeScenario: false,
	})

	if len(definitions) != 1 {
		t.Fatalf("expected 1 smoke definition, got %d", len(definitions))
	}
	if definitions[0].name != "two-whales-100-two-normals-2" {
		t.Fatalf("expected smoke scenario %q, got %q", "two-whales-100-two-normals-2", definitions[0].name)
	}
}

func TestFairnessScenarioDefinitionsIncludeNewLargeScenario(t *testing.T) {
	// Input: benchmark options with large fairness scenarios enabled.
	// Outcome: both large opt-in fairness scenarios are included, including the new medium non-whale case.
	definitions := fairnessScenarioDefinitions(benchmarkOptions{
		mode:                 benchmarkModeScheduler,
		includeLargeScenario: true,
	})

	expectedNames := []string{
		"two-whales-100-two-normals-2",
		"two-whales-200000-two-normals-2",
		"two-whales-200000-two-non-whales-1000",
	}
	if len(definitions) != len(expectedNames) {
		t.Fatalf("expected %d definitions, got %d", len(expectedNames), len(definitions))
	}
	for definitionIndex, expectedName := range expectedNames {
		if definitions[definitionIndex].name != expectedName {
			t.Fatalf("definition %d: expected %q, got %q", definitionIndex, expectedName, definitions[definitionIndex].name)
		}
	}

	newScenario := definitions[2]
	if newScenario.eventsPerCustomer["customer-c"] != 1000 || newScenario.eventsPerCustomer["customer-d"] != 1000 {
		t.Fatalf("expected non-whale customers to have 1000 messages, got %d and %d", newScenario.eventsPerCustomer["customer-c"], newScenario.eventsPerCustomer["customer-d"])
	}
	if newScenario.syntheticWorkIterations != 64 {
		t.Fatalf("expected new large scenario to reuse synthetic work iterations 64, got %d", newScenario.syntheticWorkIterations)
	}
}
