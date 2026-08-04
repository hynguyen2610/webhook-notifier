# Webhook Notifier

This project is a Go-based webhook notifier MVP built around a PostgreSQL-backed work queue.

Current runtime flow:

1. `mock-event-generator` sends `SubscriberEvent` batches to the notifier HTTP ingest API
2. `notifier` resolves webhook registrations from PostgreSQL
3. `notifier` writes delivery jobs into a PostgreSQL queue table
4. `notifier` polls and claims pending queue rows
5. the round-robin scheduler shares work fairly across customers
6. workers send HTTP `POST` requests to customer webhook endpoints
7. failed deliveries retry with exponential backoff
8. exhausted deliveries are marked as dead-lettered in PostgreSQL

## Current Status

What is implemented now:

- PostgreSQL-backed webhook registration lookup
- PostgreSQL-backed delivery queue
- round-robin scheduler
- configurable worker pool
- retry with exponential backoff
- mock webhook receiver for local testing
- mock event generator for local testing
- health, stats, registration snapshot, DLQ, and Prometheus metrics endpoints
- PostgreSQL-backed integration coverage for queue and registration behavior

What is still incomplete:

- final outbound webhook payload contract split from the internal subscriber event contract
- deeper observability around queue depth and polling behavior
- architecture diagrams and older planning docs still need broader cleanup

## Prerequisites

- Go `1.26.4` or compatible
- PostgreSQL reachable from the machine running the apps

Known Kubernetes services already present in your environment:

- PostgreSQL service used by these scripts: `user-org-db-service.default.svc.cluster.local:5432`
- Prometheus: `prometheus.monitoring.svc.cluster.local:9090`
- Grafana: `grafana.monitoring.svc.cluster.local:3000`

## Registration Schema Assumption

The notifier expects webhook registrations to be readable with these default queries:

```sql
SELECT webhook_url
FROM webhook_registrations
WHERE customer_id = $1
  AND is_active = TRUE
ORDER BY webhook_url;
```

```sql
SELECT customer_id, webhook_url
FROM webhook_registrations
WHERE is_active = TRUE
ORDER BY customer_id, webhook_url;
```

Minimum expected table shape:

```sql
CREATE TABLE IF NOT EXISTS webhook_registrations (
  customer_id TEXT NOT NULL,
  webhook_url TEXT NOT NULL,
  is_active BOOLEAN NOT NULL DEFAULT TRUE
);
```

Example seed data for local verification:

```sql
INSERT INTO webhook_registrations (customer_id, webhook_url, is_active)
VALUES
  ('customer-a', 'http://localhost:28082/webhook/customer-a', TRUE),
  ('customer-b', 'http://localhost:28082/webhook/customer-b', TRUE),
  ('customer-c', 'http://localhost:28082/webhook/customer-c', TRUE);
```

If your schema is different, override:

- `NOTIFIER_REGISTRATION_RESOLVE_QUERY`
- `NOTIFIER_REGISTRATION_SNAPSHOT_QUERY`

## Queue Schema

The notifier creates its PostgreSQL queue table automatically on startup:

```sql
CREATE TABLE IF NOT EXISTS webhook_delivery_queue (
  id BIGSERIAL PRIMARY KEY,
  event_id TEXT NOT NULL,
  customer_id TEXT NOT NULL,
  subscriber_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  occurred_at TIMESTAMPTZ NOT NULL,
  webhook_url TEXT NOT NULL,
  status TEXT NOT NULL,
  available_at TIMESTAMPTZ NOT NULL,
  claimed_at TIMESTAMPTZ NULL,
  claim_owner TEXT NULL,
  retry_count INTEGER NOT NULL DEFAULT 0,
  last_error TEXT NULL,
  dead_lettered_at TIMESTAMPTZ NULL,
  completed_at TIMESTAMPTZ NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);
```

Useful queue states:

- `pending`
- `claimed`
- `completed`
- `dead_lettered`

## Environment Variables

### Notifier

Required:

- `NOTIFIER_POSTGRES_DSN`

Common:

- `NOTIFIER_HTTP_ADDRESS` default: `:28080`
- `NOTIFIER_WORKER_COUNT` default: `4`
- `NOTIFIER_REQUEST_TIMEOUT` default: `5s`
- `NOTIFIER_MAX_RETRY_ATTEMPTS` default: `3`
- `NOTIFIER_INITIAL_RETRY_DELAY` default: `1s`
- `NOTIFIER_QUEUE_CLAIM_BATCH_SIZE` default: `32`
- `NOTIFIER_QUEUE_POLL_INTERVAL` default: `250ms`
- `NOTIFIER_LOG_LEVEL` default: `INFO`
- `NOTIFIER_REGISTRATION_RESOLVE_QUERY`
- `NOTIFIER_REGISTRATION_SNAPSHOT_QUERY`

### Mock Event Generator

- `GENERATOR_HTTP_ADDRESS` default: `:28081`
- `GENERATOR_NOTIFIER_BASE_URL` default: `http://localhost:28080`
- `GENERATOR_DEFAULT_CUSTOMER_COUNT` default: `5`
- `GENERATOR_RANDOM_SEED` default: current timestamp
- `GENERATOR_LOG_LEVEL` default: `INFO`

### Mock Webhook Receiver

- `RECEIVER_HTTP_ADDRESS` default: `:28082`
- `RECEIVER_LOG_LEVEL` default: `INFO`

## Run The PostgreSQL-Backed Flow

Recommended run order:

1. start PostgreSQL and seed webhook registrations
2. start mock webhook receiver
3. start notifier
4. start mock event generator
5. generate test events

### Fastest Local Start

If you want one command that ensures PostgreSQL local access, bootstraps the database, and starts the receiver, notifier, and generator together, use:

```bash
scripts/start-local-stack.sh
```

This script keeps running until you stop it with `Ctrl+C`.

It writes logs to:

- `.tmp/local-stack/port-forwards.log`
- `.tmp/local-stack/mock-receiver.log`
- `.tmp/local-stack/notifier.log`
- `.tmp/local-stack/mock-generator.log`

### 1. Make PostgreSQL Reachable From Local Development

If you want a helper that prepares PostgreSQL local access and keeps the port-forward running in the current terminal, use:

```bash
scripts/ensure-local-port-forwards.sh
```

If you explicitly want the helper to exit and clean up the port-forward when it finishes, set:

```bash
KEEP_RUNNING=false scripts/ensure-local-port-forwards.sh
```

Default forwarded address:

- PostgreSQL: `127.0.0.1:15432`

### 2. Start The Mock Receiver

```bash
go run ./cmd/mock-webhook-receiver
```

Health check:

```bash
curl -sS http://localhost:28082/health
```

### 3. Start The Notifier

Example using the default local port-forwarded PostgreSQL address:

```bash
NOTIFIER_POSTGRES_DSN='postgres://postgres:password@127.0.0.1:15432/webhook_notifier?sslmode=disable' \
go run ./cmd/notifier
```

Health check:

```bash
curl -sS http://localhost:28080/health
```

Available endpoints:

- `GET /health`
- `GET /metrics`
- `GET /stats`
- `GET /registrations`
- `GET /dlq`
- `POST /events`
- `POST /events/batch`

### 4. Start The Mock Event Generator

```bash
GENERATOR_NOTIFIER_BASE_URL='http://localhost:28080' \
go run ./cmd/mock-event-generator
```

Health check:

```bash
curl -sS http://localhost:28081/health
```

### 5. Generate Events

Before generating events, keep the terminal running `scripts/ensure-local-port-forwards.sh` or `scripts/start-local-stack.sh` open. If that helper exits, the PostgreSQL forward stops and the notifier will lose database connectivity.

Single customer batch:

```bash
curl -sS -X POST http://localhost:28081/generate \
  -H 'Content-Type: application/json' \
  -d '{
    "customerId": "customer-a",
    "eventType": "subscriber.created",
    "count": 5
  }'
```

Bulk scenario:

```bash
curl -sS -X POST http://localhost:28081/generate/bulk \
  -H 'Content-Type: application/json' \
  -d '{
    "customers": 3,
    "eventsPerCustomer": 10
  }'
```

Whale scenario:

```bash
curl -sS -X POST http://localhost:28081/scenario/whale
```

Mixed scenario:

```bash
curl -sS -X POST http://localhost:28081/scenario/mixed
```

### Full Flow Smoke Test

Run the end-to-end local verification script:

```bash
scripts/test-full-flow.sh
```

This script:

- ensures PostgreSQL local access
- seeds registration rows
- starts the receiver, notifier, and generator
- sends test events through the generator
- verifies receiver statistics for the expected customer

## Verify The Flow

Check receiver statistics:

```bash
curl -sS http://localhost:28082/stats
```

Check notifier statistics:

```bash
curl -sS http://localhost:28080/stats
```

Check notifier registration snapshot:

```bash
curl -sS http://localhost:28080/registrations
```

Check notifier dead-letter view:

```bash
curl -sS http://localhost:28080/dlq
```

Check metrics:

```bash
curl -sS http://localhost:28080/metrics
```

Inspect queue state directly in PostgreSQL:

```bash
psql 'postgres://postgres:password@127.0.0.1:15432/webhook_notifier?sslmode=disable' \
  -c "SELECT status, customer_id, event_id, retry_count, dead_lettered_at FROM webhook_delivery_queue ORDER BY id DESC LIMIT 20"
```

## Simulate Receiver Failures

Make a customer always fail with `500`:

```bash
curl -sS -X POST http://localhost:28082/config/customer-a \
  -H 'Content-Type: application/json' \
  -d '{
    "mode": "error500"
  }'
```

Make a customer timeout with a 5-second delay:

```bash
curl -sS -X POST http://localhost:28082/config/customer-a \
  -H 'Content-Type: application/json' \
  -d '{
    "mode": "timeout",
    "delay": 5000
  }'
```

Make a customer fail randomly:

```bash
curl -sS -X POST http://localhost:28082/config/customer-a \
  -H 'Content-Type: application/json' \
  -d '{
    "mode": "random",
    "failureProbability": 0.5
  }'
```

Reset receiver stats:

```bash
curl -sS -X POST http://localhost:28082/stats/reset
```

## Direct Ingest API

The notifier ingest endpoints are part of the normal PostgreSQL-backed runtime, not just a fallback path.

Example direct notifier batch request:

```bash
curl -sS -X POST http://localhost:28080/events/batch \
  -H 'Content-Type: application/json' \
  -d '[
    {
      "eventId": "event-1",
      "customerId": "customer-a",
      "subscriberId": "subscriber-001",
      "eventType": "subscriber.created",
      "occurredAt": "2026-08-02T10:00:00Z"
    }
  ]'
```

## Multi-Instance Benchmark

Run the PostgreSQL-backed multi-instance benchmark from the repository root:

```bash
bash scripts/run-multi-instance-benchmark.sh
```

What this benchmark covers:

- preloaded PostgreSQL queue rows
- PostgreSQL row claiming across multiple notifier processes
- round-robin scheduler handoff
- worker execution
- local mock receiver HTTP delivery

What this benchmark does not cover:

- notifier HTTP ingest cost
- retry or dead-letter behavior
- remote network latency
- production PostgreSQL sizing or tuning

Latest local result captured on `2026-08-03`:

- command: `bash scripts/run-multi-instance-benchmark.sh`
- scenario: `two-whales-5000-two-non-whales-100`
- report: `loadtest/reports/multi-instance-benchmark-20260803-235517.md`
- per-instance worker count: `4`
- queue claim batch size: `32`
- queue poll interval: `50ms`
- retries: disabled

| Notifier Instances | Total Jobs | Total Duration | Jobs/Sec | `customer-c` First Completion | `customer-d` First Completion |
| --- | ---: | ---: | ---: | ---: | ---: |
| `1` | `10200` | `16.606s` | `614.23` | `16085.589ms` | `16250.358ms` |
| `2` | `10200` | `13.187s` | `773.49` | `8852.968ms` | `8908.239ms` |
| `4` | `10200` | `10.990s` | `928.15` | `5237.529ms` | `5281.965ms` |

Key takeaway from that run:

- throughput scaled up with more notifier instances: about `+25.93%` from `1` to `2` instances and about `+51.11%` from `1` to `4` instances
- fairness did not hold for the small customers in the early-completion window: `customer-a` owned the first `20` completions in every run
- this points to PostgreSQL claim order dominating scheduler fairness when the queue is preloaded in customer-grouped order

Use this result carefully:

- it is stronger evidence than the in-memory scheduler benchmark because it includes real PostgreSQL claim behavior
- it is still a local-machine benchmark, not production proof
- it is a processing-path benchmark, not an ingest-path benchmark
- the next useful experiment is to vary queue insertion order, claim batch size, and poll interval to see how much of the fairness regression comes from queue claiming versus worker execution

## In-Memory Scheduler Benchmark

Run the benchmark from the repository root:

```bash
go run ./cmd/scheduler-benchmark --include-large-fairness-case=false
```

Run the matching in-memory app fairness benchmark:

```bash
go run ./cmd/scheduler-benchmark --mode app --include-large-fairness-case=false
```

Enable the large opt-in fairness scenario when you want the deeper whale run:

```bash
go run ./cmd/scheduler-benchmark --mode app --include-large-fairness-case=true
```

How to interpret the output:

- the `Throughput Benchmark` tab is scheduler-only microbenchmark evidence
- the `Fairness Benchmark` tab is the customer-facing progress view
- `scheduler` mode includes only the round-robin scheduler plus the synthetic worker harness
- `app` mode includes enqueue, in-memory queue claim, scheduler handoff, worker execution, and synthetic delivery work
- `app` mode still excludes PostgreSQL queue behavior, notifier HTTP ingest, real outbound HTTP delivery cost, and failure-path retry or DLQ behavior

Large opt-in fairness scenarios currently include:

- `two-whales-200000-two-normals-2`
- `two-whales-200000-two-non-whales-1000`

Report structure decision:

- keep one HTML report file per run
- keep throughput and fairness evidence in separate tabs inside that file so reviewers can scan one artifact without mixing the result types

Expected runtime on this repository as measured on `2026-08-03` with the large fairness case skipped:

- scheduler smoke run: about `13s`
- app smoke run: about `7s`
- large fairness scenario enabled: expect materially longer runtime because the run adds `two-whales-200000-two-normals-2` and `two-whales-200000-two-non-whales-1000` in addition to the smoke scenario

Example smoke-run report files generated on `2026-08-03`:

- `loadtest/reports/scheduler-benchmark-scheduler-20260803-130540.html`
- `loadtest/reports/scheduler-benchmark-app-20260803-130553.html`

The benchmark still prints a clickable absolute report path in terminal output after each run.

## Test Commands

Run the focused unit and integration coverage that exercises the PostgreSQL-backed path:

```bash
go test ./internal/mockgenerator ./internal/notifier ./internal/registration ./internal/retry ./internal/scheduler ./internal/workqueue
```

Run the whole repository:

```bash
go test ./...
```

## Known Gaps

- outbound webhook payload is still the same shape as the internal subscriber event
- queue polling and queue depth metrics can be improved
- some older planning and diagram docs still describe the retired Kafka architecture
