# App Load Test Opt-In Checklist

## Goal

- [x] Add an opt-in mode that runs app-level load tests instead of the current scheduler-only benchmark path
- [x] Keep the current scheduler benchmark as the default mode
- [x] Make it clear from terminal and HTML output which mode was executed

## Mode Selection

- [x] Choose the opt-in trigger
- [x] Support a simple CLI flag such as `--mode app`
- [x] Support an environment-variable fallback if needed
- [x] Validate invalid mode values with a clear error message

## App-Level Test Scope

- [x] Run the real notifier worker flow instead of the synthetic scheduler-only worker harness
- [x] Decide whether the opt-in mode should target in-memory test doubles or PostgreSQL-backed queue flow first
- [x] Document exactly which app components are included in app mode
- [x] Document which components remain out of scope in app mode

## Fairness Scenarios

- [x] Reuse `two-whales-100-two-normals-2` in app mode
- [x] Reuse `two-whales-200000-two-normals-2` in app mode
- [x] Run each app-mode fairness scenario with `1`, `4`, and `8` workers

## Metrics

- [x] Record total duration for each worker-count run
- [x] Record total `jobs/sec` for each worker-count run
- [x] Record per-customer first completion duration
- [x] Record per-customer finish duration
- [x] Record per-customer share of the first completion window
- [x] Confirm the same fairness metrics are available in both scheduler mode and app mode where applicable

## Reporting

- [x] Print the selected mode near the top of the terminal output
- [x] Print the selected mode near the top of the HTML report
- [x] Keep the report file path output clickable in the terminal
- [x] Separate scheduler-mode and app-mode result sections clearly
- [x] Include notes about whether results are synthetic-harness or app-level

## Safety And Runtime Control

- [x] Make app mode opt-in so normal benchmark runs stay fast
- [x] Add guardrails for long-running app-mode scenarios
- [x] Add a way to skip the `200000` whale scenario when a faster smoke run is needed
- [x] Document expected runtime tradeoffs for scheduler mode versus app mode

## Verification

- [x] Confirm default run still uses scheduler mode
- [x] Confirm opt-in run switches to app mode
- [x] Confirm both modes generate valid HTML reports
- [x] Confirm worker-count comparisons remain readable in both modes
- [x] Confirm the generated load test report path is included in the response after test runs
