#!/usr/bin/env bash
set -euo pipefail

profile="${1:-localhost}"
if [[ "$profile" == -* ]]; then
  set -- "$profile" "$@"
  profile="localhost"
else
  shift || true
fi

case "$profile" in
  localhost|computercat) ;;
  *)
    echo "usage: ./up.sh [localhost|computercat] [docker compose up args...]" >&2
    exit 1
    ;;
esac

exec docker compose --profile "$profile" up --build --force-recreate --remove-orphans "$@"
