# Multi-Instance Benchmark Report

- date: `2026-08-03T23:53:43Z`
- scenario: `two-whales-5000-two-non-whales-100`
- benchmark database: `webhook_notifier_benchmark`
- notifier worker count per instance: `4`
- notifier queue claim batch size: `32`
- notifier queue poll interval: `50ms`
- retries: disabled (`NOTIFIER_MAX_RETRY_ATTEMPTS=0`)
- receiver base URL: `http://127.0.0.1:28092`
- workload: `customer-a=5000`, `customer-b=5000`, `customer-c=100`, `customer-d=100`

This run preloads the PostgreSQL queue directly before starting the notifier instances. That means the result measures PostgreSQL-backed claiming, scheduling, worker execution, and local HTTP delivery, but not HTTP ingest cost.

## 1 instance

- start time: `2026-08-03T23:53:43.6NZ`
- total jobs: `10200`
- total duration seconds: `43216.963`
- jobs per second: `0.24`

| Customer | Job Count | First Completion ms | Finish Completion ms | Early Share of First 20 |
| --- | ---: | ---: | ---: | ---: |
| `customer-a` | 5000 | 43200803.546 | 43208735.105 | 1.000 |
| `customer-b` | 5000 | 43208717.897 | 43216623.693 | 0.000 |
| `customer-c` | 100 | 43216596.595 | 43216782.380 | 0.000 |
| `customer-d` | 100 | 43216744.738 | 43216962.951 | 0.000 |

## 2 instances

- start time: `2026-08-03T23:54:01.6NZ`
- total jobs: `10200`
- total duration seconds: `43212.474`
- jobs per second: `0.24`

| Customer | Job Count | First Completion ms | Finish Completion ms | Early Share of First 20 |
| --- | ---: | ---: | ---: | ---: |
| `customer-a` | 5000 | 43199630.476 | 43208344.338 | 1.000 |
| `customer-b` | 5000 | 43203904.444 | 43212474.320 | 0.000 |
| `customer-c` | 100 | 43208381.908 | 43208892.761 | 0.000 |
| `customer-d` | 100 | 43208475.326 | 43208743.678 | 0.000 |

## 4 instances

- start time: `2026-08-03T23:54:14.6NZ`
- total jobs: `10200`
- total duration seconds: `43210.804`
- jobs per second: `0.24`

| Customer | Job Count | First Completion ms | Finish Completion ms | Early Share of First 20 |
| --- | ---: | ---: | ---: | ---: |
| `customer-a` | 5000 | 43200107.500 | 43208638.468 | 1.000 |
| `customer-b` | 5000 | 43203563.732 | 43210804.005 | 0.000 |
| `customer-c` | 100 | 43206175.481 | 43206975.743 | 0.000 |
| `customer-d` | 100 | 43206311.943 | 43207009.938 | 0.000 |

