# Next Change Steps

## Goal

- [ ] Review overall code quality with a focus on test coverage decisions and deprecated or obsolete code paths

## Test Coverage Review

- [ ] Review whether critical delivery, retry, queue, registration, and HTTP ingestion paths are covered by tests
- [ ] Identify important behavior that is still untested
- [ ] Decide whether missing coverage should be added as unit tests, integration tests, or left uncovered with rationale
- [ ] Confirm integration tests cover the main end-to-end success, retry, timeout, and dead-letter scenarios
- [ ] Confirm unit tests exist where they verify branch logic more clearly than integration tests alone
- [ ] Review whether current tests are readable, stable, and focused on behavior instead of implementation details
- [ ] Document gaps that are high risk for regression
- [ ] Avoid treating 100% line coverage as a goal by itself; prefer strong coverage on high-risk paths

## Deprecated Or Obsolete Code Review

- [ ] Check for deprecated standard library APIs, dependencies, or patterns in current use
- [ ] Check for unused or effectively obsolete internal code after recent refactors
- [ ] Check for scripts, docs, or test helpers that no longer match the current system behavior
- [ ] Record each deprecated or obsolete item with a recommendation to remove, replace, or keep temporarily

## Verification

- [ ] Summarize whether current automated tests are sufficient for the most important system behaviors
- [ ] Summarize whether any deprecated or obsolete code needs immediate follow-up
