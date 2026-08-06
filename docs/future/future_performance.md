# Future Performance Optimization

## Purpose

This document captures performance-focused follow-up ideas for the current
PostgreSQL-backed notifier architecture.

It is intentionally narrower than
[future_scalability.md](./future_scalability.md):

- `future_scalability.md` focuses on platform evolution, horizontal deployment,
  and larger-system architecture
- this document focuses on throughput, latency, queue-drain efficiency, and
  wasted work inside the current notifier design

The goal is to improve real delivery performance before taking on more complex
platform changes.

## Guiding Principle

Prefer optimization in this order:

1. measure the bottleneck clearly
2. remove wasted work in the current design
3. tune queue claim and worker behavior
4. only then add more infrastructure complexity

## Priority Roadmap

| Priority | Area | Optimization | Why |
| --- | --- | --- | --- |
| **P0** | Measurement | Add clearer queue-claim, poll, worker-busy, and retry-delay metrics | Performance work is low-confidence without direct visibility into where time is spent |
| **P0** | Queue Claim Path | Benchmark and tune `NOTIFIER_QUEUE_CLAIM_BATCH_SIZE` and `NOTIFIER_QUEUE_POLL_INTERVAL` | Current fairness and throughput behavior are strongly influenced by claim batching and poll timing |
| **P0** | Delivery Concurrency | Tune worker count against real queue depth and receiver behavior | Too few workers wastes capacity; too many can add DB churn and HTTP contention |
| **P0** | Retry Pressure | Reduce waste from repeatedly retrying unhealthy endpoints | Failed endpoints can consume queue capacity and worker time disproportionately |
| **P1** | Database Indexes | Revisit queue and registration indexes using measured query patterns | Queue claim and update cost can dominate throughput before scheduler cost does |
| **P1** | Scheduler Efficiency | Measure scheduler queue depth, drain speed, and enqueue/dequeue cost under heavier workloads | Helps confirm whether the scheduler remains a minor cost relative to claim and delivery work |
| **P1** | Database Efficiency | Revisit queue indexes, claim query shape, and update patterns under load | PostgreSQL claim/update cost is likely to dominate before pure Go scheduler cost does |
| **P1** | Delivery HTTP Path | Reuse transport settings and measure timeout/connection behavior more explicitly | Slow network behavior can silently cap throughput and inflate retry pressure |
| **P2** | Read Caching | Add caching only for measured hot read paths such as registration lookup | Caching adds invalidation complexity and should follow proof that read cost matters |
| **P2** | Customer Isolation | Add controls to limit how much one unhealthy or whale customer can hurt everybody else | Improves tail latency and fairness under uneven workloads |
| **P2** | Payload/Serialization | Measure JSON marshal and request creation overhead before optimizing | Likely lower impact than queue or network cost, but worth validating |
| **P3** | Architectural Split | Consider separating claim/schedule and delivery roles only if measurements show clear contention | Extra moving parts should be justified by observed bottlenecks, not by default |

## Optimization Areas

## 1. Queue Claim And Poll Tuning

The strongest current candidate bottleneck is the PostgreSQL claim loop rather
than the in-memory scheduler.

Focus areas:

- vary `NOTIFIER_QUEUE_CLAIM_BATCH_SIZE`
- vary `NOTIFIER_QUEUE_POLL_INTERVAL`
- measure completed jobs per second and oldest pending age together
- compare fairness effects when queue rows are inserted in grouped versus mixed
  customer order

Relevant code:

- [internal/notifier/poller.go](../../internal/notifier/poller.go)
- [internal/workqueue/postgres.go](../../internal/workqueue/postgres.go)

## 2. Worker Utilization

Worker count should be tuned against:

- receiver latency
- retry rate
- queue claim throughput
- DB update pressure

Increasing workers blindly may hurt total performance if:

- workers sit idle waiting for queue claims
- DB updates become the dominant bottleneck
- too many concurrent failing requests create retry storms

Relevant code:

- [internal/notifier/runtime.go](../../internal/notifier/runtime.go)
- [internal/notifier/worker.go](../../internal/notifier/worker.go)

## 3. Retry Cost Control

Retry improves reliability, but it can waste capacity when endpoints are slow or
persistently failing.

Useful future improvements:

- stronger metrics for retry backlog versus fresh work backlog
- endpoint-health-aware backoff tuning
- circuit-breaker style pauses for repeatedly failing destinations
- per-customer retry pressure visibility

Relevant code:

- [internal/notifier/worker_delivery.go](../../internal/notifier/worker_delivery.go)
- [internal/retry/backoff.go](../../internal/retry/backoff.go)

## 4. Database Index Strategy

Database indexes are a likely high-value optimization area because the notifier
depends heavily on PostgreSQL queue claims and queue-state updates.

Priority candidates:

- claim query support for `status`, `available_at`, and stable ordering by `id`
- update-path support for retry, completion, and dead-letter transitions
- registration lookup support for `customer_id` and `is_active`
- archive and inspection query support if dead-letter or completed rows grow
  significantly

Important guardrails:

- add or change indexes based on measured query plans, not guesswork
- weigh read/query improvement against extra write amplification
- treat queue indexes as a throughput feature, not just a reporting feature

Relevant code:

- [internal/workqueue/postgres.go](../../internal/workqueue/postgres.go)
- [internal/registration/postgres.go](../../internal/registration/postgres.go)

## 5. Database Write Efficiency

The queue architecture depends on repeated claim, retry, completion, and
dead-letter updates.

Future investigation areas:

- whether current indexes remain sufficient under larger queue sizes
- whether claim ordering should evolve beyond simple `available_at, id`
- whether update frequency can be reduced without losing observability
- whether completed/dead-letter retention should eventually move to archive
  tables

Relevant code:

- [internal/workqueue/postgres.go](../../internal/workqueue/postgres.go)

## 6. Caching Strategy

Caching may help, but it should follow measurement rather than lead the
optimization plan.

Best candidate:

- cache active webhook registration lookups by `customer_id`

Why this is not first:

- queue claiming and delivery HTTP are more likely bottlenecks than registration
  reads
- stale registration data can misroute deliveries if cache invalidation is weak
- a cache helps only if repeated customer lookup cost is material in real runs

If caching is introduced, prefer:

- short TTLs or explicit invalidation
- clear metrics for cache hit rate and stale-read risk
- keeping queue-state correctness independent from the cache

Relevant code:

- [internal/registration/postgres.go](../../internal/registration/postgres.go)
- [internal/notifier/handlers.go](../../internal/notifier/handlers.go)

## 7. Benchmark Confidence

Performance optimization needs stronger evidence than one benchmark shape.

Useful next experiments:

- compare prequeued versus ingest-path measurements explicitly
- compare grouped-row insertion versus interleaved-row insertion
- compare fast receiver versus deliberately slow receiver
- compare no-retry versus retry-heavy workloads

Supporting docs:

- [../spec/load-test.md](../spec/load-test.md)
- [../spec/multi-instance-load-test.md](../spec/multi-instance-load-test.md)
- [../test-report/multi-instance-benchmark-20260803-235333.md](../test-report/multi-instance-benchmark-20260803-235333.md)

## What Not To Optimize First

These are lower-confidence first moves unless measurements justify them:

- micro-optimizing JSON serialization
- adding cache layers before proving read-path cost matters
- splitting services before proving a current-process bottleneck
- introducing Kafka purely for theoretical throughput
- adding Kubernetes autoscaling before app-level bottlenecks are observable

## Exit Criteria For Future Performance Work

Performance-focused follow-up should ideally answer:

- what currently limits throughput first: queue claim, delivery HTTP cost, retry
  pressure, or DB updates
- which configuration knobs materially improve throughput without harming
  fairness
- whether horizontal scaling is limited more by PostgreSQL claim behavior or by
  worker execution
- which optimizations are still valid within the PostgreSQL-backed architecture
  before larger platform changes are needed
