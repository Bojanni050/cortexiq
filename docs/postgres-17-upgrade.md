# Upgrading production from PostgreSQL 16 to 17

Keyword recall uses BM25 through the `pg_textsearch` extension (migration 00061),
and that extension supports PostgreSQL 17 and 18 only. The data directory of a
PostgreSQL 16 volume cannot be opened by a 17 server, so this is a dump and
restore into a new volume. The old volume is never touched, which makes every
step before the final switch reversible.

Expect downtime for the length of one dump, one restore and the index builds.
On a database of this size that is minutes, not hours; measure it in the
rehearsal below instead of trusting that sentence.

## Status: production database upgraded on 2026-11-11

The database was moved to PostgreSQL 17 with the application left at 0.1.73. The
BM25 migration (00061) ships with the next release, so the two risks were taken one
at a time. What differed from the plan below:

- No release existed yet, so the image was built on the VPS from `deploy/postgres`
  (same Dockerfile as CI and the rehearsal) and tagged `cortexiq-postgres:17-bm25`.
  `docker-compose.prod.yml` on the VPS points at that local tag, with a copy of the
  old file as `docker-compose.prod.yml.bak-pre-pg17`. The Dockerfile stays in
  `/opt/cortexiq/deploy/postgres/`; if the image is ever pruned, rebuild it with
  `docker build -t cortexiq-postgres:17-bm25 deploy/postgres` before recreating the
  container, or compose cannot start it.
- Downtime was about 70 seconds (app stopped 19:14:37 UTC, back around 19:15:45).
- The image was smoke-tested in a throwaway container first. A verified dump was
  taken with the app stopped; `pg_restore` took 2 s with exit code 0.
- Row counts before and after were identical: 2,944 facts, 742 episodes, 9,034
  relationships, 2,900 patterns, goose 60. HNSW and GIN indexes were back, `ANALYZE`
  was run, health and readiness returned 200 and the logs showed no errors.
- The daily backup script was run once against PostgreSQL 17 and verified.
- The old PostgreSQL 16 volume `cortexiq_postgres_data` (192 MB) is untouched.

### BM25 live: release v0.1.74, deployed 2026-11-11

Release v0.1.74 (commit e2182ed) built every job, including `build-postgres-docker`
for the first time. The application was deployed in about 23 s of downtime (stopped
19:35:46 UTC, up 19:36:09), with a verified dump (`cortexiq-20261111T193547Z.dump`)
as the rollback point and `.env.bak-pre0.1.74` beside it. Migration 00061 ran on
startup: goose 61, `facts_content_bm25` and `episodes_content_bm25` exist, no
`content_tsv` column is left. Two live recalls returned `match: "hybrid"` with the
port-8445 facts ranked first, and the BM25 indexes show scans in
`pg_stat_user_indexes`.

One thing to know: `cortexiq` and `cortexiq-ui` both run the migrations at startup
and they started together. The main process applied 00061 first; the UI's concurrent
attempt failed with `relation "facts_content_bm25" already exists` (SQLSTATE 42P07),
left nothing half-applied because migrations run in a transaction, and the restart
policy brought it back 0.6 s later. It healed itself, but a slower migration would
make the overlap longer. A rollback now needs the dump (or the migration's Down),
because the old application reads the dropped `content_tsv` columns.

Optional, not done: the compose file still points at the locally built
`cortexiq-postgres:17-bm25`; the published `ghcr.io/bojanni050/cortexiq-postgres:0.1.74`
can replace it (one more database restart of a few seconds).

Rollback to the old database while the PG16 volume exists: stop the stack, restore
`docker-compose.prod.yml.bak-pre-pg17`, `docker compose up -d`. Anything written
after 19:15 UTC exists only in the new volume.

## What changes

- `postgres` image: `pgvector/pgvector:pg16` becomes
  `ghcr.io/bojanni050/cortexiq-postgres:<release>` (PostgreSQL 17 + pgvector +
  pg_textsearch, built from `deploy/postgres`, published by the release workflow).
- The server starts with `shared_preload_libraries=pg_textsearch`. The image does
  that itself; there is nothing to configure.
- Migration 00061 builds two BM25 indexes (`facts_content_bm25`,
  `episodes_content_bm25`) and drops the old `content_tsv` columns and their GIN
  indexes. It runs when the application starts, so it needs the new image first.
  On an old image it stops with "pg_textsearch is not installed" instead of
  half-applying.
- Recall gets faster as the corpus grows; see the benchmark in
  `docs/walkthrough2.md` (2026-11-11).

## 0. Rehearse on a copy (do this first, off the VPS)

Take the newest dump from `backups/`, restore it into a throwaway PostgreSQL 17
container built from `deploy/postgres`, point a local build of this release at it
and let the migration run. That proves three things before production is touched:
the migration succeeds on real data, how long it takes, and that recall still
returns what it did.

```bash
docker build -t cortexiq-postgres:rehearsal deploy/postgres
docker run -d --name pg17-rehearsal -p 55433:5432 \
  -e POSTGRES_USER=cortexiq -e POSTGRES_PASSWORD=cortexiq -e POSTGRES_DB=cortexiq \
  cortexiq-postgres:rehearsal
cat cortexiq-<timestamp>.dump | docker exec -i pg17-rehearsal \
  pg_restore --no-owner -U cortexiq -d cortexiq
# then start the application against postgres://cortexiq:cortexiq@localhost:55433/cortexiq
# and let it apply migration 00061; time it.
```

Rehearsed on 2026-11-11 with the daily dump of that morning (47 MB; 2,944 facts,
728 episodes, 128 MB, schema at migration 60): `pg_restore` of the PostgreSQL 16
dump into the PostgreSQL 17 image took 5 s, and `db.Open` (migration 00061, HNSW
check, dimension lock) took 335 ms. The migration succeeded, both `*_bm25` indexes
existed and no `content_tsv` column was left. Expect the whole switch to be
minutes, dominated by pulling the image and starting the containers.

## 1. Prepare

1. Cut a release so the postgres image exists on GHCR (the release workflow
   publishes it next to the application image). Note the tag, for example `0.2.0`.
2. Set `CORTEXIQ_POSTGRES_IMAGE_TAG` and `CORTEXIQ_IMAGE_TAG` in `.env`.
3. In `docker-compose.prod.yml`, give the new server its own volume and keep the
   old one declared so it stays available for rollback:

   ```yaml
   services:
     postgres:
       volumes:
         - postgres17_data:/var/lib/postgresql/data
   volumes:
     postgres_data:      # old PostgreSQL 16 data, kept for rollback
     postgres17_data:
   ```

## 2. Switch

```bash
cd /opt/cortexiq

# Stop writers, then take the final dump from the still-running 16 server.
docker compose -f docker-compose.prod.yml stop cortexiq cortexiq-ui
/opt/cortexiq/scripts/backup-prod.sh          # verified, with checksum
ls -l backups | tail -3                       # note the new dump's name

# Replace only the database. The old volume stays as it is.
docker compose -f docker-compose.prod.yml stop postgres
docker compose -f docker-compose.prod.yml pull postgres
docker compose -f docker-compose.prod.yml up -d postgres   # empty 17 server, new volume

# Restore into it.
cat backups/cortexiq-<timestamp>.dump | \
  docker compose -f docker-compose.prod.yml exec -T postgres \
  pg_restore --no-owner -U cortexiq -d cortexiq

# Start the application. It applies migration 00061 on startup.
docker compose -f docker-compose.prod.yml up -d cortexiq cortexiq-ui
docker compose -f docker-compose.prod.yml logs -f cortexiq     # wait for the migration
```

## 3. Verify

```bash
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:18101/healthz   # expect 200

docker compose -f docker-compose.prod.yml exec -T postgres psql -U cortexiq -d cortexiq -c \
  "select max(version_id) from goose_db_version" \
  -c "select indexname from pg_indexes where indexname like '%bm25%'" \
  -c "select (select count(*) from facts) facts, (select count(*) from episodes) episodes"
```

Expect migration `61`, both `*_bm25` indexes, and the same fact and episode counts
as before the switch. Then run a recall for a term you know is stored (a port
number or a hostname) and confirm it comes back with `match: "hybrid"`.

If the first recall after the restore looks slow, run `ANALYZE facts; ANALYZE
episodes;` once: the planner chooses between the BM25 index and a filter-first
plan from table statistics, and a freshly restored database has none yet.

## Rollback

Until the old volume is deleted, rolling back is a configuration change:

```bash
docker compose -f docker-compose.prod.yml stop cortexiq cortexiq-ui postgres
# Point the postgres service back at the postgres_data volume and at
# image: pgvector/pgvector:pg16, set CORTEXIQ_IMAGE_TAG to the previous release.
docker compose -f docker-compose.prod.yml up -d
```

Anything written after the switch exists only in the new volume. If that matters,
dump the 17 database first; a 17 dump cannot be restored into 16, so those rows
would have to be re-added by hand.

Keep the old volume for at least a week of normal use before removing it.

## Limits worth knowing

- BM25 statistics (document frequency, average length) cover the whole table, not
  one namespace, so a term that is common in one project but rare overall still
  scores as rare. Scores are comparable within a result set, which is all the
  rank fusion uses.
- `pg_textsearch` ships packages for amd64 and arm64 only; the image build fails
  for any other architecture instead of producing an image that cannot start.
- PostgreSQL 18 is supported by the extension too, but this image pins 17. Moving
  to 18 is the same dump and restore with a different base image.
