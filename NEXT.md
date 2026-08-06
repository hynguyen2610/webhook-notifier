# Next Change Steps

## Goal

- [x] Split delivery execution and retry/dead-letter handling out of `runWorker` and into separate notifier files so each responsibility is easier to read and test

## Refactor Target

- [x] Keep runtime behavior unchanged for success, retry, and dead-letter flows
- [x] Make worker control flow easier to scan by moving branch-specific logic into named helper functions
- [x] Move extracted worker helper functions into separate files instead of keeping all delivery outcome branches in `worker.go`
- [x] Keep the split inside the current notifier process instead of introducing a new service or background retry component
- [x] Preserve the current PostgreSQL queue-driven retry model where failed jobs become available again through `available_at`

## Implementation Steps

- [x] Extract a function that performs one delivery attempt and returns the delivery result
- [x] Extract a function that handles successful delivery queue updates and metrics
- [x] Extract a function that handles retryable failure decisions, backoff calculation, and queue updates
- [x] Extract a function that handles permanent failure and dead-letter recording
- [x] Place the extracted worker helpers in one or more dedicated notifier files with names that match their responsibility
- [x] Keep helper names descriptive and aligned with repository naming guidance
- [x] Avoid changing public behavior, retry thresholds, or queue state transitions during the refactor

## Tests

- [x] Keep the existing notifier retry and integration tests passing without changing their behavioral assertions
- [x] Add or update focused unit coverage only if the refactor introduces new branch logic that is hard to verify through existing tests

## Verification

- [x] Run notifier tests that cover success, retry, and dead-letter paths
- [x] Confirm the worker still marks successful jobs as delivered
- [x] Confirm retryable failures still update `available_at` and increment retry state as before
- [x] Confirm exhausted failures still land in the dead-letter path as before
