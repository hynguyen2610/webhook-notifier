# Next Change Steps

## Goal

- [x] Implement the assignment load test from `docs/spec/load-test.md` as a simple single-instance PostgreSQL-backed path

## Guardrails

- [x] Preserve the existing multi-instance benchmark command and behavior unchanged
- [x] Add the assignment load test as an explicit mode, wrapper, or clearly named command
- [x] Keep the implementation simple enough for local assignment use without extra load-test infrastructure

## Test Suites

- [x] Add a smoke load test dataset: `customer-a=100`, `customer-b=20`, `customer-c=20`
- [x] Add a fairness load test dataset: `whale=100000`, `small-b=100`, `small-c=100`
- [x] Keep both suites on one notifier instance with `20` workers

## Metrics To Report

- [x] Queue depth from PostgreSQL queue snapshots during the run
- [x] End-to-end delivery latency computed as `completed_at - created_at`
- [x] Retry count computed from queue retry state
- [x] Oldest pending event age during the run

## Implementation Steps

- [x] Reuse the existing PostgreSQL-backed benchmark script helpers for database setup, receiver startup, queue preload, notifier startup, and report generation
- [x] Add a single-instance assignment load test path that does not run the `1/2/4` instance comparison loop
- [x] Add a markdown report format focused only on the four assignment metrics and the validation outcome
- [x] Document clearly in the report that queue rows are preloaded directly into PostgreSQL and that HTTP ingest benchmarking is out of scope

## Validation Checks

- [x] Smoke test: queue depth returns to zero, retry count stays zero, oldest pending age stays bounded, all events are delivered
Smoke run completed on `2026-08-05` with report `loadtest/reports/assignment-load-test-smoke-20260805-032847.md`.
- [ ] Fairness test: queue drains fully, retry count stays zero, oldest pending age does not continuously increase, small-customer events complete while whale backlog is still being processed
- [ ] End-to-end delivery latency remains stable enough to support the assignment narrative in both suites

## Verification

- [x] Run the smoke load test locally and confirm it finishes in under `10` seconds
- [ ] Run the fairness load test locally and confirm it finishes within the expected `30–60` second target if practical on the local machine
- [x] Confirm the generated report path is printed and the metric values are believable
