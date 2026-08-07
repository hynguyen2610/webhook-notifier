# Multi-Instance Benchmark Report

- date: `2026-08-07T03:46:20Z`
- preset: `balanced`
- scenario: `two-whales-3500-two-non-whales-100`
- benchmark database: `webhook_notifier_benchmark`
- notifier worker count per instance: `4`
- notifier queue claim batch size: `32`
- notifier queue poll interval: `50ms`
- retries: disabled (`NOTIFIER_MAX_RETRY_ATTEMPTS=0`)
- receiver base URL: `http://127.0.0.1:28092`
- workload: `customer-a=3500`, `customer-b=3500`, `customer-c=100`, `customer-d=100`

This run preloads the PostgreSQL queue directly before starting the notifier instances. That means the result measures PostgreSQL-backed claiming, scheduling, worker execution, and local HTTP delivery, but not HTTP ingest cost.

## 1 instance

- start time: `2026-08-07T03:46:20.810Z`
- total jobs: `7200`
- total duration seconds: `13.194`
- jobs per second: `545.70`

- max oldest pending event age seconds: `13.411`

| Customer | Job Count | First Completion ms | Finish Completion ms | Early Share of First 20 |
| --- | ---: | ---: | ---: | ---: |
| `customer-a` | 3500 | 877.068 | 6777.976 | 1.000 |
| `customer-b` | 3500 | 6704.916 | 12857.218 | 0.000 |
| `customer-c` | 100 | 12769.242 | 13016.038 | 0.000 |
| `customer-d` | 100 | 12919.568 | 13194.087 | 0.000 |

## 2 instances

- start time: `2026-08-07T03:46:43.059Z`
- total jobs: `7200`
- total duration seconds: `9.137`
- jobs per second: `788.02`

- max oldest pending event age seconds: `9.298`

| Customer | Job Count | First Completion ms | Finish Completion ms | Early Share of First 20 |
| --- | ---: | ---: | ---: | ---: |
| `customer-a` | 3500 | 72.483 | 4548.916 | 1.000 |
| `customer-b` | 3500 | 4412.083 | 9041.264 | 0.000 |
| `customer-c` | 100 | 8827.419 | 9081.804 | 0.000 |
| `customer-d` | 100 | 8979.086 | 9136.851 | 0.000 |

## 4 instances

- start time: `2026-08-07T03:47:09.536Z`
- total jobs: `7200`
- total duration seconds: `8.553`
- jobs per second: `841.79`

- max oldest pending event age seconds: `8.631`

| Customer | Job Count | First Completion ms | Finish Completion ms | Early Share of First 20 |
| --- | ---: | ---: | ---: | ---: |
| `customer-a` | 3500 | 99.044 | 4246.451 | 1.000 |
| `customer-b` | 3500 | 3822.758 | 8394.203 | 0.000 |
| `customer-c` | 100 | 8131.559 | 8550.345 | 0.000 |
| `customer-d` | 100 | 8291.294 | 8553.243 | 0.000 |

