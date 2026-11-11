#!/usr/bin/env bash
set -euo pipefail
exec </dev/null
# Own Git inputs for fixture preparation and every Gateway child process.
while IFS= read -r variable_name; do
  case "$variable_name" in
    GIT_*) unset "$variable_name" ;;
  esac
done < <(compgen -e)
export CI=1 GIT_TERMINAL_PROMPT=0 GIT_CONFIG_NOSYSTEM=1
export GIT_CONFIG_SYSTEM=/dev/null GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_COUNT=0

repository_root="$(cd "$(dirname "$0")/.." && pwd -P)"
gateway_command="${MPRLAB_GATEWAY_EXECUTABLE:-mprlab-gateway}"
fixture_workspace="$(mktemp -d)"
trap 'rm -rf "$fixture_workspace"' EXIT
fixture_workspace="$(cd "$fixture_workspace" && pwd -P)"
fixture_root="$fixture_workspace/application"
fixture_origin="$fixture_workspace/origin.git"
fixture_transport="$fixture_workspace/git-transport.sh"
plan_log="$fixture_workspace/plan.log"
export MPRLAB_GATEWAY_OPERATOR_ROOT="$fixture_workspace/operator"
mkdir -p "$fixture_root"

# Plan current tracked bytes through real Git and a local provider transport.
while IFS= read -r -d '' relative_path; do
  mkdir -p "$fixture_root/$(dirname "$relative_path")"
  cp "$repository_root/$relative_path" "$fixture_root/$relative_path"
done < <(git -C "$repository_root" ls-files -z)

# Resolve only synthetic inputs in the fixture's selected environment file.
unset DATABASE_URL DEPLOY_SUDO_PASSWORD_TUTOSH MPRLAB_TAUTH_MANAGEMENT_URL \
  MPRLAB_TAUTH_PROVISIONING_CREDENTIALS TAUTH_GOOGLE_CLIENT_ID TAUTH_JWT_SIGNING_KEY
cat > "$fixture_root/.mprlab/deploy/.env" <<'ENVIRONMENT'
DATABASE_URL=sqlite:///srv/data/ledger.db
DEPLOY_SUDO_PASSWORD_TUTOSH=fixture-only-password
MPRLAB_TAUTH_MANAGEMENT_URL=https://tauth.example.invalid
MPRLAB_TAUTH_PROVISIONING_CREDENTIALS=fixture-only-credential
TAUTH_GOOGLE_CLIENT_ID=fixture-only-client
TAUTH_JWT_SIGNING_KEY=fixture-only-signing-key
ENVIRONMENT

git -C "$fixture_root" init --quiet --initial-branch=master
git -C "$fixture_root" add --all
git -C "$fixture_root" -c core.hooksPath=/dev/null -c commit.gpgsign=false \
  -c user.name='Gateway integration test' -c user.email='gateway-test@example.invalid' \
  commit --quiet -m 'Gateway manifest integration fixture'
git clone --quiet --bare "$fixture_root" "$fixture_origin"

# Keep the canonical GitHub origin shape and isolate its external transport.
cat > "$fixture_transport" <<'TRANSPORT'
set -eu
if [ "$#" -ne 2 ] || [ "$1" != git@github.com ] || [ "$2" != "git-upload-pack 'tyemirov/ledger.git'" ]; then
  printf 'Unexpected Git fixture transport request.\n' >&2
  exit 2
fi
exec git upload-pack "$LEDGER_GATEWAY_PLAN_FIXTURE_ORIGIN"
TRANSPORT
export LEDGER_GATEWAY_PLAN_FIXTURE_ORIGIN="$fixture_origin"
git -C "$fixture_root" remote add origin git@github.com:tyemirov/ledger.git
export GIT_SSH_COMMAND="sh '$fixture_transport'" GIT_SSH_VARIANT=simple
git -C "$fixture_root" config branch.master.remote origin
git -C "$fixture_root" config branch.master.merge refs/heads/master
git -C "$fixture_root" fetch --quiet --no-tags origin '+refs/heads/master:refs/remotes/origin/master'

status=0
"$gateway_command" app-plan-release --app-root "$fixture_root" > "$plan_log" 2>&1 || status=$?
if [ "$status" -ne 0 ]; then
  cat "$plan_log" >&2
  exit "$status"
fi
printf 'Installed Gateway manifest planning passed.\n'
