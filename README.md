# Webhook Notifier

This project is a Go-based webhook notifier MVP.

Current runtime flow:

1. `mock-event-generator` publishes `SubscriberEvent` messages to Kafka
2. `notifier` consumes events from Kafka
3. `notifier` loads webhook endpoint registrations from PostgreSQL
4. `notifier` schedules delivery jobs across customers
5. workers send HTTP `POST` requests to customer webhook endpoints
6. failed deliveries retry with exponential backoff
7. exhausted deliveries are published to the DLQ topic

## Current Status

What is implemented now:

- Kafka producer in the mock event generator
- Kafka consumer in the notifier
- PostgreSQL-backed webhook registration lookup
- round-robin scheduler
- configurable worker pool
- retry with exponential backoff
- mock webhook receiver for local testing
- health, stats, registration snapshot, DLQ, and Prometheus metrics endpoints

What is not fully validated yet:

- full live end-to-end flow against your real Kafka and PostgreSQL environment
- integration tests for the complete runtime path
- final outbound webhook payload contract split from the Kafka event contract

## Prerequisites

- Go `1.26.4` or compatible
- Kafka reachable from the machine running the apps
- PostgreSQL reachable from the machine running the notifier

Known Kubernetes services already present in your environment:

- Kafka: `kafka-service.default.svc.cluster.local:9092`
- Prometheus: `prometheus.monitoring.svc.cluster.local:9090`
- Grafana: `grafana.monitoring.svc.cluster.local:3000`

## Registration Schema Assumption

The notifier currently expects webhook registrations to be readable with these default queries:

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

Shell script to create the database if it does not already exist, then create the table and seed rows:

```bash
#!/usr/bin/env bash
set -euo pipefail

POSTGRES_ADMIN_DSN="${POSTGRES_ADMIN_DSN:-postgres://postgres:password@localhost:5432/postgres?sslmode=disable}"
WEBHOOK_NOTIFIER_DB_NAME="${WEBHOOK_NOTIFIER_DB_NAME:-webhook_notifier}"

database_exists="$(
  psql "$POSTGRES_ADMIN_DSN" -tAc "SELECT 1 FROM pg_database WHERE datname = '${WEBHOOK_NOTIFIER_DB_NAME}'"
)"

if [ "$database_exists" != "1" ]; then
  psql "$POSTGRES_ADMIN_DSN" -c "CREATE DATABASE ${WEBHOOK_NOTIFIER_DB_NAME}"
fi

NOTIFIER_DATABASE_DSN="${NOTIFIER_DATABASE_DSN:-postgres://postgres:password@localhost:5432/${WEBHOOK_NOTIFIER_DB_NAME}?sslmode=disable}"

psql "$NOTIFIER_DATABASE_DSN" <<'SQL'
CREATE TABLE IF NOT EXISTS webhook_registrations (
  customer_id TEXT NOT NULL,
  webhook_url TEXT NOT NULL,
  is_active BOOLEAN NOT NULL DEFAULT TRUE
);

INSERT INTO webhook_registrations (customer_id, webhook_url, is_active)
VALUES
  ('customer-a', 'http://localhost:28082/webhook/customer-a', TRUE),
  ('customer-b', 'http://localhost:28082/webhook/customer-b', TRUE),
  ('customer-c', 'http://localhost:28082/webhook/customer-c', TRUE)
ON CONFLICT DO NOTHING;
SQL
```

If your schema is different, override:

- `NOTIFIER_REGISTRATION_RESOLVE_QUERY`
- `NOTIFIER_REGISTRATION_SNAPSHOT_QUERY`

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
- `NOTIFIER_LOG_LEVEL` default: `INFO`
- `NOTIFIER_KAFKA_BROKERS` default: empty
- `NOTIFIER_KAFKA_HOST_OVERRIDES` default: empty
- `NOTIFIER_KAFKA_TOPIC` default: `subscriber-events`
- `NOTIFIER_KAFKA_CONSUMER_GROUP` default: `webhook-notifier`
- `NOTIFIER_KAFKA_DLQ_TOPIC` default: `subscriber-events-dlq`
- `NOTIFIER_REGISTRATION_RESOLVE_QUERY`
- `NOTIFIER_REGISTRATION_SNAPSHOT_QUERY`

### Mock Event Generator

- `GENERATOR_HTTP_ADDRESS` default: `:28081`
- `GENERATOR_NOTIFIER_BASE_URL` default: `http://localhost:28080`
- `GENERATOR_DEFAULT_CUSTOMER_COUNT` default: `5`
- `GENERATOR_RANDOM_SEED` default: current timestamp
- `GENERATOR_LOG_LEVEL` default: `INFO`
- `GENERATOR_KAFKA_BROKERS` default: empty
- `GENERATOR_KAFKA_HOST_OVERRIDES` default: empty
- `GENERATOR_KAFKA_TOPIC` default: `subscriber-events`

### Mock Webhook Receiver

- `RECEIVER_HTTP_ADDRESS` default: `:28082`
- `RECEIVER_LOG_LEVEL` default: `INFO`

## Run The Kafka-Backed Flow

Recommended run order:

1. start PostgreSQL and seed webhook registrations
2. make Kafka reachable from your local machine
3. start mock webhook receiver
4. start notifier
5. start mock event generator
6. generate test events

### 1a. Make Kafka Reachable From Local Development

Your Kafka broker currently advertises:

```text
PLAINTEXT://kafka-service:9092
```

That host name works inside Kubernetes, but not from a normal shell on your machine.

For local development, use a `kubectl port-forward` and Kafka host overrides:

```bash
kubectl port-forward -n default svc/kafka-service 9092:9092
```

Then set the Kafka host override env vars so the Kafka client rewrites the broker's advertised name:

```bash
NOTIFIER_KAFKA_HOST_OVERRIDES='kafka-service=127.0.0.1,kafka-service.default.svc.cluster.local=127.0.0.1'
GENERATOR_KAFKA_HOST_OVERRIDES='kafka-service=127.0.0.1,kafka-service.default.svc.cluster.local=127.0.0.1'
```

Without this override, a local run will fail because the broker metadata points clients back to `kafka-service:9092`.

If you want a single helper to prepare both PostgreSQL and Kafka local access, use:

```bash
scripts/ensure-local-port-forwards.sh
```

To keep the port-forwards attached to the helper process until you stop it:

```bash
KEEP_RUNNING=true scripts/ensure-local-port-forwards.sh
```

Default forwarded addresses:

- PostgreSQL: `127.0.0.1:15432`
- Kafka: `127.0.0.1:9092`

### 2. Start The Mock Receiver

```bash
go run ./cmd/mock-webhook-receiver
```

Health check:

```bash
curl -sS http://localhost:28082/health
```

### 3. Start The Notifier

Example using local port-forwarded Kafka and a local PostgreSQL DSN:

```bash
NOTIFIER_POSTGRES_DSN='postgres://postgres:password@localhost:5432/webhook_notifier?sslmode=disable' \
NOTIFIER_KAFKA_BROKERS='127.0.0.1:9092' \
NOTIFIER_KAFKA_HOST_OVERRIDES='kafka-service=127.0.0.1,kafka-service.default.svc.cluster.local=127.0.0.1' \
NOTIFIER_KAFKA_TOPIC='subscriber-events' \
NOTIFIER_KAFKA_DLQ_TOPIC='subscriber-events-dlq' \
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

Kafka-backed mode:

```bash
GENERATOR_KAFKA_BROKERS='127.0.0.1:9092' \
GENERATOR_KAFKA_HOST_OVERRIDES='kafka-service=127.0.0.1,kafka-service.default.svc.cluster.local=127.0.0.1' \
GENERATOR_KAFKA_TOPIC='subscriber-events' \
go run ./cmd/mock-event-generator
```

Health check:

```bash
curl -sS http://localhost:28081/health
```

### 5. Generate Events

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

## Dev-Only HTTP Fallback

The notifier still exposes `POST /events` and `POST /events/batch`.

That path is useful for quick local development, but the intended runtime flow is Kafka-first. Prefer the generator-to-Kafka path for normal validation.

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

## Test Commands

Run the focused unit tests that exist today:

```bash
go test ./internal/registration ./internal/retry ./internal/scheduler
```

Run the whole repository:

```bash
go test ./...
```

## Known Gaps

- full end-to-end integration tests are still missing
- outbound webhook payload is still the same shape as the Kafka event
- live verification against your exact PostgreSQL schema still needs to be completed
