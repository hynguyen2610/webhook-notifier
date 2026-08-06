# Webhook Notifier

## 1. Overview

This project is a Go webhook notifier assignment built around a PostgreSQL-backed
work queue. It accepts subscriber events, resolves registered webhook endpoints,
queues delivery work durably in PostgreSQL, schedules jobs fairly across
customers, retries transient failures with backoff, and records exhausted
deliveries in a dead-letter path.

Explicitly supported event types:

- `subscriber.created`
- `subscriber.added_to_segment`
- `subscriber.unsubscribed`

The system still accepts any non-empty `eventType` string and preserves it
through queueing and delivery, but the three values above are the documented and
tested set for local tooling and examples.

## 2. Assignment Challenges

### Reliability

The system favors durable queue state and explicit retry/dead-letter handling
over in-memory best effort. Events are persisted before worker delivery, retry
state is stored through `available_at` and `retry_count`, and failures stay
queryable in PostgreSQL.

Implementation:

- Queue persistence and state transitions: [internal/workqueue/postgres.go](internal/workqueue/postgres.go)
- Delivery, retry, and dead-letter handling: [internal/notifier/worker.go](internal/notifier/worker.go), [internal/notifier/worker_delivery.go](internal/notifier/worker_delivery.go), [internal/notifier/worker_dead_letter.go](internal/notifier/worker_dead_letter.go)
- HTTP delivery timeout and retryability classification: [internal/delivery/http_client.go](internal/delivery/http_client.go)

Evidence:

- Retry then success: [internal/notifier/retry_integration_test.go](internal/notifier/retry_integration_test.go)
- PostgreSQL-backed end-to-end delivery: [internal/notifier/postgres_full_flow_integration_test.go](internal/notifier/postgres_full_flow_integration_test.go)
- Queue retry/dead-letter persistence: [internal/workqueue/postgres_integration_test.go](internal/workqueue/postgres_integration_test.go)

### Fairness

The system uses round-robin scheduling after queue claim so one busy customer is
less likely to monopolize worker execution inside a notifier instance. The
fairness claim is intentionally modest: the scheduler is fairness-oriented, not
an exact ordering guarantee across all concurrency and PostgreSQL claim timing.

Implementation:

- Round-robin scheduler: [internal/scheduler/round_robin.go](internal/scheduler/round_robin.go)
- Notifier fairness flow: [internal/notifier/poller.go](internal/notifier/poller.go), [internal/notifier/runtime.go](internal/notifier/runtime.go)

Evidence:

- In-memory scheduler fairness behavior: [internal/scheduler/round_robin_test.go](internal/scheduler/round_robin_test.go)
- Notifier fairness integration coverage: [internal/notifier/fairness_integration_test.go](internal/notifier/fairness_integration_test.go)
- PostgreSQL-backed fairness coverage: [internal/notifier/postgres_fairness_integration_test.go](internal/notifier/postgres_fairness_integration_test.go)

### Horizontal Scalability

The notifier is designed so multiple processes can share the same PostgreSQL
queue. Workers claim available rows with database-backed coordination, then each
instance schedules and executes work independently.

Implementation:

- PostgreSQL queue claim and state transitions: [internal/workqueue/postgres.go](internal/workqueue/postgres.go)
- Multi-instance benchmark tooling: [scripts/run-multi-instance-benchmark.sh](scripts/run-multi-instance-benchmark.sh)

Evidence:

- Latest multi-instance benchmark report: [docs/test-report/multi-instance-benchmark-20260803-235333.md](docs/test-report/multi-instance-benchmark-20260803-235333.md)
- Benchmark methodology: [docs/spec/multi-instance-load-test.md](docs/spec/multi-instance-load-test.md)
- Benchmark-report clarity ADR: [docs/adr/0004_improve_benchmark_report_clarity_and_app_level_confidence.md](docs/adr/0004_improve_benchmark_report_clarity_and_app_level_confidence.md)

## 3. Architecture

High-level runtime flow:

1. `mock-event-generator` or a direct client sends subscriber events to the notifier ingest API.
2. `notifier` resolves webhook registrations from PostgreSQL.
3. `notifier` writes delivery jobs into `webhook_delivery_queue`.
4. notifier workers claim pending rows, run round-robin scheduling, and deliver HTTP webhooks.
5. retryable failures are rescheduled through `available_at`; exhausted failures are marked dead-lettered.

Key code entry points:

- Notifier application setup: [internal/notifier/app.go](internal/notifier/app.go)
- HTTP ingest handlers: [internal/notifier/handlers.go](internal/notifier/handlers.go)
- Queue repository: [internal/workqueue/postgres.go](internal/workqueue/postgres.go)
- Mock receiver: [internal/mockreceiver/app.go](internal/mockreceiver/app.go)
- Mock generator: [internal/mockgenerator/app.go](internal/mockgenerator/app.go)

Supporting diagrams:

- [docs/diagram/001_system_architecture.mmd](docs/diagram/001_system_architecture.mmd)
- [docs/diagram/002_multi_instances.mmd](docs/diagram/002_multi_instances.mmd)
- [docs/diagram/003_worker_pool.mmd](docs/diagram/003_worker_pool.mmd)

## 4. Design Decisions

The highest-impact design choices are:

- PostgreSQL-backed work queue instead of Kafka for this assignment’s delivery durability, inspectability, and operational simplicity: [docs/adr/0003_postgres_backed_work_queue.md](docs/adr/0003_postgres_backed_work_queue.md)
- Phase-1 MVP scope before broader production features: [docs/adr/0001_phase_1_mvp_scope.md](docs/adr/0001_phase_1_mvp_scope.md)
- Fairness evaluated as customer progress rather than exact global ordering: [docs/adr/0005_relax_exact_delivery_order_assertions_in_fairness_tests.md](docs/adr/0005_relax_exact_delivery_order_assertions_in_fairness_tests.md)
- Benchmark reports should separate throughput confidence from fairness confidence: [docs/adr/0004_improve_benchmark_report_clarity_and_app_level_confidence.md](docs/adr/0004_improve_benchmark_report_clarity_and_app_level_confidence.md)

Broader trade-offs and decision history:

- Trade-offs: [docs/tradeoff.md](docs/tradeoff.md)
- Decision evolution: [docs/decision-evolution.md](docs/decision-evolution.md)

## 5. Testing

The test strategy is layered:

- unit coverage for focused logic such as retry backoff, scheduling, request validation, and metrics
- integration coverage for notifier delivery, retry, timeout, dead-letter, fairness, and PostgreSQL-backed queue behavior
- benchmark and load-test artifacts for fairness and horizontal-scale evidence

Useful commands:

```bash
go test ./internal/mockgenerator ./internal/mockreceiver ./internal/notifier ./internal/registration ./internal/retry ./internal/scheduler ./internal/workqueue
```

```bash
scripts/start-local-stack.sh
```

```bash
scripts/test-full-flow.sh
```

Representative evidence:

- Notifier retry and dead-letter behavior: [internal/notifier/retry_integration_test.go](internal/notifier/retry_integration_test.go)
- PostgreSQL-backed ingest-to-delivery flow: [internal/notifier/postgres_full_flow_integration_test.go](internal/notifier/postgres_full_flow_integration_test.go)
- Queue semantics: [internal/workqueue/postgres_integration_test.go](internal/workqueue/postgres_integration_test.go)
- Fairness-focused integration coverage: [internal/notifier/postgres_fairness_integration_test.go](internal/notifier/postgres_fairness_integration_test.go)

## 6. Future Work

The most important next steps are:

- separate the final outbound webhook payload contract from the internal subscriber event model
- improve observability around queue polling, claim behavior, and dead-letter metrics
- strengthen production-facing fairness and horizontal-scale evidence beyond local-machine benchmarks
- evolve scheduling and ordering guarantees only when the product contract requires stronger semantics

More detail:

- Future scalability: [docs/future/future_scalability.md](docs/future/future_scalability.md)
- Future scheduling discussion: [docs/future/future_scheduling.md](docs/future/future_scheduling.md)
- Ordering discussion: [docs/future/ordering-discussion.md](docs/future/ordering-discussion.md)

## Appendix

### A. Load Test Methodology

- Single-instance load-test spec: [docs/spec/load-test.md](docs/spec/load-test.md)
- Multi-instance load-test spec: [docs/spec/multi-instance-load-test.md](docs/spec/multi-instance-load-test.md)

### B. Load Test Results

- Multi-instance benchmark report: [docs/test-report/multi-instance-benchmark-20260803-235333.md](docs/test-report/multi-instance-benchmark-20260803-235333.md)
- Scheduler benchmark report: [docs/test-report/scheduler-benchmark-scheduler-20260804-003724.html](docs/test-report/scheduler-benchmark-scheduler-20260804-003724.html)
- Metrics appendix: [docs/test-report/load-test-metrics-appendix.html](docs/test-report/load-test-metrics-appendix.html)

### C. Fairness Measurements

- Scheduler benchmark source and report generator: [cmd/scheduler-benchmark](cmd/scheduler-benchmark)
- Worker fairness scenarios: [docs/plan/worker-fairness-scenarios.md](docs/plan/worker-fairness-scenarios.md)
- PostgreSQL-backed fairness tests: [internal/notifier/postgres_fairness_integration_test.go](internal/notifier/postgres_fairness_integration_test.go)

### D. Architecture Trade-offs

- Trade-offs: [docs/tradeoff.md](docs/tradeoff.md)
- PostgreSQL work queue ADR: [docs/adr/0003_postgres_backed_work_queue.md](docs/adr/0003_postgres_backed_work_queue.md)
- Assignment scope ADR: [docs/adr/0002_assignment_scope_and_clarification.md](docs/adr/0002_assignment_scope_and_clarification.md)

### E. Future Production Evolution

- Future scalability: [docs/future/future_scalability.md](docs/future/future_scalability.md)
- Future scheduling: [docs/future/future_scheduling.md](docs/future/future_scheduling.md)
- Ordering discussion: [docs/future/ordering-discussion.md](docs/future/ordering-discussion.md)
