package retry

import "time"

type ExponentialBackoffPolicy struct {
	InitialDelay    time.Duration
	MaxRetryAttempt int
}

func (policy ExponentialBackoffPolicy) NextDelay(attempt int) time.Duration {
	if attempt <= 0 {
		return policy.InitialDelay
	}

	delay := policy.InitialDelay
	for retryAttempt := 1; retryAttempt <= attempt; retryAttempt++ {
		delay *= 2
	}

	return delay
}

func (policy ExponentialBackoffPolicy) CanRetry(attempt int) bool {
	return attempt < policy.MaxRetryAttempt
}
