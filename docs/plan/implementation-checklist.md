# Webhook Notifier Implementation Checklist

This checklist translates the current requirement and planning documents into an execution-ready implementation plan for the assignment.

## Existing Kubernetes Environment

- [x] Kafka is already running in Kubernetes:
  - [x] Namespace: `default`
  - [x] Service: `kafka-service`
  - [x] Cluster DNS: `kafka-service.default.svc.cluster.local:9092`
- [x] Observability stack is already running in Kubernetes:
  - [x] Namespace: `monitoring`
  - [x] Prometheus service: `prometheus.monitoring.svc.cluster.local:9090`
  - [x] Grafana service: `grafana.monitoring.svc.cluster.local:3000`
  - [x] Kafka exporter service: `kafka-exporter.monitoring.svc.cluster.local:9308`
- [ ] Add notifier deployment annotations or scrape configuration so Prometheus can collect `/metrics`

## Requirement Summary

- [x] Confirm architecture direction: Go monolith consuming Kafka events and delivering customer webhooks
- [x] Confirm assignment implementation scope is the notifier service itself
- [x] Confirm supporting test utilities may be mocked for local development and demonstration:
  - [x] Mock Event Generator
  - [x] Mock Webhook Receiver
- [x] Confirm key behaviours:
  - [x] Fair per-customer scheduling
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

- [x] Subscriber events are already published to Kafka
- [x] Webhook registrations are read directly from PostgreSQL for the MVP
- [x] HTTP 2xx means delivery success
- [x] Retryable failures initially include timeout, HTTP 5xx, and temporary network errors
- [ ] HTTP 4xx responses are non-retryable unless clarified later
- [x] At-least-once delivery is acceptable for the assignment
- [x] Event ordering is not required for this assignment
- [x] Payload format only needs to support the Subscriber domain event
- [x] Receivers are expected to handle duplicate event IDs idempotently

## Step 0: Project Setup

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

## Step 1: Core Domain And Contracts

- [x] Define subscriber event model
- [x] Define webhook delivery job model
- [ ] Define retry metadata model
- [x] Define DLQ message model
- [x] Define customer webhook endpoint configuration model
- [ ] Define scheduler strategy interface
- [ ] Define rate limiter interface
- [ ] Define delivery client interface

## Step 2: Webhook Registration Source

- [x] Add PostgreSQL-backed registration reads behind a registry interface
- [x] Define webhook registration table access pattern and query contract
- [x] Make PostgreSQL-backed registration reads the only notifier runtime path

## Step 3: Mock Event Generator

- [x] Implement HTTP server for generator utility
- [x] Implement Kafka producer
- [x] Add `POST /generate`
- [x] Add `POST /generate/bulk`
- [x] Add `POST /scenario/whale`
- [x] Add `POST /scenario/mixed`
- [ ] Generate deterministic test data when seed is provided
- [ ] Validate generator requests and return useful errors
- [ ] Add configuration for broker, topic, customer count, and seed
- [ ] Document example requests for each scenario

## Step 4: Mock Webhook Receiver

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

## Step 5: Kafka Consumer

- [x] Implement Kafka consumer group
- [x] Subscribe to the event topic
- [x] Deserialize incoming subscriber events
- [x] Validate required event fields
- [x] Resolve target webhook endpoints for each event
- [x] Transform events into internal delivery jobs
- [x] Handle consumer shutdown cleanly
- [x] Define offset commit strategy aligned with at-least-once delivery

## Step 6: Fair Scheduler

- [x] Implement per-customer queues
- [x] Implement queue manager
- [x] Implement round-robin scheduler
- [x] Ensure one noisy customer cannot fully starve others
- [ ] Prove fairness with an uneven multi-customer benchmark
- [ ] Add tests covering fairness under uneven traffic

## Step 7: Worker Pool And Delivery

- [x] Implement configurable worker pool
- [x] Implement job dispatch channel
- [x] Implement HTTP POST webhook delivery
- [x] Add request timeout configuration
- [x] Capture delivery duration and response status
- [x] Treat HTTP 2xx as success
- [x] Classify retryable vs non-retryable failures
- [x] Preserve customer isolation when one endpoint is slow or failing

## Step 8: Retry And DLQ

- [x] Implement retry manager
- [x] Implement exponential backoff policy
- [x] Make retry attempts and delays configurable
- [x] Requeue retryable jobs after delay
- [x] Publish exhausted jobs to DLQ topic
- [x] Include failure reason and retry count in DLQ message
- [ ] Add tests for retry transitions and DLQ routing

## Step 9: Observability

- [x] Add structured logs containing:
  - [x] event ID
  - [x] customer ID
  - [x] webhook URL
  - [x] retry count
  - [x] delivery status
  - [x] processing duration
- [x] Add metrics for:
  - [x] received events
  - [x] delivered events
  - [x] failed deliveries
  - [x] retry count
  - [x] DLQ count
  - [x] request latency
  - [ ] throughput
- [x] Add `GET /health`
- [x] Add `GET /metrics`

## Step 10: Packaging And Local Environment

- [ ] Add `Dockerfile` for notifier
- [ ] Add local `docker-compose.yml` for Kafka and supporting services
- [ ] Add runnable local stack for notifier, generator, and receiver
- [ ] Add sample environment files
- [ ] Add Mermaid diagram references or generated architecture images to README

## Step 11: Kubernetes Deployment

- [ ] Add manifests for:
  - [ ] namespace
  - [ ] ConfigMaps
  - [ ] Secrets if needed
  - [ ] notifier deployment and service
  - [ ] generator deployment and service
  - [ ] receiver deployment and service
  - [x] Kafka dependencies or integration notes
- [x] Ensure notifier supports horizontal scaling with Kafka consumer groups
- [ ] Add deployment instructions
- [ ] Optionally add HPA placeholders

## Step 12: Testing

- [ ] Add unit tests for domain models and validation
- [ ] Add unit tests for scheduler behaviour
- [ ] Add unit tests for retry classification and backoff
- [ ] Add integration tests for end-to-end happy path
- [ ] Add integration tests for retry then success
- [ ] Add integration tests for retry exhaustion to DLQ
- [ ] Add integration tests for whale customer fairness scenario
- [ ] Add failure tests for slow or unavailable receiver

## Step 13: Benchmark And Validation

- [ ] Run small scenario benchmark
- [ ] Run high-volume multi-customer benchmark
- [ ] Run whale-customer fairness benchmark
- [ ] Measure throughput, latency, retries, and DLQ volume
- [ ] Validate smaller customers still make progress during whale scenario
- [ ] Document benchmark findings and limitations

## Step 14: Documentation And Submission Readiness

- [ ] Finalize README
- [ ] Document architecture and component responsibilities
- [ ] Document API contracts for generator and receiver
- [ ] Document configuration variables
- [ ] Document retry policy and fairness strategy
- [ ] Document trade-offs and out-of-scope items
- [ ] Document known risks and future enhancements

## Open Questions To Resolve

- [ ] Which exact HTTP response codes should be retried besides timeout and HTTP 5xx?
- [ ] What retry count, base delay, and max delay are expected?
- [ ] What exact outbound webhook payload format is expected?
- [ ] What PostgreSQL schema or table contract should the notifier read for webhook registrations?
- [ ] Is equal fairness enough, or should future premium-tier support influence current interfaces?

## Suggested Delivery Milestones

- [x] Milestone 1: project bootstrap and local app startup
- [x] Milestone 2: mock utilities working independently
- [x] Milestone 3: notifier consumes Kafka and delivers basic webhooks
- [ ] Milestone 4: fairness validation is complete
- [ ] Milestone 5: retry, DLQ, logging, metrics, and health complete
- [ ] Milestone 6: Docker, Kubernetes manifests, and documentation complete
- [ ] Milestone 7: benchmark validation and final cleanup complete
