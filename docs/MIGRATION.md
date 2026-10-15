# Migrating a pre-rebrand deployment (Stash → CortexIQ)

CortexIQ carries no compatibility layer for the pre-rebrand layout: the `STASH_*`
environment variables, the `STASHCONFIG` file variable, the `stash-export`
import format and the `stash` MCP server name are all gone. This runbook moves a
live deployment from the old `/opt/stash` stack to a fresh `/opt/cortexiq`
stack, carrying the data across with a dump/restore.

## Why a dump/restore instead of a rename

Docker Compose derives the *real* name of a named volume from the compose
**project name** — the `name:` field, or the directory name when it is absent.
The old stack therefore stores its data in a volume called
`stash_postgres_data`. Renaming the project to `cortexiq` makes Compose look for
`cortexiq_postgres_data`, which does not exist yet, so it creates a **new, empty**
volume and the old data is silently left behind. There is no in-place rename that
avoids this. Migrate by dumping the old database and restoring it into the fresh
one; the old volume stays untouched until you delete it on purpose.

## Prerequisites

- Root (or docker-group) access to the VPS and a shell in the deployment dir.
- The current CortexIQ repo checked out (or the release tarball) to roll the new
  stack from.
- Enough free disk for a second Postgres volume plus one dump.

## 1. Take a fresh dump on the old stack

From the old deployment (`/opt/stash`), with the old stack still running:

```bash
cd /opt/stash
docker compose exec -T postgres \
  pg_dump -U stash -d stash -Fc > /tmp/stash-$(date -u +%Y%m%dT%H%M%SZ).dump
```

Verify the dump is readable before you touch anything else:

```bash
ls -l /tmp/stash-*.dump
docker compose cp "$(ls -t /tmp/stash-*.dump | head -1)" postgres:/tmp/migrate.dump
docker compose exec -T postgres pg_restore --list /tmp/migrate.dump | head
```

Keep this dump until the new stack is proven live.

## 2. Roll out the new stack

```bash
mkdir -p /opt/cortexiq
# copy the repo (or unzip the release) into /opt/cortexiq
cd /opt/cortexiq
```

Create `/opt/cortexiq/.env` with `CORTEXIQ_*` names (the old `STASH_*` names are
no longer read). At minimum:

```
POSTGRES_PASSWORD=<same-or-new-strong-password>
CORTEXIQ_POSTGRES_DSN is derived in compose; set the rest from your old .env:
CORTEXIQ_VECTOR_DIM=...
CORTEXIQ_OPENAI_API_KEY=...
CORTEXIQ_OPENAI_BASE_URL=...
CORTEXIQ_EMBEDDING_MODEL=...
CORTEXIQ_REASONER_MODEL=...
CORTEXIQ_API_TOKEN=...        # keep the same token so MCP clients keep working
```

Copy `docker-compose.prod.example.yml` to `docker-compose.prod.yml`. It already
sets `name: cortexiq`, so Compose creates the project (and its
`cortexiq_postgres_data` volume) with the new naming.

```bash
cp docker-compose.prod.example.yml docker-compose.prod.yml
docker compose -f docker-compose.prod.yml up -d postgres
```

Wait for Postgres to become healthy:

```bash
docker compose -f docker-compose.prod.yml ps
```

The Postgres role (`cortexiq`) and database (`cortexiq`) are created by the
image from `POSTGRES_USER`/`POSTGRES_DB`, so they exist as soon as the container
is up.

## 3. Restore the dump into the new database

```bash
docker compose -f docker-compose.prod.yml cp /tmp/stash-<timestamp>.dump postgres:/tmp/migrate.dump
docker compose -f docker-compose.prod.yml exec -T postgres \
  pg_restore --clean --if-exists --no-owner -U cortexiq -d cortexiq /tmp/migrate.dump
docker compose -f docker-compose.prod.yml exec -T postgres rm -f /tmp/migrate.dump
```

`--no-owner` avoids errors when the dump's ownership lines name the old `stash`
role. Past `pg_restore` warnings about missing objects on a brand-new database
are expected and harmless.

## 4. Start the application and verify

```bash
docker compose -f docker-compose.prod.yml up -d
docker compose -f docker-compose.prod.yml logs --tail=50 cortexiq
```

Verify the two endpoints that the MCP client and the dashboard use:

- **MCP (SSE):** `https://server.taile3931a.ts.net:8444/sse` — point the MCP
  client at the same URL as before and confirm a tool call succeeds (for example
  `memory_stats`). The Bearer token is unchanged; only its environment variable
  name changed.
- **Dashboard:** `https://server.taile3931a.ts.net:8445` — log in (token is saved
  under the `cortexiq_token` localStorage key) and confirm the memory counts match
  the old instance (start from the **Stats** tab).

The Tailscale ports (8444/8445) and the `/sse` path are unchanged on purpose, so
the client configuration needs no edit beyond the server name and env var rename
(see below).

## 5. Retire the old artefacts

Only after the new stack is green:

```bash
# stop and remove the old stack (keeps its volume until you delete it)
cd /opt/stash && docker compose down

# old backups can go too
rm -f /opt/stash/backups/stash-*.dump*

# old systemd units, if they were installed
systemctl disable --now stash-backup.timer 2>/dev/null || true
rm -f /etc/systemd/system/stash-backup.timer /etc/systemd/system/stash-backup.service
systemctl daemon-reload

# install the new timer from the new location
/opt/cortexiq/scripts/install-backup-timer.sh
```

When you are certain the restore is correct, remove the old deployment directory
and the orphaned volume:

```bash
# review what will be deleted first
docker volume ls | grep stash
rm -rf /opt/stash
docker volume rm stash_postgres_data    # only once you are sure
```

## 6. Update the MCP client

The MCP server now announces itself as `cortexiq`, and its auth variable is
`CORTEXIQ_AUTH`. In the client config (`~/.config/opencode/opencode.json` on
this machine) rename the `stash` server key to `cortexiq` and the
`STASH_AUTH` environment variable to `CORTEXIQ_AUTH`. The token itself does not
change.

## Rollback

Nothing in this runbook mutates the old stack until step 5. If the new stack
fails to come up, point the MCP client back at the old URL/stack and stop; the
old `/opt/stash` deployment and its volume are still intact.
