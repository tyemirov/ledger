#!/usr/bin/env bash
set -euo pipefail

repository_root="$(cd "$(dirname "$0")/.." && pwd -P)"
fixture_root="$(mktemp -d)"
trap 'rm -rf "$fixture_root"' EXIT
application_root="$fixture_root/application with spaces"
runtime_bin="$fixture_root/runtime bin"
tools_bin="$fixture_root/tools"
make_command="$(command -v make)"
git_command="$(command -v git)"
mkdir -p "$application_root" "$runtime_bin" "$tools_bin"
cp "$repository_root/Makefile" "$application_root/Makefile"
git -C "$application_root" init --quiet
application_root="$(cd "$application_root" && pwd -P)"
ln -s "$git_command" "$tools_bin/git"
ln -s "$(command -v dirname)" "$tools_bin/dirname"

# The application Makefile is real. Only its external Gateway dependency is controlled.
cat > "$fixture_root/gateway.c" <<'C'
#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>

int main(int argc, char **argv) {
    char directory[4096];
    if (getcwd(directory, sizeof(directory)) == NULL) return 1;
    printf("cwd=%s\n", directory);
    for (int index = 1; index < argc; index++) printf("arg=%s\n", argv[index]);
    const char *operator_root = getenv("MPRLAB_GATEWAY_OPERATOR_ROOT");
    printf("operator=%s\n", operator_root == NULL ? "" : operator_root);
    if (getenv("TEST_GATEWAY_FAILURE") != NULL) {
        fprintf(stderr, "gateway fixture failure\n");
        return 42;
    }
    return 0;
}
C
cc "$fixture_root/gateway.c" -o "$runtime_bin/mprlab-gateway"
unset MPRLAB_GATEWAY_EXECUTABLE MAKEFLAGS MFLAGS TEST_GATEWAY_FAILURE
export MPRLAB_GATEWAY_OPERATOR_ROOT="$fixture_root/operator with spaces"

for operation in release publish deploy; do
  expected="$(printf 'cwd=%s\narg=app-%s\narg=--app-root\narg=%s\noperator=%s' \
    "$application_root" "$operation" "$application_root" "$MPRLAB_GATEWAY_OPERATOR_ROOT")"
  for selection in path explicit; do
    arguments=()
    if [ "$selection" = explicit ]; then
      arguments+=("MPRLAB_GATEWAY_EXECUTABLE=$runtime_bin/mprlab-gateway")
      command_path="$tools_bin"
    else
      command_path="$runtime_bin:$tools_bin"
    fi
    status=0
    actual="$(PATH="$command_path" "$make_command" --no-print-directory -s -C "$application_root" "$operation" "${arguments[@]}" 2> "$fixture_root/success.err")" || status=$?
    if [ "$status" -ne 0 ]; then
      cat "$fixture_root/success.err" >&2
      exit "$status"
    fi
    if [ -s "$fixture_root/success.err" ]; then
      cat "$fixture_root/success.err" >&2
      exit 1
    fi
    if [ "$actual" != "$expected" ]; then
      printf '%s %s: unexpected Gateway invocation\n%s\n' "$operation" "$selection" "$actual" >&2
      exit 1
    fi
  done

  for selection in path explicit; do
    arguments=()
    if [ "$selection" = explicit ]; then
      arguments+=("MPRLAB_GATEWAY_EXECUTABLE=$fixture_root/missing runtime")
    fi
    status=0
    PATH="$tools_bin" "$make_command" --no-print-directory -s -C "$application_root" "$operation" "${arguments[@]}" > "$fixture_root/missing.log" 2>&1 || status=$?
    if [ "$status" -ne 2 ]; then
      cat "$fixture_root/missing.log" >&2
      printf 'missing runtime: expected Make status 2, got %s\n' "$status" >&2
      exit 1
    fi
    grep -F 'Gateway runtime is unavailable:' "$fixture_root/missing.log" >/dev/null
    if grep -F 'arg=' "$fixture_root/missing.log"; then exit 1; fi
  done

  status=0
  TEST_GATEWAY_FAILURE=1 PATH="$runtime_bin:$tools_bin" "$make_command" --no-print-directory -s -C "$application_root" "$operation" > "$fixture_root/failure.log" 2>&1 || status=$?
  if [ "$status" -ne 2 ]; then
    cat "$fixture_root/failure.log" >&2
    printf 'Gateway failure: expected Make status 2, got %s\n' "$status" >&2
    exit 1
  fi
  grep -F 'gateway fixture failure' "$fixture_root/failure.log" >/dev/null
  grep -F 'Error 42' "$fixture_root/failure.log" >/dev/null
done

printf 'Installed Gateway Make integration passed.\n'
