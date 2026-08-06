# Next Change Steps

## Goal

- [x] Clearly support these three subscriber event types across the system:
- [x] `subscriber.created`
- [x] `subscriber.added_to_segment`
- [x] `subscriber.unsubscribed`

## Contract Decision

- [x] Keep event type acceptance open-ended for any non-empty value
- [x] Define `subscriber.created`, `subscriber.added_to_segment`, and `subscriber.unsubscribed` as explicitly supported event types in code and docs
- [x] Confirm that existing example or benchmark values such as `subscriber.updated` and `subscriber.deleted` remain allowed but are not part of the explicitly supported set

## Notifier Changes

- [x] Ensure notifier ingest accepts the three supported event types
- [x] Ensure outbound delivery preserves the incoming event type unchanged
- [x] Confirm retry, dead-letter, queue, and fairness behavior remain event-type agnostic

## Mock Event Generator Changes

- [x] Ensure `POST /generate` can create `subscriber.created`
- [x] Ensure `POST /generate` can create `subscriber.added_to_segment`
- [x] Ensure `POST /generate` can create `subscriber.unsubscribed`
- [x] Update generator defaults, scenarios, and examples so event types are intentional and aligned with the supported set

## Mock Receiver Changes

- [x] Confirm receiver statistics still count deliveries correctly by event type
- [x] Confirm last-event capture works correctly for all three supported event types

## Test Coverage

- [x] Add unit tests for open-ended event-type acceptance behavior where useful
- [x] Add notifier integration coverage for `subscriber.created`
- [x] Add notifier integration coverage for `subscriber.added_to_segment`
- [x] Add notifier integration coverage for `subscriber.unsubscribed`
- [x] Add generator handler coverage for accepted supported event types
- [x] Confirm receiver tests still verify per-type statistics behavior

## Documentation

- [x] Update `README.md` so supported event types are explicit
- [x] Update [docs/tools/mock-event-generator.md](/Users/hdnguyen/Documents/dev/go/webhook-notifier/docs/tools/mock-event-generator.md) examples to reflect the supported set
- [x] Update [docs/plan/subscriber-event-type-expansion-checklist.md](/Users/hdnguyen/Documents/dev/go/webhook-notifier/docs/plan/subscriber-event-type-expansion-checklist.md) as implementation progresses
- [x] Clarify that the system accepts open-ended non-empty event types while explicitly documenting the three supported types

## Verification

- [x] Run focused package tests for notifier, generator, and receiver
- [x] Verify end-to-end delivery succeeds for `subscriber.created`
- [x] Verify end-to-end delivery succeeds for `subscriber.added_to_segment`
- [x] Verify end-to-end delivery succeeds for `subscriber.unsubscribed`

## Exit Criteria

- [x] The three supported event types are explicit in code, tests, and docs
- [x] Local tooling can generate all three supported event types
- [x] Automated tests protect the supported event-type behavior
