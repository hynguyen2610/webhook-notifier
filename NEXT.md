# Next Change Steps

## Goal

- [ ] Make the multi-instance load test finish quickly enough for normal local use and exit cleanly after all measurements are recorded

## Runtime Target

- [ ] Keep the default `balanced` preset under `1` minute on a typical local machine
- [ ] Keep the default workload only large enough to show throughput improvement and fairness behavior across `1`, `2`, and `4` instances
- [ ] Treat the runtime target as a dataset-sizing guideline, not as an early-stop condition for the benchmark
- [ ] Ensure the benchmark always completes all configured instance-count measurements before exiting

## Exit Behavior

- [ ] Make notifier shutdown progress visible after each measurement section is written
- [ ] Let the benchmark finish all instance measurements before exit
- [ ] Make the post-measurement shutdown path clear and graceful so the tool does not appear stuck after the benchmark is already complete

## Implementation Steps

- [ ] Tune the default `balanced` preset only by reducing workload size, not by adding a benchmark cutoff
- [ ] Improve notifier shutdown logging in `scripts/run-multi-instance-benchmark.sh` so users can see which process is still exiting
- [ ] Log which shutdown phase the script is in and which notifier process is still being waited on
- [ ] Preserve the existing report-writing behavior that prints report progress after each instance section is appended
- [ ] Keep the multi-instance comparison semantics unchanged while improving runtime ergonomics

## Verification

- [ ] Run the default `balanced` preset locally and confirm the full `1` / `2` / `4` comparison finishes in under `1` minute if practical
- [ ] Confirm the script exits promptly after the final report link is printed
- [ ] Confirm the report still contains the expected throughput and fairness sections for every instance count
