# ADR 0003: Replace Kafka With A PostgreSQL-Backed Work Queue

- Status: Accepted
- Date: 2026-08-03
- Deciders: Project implementation team

## Context

The current implementation uses Kafka as the transport between event generation and
webhook delivery.

That design works, but it increases the amount of infrastructure and operational
knowledge required to run and explain this project:

- local development depends on Kafka connectivity and port-forwards
- the notifier can fail due to broker reachability issues unrelated to delivery logic
- the mock event generator exists mainly to publish into Kafka
- the project needs Kafka-specific CI, local setup, and troubleshooting steps

For this assignment, the project goal is to demonstrate a reliable webhook
notifier with fair scheduling, retries, DLQ handling, and PostgreSQL-backed
webhook registrations.

Kafka is not the core requirement of the assignment itself. It is an
implementation choice that adds complexity to an otherwise simpler notifier.

Because PostgreSQL is already required for webhook registrations, the team can
simplify the architecture by using the same database as the work queue store.

## Decision

The project will stop using Kafka as the primary event transport and will move to
a **PostgreSQL-backed work queue** using the same PostgreSQL database already used
for webhook registrations.

The new default flow will be:

1. incoming subscriber events are written into a PostgreSQL queue table
2. the notifier polls and claims pending jobs from PostgreSQL
3. the existing scheduler and worker pool process claimed jobs
4. retry metadata and terminal failure state are persisted in PostgreSQL
5. dead-lettered records remain queryable in PostgreSQL instead of being
   published to a Kafka DLQ topic

Kafka-related components may remain temporarily during migration, but they are no
longer the target architecture for the simplified project.

## Decision Drivers

- reduce local setup complexity
- avoid Kafka-specific connectivity failures during demonstration
- keep infrastructure aligned with the minimum assignment needs
- reuse the PostgreSQL dependency that already exists
- make the project easier to explain, run, and submit

## Consequences

### Positive

- one fewer infrastructure dependency to run locally
- simpler README, bootstrap, and troubleshooting paths
- easier end-to-end testing with a single persistent store
- queue state, retries, and dead letters become directly inspectable in SQL
- fewer moving parts for demonstration and submission review

### Trade-Offs

- the system loses Kafka consumer-group semantics as the default scaling model
- queue claiming and concurrency control must now be implemented carefully in SQL
- PostgreSQL becomes more central to runtime correctness
- some existing Kafka integration code, tests, and workflow steps will need to be
  removed or replaced

## Implementation Guidance

The migration should prefer small, explicit steps:

1. introduce queue tables for pending, claimed, completed, and dead-lettered work
2. add a queue repository abstraction for enqueue, claim, ack, retry, and dead-letter operations
3. move the notifier intake path from Kafka consumption to PostgreSQL queue polling
4. update the mock event generator to write into PostgreSQL instead of Kafka
5. remove Kafka from local stack scripts, README instructions, and CI workflows
6. replace Kafka integration tests with PostgreSQL queue integration tests

## Out Of Scope For This ADR

This ADR does not define:

- the exact SQL schema
- the exact row-locking strategy
- whether polling uses `FOR UPDATE SKIP LOCKED` or another claim pattern
- whether event ingestion happens by HTTP only or through a separate producer process

Those details should be captured in follow-up implementation work or a more
specific ADR if needed.

## Related Files

- [README.md](../README.md)
- [implementation-checklist.md](../plan/implementation-checklist.md)
- [internal/notifier/app.go](../../internal/notifier/app.go)
- [internal/registration/postgres.go](../../internal/registration/postgres.go)
- [internal/kafka/segmentio.go](../../internal/kafka/segmentio.go)
- [scripts/start-local-stack.sh](../../scripts/start-local-stack.sh)
