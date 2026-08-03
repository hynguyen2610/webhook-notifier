# Scheduler Benchmark Checklist

## Goal

- [x] Capture scheduler `ns/op`
- [x] Capture scheduler `allocs/op`
- [x] Capture scheduler `bytes/op`
- [x] Capture fairness scenario total `jobs/sec`
- [x] Capture fairness scenario progress metrics for non-whale versus whale customers
- [x] Print benchmark results to the terminal
- [x] Save an HTML benchmark report under `loadtest/reports/`

## Workloads

- [x] Single-customer burst workload
- [x] Balanced multi-customer workload
- [x] Whale scenario workload with smaller customers mixed in
- [x] Huge mixed workload to show scaling behavior at larger data sizes
- [x] Fairness case with two whales at `100` messages and two normal customers at `2` messages
- [x] Fairness case with two whales at `200000` messages and two normal customers at `2` messages

## Validation

- [x] Confirm the benchmark command runs locally
- [x] Confirm the report file is created with a timestamped name
- [x] Confirm `loadtest/reports/` is ignored by git
- [x] Confirm each report includes a readable table and charts
- [x] Confirm each fairness case includes total throughput comparisons across `1`, `4`, and `8` workers
- [x] Confirm each fairness case includes per-customer first completion, finish completion, and early-share comparisons
