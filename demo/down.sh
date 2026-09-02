#!/usr/bin/env bash
set -euo pipefail

repository_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
compose_file="${repository_root}/demo/docker-compose.yml"
runtime_directory="${repository_root}/.cache/ledger-local"

fail() {
  printf 'error: %s\n' "$1" >&2
  exit 1
}

if [[ "$#" -ne 0 ]]; then
  fail "make down accepts no arguments"
fi
if [[ "${LEDGER_LOCAL_TEST_MODE:-0}" == 1 ]]; then
  [[ -n "${LEDGER_LOCAL_RUNTIME_DIR:-}" ]] || fail "LEDGER_LOCAL_RUNTIME_DIR is required in local lifecycle test mode"
  runtime_directory="${LEDGER_LOCAL_RUNTIME_DIR}"
elif [[ -n "${LEDGER_LOCAL_RUNTIME_DIR:-}" ]]; then
  fail "LEDGER_LOCAL_RUNTIME_DIR is reserved for the local lifecycle test harness"
fi

[[ -f "${compose_file}" ]] || fail "missing local Compose file: ${compose_file}"
command -v docker >/dev/null 2>&1 || fail "docker is required to stop Ledger"
docker compose version >/dev/null 2>&1 || fail "docker compose is required to stop Ledger"

ledger_environment_file="${runtime_directory}/ledger.env"
tauth_environment_file="${runtime_directory}/tauth.env"
if [[ ! -f "${ledger_environment_file}" ]]; then
  ledger_environment_file="/dev/null"
fi
if [[ ! -f "${tauth_environment_file}" ]]; then
  tauth_environment_file="/dev/null"
fi
export LEDGER_LOCAL_LEDGER_ENV_FILE="${ledger_environment_file}"
export LEDGER_LOCAL_TAUTH_ENV_FILE="${tauth_environment_file}"

docker compose \
  --file "${compose_file}" \
  --project-name ledger-local \
  down --remove-orphans

printf 'Ledger local runtime stopped. Local data remains available for the next start.\n'
