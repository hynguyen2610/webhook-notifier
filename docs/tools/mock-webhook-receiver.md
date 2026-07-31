# Mock Webhook Receiver Specification

**Version:** 1.0

---

# Purpose

The Mock Webhook Receiver simulates customer webhook endpoints.

It enables testing of:

- successful deliveries
- retries
- timeout handling
- rate limiting
- scheduler fairness
- benchmarking

This application is a testing utility and is not part of the production system.

---

# Goals

- Receive webhook POST requests.
- Return configurable HTTP responses.
- Simulate slow endpoints.
- Simulate temporary failures.
- Provide delivery statistics.

---

# Non Goals

The receiver will not:

- Validate payload schemas.
- Authenticate requests.
- Persist notifications.
- Implement business logic.

---

# High-Level Architecture

HTTP Server

↓

Scenario Engine

↓

Response Generator

↓

Statistics

---

# Endpoint

POST

```
/webhook/{customerId}
```

Request Body

```json
{
  "eventId": "...",
  "eventType": "...",
  "subscriberId": "...",
  "timestamp": "..."
}
```

---

# Response Modes

Each customer endpoint can be configured independently.

## Success

Returns

```
HTTP 200
```

---

## Client Error

Returns

```
HTTP 400
```

---

## Unauthorized

Returns

```
HTTP 401
```

---

## Server Error

Returns

```
HTTP 500
```

---

## Timeout

Sleeps for configured duration before responding.

Example

```
10 seconds
```

---

## Random Failure

Randomly returns

```
200

or

500
```

using configurable probability.

---

# Configuration API

Update Customer Behaviour

POST

```
/config/{customerId}
```

Request

```json
{
  "mode": "timeout",
  "delay": 5000
}
```

---

Supported Modes

| Mode | Description |
|--------|-------------|
| success | Always returns HTTP 200 |
| timeout | Sleeps before responding |
| error500 | Always returns HTTP 500 |
| error400 | Always returns HTTP 400 |
| unauthorized | Returns HTTP 401 |
| random | Random success/failure |

---

# Statistics API

GET

```
/stats
```

Returns

```json
{
  "received": 1200,
  "success": 1100,
  "failed": 100,
  "averageLatencyMs": 145
}
```

---

## Reset Statistics

POST

```
/stats/reset
```

---

# Logging

Every received request should log

- timestamp
- customer ID
- event ID
- HTTP status
- processing time

---

# Benchmark Support

The receiver should support configurable response latency.

Examples

```
10ms

100ms

500ms

2s
```

This allows benchmarking different customer behaviours.

---

# Future Enhancements

- Signature verification
- HTTPS support
- Payload validation
- Web dashboard
- Failure scripting
- Response templates