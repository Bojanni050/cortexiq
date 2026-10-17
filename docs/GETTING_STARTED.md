<!-- Modifications copyright (c) 2026 Bojan van der Heide -->
# Getting Started with CortexIQ

You ran `docker compose up`. Here's what to do next.

For production, do not run the default compose file on the VPS. Copy
`docker-compose.prod.example.yml` to `docker-compose.prod.yml`, keep secrets in
`.env`, and always deploy explicitly with:

```bash
docker compose -f docker-compose.prod.yml up -d --build
```

The explicit file is important: the default compose file is the local-development
variant and uses different ports and database defaults.

For cost protection, set `CORTEXIQ_MAX_LLM_CALLS_PER_RUN=50` in the production `.env`.
It limits reasoner calls for one namespace consolidation; `0` means unlimited.
Prometheus exposes `reasoner_requests_total`, `reasoner_prompt_tokens_total`,
and `reasoner_completion_tokens_total`, each labelled by pipeline stage.

## 1. Verify CortexIQ is running

```bash
docker compose ps
docker compose logs cortexiq | tail -20
```

You should see the MCP server listening on port **8080**. The SSE endpoint is:

```
http://localhost:8080/sse
```

Quick check (expects HTTP 200 or SSE handshake):

```bash
curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8080/sse
```

## 1b. Open the dashboard (optional)

The HTTP server on port **9090** also serves a web dashboard:

```
http://localhost:9090/ui
```

From there you can browse namespaces, run semantic recall, inspect facts, goals, hypotheses, contradictions, failures, and causal links — and manage them directly.

### 1c. Or call the HTTP API directly (optional)

Headless integrations use the same listener as the dashboard (port **9090**, not the MCP port), with `Authorization: Bearer <token>` when a token is set:

```bash
curl -X POST http://localhost:9090/api/v1/remember \
  -H "Authorization: Bearer $CORTEXIQ_API_TOKEN" -H "Content-Type: application/json" \
  -d '{"content":"Bo prefers Go for backend work","namespace":"/projects/gaia"}'

curl -X POST http://localhost:9090/api/v1/recall \
  -H "Authorization: Bearer $CORTEXIQ_API_TOKEN" -H "Content-Type: application/json" \
  -d '{"query":"backend language preference","namespaces":["/projects"]}'
```

`namespace` may be replaced by a `namespaces` array (first entry wins); `importance` (0..1, default 0.5) and `occurred_at` (RFC3339) are optional. Both endpoints answer the same shape as the matching MCP tools.

Each recall result names the table its id belongs to (`fact_id` for a fact, `episode_id` for an episode, plus `type`). Use the typed field when you act on a result — `id` alone is ambiguous, because facts and episodes number their rows independently and one id exists in both tables. A missing row answers `404`, a bad request `400`, so a client can tell its own mistake from a server fault.

## 2. Connect your MCP client

Point any MCP-over-SSE client at `http://localhost:8080/sse`.

**Cursor** — create or edit `~/.cursor/mcp.json`:

```json
{
  "mcpServers": {
    "cortexiq": {
      "url": "http://localhost:8080/sse"
    }
  }
}
```

Restart Cursor (or reload MCP). CortexIQ should appear in your MCP tool list.

See [README MCP Client Setup](../README.md#mcp-client-setup) for Claude Desktop, Windsurf, and OpenCode configs.

## 3. First session workflow

Once connected, ask your agent to use CortexIQ tools in this order:

| Step | MCP tool | What it does |
|------|----------|--------------|
| 1 | `init` | Creates `/self` namespace scaffold (capabilities, limits, preferences) |
| 2 | `remember` | Store an episode or fact ("User prefers Python", "Project uses Postgres") |
| 3 | `recall` | Semantic search over what you stored |
| 4 | `consolidate` | Run the pipeline that turns episodes into structured facts |

Example prompts to your agent:

- *"Call CortexIQ `init` to set up memory namespaces."*
- *"Use CortexIQ `remember` to store: this project uses CortexIQ on port 8080 with pgvector."*
- *"Use CortexIQ `recall` to find what you know about this project's stack."*
- *"Run CortexIQ `consolidate` to process recent episodes into facts."*

## 4. Background consolidation

When started with `mcp serve --with-consolidation` (the Docker Compose default), CortexIQ consolidates in the background. You can also trigger manually via the `consolidate` MCP tool or CLI:

```bash
docker compose exec cortexiq /cortexiq consolidate run
```

## 5. Configuration checklist

If tools fail, check `.env`:

| Variable | Purpose |
|----------|---------|
| `CORTEXIQ_OPENAI_API_KEY` | Embeddings + reasoner (OpenAI, OpenRouter, or compatible) |
| `CORTEXIQ_OPENAI_BASE_URL` | API base URL |
| `CORTEXIQ_EMBEDDING_MODEL` | Must match `CORTEXIQ_VECTOR_DIM` (1536 for `text-embedding-3-small`) |
| `CORTEXIQ_REASONER_MODEL` | Model used during consolidation |

**Running fully local?** See [LOCAL_OLLAMA.md](LOCAL_OLLAMA.md) — Ollama on the host, no cloud API key.

**Using Atlas Cloud?** CortexIQ already supports it through the OpenAI-compatible API:

```bash
CORTEXIQ_OPENAI_API_KEY=your-atlas-cloud-api-key
CORTEXIQ_OPENAI_BASE_URL=https://api.atlascloud.ai/v1
CORTEXIQ_EMBEDDING_MODEL=text-embedding-3-small
CORTEXIQ_REASONER_MODEL=deepseek-ai/DeepSeek-V3-0324
CORTEXIQ_VECTOR_DIM=1536
```

Atlas Cloud docs: [https://www.atlascloud.ai/docs](https://www.atlascloud.ai/docs)

## Troubleshooting

**MCP client can't connect**

- Confirm `docker compose up` finished and port 8080 is not in use elsewhere.
- Use `http://localhost:8080/sse` (not `https`) for local Docker.

**Empty recall results**

- Run `init` first, then `remember`, then `recall`.
- Embeddings require a valid API key in `.env`.

**Consolidation does nothing**

- Needs episodes via `remember` first.
- Check logs: `docker compose logs cortexiq`.

**Provider returns auth or model errors**

- Confirm `CORTEXIQ_OPENAI_BASE_URL` points to the correct provider endpoint.
- Confirm the embedding and reasoner model names match what your provider exposes.
- Atlas Cloud users should use `https://api.atlascloud.ai/v1` and a valid API key.

## Next steps

- Explore namespaces: `list_namespaces`, `create_namespace`, `rename_namespace`
- Track goals and failures: `create_goal`, `create_failure`
- Full tool list: see the [docs site MCP section](https://bojanni050.github.io/cortexiq/#mcp)
