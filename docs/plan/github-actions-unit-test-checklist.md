# GitHub Actions Unit Test Checklist

This checklist captures the work needed to run Go unit tests automatically in GitHub whenever code is pushed or a pull request is opened or updated.

## Goal

- [x] Run unit tests automatically on `push`
- [x] Run unit tests automatically on `pull_request`
- [x] Fail the workflow when tests fail
- [x] Keep the workflow fast and easy to maintain

## Step 1: Create The Workflow File

- [x] Add `.github/workflows/unit-tests.yml`
- [x] Name the workflow clearly, for example `Unit Tests`
- [ ] Configure workflow triggers:
  - [x] `push`
  - [x] `pull_request`

## Step 2: Set Up The Runner

- [x] Use `ubuntu-latest`
- [x] Check out the repository with `actions/checkout`
- [x] Set up Go with `actions/setup-go`
- [x] Pin the Go version from `go.mod` or agree on a specific CI version

## Step 3: Install And Cache Dependencies

- [x] Enable Go module caching through `actions/setup-go`
- [x] Verify the workflow can download dependencies with `go mod download` if needed
- [x] Keep the dependency step minimal if `go test` already handles it well enough

## Step 4: Run Unit Tests

- [x] Run targeted unit tests or `go test ./...`
- [x] Decide whether integration-style packages should be excluded from this workflow
- [ ] Add `-count=1` if cached test results should be avoided in CI
- [x] Make sure the workflow exits non-zero on any failing test

## Step 5: Validate Repository Fit

- [x] Confirm the workflow does not require local-only services such as Kafka or PostgreSQL for unit-test-only execution
- [x] Separate true unit tests from environment-dependent tests if needed
- [x] Ensure new tests added later follow the same CI-safe pattern

## Step 6: Verify On GitHub

- [ ] Push the workflow branch to GitHub
- [ ] Confirm the workflow runs after push
- [ ] Open or update a pull request
- [ ] Confirm the workflow runs for the pull request
- [ ] Confirm logs clearly show which test command was executed

## Step 7: Optional Improvements

- [ ] Add branch filters if only selected branches should run on push
- [ ] Add status badge to `README.md`
- [ ] Split lint and unit tests into separate workflows if CI grows
- [ ] Add matrix testing for multiple Go versions if compatibility matters

## Suggested First Version

```yaml
name: Unit Tests

on:
  push:
  pull_request:

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - name: Check out repository
        uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true

      - name: Run unit tests
        run: go test ./...
```

## Open Decisions

- [x] Should CI run all Go tests or only unit-test packages?
- [ ] Do we want branch filters for `push`, such as only `main` and feature branches?
- [ ] Do we want a separate workflow later for lint, formatting, or integration tests?
