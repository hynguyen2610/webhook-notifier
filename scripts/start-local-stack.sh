#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LOG_DIR="${ROOT_DIR}/.tmp/local-stack"
mkdir -p "${LOG_DIR}"

KILL_PORT_SCRIPT="${ROOT_DIR}/scripts/kill-local-service-ports.sh"
PORT_FORWARD_SCRIPT="${ROOT_DIR}/scripts/ensure-local-port-forwards.sh"

POSTGRES_LOCAL_ADDRESS="${POSTGRES_LOCAL_ADDRESS:-127.0.0.1:15432}"
POSTGRES_ADMIN_DSN="${POSTGRES_ADMIN_DSN:-postgres://postgres:password@127.0.0.1:15432/postgres?sslmode=disable}"
WEBHOOK_NOTIFIER_DB_NAME="${WEBHOOK_NOTIFIER_DB_NAME:-webhook_notifier}"
NOTIFIER_POSTGRES_DSN="${NOTIFIER_POSTGRES_DSN:-postgres://postgres:password@127.0.0.1:15432/${WEBHOOK_NOTIFIER_DB_NAME}?sslmode=disable}"

KAFKA_LOCAL_ADDRESS="${KAFKA_LOCAL_ADDRESS:-127.0.0.1:9092}"
KAFKA_HOST_OVERRIDES="${KAFKA_HOST_OVERRIDES:-kafka-service=127.0.0.1,kafka-service.default.svc.cluster.local=127.0.0.1}"
NOTIFIER_TOPIC="${NOTIFIER_TOPIC:-subscriber-events}"
NOTIFIER_DLQ_TOPIC="${NOTIFIER_DLQ_TOPIC:-subscriber-events-dlq}"

NOTIFIER_HTTP_ADDRESS="${NOTIFIER_HTTP_ADDRESS:-:28080}"
GENERATOR_HTTP_ADDRESS="${GENERATOR_HTTP_ADDRESS:-:28081}"
RECEIVER_HTTP_ADDRESS="${RECEIVER_HTTP_ADDRESS:-:28082}"

NOTIFIER_BASE_URL="${NOTIFIER_BASE_URL:-http://localhost:28080}"
GENERATOR_BASE_URL="${GENERATOR_BASE_URL:-http://localhost:28081}"
RECEIVER_BASE_URL="${RECEIVER_BASE_URL:-http://localhost:28082}"

STARTED_PIDS=()

RED='\033[0;31m'
LIME='\033[0;32m'
YELLOW='\033[1;33m'
RESET='\033[0m'

cleanup() {
  local exit_code=$?
  for process_id in "${STARTED_PIDS[@]:-}"; do
    if kill -0 "${process_id}" >/dev/null 2>&1; then
      kill "${process_id}" >/dev/null 2>&1 || true
      wait "${process_id}" 2>/dev/null || true
    fi
  done
  exit "${exit_code}"
}

trap cleanup EXIT

log() {
  printf '[local-stack] %s\n' "$1"
}

health_status_label() {
  local service_name="$1"
  local health_url="$2"

  if curl --max-time 1 -fsS "${health_url}" >/dev/null 2>&1; then
    printf '%b%s:UP%b' "${LIME}" "${service_name}" "${RESET}"
    return 0
  fi

  printf '%b%s:DOWN%b' "${RED}" "${service_name}" "${RESET}"
  return 1
}

print_health_summary() {
  local receiver_status
  local notifier_status
  local generator_status

  receiver_status="$(health_status_label "receiver" "${RECEIVER_BASE_URL}/health")"
  notifier_status="$(health_status_label "notifier" "${NOTIFIER_BASE_URL}/health")"
  generator_status="$(health_status_label "generator" "${GENERATOR_BASE_URL}/health")"

  printf '\r[local-stack] health %s  %s  %s  %bupdated:%s%b' \
    "${receiver_status}" \
    "${notifier_status}" \
    "${generator_status}" \
    "${YELLOW}" \
    "$(date '+%H:%M:%S')" \
    "${RESET}"
}

wait_for_http_ok() {
  local service_name="$1"
  local health_url="$2"
  local attempts="${3:-20}"

  for ((attempt=1; attempt<=attempts; attempt++)); do
    if curl --max-time 2 -fsS "${health_url}" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done

  log "timed out waiting for ${service_name} at ${health_url}"
  return 1
}

start_go_service() {
  local service_name="$1"
  local health_url="$2"
  local log_file="$3"
  shift 3

  if curl -fsS "${health_url}" >/dev/null 2>&1; then
    log "${service_name} already healthy at ${health_url}"
    return 0
  fi

  log "starting ${service_name}"
  (
    cd "${ROOT_DIR}"
    env "$@" >"${log_file}" 2>&1
  ) &
  local process_id=$!
  STARTED_PIDS+=("${process_id}")

  if ! wait_for_http_ok "${service_name}" "${health_url}" 20; then
    log "${service_name} log output:"
    tail -n 120 "${log_file}" || true
    return 1
  fi

  log "${service_name} is healthy"
}

cleanup_service_ports() {
  log "freeing local service ports before startup"
  NOTIFIER_PORT="${NOTIFIER_HTTP_ADDRESS#:}" \
  GENERATOR_PORT="${GENERATOR_HTTP_ADDRESS#:}" \
  RECEIVER_PORT="${RECEIVER_HTTP_ADDRESS#:}" \
  "${KILL_PORT_SCRIPT}"
}

start_port_forwards() {
  log "ensuring PostgreSQL and Kafka port-forwards"
  (
    cd "${ROOT_DIR}"
    KEEP_RUNNING=true "${PORT_FORWARD_SCRIPT}" >"${LOG_DIR}/port-forwards.log" 2>&1
  ) &
  local process_id=$!
  STARTED_PIDS+=("${process_id}")

  if ! wait_for_tcp "${POSTGRES_LOCAL_ADDRESS}" 20; then
    log "port-forward log output:"
    tail -n 120 "${LOG_DIR}/port-forwards.log" || true
    return 1
  fi
  if ! wait_for_tcp "${KAFKA_LOCAL_ADDRESS}" 20; then
    log "port-forward log output:"
    tail -n 120 "${LOG_DIR}/port-forwards.log" || true
    return 1
  fi
}

wait_for_tcp() {
  local address="$1"
  local attempts="${2:-20}"
  local host="${address%%:*}"
  local port="${address##*:}"

  for ((attempt=1; attempt<=attempts; attempt++)); do
    if nc -z "${host}" "${port}" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done

  log "timed out waiting for TCP address ${address}"
  return 1
}

bootstrap_database() {
  log "ensuring PostgreSQL database and registration rows"

  local database_exists
  database_exists="$(
    psql "${POSTGRES_ADMIN_DSN}" -tAc "SELECT 1 FROM pg_database WHERE datname = '${WEBHOOK_NOTIFIER_DB_NAME}'"
  )"

  if [[ "${database_exists}" != "1" ]]; then
    psql "${POSTGRES_ADMIN_DSN}" -c "CREATE DATABASE ${WEBHOOK_NOTIFIER_DB_NAME}"
  fi

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

show_summary() {
  cat <<EOF

Local stack is running.

Endpoints:
- Receiver: ${RECEIVER_BASE_URL}
- Notifier: ${NOTIFIER_BASE_URL}
- Generator: ${GENERATOR_BASE_URL}

Logs:
- ${LOG_DIR}/port-forwards.log
- ${LOG_DIR}/mock-receiver.log
- ${LOG_DIR}/notifier.log
- ${LOG_DIR}/mock-generator.log

Live health checks run every 1 second below.
Press Ctrl+C to stop the services and port-forwards started by this script.
EOF
}

main() {
  cleanup_service_ports
  start_port_forwards
  bootstrap_database

  start_go_service \
    "mock receiver" \
    "${RECEIVER_BASE_URL}/health" \
    "${LOG_DIR}/mock-receiver.log" \
    RECEIVER_HTTP_ADDRESS="${RECEIVER_HTTP_ADDRESS}" \
    go run ./cmd/mock-webhook-receiver

  start_go_service \
    "notifier" \
    "${NOTIFIER_BASE_URL}/health" \
    "${LOG_DIR}/notifier.log" \
    NOTIFIER_HTTP_ADDRESS="${NOTIFIER_HTTP_ADDRESS}" \
    NOTIFIER_POSTGRES_DSN="${NOTIFIER_POSTGRES_DSN}" \
    NOTIFIER_KAFKA_BROKERS="${KAFKA_LOCAL_ADDRESS}" \
    NOTIFIER_KAFKA_HOST_OVERRIDES="${KAFKA_HOST_OVERRIDES}" \
    NOTIFIER_KAFKA_TOPIC="${NOTIFIER_TOPIC}" \
    NOTIFIER_KAFKA_DLQ_TOPIC="${NOTIFIER_DLQ_TOPIC}" \
    go run ./cmd/notifier

  start_go_service \
    "mock generator" \
    "${GENERATOR_BASE_URL}/health" \
    "${LOG_DIR}/mock-generator.log" \
    GENERATOR_HTTP_ADDRESS="${GENERATOR_HTTP_ADDRESS}" \
    GENERATOR_KAFKA_BROKERS="${KAFKA_LOCAL_ADDRESS}" \
    GENERATOR_KAFKA_HOST_OVERRIDES="${KAFKA_HOST_OVERRIDES}" \
    GENERATOR_KAFKA_TOPIC="${NOTIFIER_TOPIC}" \
    GENERATOR_NOTIFIER_BASE_URL="${NOTIFIER_BASE_URL}" \
    go run ./cmd/mock-event-generator

  show_summary

  while true; do
    print_health_summary
    sleep 1
  done
}

main "$@"
