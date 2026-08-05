#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ARTIFACT_DIR="${ROOT_DIR}/.tmp/multi-instance-benchmark"
REPORT_DIR="${ROOT_DIR}/loadtest/reports"
BENCHMARK_DATABASE_NAME="${BENCHMARK_DATABASE_NAME:-webhook_notifier_benchmark}"
POSTGRES_ADMIN_DSN="${POSTGRES_ADMIN_DSN:-postgres://postgres:password@127.0.0.1:15432/postgres?sslmode=disable}"
BENCHMARK_POSTGRES_DSN="${BENCHMARK_POSTGRES_DSN:-postgres://postgres:password@127.0.0.1:15432/${BENCHMARK_DATABASE_NAME}?sslmode=disable}"
RECEIVER_HTTP_ADDRESS="${RECEIVER_HTTP_ADDRESS:-:28092}"
NOTIFIER_WORKER_COUNT="${NOTIFIER_WORKER_COUNT:-4}"
NOTIFIER_QUEUE_CLAIM_BATCH_SIZE="${NOTIFIER_QUEUE_CLAIM_BATCH_SIZE:-32}"
NOTIFIER_QUEUE_POLL_INTERVAL="${NOTIFIER_QUEUE_POLL_INTERVAL:-50ms}"
NOTIFIER_REQUEST_TIMEOUT="${NOTIFIER_REQUEST_TIMEOUT:-2s}"
INSTANCE_COUNTS=(${INSTANCE_COUNTS:-1 2 4})

CUSTOMER_A_EVENTS="${CUSTOMER_A_EVENTS:-5000}"
CUSTOMER_B_EVENTS="${CUSTOMER_B_EVENTS:-5000}"
CUSTOMER_C_EVENTS="${CUSTOMER_C_EVENTS:-100}"
CUSTOMER_D_EVENTS="${CUSTOMER_D_EVENTS:-100}"
TOTAL_EVENT_COUNT=$((CUSTOMER_A_EVENTS + CUSTOMER_B_EVENTS + CUSTOMER_C_EVENTS + CUSTOMER_D_EVENTS))
EARLY_COMPLETION_WINDOW="${EARLY_COMPLETION_WINDOW:-20}"
SCENARIO_NAME="${SCENARIO_NAME:-two-whales-5000-two-non-whales-100}"

RECEIVER_HOST="127.0.0.1"
RECEIVER_PORT="${RECEIVER_HTTP_ADDRESS##*:}"
RECEIVER_BASE_URL="http://${RECEIVER_HOST}:${RECEIVER_PORT}"
RECEIVER_LOG_PATH="${ARTIFACT_DIR}/mock-receiver.log"
NOTIFIER_BINARY_PATH="${ARTIFACT_DIR}/notifier"
RECEIVER_BINARY_PATH="${ARTIFACT_DIR}/mock-webhook-receiver"
TIMESTAMP="$(date -u +"%Y%m%d-%H%M%S")"
REPORT_PATH="${REPORT_DIR}/multi-instance-benchmark-${TIMESTAMP}.md"

RECEIVER_PROCESS_ID=""
NOTIFIER_PROCESS_IDS=()
LAST_RUN_MAX_OLDEST_PENDING_EVENT_AGE_SECONDS="0"

log() {
  printf '[multi-instance-benchmark] %s\n' "$1"
}

current_benchmark_timestamp() {
  sql_benchmark "SELECT to_char(clock_timestamp() AT TIME ZONE 'UTC', 'YYYY-MM-DD\"T\"HH24:MI:SS.US\"Z\"');"
}

sql_admin() {
  psql "${POSTGRES_ADMIN_DSN}" -v ON_ERROR_STOP=1 -AtF $'\t' -c "$1"
}

sql_benchmark() {
  psql "${BENCHMARK_POSTGRES_DSN}" -v ON_ERROR_STOP=1 -AtF $'\t' -c "$1"
}

cleanup() {
  local exit_code=$?

  for process_id in "${NOTIFIER_PROCESS_IDS[@]:-}"; do
    if [[ -n "${process_id}" ]] && kill -0 "${process_id}" >/dev/null 2>&1; then
      kill "${process_id}" >/dev/null 2>&1 || true
      wait "${process_id}" 2>/dev/null || true
    fi
  done

  if [[ -n "${RECEIVER_PROCESS_ID}" ]] && kill -0 "${RECEIVER_PROCESS_ID}" >/dev/null 2>&1; then
    kill "${RECEIVER_PROCESS_ID}" >/dev/null 2>&1 || true
    wait "${RECEIVER_PROCESS_ID}" 2>/dev/null || true
  fi

  exit "${exit_code}"
}

trap cleanup EXIT

wait_for_health() {
  local health_url="$1"
  local service_name="$2"

  for _ in $(seq 1 80); do
    if curl -fsS "${health_url}" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.25
  done

  log "${service_name} did not become healthy at ${health_url}"
  return 1
}

wait_for_completed_count() {
  local expected_count="$1"
  LAST_RUN_MAX_OLDEST_PENDING_EVENT_AGE_SECONDS="0"

  for attempt_number in $(seq 1 600); do
    local queue_snapshot
    queue_snapshot="$(sql_benchmark "
SELECT
  COUNT(*) FILTER (WHERE status = 'completed') AS completed_count,
  COALESCE(
    ROUND(
      MAX(
        CASE
          WHEN status = 'pending' THEN EXTRACT(EPOCH FROM clock_timestamp() - created_at)
          ELSE 0
        END
      )::numeric,
      3
    ),
    0
  ) AS max_oldest_pending_event_age_seconds
FROM webhook_delivery_queue;
")"
    local completed_count
    local observed_oldest_pending_event_age_seconds
    IFS=$'\t' read -r completed_count observed_oldest_pending_event_age_seconds <<<"${queue_snapshot}"
    if awk "BEGIN {exit !(${observed_oldest_pending_event_age_seconds} > ${LAST_RUN_MAX_OLDEST_PENDING_EVENT_AGE_SECONDS})}"; then
      LAST_RUN_MAX_OLDEST_PENDING_EVENT_AGE_SECONDS="${observed_oldest_pending_event_age_seconds}"
    fi
    if [[ "${completed_count}" == "${expected_count}" ]]; then
      return 0
    fi

    if (( attempt_number % 50 == 0 )); then
      log "progress ${completed_count}/${expected_count} completed; max oldest pending age ${LAST_RUN_MAX_OLDEST_PENDING_EVENT_AGE_SECONDS}s"
    fi

    sleep 0.1
  done

  log "timed out waiting for ${expected_count} completed jobs"
  return 1
}

ensure_benchmark_database() {
  local database_exists
  database_exists="$(sql_admin "SELECT 1 FROM pg_database WHERE datname = '${BENCHMARK_DATABASE_NAME}';")"
  if [[ "${database_exists}" != "1" ]]; then
    log "creating benchmark database ${BENCHMARK_DATABASE_NAME}"
    sql_admin "CREATE DATABASE ${BENCHMARK_DATABASE_NAME};"
  fi
}

ensure_schema() {
  sql_benchmark "
CREATE TABLE IF NOT EXISTS webhook_registrations (
  customer_id TEXT NOT NULL,
  webhook_url TEXT NOT NULL,
  is_active BOOLEAN NOT NULL DEFAULT TRUE
);

CREATE TABLE IF NOT EXISTS webhook_delivery_queue (
  id BIGSERIAL PRIMARY KEY,
  event_id TEXT NOT NULL,
  customer_id TEXT NOT NULL,
  subscriber_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  occurred_at TIMESTAMPTZ NOT NULL,
  webhook_url TEXT NOT NULL,
  status TEXT NOT NULL,
  available_at TIMESTAMPTZ NOT NULL,
  claimed_at TIMESTAMPTZ NULL,
  claim_owner TEXT NULL,
  retry_count INTEGER NOT NULL DEFAULT 0,
  last_error TEXT NULL,
  dead_lettered_at TIMESTAMPTZ NULL,
  completed_at TIMESTAMPTZ NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS webhook_delivery_queue_pending_idx
  ON webhook_delivery_queue (status, available_at, id);
"
}

build_binaries() {
  mkdir -p "${ARTIFACT_DIR}" "${REPORT_DIR}"
  go build -o "${NOTIFIER_BINARY_PATH}" ./cmd/notifier
  go build -o "${RECEIVER_BINARY_PATH}" ./cmd/mock-webhook-receiver
}

start_receiver() {
  log "starting mock receiver on ${RECEIVER_HTTP_ADDRESS}"
  RECEIVER_HTTP_ADDRESS="${RECEIVER_HTTP_ADDRESS}" "${RECEIVER_BINARY_PATH}" >"${RECEIVER_LOG_PATH}" 2>&1 &
  RECEIVER_PROCESS_ID=$!
  wait_for_health "${RECEIVER_BASE_URL}/health" "mock receiver"
}

reset_benchmark_tables() {
  sql_benchmark "
TRUNCATE webhook_delivery_queue RESTART IDENTITY;
DELETE FROM webhook_registrations;

INSERT INTO webhook_registrations (customer_id, webhook_url, is_active)
VALUES
  ('customer-a', '${RECEIVER_BASE_URL}/webhook/customer-a', TRUE),
  ('customer-b', '${RECEIVER_BASE_URL}/webhook/customer-b', TRUE),
  ('customer-c', '${RECEIVER_BASE_URL}/webhook/customer-c', TRUE),
  ('customer-d', '${RECEIVER_BASE_URL}/webhook/customer-d', TRUE);
"
}

preload_workload() {
  local created_at
  created_at="$(current_benchmark_timestamp)"

  sql_benchmark "
INSERT INTO webhook_delivery_queue (
  event_id, customer_id, subscriber_id, event_type, occurred_at, webhook_url,
  status, available_at, retry_count, created_at, updated_at
)
SELECT
  'customer-a-event-' || sequence_number,
  'customer-a',
  'subscriber-customer-a-' || sequence_number,
  'subscriber.updated',
  TIMESTAMPTZ '${created_at}',
  '${RECEIVER_BASE_URL}/webhook/customer-a',
  'pending',
  TIMESTAMPTZ '${created_at}',
  0,
  TIMESTAMPTZ '${created_at}',
  TIMESTAMPTZ '${created_at}'
FROM generate_series(1, ${CUSTOMER_A_EVENTS}) AS sequence_number;

INSERT INTO webhook_delivery_queue (
  event_id, customer_id, subscriber_id, event_type, occurred_at, webhook_url,
  status, available_at, retry_count, created_at, updated_at
)
SELECT
  'customer-b-event-' || sequence_number,
  'customer-b',
  'subscriber-customer-b-' || sequence_number,
  'subscriber.updated',
  TIMESTAMPTZ '${created_at}',
  '${RECEIVER_BASE_URL}/webhook/customer-b',
  'pending',
  TIMESTAMPTZ '${created_at}',
  0,
  TIMESTAMPTZ '${created_at}',
  TIMESTAMPTZ '${created_at}'
FROM generate_series(1, ${CUSTOMER_B_EVENTS}) AS sequence_number;

INSERT INTO webhook_delivery_queue (
  event_id, customer_id, subscriber_id, event_type, occurred_at, webhook_url,
  status, available_at, retry_count, created_at, updated_at
)
SELECT
  'customer-c-event-' || sequence_number,
  'customer-c',
  'subscriber-customer-c-' || sequence_number,
  'subscriber.updated',
  TIMESTAMPTZ '${created_at}',
  '${RECEIVER_BASE_URL}/webhook/customer-c',
  'pending',
  TIMESTAMPTZ '${created_at}',
  0,
  TIMESTAMPTZ '${created_at}',
  TIMESTAMPTZ '${created_at}'
FROM generate_series(1, ${CUSTOMER_C_EVENTS}) AS sequence_number;

INSERT INTO webhook_delivery_queue (
  event_id, customer_id, subscriber_id, event_type, occurred_at, webhook_url,
  status, available_at, retry_count, created_at, updated_at
)
SELECT
  'customer-d-event-' || sequence_number,
  'customer-d',
  'subscriber-customer-d-' || sequence_number,
  'subscriber.updated',
  TIMESTAMPTZ '${created_at}',
  '${RECEIVER_BASE_URL}/webhook/customer-d',
  'pending',
  TIMESTAMPTZ '${created_at}',
  0,
  TIMESTAMPTZ '${created_at}',
  TIMESTAMPTZ '${created_at}'
FROM generate_series(1, ${CUSTOMER_D_EVENTS}) AS sequence_number;
"
}

stop_notifiers() {
  for process_id in "${NOTIFIER_PROCESS_IDS[@]:-}"; do
    if [[ -n "${process_id}" ]] && kill -0 "${process_id}" >/dev/null 2>&1; then
      kill "${process_id}" >/dev/null 2>&1 || true
      wait "${process_id}" 2>/dev/null || true
    fi
  done
  NOTIFIER_PROCESS_IDS=()
}

start_notifiers() {
  local instance_count="$1"
  NOTIFIER_PROCESS_IDS=()

  for instance_number in $(seq 1 "${instance_count}"); do
    local log_path="${ARTIFACT_DIR}/notifier-${instance_count}x-instance-${instance_number}.log"
    NOTIFIER_HTTP_ADDRESS=":0" \
    NOTIFIER_POSTGRES_DSN="${BENCHMARK_POSTGRES_DSN}" \
    NOTIFIER_WORKER_COUNT="${NOTIFIER_WORKER_COUNT}" \
    NOTIFIER_MAX_RETRY_ATTEMPTS="0" \
    NOTIFIER_REQUEST_TIMEOUT="${NOTIFIER_REQUEST_TIMEOUT}" \
    NOTIFIER_QUEUE_CLAIM_BATCH_SIZE="${NOTIFIER_QUEUE_CLAIM_BATCH_SIZE}" \
    NOTIFIER_QUEUE_POLL_INTERVAL="${NOTIFIER_QUEUE_POLL_INTERVAL}" \
    "${NOTIFIER_BINARY_PATH}" >"${log_path}" 2>&1 &
    NOTIFIER_PROCESS_IDS+=("$!")
  done
}

append_run_report() {
  local instance_count="$1"
  local benchmark_started_at_label="$2"
  local benchmark_started_at_epoch="$3"

  {
    printf '## %s instance%s\n\n' "${instance_count}" "$([[ "${instance_count}" == "1" ]] && printf '' || printf 's')"
    printf -- '- start time: `%s`\n' "${benchmark_started_at_label}"
    printf -- '- total jobs: `%s`\n' "${TOTAL_EVENT_COUNT}"
    printf -- '- total duration seconds: `%s`\n' "$(sql_benchmark "SELECT ROUND((MAX(EXTRACT(EPOCH FROM completed_at)) - ${benchmark_started_at_epoch})::numeric, 3) FROM webhook_delivery_queue WHERE status = 'completed';")"
    printf -- '- jobs per second: `%s`\n\n' "$(sql_benchmark "SELECT ROUND(COUNT(*) / NULLIF((MAX(EXTRACT(EPOCH FROM completed_at)) - ${benchmark_started_at_epoch}), 0)::numeric, 2) FROM webhook_delivery_queue WHERE status = 'completed';")"
    printf -- '- max oldest pending event age seconds: `%s`\n\n' "${LAST_RUN_MAX_OLDEST_PENDING_EVENT_AGE_SECONDS}"
    printf '| Customer | Job Count | First Completion ms | Finish Completion ms | Early Share of First %s |\n' "${EARLY_COMPLETION_WINDOW}"
    printf '| --- | ---: | ---: | ---: | ---: |\n'
    sql_benchmark "
WITH early_completions AS (
  SELECT customer_id
  FROM webhook_delivery_queue
  WHERE status = 'completed'
  ORDER BY completed_at, id
  LIMIT ${EARLY_COMPLETION_WINDOW}
)
SELECT
  queue.customer_id,
  COUNT(*) AS job_count,
  ROUND((MIN(EXTRACT(EPOCH FROM queue.completed_at)) - ${benchmark_started_at_epoch}) * 1000, 3) AS first_completion_ms,
  ROUND((MAX(EXTRACT(EPOCH FROM queue.completed_at)) - ${benchmark_started_at_epoch}) * 1000, 3) AS finish_completion_ms,
  ROUND(COALESCE((
    SELECT COUNT(*)::numeric / ${EARLY_COMPLETION_WINDOW}
    FROM early_completions
    WHERE early_completions.customer_id = queue.customer_id
  ), 0), 3) AS early_share
FROM webhook_delivery_queue AS queue
WHERE queue.status = 'completed'
GROUP BY queue.customer_id
ORDER BY queue.customer_id;
" | while IFS=$'\t' read -r customer_id job_count first_completion_ms finish_completion_ms early_share; do
      printf '| `%s` | %s | %s | %s | %s |\n' "${customer_id}" "${job_count}" "${first_completion_ms}" "${finish_completion_ms}" "${early_share}"
    done
    printf '\n'
  } >>"${REPORT_PATH}"
}

main() {
  ensure_benchmark_database
  ensure_schema
  build_binaries
  start_receiver

  {
    printf '# Multi-Instance Benchmark Report\n\n'
    printf -- '- date: `%s`\n' "$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
    printf -- '- scenario: `%s`\n' "${SCENARIO_NAME}"
    printf -- '- benchmark database: `%s`\n' "${BENCHMARK_DATABASE_NAME}"
    printf -- '- notifier worker count per instance: `%s`\n' "${NOTIFIER_WORKER_COUNT}"
    printf -- '- notifier queue claim batch size: `%s`\n' "${NOTIFIER_QUEUE_CLAIM_BATCH_SIZE}"
    printf -- '- notifier queue poll interval: `%s`\n' "${NOTIFIER_QUEUE_POLL_INTERVAL}"
    printf -- '- retries: disabled (`NOTIFIER_MAX_RETRY_ATTEMPTS=0`)\n'
    printf -- '- receiver base URL: `%s`\n' "${RECEIVER_BASE_URL}"
    printf -- '- workload: `customer-a=%s`, `customer-b=%s`, `customer-c=%s`, `customer-d=%s`\n\n' \
      "${CUSTOMER_A_EVENTS}" "${CUSTOMER_B_EVENTS}" "${CUSTOMER_C_EVENTS}" "${CUSTOMER_D_EVENTS}"
    printf 'This run preloads the PostgreSQL queue directly before starting the notifier instances. That means the result measures PostgreSQL-backed claiming, scheduling, worker execution, and local HTTP delivery, but not HTTP ingest cost.\n\n'
  } >"${REPORT_PATH}"

  for instance_count in "${INSTANCE_COUNTS[@]}"; do
    log "running benchmark with ${instance_count} notifier instance(s)"
    reset_benchmark_tables
    preload_workload
    curl -fsS -X POST "${RECEIVER_BASE_URL}/stats/reset" >/dev/null
    benchmark_started_at_label="$(sql_benchmark "SELECT to_char(clock_timestamp() AT TIME ZONE 'UTC', 'YYYY-MM-DD\"T\"HH24:MI:SS.MS\"Z\"');")"
    benchmark_started_at_epoch="$(sql_benchmark "SELECT EXTRACT(EPOCH FROM clock_timestamp());")"
    start_notifiers "${instance_count}"
    wait_for_completed_count "${TOTAL_EVENT_COUNT}"
    append_run_report "${instance_count}" "${benchmark_started_at_label}" "${benchmark_started_at_epoch}"
    stop_notifiers
  done

  log "benchmark report written to ${REPORT_PATH}"
}

main "$@"
