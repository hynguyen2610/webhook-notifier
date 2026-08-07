# ADR 0004: Improve Benchmark Report Clarity And App-Level Confidence

- Status: Accepted
- Date: 2026-08-03
- Deciders: Project implementation team

## Context

The project now has two benchmark modes:

- `scheduler` mode, which measures scheduler behavior with a synthetic worker harness
- `app` mode, which exercises the notifier enqueue, poll, and worker flow with
  in-memory test doubles and synthetic delivery work

Those modes are useful, but the latest reports exposed several customer-facing
gaps:

- scheduler microbenchmark results and fairness results are mixed into the same
  report without a strong conclusion layer
- fairness metrics are not yet easy to interpret from a product perspective
- the meaning of `early share` is unclear without more context
- app mode still does not represent full end-to-end production behavior because
  it excludes PostgreSQL queue behavior, notifier HTTP ingest, and real webhook
  network cost
- the latest reports raise legitimate questions about whether app-mode fairness
  differs from scheduler-mode fairness because of queue claim order or worker
  behavior

For this assignment, the benchmark suite should do more than emit numbers. It
should help a reviewer or customer quickly answer:

1. what was actually tested
2. how fairness should be interpreted
3. whether horizontal scale claims are supported by evidence
4. what confidence level the current results deserve

## Decision

The benchmark and load-test suite will be improved in four ways:

1. **Clarify benchmark scope in every report**
   Each report must state what the selected mode includes, what it excludes, and
   what conclusions the reader should or should not draw from it.

2. **Use progress-oriented fairness metrics**
   Fairness reporting will prioritize customer-facing progress signals such as:
   - first completion time
   - full completion time
   - share of an early completion window
   - derived comparisons showing whether normal customers finish materially
     earlier than whales

3. **Separate throughput evidence from fairness evidence more clearly**
   Scheduler throughput metrics should remain available, but fairness and
   application-level conclusions should be presented in their own explicit tab or
   report outcome so readers do not confuse microbenchmark results with
   fairness behavior.

4. **Increase confidence in app-mode results incrementally**
   The current in-memory app mode is an intermediate confidence step, not the
   final proof point. Follow-up work should add stronger end-to-end coverage,
   beginning with a single-instance PostgreSQL-backed integration load test that
   keeps runtime manageable while validating report metrics against real queue
   behavior, and only then expanding to broader multi-instance comparisons when
   practical.

## Decision Drivers

- make benchmark results understandable to reviewers without verbal explanation
- reduce ambiguity about what fairness metrics actually mean
- avoid overstating confidence from synthetic or partially integrated tests
- preserve a fast default benchmark while still allowing deeper opt-in runs
- keep the next confidence step small enough to debug metric behavior quickly
- create a path from microbenchmark evidence to stronger app-level evidence

## Consequences

### Positive

- reports become easier to read and defend
- benchmark conclusions become more honest and more actionable
- fairness regressions become easier to spot
- customers and reviewers can distinguish between scheduler-only and app-level
  confidence
- future end-to-end benchmark work has a documented direction

### Trade-Offs

- reporting logic becomes more opinionated and more complex
- some metrics may be removed or de-emphasized even if they are technically
  correct but confusing
- stronger app-level confidence will require more setup, runtime, and possibly
  PostgreSQL-backed benchmark infrastructure
- a staged approach means some metric validation will happen in single-instance
  integration runs before the same signals are trusted in multi-instance reports

## Implementation Guidance

The improvement work should proceed in small steps:

1. add a concise “how to read this report” explanation to benchmark output
2. refine fairness metrics and labels to emphasize customer progress
3. add derived comparisons such as whale vs non-whale finish gap
4. make throughput and fairness evidence easier to compare without mixing their
   meaning, including separate tabs in the HTML report
5. add stronger opt-in app-mode coverage, beginning with current in-memory flow
   and then a single-instance PostgreSQL-backed integration load test before
   broader multi-instance runs
6. document known confidence limits in every report mode

## Out Of Scope For This ADR

This ADR does not define:

- the exact SQL-backed app-mode benchmark implementation
- the exact charting library or visual style of the report
- pass/fail thresholds for fairness or throughput
- CI policy for when heavy benchmark scenarios should run

Those details should be handled in follow-up checklist work and implementation.

## Follow-Up Direction

The next benchmark-focused change should prefer a single-instance integration
load test over a broader scale-out pass when the goal is to validate newly
added report metrics such as oldest pending event age. That narrower path gives
faster feedback, reduces silent runtime during debugging, and makes it easier to
separate metric correctness problems from horizontal-scaling behavior.

## Related Files

- [README.md](../README.md)
- [cmd/scheduler-benchmark/README.md](../../cmd/scheduler-benchmark/README.md)
- [app-load-test-opt-in-checklist.md](../plan/app-load-test-opt-in-checklist.md)
- [multi-instance-benchmark-checklist.md](../plan/multi-instance-benchmark-checklist.md)
- [scheduler-benchmark-checklist.md](../plan/scheduler-benchmark-checklist.md)
- [internal/notifier/app.go](../../internal/notifier/app.go)
- [cmd/scheduler-benchmark/main.go](../../cmd/scheduler-benchmark/main.go)
