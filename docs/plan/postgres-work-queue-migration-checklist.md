# PostgreSQL Work Queue Migration Checklist

This checklist translates [ADR 0003](../adr/0003_postgres_backed_work_queue.md) into an execution plan for replacing Kafka with a PostgreSQL-backed work queue while keeping the project simple and demo-friendly.

## Goal

- [ ] Remove Kafka as the default runtime dependency
- [ ] Use PostgreSQL for both webhook registrations and queued delivery work
- [ ] Preserve fair scheduling, retries, and dead-letter handling
- [ ] Simplify local setup, CI, and documentation

## Step 1: Define The Queue Model

- [ ] Decide the queue table layout
- [ ] Define required columns for queued work:
  - [ ] job ID
  - [ ] event ID
  - [ ] customer ID
  - [ ] subscriber ID
  - [ ] event type
  - [ ] payload
  - [ ] status
  - [ ] available at
  - [ ] claimed at
  - [ ] claim owner
  - [ ] retry count
  - [ ] last error
  - [ ] dead-lettered at
  - [ ] created at
  - [ ] updated at
- [ ] Decide whether completed jobs are retained or deleted
- [ ] Decide how dead-lettered jobs are represented in PostgreSQL

## Step 2: Add Database Schema And Access Layer

- [x] Add SQL schema for queue tables
- [x] Add indexes for claim and retry queries
- [x] Add a queue repository abstraction
- [x] Add repository methods for:
  - [x] enqueue
  - [x] claim next jobs
  - [x] acknowledge success
  - [x] reschedule retry
  - [x] mark dead letter
  - [x] inspect queue state
- [x] Decide and document the row-claiming strategy
Current implementation uses `FOR UPDATE SKIP LOCKED` in `ClaimAvailableDeliveries` so multiple notifier instances can claim disjoint rows safely.

## Step 3: Replace Kafka Intake In The Notifier

- [x] Remove Kafka consumer as the default notifier intake path
- [x] Add a PostgreSQL polling loop in the notifier
- [x] Claim pending jobs from PostgreSQL in bounded batches
- [x] Translate claimed rows into internal delivery jobs
- [x] Keep graceful shutdown behavior for the polling loop
- [x] Ensure multiple notifier instances can claim work safely

## Step 4: Preserve Scheduler And Worker Behavior

- [x] Keep per-customer queue isolation
- [x] Keep round-robin scheduling behavior after queue claims
- [x] Keep configurable worker pool behavior
- [x] Ensure slow or failing customers do not block others

## Step 5: Move Retry And DLQ State Into PostgreSQL

- [x] Replace Kafka DLQ publishing with PostgreSQL dead-letter persistence
- [x] Persist retry attempts and next available time in PostgreSQL
- [x] Requeue retryable jobs through PostgreSQL instead of in-memory timers alone
- [x] Keep exponential backoff behavior
- [x] Keep failure reason visibility for dead-lettered jobs

## Step 6: Update The Mock Event Generator

- [ ] Replace Kafka publishing with PostgreSQL enqueue writes
- [x] Keep existing HTTP endpoints:
  - [x] `POST /generate`
  - [x] `POST /generate/bulk`
  - [x] `POST /scenario/whale`
  - [x] `POST /scenario/mixed`
- [x] Keep deterministic seeded generation behavior
- [x] Keep useful request validation and logging

## Step 7: Update Observability

- [ ] Replace Kafka-specific logs with queue-specific logs
- [ ] Add queue polling and claim metrics
- [ ] Add queue depth metrics
- [ ] Add dead-letter record metrics sourced from PostgreSQL flow
- [ ] Keep health endpoint behavior meaningful without Kafka

## Step 8: Update Tests

- [ ] Remove or replace Kafka integration tests
- [ ] Add PostgreSQL queue integration tests for:
  - [x] enqueue and claim
  - [x] retry then success
  - [x] retry exhaustion to dead letter
  - [ ] multi-customer fairness
  - [x] concurrent claim safety
- [x] Keep existing notifier fast integration coverage where still applicable
- [ ] Add race coverage for the PostgreSQL polling and claim path

## Step 9: Update Local Tooling

- [ ] Remove Kafka dependency from `scripts/start-local-stack.sh`
- [ ] Remove Kafka dependency from `scripts/ensure-local-port-forwards.sh`
- [ ] Remove Kafka-specific local troubleshooting steps
- [ ] Keep one-command local startup working with PostgreSQL only

## Step 10: Update CI

- [ ] Remove Kafka provisioning from integration workflow
- [ ] Replace Kafka-backed integration test steps with PostgreSQL queue integration tests
- [ ] Review whether the dedicated race workflow still covers the right packages
- [ ] Simplify CI runtime where Kafka was previously required

## Step 11: Update Documentation

- [ ] Rewrite README runtime flow to use PostgreSQL queueing
- [ ] Update architecture diagrams
- [ ] Update configuration documentation
- [ ] Remove Kafka bootstrap instructions from README
- [ ] Document how to inspect queued, retried, and dead-lettered jobs in PostgreSQL

## Step 12: Cleanup

- [ ] Remove unused Kafka code paths
- [ ] Remove unused Kafka configuration fields
- [ ] Remove obsolete Kafka scripts and references
- [ ] Remove obsolete Kafka-specific tests and workflow assumptions

## Open Decisions

- [ ] Should completed jobs be deleted immediately or retained for inspection?
- [ ] What batch size should the notifier claim per poll?
- [ ] What polling interval should the notifier use by default?
- [ ] Should retries be fully database-driven or partially timer-driven after claim?
- [ ] Do we want a separate table for dead letters or a status column on one queue table?
