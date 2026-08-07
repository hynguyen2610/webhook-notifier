# ADR 0003: Replace Kafka with a PostgreSQL-Backed Work Queue

- **Status:** Accepted
- **Date:** 2026-08-03
- **Deciders:** Project implementation team

---

# Context

The initial implementation used Kafka to transport subscriber events from the mock event producer to the webhook notifier.

Kafka is a proven platform for high-throughput event streaming and distributed messaging. However, this assignment focuses on building a reliable webhook notifier rather than a general-purpose event streaming platform.

The core functional requirements are:

- reliable webhook delivery
- at-least-once delivery semantics
- retries with exponential backoff
- dead-letter handling
- fair scheduling across customers
- horizontal worker execution
- persistence of webhook registrations

PostgreSQL is already required to store webhook registrations. It also provides durable storage, transactions, and row-level locking, making it capable of acting as a reliable work queue for the expected workload.

Because this project is a proof of concept, introducing Kafka adds operational complexity without providing significant benefits for the required functionality.

---

# Decision

Kafka will no longer be used as the primary transport layer.

Subscriber events are persisted directly into a PostgreSQL-backed work queue.

The processing pipeline becomes:

1. subscriber events are inserted into the work queue
2. notifier instances poll and claim pending deliveries
3. claimed deliveries are passed to the Round Robin scheduler
4. the scheduler dispatches jobs fairly across customers
5. workers deliver webhooks
6. retries and terminal failures are persisted back into PostgreSQL
7. dead-lettered deliveries remain available for inspection in the database

The scheduler, retry logic, and worker pool remain unchanged. Only the mechanism used to obtain work changes from Kafka consumption to PostgreSQL queue polling.

---

# Decision Drivers

The decision was based on the following considerations:

- Customer-level fairness is a primary requirement of the assignment.
- PostgreSQL allows the notifier to claim batches of pending deliveries and apply scheduling before dispatching work to workers.
- Reliable work claiming can be implemented using transactions and row-level locking (`FOR UPDATE SKIP LOCKED`).
- PostgreSQL is already required for webhook registrations, allowing the existing datastore to be reused.
- Queue state, retries, and dead-letter records become immediately inspectable through SQL.
- The assignment evaluates webhook delivery behaviour rather than distributed event streaming infrastructure.
- Removing Kafka reduces infrastructure while preserving the required functionality.

---

# Why PostgreSQL Was Chosen

This notifier is fundamentally a **durable work queue**, not an event streaming platform.

Its primary responsibilities are:

- reliably storing pending deliveries
- safely distributing work across multiple notifier instances
- retrying failed deliveries
- recording dead-lettered deliveries
- scheduling work fairly across customers

Customer-level fairness is one of the most important requirements of the assignment.

With a PostgreSQL-backed queue, notifier instances can claim batches of pending deliveries, organize them by customer, and apply the existing Round Robin scheduler before dispatching work to the worker pool.

This gives the application full control over scheduling decisions instead of coupling fairness to broker partitioning or message ordering.

PostgreSQL also provides the transactional guarantees required for reliable work claiming while reusing the same datastore that already stores webhook registrations.

Kafka is an excellent choice when systems require:

- very high ingestion throughput
- multiple independent consumers
- event replay
- long-term event retention
- cross-service event streaming

Those capabilities become valuable at larger scale but are outside the scope of this proof-of-concept assignment.

---

# Consequences

## Positive

- Removes an infrastructure dependency.
- Simplifies local development and deployment.
- Simplifies CI and integration testing.
- Queue state can be inspected directly using SQL.
- Retries and dead-letter records remain durable in the database.
- Customer-level fairness is implemented entirely within the notifier instead of depending on broker partitioning.
- The architecture becomes easier to explain and review.

## Trade-Offs

Compared with Kafka, this approach has several limitations:

- PostgreSQL becomes the primary bottleneck as throughput grows.
- Queue polling generates additional database load.
- Horizontal scaling depends on efficient row claiming and indexing instead of Kafka partitions.
- Event replay capabilities are more limited.
- Supporting multiple independent consumers is less flexible than using an event streaming platform.

These trade-offs are acceptable because the assignment prioritizes reliable webhook delivery, customer fairness, and simplicity over maximum streaming throughput.

---

# Future Evolution

The notifier is intentionally structured so that queue storage is separated from scheduling and delivery.

If future requirements include:

- significantly higher throughput
- multiple downstream consumers
- event replay
- cross-service event streaming
- large-scale distributed deployments

the ingestion layer can be migrated to Kafka without requiring fundamental changes to the scheduler, retry logic, or worker pool.

Kafka would then become responsible for durable event transport, while the notifier would continue to focus on fair scheduling and reliable webhook delivery.

---

# Implementation Guidance

The migration consists of the following steps:

1. Persist subscriber events into a PostgreSQL work queue.
2. Poll and claim pending deliveries using transactional row locking (`FOR UPDATE SKIP LOCKED`).
3. Feed claimed deliveries into the existing scheduler.
4. Persist acknowledgements, retries, and dead-letter state.
5. Update the mock event producer to enqueue into PostgreSQL.
6. Remove Kafka-specific startup scripts, documentation, and integration tests.
7. Replace Kafka integration tests with PostgreSQL queue integration tests.

---

# Out of Scope

This ADR does not define:

- the queue table schema
- polling interval tuning
- retry scheduling policy
- queue indexing strategy
- producer implementation
- database performance tuning

Those implementation details are documented separately or may be addressed by future ADRs.

---

# Related Files

- `README.md`
- `docs/plan/implementation-checklist.md`
- `internal/workqueue/`
- `internal/notifier/`
- `internal/registration/`