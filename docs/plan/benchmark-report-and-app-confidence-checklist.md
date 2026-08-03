# Benchmark Report And App Confidence Checklist

## Goal

- [ ] Make benchmark reports easier for customers and reviewers to interpret
- [ ] Strengthen confidence in app-mode fairness and scale conclusions
- [ ] Preserve a fast default scheduler-mode benchmark path

## Report Clarity

- [ ] Add a short “how to read this report” section near the top of each report
- [ ] Explain what `scheduler` mode includes and excludes
- [ ] Explain what `app` mode includes and excludes
- [ ] Add a short conclusion block summarizing the main fairness and throughput takeaway
- [ ] Make it obvious when the large fairness scenario was skipped
- [x] Split throughput benchmark content and fairness benchmark content into separate HTML tabs

## Fairness Metrics

- [ ] Review whether the current first-completion, finish-completion, and early-share metrics are sufficient
- [ ] Add a derived whale-vs-non-whale completion-gap metric
- [ ] Add a derived statement showing whether non-whale customers finished before whales
- [ ] Rename any metric labels that are technically correct but easy to misread
- [ ] Revisit the size of the early completion window and document why that window was chosen

## Scheduler vs App Evidence

- [ ] Make scheduler microbenchmark results visually distinct from fairness conclusions
- [x] Present throughput and fairness in separate tabs so the two result types are easier to scan
- [ ] Decide whether scheduler and fairness results should remain in one file or split into separate report files
- [ ] Ensure no chart or heading implies app-level confidence when the numbers are scheduler-only
- [ ] Ensure app-mode reports clearly distinguish in-memory app flow from a full end-to-end deployment

## App-Mode Confidence

- [ ] Investigate why app mode at low worker counts can delay non-whale first completions more than scheduler mode
- [ ] Determine whether queue claim order or poll batching is influencing fairness in app mode
- [ ] Add an explanation of the app-mode fairness pipeline: enqueue, claim, schedule, worker completion
- [ ] Decide whether the next app-mode step should be PostgreSQL-backed queue coverage
- [ ] Define what evidence is still needed before claiming app-level horizontal scale more confidently

## Scenarios

- [ ] Re-run the `two-whales-100-two-normals-2` scenario with both scheduler and app mode after report changes
- [ ] Re-run the `two-whales-200000-two-normals-2` scenario with both scheduler and app mode after report changes
- [ ] Compare worker counts `1`, `4`, and `8` consistently across both modes
- [ ] Capture at least one “fast smoke run” example and one “full run” example in the docs

## Runtime And Safety

- [ ] Document expected runtime for scheduler mode smoke runs
- [ ] Document expected runtime for app mode smoke runs
- [ ] Document expected runtime when the large fairness scenario is enabled
- [ ] Keep the large whale scenario opt-in or skippable for day-to-day development

## Verification

- [ ] Confirm a customer can explain the main result after reading only the report
- [ ] Confirm a reviewer can tell synthetic-harness evidence from app-level evidence
- [ ] Confirm benchmark output still prints clickable report paths
- [ ] Confirm the improved report remains readable on desktop and mobile
