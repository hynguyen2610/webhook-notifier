#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ARTIFACT_DIR="${ROOT_DIR}/.tmp/single-instance-load-test"
REPORT_DIR="${ROOT_DIR}/loadtest/reports"
BENCHMARK_DATABASE_NAME="${BENCHMARK_DATABASE_NAME:-webhook_notifier_benchmark}"
POSTGRES_ADMIN_DSN="${POSTGRES_ADMIN_DSN:-postgres://postgres:password@127.0.0.1:15432/postgres?sslmode=disable}"
BENCHMARK_POSTGRES_DSN="${BENCHMARK_POSTGRES_DSN:-postgres://postgres:password@127.0.0.1:15432/${BENCHMARK_DATABASE_NAME}?sslmode=disable}"
RECEIVER_HTTP_ADDRESS="${RECEIVER_HTTP_ADDRESS:-:28093}"
NOTIFIER_WORKER_COUNT="${NOTIFIER_WORKER_COUNT:-20}"
NOTIFIER_QUEUE_CLAIM_BATCH_SIZE="${NOTIFIER_QUEUE_CLAIM_BATCH_SIZE:-32}"
NOTIFIER_QUEUE_POLL_INTERVAL="${NOTIFIER_QUEUE_POLL_INTERVAL:-50ms}"
NOTIFIER_REQUEST_TIMEOUT="${NOTIFIER_REQUEST_TIMEOUT:-2s}"
SUITE_NAME="${1:-smoke}"

case "${SUITE_NAME}" in
  smoke)
    CUSTOMER_A_EVENTS=100
    CUSTOMER_B_EVENTS=20
    CUSTOMER_C_EVENTS=20
    SCENARIO_NAME="single-instance-smoke"
    TARGET_RUNTIME_SECONDS=10
    ;;
  fairness)
    CUSTOMER_A_EVENTS=100000
    CUSTOMER_B_EVENTS=100
    CUSTOMER_C_EVENTS=100
    SCENARIO_NAME="single-instance-fairness"
    TARGET_RUNTIME_SECONDS=60
    ;;
  *)
    printf 'usage: %s [smoke|fairness]\n' "$0" >&2
    exit 1
    ;;
esac

CUSTOMER_D_EVENTS=0
TOTAL_EVENT_COUNT=$((CUSTOMER_A_EVENTS + CUSTOMER_B_EVENTS + CUSTOMER_C_EVENTS + CUSTOMER_D_EVENTS))
RECEIVER_HOST="127.0.0.1"
RECEIVER_PORT="${RECEIVER_HTTP_ADDRESS##*:}"
RECEIVER_BASE_URL="http://${RECEIVER_HOST}:${RECEIVER_PORT}"
RECEIVER_LOG_PATH="${ARTIFACT_DIR}/mock-receiver.log"
NOTIFIER_BINARY_PATH="${ARTIFACT_DIR}/notifier"
RECEIVER_BINARY_PATH="${ARTIFACT_DIR}/mock-webhook-receiver"
TIMESTAMP="$(date -u +"%Y%m%d-%H%M%S")"
REPORT_PATH="${REPORT_DIR}/single-instance-load-test-${SUITE_NAME}-${TIMESTAMP}.md"
SNAPSHOT_PATH="${ARTIFACT_DIR}/${SUITE_NAME}-queue-snapshots.tsv"

RECEIVER_PROCESS_ID=""
NOTIFIER_PROCESS_ID=""
BENCHMARK_STARTED_AT_EPOCH=""
LAST_RUN_MAX_OLDEST_PENDING_EVENT_AGE_SECONDS="0"

log() {
  printf '[single-instance-load-test] %s\n' "$1"
}

sql_admin() {
  psql "${POSTGRES_ADMIN_DSN}" -v ON_ERROR_STOP=1 -AtF $'\t' -c "$1"
}

sql_benchmark() {
  psql "${BENCHMARK_POSTGRES_DSN}" -v ON_ERROR_STOP=1 -AtF $'\t' -c "$1"
}

current_benchmark_timestamp() {
  sql_benchmark "SELECT to_char(clock_timestamp() AT TIME ZONE 'UTC', 'YYYY-MM-DD\"T\"HH24:MI:SS.US\"Z\"');"
}

cleanup() {
  local exit_code=$?

  if [[ -n "${NOTIFIER_PROCESS_ID}" ]] && kill -0 "${NOTIFIER_PROCESS_ID}" >/dev/null 2>&1; then
    kill "${NOTIFIER_PROCESS_ID}" >/dev/null 2>&1 || true
    wait "${NOTIFIER_PROCESS_ID}" 2>/dev/null || true
  fi

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

reset_tables() {
  sql_benchmark "
TRUNCATE webhook_delivery_queue RESTART IDENTITY;
DELETE FROM webhook_registrations;

INSERT INTO webhook_registrations (customer_id, webhook_url, is_active)
VALUES
  ('customer-a', '${RECEIVER_BASE_URL}/webhook/customer-a', TRUE),
  ('customer-b', '${RECEIVER_BASE_URL}/webhook/customer-b', TRUE),
  ('customer-c', '${RECEIVER_BASE_URL}/webhook/customer-c', TRUE);
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
SELECT 'customer-a-event-' || sequence_number, 'customer-a', 'subscriber-customer-a-' || sequence_number, 'subscriber.created',
  TIMESTAMPTZ '${created_at}', '${RECEIVER_BASE_URL}/webhook/customer-a', 'pending', TIMESTAMPTZ '${created_at}', 0, TIMESTAMPTZ '${created_at}', TIMESTAMPTZ '${created_at}'
FROM generate_series(1, ${CUSTOMER_A_EVENTS}) AS sequence_number;

INSERT INTO webhook_delivery_queue (
  event_id, customer_id, subscriber_id, event_type, occurred_at, webhook_url,
  status, available_at, retry_count, created_at, updated_at
)
SELECT 'customer-b-event-' || sequence_number, 'customer-b', 'subscriber-customer-b-' || sequence_number, 'subscriber.created',
  TIMESTAMPTZ '${created_at}', '${RECEIVER_BASE_URL}/webhook/customer-b', 'pending', TIMESTAMPTZ '${created_at}', 0, TIMESTAMPTZ '${created_at}', TIMESTAMPTZ '${created_at}'
FROM generate_series(1, ${CUSTOMER_B_EVENTS}) AS sequence_number;

INSERT INTO webhook_delivery_queue (
  event_id, customer_id, subscriber_id, event_type, occurred_at, webhook_url,
  status, available_at, retry_count, created_at, updated_at
)
SELECT 'customer-c-event-' || sequence_number, 'customer-c', 'subscriber-customer-c-' || sequence_number, 'subscriber.created',
  TIMESTAMPTZ '${created_at}', '${RECEIVER_BASE_URL}/webhook/customer-c', 'pending', TIMESTAMPTZ '${created_at}', 0, TIMESTAMPTZ '${created_at}', TIMESTAMPTZ '${created_at}'
FROM generate_series(1, ${CUSTOMER_C_EVENTS}) AS sequence_number;
"
}

start_notifier() {
  local log_path="${ARTIFACT_DIR}/notifier-${SUITE_NAME}.log"
  NOTIFIER_HTTP_ADDRESS=":0" \
  NOTIFIER_POSTGRES_DSN="${BENCHMARK_POSTGRES_DSN}" \
  NOTIFIER_WORKER_COUNT="${NOTIFIER_WORKER_COUNT}" \
  NOTIFIER_MAX_RETRY_ATTEMPTS="0" \
  NOTIFIER_REQUEST_TIMEOUT="${NOTIFIER_REQUEST_TIMEOUT}" \
  NOTIFIER_QUEUE_CLAIM_BATCH_SIZE="${NOTIFIER_QUEUE_CLAIM_BATCH_SIZE}" \
  NOTIFIER_QUEUE_POLL_INTERVAL="${NOTIFIER_QUEUE_POLL_INTERVAL}" \
  "${NOTIFIER_BINARY_PATH}" >"${log_path}" 2>&1 &
  NOTIFIER_PROCESS_ID=$!
}

capture_queue_snapshot() {
  sql_benchmark "
SELECT
  EXTRACT(EPOCH FROM clock_timestamp()),
  COUNT(*) FILTER (WHERE status = 'pending'),
  COUNT(*) FILTER (WHERE status = 'claimed'),
  COUNT(*) FILTER (WHERE status = 'completed'),
  COALESCE(SUM(retry_count), 0),
  COALESCE(
    ROUND(
      MAX(
        CASE
          WHEN status IN ('pending', 'claimed') THEN EXTRACT(EPOCH FROM clock_timestamp() - created_at)
          ELSE 0
        END
      )::numeric,
      3
    ),
    0
  )
FROM webhook_delivery_queue;
"
}

wait_for_completion() {
  : >"${SNAPSHOT_PATH}"
  LAST_RUN_MAX_OLDEST_PENDING_EVENT_AGE_SECONDS="0"

  for attempt_number in $(seq 1 3600); do
    local snapshot
    snapshot="$(capture_queue_snapshot)"
    local snapshot_epoch pending_count claimed_count completed_count retry_count oldest_pending_age_seconds
    IFS=$'\t' read -r snapshot_epoch pending_count claimed_count completed_count retry_count oldest_pending_age_seconds <<<"${snapshot}"
    printf '%s\t%s\t%s\t%s\t%s\t%s\n' \
      "${snapshot_epoch}" "${pending_count}" "${claimed_count}" "${completed_count}" "${retry_count}" "${oldest_pending_age_seconds}" >>"${SNAPSHOT_PATH}"

    if awk "BEGIN {exit !(${oldest_pending_age_seconds} > ${LAST_RUN_MAX_OLDEST_PENDING_EVENT_AGE_SECONDS})}"; then
      LAST_RUN_MAX_OLDEST_PENDING_EVENT_AGE_SECONDS="${oldest_pending_age_seconds}"
    fi

    if [[ "${completed_count}" == "${TOTAL_EVENT_COUNT}" && "${pending_count}" == "0" && "${claimed_count}" == "0" ]]; then
      return 0
    fi

    if (( attempt_number % 50 == 0 )); then
      log "progress ${completed_count}/${TOTAL_EVENT_COUNT} completed; pending ${pending_count}; claimed ${claimed_count}; max oldest pending age ${LAST_RUN_MAX_OLDEST_PENDING_EVENT_AGE_SECONDS}s"
    fi
    sleep 0.1
  done

  log "timed out waiting for ${TOTAL_EVENT_COUNT} completed jobs"
  return 1
}

write_report() {
  local benchmark_started_at_label="$1"
  local total_duration_seconds average_latency_seconds maximum_latency_seconds retry_count final_pending_count max_unfinished_queue_depth max_completed_count
  total_duration_seconds="$(sql_benchmark "SELECT ROUND((MAX(EXTRACT(EPOCH FROM completed_at)) - ${BENCHMARK_STARTED_AT_EPOCH})::numeric, 3) FROM webhook_delivery_queue WHERE status = 'completed';")"
  average_latency_seconds="$(sql_benchmark "SELECT ROUND(AVG(EXTRACT(EPOCH FROM completed_at - created_at))::numeric, 3) FROM webhook_delivery_queue WHERE status = 'completed';")"
  maximum_latency_seconds="$(sql_benchmark "SELECT ROUND(MAX(EXTRACT(EPOCH FROM completed_at - created_at))::numeric, 3) FROM webhook_delivery_queue WHERE status = 'completed';")"
  retry_count="$(sql_benchmark "SELECT COALESCE(SUM(retry_count), 0) FROM webhook_delivery_queue;")"
  final_pending_count="$(sql_benchmark "SELECT COUNT(*) FROM webhook_delivery_queue WHERE status IN ('pending', 'claimed');")"
  max_unfinished_queue_depth="$(awk -F $'\t' 'BEGIN{max=0} {unfinished=$2 + $3; if (unfinished > max) max=unfinished} END{print max+0}' "${SNAPSHOT_PATH}")"
  max_completed_count="$(awk -F $'\t' 'BEGIN{max=0} {if ($4 > max) max=$4} END{print max+0}' "${SNAPSHOT_PATH}")"

  {
    printf '# Single-Instance Load Test Report\n\n'
    printf -- '- date: `%s`\n' "$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
    printf -- '- suite: `%s`\n' "${SUITE_NAME}"
    printf -- '- scenario: `%s`\n' "${SCENARIO_NAME}"
    printf -- '- benchmark database: `%s`\n' "${BENCHMARK_DATABASE_NAME}"
    printf -- '- notifier instances: `1`\n'
    printf -- '- notifier worker count: `%s`\n' "${NOTIFIER_WORKER_COUNT}"
    printf -- '- queue claim batch size: `%s`\n' "${NOTIFIER_QUEUE_CLAIM_BATCH_SIZE}"
    printf -- '- queue poll interval: `%s`\n' "${NOTIFIER_QUEUE_POLL_INTERVAL}"
    printf -- '- expected runtime target seconds: `%s`\n' "${TARGET_RUNTIME_SECONDS}"
    printf -- '- queue entry path: direct PostgreSQL preload\n'
    printf -- '- workload: `customer-a=%s`, `customer-b=%s`, `customer-c=%s`\n\n' "${CUSTOMER_A_EVENTS}" "${CUSTOMER_B_EVENTS}" "${CUSTOMER_C_EVENTS}"
    printf 'This single-instance load test intentionally measures the PostgreSQL-backed processing path only. It excludes notifier HTTP ingest benchmarking so the run stays simple and focused on queue claiming, scheduling, worker execution, retry behavior, and delivery completion.\n\n'
    printf '## Run Overview\n\n'
    printf -- '- total messages sent: `%s`\n' "${TOTAL_EVENT_COUNT}"
    printf -- '- total messages delivered: `%s`\n' "${max_completed_count}"
    printf -- '- total processing time seconds: `%s`\n\n' "${total_duration_seconds}"
    printf '## Validation Outcome\n\n'
    printf -- '- all events delivered successfully: `%s`\n' "$([[ "${max_completed_count}" == "${TOTAL_EVENT_COUNT}" ]] && printf 'yes' || printf 'no')"
    printf -- '- queue depth drained to zero: `%s`\n' "$([[ "${final_pending_count}" == "0" ]] && printf 'yes' || printf 'no')"
    printf -- '- retry count remained zero: `%s`\n' "$([[ "${retry_count}" == "0" ]] && printf 'yes' || printf 'no')"
    printf -- '- small-customer completion before whale backlog finished: `%s`\n\n' "$(sql_benchmark "SELECT CASE WHEN EXISTS (SELECT 1 FROM webhook_delivery_queue small WHERE small.customer_id IN ('customer-b', 'customer-c') AND small.status = 'completed' AND small.completed_at < (SELECT MAX(whale.completed_at) FROM webhook_delivery_queue whale WHERE whale.customer_id = 'customer-a' AND whale.status = 'completed')) THEN 'yes' ELSE 'no' END;")"
    printf '## Metric Summary\n\n'
    printf -- '- queue depth initial: `%s`\n' "${TOTAL_EVENT_COUNT}"
    printf -- '- queue depth final: `%s`\n' "${final_pending_count}"
    printf -- '- queue depth max unfinished: `%s`\n' "${max_unfinished_queue_depth}"
    printf -- '- end-to-end delivery latency average seconds: `%s`\n' "${average_latency_seconds}"
    printf -- '- end-to-end delivery latency max seconds: `%s`\n' "${maximum_latency_seconds}"
    printf -- '- retry count: `%s`\n' "${retry_count}"
    printf -- '- max oldest pending event age seconds: `%s`\n' "${LAST_RUN_MAX_OLDEST_PENDING_EVENT_AGE_SECONDS}"
    printf -- '- total duration seconds: `%s`\n' "${total_duration_seconds}"
    printf -- '- run start time: `%s`\n\n' "${benchmark_started_at_label}"
    printf '## Queue Snapshots\n\n'
    printf '| Elapsed Seconds | Pending | Claimed | Unfinished Queue Depth | Completed | Retry Count | Oldest Pending Age Seconds |\n'
    printf '| ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n'
    awk -F $'\t' -v started_at="${BENCHMARK_STARTED_AT_EPOCH}" '
      {
        elapsed = sprintf("%.3f", $1 - started_at)
        unfinished = $2 + $3
        printf("| %s | %s | %s | %s | %s | %s | %s |\n", elapsed, $2, $3, unfinished, $4, $5, $6)
      }
    ' "${SNAPSHOT_PATH}"
  } >"${REPORT_PATH}"
}

main() {
  ensure_benchmark_database
  ensure_schema
  build_binaries
  start_receiver
  reset_tables
  preload_workload
  curl -fsS -X POST "${RECEIVER_BASE_URL}/stats/reset" >/dev/null
  local benchmark_started_at_label
  benchmark_started_at_label="$(sql_benchmark "SELECT to_char(clock_timestamp() AT TIME ZONE 'UTC', 'YYYY-MM-DD\"T\"HH24:MI:SS.MS\"Z\"');")"
  BENCHMARK_STARTED_AT_EPOCH="$(sql_benchmark "SELECT EXTRACT(EPOCH FROM clock_timestamp());")"
  start_notifier
  wait_for_completion
  write_report "${benchmark_started_at_label}"
  log "load test report written to ${REPORT_PATH}"
}

main "$@"
