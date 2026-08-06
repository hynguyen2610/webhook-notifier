# Mock Event Generator Specification

**Version:** 1.1

---

# Purpose

The Mock Event Generator simulates upstream subscriber event creation for local
development, integration testing, benchmarking, and demonstration.

It sends generated events to the notifier HTTP ingest API, which then persists
delivery work in PostgreSQL.

It is **not** part of the production notification system.

---

# Goals

- Generate realistic webhook events.
- Produce configurable event volumes.
- Simulate whale customers.
- Support repeatable benchmark scenarios.
- Eliminate dependency on upstream systems.

---

# Non Goals

The generator will not:

- Validate business rules.
- Persist generated events directly.
- Simulate webhook delivery.
- Manage webhook registrations.

---

# High-Level Architecture

REST API

↓

Event Generator

↓

Notifier Ingest API

↓

PostgreSQL Work Queue

---

# Event Model

Each generated event should contain at minimum:

```json
{
  "eventId": "UUID",
  "customerId": "customer-a",
  "subscriberId": "subscriber-001",
  "eventType": "subscriber.created",
  "occurredAt": "2026-08-01T10:00:00Z"
}
```

Additional fields may be added as required.

The generator explicitly supports these event types in examples and mixed
scenarios:

- `subscriber.created`
- `subscriber.added_to_segment`
- `subscriber.unsubscribed`

The generator still accepts other non-empty `eventType` values when requested.

---

# Configuration

| Property | Description |
|------------|-------------|
| Notifier Base URL | Base URL for notifier ingest endpoints |
| Default Customer Count | Number of generated customers |
| Random Seed | Optional deterministic generation |

---

# REST API

## Generate Events

POST

```
/generate
```

Request

```json
{
  "customerId": "customer-a",
  "eventType": "subscriber.added_to_segment",
  "count": 100
}
```

Validation notes:

- `customerId` is required
- `count` defaults to `1` when omitted or set to `0`
- negative `count` values are rejected

Response

```json
{
  "generated": 100
}
```

Example:

```bash
curl -X POST http://localhost:28081/generate \
  -H 'Content-Type: application/json' \
  -d '{"customerId":"customer-a","eventType":"subscriber.unsubscribed","count":100}'
```

---

## Generate Bulk Load

POST

```
/generate/bulk
```

Example

```json
{
  "customers": 50,
  "eventsPerCustomer": 1000
}
```

Validation notes:

- `customers` defaults to the configured default customer count when omitted or set to `0`
- `eventsPerCustomer` defaults to `100` when omitted or set to `0`
- negative values are rejected

Example:

```bash
curl -X POST http://localhost:28081/generate/bulk \
  -H 'Content-Type: application/json' \
  -d '{"customers":5,"eventsPerCustomer":20}'
```

---

## Whale Scenario

POST

```
/scenario/whale
```

Produces

```
Customer A
2000 events

Customer B
100 events

Customer C
100 events
```

Used for fairness testing.

Example:

```bash
curl -X POST http://localhost:28081/scenario/whale
```

---

## Mixed Scenario

POST

```
/scenario/mixed
```

Produces multiple customers with randomized event rates.

By default, the mixed scenario rotates across the explicitly supported event
types: `subscriber.created`, `subscriber.added_to_segment`, and
`subscriber.unsubscribed`.

When `GENERATOR_RANDOM_SEED` is set, the generator produces the same event IDs,
subscriber IDs, and timestamps for the same request sequence, which makes
benchmark runs repeatable.

Example:

```bash
curl -X POST http://localhost:28081/scenario/mixed
```

---

# Benchmark Scenarios

The following scenarios should be available.

## Small

```
5 customers

100 events
```

---

## Whale

```
1 customer

2000 events

2 normal customers

100 events each
```

---

# Success Criteria

Generator successfully submits all requested events to the notifier ingest API.

The notifier accepts the requests and persists the resulting delivery work in PostgreSQL.

---

# Future Enhancements

- Scheduled event generation
- CSV import
- Replay captured events
- Random payload generator
- Web UI
