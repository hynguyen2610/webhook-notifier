# Customer Requirements Document (CRD)

**Project:** Reliable & Scalable Webhook Notification Service
**Version:** 0.1 (Draft)
**Status:** Draft – Subject to clarification from customer

---

# 1. Introduction

## 1.1 Purpose

This document captures the functional and non-functional requirements for building a scalable webhook notification service. It serves as the foundation for the system design, implementation, testing, and acceptance of the assignment.

Where requirements are ambiguous, reasonable assumptions are documented separately and may be updated after receiving clarification from the customer.

---

# 2. Project Objectives

The system shall:

* Receive subscriber-related events.
* Deliver webhook notifications to registered customer endpoints.
* Handle large numbers of events reliably.
* Ensure fair processing among customers.
* Support horizontal scalability.
* Be observable and easy to operate.
* Continue functioning even when customer endpoints experience failures.

---

# 3. Business Context

The notification service enables customers to receive real-time updates whenever subscriber-related events occur.

Example events include:

* Subscriber Created
* Subscriber Updated
* Subscriber Deleted
* Subscriber Unsubscribed

Each customer registers one or more webhook URLs where notifications will be delivered.

The notification service acts as an event delivery platform between the event source and customer applications.

---

# 4. Functional Requirements

## FR-1 Receive Subscriber Events

The system shall receive subscriber events from the event source.

Events are expected to be published into Kafka before being processed.

Each event shall contain sufficient information to generate a webhook notification.

---

## FR-2 Webhook Delivery

For every received event, the system shall:

* determine the registered webhook endpoints
* construct the notification payload
* deliver an HTTP POST request to each registered endpoint

---

## FR-3 Support Multiple Customers

The system shall support multiple independent customers.

Each customer may have:

* one or more webhook endpoints
* different event volumes
* different webhook response times

---

## FR-4 Retry Failed Deliveries

If webhook delivery fails, the system shall retry the request using an exponential backoff strategy.

Retryable failures include:

* timeout
* HTTP 5xx
* temporary network failures

Non-retryable failures (subject to clarification) may include:

* HTTP 400
* HTTP 401
* HTTP 403
* HTTP 404

---

## FR-5 Dead Letter Queue

After all retry attempts are exhausted, the notification shall be moved into a Dead Letter Queue (DLQ).

DLQ messages shall be retained for future investigation.

---

## FR-6 Fair Processing

The system shall prevent a single high-volume customer from monopolizing processing resources.

Each customer shall continue making progress even when another customer generates significantly more events.

A fair scheduling strategy shall be implemented.

Initial implementation will use:

* Round Robin scheduling

The scheduler shall be extensible to support:

* Weighted Round Robin
* Deficit Round Robin

without major architectural changes.

---

## FR-7 Rate Limiting

The system shall support per-customer rate limiting.

The objective is to:

* prevent excessive requests toward customer endpoints
* avoid resource monopolization
* improve overall system stability

Rate limits shall be configurable.

---

## FR-8 Configurable Worker Pool

The number of concurrent delivery workers shall be configurable.

The worker pool should allow throughput tuning without code changes.

---

## FR-9 Logging

The system shall produce structured logs including:

* event ID
* customer ID
* webhook URL
* retry count
* delivery status
* processing duration

---

## FR-10 Metrics

The system shall expose operational metrics including:

* received events
* delivered events
* failed deliveries
* retry count
* DLQ count
* request latency
* throughput

---

## FR-11 Health Check

The service shall expose a health endpoint for operational monitoring.

---

# 5. Non-Functional Requirements

## NFR-1 Reliability

The system should achieve at-least-once delivery.

Temporary failures should not result in message loss.

---

## NFR-2 Scalability

The system shall support horizontal scaling by deploying multiple notifier instances.

Kafka Consumer Groups shall distribute partitions across running instances.

No code changes should be required when scaling horizontally.

---

## NFR-3 Fairness

The system shall ensure that high-volume customers do not prevent smaller customers from receiving webhook deliveries.

---

## NFR-4 Availability

Failure of one customer endpoint shall not prevent processing of other customers.

---

## NFR-5 Performance

The system should support increasing throughput by:

* increasing worker count
* adding Kafka partitions
* adding notifier instances

---

## NFR-6 Observability

Operators shall be able to understand system health through:

* logs
* metrics
* health endpoints

---

## NFR-7 Configurability

Operational parameters shall be externally configurable.

Examples include:

* worker count
* retry attempts
* retry delay
* request timeout
* rate limits

---

## NFR-8 Selected tools and approach

Implement in Go language.
Architecture: Monolith
Spirit: Mimimal implementation that ensure the customer expectation, avoid unnecessary over-engineering
# 6. Constraints

The implementation is intended as a Proof of Concept.

External systems may be mocked where appropriate.

Examples include:

* subscriber event producer
* webhook endpoint receivers
* webhook registration repository

---

# 7. Out of Scope

The following capabilities are considered future enhancements and are not required for the initial implementation.

* Webhook registration management APIs
* User interface
* Authentication & authorization
* Replay API
* Circuit breaker
* Persistent webhook repository
* Webhook signature verification
* Delivery history dashboard

---

# 8. Assumptions

The following assumptions are made until clarified.

1. Subscriber events are already published into Kafka.

2. Webhook registrations already exist.

3. Webhook registration APIs are outside the assignment scope.

4. Customer webhook endpoints are external systems.

5. HTTP 2xx indicates successful delivery.

6. Failed webhook deliveries may be retried.

7. At-least-once delivery is acceptable.

8. Event ordering requirements are not specified.

9. Payload format is predefined or can be mocked.

---

# 9. Acceptance Criteria

The implementation will be considered successful if it demonstrates:

* Receiving events from Kafka.
* Delivering HTTP webhook requests.
* Retrying failed requests.
* Sending exhausted retries to DLQ.
* Fair processing across customers.
* Configurable worker concurrency.
* Per-customer rate limiting.
* Horizontal scalability using Kafka Consumer Groups.
* Structured logging.
* Operational metrics.
* Documentation describing architecture and design decisions.

---

# 10. Future Enhancements

The architecture should support future capabilities including:

* Weighted Round Robin scheduling
* Deficit Round Robin scheduling
* Circuit breaker
* Replay from DLQ
* Persistent webhook repository
* Dynamic configuration service
* Webhook signature verification
* Customer SLA tiers
* Benchmark automation
* Delivery analytics dashboard

---

# 11. Risks & Open Questions

The following items require customer clarification before implementation:

1. Is event ordering required per customer or subscriber?

2. Which HTTP response codes should be retried?

3. What is the expected retry policy?

4. What is the expected webhook payload format?

5. Are webhook endpoints assumed to be trusted?

6. Is webhook authentication required?

7. Are rate limits expected to be configurable per customer?

8. Is equal fairness sufficient, or are premium customer tiers expected?

9. Is benchmarking expected as part of the submission?

10. Should webhook registration be mocked or implemented?

11. Is agile/incremental delivery acceptable, or is only the final submission expected?

12. During the implementation period, may additional clarification questions be asked if new ambiguities are discovered?
