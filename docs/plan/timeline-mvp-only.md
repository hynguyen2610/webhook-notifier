# F-Company Webhook Assignment - Implementation Plan & Estimation (Historical)

> Historical note: this timeline reflects the original Kafka-first MVP plan
> before the project moved to the current PostgreSQL-backed queue design. Keep
> it as delivery history, not as the source of truth for today's architecture
> or setup steps.

> **Objective:** Deliver a functional MVP within **6 working days (5 hours/day)**. Production-ready features are intentionally deferred.

## Phase 1 - MVP (Assignment Scope)

### Day-by-Day Plan

| Day | Tasks | Estimated Hours |
|-----|-------|----------------:|
| **Day 1** | Project setup, dependency management, Kafka integration, project structure | **5** |
| **Day 2** | Implement Webhook Receiver API, payload validation, Kafka Producer | **5** |
| **Day 3** | Implement Notification Worker, Kafka Consumer, HTTP delivery to subscriber | **5** |
| **Day 4** | Build Mock Subscriber, failure simulation (timeout, HTTP 500, connection refused), retry mechanism | **5** |
| **Day 5** | End-to-end integration testing, logging improvements, bug fixes | **5** |
| **Day 6** | Documentation, architecture diagrams, deployment guide, final polishing and demo preparation | **5** |

### MVP Task Breakdown

| Task | Description | Est. (Hours) |
|------|-------------|-------------:|
| Project Setup | Initialize project, configuration, Docker, Kafka client | 3 |
| Architecture Design | Define event flow and project structure | 2 |
| Webhook Receiver | Receive and validate webhook requests | 4 |
| Kafka Producer | Publish webhook events | 2 |
| Notification Worker | Consume events and dispatch HTTP requests | 5 |
| Mock Subscriber | Simulate customer endpoint | 3 |
| Failure Simulation | Timeout, HTTP 500, connection refused | 2 |
| Retry Logic | Basic configurable retry policy | 3 |
| Logging | Structured logging | 2 |
| Integration Testing | Verify complete workflow | 2 |
| Documentation | README, diagrams, API usage | 2 |

| | **Total Estimated Effort** | **30 Hours** |

---

# Phase 2 - Production Readiness (Future Enhancements)

| Feature | Description | Est. (Hours) |
|---------|-------------|-------------:|
| Manual Kafka Offset Commit | Commit offsets after successful processing | 4 |
| Dead Letter Queue (DLQ) | Persist permanently failed deliveries | 5 |
| Idempotency | Prevent duplicate webhook processing | 6 |
| Exponential Backoff | Retry with configurable delay and jitter | 4 |
| Rate Limiting | Protect downstream endpoints | 4 |
| Metrics | Prometheus metrics | 4 |
| Distributed Tracing | OpenTelemetry & Jaeger | 6 |
| Health Checks | Readiness and liveness probes | 2 |
| Kubernetes Deployment | Deploy services to Kubernetes | 8 |
| Horizontal Scaling | Validate multi-worker deployment | 4 |
| CI/CD Pipeline | Automated build and deployment | 6 |
| Monitoring Dashboard | Grafana dashboards and alerting | 5 |

| | **Total Estimated Effort** | **58 Hours** |

---

# Summary

| Phase | Duration | Estimated Hours |
|-------|----------|----------------:|
| Phase 1 – MVP | **6 working days** | **30 hours** |
| Phase 2 – Production Readiness | ~2 weeks | **58 hours** |
| **Overall** | | **88 hours** |

## MVP Deliverables

- Webhook Receiver API
- Kafka Producer
- Notification Worker
- Mock Subscriber
- Retry mechanism
- Failure simulation
- Architecture diagrams
- README and deployment guide
