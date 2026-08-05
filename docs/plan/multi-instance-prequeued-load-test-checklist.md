# Multi-Instance Prequeued Load Test Checklist

## Goal

- [x] Keep the multi-instance load test focused on PostgreSQL-backed queue claiming, scheduling, worker execution, and delivery completion
- [x] Preserve direct prequeueing of webhook delivery rows as the default event entry path for the multi-instance benchmark
- [x] Keep the default local runtime closer to about `30` seconds without losing the multi-instance comparison signal

## Rationale

- [x] Make it explicit that HTTP ingest benchmarking is out of scope for the multi-instance load test
- [x] Keep the benchmark simple enough to reason about when comparing `1`, `2`, and `4` notifier instances
- [x] Avoid introducing a second moving part such as generator ingest when the benchmark goal is queue-processing comparison

## Scope

- [x] Benchmark starts after queue rows are already present in PostgreSQL
- [x] Benchmark includes one mock receiver and multiple notifier instances
- [x] Benchmark continues to exclude notifier ingest API cost
- [x] Benchmark continues to exclude production-scale infrastructure claims

## Implementation Checks

- [x] Keep webhook registrations seeded directly before each run
- [x] Keep queue rows preloaded directly into `webhook_delivery_queue` before notifier startup
- [x] Keep the report text explicit that the workload is prequeued
- [x] Keep instance-count comparison behavior unchanged while simplifying the benchmark intent
- [x] Add a lighter default preset while preserving the older heavier workload as an explicit opt-in preset

## Metrics

- [ ] Continue reporting total jobs, total duration, jobs per second, and oldest pending event age
- [ ] Continue reporting per-customer completion timing so fairness effects remain visible
- [ ] Decide whether queue-depth trend reporting should be added without complicating the benchmark too much

## Validation

- [ ] Confirm the benchmark still produces comparable `1`, `2`, and `4` instance runs
- [ ] Confirm readers can tell from the report that the benchmark measures processing-path behavior, not ingest-path behavior
- [ ] Confirm the simplified benchmark remains sufficient for discussing multi-instance fairness and throughput tradeoffs

## Exit Ergonomics

- [x] Make notifier shutdown progress visible after measurements are complete
- [x] Keep post-measurement notifier shutdown bounded and explicit

## Documentation

- [x] Add a dedicated multi-instance load test specification with explicit throughput and fairness goals
