# Scheduler Benchmark Checklist

## Goal

- [x] Capture scheduler `ns/op`
- [x] Capture scheduler `allocs/op`
- [x] Capture scheduler `bytes/op`
- [x] Print benchmark results to the terminal
- [x] Save an HTML benchmark report under `loadtest/reports/`

## Workloads

- [x] Single-customer burst workload
- [x] Balanced multi-customer workload
- [x] Whale scenario workload with smaller customers mixed in
- [x] Huge mixed workload to show scaling behavior at larger data sizes

## Validation

- [x] Confirm the benchmark command runs locally
- [x] Confirm the report file is created with a timestamped name
- [x] Confirm `loadtest/reports/` is ignored by git
- [x] Confirm each report includes a readable table and charts
