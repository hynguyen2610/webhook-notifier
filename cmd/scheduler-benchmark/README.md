# Scheduler Benchmark

This tool runs a simple benchmark against the round-robin scheduler and records the results both on screen and in a timestamped HTML report file.

## Run

From the repository root:

```bash
go run ./cmd/scheduler-benchmark
```

## What It Measures

The benchmark records these metrics for each scenario:

- `ns/op`: average nanoseconds per benchmark iteration
- `allocs/op`: average heap allocations per benchmark iteration
- `bytes/op`: average allocated bytes per benchmark iteration
- `ops/sec`: derived throughput based on `ns/op`
- `jobs/sec`: derived job throughput based on `ops/sec * jobs per iteration`

One benchmark iteration includes:

1. creating a scheduler
2. enqueueing the scenario jobs
3. reading scheduled jobs until all jobs are drained
4. shutting down the scheduler

## Scenarios

The tool currently benchmarks these workloads:

- `single-customer-burst`: one customer with a large burst of queued jobs
- `balanced-three-customers`: three customers with evenly distributed jobs
- `whale-scenario`: one whale customer plus smaller customers mixed into the same workload
- `huge-mixed-customer-load`: a much larger mixed workload intended to make growth in `ns/op`, allocations, and bytes easier to compare across runs

## Output

The tool does two things on each run:

1. prints an aligned text table to standard output
2. writes a self-contained HTML report with a styled table and charts to `loadtest/reports/`

Report files use this naming pattern:

```text
loadtest/reports/scheduler-benchmark-YYYYMMDD-HHMMSS.md
loadtest/reports/scheduler-benchmark-YYYYMMDD-HHMMSS.html
```

The `loadtest/reports/` directory is ignored by git.
