# Backup and restore

Stash keeps everything in Postgres, so a backup is a Postgres dump. Two things
are worth backing up and they live in different places:

- the **database** — episodes, facts, namespaces, goals, hypotheses,
  contradictions, failures, causal links, consolidation history;
- the **configuration** — `/opt/stash/.env` and `docker-compose.prod.yml`, which
  hold secrets and are intentionally not in Git.

`stash export` (JSON Lines) is for *migration* between instances, not for
disaster recovery: it excludes embeddings and operational tables.

## Daily backup

On the VPS, from `/opt/stash`:

```bash
/opt/stash/scripts/backup-prod.sh
```

It writes `stash-<timestamp>.dump` (Postgres custom format), plus a `.toc`
contents listing and a `.sha256` checksum, into `/opt/stash/backups`. It fails
if the dump is empty, which catches a `pg_dump` that died mid-run.

### Retention

`BACKUP_RETENTION_DAYS` (default `7`, `0` keeps everything) prunes local dumps
older than that on every successful run.

### Off-box copy

A backup on the same disk does not survive disk loss. Set `BACKUP_REMOTE` to an
rsync target and copy happens automatically:

```bash
BACKUP_REMOTE=backup@other-host:/srv/stash-backups/ /opt/stash/scripts/backup-prod.sh
```

`rsync` must be installed; if it is missing the run warns and continues.

## Schedule it (systemd timer)

```bash
cd /opt/stash
STASH_DIR=/opt/stash ./scripts/install-backup-timer.sh
```

This installs and starts `stash-backup.timer` (daily at 03:30 with a random
delay; `Persistent=true` catches up after downtime). Check it:

```bash
systemctl list-timers stash-backup.timer
systemctl start stash-backup.service      # run one now
journalctl -u stash-backup.service -n 50  # last run output
```

## Restore

Restore is destructive and asks for confirmation. It stops the app, restores
into the existing database, and starts the app again:

```bash
/opt/stash/scripts/restore-prod.sh /opt/stash/backups/stash-<timestamp>.dump --confirm
```

Afterwards, verify before trusting it:

```bash
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:18101/healthz   # expect 200
```

### Restore drill

Do this on a throwaway database, not in production, at least once:

```bash
# 1. create an empty database on the same server
docker compose -f docker-compose.prod.yml exec -T postgres \
  createdb -U stash stash_restore_test

# 2. load the dump into it
cat /opt/stash/backups/stash-<timestamp>.dump | \
  docker compose -f docker-compose.prod.yml exec -T postgres \
  pg_restore --no-owner -U stash -d stash_restore_test

# 3. sanity-check a couple of counts
docker compose -f docker-compose.prod.yml exec -T postgres \
  psql -U stash -d stash_restore_test -c \
  'select (select count(*) from facts) as facts, (select count(*) from episodes) as episodes'

# 4. drop it
docker compose -f docker-compose.prod.yml exec -T postgres \
  dropdb -U stash stash_restore_test
```

An untested backup is a hope, not a backup. Run the drill after any change to
the schema or the backup scripts.

## Configuration backup

The database dump does not include the compose file or `.env`. Copy them
separately (they contain secrets, so treat the copy as a secret):

```bash
install -m 0600 /opt/stash/.env              /opt/stash/backups/env-$(date -u +%F)
install -m 0600 /opt/stash/docker-compose.prod.yml /opt/stash/backups/compose-$(date -u +%F)
```
