package retry

import (
	"testing"
	"time"
)

func TestExponentialBackoffPolicyNextDelay(t *testing.T) {
	// Input: retry attempts from -1 through 3 with an initial delay of 1 second.
	// Outcome: delay starts at 1 second and doubles for each retry attempt above zero.
	backoffPolicy := ExponentialBackoffPolicy{
		InitialDelay:    time.Second,
		MaxRetryAttempt: 3,
	}

	testCases := []struct {
		name          string
		attempt       int
		expectedDelay time.Duration
	}{
		{name: "negative attempt uses initial delay", attempt: -1, expectedDelay: time.Second},
		{name: "zero attempt uses initial delay", attempt: 0, expectedDelay: time.Second},
		{name: "first retry doubles delay", attempt: 1, expectedDelay: 2 * time.Second},
		{name: "second retry quadruples delay", attempt: 2, expectedDelay: 4 * time.Second},
		{name: "third retry keeps exponential growth", attempt: 3, expectedDelay: 8 * time.Second},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			actualDelay := backoffPolicy.NextDelay(testCase.attempt)
			if actualDelay != testCase.expectedDelay {
				t.Fatalf("expected delay %v, got %v", testCase.expectedDelay, actualDelay)
			}
		})
	}
}

func TestExponentialBackoffPolicyCanRetry(t *testing.T) {
	// Input: retry attempts from 0 through 4 with MaxRetryAttempt set to 3.
	// Outcome: attempts below 3 can retry, while attempts 3 and above cannot retry.
	backoffPolicy := ExponentialBackoffPolicy{
		InitialDelay:    time.Second,
		MaxRetryAttempt: 3,
	}

	testCases := []struct {
		attempt     int
		shouldRetry bool
	}{
		{attempt: 0, shouldRetry: true},
		{attempt: 1, shouldRetry: true},
		{attempt: 2, shouldRetry: true},
		{attempt: 3, shouldRetry: false},
		{attempt: 4, shouldRetry: false},
	}

	for _, testCase := range testCases {
		actual := backoffPolicy.CanRetry(testCase.attempt)
		if actual != testCase.shouldRetry {
			t.Fatalf("attempt %d: expected %t, got %t", testCase.attempt, testCase.shouldRetry, actual)
		}
	}
}
