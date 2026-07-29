#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage:
  scripts/deploy.sh [options]

Deploys the published Ledger image through the dedicated mprlab-gateway
Ledger profile after verifying that the release image has been published.

Options:
  --gateway-dir <path>  Gateway checkout. Default: $GATEWAY_DIR or sibling ../mprlab-gateway
  --image <value>       Image repository. Default: $DOCKER_IMAGE or ghcr.io/tyemirov/ledger
  --tag <value>         Release tag. Default: v* tag pointing at HEAD
  --skip-image-verify   Skip release/latest image digest verification
  --skip-backend        Skip gateway deployment
  --help                Show this help text
USAGE
}

env_or_default() {
  local name="$1"
  local fallback="$2"
  local value=""
  if [[ -v "${name}" ]]; then
    value="${!name}"
  fi
  if [[ -n "${value}" ]]; then
    printf "%s\n" "${value}"
  else
    printf "%s\n" "${fallback}"
  fi
}

GATEWAY_DIR="$(env_or_default GATEWAY_DIR "")"
IMAGE_REPOSITORY="$(env_or_default DOCKER_IMAGE ghcr.io/tyemirov/ledger)"
TAG="$(env_or_default DEPLOY_TAG "")"
SKIP_IMAGE_VERIFY="false"
SKIP_BACKEND="false"
DEFAULT_BRANCH="master"
temporary_directory="$(mktemp -d)"
cleanup() {
  rm -rf "${temporary_directory}"
}
trap cleanup EXIT

inspect_digest() {
  local image_ref="$1"
  local error_file="$2"
  local inspect_output
  local inspect_status
  local digest

  if inspect_output="$(timeout -k 30s -s SIGKILL 30s docker buildx imagetools inspect "${image_ref}" 2>"${error_file}")"; then
    digest="$(awk '/^Digest:/ {print $2; exit}' <<<"${inspect_output}")"
    [[ "${digest}" =~ ^sha256:[0-9a-f]{64}$ ]] || {
      echo "error: registry returned no valid digest for ${image_ref}" >&2
      return 1
    }
    printf '%s\n' "${digest}"
    return 0
  else
    inspect_status="$?"
  fi

  if grep -Eqi 'not found|manifest unknown|name unknown|no such manifest' "${error_file}"; then
    return 2
  fi
  cat "${error_file}" >&2
  echo "error: registry lookup failed for ${image_ref} with status ${inspect_status}" >&2
  return 1
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --gateway-dir)
      [[ $# -ge 2 ]] || { echo "error: --gateway-dir requires a value" >&2; exit 1; }
      GATEWAY_DIR="$2"
      shift 2
      ;;
    --image)
      [[ $# -ge 2 ]] || { echo "error: --image requires a value" >&2; exit 1; }
      IMAGE_REPOSITORY="$2"
      shift 2
      ;;
    --tag)
      [[ $# -ge 2 ]] || { echo "error: --tag requires a value" >&2; exit 1; }
      TAG="$2"
      shift 2
      ;;
    --skip-image-verify)
      SKIP_IMAGE_VERIFY="true"
      shift
      ;;
    --skip-backend)
      SKIP_BACKEND="true"
      shift
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      echo "error: unknown argument: $1" >&2
      usage
      exit 1
      ;;
  esac
done

repo_root="$(git rev-parse --show-toplevel)"
cd "${repo_root}"
if [[ -z "${GATEWAY_DIR}" ]]; then
  GATEWAY_DIR="${repo_root}/../mprlab-gateway"
fi
[[ -d "${GATEWAY_DIR}" ]] || { echo "error: gateway checkout not found: ${GATEWAY_DIR}" >&2; exit 1; }

if [[ "${SKIP_BACKEND}" != "true" ]]; then
  timeout -k 30s -s SIGKILL 30s git fetch origin "${DEFAULT_BRANCH}" --tags
  current_branch="$(git rev-parse --abbrev-ref HEAD)"
  [[ "${current_branch}" == "${DEFAULT_BRANCH}" ]] || { echo "error: deployment is allowed only from branch '${DEFAULT_BRANCH}' (current: '${current_branch}')" >&2; exit 1; }
  [[ -z "$(git status --porcelain)" ]] || { echo "error: working tree is dirty; commit or stash changes before deploying" >&2; exit 1; }
  head_sha="$(git rev-parse HEAD)"
  remote_sha="$(git rev-parse "origin/${DEFAULT_BRANCH}")"
  [[ "${head_sha}" == "${remote_sha}" ]] || { echo "error: local ${DEFAULT_BRANCH} is not at origin/${DEFAULT_BRANCH}; pull/push first" >&2; exit 1; }
  if [[ -z "${TAG}" ]]; then
    TAG="$(git tag --points-at HEAD --list 'v*' --sort=-version:refname | head -n 1)"
  fi
  [[ -n "${TAG}" ]] || { echo "error: no v* release tag points at HEAD; run make publish first" >&2; exit 1; }
  [[ "${TAG}" == v* ]] || { echo "error: deploy tag must be a v* release tag (got: ${TAG})" >&2; exit 1; }
  tag_sha="$(git rev-list -n 1 "${TAG}" 2>/dev/null || true)"
  [[ "${tag_sha}" == "${head_sha}" ]] || { echo "error: deploy tag ${TAG} does not point at HEAD; run make publish first" >&2; exit 1; }
fi

if [[ "${SKIP_IMAGE_VERIFY}" != "true" ]]; then
  if [[ -z "${TAG}" ]]; then
    TAG="$(git tag --points-at HEAD --list 'v*' --sort=-version:refname | head -n 1)"
  fi
  [[ -n "${TAG}" ]] || { echo "error: no v* release tag points at HEAD; run make publish first" >&2; exit 1; }
  command -v docker >/dev/null 2>&1 || { echo "error: docker is required for image verification" >&2; exit 1; }
  docker buildx version >/dev/null 2>&1 || { echo "error: docker buildx is required for image verification" >&2; exit 1; }
  echo "==> [deploy] Verifying ${IMAGE_REPOSITORY}:latest matches release ${TAG}"
  if release_digest="$(inspect_digest "${IMAGE_REPOSITORY}:${TAG}" "${temporary_directory}/release-image.error")"; then
    :
  else
    inspect_status="$?"
    if [[ "${inspect_status}" -eq 2 ]]; then
      echo "error: ${IMAGE_REPOSITORY}:${TAG} is not published; run make publish" >&2
    fi
    exit 1
  fi
  if latest_digest="$(inspect_digest "${IMAGE_REPOSITORY}:latest" "${temporary_directory}/latest-image.error")"; then
    :
  else
    inspect_status="$?"
    if [[ "${inspect_status}" -eq 2 ]]; then
      echo "error: ${IMAGE_REPOSITORY}:latest is not published; run make publish" >&2
    fi
    exit 1
  fi
  [[ "${release_digest}" == "${latest_digest}" ]] || { echo "error: ${IMAGE_REPOSITORY}:latest does not match ${TAG}; run make publish first" >&2; exit 1; }
fi

if [[ "${SKIP_BACKEND}" != "true" ]]; then
  echo "==> [deploy] Deploying Ledger through mprlab-gateway"
  timeout --foreground -k 1200s -s SIGKILL 1200s make -C "${GATEWAY_DIR}" deploy-ledger-backend
fi

echo "Ledger deploy complete"
