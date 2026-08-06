# Ordering Strategies

The current webhook notifier **does not guarantee per-customer ordering**.

This decision is intentional. The assignment explicitly states that **out-of-order delivery is acceptable**, while also emphasizing **fairness across customers** and **high throughput**. Enforcing strict ordering would require serializing delivery for each customer, reducing concurrency and increasing latency for high-volume customers.

If ordering becomes a future requirement, the following strategies could be considered.

---

# Strategy Comparison

| Strategy | Approach | Guarantee | Pros | Trade-offs | Why Not Chosen |
|----------|----------|-----------|------|------------|----------------|
| **Per-Customer Lock** | Pessimistic | Strict ordering | ✅ Simple to retrofit into the existing worker pool<br>✅ Easy to reason about | ❌ Slow deliveries block later events for the same customer<br>❌ Lower throughput | Ordering is not currently required, so the loss of concurrency is unnecessary. |
| **Per-Customer Queue / Actor** | Pessimistic | Strict ordering | ✅ Clear ownership model<br>✅ No explicit locking<br>✅ Preserves ordering naturally | ❌ Additional queue lifecycle management | Good future evolution if ordering becomes mandatory, but introduces complexity not justified today. |
| **Kafka-style Partitioning** | Pessimistic | Ordering within a partition | ✅ Industry-proven<br>✅ Horizontally scalable<br>✅ Preserves ordering per partition | ❌ Requires Kafka and partition management<br>❌ Hot customers may become partition bottlenecks | Current scale does not justify Kafka's operational complexity. |
| **Sequence Numbers + Reorder Buffer** | Optimistic | Ordered output | ✅ Maximum parallelism<br>✅ Workers remain fully concurrent | ❌ Most complex solution<br>❌ Requires buffering and reorder logic | Ordering is optional, making this additional complexity unnecessary. |
| **Version Numbers at Receiver** | Optimistic | Receiver reconstructs ordering | ✅ Sender remains simple<br>✅ Highly scalable | ❌ Requires receiver support<br>❌ Outside the notifier's control | The notifier cannot assume downstream systems implement version-aware processing. |

---

# Ordering vs Throughput

Strict ordering requires processing each customer's events sequentially.

Example:

```text
Customer A

A1
↓
A2
↓
A3
```

If `A1` is delayed by a slow downstream webhook, `A2` and `A3` must wait.

Horizontal scaling can increase the number of customers processed concurrently, but it **cannot increase throughput for a single ordered customer stream** because serialization is required to preserve ordering.

---

# Recommendation

Given the assignment requirements, the current implementation intentionally prioritizes:

- ✅ Fair scheduling across customers
- ✅ Concurrent worker execution
- ✅ High throughput
- ✅ Simpler operational model

instead of:

- ❌ Strict per-customer ordering

This aligns with the stated requirement that out-of-order delivery is acceptable while ensuring that large customers cannot monopolize processing.

---

# Future Evolution

If ordering becomes a requirement, the preferred evolution would be:

1. **Per-Customer Queue (Actor Model)** – Best balance between simplicity, maintainability, and scalability.
2. **Per-Customer Lock** – Smallest change to the existing architecture.
3. **Kafka-style Partitioning** – Preferred for very large-scale deployments requiring durable event streaming.
4. **Sequence Numbers + Reorder Buffer** – Highest throughput, but also the highest implementation complexity.

---

# Summary

| Requirement | Recommended Strategy |
|-------------|----------------------|
| Current assignment | ✅ Fair scheduling with concurrent workers |
| Strict ordering with minimal changes | ✅ Per-Customer Lock |
| Strict ordering with clean architecture | ✅ Per-Customer Queue / Actor |
| Massive-scale event streaming | ✅ Kafka-style Partitioning |
| Maximum throughput with ordering | ✅ Sequence Numbers + Reorder Buffer |