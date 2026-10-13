#!/usr/bin/env bash
set -euo pipefail

# Full logical backup of the production Postgres database.
# Run on the VPS from /opt/stash or set STASH_DIR explicitly.
STASH_DIR="${STASH_DIR:-/opt/stash}"
COMPOSE_FILE="${COMPOSE_FILE:-docker-compose.prod.yml}"
BACKUP_DIR="${BACKUP_DIR:-$STASH_DIR/backups}"

cd "$STASH_DIR"
mkdir -p "$BACKUP_DIR"

timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
backup="$BACKUP_DIR/stash-$timestamp.dump"

echo "Creating $backup"
docker compose -f "$COMPOSE_FILE" exec -T postgres \
  pg_dump -U stash -d stash -Fc > "$backup"

test -s "$backup"
cat "$backup" | docker compose -f "$COMPOSE_FILE" exec -T postgres \
  pg_restore --list > "$backup.toc"
sha256sum "$backup" > "$backup.sha256"

echo "Backup verified: $backup"
echo "Contents: $backup.toc"
echo "Checksum: $backup.sha256"
