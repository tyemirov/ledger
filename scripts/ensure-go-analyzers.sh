#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'Go analyzer bootstrap: %s\n' "$1" >&2
  exit 1
}

read_language() {
  if [[ ! "$1" =~ ^go([0-9]+)\.([0-9]+)(\.[0-9]+)?$ ]]; then
    fail "cannot read $2: $1"
  fi
  language_major="${BASH_REMATCH[1]}"
  language_minor="${BASH_REMATCH[2]}"
}

active_version="$(go env GOVERSION)" || fail 'read active Go toolchain failed'
read_language "$active_version" 'active Go language version'
active_major="$language_major"
active_minor="$language_minor"

read_tool_language() {
  local metadata first_line
  metadata="$(go version -m "$tool_path")" || fail "read Go build metadata for $tool_name failed"
  first_line="${metadata%%$'\n'*}"
  if [[ "$first_line" != "$tool_path: "* ]]; then
    fail "cannot read Go build metadata for $tool_name"
  fi
  read_language "${first_line#"$tool_path: "}" "Go build metadata for $tool_name"
}

tool_is_older() {
  [ "$language_major" -lt "$active_major" ] || {
    [ "$language_major" -eq "$active_major" ] && [ "$language_minor" -lt "$active_minor" ]
  }
}

for declaration in "$@"; do
  tool_name="${declaration%%=*}"
  module="${declaration#*=}"
  install=false
  if tool_path="$(command -v "$tool_name")"; then
    read_tool_language
    if tool_is_older; then install=true; fi
  else
    install=true
  fi
  if [ "$install" = true ]; then
    printf 'Install %s for Go %s.%s\n' "$tool_name" "$active_major" "$active_minor"
    status=0
    go install "$module" || status=$?
    if [ "$status" -ne 0 ]; then
      printf 'Go analyzer bootstrap: install %s failed with status %s\n' "$tool_name" "$status" >&2
      exit "$status"
    fi
    tool_path="$(command -v "$tool_name")" || fail "installed $tool_name is unavailable on PATH"
    read_tool_language
    if tool_is_older; then fail "installed $tool_name remains older than the active Go language version"; fi
  fi
done
