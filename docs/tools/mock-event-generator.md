# Mock Event Generator Specification

**Version:** 1.0

---

# Purpose

The Mock Event Generator simulates F-Company publishing subscriber events into Kafka.

It is intended solely for local development, integration testing, benchmarking, and demonstration.

It is **not** part of the production notification system.

---

# Goals

- Generate realistic webhook events.
- Produce configurable event volumes.
- Simulate whale customers.
- Support repeatable benchmark scenarios.
- Eliminate dependency on F-Company.

---

# Non Goals

The generator will not:

- Validate business rules.
- Persist generated events.
- Simulate webhook delivery.
- Manage webhook registrations.

---

# High-Level Architecture

REST API

↓

Event Generator

↓

Kafka Producer

↓

Kafka Topic

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

---

# Configuration

| Property | Description |
|------------|-------------|
| Kafka Broker | Kafka bootstrap server |
| Topic | Event topic |
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
  "eventType": "subscriber.created",
  "count": 100
}
```

Response

```json
{
  "generated": 100
}
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

---

## Whale Scenario

POST

```
/scenario/whale
```

Produces

```
Customer A
100000 events

Customer B
100 events

Customer C
100 events
```

Used for fairness testing.

---

## Mixed Scenario

POST

```
/scenario/mixed
```

Produces multiple customers with randomized event rates.

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

100000 events

20 normal customers

500 events each
```

---

# Success Criteria

Generator successfully publishes all requested events into Kafka.

No delivery guarantees are required beyond Kafka producer acknowledgements.

---

# Future Enhancements

- Scheduled event generation
- CSV import
- Replay captured events
- Random payload generator
- Web UI