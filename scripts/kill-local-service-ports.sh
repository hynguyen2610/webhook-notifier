#!/usr/bin/env bash
set -euo pipefail

NOTIFIER_PORT="${NOTIFIER_PORT:-28080}"
GENERATOR_PORT="${GENERATOR_PORT:-28081}"
RECEIVER_PORT="${RECEIVER_PORT:-28082}"

log() {
  printf '[kill-ports] %s\n' "$1"
}

kill_port_if_needed() {
  local port="$1"
  local process_ids

  process_ids="$(lsof -ti tcp:"${port}" || true)"
  if [[ -z "${process_ids}" ]]; then
    log "no process is listening on port ${port}"
    return 0
  fi

  log "killing process(es) on port ${port}: ${process_ids}"
  kill ${process_ids}

  sleep 1
  local remaining_process_ids
  remaining_process_ids="$(lsof -ti tcp:"${port}" || true)"
  if [[ -n "${remaining_process_ids}" ]]; then
    log "force killing remaining process(es) on port ${port}: ${remaining_process_ids}"
    kill -9 ${remaining_process_ids}
  fi
}

main() {
  kill_port_if_needed "${NOTIFIER_PORT}"
  kill_port_if_needed "${GENERATOR_PORT}"
  kill_port_if_needed "${RECEIVER_PORT}"
  log "done"
}

main "$@"
