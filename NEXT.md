# Next Change Steps

## Goal

- [ ] Implement the assignment load test from `docs/spec/load-test.md` as a simple single-instance PostgreSQL-backed path

## Guardrails

- [ ] Preserve the existing multi-instance benchmark command and behavior unchanged
- [ ] Add the assignment load test as an explicit mode, wrapper, or clearly named command
- [ ] Keep the implementation simple enough for local assignment use without extra load-test infrastructure

## Test Suites

- [ ] Add a smoke load test dataset: `customer-a=100`, `customer-b=20`, `customer-c=20`
- [ ] Add a fairness load test dataset: `whale=100000`, `small-b=100`, `small-c=100`
- [ ] Keep both suites on one notifier instance with `20` workers

## Metrics To Report

- [ ] Queue depth from PostgreSQL queue snapshots during the run
- [ ] End-to-end delivery latency computed as `completed_at - created_at`
- [ ] Retry count computed from queue retry state
- [ ] Oldest pending event age during the run

## Implementation Steps

- [ ] Reuse the existing PostgreSQL-backed benchmark script helpers for database setup, receiver startup, queue preload, notifier startup, and report generation
- [ ] Add a single-instance assignment load test path that does not run the `1/2/4` instance comparison loop
- [ ] Add a markdown report format focused only on the four assignment metrics and the validation outcome
- [ ] Document clearly in the report that queue rows are preloaded directly into PostgreSQL and that HTTP ingest benchmarking is out of scope

## Validation Checks

- [ ] Smoke test: queue depth returns to zero, retry count stays zero, oldest pending age stays bounded, all events are delivered
- [ ] Fairness test: queue drains fully, retry count stays zero, oldest pending age does not continuously increase, small-customer events complete while whale backlog is still being processed
- [ ] End-to-end delivery latency remains stable enough to support the assignment narrative in both suites

## Verification

- [ ] Run the smoke load test locally and confirm it finishes in under `10` seconds
- [ ] Run the fairness load test locally and confirm it finishes within the expected `30–60` second target if practical on the local machine
- [ ] Confirm the generated report path is printed and the metric values are believable
