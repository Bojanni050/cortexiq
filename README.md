<!-- Modifications copyright (c) 2026 Bojan van der Heide -->
# CortexIQ

**Your AI has amnesia. We fixed it.**

Every LLM starts every conversation from zero. CortexIQ gives your agent persistent memory — it remembers, recalls, consolidates, and learns across sessions. No more explaining yourself from scratch.

Open source. Self-hosted. Works with any MCP-compatible agent.

---

> **Don't want to self-host?**
> **[usestash.io](https://usestash.io)** is the hosted cloud version — sign in with Google, copy one MCP URL, and you're done. Free to start.

---

## Quick Start

```bash
git clone https://github.com/Bojanni050/cortexiq.git
cd cortexiq
cp .env.example .env   # edit with your API key + model
docker compose up
```

That's it. Postgres + pgvector, migrations, MCP server with background consolidation — all in one command.

**Next:** [Getting Started guide](docs/GETTING_STARTED.md) — connect your MCP client, run `init` / `remember` / `recall`, and verify everything works.

**Fully local (no cloud API):** [Ollama setup guide](docs/LOCAL_OLLAMA.md) — host Ollama + Docker Compose, private embeddings and reasoner.

## Cleaning up duplicate facts

Rewordings and translations of the same fact can pile up. New duplicates are caught during consolidation (see `CORTEXIQ_CONSOLIDATION_DEDUP_*` in `.env.example`). To clean up existing ones:

```bash
cortexiq dedupe                       # dry run: shows what would be merged
cortexiq dedupe --review              # also let the LLM judge borderline pairs
cortexiq dedupe --apply               # merge: the best-supported fact survives, the rest are soft-deleted
```

## Backup, export and import

```bash
cortexiq export -n /projects -o backup.jsonl        # episodes + facts as JSON Lines
cortexiq import backup.jsonl --dry-run              # validate and count
cortexiq import backup.jsonl                        # re-embeds with the configured model
```

Embeddings are not exported, so a file can be imported into an instance that uses a different embedding model. Importing is idempotent (same namespace + content is skipped). Use `--types episode` or `--types fact` to avoid importing both kinds — imported episodes are consolidated again, which can create duplicate facts that consolidation then merges.

For a complete production backup, use the Postgres scripts on the VPS:

```bash
/opt/cortexiq/scripts/backup-prod.sh
/opt/cortexiq/scripts/restore-prod.sh /opt/cortexiq/backups/cortexiq-<timestamp>.dump --confirm
```

Backups can be scheduled with a systemd timer and pruned automatically; see
[`docs/BACKUP.md`](docs/BACKUP.md) for retention, off-box copies, and a restore
drill. See [`docs/QUALITY_EVALUATION.md`](docs/QUALITY_EVALUATION.md) before
changing the reasoner model or consolidation settings.

## Atlas Cloud

> [Atlas Cloud](https://www.atlascloud.ai/?utm_source=github&utm_medium=link&utm_campaign=stash) is a full-modal AI inference platform that gives developers a single AI API to access video generation, image generation, and LLM APIs. Instead of managing multiple vendor integrations, you connect once and get unified access to 300+ curated models across all modalities.
>
> Check out Atlas Cloud's coding plan promotion: [https://www.atlascloud.ai/console/coding-plan](https://www.atlascloud.ai/console/coding-plan)

CortexIQ already supports Atlas Cloud through its OpenAI-compatible API. Set your `.env` like this:

```bash
CORTEXIQ_OPENAI_API_KEY=your-atlas-cloud-api-key
CORTEXIQ_OPENAI_BASE_URL=https://api.atlascloud.ai/v1
CORTEXIQ_EMBEDDING_MODEL=text-embedding-3-small
CORTEXIQ_REASONER_MODEL=deepseek-ai/DeepSeek-V3-0324
CORTEXIQ_VECTOR_DIM=1536
```

Atlas Cloud works well here because CortexIQ only needs:

- an embeddings model for vectorization
- a chat-capable reasoning model for consolidation
- an OpenAI-compatible base URL and API key

See [Getting Started](docs/GETTING_STARTED.md) for a fuller configuration checklist.

## MCP Client Setup

After `docker compose up`, CortexIQ exposes an MCP server over SSE at:

```
http://localhost:8080/sse
```

A web dashboard is also served at `http://localhost:9090/ui` — browse and manage memories, goals, hypotheses, contradictions, and failures from your browser. The JSON API lives on that **same 9090 listener** (`POST /api/v1/recall`, `POST /api/v1/remember`, `GET /api/v1/...`); the 8080 listener speaks MCP only.

To protect CortexIQ, set `CORTEXIQ_API_TOKEN` in `.env`. It guards both the dashboard API (`/api/v1`) and the MCP server (`/sse`, `/message`): every request then needs `Authorization: Bearer <token>`. The dashboard asks for the token on first use and remembers it in the browser; MCP clients need to send the header (see below). Leave it empty to keep everything open. `/healthz`, `/readyz` and `/metrics` are not covered.

### Scoped tokens

For several callers, use `CORTEXIQ_API_TOKENS` instead: a JSON array where each
token can be read-only and/or restricted to a namespace subtree. It replaces
`CORTEXIQ_API_TOKEN` when set.

```json
[
  {"name": "dashboard", "token": "<hex>", "scope": "rw"},
  {"name": "agent", "token": "<hex>", "scope": "ro", "namespaces": ["/projects", "/user"]}
]
```

- `scope`: `rw` (default) or `ro`. Read-only tokens may only run read operations;
  the dashboard allows `GET`/`HEAD` and `POST /api/v1/recall`, and the MCP server
  allows only read tools (`recall`, `query_*`, `list_*`, `get_context`,
  `trace_causal_chain`).
- `namespaces`: allowlist of namespace paths. A token may only see and touch
  those paths and their descendants. Omit it for full access.

Namespace enforcement happens in the brain layer, so it applies to every entry
point (dashboard API, MCP tools, export) rather than per endpoint.

Point any MCP-compatible client at that URL. Example configs:

With `CORTEXIQ_API_TOKEN` set, add the header to the client config, e.g. for Cursor:
`"headers": {"Authorization": "Bearer <token>"}` next to `"url"`. For Claude Code:
`claude mcp add --transport sse cortexiq http://localhost:8080/sse --header "Authorization: Bearer <token>"`.

**Cursor** — `~/.cursor/mcp.json`
```json
{
  "mcpServers": {
    "cortexiq": {
      "url": "http://localhost:8080/sse"
    }
  }
}
```

**Claude Desktop** — `claude_desktop_config.json`
```json
{
  "mcpServers": {
    "cortexiq": {
      "url": "http://localhost:8080/sse"
    }
  }
}
```

**OpenCode** — `~/.config/opencode/config.json`
```json
{
  "mcp": {
    "cortexiq": {
      "type": "remote",
      "url": "http://localhost:8080/sse",
      "enabled": true
    }
  }
}
```

**Windsurf** — `~/.codeium/windsurf/mcp_config.json`
```json
{
  "mcpServers": {
    "cortexiq": {
      "url": "http://localhost:8080/sse"
    }
  }
}
```

Works with any agent that supports MCP over SSE — Claude Desktop, Cursor, Windsurf, Cline, Continue, OpenAI Agents, Ollama, OpenRouter, Atlas Cloud-backed setups, and more.

## What It Does

CortexIQ is a cognitive layer between your AI agent and the world. Episodes become facts. Facts become relationships. Relationships become patterns. Patterns become wisdom.

A 9-stage consolidation pipeline turns raw observations into structured knowledge — facts, relationships, causal links, patterns, contradictions, goal tracking, failure patterns, and hypothesis verification. Each stage only processes new data since the last run.

## Upstream cloud (Stash, Beta — Free)

The upstream project this fork is based on hosts a multi-tenant version at **[usestash.io](https://usestash.io/)**, free while in beta. It is not part of CortexIQ.

The cloud version is written from scratch — it shares no code with this repository. It is designed from the ground up for scalability, multi-tenancy, and long-term sustainability as a product. Feature sets differ in both directions: some things available here aren't in the cloud, and vice versa.

## Learn More

Upstream documentation: **[alash3al.github.io/stash →](https://alash3al.github.io/stash/)**

## Credits and license

CortexIQ is a fork of [Stash](https://github.com/alash3al/stash) by alash3al, Copyright 2024 Mohamed Al Ashaal. The upstream code and its license are retained, and every upstream file this fork changed carries a modification notice: see [`LICENSE-THIRD-PARTY.md`](LICENSE-THIRD-PARTY.md).

Apache 2.0 — [`LICENSE`](LICENSE) · third-party notices — [`LICENSE-THIRD-PARTY.md`](LICENSE-THIRD-PARTY.md)
