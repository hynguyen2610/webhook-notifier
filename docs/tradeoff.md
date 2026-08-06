# Tradeoffs

This document records conscious implementation tradeoffs, deferred production features, and simplifications that kept the project focused on the assignment goals.

## Project Scope

**Chosen:** Focused implementation instead of production-ready platform

### Benefits
- Keeps development focused on the core requirements: fairness, retries, durability, testing, and documentation.
- Allows deeper validation of important system behaviors instead of spreading effort across operational features.

### Trade-offs
- Does not include production concerns such as authentication, high availability deployment, dashboards, or full deployment automation.
- Additional work would be required before operating this system in a production environment.

---

## Queue Infrastructure

**Chosen:** PostgreSQL-backed work queue instead of Kafka-based event transport

### Benefits
- Simpler local setup and testing environment.
- Delivery state, retry attempts, and dead-letter status can be stored together with queue data.
- Easier debugging because queue state is directly queryable.
- Provides sufficient durability, work claiming, and retry support for the expected workload of this assignment.

### Trade-offs
- Kafka provides better horizontal scalability and is more suitable for very high-throughput event pipelines.
- A database queue introduces database load as throughput increases.
- For significantly larger workloads, a dedicated messaging system may become more appropriate.

---

## Validation Strategy

**Chosen:** Comprehensive automated testing instead of manual verification

### Benefits
- Provides repeatable validation of fairness, retry behavior, concurrency handling, and failure scenarios.
- Makes behavioral requirements easier to demonstrate.
- Helps prevent regressions during implementation changes.

### Trade-offs
- Requires additional development time to build and maintain test scenarios.
- Some complex production scenarios still require operational validation.

---

## Continuous Integration

**Chosen:** Automated GitHub Actions workflow instead of local-only testing

### Benefits
- Ensures tests run consistently in a clean environment.
- Detects regressions before changes are merged.
- Provides confidence that the project remains buildable.

### Trade-offs
- Adds CI configuration and maintenance overhead.
- Execution time increases as more integration and load tests are added.

---

## Queue Capacity Limits

**Chosen:** No per-customer pending-job limit

### Benefits
- Keeps queue management simple.
- Allows PostgreSQL to provide durable buffering without introducing additional admission policies.
- Avoids making assumptions about customer-specific workload limits.

### Trade-offs
- A single customer can accumulate a large backlog.
- Production systems may require quotas or rate limits to prevent resource exhaustion.

---

## Load Test Data Path

**Chosen:** Preloaded PostgreSQL delivery rows instead of benchmarking the complete ingestion pipeline

### Benefits
- Keeps benchmarks focused on scheduling, claiming, worker execution, and delivery behavior.
- Produces more deterministic measurements.
- Avoids mixing ingestion performance with delivery pipeline performance.

### Trade-offs
- Does not measure the performance of the full end-to-end event ingestion flow.
- Additional testing would be required to evaluate the complete pipeline.

---

## Queue Polling Strategy

**Chosen:** Incremental polling with bounded batch size instead of large batch claims

### Benefits
- Allows work from different customers to enter the scheduler more frequently.
- Reduces the chance that one customer's backlog dominates in-memory queues.
- Improves responsiveness for smaller customers.

### Trade-offs
- More frequent database queries may increase database overhead.
- Larger batch sizes could improve database efficiency for extremely high-throughput workloads.

---

## Scheduled Queue Backpressure

**Chosen:** Bounded in-memory scheduled queue limit instead of unlimited scheduler buffering

### Benefits
- Prevents the poller from continuing to pull PostgreSQL work indefinitely when workers are already behind.
- Reduces the risk of unbounded in-memory growth inside a notifier instance.
- Keeps PostgreSQL as the durable backlog when local worker capacity is temporarily saturated.
- Makes overload behavior easier to reason about because the queue bound is tied to worker capacity through `workerCount * 10`.

### Trade-offs
- If the scheduled queue is already full, newly claimable PostgreSQL rows wait longer before entering the in-process scheduler.
- Throughput can drop in some workloads if the queue limit is too conservative for the available CPU, network, or downstream webhook capacity.
- Adds another tuning knob, `NOTIFIER_SCHEDULED_QUEUE_LIMIT_FACTOR`, that may need adjustment as workloads change.
- A fixed queue bound is a simple backpressure mechanism, but it is less adaptive than dynamic claim sizing or feedback based on real worker utilization.

---

## Observability Scope

**Chosen:** Focused benchmark and load-test metrics instead of production monitoring stack

### Benefits
- Provides enough visibility to validate assignment goals.
- Keeps implementation focused on measurable system behavior.
- Avoids building operational infrastructure unrelated to the assignment.

### Trade-offs
- Does not provide complete production monitoring capabilities.
- Additional metrics, dashboards, and alerting would be required for production operation.

---

## Benchmark Deployment Model

**Chosen:** Single-instance and horizontal-scaling benchmarks instead of full distributed platform benchmarking

### Benefits
- Produces deterministic and explainable measurements.
- Focuses on scheduler behavior, throughput, and fairness.
- Minimizes unrelated deployment variables.

### Trade-offs
- Does not represent every production deployment scenario.
- Additional testing would be needed for infrastructure-level concerns such as network latency, multi-region deployment, and failover behavior.

---

## How to Read This File

Unlike **decision-evolution.md**, which explains how the architecture evolved, this document explains the benefits gained and limitations accepted for each implementation choice.
