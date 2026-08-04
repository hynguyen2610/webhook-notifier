# Multi-Instance Benchmark Checklist

## Feasibility

- [x] Confirm the current runtime can support multiple notifier instances against one PostgreSQL queue
- [x] Confirm the queue claim path is designed for safe concurrent claims across instances
- [x] Confirm the current in-memory benchmark is not sufficient proof for multi-instance fairness
- [x] Add deployment-level benchmark evidence for multiple running notifier instances

## Current Reality

- [x] PostgreSQL queue claiming already uses `FOR UPDATE SKIP LOCKED`
- [x] The notifier already supports configurable worker count, poll interval, batch size, and HTTP address
- [x] Different notifier processes can run in parallel if they use different `NOTIFIER_HTTP_ADDRESS` values
- [x] Existing `scheduler-benchmark` commands only prove scheduler or single-process app behavior
- [x] Existing benchmark reports explicitly exclude PostgreSQL queue behavior and real multi-instance execution

## Goal

- [x] Measure throughput as notifier instance count increases
- [x] Measure fairness as notifier instance count increases
- [ ] Separate worker-scaling findings from instance-scaling findings
- [x] Document what the benchmark proves and what it still does not prove

## Success Criteria

- [x] Run the same workload with `1`, `2`, and `4` notifier instances
- [x] Keep the same total workload definition across all instance-count runs
- [x] Record total jobs completed, total duration, and jobs per second for each run
- [x] Record whether small customers still complete early when whale customers dominate the queue
- [x] Record whether adding instances improves throughput without causing obvious fairness regression

## Workload Design

- [ ] Define a smoke workload that is fast enough for day-to-day runs
- [x] Define a heavier workload that is large enough to expose cross-instance queue-claim behavior
- [x] Include at least one whale-versus-non-whale scenario
- [x] Keep customer mix deterministic so runs are comparable
- [x] Decide whether retries should be disabled for the first benchmark pass to reduce noise

## Suggested Scenarios

- [ ] Smoke scenario: `customer-a=100`, `customer-b=100`, `customer-c=2`, `customer-d=2`
- [x] Medium scenario: `customer-a=5000`, `customer-b=5000`, `customer-c=100`, `customer-d=100`
- [ ] Heavy fairness scenario: `customer-a=200000`, `customer-b=200000`, `customer-c=1000`, `customer-d=1000`
- [x] Decide whether each scenario should run with fixed per-instance worker count or fixed total worker budget
Current run used a fixed per-instance worker count of `4` so the benchmark reflects real scale-out of both pollers and workers.

## Control Variables

- [x] Fix PostgreSQL configuration for all comparison runs
- [x] Fix mock receiver behavior for all comparison runs
- [x] Fix notifier `NOTIFIER_WORKER_COUNT` during instance-count comparisons
- [x] Fix notifier `NOTIFIER_QUEUE_CLAIM_BATCH_SIZE` during instance-count comparisons
- [x] Fix notifier `NOTIFIER_QUEUE_POLL_INTERVAL` during instance-count comparisons
- [ ] Fix machine, Docker, and background-load conditions as much as practical

## Local Runtime Setup

- [x] Start PostgreSQL with the local Docker Compose stack
- [x] Start one mock receiver instance
- [ ] Start one mock generator instance or use repeatable ingest requests directly
- [x] Start notifier instances with `NOTIFIER_HTTP_ADDRESS=:0` so multiple local processes can run without port collisions
- [x] Verify each notifier instance connects to the same PostgreSQL database
- [x] Verify each notifier instance logs a unique `claimOwner`

## Benchmark Execution

- [x] Clear or recreate benchmark data between runs
- [x] Preload webhook registrations for all benchmark customers
- [x] Preload the full workload directly into PostgreSQL before measuring fairness completion timing
- [x] Run the benchmark with `1` notifier instance
- [x] Run the benchmark with `2` notifier instances
- [x] Run the benchmark with `4` notifier instances
- [ ] Repeat each run enough times to reduce one-off noise
- [x] Save raw notes for every run, including exact env vars and commands

## Metrics To Capture

- [x] Total jobs
- [x] Total duration
- [x] Jobs per second
- [x] Per-customer first completion time
- [x] Per-customer finish time
- [x] Share of the first `20` completed jobs by customer
- [ ] Whale-versus-non-whale finish gap
- [ ] Queue depth trend if available
- [ ] Retry count and dead-letter count if retries are enabled

## Fairness Checks

- [ ] Confirm non-whale customers still appear in early completions
Observed on `2026-08-03`: they did not. `customer-a` owned the first `20` completions in the `1`, `2`, and `4` instance runs.
- [ ] Confirm non-whale customers can fully finish before whales in the heavy fairness scenario
- [x] Check whether increasing instance count changes fairness shape materially
Observed on `2026-08-03`: throughput improved with more instances, but the early-completion fairness shape stayed poor because PostgreSQL claim order remained customer-skewed.
- [ ] Check whether larger claim batches let one instance monopolize early work
- [ ] Check whether smaller poll intervals improve throughput but worsen fairness

## Risks And Interpretation

- [x] Call out that receiver HTTP capacity can become the bottleneck instead of the notifier
- [x] Call out that PostgreSQL may become the bottleneck before scheduler fairness does
- [x] Call out that local-machine results are useful evidence, not production-proof
- [x] Avoid comparing instance-count runs against the existing in-memory benchmark as if they measured the same thing
- [x] Note whether fairness changes are caused by queue claiming, scheduling, worker execution, or receiver saturation

## Follow-Up Automation

- [x] Add a script that launches `N` notifier instances without local port collisions
- [x] Add a script that seeds registrations and benchmark workloads repeatably
- [x] Add a script that collects per-run output into a timestamped report directory
- [ ] Decide whether to create an HTML report for multi-instance runs similar to `scheduler-benchmark`

## Exit Criteria

- [x] Produce one markdown summary of findings for `1`, `2`, and `4` instances
- [x] Include exact commands, env vars, and report or note file paths
- [x] State clearly whether multi-instance scaling improved throughput
- [x] State clearly whether fairness held, improved, or regressed
- [x] List the main benchmark limitations and recommended next experiments
