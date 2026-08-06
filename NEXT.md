# Next Change Steps

## Goal

- [ ] Split delivery execution and retry/dead-letter handling out of `runWorker` and into separate notifier files so each responsibility is easier to read and test

## Refactor Target

- [ ] Keep runtime behavior unchanged for success, retry, and dead-letter flows
- [ ] Make worker control flow easier to scan by moving branch-specific logic into named helper functions
- [ ] Move extracted worker helper functions into separate files instead of keeping all delivery outcome branches in `worker.go`
- [ ] Keep the split inside the current notifier process instead of introducing a new service or background retry component
- [ ] Preserve the current PostgreSQL queue-driven retry model where failed jobs become available again through `available_at`

## Implementation Steps

- [ ] Extract a function that performs one delivery attempt and returns the delivery result
- [ ] Extract a function that handles successful delivery queue updates and metrics
- [ ] Extract a function that handles retryable failure decisions, backoff calculation, and queue updates
- [ ] Extract a function that handles permanent failure and dead-letter recording
- [ ] Place the extracted worker helpers in one or more dedicated notifier files with names that match their responsibility
- [ ] Keep helper names descriptive and aligned with repository naming guidance
- [ ] Avoid changing public behavior, retry thresholds, or queue state transitions during the refactor

## Tests

- [ ] Keep the existing notifier retry and integration tests passing without changing their behavioral assertions
- [ ] Add or update focused unit coverage only if the refactor introduces new branch logic that is hard to verify through existing tests

## Verification

- [ ] Run notifier tests that cover success, retry, and dead-letter paths
- [ ] Confirm the worker still marks successful jobs as delivered
- [ ] Confirm retryable failures still update `available_at` and increment retry state as before
- [ ] Confirm exhausted failures still land in the dead-letter path as before
