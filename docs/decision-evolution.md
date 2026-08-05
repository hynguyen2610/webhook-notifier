# Decision Evolution

This document describes how the architecture evolved as the implementation progressed. Each decision represents a change in design direction rather than a compromise in project scope.

| Decision | Alternatives | Chosen | Rationale |
| --- | --- | --- | --- |
| Project scope | Production-ready implementation vs. focused implementation | Focused implementation | The assignment emphasizes demonstrating architecture and engineering decisions within a limited timeframe. The implementation prioritizes fairness, retries, durability, testing, and documentation over broader production concerns. |
| Implementation language | Java (more familiar) vs. Go (customer ecosystem) | Go | Go is closer to the customer's technology stack and provides a more representative implementation of the expected runtime environment. It also allowed me to explore Go's concurrency model for a system involving workers, scheduling, and asynchronous delivery. |
| Queue | Kafka-style event transport and asynchronous queueing | PostgreSQL-backed work queue | Kafka provides stronger scalability and streaming capabilities, but it introduces additional operational complexity. For this assignment, the expected workload does not require Kafka-level scale. A PostgreSQL-backed queue provides sufficient durability, work claiming, and retry support while keeping the system simpler to develop, test, and operate. |
| Retry and failure handling model | Kafka consumer retry topics and external dead-letter topics vs. database-managed retry state | Persist retry state and dead-letter status in PostgreSQL | After moving from Kafka-based processing to a database-backed work queue, retry handling was redesigned to keep delivery state within the same durable storage system. This simplified recovery flows and made failed deliveries easier to inspect and replay. |
| Benchmark reporting | Provide raw benchmark output vs. provide summarized engineering insights | Add interpreted benchmark reports | Raw benchmark numbers alone do not clearly communicate system behavior or architectural impact. The report was improved to summarize the workload, measured metrics, observed behavior, and conclusions so that customers can understand whether the design meets the intended goals. |

## Related ADRs

- [0003_postgres_backed_work_queue.md](./adr/0003_postgres_backed_work_queue.md)
- [0004_improve_benchmark_report_clarity_and_app_level_confidence.md](./adr/0004_improve_benchmark_report_clarity_and_app_level_confidence.md)
- [0005_relax_exact_delivery_order_assertions_in_fairness_tests.md](./adr/0005_relax_exact_delivery_order_assertions_in_fairness_tests.md)

## Related Tradeoffs

- [tradeoff.md](./tradeoff.md)
