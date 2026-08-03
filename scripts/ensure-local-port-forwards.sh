#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LOG_DIR="${ROOT_DIR}/.tmp/port-forwards"
mkdir -p "${LOG_DIR}"

POSTGRES_LOCAL_ADDRESS="${POSTGRES_LOCAL_ADDRESS:-127.0.0.1:15432}"
POSTGRES_PORT_FORWARD_TARGET="${POSTGRES_PORT_FORWARD_TARGET:-svc/user-org-db-service}"
POSTGRES_PORT_FORWARD_NAMESPACE="${POSTGRES_PORT_FORWARD_NAMESPACE:-default}"

STARTED_PIDS=()

cleanup() {
  local exit_code=$?
  if [[ "${KEEP_RUNNING:-true}" != "true" ]]; then
    for process_id in "${STARTED_PIDS[@]:-}"; do
      if kill -0 "${process_id}" >/dev/null 2>&1; then
        kill "${process_id}" >/dev/null 2>&1 || true
        wait "${process_id}" 2>/dev/null || true
      fi
    done
  fi
  exit "${exit_code}"
}

trap cleanup EXIT

log() {
  printf '[port-forward] %s\n' "$1"
}

ensure_port_forward() {
  local component_name="$1"
  local local_address="$2"
  local namespace="$3"
  local target="$4"
  local remote_port="$5"
  local log_file="$6"

  local local_host="${local_address%%:*}"
  local local_port="${local_address##*:}"

  if nc -z "${local_host}" "${local_port}" >/dev/null 2>&1; then
    log "${component_name} already reachable at ${local_address}"
    return 0
  fi

  log "starting ${component_name} port-forward to ${local_address}"
  kubectl port-forward -n "${namespace}" "${target}" "${local_port}:${remote_port}" >"${log_file}" 2>&1 &
  local process_id=$!
  STARTED_PIDS+=("${process_id}")

  sleep 1
  if nc -z "${local_host}" "${local_port}" >/dev/null 2>&1; then
    log "${component_name} port-forward is ready"
    return 0
  fi

  log "${component_name} connection error: could not reach ${local_address} after 1s"
  log "${component_name} port-forward log output:"
  tail -n 120 "${log_file}" || true
  return 1
}

main() {
  ensure_port_forward \
    "PostgreSQL" \
    "${POSTGRES_LOCAL_ADDRESS}" \
    "${POSTGRES_PORT_FORWARD_NAMESPACE}" \
    "${POSTGRES_PORT_FORWARD_TARGET}" \
    "5432" \
    "${LOG_DIR}/postgres-port-forward.log"

  log "local port-forward is ready"
  log "PostgreSQL: ${POSTGRES_LOCAL_ADDRESS}"

  if [[ "${KEEP_RUNNING:-true}" == "true" ]]; then
    log "KEEP_RUNNING=true, leaving port-forward process attached to this script"
    while true; do
      sleep 60
    done
  fi
}

main "$@"
