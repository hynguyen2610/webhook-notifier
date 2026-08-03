#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LOG_DIR="${ROOT_DIR}/.tmp/full-flow"
mkdir -p "${LOG_DIR}"
KILL_PORT_SCRIPT="${ROOT_DIR}/scripts/kill-local-service-ports.sh"

POSTGRES_LOCAL_ADDRESS="${POSTGRES_LOCAL_ADDRESS:-127.0.0.1:15432}"
POSTGRES_PORT_FORWARD_TARGET="${POSTGRES_PORT_FORWARD_TARGET:-svc/user-org-db-service}"
POSTGRES_PORT_FORWARD_NAMESPACE="${POSTGRES_PORT_FORWARD_NAMESPACE:-default}"
POSTGRES_ADMIN_DSN="${POSTGRES_ADMIN_DSN:-postgres://postgres:password@127.0.0.1:15432/postgres?sslmode=disable}"
WEBHOOK_NOTIFIER_DB_NAME="${WEBHOOK_NOTIFIER_DB_NAME:-webhook_notifier}"
NOTIFIER_POSTGRES_DSN="${NOTIFIER_POSTGRES_DSN:-postgres://postgres:password@127.0.0.1:15432/${WEBHOOK_NOTIFIER_DB_NAME}?sslmode=disable}"

NOTIFIER_HTTP_ADDRESS="${NOTIFIER_HTTP_ADDRESS:-:28080}"
GENERATOR_HTTP_ADDRESS="${GENERATOR_HTTP_ADDRESS:-:28081}"
RECEIVER_HTTP_ADDRESS="${RECEIVER_HTTP_ADDRESS:-:28082}"

NOTIFIER_BASE_URL="${NOTIFIER_BASE_URL:-http://localhost:28080}"
GENERATOR_BASE_URL="${GENERATOR_BASE_URL:-http://localhost:28081}"
RECEIVER_BASE_URL="${RECEIVER_BASE_URL:-http://localhost:28082}"

GENERATOR_CUSTOMER_ID="${GENERATOR_CUSTOMER_ID:-customer-a}"
GENERATOR_EVENT_TYPE="${GENERATOR_EVENT_TYPE:-subscriber.created}"
GENERATOR_EVENT_COUNT="${GENERATOR_EVENT_COUNT:-5}"

STARTED_PIDS=()
FAILURE_MESSAGE=""

GREEN='\033[0;32m'
RED='\033[0;31m'
RESET='\033[0m'

cleanup() {
  local exit_code=$?
  for process_id in "${STARTED_PIDS[@]:-}"; do
    if kill -0 "${process_id}" >/dev/null 2>&1; then
      kill "${process_id}" >/dev/null 2>&1 || true
      wait "${process_id}" 2>/dev/null || true
    fi
  done

  if [[ "${exit_code}" -eq 0 ]]; then
    printf "${GREEN}✓ Full flow test passed${RESET}\n"
  else
    if [[ -z "${FAILURE_MESSAGE}" ]]; then
      FAILURE_MESSAGE="full flow test failed"
    fi
    printf "${RED}✗ %s${RESET}\n" "${FAILURE_MESSAGE}"
  fi

  exit "${exit_code}"
}

trap cleanup EXIT

log() {
  printf '[full-flow] %s\n' "$1"
}

fail_with_message() {
  FAILURE_MESSAGE="$1"
  return 1
}

cleanup_service_ports() {
  if [[ ! -x "${KILL_PORT_SCRIPT}" ]]; then
    log "service port cleanup script not found or not executable: ${KILL_PORT_SCRIPT}"
    fail_with_message "service port cleanup script is unavailable"
  fi

  log "freeing local service ports before bootstrap"
  NOTIFIER_PORT="${NOTIFIER_HTTP_ADDRESS#:}" \
  GENERATOR_PORT="${GENERATOR_HTTP_ADDRESS#:}" \
  RECEIVER_PORT="${RECEIVER_HTTP_ADDRESS#:}" \
  "${KILL_PORT_SCRIPT}"
}

wait_for_http_ok() {
  local name="$1"
  local url="$2"
  local attempts="${3:-20}"

  for ((attempt=1; attempt<=attempts; attempt++)); do
    if curl --max-time 2 -fsS "${url}" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done

  log "timed out waiting for ${name} at ${url}"
  return 1
}

start_go_service_if_needed() {
  local name="$1"
  local health_url="$2"
  local log_file="$3"
  shift 3

  if curl -fsS "${health_url}" >/dev/null 2>&1; then
    log "${name} already healthy at ${health_url}"
    return 0
  fi

  log "starting ${name}"
  (
    cd "${ROOT_DIR}"
    env "$@" >"${log_file}" 2>&1
  ) &
  local process_id=$!
  STARTED_PIDS+=("${process_id}")

  if ! wait_for_http_ok "${name}" "${health_url}" 20; then
    log "${name} log output:"
    tail -n 120 "${log_file}" || true
    fail_with_message "${name} did not become healthy at ${health_url}"
  fi

  log "${name} is healthy"
}

bootstrap_database() {
  log "ensuring PostgreSQL database exists"

  local database_exists
  database_exists="$(
    psql "${POSTGRES_ADMIN_DSN}" -tAc "SELECT 1 FROM pg_database WHERE datname = '${WEBHOOK_NOTIFIER_DB_NAME}'"
  )"

  if [[ "${database_exists}" != "1" ]]; then
    psql "${POSTGRES_ADMIN_DSN}" -c "CREATE DATABASE ${WEBHOOK_NOTIFIER_DB_NAME}"
  fi

  log "creating webhook registration table and seed rows"
  psql "${NOTIFIER_POSTGRES_DSN}" <<SQL
CREATE TABLE IF NOT EXISTS webhook_registrations (
  customer_id TEXT NOT NULL,
  webhook_url TEXT NOT NULL,
  is_active BOOLEAN NOT NULL DEFAULT TRUE
);

DELETE FROM webhook_registrations
WHERE customer_id IN ('customer-a', 'customer-b', 'customer-c');

INSERT INTO webhook_registrations (customer_id, webhook_url, is_active)
VALUES
  ('customer-a', '${RECEIVER_BASE_URL}/webhook/customer-a', TRUE),
  ('customer-b', '${RECEIVER_BASE_URL}/webhook/customer-b', TRUE),
  ('customer-c', '${RECEIVER_BASE_URL}/webhook/customer-c', TRUE);
SQL
}

ensure_postgres_port() {
  local postgres_host="${POSTGRES_LOCAL_ADDRESS%%:*}"
  local postgres_port="${POSTGRES_LOCAL_ADDRESS##*:}"

  if nc -z "${postgres_host}" "${postgres_port}" >/dev/null 2>&1; then
    log "PostgreSQL already reachable at ${POSTGRES_LOCAL_ADDRESS}"
    return 0
  fi

  log "starting PostgreSQL port-forward to ${POSTGRES_LOCAL_ADDRESS}"
  kubectl port-forward -n "${POSTGRES_PORT_FORWARD_NAMESPACE}" "${POSTGRES_PORT_FORWARD_TARGET}" "${postgres_port}:5432" >"${LOG_DIR}/postgres-port-forward.log" 2>&1 &
  local process_id=$!
  STARTED_PIDS+=("${process_id}")

  sleep 1
  if nc -z "${postgres_host}" "${postgres_port}" >/dev/null 2>&1; then
    log "PostgreSQL port-forward is ready"
    return 0
  fi

  log "PostgreSQL connection error: could not reach ${POSTGRES_LOCAL_ADDRESS} after 1s"
  log "PostgreSQL port-forward log output:"
  tail -n 120 "${LOG_DIR}/postgres-port-forward.log" || true
  fail_with_message "PostgreSQL connection error: could not reach ${POSTGRES_LOCAL_ADDRESS} after 1s"
}

reset_receiver_stats() {
  curl -fsS -X POST "${RECEIVER_BASE_URL}/stats/reset" >/dev/null
}

read_json_number_field() {
  local json_payload="$1"
  local field_name="$2"

  sed -n "s/.*\"${field_name}\":\([0-9][0-9]*\).*/\1/p" <<<"${json_payload}" | head -n 1
}

read_last_event_field() {
  local json_payload="$1"
  local field_name="$2"

  sed -n "s/.*\"lastEvent\":{[^}]*\"${field_name}\":\"\([^\"]*\)\".*/\1/p" <<<"${json_payload}" | head -n 1
}

read_event_type_count() {
  local json_payload="$1"
  local event_type="$2"

  sed -n "s/.*\"${event_type}\":\([0-9][0-9]*\).*/\1/p" <<<"${json_payload}" | head -n 1
}

generate_events() {
  log "sending test events through generator"
  local response_file="${LOG_DIR}/generator-response.json"
  local http_status
  http_status="$(
    curl -sS -o "${response_file}" -w '%{http_code}' -X POST "${GENERATOR_BASE_URL}/generate" \
    -H 'Content-Type: application/json' \
    -d "{
      \"customerId\": \"${GENERATOR_CUSTOMER_ID}\",
      \"eventType\": \"${GENERATOR_EVENT_TYPE}\",
      \"count\": ${GENERATOR_EVENT_COUNT}
    }"
  )"

  if [[ "${http_status}" != "202" ]]; then
    log "generator returned HTTP ${http_status}"
    cat "${response_file}" || true
    printf '\n' || true
    log "mock generator log output:"
    tail -n 120 "${LOG_DIR}/mock-generator.log" || true
    log "notifier log output:"
    tail -n 120 "${LOG_DIR}/notifier.log" || true
    fail_with_message "generator returned HTTP ${http_status}"
  fi
}

assert_receiver_received_events() {
  log "waiting for receiver to observe webhook deliveries"

  for ((attempt=1; attempt<=45; attempt++)); do
    local stats_json
    stats_json="$(curl -fsS "${RECEIVER_BASE_URL}/stats")"
    local received_count
    received_count="$(printf '%s' "${stats_json}" | sed -n 's/.*"received":\([0-9][0-9]*\).*/\1/p')"

    if [[ -n "${received_count}" && "${received_count}" -ge "${GENERATOR_EVENT_COUNT}" ]]; then
      log "receiver observed ${received_count} webhook requests"
      return 0
    fi

    sleep 1
  done

  log "receiver stats at failure:"
  curl -fsS "${RECEIVER_BASE_URL}/stats" || true
  log "notifier stats at failure:"
  curl -fsS "${NOTIFIER_BASE_URL}/stats" || true
  fail_with_message "receiver did not observe ${GENERATOR_EVENT_COUNT} webhook requests in time"
}

assert_customer_delivery_stats() {
  local customer_id="$1"
  local expected_received="$2"
  local expected_event_type="${3:-}"
  local customer_stats_json

  customer_stats_json="$(curl -fsS "${RECEIVER_BASE_URL}/stats/customer/${customer_id}")"

  local received_count
  received_count="$(read_json_number_field "${customer_stats_json}" "received")"
  received_count="${received_count:-0}"
  if [[ "${received_count}" -ne "${expected_received}" ]]; then
    log "unexpected receiver stats for ${customer_id}: ${customer_stats_json}"
    fail_with_message "expected ${expected_received} deliveries for ${customer_id}, got ${received_count}"
  fi

  local payload_decode_failures
  payload_decode_failures="$(read_json_number_field "${customer_stats_json}" "payloadDecodeFailures")"
  payload_decode_failures="${payload_decode_failures:-0}"
  if [[ "${payload_decode_failures}" -ne 0 ]]; then
    log "unexpected receiver stats for ${customer_id}: ${customer_stats_json}"
    fail_with_message "receiver could not decode ${payload_decode_failures} payloads for ${customer_id}"
  fi

  local customer_mismatches
  customer_mismatches="$(read_json_number_field "${customer_stats_json}" "pathPayloadCustomerMismatches")"
  customer_mismatches="${customer_mismatches:-0}"
  if [[ "${customer_mismatches}" -ne 0 ]]; then
    log "unexpected receiver stats for ${customer_id}: ${customer_stats_json}"
    fail_with_message "receiver observed ${customer_mismatches} customer mismatches for ${customer_id}"
  fi

  if [[ -n "${expected_event_type}" && "${expected_received}" -gt 0 ]]; then
    local event_type_count
    event_type_count="$(read_event_type_count "${customer_stats_json}" "${expected_event_type}")"
    event_type_count="${event_type_count:-0}"
    if [[ "${event_type_count}" -ne "${expected_received}" ]]; then
      log "unexpected receiver stats for ${customer_id}: ${customer_stats_json}"
      fail_with_message "expected ${expected_received} ${expected_event_type} payloads for ${customer_id}, got ${event_type_count}"
    fi

    local last_event_customer_id
    last_event_customer_id="$(read_last_event_field "${customer_stats_json}" "customerId")"
    if [[ "${last_event_customer_id}" != "${customer_id}" ]]; then
      log "unexpected receiver stats for ${customer_id}: ${customer_stats_json}"
      fail_with_message "expected last payload customerId ${customer_id}, got ${last_event_customer_id:-<empty>}"
    fi

    local last_event_type
    last_event_type="$(read_last_event_field "${customer_stats_json}" "eventType")"
    if [[ "${last_event_type}" != "${expected_event_type}" ]]; then
      log "unexpected receiver stats for ${customer_id}: ${customer_stats_json}"
      fail_with_message "expected last payload eventType ${expected_event_type}, got ${last_event_type:-<empty>}"
    fi
  fi
}

main() {
  cleanup_service_ports
  ensure_postgres_port
  bootstrap_database

  start_go_service_if_needed \
    "mock receiver" \
    "${RECEIVER_BASE_URL}/health" \
    "${LOG_DIR}/mock-receiver.log" \
    RECEIVER_HTTP_ADDRESS="${RECEIVER_HTTP_ADDRESS}" \
    go run ./cmd/mock-webhook-receiver

  start_go_service_if_needed \
    "notifier" \
    "${NOTIFIER_BASE_URL}/health" \
    "${LOG_DIR}/notifier.log" \
    NOTIFIER_HTTP_ADDRESS="${NOTIFIER_HTTP_ADDRESS}" \
    NOTIFIER_POSTGRES_DSN="${NOTIFIER_POSTGRES_DSN}" \
    go run ./cmd/notifier

  start_go_service_if_needed \
    "mock generator" \
    "${GENERATOR_BASE_URL}/health" \
    "${LOG_DIR}/mock-generator.log" \
    GENERATOR_HTTP_ADDRESS="${GENERATOR_HTTP_ADDRESS}" \
    GENERATOR_NOTIFIER_BASE_URL="${NOTIFIER_BASE_URL}" \
    go run ./cmd/mock-event-generator

  reset_receiver_stats
  generate_events
  assert_receiver_received_events
  assert_customer_delivery_stats "customer-a" "${GENERATOR_EVENT_COUNT}" "${GENERATOR_EVENT_TYPE}"
  assert_customer_delivery_stats "customer-b" 0
  assert_customer_delivery_stats "customer-c" 0

  log "full flow test passed"
}

main "$@"
