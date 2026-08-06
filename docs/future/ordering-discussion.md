# Design Discussion: Event Ordering

## Purpose

This document summarizes how event ordering is handled in the current implementation, why the chosen approach aligns with the assignment requirements, and how the design could evolve if stronger ordering guarantees were required.

---

# Assignment Requirement

The assignment owner clarified that:

> **Out-of-order delivery is acceptable.**

This significantly influences the architecture.

Instead of optimizing for strict ordering, the implementation prioritizes:

- Fairness across customers
- Horizontal scalability
- Low delivery latency
- High throughput
- Simplicity

---

# Current Implementation

## Architecture

```
PostgreSQL Queue
        │
        ▼
Round Robin Scheduler
        │
        ▼
Worker Pool
```

The scheduler maintains fairness by selecting pending deliveries in a round-robin fashion across customers.

Workers process scheduled jobs concurrently.

---

# Does the system preserve ordering?

## Global Ordering

❌ No.

Multiple workers execute concurrently, therefore deliveries can complete in different orders.

Example

```
Customer A

Event 1
Event 2
```

Worker execution

```
Worker 1 → Event 2

Worker 2 → Event 1
```

Result

```
Event 2 delivered first
```

This behavior is acceptable because strict ordering is not required.

---

## Per-Customer Ordering

Best effort only.

The scheduler attempts to distribute work fairly across customers, but concurrent execution and retries may reorder deliveries.

---

## Retry Ordering

Retries may also introduce reordering.

Example

```
Event 1
↓

HTTP 500

↓

Retry after 30 seconds

Meanwhile

Event 2

↓

HTTP 200
```

Result

```
Event 2

↓

Event 1
```

This is an expected consequence of at-least-once delivery.

---

# Why choose this design?

Prioritizing throughput and fairness provides better overall system utilization.

Strict ordering typically reduces concurrency because work must wait for previous events to finish.

Current implementation favors:

- Fairness
- Throughput
- Simplicity

over

- Strict ordering

---

# If strict ordering became a requirement

Several approaches are possible.

## Option 1 — Single Worker per Customer

```
Customer Queue

↓

Dedicated Worker
```

Pros

- Preserves ordering

Cons

- Poor utilization
- Difficult to scale
- Idle workers

---

## Option 2 — Customer Affinity

Hash customers to workers.

```
hash(customer_id)

↓

Worker
```

Pros

- Ordering preserved per customer
- Better utilization

Cons

- Hot customers create hot workers
- Rebalancing is difficult

---

## Option 3 — Kafka Partitioning

Use customer_id as the Kafka partition key.

```
hash(customer_id)

↓

Kafka Partition

↓

Consumer
```

Pros

- Ordering guaranteed within a partition
- Horizontal scaling

Cons

- Hot partitions
- Fairness becomes partition-local
- One whale customer may saturate a partition

---

# Fairness vs Ordering

These goals are related but different.

| Property | Current Implementation | Strict Ordering |
|----------|------------------------|-----------------|
| Fairness | ✅ | Depends |
| Throughput | High | Lower |
| Parallelism | High | Lower |
| Latency | Low | Higher |
| Ordering | Best effort | Guaranteed |

---

# Kafka Considerations

Kafka naturally distributes work using partitions.

```
Producer

↓

Kafka

↓

Consumer Group
```

Ordering is guaranteed only **within a partition**.

If partitioned by customer:

```
Customer A

↓

Partition 5
```

all Customer A events remain ordered.

However:

- one large customer may create a hot partition,
- fairness is no longer global,
- scheduling decisions become local to each partition.

Application-level scheduling may still be required if fairness is a stronger requirement than ordering.

---

# Why PostgreSQL Queue was chosen

For this MVP, PostgreSQL provides several advantages.

- Simpler architecture
- Easier debugging
- Easier testing
- Global visibility of pending deliveries
- Scheduler can implement fairness across all customers

The expected workload does not justify the operational complexity of Kafka.

Kafka would become attractive when queue throughput or database write capacity becomes the primary scalability bottleneck.

---

# Future Evolution

As throughput requirements grow, the architecture could evolve incrementally.

```
Current PostgreSQL Queue

↓

Multiple Worker Pods

↓

Leader Election

↓

Worker Autoscaling

↓

Database Sharding

↓

Kafka Queue
```

The scheduler would continue to enforce fairness unless requirements changed to prioritize throughput over global scheduling.

---

# Conclusion

The implementation intentionally does **not** guarantee strict ordering because:

- the assignment explicitly allows out-of-order delivery,
- concurrent processing significantly improves throughput,
- fairness across customers is considered more valuable than preserving event order.

If future requirements changed, ordering could be strengthened using customer affinity or Kafka partitioning, with the trade-off of reduced scheduling flexibility and lower parallelism.