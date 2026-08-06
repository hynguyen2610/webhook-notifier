# Next Change Steps

## Goal

- [ ] Reach `100%` unit test coverage for the codebase, or document any truly unreachable lines that must be excluded intentionally

## Coverage Baseline

- [ ] Keep a current coverage snapshot while working from this baseline recorded on `2026-08-06`
- [ ] `internal/retry`: `100.0%`
- [ ] `internal/scheduler`: `89.5%`
- [ ] `internal/workqueue`: `66.7%`
- [ ] `internal/registration`: `60.5%`
- [ ] `internal/mockreceiver`: `48.6%`
- [ ] `internal/mockgenerator`: `43.7%`
- [ ] `internal/notifier`: `30.0%`
- [ ] `cmd/scheduler-benchmark`: `7.2%`
- [ ] `internal/config`: `0.0%`
- [ ] `internal/delivery`: `0.0%`
- [ ] `internal/httpx`: `0.0%`
- [ ] `internal/logging`: `0.0%`
- [ ] `internal/metrics`: `0.0%`
- [ ] `internal/testsupport`: `0.0%`
- [ ] `internal/events`: no direct test files yet
- [ ] `cmd/mock-event-generator`: `0.0%`
- [ ] `cmd/mock-webhook-receiver`: `0.0%`
- [ ] `cmd/notifier`: `0.0%`

## Scope Decision

- [ ] Decide what counts toward the `100%` unit coverage target:
- [ ] include only `internal/...` packages
- [ ] include `cmd/...` entrypoints too
- [ ] decide whether `internal/testsupport` should be unit-tested directly or excluded from the target
- [ ] decide whether simple model-only packages such as `internal/events` need explicit tests or can be covered indirectly

## Low-Coverage Priority Packages

- [ ] Add unit tests for `internal/config`
- [ ] Add unit tests for `internal/delivery`
- [ ] Add unit tests for `internal/httpx`
- [ ] Add unit tests for `internal/logging`
- [ ] Add unit tests for `internal/metrics`
- [ ] Add direct unit tests for `internal/notifier` helper and handler logic not yet covered well enough
- [ ] Add direct unit tests for `internal/mockgenerator` uncovered branches
- [ ] Add direct unit tests for `internal/mockreceiver` uncovered branches

## Medium-Coverage Packages

- [ ] Raise `internal/registration` from partial to complete unit coverage
- [ ] Raise `internal/workqueue` from partial to complete unit coverage
- [ ] Raise `internal/scheduler` from `89.5%` to `100%`

## Entry Point Coverage

- [ ] Decide whether to add unit-style tests for `cmd/mock-event-generator`
- [ ] Decide whether to add unit-style tests for `cmd/mock-webhook-receiver`
- [ ] Decide whether to add unit-style tests for `cmd/notifier`
- [ ] Decide whether `cmd/scheduler-benchmark` should be driven to `100%` or treated separately from the notifier MVP target

## Branch And Error Path Coverage

- [ ] Cover validation failures
- [ ] Cover configuration parse failures
- [ ] Cover HTTP transport failures and timeout mapping
- [ ] Cover queue repository error branches where unit isolation is practical
- [ ] Cover notifier handler and worker error branches not already proven by integration tests
- [ ] Cover metrics and JSON response helper branches
- [ ] Cover graceful shutdown and startup error paths where unit testing is feasible

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

- [ ] Run `go test ./...`
- [ ] Run `go test ./... -cover`
- [ ] Confirm every package in scope reaches `100%` unit coverage or has documented justified exclusions
