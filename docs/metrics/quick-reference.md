# Metrics Quick Reference

## Core Metrics

| Metric | What it answers | Definition | Healthy signal |
| --- | --- | --- | --- |
| Queue Depth | Is backlog draining? | Number of unfinished queue rows. For PostgreSQL-backed load tests this is typically `COUNT(status IN ('pending', 'claimed'))`. | Rises when work is added, then trends back to `0`. |
| Jobs Per Second | How much throughput are we getting? | `total completed jobs / total duration` | Improves when worker or instance count increases, unless another bottleneck dominates. |
| End-to-End Delivery Latency | How long does one event take after entering the queue? | `completed_at - created_at` | Stays reasonably stable and does not spike badly for small customers under mixed load. |
| Retry Count | Are failures retrying as expected? | `SUM(retry_count)` across the test workload | `0` in normal-success scenarios; bounded and explainable in failure scenarios. |
| Oldest Pending Event Age | Is anything being starved? | `current time - created_at of oldest unfinished event` | May rise briefly while backlog builds, but should stabilize or fall as work progresses. Continuous growth is a starvation warning. |
| First Completion Time Per Customer | Does each customer start making progress early? | Time from run start until that customer's first completed job | Small customers should not wait excessively behind whale customers. |
| Finish Completion Time Per Customer | How long until each customer fully finishes? | Time from run start until that customer's final completed job | Smaller customers should often finish materially earlier than whales in fairness-oriented scenarios. |
| Early Completion Share | Who gets the first visible progress? | `customer completed jobs in first N completions / N` | Small customers should still appear in the early completion window; one whale taking nearly all early completions is a fairness concern. |

## How To Interpret Fairness

In this repository, fairness is not defined as exact global ordering.

Fairness means:

- smaller customers still begin making progress while larger customers are active
- smaller customers are not indefinitely postponed behind whale customers
- early completions are not monopolized by one dominant customer
- starvation signals such as oldest pending age do not worsen continuously

The most important fairness metrics are:

- first completion time per customer
- finish completion time per customer
- early completion share
- oldest pending event age

## How To Interpret Starvation

Starvation means some jobs or some customers stop making meaningful progress while other work continues.

The main starvation checks are:

- oldest pending event age keeps rising without recovering
- a small customer has no early completions in mixed workloads
- first completion time for a small customer becomes disproportionately late
- finish completion time for a small customer collapses toward whale completion timing when the scheduler is expected to help

## Metric Sources By Test Path

### 1. Single-Instance Load Test Spec

Primary reference:

- [docs/spec/load-test.md](../spec/load-test.md)

Primary metrics:

- queue depth
- end-to-end delivery latency
- retry count
- oldest pending event age

This path is the clearest source for queue-health and starvation-oriented metrics.

### 2. Multi-Instance Benchmark

Primary reference:

- [docs/spec/multi-instance-load-test.md](../spec/multi-instance-load-test.md)

Primary metrics:

- total duration
- jobs per second
- first completion time per customer
- finish completion time per customer
- early completion share
- oldest pending event age

This path is the main evidence for the claim:

"adding notifier instances should improve throughput without materially degrading fairness"

### 3. Scheduler Benchmark

Primary reference:

- [cmd/scheduler-benchmark/README.md](../../cmd/scheduler-benchmark/README.md)

Primary metrics:

- `ns/op`
- `allocs/op`
- `bytes/op`
- synthetic `jobs/sec`
- first completion time per customer
- finish completion time per customer
- early completion share

Important limitation:

- this benchmark is useful for scheduler behavior and fairness shape
- it is not PostgreSQL-backed queue evidence

## Fast Answer For Reviewers

If you need the shortest possible answer, these are the headline metrics:

- throughput: `jobs per second`
- fairness: `first completion`, `finish completion`, and `early completion share` by customer
- starvation: `oldest pending event age`
- queue health: `queue depth`
- reliability under failure: `retry count`

## Related References

- [README.md](../../README.md)
- [docs/test-report/load-test-metrics-appendix.html](../test-report/load-test-metrics-appendix.html)
- [docs/adr/0004_improve_benchmark_report_clarity_and_app_level_confidence.md](../adr/0004_improve_benchmark_report_clarity_and_app_level_confidence.md)
- [docs/plan/worker-fairness-scenarios.md](../plan/worker-fairness-scenarios.md)
