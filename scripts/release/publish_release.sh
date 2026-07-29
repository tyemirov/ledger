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
[[ -x "${helper}" ]] || { echo "error: release helper is not executable: ${helper}" >&2; exit 1; }

repo_root="$(git rev-parse --show-toplevel)"
artifact_dir="$(git rev-parse --git-path mprlab-release)"
[[ "${artifact_dir}" == /* ]] || artifact_dir="${repo_root}/${artifact_dir}"
if [[ ! -d "${artifact_dir}" ]] || [[ -z "$(find "${artifact_dir}" -mindepth 1 -print -quit 2>/dev/null)" ]]; then
  exec "${helper}" verify-published-release-at-head "$@"
fi

exec "${helper}" publish-prepared-release "$@"
