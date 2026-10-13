#!/usr/bin/env bash
set -euo pipefail

# Destructive full restore. A backup path and --confirm are both required.
STASH_DIR="${STASH_DIR:-/opt/stash}"
COMPOSE_FILE="${COMPOSE_FILE:-docker-compose.prod.yml}"

if [[ "${2:-}" != "--confirm" || -z "${1:-}" ]]; then
  echo "Usage: $0 /opt/stash/backups/stash-<timestamp>.dump --confirm" >&2
  exit 2
fi

backup="$1"
cd "$STASH_DIR"
test -s "$backup"
restore="/tmp/stash-restore-$$.dump"

# Verify and restore by copying the dump into the container rather than piping
# it in: a pipe makes cat receive SIGPIPE and pipefail then aborts the script.
docker compose -f "$COMPOSE_FILE" cp "$backup" "postgres:$restore" >/dev/null
docker compose -f "$COMPOSE_FILE" exec -T postgres pg_restore --list "$restore" >/dev/null

echo "WARNING: this replaces the current Stash database with $backup"
docker compose -f "$COMPOSE_FILE" stop stash stash-ui

docker compose -f "$COMPOSE_FILE" exec -T postgres \
  pg_restore --clean --if-exists --no-owner -U stash -d stash "$restore"
docker compose -f "$COMPOSE_FILE" exec -T postgres rm -f "$restore"

docker compose -f "$COMPOSE_FILE" up -d stash stash-ui
echo "Restore complete. Verify /healthz and the dashboard before normal use."
