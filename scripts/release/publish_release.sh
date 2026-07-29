#!/usr/bin/env bash
set -euo pipefail

if [[ -v RELEASE_HELPER ]]; then
  helper="${RELEASE_HELPER}"
else
  helper=""
fi
if [[ -z "${helper}" ]]; then
  helper="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/release_helper.py"
fi
command -v python3 >/dev/null 2>&1 || { echo "error: python3 is required" >&2; exit 1; }
[[ -f "${helper}" ]] || { echo "error: release helper does not exist: ${helper}" >&2; exit 1; }

repo_root="$(git rev-parse --show-toplevel)"
artifact_dir="$(git rev-parse --git-path mprlab-release)"
[[ "${artifact_dir}" == /* ]] || artifact_dir="${repo_root}/${artifact_dir}"
if [[ ! -d "${artifact_dir}" ]] || [[ -z "$(find "${artifact_dir}" -mindepth 1 -print -quit 2>/dev/null)" ]]; then
  exec python3 "${helper}" verify-published-release-at-head "$@"
fi

exec python3 "${helper}" publish-prepared-release "$@"
