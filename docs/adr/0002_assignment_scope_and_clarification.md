# ADR-0002: Assignment Scope and Clarifications

- **Status:** Accepted
- **Date:** 2026-07-31
- **Decision Makers:** Candidate, Hiring Team (via email clarification)

## Context

During the implementation of the Webhook Notifier assignment, several implementation details were clarified through email to ensure the solution aligns with the expected MVP scope.

## Decisions

### 1. Assignment Scope

The implementation is limited to the **Webhook Notifier** component.

The following components may be mocked:

- Internal event producer
- Customer webhook endpoints

### 2. Event Ordering

Maintaining event ordering is **not required** for this assignment.

Out-of-order webhook delivery is considered acceptable.

### 3. Delivery Guarantee

The notifier should provide **at-least-once delivery**.

Duplicate webhook deliveries are acceptable.

Receivers are expected to handle duplicate events using the provided event identifier (idempotent processing).

### 4. Retry Strategy

Retry behavior is left to the implementation.

A standard retry strategy (e.g., exponential backoff with a configurable retry limit) is considered acceptable.

### 5. Fairness

Fairness means:

- Preventing starvation of low-volume customers.
- Minimizing latency for small customers when large ("whale") customers generate significant traffic.

Benchmark results are encouraged to demonstrate the fairness characteristics of the implementation.

### 6. Event Payload

The assignment only needs to support the **Subscriber** domain event.

The notifier is responsible for enriching incoming events with the customer's registered webhook endpoint(s).

### 7. Webhook Registration Source

Webhook registrations are assumed to have already been created through a separate **user-facing service**.

For this MVP, the Webhook Notifier will **read webhook registrations directly from PostgreSQL**.

Managing webhook registrations (CRUD operations) is **out of scope**.

### 8. Successful Delivery

Any HTTP **2xx** response is considered a successful delivery.

### 9. Submission

The completed solution may be submitted once all implementation work is finished.

Daily progress reports are not expected.

## Consequences

The implementation will:

- Focus exclusively on the Webhook Notifier.
- Use mocked producer and customer endpoints.
- Read webhook registrations from PostgreSQL.
- Ignore event ordering guarantees.
- Implement at-least-once delivery.
- Assume idempotent receivers.
- Implement a standard retry strategy.
- Include fairness considerations and benchmark results where appropriate.
- Exclude webhook registration management and other user-facing functionality.