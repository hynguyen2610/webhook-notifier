# Next Change Steps

## Goal

- [ ] Reach `100%` unit test coverage for the codebase, or document any truly unreachable lines that must be excluded intentionally
- [ ] Bound in-memory scheduled queue growth so polling cannot outrun workers indefinitely

## Coverage Baseline

- [x] Keep a current coverage snapshot while working from this baseline recorded on `2026-08-06`
- [ ] `internal/retry`: `100.0%`
- [ ] `internal/scheduler`: `89.5%`
- [ ] `internal/workqueue`: `66.7%`
- [ ] `internal/registration`: `60.5%`
- [ ] `internal/mockreceiver`: `48.6%`
- [ ] `internal/mockgenerator`: `43.7%`
- [ ] `internal/notifier`: `30.5%`
- [ ] `cmd/scheduler-benchmark`: `7.2%`
- [x] `internal/config`: `100.0%`
- [x] `internal/delivery`: `100.0%`
- [x] `internal/httpx`: `100.0%`
- [x] `internal/logging`: `100.0%`
- [x] `internal/metrics`: `100.0%`
- [ ] `internal/testsupport`: `0.0%`
- [ ] `internal/events`: no direct test files yet
- [ ] `cmd/mock-event-generator`: `0.0%`
- [ ] `cmd/mock-webhook-receiver`: `0.0%`
- [ ] `cmd/notifier`: `0.0%`

## Scope Decision

- [x] Decide what counts toward the `100%` unit coverage target:
- [x] include only notifier core `internal/...` packages for now
- [x] keep `cmd/...` entrypoints out of the current `100%` target
- [x] keep mock apps and benchmark entrypoints as-is for now
- [ ] decide whether `internal/testsupport` should be unit-tested directly or excluded from the target
- [x] decide whether simple model-only packages such as `internal/events` need explicit tests or can be covered indirectly

## Low-Coverage Priority Packages

- [x] Add unit tests for `internal/config`
- [x] Add unit tests for `internal/delivery`
- [x] Add unit tests for `internal/httpx`
- [x] Add unit tests for `internal/logging`
- [x] Add unit tests for `internal/metrics`
- [ ] Add direct unit tests for `internal/notifier` helper, runtime, and benchmark-support logic not yet covered well enough
- [ ] Add direct unit tests for `internal/mockgenerator` uncovered branches
- [ ] Add direct unit tests for `internal/mockreceiver` uncovered branches

## Medium-Coverage Packages

- [x] Raise `internal/registration` from partial to complete unit coverage
- [x] Raise `internal/workqueue` from partial to complete unit coverage
- [x] Raise `internal/scheduler` from `89.5%` to `100%`

## Entry Point Coverage

- [x] Decide whether to add unit-style tests for `cmd/mock-event-generator`
- [x] Decide whether to add unit-style tests for `cmd/mock-webhook-receiver`
- [x] Decide whether to add unit-style tests for `cmd/notifier`
- [x] Decide whether `cmd/scheduler-benchmark` should be driven to `100%` or treated separately from the notifier MVP target

## Branch And Error Path Coverage

- [ ] Cover validation failures
- [x] Cover configuration parse failures
- [x] Cover HTTP transport failures and timeout mapping
- [ ] Cover queue repository error branches where unit isolation is practical
- [ ] Cover notifier handler and worker error branches not already proven by integration tests
- [x] Cover metrics and JSON response helper branches
- [ ] Cover graceful shutdown and startup error paths where unit testing is feasible

## Bounded Polling Queue

- [ ] Add a scheduled queue limit so the poller does not keep enqueueing work when workers fall behind
- [ ] Use `queueSize = workerCount * 10` as the bounded in-memory scheduled queue target
- [ ] Add configuration wiring for the scheduled queue limit instead of leaving the multiplier as a magic number
- [ ] Make the poller skip or delay new claims when `scheduler.QueueDepth()` is already at or above the limit
- [ ] Keep claim behavior fair across customers after adding the queue bound
- [ ] Expose the bounded queue behavior clearly in logs or metrics so overload is visible during local runs
- [ ] Add unit tests that prove the poller stops claiming when scheduled queue depth reaches the limit
- [ ] Add unit or integration coverage that proves polling resumes once workers drain enough scheduled work
- [ ] Document the default queue bound and the reasoning in `README.md` or `how-to-run.md`

## Test Quality Rules

- [ ] Keep tests unit-focused rather than relying on integration environments when a stub or fake can prove the branch
- [ ] Add short test case descriptions with expected input and expected outcome
- [ ] Prefer descriptive variable names and readable table-driven tests
- [ ] Avoid inflating coverage with low-signal assertions that do not validate behavior

## Tracking And Reporting

- [ ] Update this checklist package by package as coverage improves
- [ ] Record any lines or branches that cannot be covered reasonably and explain why
- [ ] If exclusions become necessary, document them explicitly instead of silently relaxing the `100%` goal

## Verification

- [x] Run `go test ./...`
- [x] Run `go test ./... -cover`
- [ ] Confirm every package in scope reaches `100%` unit coverage or has documented justified exclusions
