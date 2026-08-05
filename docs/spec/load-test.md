# Load Test Specification

## Objective

Validate that the Webhook Notifier satisfies the assignment requirements by demonstrating:

* Correct event delivery through the PostgreSQL-backed processing path.
* Fair scheduling across customers.
* Correct retry behavior.
* No customer starvation.

The tests intentionally focus on a small set of metrics that directly validate these requirements rather than general production performance. The load test should remain simple enough to run locally during assignment work without introducing a separate load-test platform.

This specification defines an additional single-instance load test path. It should not require removing or silently changing the existing multi-instance benchmark flow.

---

# Scope

## In Scope

* Single notifier instance
* PostgreSQL queue
* Fair scheduler
* Worker pool
* Mock webhook endpoints
* Retry mechanism
* Simple report-oriented metric collection from the existing benchmark path

## Out of Scope

The following are intentionally excluded from this assignment:

* Multi-instance notifier testing
* Horizontal scaling validation
* HTTP ingest-path benchmarking
* CPU and memory benchmarking
* Worker utilization analysis
* Adaptive concurrency limits
* Circuit breaker validation
* Long-duration soak testing

The existing multi-instance benchmark may remain in the repository for separate exploratory or comparison work, but it is not the primary single-instance load test defined here.

---

# Metrics

Only the following four metrics are collected.

For this single-instance load test, each metric should be derived directly from PostgreSQL queue state so the calculation remains easy to inspect and explain in the report.

---

## 1. Queue Depth

### Purpose

Measure backlog size and verify that work continuously progresses.

### Definition

```text
Queue Depth = Number of events currently waiting for delivery.
```

For the simple single-instance load test, queue depth may be collected from direct PostgreSQL queue snapshots during the run rather than a separate time-series system.

Suggested calculation

```text
Queue Depth = COUNT(rows WHERE status IN ('pending', 'claimed'))
```

### Expected Behaviour

* Increases when events are produced.
* Decreases steadily while workers process events.
* Eventually reaches zero.

---

## 2. End-to-End Delivery Latency

### Purpose

Measure how long an event takes to be delivered after entering the system.

### Definition

```text
End-to-End Delivery Latency =
delivery_completed_at - event_created_at
```

For this single-instance load test, `event_created_at` refers to the time the benchmark creates the queue row for delivery work. Unlike HTTP request duration, this includes:

* Queue waiting time
* Scheduler delay
* Worker acquisition
* HTTP delivery
* Retry delays (if applicable)

This metric reflects the time an event spends travelling through the notifier processing path after it has entered the PostgreSQL-backed queue.

Suggested calculation

```text
End-to-End Delivery Latency = completed_at - created_at
```

### Expected Behaviour

* Remains reasonably stable during normal operation.
* Small customer events continue completing promptly even when a whale customer generates heavy traffic.

---

## 3. Retry Count

### Purpose

Validate retry behaviour.

### Definition

```text
Retry Count =
Total number of retry attempts performed during the test.
```

### Expected Behaviour

Normal scenario

* Zero retries.

Failure scenario

* Retries occur as expected.
* Retry count remains bounded.
* Events are eventually delivered.

Suggested calculation

```text
Retry Count = SUM(retry_count)
```

---

## 4. Oldest Pending Event Age

### Purpose

Detect starvation.

### Definition

```text
Oldest Pending Event Age =
Current Time - Created Time of the oldest unfinished event
```

This metric represents how long the oldest event has remained incomplete.

For the simple single-instance load test, unfinished events may include both `pending` and currently `claimed` rows so the metric continues to reflect in-flight work rather than dropping to zero immediately after claiming.

Suggested calculation

```text
Oldest Pending Event Age =
current_time - MIN(created_at WHERE status IN ('pending', 'claimed'))
```

### Expected Behaviour

* May increase while the queue builds.
* Stabilizes or decreases as processing continues.
* Must not continuously increase throughout the test.

A continuously increasing value indicates that some events are not making progress.

---

# Test Suite 1 — Smoke Load Test

## Purpose

Quick regression test executed during development.

Expected runtime:

* Less than 10 seconds.

---

## Environment

```text
PostgreSQL Queue
    │
Notifier
20 Workers
    │
Mock Webhook Endpoints
```

Queue rows may be inserted directly by the load test harness instead of being sent through the notifier HTTP ingest API. This keeps the single-instance load test simple and focuses the measurement on queue claiming, scheduling, worker execution, retry handling, and delivery completion.

This simple load test path should be implemented as an explicit mode, wrapper, or clearly named command so the repository's existing multi-instance benchmark behavior is not changed accidentally.

---

## Dataset

| Customer   | Events |
| ---------- | -----: |
| Customer A |    100 |
| Customer B |     20 |
| Customer C |     20 |

All webhook endpoints return HTTP 200.

---

## Validation

Verify:

* Queue depth returns to zero.
* End-to-end delivery latency remains stable.
* Retry count is zero.
* Oldest pending event age remains bounded.
* All events are delivered successfully.

---

# Test Suite 2 — Fairness Load Test

## Purpose

Demonstrate that a whale customer does not starve smaller customers.

Expected runtime:

30–60 seconds.

---

## Environment

```text
PostgreSQL Queue
    │
Fair Scheduler
    │
20 Worker Pool
    │
Mock Webhook Endpoints
```

Queue rows may be inserted directly by the load test harness instead of being sent through the notifier HTTP ingest API so the fairness test stays focused on the PostgreSQL-backed delivery path.

This fairness test should also preserve any existing multi-instance benchmark command as a separate path rather than replacing it.

---

## Dataset

| Customer         |  Events |
| ---------------- | ------: |
| Whale Customer   | 100,000 |
| Small Customer B |     100 |
| Small Customer C |     100 |

All webhook endpoints return HTTP 200.

---

## Validation

### Queue Depth

Verify:

* Queue decreases steadily.
* Queue fully drains before the test completes.

---

### End-to-End Delivery Latency

Observe overall delivery latency.

Expected:

* Small customer events complete while the whale customer still has a large backlog.
* Delivery latency remains stable without excessive growth.

---

### Retry Count

Expected:

* Zero retries during normal execution.

If failure injection is enabled:

* Retries occur as expected.
* Events are eventually delivered.

---

### Oldest Pending Event Age

Observe the metric during the entire test.

Expected:

* Does not continuously increase.
* Remains bounded.
* Returns toward zero as the queue drains.

---

# Success Criteria

The notifier is considered successful when:

* Every event is eventually delivered.
* Queue depth drains to zero.
* End-to-end delivery latency remains stable.
* Retry behaviour matches expectations.
* Oldest pending event age remains bounded.
* Small customer events continue completing while whale events are still being processed.

These criteria are intended to validate assignment behaviour, not production-scale readiness.

Preserving the existing multi-instance benchmark as a separate command or mode is considered part of keeping this single-instance load test low-risk and easy to reason about.

---

# Metrics Summary

| Metric                      | Smoke Test | Fairness Test |
| --------------------------- | :--------: | :-----------: |
| Queue Depth                 |      ✓     |       ✓       |
| End-to-End Delivery Latency |      ✓     |       ✓       |
| Retry Count                 |      ✓     |       ✓       |
| Oldest Pending Event Age    |      ✓     |       ✓       |

---

# Rationale

The selected metrics directly validate the assignment requirements.

* **Queue Depth** verifies that the backlog is continuously drained.
* **End-to-End Delivery Latency** measures the complete time an event spends inside the PostgreSQL-backed notifier path, from queue row creation until successful delivery.
* **Retry Count** verifies failure handling and retry correctness.
* **Oldest Pending Event Age** detects starvation by ensuring unfinished events continue making progress.

Additional infrastructure metrics such as CPU usage, memory consumption, goroutine count, worker utilization, Prometheus scraping complexity, and latency percentiles are intentionally excluded because they are more relevant to production capacity planning than to demonstrating the fairness and delivery guarantees required by this assignment.
