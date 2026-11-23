# Status

Living page: what this build is, what is switched on, and what is still open.
For the dated snapshot it replaces, see [handoff-2026-10-18](handoff-2026-10-18.md) —
that file is history and is deliberately not updated.

Last updated: 2026-11-23, `v0.1.99` is the last deploy.

## Build

| | |
|---|---|
| Latest release | `v0.1.99`, 2026-11-23 |
| Release commit | `f9773e0` |
| Every release | tagged and published; `gh release list` is the authoritative list |
| Schema | `goose_db_version` reads `63`; migrations are append-only |
| How to check | `docker compose exec -T cortexiq /cortexiq version` — the image has no shell and `cortexiq` is not on `PATH`, so the absolute path matters |

A binary built from a working tree reports `cortexiq dev (built from source)`.
Release and Docker builds inject the tag, commit and build date; the values live
in `internal/version`.

This page avoids counts that change on every release (tags, commits, tool totals).
Where a number appears here, it is a measurement with a date attached.

### Since the 2026-11-08 snapshot (v0.1.61)

The snapshot that stood here described `v0.1.61`. What changed since, in the order
it landed:

- **Patterns reach the answer** — a reflected answer now includes patterns whose
  terms recur against the question instead of any pattern sharing one word, and the
  Patterns tab shows what was extracted. A rejection can no longer overwrite a
  cluster that was already accepted.
- **Entities at the source** — name variants fold into one entity (`Hindsight` next
  to `hindsight`, `'Wipe All Data'` next to `Wipe All Data`), descriptions and
  placeholder types are refused, and a quote, port or role inside a name no longer
  becomes part of it. Contradiction detection compares entity and property
  case-insensitively (migration `00062` adds the index for it), which is how two
  facts that disagreed only in capitalisation could finally contradict each other.
  The sweep over relationship clusters got one predicate identity (`pushed to` =
  `pushed_to`) and skips clusters a live pattern already covers.
- **BM25 keyword recall** — the keyword arm moved from `ts_rank_cd` over a stored
  tsvector to real BM25 through `pg_textsearch`, which is why PostgreSQL 17 is now a
  hard requirement; the tsvector columns and their GIN indexes are gone (migration
  `00061`, `v0.1.74`). On 500k synthetic rows one word took the old arm 160–230 ms and
  BM25 0.4–6 ms; on 15 real queries the top-3 changed mostly in BM25's favour, which
  is the IDF effect working.
- **Lenient list parsing** — every list-shaped step accepts a bare array, one loose
  object, an array under a key, or `{}`, and repairs `"fact_id": "3128"`. Before
  `v0.1.76`, 179 of 221 runs with a model call in 24 hours failed on `cannot
  unmarshal object into []…`; with the parsers fixed the first error moved on to
  truncation instead of disappearing (`v0.1.75`–`v0.1.76`).
- **Graph arm weighted per question shape** — instead of on or off, the arm can be
  fused at 0.1 for a topic question and 0.5 for a relational one
  (`CORTEXIQ_GRAPH_ROUTE`, default off, on in production since 2026-11-13). On 36
  questions written against a copy of production: 0.72 MRR with the arm off, 0.65
  with equal weight, 0.76 weighted — measured without a reranker, which is exactly
  where the arm matters (`v0.1.83`).
- **Reranker made honest** — an unconfigured reranker crashed recall through a typed
  nil, a trailing slash in the base URL produced `//rerank` and a silent 404, and a
  failing reranker was invisible. All three are fixed: an empty configuration is a
  no-op, the slash is stripped, and `recall_rerank_requests_total{status}` plus one
  log line per minute report a failure. `CORTEXIQ_RERANK_CANDIDATES` went from 100 to
  30 because Jina allows 100,000 tokens per minute (`v0.1.74`–`v0.1.84`).
- **Dashboard: Timeline and Clean-up** — a timeline per entity with a time cursor and
  a two-date comparison (true then / new since / no longer true), a Clean-up tab for
  duplicate entities, islands and weak relationships, and a hover card in the graph
  that shows the facts behind a node (`v0.1.78`–`v0.1.81`).
- **Schema setup under a lock** — `db.Open` runs goose and its check-then-create
  steps under a Postgres advisory lock, because the two containers starting together
  raced on the same migration and one died on `relation … already exists`. A
  cancelled waiter leaves no lock behind (`v0.1.75`).
- **OAuth loopback ports** — a client that registers `http://127.0.0.1/callback`
  without a port may use the port the operating system gave it, as RFC 8252 §7.3
  requires; host and path still have to match exactly (`v0.1.77`).
- **Model prices** — rates are read from the provider catalogues every six hours, and
  a model no catalogue prices can be priced by hand in Settings → Models; a catalogue
  price always wins over that row (migration `00063`, `v0.1.85`).
- **Reasoning models get room to think** — the extraction call was capped at 1024
  output tokens, and a reasoning model spends that same budget before it writes
  anything, so a bound model returned empty answers that surfaced as JSON parse
  errors or as runs that silently created nothing. The default is 8192, a bound
  model follows the configured budget and JSON mode instead of a forced one, and
  JSON mode's `{"facts": [...]}` wrapper is unwrapped (`v0.1.86`).
- **The answer can arrive as content blocks** — a reasoning provider sends the whole
  message as `[{"type":"thinking","thinking":[…]} {"type":"text","text":"[…]"}]`. The
  OpenAI SDK reads `content` as a string, stores the raw array there and reports the
  mismatch only loosely, so every stage that asked for JSON was handed a block list
  instead of its answer: the shape sweep died on `unreadable answer to 1 shape
  candidates: [{"type":"thinking"…`. `chat` now flattens blocks before any stage sees
  the content — reasoning is dropped, the text blocks are the answer — and an answer
  with nothing but a thinking block is a `ReasoningOnlyError`, which counts as an
  answer problem so the relationship stage gives up on a batch that keeps arriving
  that way. When `finish_reason` is `length` the trace ran out of budget and the
  existing budget error stands. It cost 13 runs since 2026-11-08 and every one of the
  6 failing runs after the 2026-11-23 model switch.

- **Clean-up no longer asks what it can decide** — a group whose spellings differ
  only in capitals (`Monitor`, `monitor`) holds no information to weigh, yet the
  screen asked for a Keep press per spelling, and the Bulk rule "keep the spelling
  with the most facts" would happily keep the capitalised form that the next ingest
  writes lowercase again. The server now returns `keep` — the spelling
  `NormalizeEntity` writes — for those groups only (`internal/brain/cleanup.go`,
  `DuplicateGroup.Keep`), the row renders one "Normalise to …" button instead of a
  Keep per variant, and Bulk in a capitals-only view merges straight to that form.
  A group whose only difference is a leading "the" got its own kind (`article`) and
  the same treatment, because dropping the article loses nothing; a trailing
  `(note)` stays `qualifier`, where `Foundation` and `Foundation (Gaia)` may be two
  things. Punctuation still asks.
- **And the sweep takes it without anyone pressing** — `Brain.SweepCleanup`
  (`internal/brain/cleanup_sweep.go`) walks the same duplicate groups and merges the
  decidable ones through `MergeEntities`, one group per transaction, so the rules for
  what a merge touches are unchanged. It runs beside consolidation in the process that
  owns background writes, `CORTEXIQ_CLEANUP_SWEEP` (default on) and
  `CORTEXIQ_CLEANUP_SWEEP_INTERVAL` (default `6h`), first pass five minutes after
  start. `CORTEXIQ_CLEANUP_SWEEP=false` gives the old behaviour, where only the screen
  merges. A group that fails is counted and skipped; one log line says how many merged
  and how much was rewritten. First production pass (v0.1.98): 50 groups merged, 623
  facts and 755 relations rewritten, no failures.
- **Correction: a merge target must be a spelling of the group itself** — v0.1.96
  built `keep` from `NormalizeEntity`, which also trims trailing words (`config`,
  `server`, `agent`) and cuts at " in " / " on ". On live data that turned `The
  Hermes agent config` into `hermes` and `GTM server` into `gtm`, i.e. a group welded
  onto a different entity. `canonicalSpelling` now only returns a spelling that is in
  the group and survives normalisation unchanged, and the comparison against the kept
  spelling is exact: with a case-insensitive one every capital group folded nothing,
  so the button looked right and did nothing. Released as v0.1.97.
- **Mistral as a third provider** — Settings → Models → Providers lists `mistral`
  next to `hopscotch` and `deepinfra` (`slug mistral`, `https://api.mistral.ai/v1`,
  key stored write-only). No code changed: `GET /v1/models` is the OpenAI envelope
  `parseCatalogue` already reads, and `modelsPath` uses `{base}/models`. Production
  `test` answers 62 models. The catalogue states context length and capabilities
  (`reasoning`, `function calling`, `completion fim`) but no rates, so the 35 models
  that matter for text work were priced by hand from the model cards on
  `docs.mistral.ai/models`: Large 4 $0.7/$2.1, Medium 3.5 $1.5/$7.5, Small 4
  $0.15/$0.6, Magistral 1.2 $2/$5 and $0.5/$1.5, the Ministral 3 family, Codestral,
  the Embed models with an input rate only, and the resold DeepSeek and Z.ai models.
  What stays unpriced cannot be said in dollars per million tokens: OCR is per 1 000
  pages, Voxtral per minute or per character, Moderation is free (a 0/0 row is
  refused), and `mistral-code-*` and `mistral-vibe-cli-*` are products whose token
  rate Mistral does not publish. `mistral-medium`, `mistral-medium-2604` and
  `mistral-medium-3.5` carry Medium 3.5's rate although the card lists only
  `mistral-medium-3-5`, `-3` and `-latest` as API names. A rate typed here is
  re-read every minute, so no restart is needed, and a rate Mistral ever publishes
  for the same id would outrank it.

### Earlier: since the 2026-10-30 snapshot

The snapshot that stood here described `v0.1.16`. What changed since, in the order
it landed:

- **Tags** — `tags_any` is an OR filter next to the AND filter, on recall and on
  reflect (`TagFilter{All, Any}`). Any tag condition excludes failures and goals,
  which carry no tags (`v0.1.49`).
- **Goals** — `goal_events` is an append-only log; the consolidator writes
  `progress` / `suggested_complete` / `contradicted` events instead of appending
  text to `goals.notes`, and it still never completes a goal itself. `remember`
  takes `complete_goal` so an agent can close one write with its evidence
  (`v0.1.50`).
- **Async writes** — `remember` accepts `async: true` and a durable `write_jobs`
  queue drains it (`SKIP LOCKED`, two processes safe). Off by default, because an
  async caller gets no episode id back (`v0.1.50`).
- **Patterns** — visible and deletable (`GET/DELETE /api/v1/patterns`, dashboard
  tab, `list_patterns`), deduplicated in the schema with a generated `content_key`
  and a unique partial index, and the prompt now asks for recurrence rather than a
  summary. Three parser defects surfaced only against a live model: an object
  wrapper, an id-array as the first array, and `coherence: 0.0` being clamped to
  `0.5` when it means "this is not a recurrence" (`v0.1.56`–`v0.1.61`).
- **Shape sweep** — a second pattern path that groups relationships by relation
  type and target and asks the model only to phrase or reject a candidate three
  or more sources point to. A window of consecutive facts cannot see recurrence
  that spreads over weeks; the relationship graph can. Its own `pattern_clusters`
  cursor keeps a judged candidate out of the queue until new edges arrive.
- **Stall detection** — each stage's checkpoint gets a `stage_changed_at`
  timestamp, stalled stages show on the Runs tab, and the scheduler retries when
  episode or relationship work is pending (`v0.1.51`–`v0.1.54`).
- **Model registry** — provider model metadata is read from Hopscotch's
  `data[].uniblock` block (capabilities, context window, deprecation) (`v0.1.55`).


Consolidation model: the environment still says `deepinfra/gemma-4-31B-it-turbo` via DeepInfra
(`CORTEXIQ_REASONER_MODEL`, `CORTEXIQ_REASONER_JSON_MODE=false`), but production has not used it
since 2026-11-08 08:21 — the `reasoner` row in `llm_bindings` points at
`baseten/DeepSeek-V4-Flash-0731` through the `hopscotch` provider. Triage is bound to
`deepinfra/Meta-Llama-3.1-8B-Instruct-Turbo` (the same value sits in `CORTEXIQ_TRIAGE_MODEL`), and
the critic is an environment model of its own, `deepinfra/DeepSeek-V3` (`CORTEXIQ_CRITIC_MODEL`),
which reuses `CORTEXIQ_REASONER_JSON_MODE`, so it runs without JSON mode.

The history: Gemma replaced `mistralai/Mistral-Small-24B-Instruct-2501` on 2026-10-30 because, on the
same inputs, Mistral failed to parse 13 of 40 fact calls and 6 of 20 relation batches and merged 13 of
48 genuinely different pairs in the same-fact review; Gemma had no parse errors on those stages and
merged 0 of 48 (it misses 3 of 20 true twins, which leaves a duplicate rather than losing
information). That measurement is `internal/reasoner/modeleval_test.go` (`-tags modeleval`); the
numbers are in `walkthrough2.md`.

It also produced a rule this page had to contradict: do not bind a reasoner model in the dashboard,
because that path built the client with JSON mode forced on and Gemma failed every stage that returns
a list with it. `v0.1.86` replaced the rule with the measurement behind it. A bound model now takes
`CORTEXIQ_REASONER_MAX_TOKENS` and `CORTEXIQ_REASONER_JSON_MODE` from the configuration (only triage
keeps a forced JSON mode and its own small budget), and the default output budget went from 1024 to
8192 tokens. The reason the binding looked broken on every run was the budget, not the model or the
provider: `baseten/DeepSeek-V4-Flash-0731` is a reasoning model, `max_tokens` covers the thinking and
the answer together, and 1024 tokens ran out mid-thought, leaving an empty message that surfaced as
`no JSON array or object in the response` or `unexpected end of JSON input`. Probed against the same
provider with the same prompt: JSON mode at 1024 and at 4096 → `finish_reason=length`, empty content;
without JSON mode at 1024 → length, empty; at 4096 → 2300 tokens and a valid array; JSON mode at
16384 → 3538 tokens and `{"facts": [...]}`. `reasoning_effort: low` did not
help. That second result exposed a real parser bug: JSON mode wraps the array in an object, which the
parser read as one fact without a summary and discarded silently — it now unwraps `{"facts": [...]}`
first. An answer that stops on the length limit with nothing in it is no longer reported as a parse
error but names the budget that ran out. The data before the fix: of 669 runs with a model call since
the binding was created, every one recorded at least one error (dominated by truncated JSON), and 311
of them had no `error_sample` at all and `facts_created=0` — the silent version of the same failure.
Live proof on `v0.1.86`: one new episode in `/eval` produced 6 model calls, 5 facts, 3 relationships
and 4 causal links with no error sample, then was purged again. On `v0.1.88` the same
probe (a write through the dashboard listener) also started its run in the same second
instead of waiting for the timer. The binding stays; what is still unmeasured is how
much of the earlier `relationships` failure (a provider error returned as content, on
the same fact cluster each time) survives the bigger budget — since `v0.1.88` that
answer is retried unchanged and, after three unreadable runs, written off so the
checkpoint moves on instead of freezing the namespace.

## What runs where

- Production: the `cortexiq-cortexiq-1` and `cortexiq-cortexiq-ui-1` containers on
  a single VPS, in `/opt/cortexiq`, currently `v0.1.95`. That is the version the
  binary reports: `docker compose exec -T cortexiq /cortexiq version` (the image is
  distroless, so there is no shell and no `cortexiq` on `PATH`). Postgres 17 with
  pgvector and pg_textsearch, plus a local Ollama for embeddings, sit beside them and
  are not replaced on a release. A deploy is: pull the new tag while the old version
  still runs, stop the two app containers, take a verified dump, set
  `CORTEXIQ_IMAGE_TAG` in `.env`, `docker compose up -d`, then confirm the goose
  version and the containers. Migrations `00038`–`00058` ran between `v0.1.17` and
  `v0.1.57`; `00059` (pattern cursor), `00060` (pattern source entities), `00061`
  (BM25) and `00062` (the case-insensitive entity index) followed up to `v0.1.82`;
  `00063` (`model_prices`) applied on the `v0.1.85` restart. They are all idempotent,
  and goose now runs them under an advisory lock so two containers cannot race.
- The MCP server hands its prime directive to a client at connect time, in the
  `initialize` result's `instructions` field (6445 characters on `v0.1.4`). A
  client reads that once per connection, so it has to reconnect after an upgrade
  to pick up a change. Hermes does not depend on it: its `cortexiq` memory
  provider recalls before a turn and captures after it, and writes only when a
  session actually runs — which is why the silence from 24 to 27 October was
  usage, not a broken integration.
- The dashboard (`/ui`) is three columns: navigation, the content, and a context
  column that lists the facts an episode produced or the episodes behind a fact.
  It collapses from the topbar or its own × and becomes a drawer below 1360px;
  the choice is kept in that browser, not on the server. Since `v0.1.8` it
  wears the logo's palette (teal-green accent, olive second colour) and the
  brain mark is served from `/ui/brand/mark-96.png`. Since `v0.1.13` the facts
  and episodes lists are one readable column and all of their metadata (tags,
  namespace, validity, confidence, claim, timestamps) sits in that context
  column as a labelled grid; the rail animates and respects
  `prefers-reduced-motion`. Since `v0.1.14` the navigation rail and the context
  column stay pinned while the content scrolls: on wide screens both kept their
  `position: sticky` declarations, but `overflow-x: hidden` on `html`/`body`
  made `body` a scroll container that never scrolls itself, which silently
  defeated sticky; `overflow-x: clip` restores it. On screens wider than 1600px
  the context column starts at 600px instead of 432 — a starting width, since a
  saved width always wins and the grip still reaches the 320px floor.
- Deploying: production pulls the image CI built —
  `ghcr.io/bojanni050/cortexiq:${CORTEXIQ_IMAGE_TAG}`. To deploy, bump
  `CORTEXIQ_IMAGE_TAG` in `/opt/cortexiq/.env` to the new release and run
  `docker compose -f docker-compose.prod.yml up -d`; nothing is compiled on the
  server, so the running binary is byte-for-byte the one the release workflow
  produced (compare `.Image` with the tag's digest). Rolling back is setting the
  previous tag. Migrations run on startup. Pull with the new tag inline —
  `CORTEXIQ_IMAGE_TAG=0.1.90 docker compose pull cortexiq cortexiq-ui` — because
  compose reads the tag from `.env`: a plain `pull` after bumping fetches the old
  image and the deploy then looks successful while running last release's binary.
  That is what the first attempt at the `v0.1.85` deploy did.
- `build: .` is no longer in the prod compose. The old locally built
  `cortexiq:local` image and `0.1.0`/`0.1.1` stay on the server as rollback
  artifacts, about 90 MB together; nothing pulls them. The local `latest` tag was
  untagged, so nothing on the server suggests a moving tag.
- Registry tags: exactly one tag per release (`0.1.2`, `0.1.3`, …) and nothing
  else. Checked at the registry after `v0.1.15`: `0.1.2` through `0.1.15`, fourteen
  tags for fourteen releases, no `latest` and no floating `0.1`. The workflow still
  sets `flavor: latest=false`, so `v0.1.88` published `0.1.88` and no `latest`.
  Past releases are kept rather than deleted, so a rollback can still pull the tag
  it names; production pulls `0.1.90` today. With a moving tag, "which version runs
  here" becomes a guess. `CORTEXIQ_IMAGE_TAG` is a required variable in the compose
  file, so a missing value fails the deploy instead of silently pulling
  something old.
- Getting there took one correction. `v0.1.1` was published with a `latest` tag
  even after the explicit `type=raw,value=latest` was removed, because
  `docker/metadata-action` defaults to `flavor: latest=auto` and adds `latest`
  itself. `v0.1.2` sets `flavor: latest=false`, and the result was checked at
  the registry (`/v2/bojanni050/cortexiq/tags/list` with an anonymous pull
  token) rather than in the GitHub UI. The old versions were then deleted; a tag
  cannot be removed on its own, only the version it belongs to.
- Untagged manifests still in the GHCR versions list are buildkit attestations,
  not images; they are harmless.
- This page may be one documentation-only commit ahead of the deployed binary;
  compare with `cortexiq version` rather than assuming.
- Storage: Postgres 17 + pgvector. At the `v0.1.88` deploy: 2,928 live facts,
  856 episodes, 30 namespaces. Tag coverage at the same moment: 926 facts and 297
  episodes carry a tag, over 137 distinct fact tags; `cortexiq tags` gives the
  current numbers.

## A namespace on every write

A memory's namespace says where it belongs, and the `remember` tool used to
default to `/` when a client left the parameter out: the memory landed among
everything else and was found by nobody. Since `v0.1.10` it is required. The tool
refuses without one and names the tool that answers the question —
`list_namespaces` — so the client can correct itself in the next call instead of
guessing; an explicit `/` is still accepted for a memory that is genuinely
global. The four record tools (`create_causal_link`, `create_hypothesis`,
`create_goal`, `create_failure`) refuse the same way. A client that reaches the
server without a namespace therefore stops working until it passes one, which is
the intent; the bundled Hermes integration always passes a path, and the CLI
keeps its own `/` default because a person typing a command can see the flag.

The reason it is asked per write rather than answered once by configuration: the
client knows which project it is working in and the server does not. A token that
carried a namespace was built and then dropped — an app works in more than one
project, so a default per app files the same project's knowledge under two names
depending on which editor wrote it. Namespaces belong to projects; tokens belong
to apps. Keeping those on separate axes is the point.

```
cortexiq token new --name opencode
```

generates the secret and prints the entry to paste into `CORTEXIQ_API_TOKENS` on
the server, then a restart. The secret is shown once and stored nowhere.

`cortexiq connect` writes the server entry into the clients on the machine.
Cursor's `mcp.json` is edited directly — plain JSON, and a file that turns out to
carry comments is refused rather than rewritten — while Claude Code and opencode
are driven through their own CLI, so whatever else those tools store beside
their servers survives. `--app` names one client or `all`, and an unknown name is
an error rather than a silent skip; `--global` picks the machine-wide file over
the project's; `--env-ref` writes the client's own environment reference
(`${env:…}` for Cursor, `{env:…}` for opencode, `${…}` for Claude Code) so the
secret never lands on disk; `--dry-run` prints what it would do. `disconnect`
reverses it. opencode offers no command to remove a server, so that path points
at the entry to delete instead of guessing at a JSONC file.

Four commands run without a database — `version`, `connect`, `disconnect` and
`token` — because configuring a laptop that has no CortexIQ of its own must not
require one.

The same work fixed a real defect: those four record tools resolved `/` to
*every* namespace and then indexed the first element, so a record meant for the
root could be filed under an arbitrary namespace. They now resolve exactly one
path, and refuse when they are handed none.

A token's `scope` decides what it may call. `ro` reaches fourteen tools — the
`recall`/`query_*`/`list_*` family plus `reflect`, `get_context`,
`trace_causal_chain` and `memory_stats` — and is refused everything else with
`403 {"error":"read-only token: tool not allowed"}`. `reflect` sat on the refused
side until `v0.1.12` even though its handler only calls `Brain.Reflect` and the
reflect path contains no insert: a read-only connection was denied the one tool
that reads across everything it may see. `init` stays refused, because it creates
the `/self` scaffold. The gate reads the `tools/call` body and nothing else, so
`initialize`, `tools/list` and `ping` pass for every token — a client can ask
what exists, and then learns the refusal at the call.

## A tag is asked for, not demanded

A namespace says where a memory lies; its tags say what it is about, and they
outlive it — consolidation replaces the raw episode with facts that inherit the
union of its tags, so a tag keeps filtering after the memory it came from is
gone. The namespace is a gate. Tags are not, and the difference is not
sentiment: a memory without a namespace is unfindable, a memory without a tag is
still findable, and a tag invented to satisfy a gate ("overig", "algemeen")
fragments exactly the vocabulary that `NormalizeTags` exists to protect. That
function already rejects what would drift — more than 8 tags, longer than 32
characters, anything outside lowercase letters, digits, `-`, `_` and `:` — and one
near-duplicate filter ("beslissing" against "Beslissing" against "beslissingen")
costs more than one forced tag.

What the write path does instead is answer. A `remember` without tags comes back
carrying `"untagged": true` and a hint that names the tags that namespace already
uses, on the turn where the writer is still looking:

```json
{"id": 3, "tags": [], "untagged": true,
 "hint": "Stored without tags. … This namespace already uses: beslissing, bugfix."}
```

That is the enforcement: the tool descriptions ask for tags and point at
`list_tags`, and the response says so when they were left out, with the real
vocabulary instead of a generic reminder. A response is read on the turn where a
client can still act on it; a rule in a document is read when someone remembers
to write it.

Coverage is counted for the same reason. `list_tags` — and `cortexiq tags` —
return the vocabulary *and* how many live memories carry no tag at all. This is
production at the `v0.1.12` deploy, trimmed to one entry:

```json
{"coverage": {"facts_tagged": 82, "facts_untagged": 1084,
              "episodes_tagged": 17, "episodes_untagged": 302},
 "tags": [{"tag": "beslissing", "facts": 43, "episodes": 9, "total": 52}]}
```

Twelve tags over two hundred memories and twelve tags over two thousand look
identical in the vocabulary alone; the count is what shows the gap. Here the gap
is most of the store — 82 of 1,166 facts and 17 of 328 episodes carry a tag, and
that number was not visible at all before this release. It counts what is live
now and ignores the period filter, because a period describes what was recorded
and not what the store looks like.

One thing tags cannot do: a memory that is already stored cannot be tagged. A
client that reads the hint can only tag its next write — there is no `add_tags`
tool and no CLI verb for it. Retro-tagging is the obvious next step, and it is
not built.

## Switched off, on purpose

| Setting | Production | Why |
|---|---|---|
| `CORTEXIQ_GRAPH_RECALL` | `false` | The switch itself is off: with the arm on at equal weight, relational questions gain less than topical ones lose (37 queries, 2026-10-26). Production runs `CORTEXIQ_GRAPH_ROUTE=true` instead, so relational questions get the arm at weight 0.5 and topic questions at 0.1. Any call can still force the plain switch with `"graph": true`. See below. |
| `CORTEXIQ_RERANKER_MODEL` | `jina-reranker-v3.5` | On, with `CORTEXIQ_RERANK_CANDIDATES=30`. It re-scores fused candidates and owns the top of the list; the cap keeps an ordinary recall under Jina's 100k-tokens-per-minute limit. A local reranker was measured and rejected on latency (below). |
| `CORTEXIQ_RECALL_MIN_SIMILARITY` | `0.40` | Drops weak vector-only hits. Model dependent. |
| `CORTEXIQ_CONSOLIDATION_CANDIDATE_CAP` | `0` (code default, not in `.env`) | Measured on 272 real episodes (2026-10-26): the lexical arm would add a member to 271 of them, but the near-threshold pairs are mostly state changes (v0.1.2 → v0.1.3, bug found → bug fixed, volume not shared → shared), not restatements. Left at 0. |
| `CORTEXIQ_CONSOLIDATION_DEDUP_THRESHOLD` | `0.98` | Auto-merge only near-identical facts. On 932 real facts, every pair between 0.85 and 0.98 differed in a number, an entity or a state. |
| `CORTEXIQ_CONSOLIDATION_DEDUP_REVIEW_THRESHOLD` | `0.85` | Below this, an LLM verdict was always "different"; above it the pair is worth asking. Narrowed from 0.75, which sent a third of all facts through the model. |
| `CORTEXIQ_TRIAGE_MODEL` | `deepinfra/Meta-Llama-3.1-8B-Instruct-Turbo` | Set in `.env` and bound to the same model in `llm_bindings` (through `hopscotch`), so both say the same thing and the binding is what start-up applies. It classifies a cluster before extraction and can refuse acknowledgements; without it everything goes to the reasoner. It fails open (unusable answer = durable), so enabling it risks cost, not memories. Set `CORTEXIQ_TRIAGE_LOG=true` to log one line per triage call. |
| `CORTEXIQ_WRITE_FILTER` | `off` (code default, not in `.env`) | Since `v0.1.8`, opt-in: `warn` logs what it would refuse, `strict` refuses secrets and noise. Left off until the false-positive rate on real writes is known. The hygiene cleaner that shipped beside it was removed in `v0.1.9`: it audited noise the store's own decay already handles, and its archive flag was read by nothing. |
| `CORTEXIQ_ROUTE_RECALL` | `false` (code default, not in `.env`) | Only widens recall; never measured in production. |
| `CORTEXIQ_RECALL_MMR_LAMBDA` | `0` (code default, not in `.env`) | Available per call; off by default so ranking stays plain fusion. |
| `CORTEXIQ_CONTRADICTION_REVIEW_THRESHOLD` | `0` (code default, not in `.env`) | Unreleased (2026-11-17). Off. With 0.85, a new fact that shares entity and property with an older one and is at least that similar gets the dedup's same-fact LLM review first; a rewording no longer expires the older fact. Measured on the 54 recorded contradictions: 9 pairs score 0.85 or more, 5 of them clear rewordings. Tried on all 55 recorded pairs against the production reasoner (2026-11-17, `eval dedup --llm`): it called 3 of the 5 clear rewordings "same" (facts 32, 55 and 106, which carry 146 of the 190 relations) and none of the 50 other pairs, so no real change was shielded. It kept facts 76 and 1924 apart, whose newer text adds detail. One run, one model. Off in production until the owner sets it. |
| `CORTEXIQ_DECAY_PROTECTED_TAGS` | `kern` | Since `v0.1.89` (2026-11-17), set in `.env` and passed through `docker-compose.prod.yml`. Facts carrying one of these tags are skipped by the decay sweep, so identity and core preferences neither fade nor expire from disuse. Empty (the code default) protects nothing; only facts that carry the tag are skipped, and no fact carries `kern` yet, so nothing is protected until episodes are tagged. See `design-decay-split.md`. |

These are the values read from `/opt/cortexiq/.env` on 2026-11-16, plus the defaults
that apply to settings the file does not carry (marked above). Also on:
`CORTEXIQ_GRAPH_ROUTE=true` (0.1 topic, 0.5 relational), `CORTEXIQ_DECAY_SPLIT=true`
(disuse lowers a fact's strength, not its confidence), `MAX_RESULT_SIZE=10000`,
`RERANK_CANDIDATES=30`, `REASONER_MAX_TOKENS=8192`. Two of the rows used to be
accidents rather than decisions: the candidate cap looked like ordinary configuration
nobody had measured, and the dedup thresholds were in the file twice, so the effective
values came from whichever line happened to be last (see below).

### Consolidation: what merges, and what does not (272 episodes, 932 facts)

Both consolidation decisions were measured on production data, read-only, rather
than argued from the synthetic corpus:

- **Episode clusters.** At the 0.85 similarity threshold only **9 pairs** of the
  272 active episodes reach each other at all, so clusters are effectively single
  episodes today. Turning on the lexical arm (candidate cap > 0) would add a
  member to **271 of 272** episodes, but the pairs just under the threshold are
  mostly state changes — a plugin at `v0.1.2` that broke versus `v0.1.3` that
  fixed it, a bug found versus the same bug fixed, "no shared volume" versus
  "shared volume is live". Merging those would erase the sequence. The synthetic
  result that promised 20/20 on rewordings did not transfer, because its "same"
  pairs were restatements. Cap stays `0`.
- **Fact dedup.** Of 91,290 same-namespace fact pairs, **0** reach 0.98, 33 reach
  0.85, 99 reach 0.80 and 267 reach 0.75. Read by hand, the 33 near-identical
  pairs are mostly not duplicates either: hit@1 0.75 next to hit@3 0.97, port
  8445 (UI) next to port 8444 (MCP), Hindsight cold-load next to CortexIQ
  cold-load. Cosine on this corpus says "same subject", not "same fact".

The `.env` had `CORTEXIQ_CONSOLIDATION_DEDUP_THRESHOLD` twice — `0.85` and then
`0.98` — and the container took the last one, while the review floor sat at
`0.75`. Net effect: nothing ever auto-merged (no pair reaches 0.98 anyway) and
336 facts, 36% of the corpus, were each sent to the model for a verdict whose
answer was almost always "different". The duplicate line is gone; the values are
now `0.98` and `0.85`, so a review happens only where it can change something.

Extraction itself had no such gate: every episode that survived the fragment
filter went to the reasoner, including sessions that are mostly "ok" and
"thanks". `CORTEXIQ_TRIAGE_MODEL` adds Hindsight's triage idea — a cheaper model
classifies the cluster first and may refuse it. It is off here, because there is
no cheap model configured and because the honest saving is unmeasured; a triage
answer that cannot be parsed counts as durable, so the worst case is a wasted
call rather than a lost memory.

### What the graph arm actually does (37 real queries, 2026-10-26)

| Set | n | graph off (MRR) | graph on (MRR) | better / worse |
|---|---|---|---|---|
| Topic questions | 19 | 0.496 | 0.423 | 2 / 4 |
| Entity and relation questions | 18 | 0.257 | 0.425 | 9 / 2 |
| All | 37 | 0.380 | 0.424 | 11 / 6 |

Measured twice, before and after the neighbour floor (v0.1.3). The floor did its
job — results reachable only through the graph arm fell from 9 to 1 — and no
rank moved, because the neighbours it removed were already ranked below the fold
at the limits people use. The topical loss has a different cause, and it is not
junk: a fact that shares an entity with the question gets both a vector rank and
a graph rank, so equal-weight fusion counts it twice and a plausible neighbour
outranks a better answer.

So the arm is not wrong, its default was. `CORTEXIQ_GRAPH_RECALL` is `false` in
production since 2026-10-26, and callers that need the arm pass `"graph": true`
per request — which works regardless of the global default, verified against the
same instance: a plain recall now matches graph-off exactly (no graph stage in
the trace at all, MRR 0.496 on topic questions), while the 18 relational
questions still reach MRR 0.425 when the caller asks for the arm.

The floor stays. Not because it moved a rank in this measurement, but because
production runs `CORTEXIQ_MAX_RESULT_SIZE=10000`: at a large limit those
neighbours do surface, and there the floor is the difference between a hub list
and a pile.

Gating the arm on question shape happened anyway, as a weight rather than a gate:
`CORTEXIQ_GRAPH_ROUTE=true` runs the arm on every recall and fuses it at 0.1 for a
topic question and 0.5 for a relational one. On 36 questions written against a copy
of production (2026-11-13) that scored 0.76 MRR against 0.72 with the arm off and
0.65 with equal weight — without a reranker, which is where the arm matters. With a
working Jina reranker every variant, off included, reached 0.93–0.97, so that set
cannot show a difference: the questions were derived from the fact text and are too
easy for a cross-encoder. The weights come from a sweep on 36 labels I wrote myself,
so they are a hypothesis. Production keeps the routing on because it is neutral at
worst with a working reranker and better than equal weight when the reranker fails.

### PageRank walk for the graph arm (built and measured 2026-11-23, off, keep it off)

`CORTEXIQ_GRAPH_PPR=true` swaps the arm's one-hop expansion for a personalized
PageRank over the relationship graph, after HippoRAG (arXiv:2405.14831). The arm
keeps its name and its routing weight; only the facts it proposes change. It loads
the relationships within three hops of the entities the question names, walks them
(damping 0.5, 30 iterations, edges weighted by confidence), keeps the twelve
best-scoring entities and takes at most three facts per entity, closest to the
question first. Constants live in `internal/brain/ppr.go` and are starting values.

What it is for: a fact two or three relationships from the named entity, which the
one-hop arm cannot reach. What it is not: HippoRAG's passage nodes, its LLM
recognition filter or its seed weighting by specificity. Episodes still carry no
entity, so the arm never ranks them.

Measure it with `cortexiq eval graph --queries eval/multihop_queries.jsonl`: the
report gains a "routed + ppr" column next to "routed", and 14 questions (10 at two
hops, 4 at three) that the corpus check proves are that far away. Run the standard
set too, to see whether the walk costs topic questions what the equal-weight arm did.

Measured 2026-11-23 (bge-m3, 26 facts, k=10, one run each; the PPR SQL and its
integration test now pass against PostgreSQL 17 too):

| | without graph | routed (production) | routed + ppr |
|---|---|---|---|
| multihop, 14 questions, hit@3 / MRR | 0.57 / 0.43 | 0.50 / 0.31 | 0.57 / 0.44 |
| standard, 22 questions, hit@3 / MRR | 0.86 / 0.74 | 0.95 / 0.79 | 0.86 / 0.74 |

Reading: on multi-hop the PPR arm is no better than having no graph arm at all
(hop3 unchanged at hit@3 0.75; hop2 hit@1 0.30 against 0.30). Its only gain is
that it does not do the damage the one-hop arm does on these questions (routed
MRR 0.31). On the standard set it gives up what the one-hop arm gains
(direct hit@3 1.00 → 0.67, neighbour g09/g10 back to baseline), so it is a
regression against production routing. The hypothesis that the walk reaches two
and three hops here is not supported. Caveats: 14 questions, a 26-fact corpus that
leaves top-10 almost always containing the answer, questions written by the same
person who wrote the walk. Do not switch it on. If this is pursued, the next
experiment is the walk *added to* the one-hop arm (union) rather than replacing
it, and a sweep of the constants; neither is done.


Fixed alongside it: a recall that asked for facts *and* episodes returned facts
only. Episodes were handed whatever slots the facts left over, and facts fill the
limit. Each type now searches with its own budget and the merged list is sorted
by score, so the better answer wins regardless of type.


## Tests and CI

| | |
|---|---|
| Test functions | 621 across 107 files (2026-11-16) |
| Unit tests | run everywhere, no database needed |
| Integration tests | `internal/brain` and `internal/db`, need `CORTEXIQ_TEST_DSN`; CI runs them against the PostgreSQL 17 image built from `deploy/postgres` (pgvector + pg_textsearch) and fails if any was skipped |
| Race detector | CI job `race`, `go test -race ./...` — green since its first run on 2026-10-26 |
| Coverage | 43.5% overall, gated by `.github/coverage-baseline.txt` (ratchet, cannot drop) |

Coverage is measured with the database attached. Without it `internal/brain`
reports 26% instead of 67%, so a coverage number from a bare `go test ./...` is
not comparable.

Thin spots, last measured 2026-11-08, in order of how much it hurts:
`internal/bootstrap` (12%), `internal/db` (11%), `internal/webui` (15%),
`internal/reasoner` (21%). Those are the wiring layers — a bug there breaks startup
or the dashboard, not recall.

## Removed memory: three verbs

- `forget` — soft delete, no judgement. `episodes.deleted_at` is set.
- `retract` — soft delete plus a row in `retractions` with the claim and the
  reason. For "that was wrong".
- `purge` — hard delete. Also removes facts sourced only by the purged episode,
  the matching `embedding_cache` text, retractions of the target, and writes a
  `purge_log` row without content. `purge namespace` refuses to run with child
  namespaces and needs the slug twice.

Drop the dashboard in and all three are reachable there since `v0.1.1`: `retract`
sits next to `purge` on every memory row (with the same "a reason is required"
rule as the API), and the Lifecycle tab shows retractions and purges in one
timeline. Retraction records follow the namespace selector; purge records are
global, because a purge can remove the namespace itself.

There is no undo for a purge, and there is no soft-delete sweep: tombstones stay
in place.

- A model chosen in the dashboard is stored in `llm_bindings`, and from `v0.1.5`
  the database wins over the environment **at startup**, not only in the process
  that accepted the change. Before that, a binding looked enabled and then did
  nothing in the next consolidation run, because that runs in its own process.
- **Since `v0.1.88` (2026-11-16)**: the same split, seen from the other side.
  A `remember` through the dashboard listener stored the episode and then sat
  until the next timer tick — two hours on the production schedule — because the
  in-process wake-up only reaches the process that wrote. The write now also
  publishes `pg_notify` on `cortexiq_episode` with the namespace id, and the
  consolidating process listens on a dedicated connection (a hijacked pool
  connection, closed on shutdown) and feeds the id into the same drain the local
  signal uses. The timer stays the safety net: a notification published while the
  listener restarts is lost, so the listener re-reads pending work on every
  (re)connect. Proven on `v0.1.88`: a `remember` through port 18101 at 04:24:23
  started run `105661` in the same second (1 episode, 4 model calls, 1 fact, 3
  relationships, no error sample) without a restart.
- **Since `v0.1.88` (2026-11-16)**: the relationship stage does not stop on an
  answer it cannot read. Its checkpoint only moves over a contiguous run of finished
  batches, so one batch the model kept answering unreadably froze every fact behind it
  for that namespace — four days in November 2026, until the parser was taught the
  shapes. A provider that fails a generation and returns the failure as the content
  (`{"error": "…"}`) is now recognised as a failed generation, not a parse error, and
  the request is resent unchanged; a batch that stays unreadable is written off after
  three runs, so the checkpoint moves past it and the fact ids plus the reason land in
  `error_sample` and the log. Transport failures, timeouts and an exhausted budget are
  never written off: they say nothing about the batch. The count is in memory, keyed by
  namespace and leading fact id, so a restart is a fresh attempt.
- Triage was tried on `/test` after `v0.1.5`: two fresh episodes, one durable and
  one chatter, gave `episodes_read: 2`, `episodes_skipped: 1` — the chatter was
  refused before extraction. `llm_calls` went from 5 to 6, because the triage call
  is also a call; the saving is in tokens, not in calls. One skipped episode out of
  two proves the wire is live, not the saving; that needs weeks of real sessions.

## Models and providers

The five models consolidation can use (reasoner, triage, critic, embedding,
reranker) are visible in **Settings → Models**, with the environment value next to
whatever was chosen in the dashboard, so an override never hides the deployment's
own setting.

- Providers live in `llm_providers` (migration 39), alongside the model chosen per
  purpose in `llm_bindings`. A purpose without a row keeps the environment value,
  which is why an empty table changes nothing.
- API keys are **write-only**: they are stored and never returned by the API. The
  dashboard sees `…` and the last four characters, enough to tell which key is
  loaded. An empty key on an update means "keep the stored one", so editing a
  name cannot silently erase a credential.
- **Fetch models** asks the provider itself, so pricing, context length and
  capabilities are theirs and not our guesses: DeepInfra's public catalogue
  (`/models/list`) returned 397 models, 230 of them chat-completions with prices.
  Mistral's `/v1/models` needs the key; a rejected key is reported as such and the
  key is never echoed.
- Reasoner and triage take effect immediately. The **embedding model is
  read-only**: stored vectors carry its dimensions, so switching it is a re-embed
  job, and the API refuses with `409` rather than letting recall degrade quietly.
  The reranker is changeable but needs a restart.
- **Prices come from the catalogues**, not from a variable: `GET {base}/models` of
  the environment provider and of every row in `llm_providers`, refreshed every six
  hours with a five-minute retry after a failed attempt. A model whose catalogue
  publishes no price, or a price that is not in USD, deliberately stays unpriced
  rather than showing `$0.00`. `CORTEXIQ_MODEL_PRICES` was removed in `v0.1.24`; a
  page or a log line that still names it is stale. At `v0.1.86` the table held 958
  models over two providers.
- **A price can be entered by hand** in Settings → Models, for a model no catalogue
  prices (`model_prices`, migration `00063`). It is a fallback, not an override: a
  catalogue price always wins, also when the two spell the model differently in
  case, and the row then shows "catalogue wins" instead of disappearing. The MCP
  process re-reads that table every minute and merges it with the last known
  catalogue prices, so a change applies without a restart even in the split
  deployment; the dashboard publishes it immediately for its own page. A failing
  read keeps the last known values, so a hiccup does not make calls unpriced.
- The Metrics tab reports unpriced calls per `model · stage`, because the counter
  already carries both labels. That is the only place that says *which* model is
  missing a price; it used to print a total and a variable that no longer existed.

## Open decisions

Closed on 2026-10-26 by measurement: the candidate cap stays `0`, and fact dedup
runs at `0.98` with reviews from `0.85` (see the consolidation section above).
What is left:

1. Graph routing: built and on in production, with 0.1/0.5 as the weights. Whether
   those are the right numbers needs a question set that is hard for the reranker
   and labels from questions you actually could not answer, not more of mine.
2. RRF arm weights: with routing on there is something to tune, but the measurement
   above saturates with a working reranker. Tuning before there is a harder set
   would be work with no measurable effect.
3. Cross-encoder reranker: a local one was measured on a copy of production — 0.80
   MRR against 0.85 for Jina, at roughly 2.9 s extra per recall against 0.1 s — and
   rejected on latency. Jina stays, with the metric, the log line,
   `RERANK_CANDIDATES=30` and the routing as the safety net. A fallback chain
   Jina → local is not built and is not needed unless Jina actually starts failing.
4. The reasoner binding: the reason every run failed was the 1024-token output budget,
   not the model — a reasoning model spends that budget on its thoughts and returned
   an empty answer. `v0.1.86` raises the default to 8192, lets a bound model follow
   the configured budget and JSON mode instead of a forced one, and unwraps the
   `{"facts": [...]}` shape JSON mode returns; one live run then produced 6 calls,
   5 facts, 3 relationships and no error. The binding stays. What is still open is
   how much of the `relationships` provider error (returned as content, on the same
   fact cluster each time) survives the bigger budget — watch that stage specifically.
   Since `v0.1.88` that provider error is no longer a parse failure: it is retried
   unchanged and, if it keeps failing, the batch is written off after three runs so
   the checkpoint can move on; see the note on the relationship stage above.

## Known rough edges

- No semantic versioning history before `v0.1.0`, so nothing can depend on
  "the API as of v0.x".
- A row without an embedding stops the whole namespace run: scanning it into a
  non-pointer vector fails with `unsupported data type: <nil>`, logged per query
  rather than per row. Production writes always embed, so this reaches
  hand-seeded or imported rows, not normal traffic.
- Jina rates 100,000 tokens per minute. Above that a recall gets `429` and falls
  back to the fusion order, which is now visible in
  `recall_rerank_requests_total{status="error"}` and one log line per minute.
  `CORTEXIQ_RERANK_CANDIDATES=30` keeps an ordinary recall well under the limit; a
  large `limit` still is not.
- The app image is distroless: no shell, no `cortexiq` on `PATH`, so every container
  check has to call `/cortexiq` by absolute path. `docker exec … cortexiq version`
  fails even though the binary is there.
- Windows: `go test ./...` can fail with `unlinkat ... test.exe` when a test
  binary is still running. Retry; it is not a test failure. On this machine AppLocker
  also refuses to execute some `go test` binaries written to `%TEMP%`; building the
  binary into the repository directory and running it there works around that.
- The integration suite wants a clean database. `TestGlobalMentalModelReadsAllNamespaces`
  builds a global model from a cross-namespace recall, so facts left behind by
  earlier runs can push the expected one out of the top N and fail the test.
  Use a fresh database for a full run, not the one used for manual poking.
- `docs/handoff-2026-10-18.md` still lists tags, Reflect and mental models as
  "not built". They shipped on 2026-10-23; the handoff stays as the record of
  that day.
- 2,365 live relationships (27% of 8,888, measured 2026-11-17) have a `source_fact_id`
  that points at a soft-deleted fact. All come from one manual `dedupe` on
  2026-10-13 that merged 73 facts: `mergeFacts` moved sources but not relationships.
  Fixed for future merges (unreleased). The old rows are not repaired: 248 of them
  have a live twin, 2,117 are the only carrier of their claim, and the merge did not
  record which fact survived, so a repoint would be a guess. Nothing filters on
  `source_fact_id`, so recall and the graph are unaffected; only "where does this
  come from" ends at a deleted fact. A further 226 relations come from 39 facts
  that were expired by a detected contradiction (none by disuse). Measured 2026-11-17:
  146 of them (fact 55 alone: 123) hang on "contradictions" that are rewordings of
  the same claim, so the relation is still true; only 30 name the replaced value
  (version bumps such as v0.1.12 to v0.1.17). Expiring a relation with its source
  would hide about 196 true relations, so it is not done. The cause is upstream:
  `DetectContradictions` compares entity and property and sees any different wording
  of the value as a contradiction (54 contradictions on record, none with an equal
  value, several that are paraphrases). That includes `Bo / tooling preferences`
  (fact 32), a core preference that expired on 2026-10-08 although fact 41 restates
  it. Cosine between old and new fact over all 54 pairs: 38 sit below 0.80 and are
  real changes (versions, commit ids, status); the clear rewordings (facts 32, 55, 76,
  106, 1924) score 0.894-0.938 and carry 190 of the 264 relations on these old facts;
  the closest real change scores 0.857 (`v0.1.17` to `v0.1.20 (e56a03b)`). A cut near
  0.89 separates them on this data, but 0.883 (two different confirmation records) and
  0.888 (a narrower restatement) sit on the line, so the cosine alone is too thin to
  decide; the same-fact LLM review that the dedup already uses is the safer judge.

- 2026-11-17: Settings heeft een Run now-knop (`POST /api/v1/consolidation/run`, Postgres-notify `cortexiq_run_now`) die een volledige ronde over alle namespaces start. Gebouwd en unit-getest, nog niet end-to-end of in de browser geprobeerd, niet uitgerold.

- 2026-11-18: `internal/reasoner/factfidelity_test.go` (`-tags modeleval`, `TestFactFidelity`) vergelijkt de productie-extractie met een variant zonder de regel "rewrite in your own words" en zonder de 80-procent-grounding-poort, naast de bron-episodes. Gebouwd en gecompileerd (`go vet`), nog niet gedraaid: er zijn geen invoerbestand en geen API-sleutel. Aanleiding: fact 3204 draait het onderwerp om en de poort verwerpt juiste, zelfstandige zinnen uit het episode.

- 2026-11-18: eerste meting met `TestFactFidelity` (21 episodes, een run per model): cijfertokens behouden van 171, productie / variant zonder regel en poort: DeepSeek-V4-Flash 139 / 142, gpt-oss-120b 117 / 126, Claude Opus 5.5 156 / 157. Het model weegt zwaarder dan de regel of de 80-procent-poort. Besluit over de reasoner en de poort staat open (doel 1 in CortexIQ); zie `walkthrough3.md`.

- 2026-11-18: de extractieregel "rewrite in your own words" is vervangen door "behoud de woorden waar ze juist en zelfstandig zijn" en de grounding-poort in `validateFactGrounding` staat op 0,95 woordoverlap (`groundingMaxOverlap`, was 0,80). Nog niet uitgerold en nog niet hergemeten op productie; de hermeting op 21 episodes loopt. Zie `walkthrough3.md`.

- 2026-11-18: hermeting van `TestFactFidelity` met promptregel en poort 0,95, cijfertokens behouden van 171 (eerste meting tussen haakjes): Opus 5.5 159 (156), DeepSeek-V4-Flash 141 (139), gpt-oss-120b 129 (117), Gemini 2.5 Flash 124 (niet eerder gemeten). Rapporten in `eval/fidelity-2026-11-18/hermeting-poort-095/`. De Hopscotch-route `deepinfra/…` gaf 503 en 402; controleer de reasoner-, triage- en critic-binding als die daarop staat.

- 2026-11-18: de modeldialoog in Settings > Models heeft een doorzoekbare modelkiezer (prijs per miljoen tokens in en uit per rij, sorteren op prijs of naam, beschrijving die uitklapt), in de stijl van NightCompanion. Gebouwd en in de browser geprobeerd op een testpagina met nagebootste data; nog niet tegen de echte API, niet gecommit en niet uitgerold.

- 2026-11-19: triage heeft een eigen uitvoerlimiet `CORTEXIQ_TRIAGE_MAX_TOKENS` (standaard 200, ook voor een dashboard-binding); `CORTEXIQ_REASONER_MAX_TOKENS` gold nooit voor triage. De "used all N output tokens"-fout noemt nu de limiet die voor die client geldt. Nog niet uitgerold: de compose op de server moet de variabele eerst doorgeven, en bij een reasoning-model als triage (gpt-5-nano) zet je hem op ~1024 of bind je een model zonder reasoning.

- 2026-11-19: `v0.1.92` uitgerold (de970af). Op de server staat `CORTEXIQ_TRIAGE_MAX_TOKENS=1024` in `.env` en de compose geeft de variabele door; back-ups `.env.bak-pre0.1.92` en `docker-compose.prod.yml.bak-pre0.1.92`, dump `backups/cortexiq-20261119T091802Z.dump`. Nog te zien: of de triage-fout bij `gpt-5-nano` uitblijft in de Runs-tab.

- 2026-11-19: het Clean-up-scherm is niet meer alleen-lezen: elke rij bij Weak relations heeft een Remove-knop (`POST /api/v1/relationships/{id}/dismiss`, `Brain.DismissRelationship`). Het is een zachte verwijdering (`deleted_at`), het bronfeit blijft staan en terugzetten kan met `UPDATE relationships SET deleted_at = NULL WHERE id = ...`. Alleen-lezen-tokens krijgen 403. Nog niet uitgerold. Samenvoegen van spellingvarianten bestaat niet; dat blijft de CLI-opdracht voor entiteiten.

- 2026-11-19: het Clean-up-scherm kan spellingvarianten samenvoegen: bij elke variant staat een Keep-knop die de andere spellingen van die groep naar deze hernoemt (`POST /api/v1/entities/merge`, `Brain.MergeEntities`). Eerst een dry run voor de bevestiging (aantallen feiten, relaties, duplicaten, zelfverwijzers), dan de echte run in één transactie. Herschreven worden `facts.entity` en beide kanten van `relationships`; identieke relaties vallen samen (zacht verwijderd, de zekerste blijft), relaties van een entiteit naar zichzelf verdwijnen. Feitteksten, embeddings, `contradictions` en `patterns.source_entities` blijven ongemoeid. Niet terug te draaien vanuit het scherm. Nog niet uitgerold.

- 2026-11-19: `v0.1.93` uitgerold (e2d5bf8) met de Remove- en Keep-knoppen op het Clean-up-scherm. Geen nieuwe migratie of omgevingsvariabele. Back-ups: `.env.bak-pre0.1.93`, dump `backups/cortexiq-20261119T102933Z.dump`. De knoppen zijn nog niet in de echte UI geprobeerd.

- 2026-11-19: op het Clean-up-scherm opent een klik op een term (spellingvariant of eiland-entiteit) de contextkolom met de feiten, relaties en bronepisodes van precies die spelling (`GET /api/v1/entities/context`, `Brain.EntityContext`, tot 50 per lijst met het echte totaal). Een feit of episode erin opent de gewone detailweergave. Nog niet uitgerold.

- 2026-11-19: `v0.1.94` uitgerold (a1d995c) met de context van een term op het Clean-up-scherm. Geen migratie of nieuwe omgevingsvariabele. Back-ups: `.env.bak-pre0.1.94`, geverifieerde dump `backups/cortexiq-20261119T104711Z.dump` (genomen direct na de uitrol; er is geen migratie, dus de database was identiek). De eerste dump van deze uitrol is verwijderd omdat mijn eigen `head -1` het script afbrak voor de verificatie.

- 2026-11-20: `v0.1.95` uitgerold (427a952) met de Bulk-knop op het Clean-up-scherm (per groep blijft de spelling met de meeste feiten; één dry-run-totaal, één bevestiging). Geen migratie of nieuwe omgevingsvariabele. Server `strato-vps`, `/opt/cortexiq`. Back-ups: `.env.bak-pre0.1.95`, geverifieerde dump `backups/cortexiq-20261120T182145Z.dump` (64 MB, checksum OK, genomen met de app-containers gestopt). De knop is niet in de draaiende UI geprobeerd (de UI vraagt inloggen).
- 2026-11-23: de consolidatie leest de modelbindings nu vóór elke run opnieuw uit de database (`Context.ReloadModelBindings`, alleen toegepast als reasoner/triage/critic, provider, base-URL of key veranderd is). Aanleiding: een binding die in het dashboard werd gewisseld bereikte het `cortexiq`-proces (mcp, met consolidatie) pas na een herstart, waardoor Hopscotch nog werd aangeroepen en 402 gaf. Nog niet uitgerold; productie is op `v0.1.98` en het `cortexiq`-proces is op 2026-11-23 07:19 UTC handmatig herstart. Een in het dashboard gewist binding blijft tot de volgende herstart op het huidige model staan.
