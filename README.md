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

## Telling memory it was wrong

A fact that keeps being observed keeps its confidence, even when it is wrong. `rate_fact` supplies the one signal the evidence model cannot produce on its own — whether the fact actually helped:

```
recall("which queue do we use?")                      → fact 42: "the queue is kafka"
rate_fact(fact_id: 42, correct: false, note: "user said Redis")
```

A vote never edits the fact and never touches confidence, `valid_from`/`valid_until` or decay: those are evidence the fact earned, and a vote is not new evidence. It only scales the fact's recall rank by a bounded factor (0.5–1.25), so a downvoted fact slides to the back and can drop out of the answer, while removal still happens through a superseding fact, `forget`, or confidence decay.

Every caller has one standing vote per fact — changing your mind replaces it (`replaced: true`), so nobody wins an argument by voting more often. Votes are kept append-only in `fact_feedback` for audit, but only each principal's latest vote counts. Recalls with `as_of` or a `known_from`/`known_to` window ignore feedback: today's opinion must not rewrite what was known then.

## What is not worth storing

A memory server that stores "dank je wel, tot morgen" is one whose recall fills with noise. Every write goes through the same gate (`Remember`), and content that says nothing — greetings, thanks, acknowledgements and the function words around them — is refused with `skipped: true` instead of becoming an episode:

```bash
cortexiq remember "prima, dank je wel, tot morgen"   # {"skipped": true, "reason": "..."}
cortexiq remember "push naar main"                   # stored
```

The test is not length (`CORTEXIQ_EXTRACTION_MIN_CHARS` already keeps fragments away from the extractor) and not politeness: `ja doe maar` and `nee, dat werkt niet` are decisions and instructions, and they survive because `doe` and `werkt` say something. Only a closed vocabulary of phatic words matches, in Dutch and English, so a closer that slips through costs one episode while nothing with a subject is ever swallowed. `CORTEXIQ_SOCIAL_FILTER=false` stores everything again.

## Reflect: from memories to answers

`recall` returns ranked memories; `reflect` returns an answer. It recalls relevant memories, then synthesizes a grounded response with the LLM — every claim traces back to a recalled memory, and when nothing relevant is found the answer says so instead of inventing one.

```bash
cortexiq reflect "what queue do we use and why did we switch?"
# {
#   "answer": "We use Kafka. We switched from RabbitMQ in March because...",
#   "sources": [
#     {"type": "fact", "id": 42, "content": "the queue is kafka", "score": 0.91},
#     {"type": "fact", "id": 38, "content": "switched from rabbitmq to kafka in march", "score": 0.87}
#   ],
#   "recall_count": 5,
#   "duration_ms": 340
# }
```

The same operation is available as the MCP `reflect` tool and as `POST /api/v1/reflect`. All recall options carry over: `namespaces`, `limit`, `types`, `context` (extra text the answer should consider), and the time bounds (`as_of`, `from`/`to`, `known_from`/`known_to`) — so `reflect "what did we know in March about the outage?"` answers from what the system held then, not from what it knows now.

Use `recall` when you need the raw memories (exact wording, IDs to act on). Use `reflect` when you need an answer — summaries, explanations, "what do we know about...".

## Benchmarking

CortexIQ ships with a benchmark runner for standardized memory evaluation, inspired by Hindsight's LongMemEval integration. It ingests conversation sessions, answers questions with `reflect`, and scores the answers — either by exact match or with an LLM judge.

```bash
# List available benchmark datasets
cortexiq benchmark list

# Run the example benchmark (exact-match judging)
cortexiq benchmark run --dataset eval/benchmark/example.json

# Run with an LLM judge (slower, more nuanced scoring)
cortexiq benchmark run --dataset eval/benchmark/example.json --llm-judge

# Run a subset of items
cortexiq benchmark run --dataset eval/benchmark/example.json --max-items 5

# Save the JSON report
cortexiq benchmark run --dataset eval/benchmark/example.json --output report.json
```

The report includes accuracy, average judge score, exact match rate, latency, and per-category breakdowns:

```
=== Benchmark: example ===

Items:       3
Questions:   12
Episodes:    6
Duration:    15420 ms
Errors:      0

--- Scores ---

Accuracy:           83.3%
Avg judge score:    0.792
Exact match rate:   25.0%
Avg latency:        1285 ms/question

--- By Category ---

category                       n  accuracy  avg score
single-session                 6     100.0%     0.917
multi-session                  4      75.0%     0.688
temporal                       2      50.0%     0.500
```

### Dataset format

Datasets are JSON files compatible with LongMemEval — an array of items, each with `haystack_sessions` (conversations to ingest) and `qa_pairs` (questions with gold answers). See [`eval/benchmark/README.md`](eval/benchmark/README.md) for the full format spec and [`eval/benchmark/example.json`](eval/benchmark/example.json) for a working example.

### CI integration

A GitHub Actions workflow (`.github/workflows/benchmark.yml`) runs the benchmark on every push to `main` and on PRs that touch benchmark code. It fails when accuracy drops below the configured threshold (`BENCHMARK_MIN_ACCURACY`, default 0.5), enabling regression detection. The report is uploaded as an artifact and PRs get a results comment.

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

Recall results carry an `id` that is only unique **within its own table**: facts and episodes have separate id sequences, and the same number exists in both. Every result therefore also carries a typed field — `fact_id` for a fact, `episode_id` for an episode — and a `type` (`"fact"` or `"episode"`). Act on the typed field, not on `id`: `DELETE /api/v1/facts/<an episode id>` deletes an unrelated fact, and the server cannot tell that you meant a different row.

### Time-aware recall

Recall accepts optional `from` and `to` bounds as an RFC3339 timestamp or
`YYYY-MM-DD`. They form a half-open interval `[from, to)`: episodes are matched
by `occurred_at`, while facts are matched when their validity interval overlaps
it (falling back to `created_at` when `valid_from` is absent). That is *valid
time*: when something happened or was true. Separately, *knowledge time* asks
what the system held: `as_of` answers "what did we know at that moment" for facts
and episodes alike (a fact counts from `created_at` until it was superseded or
expired, `known_until`), and `known_from`/`known_to` give the half-open window
`[known_from, known_to)` of the same axis ("what did we hold during March",
including beliefs replaced inside it). `as_of` is the point form, so it cannot be
combined with `known_from`/`known_to`; either can be combined with `from`/`to`.
A fact recorded in October about March is not returned by `as_of` in March, and
a fact whose validity ended is still returned by an `as_of` after that, as long
as it had not been replaced. CLI and MCP recall also recognize a small deterministic set of
Dutch and English date phrases such as “yesterday”, “last month”, “last March”,
“gisteren”, or “maart 2025”; explicit bounds take precedence. Wording decides the
axis: past-tense epistemic phrasing (“what did we know in March”, “wat wisten we in
maart”, “as of March”) makes the expression a knowledge window, while plain
“what happened in March” stays valid time. “About” or “over” ties an expression to the
subject instead (“what did we know about the outage in March” is the outage’s March),
and one query can carry both axes (“what did we know in March about the outage in
January”). Present tense (“what do we know”) is not a cue. A recall trace lists what
was inferred under `inferred`. This inference can be disabled
with `CORTEXIQ_TEMPORAL_RECALL=false`; the time-ranked recall arm can be disabled
separately with `CORTEXIQ_TEMPORAL_ARM=false`.

The REST API accepts the same explicit bounds on `POST /api/v1/recall`, for
example `{"query":"project updates","from":"2025-03-01","to":"2025-04-01"}`.
For `GET /api/v1/facts` and `cortexiq facts`, `from`/`to` filter validity
overlap and `as_of`, `known_from`, `known_to` filter knowledge time; the older `since`/`until` filters still mean `created_at`.

To protect CortexIQ, set `CORTEXIQ_API_TOKEN` in `.env`. It guards both the dashboard API (`/api/v1`) and the MCP server (`/sse`, `/message`): every request then needs `Authorization: Bearer <token>`. The dashboard asks for the token on first use and remembers it in the browser; MCP clients need to send the header (see below). Leave it empty to keep everything open. `/healthz`, `/readyz` and `/metrics` are not covered.

### Scoped tokens

For several callers, use `CORTEXIQ_API_TOKENS`: a JSON array of tokens, each with
a name and a scope. `CORTEXIQ_API_TOKEN`, if set, stays valid as one extra
full-access token — the two add up, so a dashboard token can never be lost by
listing others.

```json
[
  {"name": "dashboard", "token": "<hex>", "scope": "rw"},
  {"name": "agent", "token": "<hex>", "scope": "ro"}
]
```

- `scope`: `rw` (default, read and write) or `ro` (read only). Read-only tokens
  may only run read operations; the dashboard allows `GET`/`HEAD` and
  `POST /api/v1/recall`, and the MCP server allows only read tools (`recall`,
  `query_*`, `list_*`, `get_context`, `trace_causal_chain`).
- `name`: only used in logs and error messages.

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
