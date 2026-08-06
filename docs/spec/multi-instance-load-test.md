# Multi-Instance Load Test Specification

## Objective

Validate the horizontal-scaling claim for the Webhook Notifier by demonstrating:

* throughput increases as notifier instance count increases
* fairness does not degrade materially as notifier instance count increases

This load test is intentionally narrow. It is designed to compare queue-processing behavior across multiple notifier instances without introducing extra moving parts that would make the result harder to interpret.

The default run should stay quick enough for normal local use. A successful default comparison should finish in under `1` minute on a typical local development machine, but that runtime target is a workload-sizing goal for the default preset rather than an early-stop rule for the benchmark itself.

---

# Scope

## In Scope

* prequeued PostgreSQL delivery rows
* one shared PostgreSQL queue
* one mock webhook receiver
* multiple notifier processes against the same queue
* throughput comparison across `1`, `2`, and `4` notifier instances
* fairness comparison across the same instance counts
* simple markdown reporting

## Out of Scope

The following are intentionally excluded:

* notifier HTTP ingest-path benchmarking
* retry and dead-letter validation
* Prometheus scraping requirements
* CPU and memory benchmarking
* production infrastructure claims
* Kubernetes or distributed deployment validation

This benchmark should remain a simple local comparison of processing-path behavior.

---

# Benchmark Model

The benchmark starts after webhook registrations and delivery rows are already present in PostgreSQL.

```text
Prequeued PostgreSQL Delivery Rows
    │
Shared PostgreSQL Queue
    │
1 / 2 / 4 Notifier Instances
    │
Local Mock Webhook Receiver
```

Using direct prequeueing is intentional. It keeps the benchmark focused on:

* PostgreSQL claim order
* scheduler handoff
* worker execution
* local webhook delivery completion

It does not attempt to prove notifier ingest performance.

---

# Workload

## Default Comparison Preset

The default multi-instance comparison should use a workload that is only large enough to show scaling differences while still finishing comfortably in under `1` minute on a typical local machine.

Suggested default shape:

| Customer   | Events |
| ---------- | -----: |
| Customer A |   3500 |
| Customer B |   3500 |
| Customer C |    100 |
| Customer D |    100 |

The exact counts may be tuned, but the default preset should prefer the smallest dataset that still shows:

* measurable throughput improvement from `1` to `2` to `4` instances
* visible fairness behavior for small customers versus larger customers

The benchmark should always complete all configured instance-count measurements. Reducing workload size is the preferred way to keep runtime practical; stopping the benchmark before all configured measurements finish is not.

## Optional Presets

The benchmark may also expose:

* a lighter smoke preset for script sanity checks
* a heavier optional preset for deeper local comparison runs

Those presets are secondary to the main `1` / `2` / `4` comparison.

---

# Metrics

The benchmark should collect only the metrics needed to support the scaling claim.

## 1. Total Duration

Purpose

Measure total wall-clock time to drain the prequeued workload for each instance count.

Definition

```text
Total Duration =
last_completed_at - benchmark_started_at
```

Expected behaviour

* total duration decreases as instance count increases

## 2. Jobs Per Second

Purpose

Compare processing throughput across instance counts.

Definition

```text
Jobs Per Second =
total_completed_jobs / total_duration
```

Expected behaviour

* throughput increases from `1` to `2` to `4` instances

## 3. Per-Customer Completion Timing

Purpose

Check whether smaller customers still make visible progress as instance count changes.

Definition

* first completion time per customer
* finish completion time per customer

Expected behaviour

* small-customer completion timing should not regress materially when moving from fewer to more instances

## 4. Early Completion Share

Purpose

Detect whether early progress becomes more skewed when scaling out.

Definition

```text
Early Completion Share =
customer_completed_jobs_in_first_N / N
```

Expected behaviour

* horizontal scaling should not materially worsen early-progress skew for small customers

## 5. Oldest Pending Event Age

Purpose

Detect whether unfinished work becomes less healthy under horizontal scaling.

Definition

```text
Oldest Pending Event Age =
current_time - created_at of oldest pending row
```

Expected behaviour

* the value remains bounded during the run
* the value should not worsen materially as instance count increases

---

# Validation Logic

The benchmark is successful only if both of the following hold:

## Throughput Validation

* `2` instances complete the same workload faster than `1`
* `4` instances complete the same workload faster than `2`

## Fairness Validation

* small-customer first completion and finish completion do not regress materially as instance count increases
* early completion share for small customers does not collapse further as instance count increases
* oldest pending event age does not show worse starvation behavior at higher instance counts

If throughput improves but fairness worsens, the benchmark should report that horizontal scaling improved throughput but did not preserve fairness.

---

# Runtime And Exit Behavior

The benchmark tool should:

* print clear phase updates while it is running
* write report progress as each instance-count section becomes available
* finish the default comparison in under `1` minute when practical on a normal local machine
* complete every configured instance-count run before exiting
* exit cleanly after the final measurement is recorded

Graceful shutdown matters because the benchmark result should feel complete as soon as the measurements are complete. The tool should not appear stuck after writing the report merely because notifier processes are still being drained in the background without visibility.

If notifier shutdown takes longer than expected, the tool should still make that phase visible and keep exit behavior bounded.

---

# Reporting

The report should state clearly:

* the benchmark uses prequeued PostgreSQL rows
* the comparison is across `1`, `2`, and `4` notifier instances
* what throughput changed
* what fairness changed
* whether the horizontal-scaling claim is supported or contradicted by the observed result

The report should favor a short markdown summary over a large observability dashboard.

---

# Rationale

This benchmark exists to answer one specific question:

```text
Does adding notifier instances increase throughput without degrading fairness?
```

Everything outside that question should stay out of the benchmark unless it materially improves the credibility of the answer.
