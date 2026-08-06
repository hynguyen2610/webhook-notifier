# How To Run

This guide helps a new user start the webhook notifier locally, send a few realistic test requests, and verify that the system actually delivered them.

## What This Project Does

At a high level, the flow is:

1. a client sends subscriber events to the notifier
2. the notifier looks up webhook registrations for that customer
3. the notifier stores delivery work in PostgreSQL
4. worker processes deliver HTTP requests to the customer webhook endpoint
5. the mock receiver records what it received so you can verify the outcome

In real product terms:

- `send single` means one subscriber event happened, such as one person unsubscribing
- `send batch` means many events are submitted together, such as a backfill job or an upstream service flushing buffered events
- `bulk load` means many customers and many events, which is useful for smoke-testing throughput rather than validating one business case

## Prerequisites

Install:

- Go `1.24+`
- Docker Desktop or another Docker runtime with Compose support
- `psql`
- `curl`

Optional:

- `jq` for prettier JSON output

## Recommended Local Setup

The simplest first-run path is:

1. run PostgreSQL with Docker Compose
2. create the registration table and seed a few customers
3. run the mock receiver
4. run the notifier
5. run the mock event generator

### 1. Start PostgreSQL

From the repository root:

```bash
docker compose up -d
```

This starts PostgreSQL on `127.0.0.1:15432` with:

- database: `webhook_notifier`
- user: `postgres`
- password: `password`

### 2. Create Registrations

The notifier only creates delivery jobs for customers that have active webhook registrations.

Run:

```bash
psql 'postgres://postgres:password@127.0.0.1:15432/webhook_notifier?sslmode=disable' <<'SQL'
CREATE TABLE IF NOT EXISTS webhook_registrations (
  customer_id TEXT NOT NULL,
  webhook_url TEXT NOT NULL,
  is_active BOOLEAN NOT NULL DEFAULT TRUE
);

DELETE FROM webhook_registrations
WHERE customer_id IN (
  'customer-a',
  'customer-b',
  'customer-c',
  'customer-01',
  'customer-02',
  'customer-03',
  'customer-04',
  'customer-05'
);

INSERT INTO webhook_registrations (customer_id, webhook_url, is_active)
VALUES
  ('customer-a',  'http://localhost:28082/webhook/customer-a',  TRUE),
  ('customer-b',  'http://localhost:28082/webhook/customer-b',  TRUE),
  ('customer-c',  'http://localhost:28082/webhook/customer-c',  TRUE),
  ('customer-01', 'http://localhost:28082/webhook/customer-01', TRUE),
  ('customer-02', 'http://localhost:28082/webhook/customer-02', TRUE),
  ('customer-03', 'http://localhost:28082/webhook/customer-03', TRUE),
  ('customer-04', 'http://localhost:28082/webhook/customer-04', TRUE),
  ('customer-05', 'http://localhost:28082/webhook/customer-05', TRUE);
SQL
```

Why seed both naming styles:

- `customer-a` / `customer-b` / `customer-c` are used by the simple examples
- `customer-01` and similar IDs are used by the generator bulk endpoint

### 3. Start The Mock Receiver

In terminal 1:

```bash
go run ./cmd/mock-webhook-receiver
```

Health check:

```bash
curl http://localhost:28082/health
```

Expected result:

```json
{"status":"ok"}
```

### 4. Start The Notifier

In terminal 2:

```bash
NOTIFIER_POSTGRES_DSN='postgres://postgres:password@127.0.0.1:15432/webhook_notifier?sslmode=disable' \
go run ./cmd/notifier
```

Health check:

```bash
curl http://localhost:28080/health
```

Expected result:

```json
{"status":"ok"}
```

### 5. Start The Mock Event Generator

In terminal 3:

```bash
go run ./cmd/mock-event-generator
```

Health check:

```bash
curl http://localhost:28081/health
```

Expected result:

```json
{"status":"ok"}
```

## Basic Tasks

Before each scenario, reset the receiver stats so the result is easy to read:

```bash
curl -X POST http://localhost:28082/stats/reset
```

### Send A Single Event Directly To The Notifier

Real case:
one subscriber just got created or unsubscribed, and the upstream system wants the notifier to deliver that one event now.

Command:

```bash
curl -X POST http://localhost:28080/events \
  -H 'Content-Type: application/json' \
  -d '{
    "eventId": "evt-single-001",
    "customerId": "customer-a",
    "subscriberId": "subscriber-001",
    "eventType": "subscriber.created",
    "occurredAt": "2026-08-06T09:00:00Z"
  }'
```

Expected result:

```json
{"acceptedEvents":1,"createdJobs":1}
```

### Send A Batch Directly To The Notifier

Real case:
an upstream service is flushing multiple subscriber changes in one call instead of making one HTTP request per event.

Command:

```bash
curl -X POST http://localhost:28080/events/batch \
  -H 'Content-Type: application/json' \
  -d '[
    {
      "eventId": "evt-batch-001",
      "customerId": "customer-a",
      "subscriberId": "subscriber-010",
      "eventType": "subscriber.created",
      "occurredAt": "2026-08-06T09:05:00Z"
    },
    {
      "eventId": "evt-batch-002",
      "customerId": "customer-a",
      "subscriberId": "subscriber-011",
      "eventType": "subscriber.added_to_segment",
      "occurredAt": "2026-08-06T09:05:10Z"
    },
    {
      "eventId": "evt-batch-003",
      "customerId": "customer-a",
      "subscriberId": "subscriber-012",
      "eventType": "subscriber.unsubscribed",
      "occurredAt": "2026-08-06T09:05:20Z"
    }
  ]'
```

Expected result:

```json
{"acceptedEvents":3,"createdJobs":3}
```

### Send Several Events Through The Mock Generator

Real case:
you want realistic test traffic without hand-writing every event payload.

Command:

```bash
curl -X POST http://localhost:28081/generate \
  -H 'Content-Type: application/json' \
  -d '{
    "customerId": "customer-a",
    "eventType": "subscriber.unsubscribed",
    "count": 5
  }'
```

Expected result:

```json
{"generated":5}
```

### Send A Multi-Customer Batch Through The Generator

Real case:
you want a quick smoke test that touches multiple customers and produces more realistic queue activity.

Command:

```bash
curl -X POST http://localhost:28081/generate/bulk \
  -H 'Content-Type: application/json' \
  -d '{
    "customers": 5,
    "eventsPerCustomer": 20
  }'
```

Expected result:

```json
{"generated":100}
```

Note:
this call uses customers named `customer-01` through `customer-05`, which is why the setup step seeds those registrations.

### Run The Whale Scenario

Real case:
one customer is much larger than the others, and you want to observe whether smaller customers still make progress.

Command:

```bash
curl -X POST http://localhost:28081/scenario/whale
```

Expected result:

```json
{"generated":2200}
```

## How To Verify Results

Use all three checks below. Together they tell you whether the event was accepted, delivered, and persisted correctly.

### 1. Check Receiver Stats

This confirms the customer endpoint actually received webhook requests.

```bash
curl http://localhost:28082/stats
```

Useful fields:

- `received`: total webhook requests received
- `success`: total 2xx responses returned by the receiver
- `failed`: total non-2xx responses returned by the receiver
- `customers`: per-customer breakdown

To inspect one customer:

```bash
curl http://localhost:28082/stats/customer/customer-a
```

What good looks like after a successful single send for `customer-a`:

- `received` increased by `1`
- `success` increased by `1`
- `eventTypeCounts.subscriber.created` increased by `1`
- `lastEvent.customerId` is `customer-a`

### 2. Check Notifier Stats

This confirms the notifier accepted the event and tracked delivery activity.

```bash
curl http://localhost:28080/stats
```

Useful fields:

- `receivedEvents`: events accepted by the notifier
- `deliveredEvents`: successful deliveries
- `failedDeliveries`: failed delivery attempts
- `retriedDeliveries`: retry attempts scheduled
- `deadLetterCount`: permanently failed deliveries

For a happy-path single send, the usual expectation is:

- `receivedEvents` increases by `1`
- `deliveredEvents` increases by `1`
- `deadLetterCount` stays unchanged

### 3. Check PostgreSQL Queue State

This confirms the queue rows were written and transitioned to the expected status.

See recent queue items:

```bash
psql 'postgres://postgres:password@127.0.0.1:15432/webhook_notifier?sslmode=disable' \
  -c "SELECT id, event_id, customer_id, event_type, status, retry_count, created_at, completed_at FROM webhook_delivery_queue ORDER BY id DESC LIMIT 20;"
```

Status meanings:

- `pending`: queued and waiting to be claimed
- `claimed`: currently owned by a worker
- `completed`: delivered successfully
- `dead_lettered`: permanently failed after retry handling

Check dead-letter contents:

```bash
curl http://localhost:28080/dlq
```

Check active registrations:

```bash
curl http://localhost:28080/registrations
```

## Failure Testing

You can also verify retry and dead-letter behavior.

### Make A Customer Return HTTP 500

```bash
curl -X POST http://localhost:28082/config/customer-a \
  -H 'Content-Type: application/json' \
  -d '{"mode":"error500"}'
```

Then send a test event:

```bash
curl -X POST http://localhost:28080/events \
  -H 'Content-Type: application/json' \
  -d '{
    "eventId": "evt-retry-001",
    "customerId": "customer-a",
    "subscriberId": "subscriber-retry-001",
    "eventType": "subscriber.created",
    "occurredAt": "2026-08-06T09:10:00Z"
  }'
```

What to watch:

- `curl http://localhost:28080/stats`
- `curl http://localhost:28080/dlq`
- the queue row in PostgreSQL

Typical outcome:

- `failedDeliveries` increases
- `retriedDeliveries` increases until retries are exhausted
- the final row may land in `dead_lettered`

Reset the receiver to success mode:

```bash
curl -X POST http://localhost:28082/config/customer-a \
  -H 'Content-Type: application/json' \
  -d '{"mode":"success"}'
```

## Alternative Setup Path

If your team already has PostgreSQL exposed through Kubernetes port-forwarding, the repo also includes:

```bash
scripts/start-local-stack.sh
```

That script bootstraps registrations and starts the receiver, notifier, and generator for you. It is convenient in an environment where the PostgreSQL port-forward helper already works, but the Docker Compose path above is usually the easiest first experience for a new user.
