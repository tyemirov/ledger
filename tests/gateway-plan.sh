#!/usr/bin/env bash
set -euo pipefail

repository_root="$(cd "$(dirname "$0")/.." && pwd -P)"
gateway_command="${MPRLAB_GATEWAY_EXECUTABLE:-mprlab-gateway}"
fixture_root="$(mktemp -d)"
trap 'rm -rf "$fixture_root"' EXIT
fixture_root="$(cd "$fixture_root" && pwd -P)"

# Plan the current tracked source, including edits, in a disposable Git fixture.
while IFS= read -r -d '' relative_path; do
  mkdir -p "$fixture_root/$(dirname "$relative_path")"
  cp "$repository_root/$relative_path" "$fixture_root/$relative_path"
done < <(git -C "$repository_root" ls-files -z)

git -C "$fixture_root" init --quiet --initial-branch=master
git -C "$fixture_root" remote add origin "$(git -C "$repository_root" remote get-url origin)"
git -C "$fixture_root" add --all
git -C "$fixture_root" -c core.hooksPath=/dev/null -c commit.gpgsign=false \
  -c user.name='Gateway integration test' -c user.email='gateway-test@example.invalid' \
  commit --quiet -m 'Gateway manifest integration fixture'

"$gateway_command" app-plan-release --app-root "$fixture_root"
