#!/usr/bin/env bash
set -euo pipefail

# Destructive full restore. A backup path and --confirm are both required.
#   CORTEXIQ_DIR, CORTEXIQ_DB_USER, CORTEXIQ_DB_NAME and CORTEXIQ_APP_SERVICE/
#   CORTEXIQ_UI_SERVICE default to the pre-rebrand production values.
if [[ -z "${CORTEXIQ_DIR:-}" ]]; then
  if [[ -d /opt/stash ]]; then CORTEXIQ_DIR=/opt/stash; else CORTEXIQ_DIR=/opt/cortexiq; fi
fi
COMPOSE_FILE="${COMPOSE_FILE:-docker-compose.prod.yml}"
DB_USER="${CORTEXIQ_DB_USER:-stash}"
DB_NAME="${CORTEXIQ_DB_NAME:-stash}"
APP_SERVICE="${CORTEXIQ_APP_SERVICE:-stash}"
UI_SERVICE="${CORTEXIQ_UI_SERVICE:-stash-ui}"

if [[ "${2:-}" != "--confirm" || -z "${1:-}" ]]; then
  echo "Usage: $0 $CORTEXIQ_DIR/backups/cortexiq-<timestamp>.dump --confirm" >&2
  exit 2
fi

backup="$1"
cd "$CORTEXIQ_DIR"
test -s "$backup"
restore="/tmp/cortexiq-restore-$$.dump"

# Verify and restore by copying the dump into the container rather than piping
# it in: a pipe makes cat receive SIGPIPE and pipefail then aborts the script.
docker compose -f "$COMPOSE_FILE" cp "$backup" "postgres:$restore" >/dev/null
docker compose -f "$COMPOSE_FILE" exec -T postgres pg_restore --list "$restore" >/dev/null

echo "WARNING: this replaces the current CortexIQ database with $backup"
docker compose -f "$COMPOSE_FILE" stop "$APP_SERVICE" "$UI_SERVICE"

docker compose -f "$COMPOSE_FILE" exec -T postgres \
  pg_restore --clean --if-exists --no-owner -U "$DB_USER" -d "$DB_NAME" "$restore"
docker compose -f "$COMPOSE_FILE" exec -T postgres rm -f "$restore"

docker compose -f "$COMPOSE_FILE" up -d "$APP_SERVICE" "$UI_SERVICE"
echo "Restore complete. Verify /healthz and the dashboard before normal use."
