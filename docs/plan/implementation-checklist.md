# Webhook Notifier Implementation Checklist

This checklist translates the current requirement and planning documents into an execution-ready implementation plan for the assignment.

## Requirement Summary

- [x] Confirm architecture direction: Go monolith consuming Kafka events and delivering customer webhooks
- [x] Confirm scope includes notifier service plus two testing utilities:
  - [x] Mock Event Generator
  - [x] Mock Webhook Receiver
- [x] Confirm key behaviours:
  - [x] Fair per-customer scheduling
  - [x] Per-customer rate limiting
  - [x] Configurable worker pool
  - [x] Retry with exponential backoff
  - [x] DLQ after retry exhaustion
  - [x] Structured logs, metrics, and health endpoint
- [x] Confirm non-goals for initial implementation:
  - [x] Webhook registration management API
  - [x] UI
  - [x] Authentication and authorization
  - [x] Replay API
  - [x] Persistent webhook registration repository

## Assumptions To Build Against

- [ ] Subscriber events are already published to Kafka
- [ ] Webhook registrations can be mocked with static configuration or in-memory data
- [ ] HTTP 2xx means delivery success
- [ ] Retryable failures initially include timeout, HTTP 5xx, and temporary network errors
- [ ] HTTP 4xx responses are non-retryable unless clarified later
- [ ] At-least-once delivery is acceptable for the assignment
- [ ] Event ordering is not guaranteed unless later required
- [ ] Payload format can follow the mock event generator specification

## Phase 0: Project Setup

- [x] Initialize Go module and project structure
- [x] Add `cmd/` entrypoints for:
  - [x] notifier
  - [x] mock-event-generator
  - [x] mock-webhook-receiver
- [x] Add internal package layout for:
  - [x] configuration
  - [x] logging
  - [x] HTTP server
  - [x] Kafka
  - [x] scheduler
  - [ ] rate limiter
  - [ ] worker pool
  - [x] retry
  - [x] delivery
  - [ ] metrics
- [x] Add graceful shutdown wiring
- [x] Add configuration loading from environment variables or config file
- [x] Add structured logging baseline
- [ ] Add README bootstrap section with local run instructions

## Phase 1: Core Domain And Contracts

- [x] Define subscriber event model
- [x] Define webhook delivery job model
- [ ] Define retry metadata model
- [x] Define DLQ message model
- [x] Define customer webhook endpoint configuration model
- [ ] Define scheduler strategy interface
- [ ] Define rate limiter interface
- [ ] Define delivery client interface

## Phase 2: Mock Webhook Registration Source

- [x] Implement a simple mocked webhook registration source
- [x] Support one or more endpoints per customer
- [x] Make registrations externally configurable
- [ ] Add sample customer-to-webhook mapping for local testing

## Phase 3: Mock Event Generator

- [x] Implement HTTP server for generator utility
- [ ] Implement Kafka producer
- [x] Add `POST /generate`
- [x] Add `POST /generate/bulk`
- [x] Add `POST /scenario/whale`
- [x] Add `POST /scenario/mixed`
- [ ] Generate deterministic test data when seed is provided
- [ ] Validate generator requests and return useful errors
- [ ] Add configuration for broker, topic, customer count, and seed
- [ ] Document example requests for each scenario

## Phase 4: Mock Webhook Receiver

- [x] Implement HTTP server for receiver utility
- [x] Add `POST /webhook/{customerId}`
- [x] Add `POST /config/{customerId}`
- [x] Add `GET /stats`
- [x] Add `POST /stats/reset`
- [ ] Support modes:
  - [x] success
  - [x] timeout
  - [x] error500
  - [x] error400
  - [x] unauthorized
  - [x] random
- [x] Track received, success, failed, and average latency statistics
- [x] Allow per-customer response behavior

## Phase 5: Kafka Consumer

- [ ] Implement Kafka consumer group
- [ ] Subscribe to the event topic
- [ ] Deserialize incoming subscriber events
- [ ] Validate required event fields
- [ ] Resolve target webhook endpoints for each event
- [ ] Transform events into internal delivery jobs
- [ ] Handle consumer shutdown cleanly
- [ ] Define offset commit strategy aligned with at-least-once delivery

## Phase 6: Fair Scheduler

- [x] Implement per-customer queues
- [x] Implement queue manager
- [x] Implement round-robin scheduler
- [x] Ensure one noisy customer cannot fully starve others
- [ ] Expose scheduler extension point for:
  - [ ] weighted round robin
  - [ ] deficit round robin
- [ ] Add tests covering fairness under uneven traffic

## Phase 7: Per-Customer Rate Limiter

- [ ] Implement token bucket limiter per customer
- [ ] Make rate and burst configurable
- [ ] Ensure independent limits across customers
- [ ] Decide scheduler interaction when a customer is rate limited
- [ ] Add tests for burst handling and refill behaviour

## Phase 8: Worker Pool And Delivery

- [x] Implement configurable worker pool
- [x] Implement job dispatch channel
- [x] Implement HTTP POST webhook delivery
- [x] Add request timeout configuration
- [x] Capture delivery duration and response status
- [x] Treat HTTP 2xx as success
- [x] Classify retryable vs non-retryable failures
- [x] Preserve customer isolation when one endpoint is slow or failing

## Phase 9: Retry And DLQ

- [x] Implement retry manager
- [x] Implement exponential backoff policy
- [x] Make retry attempts and delays configurable
- [x] Requeue retryable jobs after delay
- [ ] Publish exhausted jobs to DLQ topic
- [x] Include failure reason and retry count in DLQ message
- [ ] Add tests for retry transitions and DLQ routing

## Phase 10: Observability

- [x] Add structured logs containing:
  - [x] event ID
  - [x] customer ID
  - [x] webhook URL
  - [x] retry count
  - [x] delivery status
  - [x] processing duration
- [ ] Add metrics for:
  - [ ] received events
  - [ ] delivered events
  - [ ] failed deliveries
  - [ ] retry count
  - [ ] DLQ count
  - [ ] request latency
  - [ ] throughput
- [x] Add `GET /health`
- [ ] Add `GET /metrics`

## Phase 11: Packaging And Local Environment

- [ ] Add `Dockerfile` for notifier
- [ ] Add local `docker-compose.yml` for Kafka and supporting services
- [ ] Add runnable local stack for notifier, generator, and receiver
- [ ] Add sample environment files
- [ ] Add Mermaid diagram references or generated architecture images to README

## Phase 12: Kubernetes Deployment

- [ ] Add manifests for:
  - [ ] namespace
  - [ ] ConfigMaps
  - [ ] Secrets if needed
  - [ ] notifier deployment and service
  - [ ] generator deployment and service
  - [ ] receiver deployment and service
  - [ ] Kafka dependencies or integration notes
- [ ] Ensure notifier supports horizontal scaling with Kafka consumer groups
- [ ] Add deployment instructions
- [ ] Optionally add HPA placeholders

## Phase 13: Testing

- [ ] Add unit tests for domain models and validation
- [ ] Add unit tests for scheduler behaviour
- [ ] Add unit tests for rate limiter behaviour
- [ ] Add unit tests for retry classification and backoff
- [ ] Add integration tests for end-to-end happy path
- [ ] Add integration tests for retry then success
- [ ] Add integration tests for retry exhaustion to DLQ
- [ ] Add integration tests for whale customer fairness scenario
- [ ] Add failure tests for slow or unavailable receiver

## Phase 14: Benchmark And Validation

- [ ] Run small scenario benchmark
- [ ] Run high-volume multi-customer benchmark
- [ ] Run whale-customer fairness benchmark
- [ ] Measure throughput, latency, retries, and DLQ volume
- [ ] Validate smaller customers still make progress during whale scenario
- [ ] Document benchmark findings and limitations

## Phase 15: Documentation And Submission Readiness

- [ ] Finalize README
- [ ] Document architecture and component responsibilities
- [ ] Document API contracts for generator and receiver
- [ ] Document configuration variables
- [ ] Document retry policy and fairness strategy
- [ ] Document trade-offs and out-of-scope items
- [ ] Document known risks and future enhancements

## Open Questions To Resolve

- [ ] Is ordering required per customer or subscriber?
- [ ] Which exact HTTP response codes should be retried?
- [ ] What retry count, base delay, and max delay are expected?
- [ ] What exact webhook payload format is expected?
- [ ] Should rate limits be globally defaulted or configured per customer?
- [ ] Is benchmark evidence required in the final submission?
- [ ] Should webhook registrations remain mocked, or does the reviewer expect a simple persistence layer?
- [ ] Is equal fairness enough, or should future premium-tier support influence current interfaces?

## Suggested Delivery Milestones

- [x] Milestone 1: project bootstrap and local app startup
- [x] Milestone 2: mock utilities working independently
- [ ] Milestone 3: notifier consumes Kafka and delivers basic webhooks
- [ ] Milestone 4: fairness, rate limiting, and worker pool complete
- [ ] Milestone 5: retry, DLQ, logging, metrics, and health complete
- [ ] Milestone 6: Docker, Kubernetes manifests, and documentation complete
- [ ] Milestone 7: benchmark validation and final cleanup complete
