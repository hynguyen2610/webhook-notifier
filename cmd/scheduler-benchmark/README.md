# Scheduler Benchmark

This tool runs scheduler benchmarks plus explicit worker fairness scenarios and records the results both on screen and in a timestamped HTML report file.

## Important Scope Note

The numbers produced by `go run ./cmd/scheduler-benchmark ...` are in-memory benchmark numbers only. They are useful for scheduler cost and fairness shape, but they are not PostgreSQL-backed queue measurements.

## Run

From the repository root:

```bash
go run ./cmd/scheduler-benchmark
```

Opt in to app-level fairness runs:

```bash
go run ./cmd/scheduler-benchmark --mode app
```

Skip the `200000`-message whale scenario for a faster smoke run:

```bash
go run ./cmd/scheduler-benchmark --include-large-fairness-case=false
```

Environment-variable fallback:

- `SCHEDULER_BENCHMARK_MODE=scheduler|app`
- `SCHEDULER_BENCHMARK_SKIP_LARGE_FAIRNESS_CASE=true|false`

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

Mode-specific scope:

- `scheduler` mode: scheduler plus the benchmark's synthetic worker harness
- `app` mode: notifier enqueue, in-memory registry lookup, in-memory queue polling, real notifier worker flow, and synthetic delivery work

Out of scope in `app` mode:

- PostgreSQL queue behavior
- notifier HTTP server startup and ingest endpoints
- real outbound HTTP webhook delivery
- retries and DLQ behavior under failure

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
- `two-whales-200000-two-non-whales-1000`: `200000`, `200000`, `1000`, and `1000` queued messages across four customers

Each fairness scenario runs with worker counts `1`, `4`, and `8`.

## Output

The tool does two things on each run:

1. prints an aligned text table to standard output
2. writes a self-contained HTML report with separate **Throughput Benchmark** and **Fairness Benchmark** tabs to `loadtest/reports/`

Report files use this naming pattern:

```text
loadtest/reports/scheduler-benchmark-scheduler-YYYYMMDD-HHMMSS.html
loadtest/reports/scheduler-benchmark-app-YYYYMMDD-HHMMSS.html
```

The `loadtest/reports/` directory is ignored by git.

## Interpretation And Runtime Notes

Keep one HTML report per run, but use the tabs differently:

- **Throughput Benchmark**: scheduler-only microbenchmark evidence
- **Fairness Benchmark**: customer-facing progress evidence

The report now calls out what each mode includes and excludes so reviewers do not mistake synthetic harness results for full app-level scale proof.

Measured smoke-run examples from `2026-08-03` with `--include-large-fairness-case=false`:

- `scheduler` mode: about `13s`
- `app` mode: about `7s`

The large fairness cases, including `two-whales-200000-two-normals-2` and `two-whales-200000-two-non-whales-1000`, remain opt-in and should be treated as slower full runs because they add much heavier scenarios on top of the smoke scenario.

Example report paths generated on `2026-08-03`:

```text
loadtest/reports/scheduler-benchmark-scheduler-20260803-130540.html
loadtest/reports/scheduler-benchmark-app-20260803-130553.html
```
