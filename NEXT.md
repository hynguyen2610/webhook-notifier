# Next Change Steps

## Goal

- [x] Clean up legacy or drifted event-type and benchmark references so code, scripts, and docs match the current supported contract

## Legacy Benchmark Preset Review

- [x] Decide whether the `legacy-medium` preset in `scripts/run-multi-instance-benchmark.sh` should be kept, renamed, or removed
- [x] If the preset is kept, document why it remains intentionally legacy
- [x] If the preset is renamed or removed, update `README.md` and `docs/spec/multi-instance-load-test.md`

## Event Type Drift Cleanup

- [x] Review benchmark and load-test scripts that still hard-code `subscriber.updated`
- [x] Decide whether those script event types should switch to one of the explicitly supported event types
- [x] If benchmark-only event types remain, document clearly that they are allowed but not part of the explicitly supported set
- [x] Update scripts to avoid accidental drift between documented supported event types and generated benchmark traffic

## Code And Test Consistency

- [x] Review remaining raw event-type string literals in tests and helper code
- [x] Replace raw literals with shared event-type constants where that improves consistency
- [x] Keep intentionally non-supported example values explicit where they are useful to test open-ended acceptance

## Documentation Cleanup

- [x] Make sure benchmark and load-test docs do not accidentally imply `subscriber.updated` is a supported primary example
- [x] Make sure docs consistently distinguish between explicitly supported event types and merely allowed event types

## Verification

- [x] Run focused tests for packages affected by cleanup changes
- [x] Recheck benchmark and load-test docs and scripts for event-type consistency after cleanup
