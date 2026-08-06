# Future Security & Observability Enhancements

> **Scope:** These enhancements remain mostly **out of scope for the MVP**. The
> current implementation already includes a few operational foundations, but the
> items below describe what would still be needed for stronger production-grade
> security and observability.

## Current Status Summary

The current codebase already has:

- Prometheus-compatible metrics exposed at `/metrics`
- structured key/value logging through `log/slog`
- health and operational inspection endpoints such as `/health`, `/stats`,
  `/registrations`, and `/dlq`

What it does **not** yet have is the broader production ecosystem around those
basics, such as secrets management, webhook signing, distributed tracing,
metrics scrape/deployment setup, dashboards, alerts, or network hardening.

## Future Enhancements

| Priority | Enhancement | Why | Current Status |
|----------|-------------|-----|----------------|
| High | Secret Management | Store database credentials, API keys, and webhook secrets in Kubernetes Secrets or a dedicated secret manager instead of plain environment variables. | Future Work |
| High | HMAC Webhook Signing | Sign outbound webhook payloads so receivers can verify authenticity and detect payload tampering. | Future Work |
| Medium | Structured Logging Standardization | Keep structured logs, but standardize JSON output, field conventions, and centralized aggregation for production operations. | Partial: basic structured logging exists |
| Medium | Prometheus/Grafana Operational Integration | Keep the `/metrics` endpoint, but add scrape configuration, dashboards, and production-ready metric conventions. | Partial: metrics endpoint already exists |
| Medium | Distributed Tracing | Instrument the notifier using OpenTelemetry to trace an event from ingest through scheduling, retries, and delivery. | Future Work |
| Medium | Kubernetes Network Policies | Restrict pod-to-pod communication to only required services, reducing attack surface within the cluster. | Future Work |
| Low | Mutual TLS (mTLS) | Encrypt and authenticate service-to-service communication within the production environment where required. | Future Work |
| Low | Delivery Rate Monitoring & Alerts | Alert on abnormal retry rates, elevated delivery latency, or increasing failure ratios before they impact customers. | Future Work |

---

## Why These Remain Future Work

The assignment centers on proving:

- reliable webhook delivery
- fairness across customers
- retry behavior
- worker concurrency
- horizontal scalability

Because of that scope, the implementation prioritized core delivery behavior
over production hardening.

That means the current MVP intentionally stops short of:

- secrets rotation and external secret managers
- cryptographic webhook authenticity guarantees
- production alerting and dashboard authoring
- distributed tracing across service boundaries
- Kubernetes-level network isolation controls

---

## Current MVP Operational Foundations

The current implementation already provides the following building blocks:

| Capability | Status | Evidence |
|------------|--------|----------|
| Retry with exponential backoff | ✅ | [internal/retry/backoff.go](../../internal/retry/backoff.go) |
| At-least-once delivery | ✅ | [internal/notifier/worker_delivery.go](../../internal/notifier/worker_delivery.go) |
| Fair round-robin scheduling | ✅ | [internal/scheduler/round_robin.go](../../internal/scheduler/round_robin.go) |
| Concurrent worker pool | ✅ | [internal/notifier/runtime.go](../../internal/notifier/runtime.go) |
| Graceful shutdown | ✅ | [internal/notifier/runtime.go](../../internal/notifier/runtime.go), [internal/mockreceiver/runtime.go](../../internal/mockreceiver/runtime.go) |
| Prometheus-compatible metrics endpoint | ✅ | [internal/metrics/notifier.go](../../internal/metrics/notifier.go), [internal/notifier/app.go](../../internal/notifier/app.go) |
| Structured key/value logging | ✅ | [internal/notifier/metrics.go](../../internal/notifier/metrics.go), [internal/notifier/worker_delivery.go](../../internal/notifier/worker_delivery.go) |
| Health and operational inspection endpoints | ✅ | [internal/notifier/app.go](../../internal/notifier/app.go), [internal/mockreceiver/app.go](../../internal/mockreceiver/app.go) |
| Unit and integration tests | ✅ | [README.md](../../README.md) |

---

## Production Evolution Path

A practical production hardening sequence could be:

1. move secrets out of plain runtime environment configuration into Kubernetes
   Secrets or a dedicated secret manager
2. standardize JSON structured logging and field conventions
3. add Prometheus scrape configuration plus Grafana dashboards
4. add alerting for retry storms, delivery latency regressions, and rising
   failure ratios
5. add HMAC signing for outbound webhook delivery
6. add distributed tracing for ingest, queueing, retry, and delivery flow
7. add Kubernetes Network Policies and mTLS where required by the deployment
   environment

---

## Notes

Audit logging is still intentionally **not** a primary focus here.

The notifier does not expose administrative mutation operations such as
creating, updating, or deleting webhook registrations. Its main responsibility
is receiving events, scheduling delivery work, and executing outbound delivery.
For that reason, operational metrics, structured logs, and tracing provide more
practical value than audit trails in the current service boundary.
