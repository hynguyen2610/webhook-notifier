package main

import (
	"testing"
	"time"
)

func TestAnalyzeFairnessRunReportsNonWhalesFinishFirst(t *testing.T) {
	// Input: two whale customers that finish later than two non-whale customers.
	// Outcome: the derived gap is positive and the report marks non-whales as finishing first.
	insights := analyzeFairnessRun([]customerFairnessSummary{
		{customerSegment: "whale", customerID: "customer-a", firstFinishDuration: 80 * time.Millisecond, finishDuration: 900 * time.Millisecond},
		{customerSegment: "whale", customerID: "customer-b", firstFinishDuration: 90 * time.Millisecond, finishDuration: 1100 * time.Millisecond},
		{customerSegment: "non-whale", customerID: "customer-c", firstFinishDuration: 20 * time.Millisecond, finishDuration: 120 * time.Millisecond},
		{customerSegment: "non-whale", customerID: "customer-d", firstFinishDuration: 25 * time.Millisecond, finishDuration: 150 * time.Millisecond},
	})

	if !insights.nonWhalesFinishedBeforeWhales {
		t.Fatalf("expected non-whales to finish before whales")
	}
	if insights.whaleVsNonWhaleCompletionGap <= 0 {
		t.Fatalf("expected positive completion gap, got %s", insights.whaleVsNonWhaleCompletionGap)
	}
}

func TestAnalyzeFairnessRunReportsOverlapWhenWhalesFinishEarly(t *testing.T) {
	// Input: one whale customer fully finishes before a non-whale customer.
	// Outcome: the derived result marks that non-whales did not finish before whales.
	insights := analyzeFairnessRun([]customerFairnessSummary{
		{customerSegment: "whale", customerID: "customer-a", firstFinishDuration: 30 * time.Millisecond, finishDuration: 100 * time.Millisecond},
		{customerSegment: "whale", customerID: "customer-b", firstFinishDuration: 35 * time.Millisecond, finishDuration: 140 * time.Millisecond},
		{customerSegment: "non-whale", customerID: "customer-c", firstFinishDuration: 20 * time.Millisecond, finishDuration: 130 * time.Millisecond},
		{customerSegment: "non-whale", customerID: "customer-d", firstFinishDuration: 22 * time.Millisecond, finishDuration: 160 * time.Millisecond},
	})

	if insights.nonWhalesFinishedBeforeWhales {
		t.Fatalf("expected whales to overlap or finish before a non-whale")
	}
}
