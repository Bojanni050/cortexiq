<!-- Modifications copyright (c) 2026 Bojan van der Heide -->
# Local setup with Ollama (no cloud API)

CortexIQ's landing page promises fully local, fully private memory. This guide closes the gap between marketing and `.env` — **Ollama on your machine, CortexIQ in Docker, zero outbound API keys.**

Contributed by [Sean Campbell](https://github.com/rudi193-cmd) · tested on Linux with Docker Compose.

---

## What you need

- [Ollama](https://ollama.com) running on the **host** (not inside the CortexIQ container)
- Docker Compose (CortexIQ's default install path)
- ~2 GB disk for models + Postgres volume

Pull models **before** `docker compose up`:

```bash
ollama pull nomic-embed-text
ollama pull qwen2.5:3b
```

| Role | Model | Why |
|------|-------|-----|
| Embeddings | `nomic-embed-text` | 768-dim vectors; matches `CORTEXIQ_VECTOR_DIM=768` |
| Reasoner | `qwen2.5:3b` (or `llama3.2:3b`) | Consolidation synthesis; small models work for dev |

---

## Configure `.env`

Copy `.env.example` → `.env` and use the **Ollama block** below.

CortexIQ runs in Docker; Ollama runs on the host. From inside the container, reach the host API at:

- **Linux:** `http://host.docker.internal:11434/v1` (Docker 20.10+) or your bridge IP, e.g. `http://172.17.0.1:11434/v1`
- **macOS / Windows Docker Desktop:** `http://host.docker.internal:11434/v1`

```bash
# --- Ollama (fully local) ---
CORTEXIQ_OPENAI_API_KEY=ollama          # required field; Ollama ignores the value
CORTEXIQ_OPENAI_BASE_URL=http://host.docker.internal:11434/v1
CORTEXIQ_EMBEDDING_MODEL=nomic-embed-text
CORTEXIQ_REASONER_MODEL=qwen2.5:3b
CORTEXIQ_VECTOR_DIM=768                 # ⚠ set once before first run — cannot change later
```

> **Warning:** `CORTEXIQ_VECTOR_DIM` locks at first init. If you already started CortexIQ with `1536`, reset the Postgres volume or use a fresh database before switching to Ollama embeddings.

---

## Launch

```bash
docker compose up --build
```

Verify:

```bash
curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8080/sse
docker compose logs cortexiq | tail -20
```

Then follow [Getting Started](GETTING_STARTED.md): connect MCP → `init` → `remember` → `recall` → `consolidate`.

---

## Troubleshooting

**`connection refused` to Ollama**

- Confirm Ollama is listening: `curl http://localhost:11434/api/tags`
- On Linux, if `host.docker.internal` fails, add to `docker-compose.yml` under `cortexiq`:

```yaml
extra_hosts:
  - "host.docker.internal:host-gateway"
```

**Empty recall / embedding errors**

- Model names must match `ollama list` exactly
- `CORTEXIQ_VECTOR_DIM` must match the embedding model (768 for `nomic-embed-text`)

**Consolidation slow or noisy**

- Smaller reasoners (`qwen2.5:3b`, `llama3.2:3b`) are fine for dev; use a larger model only if synthesis quality matters

---

## OpenRouter vs Ollama

| | Ollama (this guide) | OpenRouter / cloud |
|---|---------------------|-------------------|
| API key | Not required | Required |
| Privacy | Stays on your LAN | Leaves your network |
| Setup | Host Ollama + Docker bridge | `.env` API key only |
| Best for | Local dev, air-gapped, Willow-style fleets | Quick eval, largest models |

Both paths use the same MCP tools — CortexIQ is model-agnostic by design.

---

*Questions or fixes:* open an issue on [Bojanni050/cortexiq](https://github.com/Bojanni050/cortexiq/issues).
