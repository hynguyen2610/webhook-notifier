# Scheduler Benchmark

This tool runs scheduler benchmarks plus explicit worker fairness scenarios and records the results both on screen and in a timestamped HTML report file.

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

The worker fairness scenarios record these metrics for each worker count:

- total `duration`: wall-clock time to finish the full scenario
- total `jobs/sec`: total delivered jobs divided by wall-clock time
- per-customer first completion duration: how quickly each customer starts making progress
- per-customer finish duration: how long each customer takes to fully finish
- early completion share: each customer's share of the first `20` completed jobs

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

The worker fairness scenarios are:

- `two-whales-100-two-normals-2`: `100`, `100`, `2`, and `2` queued messages across four customers
- `two-whales-200000-two-normals-2`: `200000`, `200000`, `2`, and `2` queued messages across four customers

Each fairness scenario runs with worker counts `1`, `4`, and `8`.

## Output

The tool does two things on each run:

1. prints an aligned text table to standard output
2. writes a self-contained HTML report with fairness comparison tables and charts to `loadtest/reports/`

Report files use this naming pattern:

```text
loadtest/reports/scheduler-benchmark-YYYYMMDD-HHMMSS.html
```

The `loadtest/reports/` directory is ignored by git.
