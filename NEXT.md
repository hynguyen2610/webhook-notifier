# Next Change Steps

## Goal

- [ ] Clearly support these three subscriber event types across the system:
- [ ] `subscriber.created`
- [ ] `subscriber.added_to_segment`
- [ ] `subscriber.unsubscribed`

## Contract Decision

- [ ] Keep event type acceptance open-ended for any non-empty value
- [ ] Define `subscriber.created`, `subscriber.added_to_segment`, and `subscriber.unsubscribed` as explicitly supported event types in code and docs
- [ ] Confirm that existing example or benchmark values such as `subscriber.updated` and `subscriber.deleted` remain allowed but are not part of the explicitly supported set

## Notifier Changes

- [ ] Ensure notifier ingest accepts the three supported event types
- [ ] Ensure outbound delivery preserves the incoming event type unchanged
- [ ] Confirm retry, dead-letter, queue, and fairness behavior remain event-type agnostic

## Mock Event Generator Changes

- [ ] Ensure `POST /generate` can create `subscriber.created`
- [ ] Ensure `POST /generate` can create `subscriber.added_to_segment`
- [ ] Ensure `POST /generate` can create `subscriber.unsubscribed`
- [ ] Update generator defaults, scenarios, and examples so event types are intentional and aligned with the supported set

## Mock Receiver Changes

- [ ] Confirm receiver statistics still count deliveries correctly by event type
- [ ] Confirm last-event capture works correctly for all three supported event types

## Test Coverage

- [ ] Add unit tests for open-ended event-type acceptance behavior where useful
- [ ] Add notifier integration coverage for `subscriber.created`
- [ ] Add notifier integration coverage for `subscriber.added_to_segment`
- [ ] Add notifier integration coverage for `subscriber.unsubscribed`
- [ ] Add generator handler coverage for accepted supported event types
- [ ] Confirm receiver tests still verify per-type statistics behavior

## Documentation

- [ ] Update `README.md` so supported event types are explicit
- [ ] Update [docs/tools/mock-event-generator.md](/Users/hdnguyen/Documents/dev/go/webhook-notifier/docs/tools/mock-event-generator.md) examples to reflect the supported set
- [ ] Update [docs/plan/subscriber-event-type-expansion-checklist.md](/Users/hdnguyen/Documents/dev/go/webhook-notifier/docs/plan/subscriber-event-type-expansion-checklist.md) as implementation progresses
- [ ] Clarify that the system accepts open-ended non-empty event types while explicitly documenting the three supported types

## Verification

- [ ] Run focused package tests for notifier, generator, and receiver
- [ ] Verify end-to-end delivery succeeds for `subscriber.created`
- [ ] Verify end-to-end delivery succeeds for `subscriber.added_to_segment`
- [ ] Verify end-to-end delivery succeeds for `subscriber.unsubscribed`

## Exit Criteria

- [ ] The three supported event types are explicit in code, tests, and docs
- [ ] Local tooling can generate all three supported event types
- [ ] Automated tests protect the supported event-type behavior
