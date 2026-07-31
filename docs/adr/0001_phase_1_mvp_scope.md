# ADR 0001: Build To Phase 1 MVP Scope First

- Status: Accepted
- Date: 2026-07-31
- Deciders: Project implementation team

## Context

The repository contains two planning views:

- [estimated-timeline.md](../plan/estimated-timeline.md), which describes a broader multi-phase implementation
- [timeline-mvp-only.md](../plan/timeline-mvp-only.md), which narrows the assignment to a 6-day MVP

The current codebase already includes the core delivery path plus a small amount of functionality that the MVP-only timeline lists under later production-readiness work.

Without an explicit decision record, it is easy to lose track of whether the team is:

- building only the assignment MVP
- partially implementing future-ready features
- changing the project scope unintentionally

## Decision

The project will be implemented with **Phase 1 MVP as the primary delivery target**.

The default build priority is:

1. Webhook event intake
2. Kafka publish and consume flow
3. Notification worker delivery
4. Mock subscriber behavior and failure simulation
5. Basic retry handling
6. End-to-end validation
7. Documentation and demo readiness

The following items are considered **in scope for the MVP**:

- notifier application bootstrap
- mock event generator
- mock webhook receiver
- Kafka producer integration
- Kafka consumer integration
- worker-based webhook delivery
- structured logging
- basic retry behavior
- health endpoint

The following items are considered **not required to advance the next implementation steps**, even if some early code already exists for them:

- production-grade DLQ workflows
- full observability rollout and dashboards
- rate limiting
- advanced fairness strategies beyond the current simple scheduler
- Kubernetes polish and autoscaling
- tracing
- CI/CD automation

## Early Implementations Already Present

The codebase already contains a few capabilities that are closer to later-phase work:

- Prometheus-compatible `/metrics`
- Kafka-backed DLQ publishing
- consumer-group based Kafka reader setup

These are allowed to remain in the codebase because they do not block MVP delivery and they align with the existing Kubernetes environment. However, they must be treated as **supporting enhancements**, not as signals to expand the active delivery scope.

## Consequences

### Positive

- The team keeps momentum on the assignment-critical path.
- The repository has a clear answer to "are we building MVP only?"
- Existing early infrastructure work can remain without forcing more scope expansion.
- Future contributors can distinguish between "implemented early" and "required now".

### Trade-Offs

- Some code will exist before it becomes a priority to harden or validate fully.
- The repository may temporarily contain a mix of MVP-complete areas and future-facing scaffolding.
- Documentation must clearly state which features are primary versus opportunistic.

## Guidance For Next Steps

The next implementation steps should focus on:

1. making Kafka the clearly documented default runtime flow
2. separating Kafka event payloads from outbound webhook payloads
3. validating the full generator -> Kafka -> notifier -> receiver flow
4. tightening README and deployment instructions for the existing Kubernetes environment

The next implementation steps should avoid spending time on:

1. rate limiting
2. advanced scheduler policies
3. dashboard authoring
4. tracing
5. autoscaling and deployment polish beyond what is needed for demonstration

## Related Files

- [timeline-mvp-only.md](../plan/timeline-mvp-only.md)
- [implementation-checklist.md](../plan/implementation-checklist.md)
- [internal/notifier/app.go](../../internal/notifier/app.go)
- [internal/mockgenerator/app.go](../../internal/mockgenerator/app.go)
- [internal/mockreceiver/app.go](../../internal/mockreceiver/app.go)
