# Worker Fairness Benchmark Scenarios

## Goal

Demonstrate that increasing worker count improves throughput while the
round-robin scheduler still lets smaller customers finish early even when two
whale customers dominate the queue.

## Worker Counts

Each fairness scenario runs with these worker counts:

- `1`
- `4`
- `8`

## Scenario 1

Name: `two-whales-100-two-normals-2`

Customer message counts:

- `customer-a`: `100` messages
- `customer-b`: `100` messages
- `customer-c`: `2` messages
- `customer-d`: `2` messages

Record for each worker count:

- total `jobs/sec`
- total duration
- per-customer first completion duration
- per-customer finish duration
- per-customer share of the first `20` completed jobs

## Scenario 2

Name: `two-whales-200000-two-normals-2`

Customer message counts:

- `customer-a`: `200000` messages
- `customer-b`: `200000` messages
- `customer-c`: `2` messages
- `customer-d`: `2` messages

Record for each worker count:

- total `jobs/sec`
- total duration
- per-customer first completion duration
- per-customer finish duration
- per-customer share of the first `20` completed jobs

## Benchmark Notes

- The fairness benchmark uses a synthetic in-process delivery step instead of
  real HTTP calls so the `200000`-message whale case stays practical to run
  locally.
- The main comparison targets are relative throughput by worker count and how
  quickly the two normal customers finish compared with the whales.
