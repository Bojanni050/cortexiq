#!/usr/bin/env bash
set -euo pipefail

# Full logical backup of the production Postgres database.
# Run on the VPS from /opt/stash or set STASH_DIR explicitly.
#
# Env:
#   STASH_DIR              default /opt/stash
#   COMPOSE_FILE           default docker-compose.prod.yml
#   BACKUP_DIR             default $STASH_DIR/backups
#   BACKUP_RETENTION_DAYS  default 7  (0 keeps everything)
#   BACKUP_REMOTE          optional rsync target, e.g. user@host:/srv/stash-backups/
STASH_DIR="${STASH_DIR:-/opt/stash}"
COMPOSE_FILE="${COMPOSE_FILE:-docker-compose.prod.yml}"
BACKUP_DIR="${BACKUP_DIR:-$STASH_DIR/backups}"
RETENTION_DAYS="${BACKUP_RETENTION_DAYS:-7}"

cd "$STASH_DIR"
mkdir -p "$BACKUP_DIR"

timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
backup="$BACKUP_DIR/stash-$timestamp.dump"

echo "Creating $backup"
docker compose -f "$COMPOSE_FILE" exec -T postgres \
  pg_dump -U stash -d stash -Fc > "$backup"

# A dump that pg_dump could not finish would be empty or unreadable; fail loudly.
test -s "$backup"
cat "$backup" | docker compose -f "$COMPOSE_FILE" exec -T postgres \
  pg_restore --list > "$backup.toc"
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
  find "$BACKUP_DIR" -maxdepth 1 -type f -name 'stash-*.dump*' -mtime +"$RETENTION_DAYS" -delete
  echo "Pruned local backups older than ${RETENTION_DAYS}d"
fi

echo "Backup verified: $backup"
echo "Contents: $backup.toc"
echo "Checksum: $backup.sha256"
