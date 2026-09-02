#!/usr/bin/env bash
set -euo pipefail

repository_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
compose_file="${repository_root}/demo/docker-compose.yml"
runtime_directory="${repository_root}/.cache/ledger-local"
ledger_environment_file="${runtime_directory}/ledger.env"
tauth_environment_file="${runtime_directory}/tauth.env"
signing_key_file="${runtime_directory}/session-signing-key"
compose_project="ledger-local"
public_origin="http://localhost:8000"
tauth_tenant_id="ledger-local"
session_cookie_name="ledger_local_session"
refresh_cookie_name="ledger_local_refresh"
google_client_id="611549676198-d8800qv64voofseor1qod1euto5duivu.apps.googleusercontent.com"

fail() {
  printf 'error: %s\n' "$1" >&2
  exit 1
}

if [[ "$#" -ne 0 ]]; then
  fail "make up accepts no arguments"
fi
if [[ "${LEDGER_LOCAL_TEST_MODE:-0}" == 1 ]]; then
  [[ -n "${LEDGER_LOCAL_RUNTIME_DIR:-}" ]] || fail "LEDGER_LOCAL_RUNTIME_DIR is required in local lifecycle test mode"
  runtime_directory="${LEDGER_LOCAL_RUNTIME_DIR}"
  ledger_environment_file="${runtime_directory}/ledger.env"
  tauth_environment_file="${runtime_directory}/tauth.env"
  signing_key_file="${runtime_directory}/session-signing-key"
elif [[ -n "${LEDGER_LOCAL_RUNTIME_DIR:-}" ]]; then
  fail "LEDGER_LOCAL_RUNTIME_DIR is reserved for the local lifecycle test harness"
fi

[[ -f "${compose_file}" ]] || fail "missing local Compose file: ${compose_file}"
[[ -f "${repository_root}/demo/configs/config.yml" ]] || fail "missing local Ledger config"
[[ -f "${repository_root}/demo/configs/tauth.config.yaml" ]] || fail "missing local TAuth config"
command -v openssl >/dev/null 2>&1 || fail "openssl is required to generate the local signing key"
if [[ "${UP_DRY_RUN:-0}" != 1 ]]; then
  command -v docker >/dev/null 2>&1 || fail "docker is required to start Ledger"
  docker info >/dev/null 2>&1 || fail "docker is installed but the daemon is unavailable"
  docker compose version >/dev/null 2>&1 || fail "docker compose is required to start Ledger"
  command -v curl >/dev/null 2>&1 || fail "curl is required to verify Ledger"
fi

runtime_parent="$(dirname "${runtime_directory}")"
[[ ! -L "${runtime_parent}" && ( ! -e "${runtime_parent}" || -d "${runtime_parent}" ) ]] || fail "the local runtime parent must be a non-symlink directory"
[[ ! -L "${runtime_directory}" && ( ! -e "${runtime_directory}" || -d "${runtime_directory}" ) ]] || fail "the local runtime path must be a non-symlink directory"

umask 077
mkdir -p "${runtime_directory}"
chmod 700 "${runtime_directory}"
if [[ ! -f "${signing_key_file}" ]]; then
  signing_key_temporary_file="${signing_key_file}.tmp"
  openssl rand -base64 48 | tr -d '\r\n' >"${signing_key_temporary_file}"
  printf '\n' >>"${signing_key_temporary_file}"
  chmod 600 "${signing_key_temporary_file}"
  mv "${signing_key_temporary_file}" "${signing_key_file}"
fi
[[ ! -L "${signing_key_file}" && -s "${signing_key_file}" ]] || fail "the local signing key must be a nonempty regular file"
chmod 600 "${signing_key_file}"
session_signing_key="$(tr -d '\r\n' <"${signing_key_file}")"

ledger_environment_temporary_file="${ledger_environment_file}.tmp"
{
  printf 'DATABASE_URL=sqlite:///srv/data/ledger.db\n'
  printf 'LEDGER_PUBLIC_ORIGIN=%s\n' "${public_origin}"
  printf 'TAUTH_GOOGLE_CLIENT_ID=%s\n' "${google_client_id}"
  printf 'TAUTH_JWT_ISSUER=tauth\n'
  printf 'TAUTH_JWT_SIGNING_KEY=%s\n' "${session_signing_key}"
  printf 'TAUTH_LOGIN_PATH=/auth/google\n'
  printf 'TAUTH_LOGOUT_PATH=/auth/logout\n'
  printf 'TAUTH_NONCE_PATH=/auth/nonce\n'
  printf 'TAUTH_SESSION_COOKIE_NAME=%s\n' "${session_cookie_name}"
  printf 'TAUTH_SESSION_PATH=/auth/session\n'
  printf 'TAUTH_TENANT_ID=%s\n' "${tauth_tenant_id}"
  printf 'TAUTH_URL=%s\n' "${public_origin}"
} >"${ledger_environment_temporary_file}"
chmod 600 "${ledger_environment_temporary_file}"
mv "${ledger_environment_temporary_file}" "${ledger_environment_file}"

tauth_environment_temporary_file="${tauth_environment_file}.tmp"
{
  printf 'TAUTH_CONFIG_FILE=/config.yaml\n'
  printf 'TAUTH_DATABASE_URL=sqlite:///data/tauth.db\n'
  printf 'TAUTH_GOOGLE_CLIENT_ID=%s\n' "${google_client_id}"
  printf 'TAUTH_JWT_SIGNING_KEY=%s\n' "${session_signing_key}"
  printf 'TAUTH_PUBLIC_ORIGIN=%s\n' "${public_origin}"
  printf 'TAUTH_REFRESH_COOKIE_NAME=%s\n' "${refresh_cookie_name}"
  printf 'TAUTH_SESSION_COOKIE_NAME=%s\n' "${session_cookie_name}"
  printf 'TAUTH_TENANT_ID=%s\n' "${tauth_tenant_id}"
} >"${tauth_environment_temporary_file}"
chmod 600 "${tauth_environment_temporary_file}"
mv "${tauth_environment_temporary_file}" "${tauth_environment_file}"

export LEDGER_LOCAL_LEDGER_ENV_FILE="${ledger_environment_file}"
export LEDGER_LOCAL_TAUTH_ENV_FILE="${tauth_environment_file}"
compose_command=(docker compose --file "${compose_file}" --project-name "${compose_project}")

printf 'Ledger local runtime\n'
printf 'Browser URL: %s/\n' "${public_origin}"
printf 'gRPC address: localhost:50051\n'
printf 'Database: SQLite\n'
printf 'Local data: preserved by make down\n'

if [[ "${UP_DRY_RUN:-0}" == 1 ]]; then
  printf 'Dry run complete.\n'
  exit 0
fi

"${compose_command[@]}" down --remove-orphans
if ! "${compose_command[@]}" up --build --detach --remove-orphans --wait; then
  "${compose_command[@]}" logs --tail=200 >&2 || true
  fail "Ledger did not start"
fi

http_status() {
  curl --silent --output /dev/null --write-out '%{http_code}' --max-time 2 \
    --header "X-TAuth-Tenant: ${tauth_tenant_id}" "$1" 2>/dev/null || true
}

ready=0
for _ in {1..120}; do
  root_status="$(http_status "${public_origin}/")"
  health_status="$(http_status "${public_origin}/healthz")"
  config_status="$(http_status "${public_origin}/config-ui.yaml")"
  session_status="$(http_status "${public_origin}/auth/session")"
  tenant_status="$(http_status "${public_origin}/api/tenants?limit=50")"
  if [[ "${root_status}" == 200 && "${health_status}" == 200 && "${config_status}" == 200 && "${session_status}" == 204 && "${tenant_status}" == 401 ]]; then
    page_body="$(curl --silent --fail --max-time 2 "${public_origin}/" || true)"
    config_body="$(curl --silent --fail --max-time 2 "${public_origin}/config-ui.yaml" || true)"
    if [[ "${page_body}" == *'data-config-url="/config-ui.yaml"'* && "${page_body}" == *'mpr-ui@latest/mpr-ui.js'* && "${page_body}" != *'tauth.js'* && "${config_body}" == *"tenantId: \"${tauth_tenant_id}\""* && "${config_body}" == *"tauthUrl: \"${public_origin}\""* ]]; then
      ready=1
      break
    fi
  fi
  sleep 0.5
done

if [[ "${ready}" != 1 ]]; then
  "${compose_command[@]}" logs --tail=200 >&2 || true
  "${compose_command[@]}" down --remove-orphans >/dev/null 2>&1 || true
  fail "Ledger did not satisfy the local readiness contract"
fi

printf 'Ledger is ready.\n'
printf 'Browser URL: %s/\n' "${public_origin}"
printf 'gRPC address: localhost:50051\n'
printf 'Readiness: page=200, health=200, config=200, TAuth session=204, protected tenants=401.\n'
printf 'Stop the local runtime with make down.\n'
