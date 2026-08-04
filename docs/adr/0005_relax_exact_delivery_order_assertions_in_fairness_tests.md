# ADR 0005: Relax Exact Delivery Order Assertions In Fairness Tests

- Status: Accepted
- Date: 2026-08-04
- Deciders: Project implementation team

## Context

The notifier currently aims to provide:

- at-least-once delivery
- fair progress across customers
- PostgreSQL-backed queue claiming
- round-robin scheduling within each notifier instance

The project explicitly does **not** require strict event ordering as part of the
current assignment scope. That decision is already recorded in
[ADR 0002](./0002_assignment_scope_and_clarification.md).

Even with that scope decision, some fairness tests still assert an exact early
delivery sequence such as:

- `customer-a`
- `customer-b`
- `customer-c`
- `customer-a`
- `customer-b`
- `customer-c`

Those assertions are stronger than the current implementation contract.

The current pipeline includes:

1. enqueue order into the queue
2. queue claim order
3. scheduler handoff timing
4. worker execution timing

Because those stages are concurrent, especially under `go test -race` and CI
runtime variance, exact early delivery order can vary even when the smaller
customers still make meaningful progress before the whale customer fully drains
the queue.

As a result, tests that require exact first-N ordering are brittle and can fail
without proving a real correctness regression.

## Decision

For the current implementation phase, fairness tests will **not** require exact
delivery order guarantees.

Instead, fairness tests should validate the weaker property that the system
currently intends to guarantee:

- low-volume customers make observable progress early
- one noisy customer does not fully starve other active customers
- fairness is evaluated as progress behavior, not strict sequence identity

Until the project intentionally implements and documents stronger ordering
guarantees, tests should avoid asserting exact first-N delivery sequences across
customers.

## Decision Drivers

- the assignment scope does not require strict ordering
- the current scheduler is fairness-oriented, not order-guaranteeing
- queue claim timing and goroutine scheduling make exact order brittle in CI
- flaky fairness tests reduce trust in the test suite
- a weaker progress-based assertion better matches the real contract today

## Consequences

### Positive

- fairness tests become more stable in local runs and GitHub Actions
- the test suite better reflects the real current behavior
- failures are more likely to signal actual starvation or fairness regressions
- the project avoids overstating an ordering guarantee that does not exist yet

### Trade-Offs

- tests will no longer prove exact alternation between customers
- some subtle order-shape regressions may go unnoticed until stronger ordering
  work is implemented
- future ordering guarantees will require new tests and likely a more explicit
  scheduler or queue-claim contract

## Implementation Guidance

When updating fairness tests:

1. replace exact sequence assertions with progress-based checks
2. prefer assertions such as:
   - non-whale customers appear within an early completion window
   - the whale customer does not exclusively own the whole early window
   - smaller customers begin completing work before the whale fully drains
3. keep exact-order assertions only where the implementation contract truly
   guarantees them in a deterministic single-component scope

## Out Of Scope For This ADR

This ADR does not define:

- a future strict ordering policy
- weighted or prioritized fairness strategies
- customer-aware PostgreSQL claim ordering
- changes to the current scheduler algorithm

Those belong to future implementation work if stronger delivery ordering becomes
a project goal.

## Related Files

- [ADR 0002](./0002_assignment_scope_and_clarification.md)
- [internal/notifier/fairness_integration_test.go](../../internal/notifier/fairness_integration_test.go)
- [internal/notifier/postgres_fairness_integration_test.go](../../internal/notifier/postgres_fairness_integration_test.go)
- [internal/scheduler/round_robin.go](../../internal/scheduler/round_robin.go)
