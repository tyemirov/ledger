#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage:
  publish_container_artifacts.sh

Loads container archives prepared by make release, pushes only missing immutable
platform images, and creates missing version/latest manifests. If the local
release artifact is absent, verifies the already-published registry state.
It never builds an image.
USAGE
}

if [[ $# -gt 0 ]]; then
  case "$1" in --help|-h) usage; exit 0 ;; *) echo "error: no arguments are supported" >&2; exit 1 ;; esac
fi

command -v docker >/dev/null 2>&1 || { echo "error: docker is required" >&2; exit 1; }
command -v python3 >/dev/null 2>&1 || { echo "error: python3 is required" >&2; exit 1; }
docker buildx version >/dev/null 2>&1 || { echo "error: docker buildx is required" >&2; exit 1; }

repo_root="$(git rev-parse --show-toplevel)"
artifact_dir="$(git rev-parse --git-path mprlab-release)"
[[ "${artifact_dir}" == /* ]] || artifact_dir="${repo_root}/${artifact_dir}"
publish_timeout="${PUBLISH_CONTAINER_TIMEOUT_SECONDS:-1200}"
[[ "${publish_timeout}" =~ ^[1-9][0-9]*$ ]] || { echo "error: PUBLISH_CONTAINER_TIMEOUT_SECONDS must be a positive integer" >&2; exit 1; }
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

  if inspect_output="$(timeout -k "${publish_timeout}s" -s SIGKILL "${publish_timeout}s" docker buildx imagetools inspect "${image_ref}" 2>"${error_file}")"; then
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

write_raw_manifest() {
  local image_ref="$1"
  local output_file="$2"
  local error_file="$3"

  if ! timeout -k "${publish_timeout}s" -s SIGKILL "${publish_timeout}s" \
    docker buildx imagetools inspect "${image_ref}" --raw >"${output_file}" 2>"${error_file}"
  then
    cat "${error_file}" >&2
    echo "error: registry manifest lookup failed for ${image_ref}" >&2
    return 1
  fi
}

verify_version_sources() {
  local version_ref="$1"
  local version_digest="$2"
  local platform_plan="$3"
  local verification_directory="$4"
  local platform
  local token
  local local_ref
  local expected_image_id
  local platform_ref
  local platform_state
  local planned_digest
  local source_digest
  local source_raw
  local version_raw="${verification_directory}/version.json"
  local source_index=0
  local source_manifest_args=()

  mkdir -p "${verification_directory}"
  while IFS=$'\t' read -r platform token local_ref expected_image_id platform_ref platform_state planned_digest; do
    [[ -n "${platform}" ]] || continue
    source_digest="$(inspect_digest "${platform_ref}" "${verification_directory}/source-${source_index}.error")" || return 1
    [[ "${source_digest}" == "${expected_image_id}" ]] || {
      echo "error: immutable container tag ${platform_ref} has ${source_digest}, expected ${expected_image_id}" >&2
      return 1
    }
    source_raw="${verification_directory}/source-${source_index}.json"
    write_raw_manifest "${platform_ref}" "${source_raw}" "${verification_directory}/source-${source_index}-raw.error" || return 1
    source_manifest_args+=("${platform}" "${source_digest}" "${source_raw}")
    source_index=$((source_index + 1))
  done <"${platform_plan}"

  write_raw_manifest "${version_ref}" "${version_raw}" "${verification_directory}/version.error" || return 1
  python3 - "${version_raw}" "${version_digest}" "${source_manifest_args[@]}" <<'PY'
import collections
import json
import sys

version_raw, version_digest, *source_values = sys.argv[1:]
if not source_values or len(source_values) % 3:
    raise SystemExit("invalid prepared container source list")


def normalized_entries(document, digest, default_platform):
    manifests = document.get("manifests")
    if isinstance(manifests, list):
        return [
            (
                entry.get("mediaType"),
                entry.get("digest"),
                (entry.get("platform") or {}).get("os"),
                (entry.get("platform") or {}).get("architecture"),
                (entry.get("platform") or {}).get("variant"),
            )
            for entry in manifests
        ]
    platform_os, platform_architecture = default_platform.split("/", 1)
    return [(document.get("mediaType"), digest, platform_os, platform_architecture, None)]


expected = []
default_platforms = []
for index in range(0, len(source_values), 3):
    platform, digest, raw_path = source_values[index : index + 3]
    default_platforms.append(platform)
    with open(raw_path, encoding="utf-8") as handle:
        expected.extend(normalized_entries(json.load(handle), digest, platform))

with open(version_raw, encoding="utf-8") as handle:
    version_document = json.load(handle)
if isinstance(version_document.get("manifests"), list):
    actual = normalized_entries(version_document, version_digest, default_platforms[0])
elif len(default_platforms) == 1:
    actual = normalized_entries(version_document, version_digest, default_platforms[0])
else:
    raise SystemExit("published multi-platform version tag is not an image index")

if collections.Counter(actual) != collections.Counter(expected):
    print("published version manifest does not match its exact prepared platform sources", file=sys.stderr)
    print(f"expected={sorted(expected)}", file=sys.stderr)
    print(f"actual={sorted(actual)}", file=sys.stderr)
    raise SystemExit(1)
PY
}

verify_published_container_without_artifact() {
  local image="${DOCKER_IMAGE:-}"
  local platforms="${PUBLISH_PLATFORMS:-}"
  local head_tag_output
  local head_tag
  local version
  local head_commit
  local tag_commit
  local platform_plan
  local seen_platforms="|"
  local platform
  local token
  local platform_ref
  local platform_digest
  local version_ref
  local version_digest
  local latest_ref
  local latest_digest
  local inspect_status
  local head_release_tags=()

  [[ -n "${image}" ]] || {
    echo "error: DOCKER_IMAGE is required to verify a published release without local artifacts" >&2
    exit 1
  }
  [[ -n "${platforms}" ]] || {
    echo "error: PUBLISH_PLATFORMS is required to verify a published release without local artifacts" >&2
    exit 1
  }

  head_tag_output="$(git tag --points-at HEAD --list 'v*' --sort=-version:refname)"
  while IFS= read -r head_tag; do
    [[ -n "${head_tag}" ]] || continue
    if [[ "${head_tag}" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$ ]]; then
      head_release_tags+=("${head_tag}")
    fi
  done <<<"${head_tag_output}"
  [[ "${#head_release_tags[@]}" -eq 1 ]] || {
    echo "error: published container verification requires one exact SemVer release tag at HEAD" >&2
    exit 1
  }
  version="${head_release_tags[0]}"
  [[ "$(git cat-file -t "refs/tags/${version}")" == "tag" ]] || {
    echo "error: release tag ${version} must be annotated" >&2
    exit 1
  }
  head_commit="$(git rev-parse HEAD)"
  tag_commit="$(git rev-parse "${version}^{commit}")"
  [[ "${tag_commit}" == "${head_commit}" ]] || {
    echo "error: release tag ${version} does not point at HEAD" >&2
    exit 1
  }

  platform_plan="${temporary_directory}/published-platform-plan.tsv"
  : >"${platform_plan}"
  IFS=',' read -r -a platform_list <<<"${platforms}"
  [[ "${#platform_list[@]}" -gt 0 ]] || {
    echo "error: PUBLISH_PLATFORMS must select at least one platform" >&2
    exit 1
  }
  for platform in "${platform_list[@]}"; do
    [[ "${platform}" =~ ^linux/(amd64|arm64)$ ]] || {
      echo "error: unsupported published platform: ${platform}" >&2
      exit 1
    }
    [[ "${seen_platforms}" != *"|${platform}|"* ]] || {
      echo "error: duplicate published platform: ${platform}" >&2
      exit 1
    }
    seen_platforms+="${platform}|"
    token="${platform//\//-}"
    platform_ref="${image}:${version}-${token}"
    if platform_digest="$(inspect_digest "${platform_ref}" "${temporary_directory}/published-${token}.error")"; then
      :
    else
      inspect_status="$?"
      if [[ "${inspect_status}" -eq 2 ]]; then
        echo "error: immutable container tag ${platform_ref} is not published; local release artifacts are unavailable" >&2
      fi
      exit 1
    fi
    printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
      "${platform}" "${token}" "published" "${platform_digest}" "${platform_ref}" "exact" "${platform_digest}" \
      >>"${platform_plan}"
  done

  version_ref="${image}:${version}"
  if version_digest="$(inspect_digest "${version_ref}" "${temporary_directory}/published-version.error")"; then
    :
  else
    inspect_status="$?"
    if [[ "${inspect_status}" -eq 2 ]]; then
      echo "error: immutable container tag ${version_ref} is not published; local release artifacts are unavailable" >&2
    fi
    exit 1
  fi
  verify_version_sources \
    "${version_ref}" \
    "${version_digest}" \
    "${platform_plan}" \
    "${temporary_directory}/published-version-verification" || {
      echo "error: immutable container tag ${version_ref} does not match its exact platform tags" >&2
      exit 1
    }

  if [[ ! "${version}" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+- ]]; then
    latest_ref="${image}:latest"
    if latest_digest="$(inspect_digest "${latest_ref}" "${temporary_directory}/published-latest.error")"; then
      :
    else
      inspect_status="$?"
      if [[ "${inspect_status}" -eq 2 ]]; then
        echo "error: ${latest_ref} is not published; local release artifacts are unavailable" >&2
      fi
      exit 1
    fi
    [[ "${latest_digest}" == "${version_digest}" ]] || {
      echo "error: ${latest_ref} digest ${latest_digest} does not match ${version_ref} digest ${version_digest}" >&2
      exit 1
    }
  fi
  echo "Container ${version_ref} is already published with exact platform manifests; no registry changes are required."
}

if [[ ! -d "${artifact_dir}" ]] || [[ -z "$(find "${artifact_dir}" -mindepth 1 -print -quit 2>/dev/null)" ]]; then
  verify_published_container_without_artifact
  exit 0
fi

command -v gh >/dev/null 2>&1 || { echo "error: gh is required" >&2; exit 1; }
helper="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/release_helper.py"
"${helper}" verify-release-artifact >/dev/null
release_version="$(python3 - "${artifact_dir}/manifest.json" <<'PY'
import json
import sys
print(json.load(open(sys.argv[1], encoding="utf-8"))["version"])
PY
)"

mapfile -t descriptors < <(find "${artifact_dir}/payloads/containers" -mindepth 2 -maxdepth 2 -name container.json -type f | LC_ALL=C sort)
[[ "${#descriptors[@]}" -gt 0 ]] || { echo "error: no prepared container artifacts found; run make release" >&2; exit 1; }

if python3 - "${descriptors[@]}" <<'PY'
import json
import sys
raise SystemExit(0 if any(json.load(open(path, encoding="utf-8"))["image"].startswith("ghcr.io/") for path in sys.argv[1:]) else 1)
PY
then
  registry_username="$(gh api user --jq .login)"
  registry_token="$(gh auth token)"
  printf '%s' "${registry_token}" | timeout -k 30s -s SIGKILL 30s docker login ghcr.io --username "${registry_username}" --password-stdin
  unset registry_token
fi

descriptor_index=0
for descriptor in "${descriptors[@]}"; do
  metadata="$(python3 - "${descriptor}" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1], encoding="utf-8"))
if data.get("schema_version") != 1 or data.get("artifact_kind") != "mprlab.container":
    raise SystemExit("invalid container artifact descriptor")
print(data["name"])
print(data["image"])
print(data["version"])
for platform in data["platforms"]:
    print("\t".join([platform["platform"], platform["token"], platform["local_ref"], platform["image_id"], platform["archive"], platform["sha256"]]))
PY
)"
  name="$(sed -n '1p' <<<"${metadata}")"
  image="$(sed -n '2p' <<<"${metadata}")"
  version="$(sed -n '3p' <<<"${metadata}")"
  [[ "${version}" == "${release_version}" ]] || { echo "error: ${name} was prepared for ${version}, expected ${release_version}" >&2; exit 1; }
  publish_latest="true"
  if [[ "${version}" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+- ]]; then
    publish_latest="false"
  fi
  if [[ "${image}" == */* && "${image%%/*}" == *.* && "${image}" != ghcr.io/* ]]; then
    echo "error: unsupported explicit container registry for ${image}" >&2
    exit 1
  fi
  platform_plan="${temporary_directory}/platform-plan-${descriptor_index}.tsv"
  : >"${platform_plan}"
  missing_platforms=0

  while IFS=$'\t' read -r platform token local_ref expected_image_id archive_relative expected_sha256; do
    [[ -n "${platform}" ]] || continue
    archive="${artifact_dir}/${archive_relative}"
    actual_sha256="$(shasum -a 256 "${archive}" | awk '{print $1}')"
    [[ "${actual_sha256}" == "${expected_sha256}" ]] || { echo "error: container archive hash mismatch: ${archive_relative}" >&2; exit 1; }
    timeout -k "${publish_timeout}s" -s SIGKILL "${publish_timeout}s" docker load --input "${archive}" >/dev/null
    actual_image_id="$(docker image inspect "${local_ref}" --format '{{.Id}}')"
    [[ "${actual_image_id}" == "${expected_image_id}" ]] || {
      echo "error: loaded image ${name} ${platform} has ${actual_image_id}, expected ${expected_image_id}" >&2
      exit 1
    }
    platform_ref="${image}:${version}-${token}"
    platform_digest=""
    if platform_digest="$(inspect_digest "${platform_ref}" "${temporary_directory}/platform-${descriptor_index}-${token}.error")"; then
      [[ "${platform_digest}" == "${expected_image_id}" ]] || {
        echo "error: immutable container tag ${platform_ref} has ${platform_digest}, expected ${expected_image_id}" >&2
        exit 1
      }
      platform_state="exact"
    else
      inspect_status="$?"
      if [[ "${inspect_status}" -eq 2 ]]; then
        platform_state="missing"
        missing_platforms=$((missing_platforms + 1))
      else
        exit 1
      fi
    fi
    printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
      "${platform}" "${token}" "${local_ref}" "${expected_image_id}" "${platform_ref}" "${platform_state}" "${platform_digest}" \
      >>"${platform_plan}"
  done < <(tail -n +4 <<<"${metadata}")

  [[ -s "${platform_plan}" ]] || { echo "error: ${name} has no prepared platforms" >&2; exit 1; }
  version_ref="${image}:${version}"
  version_digest=""
  if version_digest="$(inspect_digest "${version_ref}" "${temporary_directory}/version-${descriptor_index}.error")"; then
    if [[ "${missing_platforms}" -gt 0 ]]; then
      echo "error: immutable container tag ${version_ref} exists while ${missing_platforms} prepared platform tag(s) are missing" >&2
      exit 1
    fi
    verify_version_sources \
      "${version_ref}" \
      "${version_digest}" \
      "${platform_plan}" \
      "${temporary_directory}/verify-${descriptor_index}-preflight" || {
        echo "error: immutable container tag ${version_ref} conflicts with the prepared release" >&2
        exit 1
      }
    version_state="exact"
  else
    inspect_status="$?"
    if [[ "${inspect_status}" -eq 2 ]]; then
      version_state="missing"
    else
      exit 1
    fi
  fi

  sources=()
  while IFS=$'\t' read -r platform token local_ref expected_image_id platform_ref platform_state planned_digest; do
    [[ -n "${platform}" ]] || continue
    sources+=("${platform_ref}")
    if [[ "${platform_state}" == "exact" ]]; then
      echo "==> [publish] Verified ${platform_ref}; no push needed"
      continue
    fi
    docker tag "${local_ref}" "${platform_ref}"
    echo "==> [publish] Pushing ${platform_ref}"
    timeout -k "${publish_timeout}s" -s SIGKILL "${publish_timeout}s" docker push "${platform_ref}"
    published_platform_digest="$(inspect_digest "${platform_ref}" "${temporary_directory}/platform-${descriptor_index}-${token}-published.error")" || {
      echo "error: pushed platform tag could not be verified: ${platform_ref}" >&2
      exit 1
    }
    [[ "${published_platform_digest}" == "${expected_image_id}" ]] || {
      echo "error: pushed platform tag ${platform_ref} has ${published_platform_digest}, expected ${expected_image_id}" >&2
      exit 1
    }
  done <"${platform_plan}"

  if [[ "${version_state}" == "missing" ]]; then
    echo "==> [publish] Creating ${version_ref}"
    timeout -k "${publish_timeout}s" -s SIGKILL "${publish_timeout}s" docker buildx imagetools create --tag "${version_ref}" "${sources[@]}"
    version_digest="$(inspect_digest "${version_ref}" "${temporary_directory}/version-${descriptor_index}-published.error")" || {
      echo "error: published version digest is missing for ${version_ref}" >&2
      exit 1
    }
    verify_version_sources \
      "${version_ref}" \
      "${version_digest}" \
      "${platform_plan}" \
      "${temporary_directory}/verify-${descriptor_index}-published" || {
        echo "error: published container tag ${version_ref} does not match the prepared release" >&2
        exit 1
      }
  else
    echo "==> [publish] Verified ${version_ref}; no manifest update needed"
  fi

  if [[ "${publish_latest}" == "true" ]]; then
    latest_ref="${image}:latest"
    latest_digest=""
    latest_is_current="false"
    if latest_digest="$(inspect_digest "${latest_ref}" "${temporary_directory}/latest-${descriptor_index}.error")"; then
      if [[ "${latest_digest}" == "${version_digest}" ]]; then
        latest_is_current="true"
      fi
    else
      inspect_status="$?"
      [[ "${inspect_status}" -eq 2 ]] || exit 1
    fi
    if [[ "${latest_is_current}" == "true" ]]; then
      echo "==> [publish] ${latest_ref} already matches ${version_ref}; no update needed"
    else
      echo "==> [publish] Updating ${latest_ref}"
      timeout -k "${publish_timeout}s" -s SIGKILL "${publish_timeout}s" docker buildx imagetools create --tag "${latest_ref}" "${sources[@]}"
      latest_digest="$(inspect_digest "${latest_ref}" "${temporary_directory}/latest-${descriptor_index}-published.error")" || {
        echo "error: published latest digest is missing for ${latest_ref}" >&2
        exit 1
      }
      [[ "${version_digest}" == "${latest_digest}" ]] || { echo "error: published version and latest digests differ for ${image}" >&2; exit 1; }
    fi
  else
    echo "==> [publish] Leaving ${image}:latest unchanged for prerelease ${version}"
  fi
  echo "Published ${image}:${version} at ${version_digest}."
  descriptor_index=$((descriptor_index + 1))
done

exit=0
