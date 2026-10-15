#!/usr/bin/env bash
set -euo pipefail

# Full logical backup of the production Postgres database.
# Run on the VPS from the deployment directory, or set CORTEXIQ_DIR.
#
# Env:
#   CORTEXIQ_DIR           deployment dir; defaults to /opt/stash when that
#                          exists (pre-rebrand layout), otherwise /opt/cortexiq
#   COMPOSE_FILE           default docker-compose.prod.yml
#   BACKUP_DIR             default $CORTEXIQ_DIR/backups
#   BACKUP_RETENTION_DAYS  default 7  (0 keeps everything)
#   BACKUP_REMOTE          optional rsync target, e.g. user@host:/srv/cortexiq-backups/
#   CORTEXIQ_DB_USER       Postgres role; default stash (live DB predates the rebrand)
#   CORTEXIQ_DB_NAME       Postgres database; default stash
if [[ -z "${CORTEXIQ_DIR:-}" ]]; then
  if [[ -d /opt/stash ]]; then CORTEXIQ_DIR=/opt/stash; else CORTEXIQ_DIR=/opt/cortexiq; fi
fi
COMPOSE_FILE="${COMPOSE_FILE:-docker-compose.prod.yml}"
BACKUP_DIR="${BACKUP_DIR:-$CORTEXIQ_DIR/backups}"
RETENTION_DAYS="${BACKUP_RETENTION_DAYS:-7}"
DB_USER="${CORTEXIQ_DB_USER:-stash}"
DB_NAME="${CORTEXIQ_DB_NAME:-stash}"

cd "$CORTEXIQ_DIR"
mkdir -p "$BACKUP_DIR"

timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
backup="$BACKUP_DIR/cortexiq-$timestamp.dump"
verify="/tmp/cortexiq-verify-$$.dump"

echo "Creating $backup"
docker compose -f "$COMPOSE_FILE" exec -T postgres \
  pg_dump -U "$DB_USER" -d "$DB_NAME" -Fc > "$backup"

# A dump that pg_dump could not finish would be empty or unreadable; fail loudly.
test -s "$backup"

# Verify by reading the dump inside the container. Piping it in via stdin is
# avoided on purpose: cat then receives SIGPIPE and pipefail aborts the script.
docker compose -f "$COMPOSE_FILE" cp "$backup" "postgres:$verify" >/dev/null
docker compose -f "$COMPOSE_FILE" exec -T postgres pg_restore --list "$verify" > "$backup.toc"
docker compose -f "$COMPOSE_FILE" exec -T postgres rm -f "$verify"
sha256sum "$backup" > "$backup.sha256"

# Off-box copy is optional but strongly recommended: a backup on the same disk
# does not survive disk loss. rsync must be installed for this to work.
if [[ -n "${BACKUP_REMOTE:-}" ]]; then
  if command -v rsync >/dev/null 2>&1; then
    rsync -az --partial "$backup" "$backup.toc" "$backup.sha256" "$BACKUP_REMOTE"
    echo "Copied to $BACKUP_REMOTE"
  else
    echo "WARNING: BACKUP_REMOTE is set but rsync is not installed; off-box copy skipped" >&2
  fi
fi

# Retention: drop local dumps (and their sidecars) older than RETENTION_DAYS.
if [[ "$RETENTION_DAYS" -gt 0 ]]; then
  find "$BACKUP_DIR" -maxdepth 1 -type f \
    \( -name 'cortexiq-*.dump*' -o -name 'stash-*.dump*' \) -mtime +"$RETENTION_DAYS" -delete
  echo "Pruned local backups older than ${RETENTION_DAYS}d"
fi

echo "Backup verified: $backup"
echo "Contents: $backup.toc"
echo "Checksum: $backup.sha256"
