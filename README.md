# Stash

**Your AI has amnesia. We fixed it.**

Every LLM starts every conversation from zero. Stash gives your agent persistent memory — it remembers, recalls, consolidates, and learns across sessions. No more explaining yourself from scratch.

Open source. Self-hosted. Works with any MCP-compatible agent.

---

> **Don't want to self-host?**
> **[usestash.io](https://usestash.io)** is the hosted cloud version — sign in with Google, copy one MCP URL, and you're done. Free to start.

---

## Quick Start

```bash
git clone https://github.com/alash3al/stash.git
cd stash
cp .env.example .env   # edit with your API key + model
docker compose up
```

That's it. Postgres + pgvector, migrations, MCP server with background consolidation — all in one command.

**Next:** [Getting Started guide](docs/GETTING_STARTED.md) — connect your MCP client, run `init` / `remember` / `recall`, and verify everything works.

**Fully local (no cloud API):** [Ollama setup guide](docs/LOCAL_OLLAMA.md) — host Ollama + Docker Compose, private embeddings and reasoner.

## Atlas Cloud

> [Atlas Cloud](https://www.atlascloud.ai/?utm_source=github&utm_medium=link&utm_campaign=stash) is a full-modal AI inference platform that gives developers a single AI API to access video generation, image generation, and LLM APIs. Instead of managing multiple vendor integrations, you connect once and get unified access to 300+ curated models across all modalities.
>
> Check out Atlas Cloud's coding plan promotion: [https://www.atlascloud.ai/console/coding-plan](https://www.atlascloud.ai/console/coding-plan)

Stash already supports Atlas Cloud through its OpenAI-compatible API. Set your `.env` like this:

```bash
STASH_OPENAI_API_KEY=your-atlas-cloud-api-key
STASH_OPENAI_BASE_URL=https://api.atlascloud.ai/v1
STASH_EMBEDDING_MODEL=text-embedding-3-small
STASH_REASONER_MODEL=deepseek-ai/DeepSeek-V3-0324
STASH_VECTOR_DIM=1536
```

Atlas Cloud works well here because Stash only needs:

- an embeddings model for vectorization
- a chat-capable reasoning model for consolidation
- an OpenAI-compatible base URL and API key

See [Getting Started](docs/GETTING_STARTED.md) for a fuller configuration checklist.

## MCP Client Setup

After `docker compose up`, Stash exposes an MCP server over SSE at:

```
http://localhost:8080/sse
```

A web dashboard is also served at `http://localhost:9090/ui` — browse and manage memories, goals, hypotheses, contradictions, and failures from your browser.

To protect Stash, set `STASH_API_TOKEN` in `.env`. It guards both the dashboard API (`/api/v1`) and the MCP server (`/sse`, `/message`): every request then needs `Authorization: Bearer <token>`. The dashboard asks for the token on first use and remembers it in the browser; MCP clients need to send the header (see below). Leave it empty to keep everything open. `/healthz`, `/readyz` and `/metrics` are not covered.

Point any MCP-compatible client at that URL. Example configs:

With `STASH_API_TOKEN` set, add the header to the client config, e.g. for Cursor:
`"headers": {"Authorization": "Bearer <token>"}` next to `"url"`. For Claude Code:
`claude mcp add --transport sse stash http://localhost:8080/sse --header "Authorization: Bearer <token>"`.

**Cursor** — `~/.cursor/mcp.json`
```json
{
  "mcpServers": {
    "stash": {
      "url": "http://localhost:8080/sse"
    }
  }
}
```

**Claude Desktop** — `claude_desktop_config.json`
```json
{
  "mcpServers": {
    "stash": {
      "url": "http://localhost:8080/sse"
    }
  }
}
```

**OpenCode** — `~/.config/opencode/config.json`
```json
{
  "mcp": {
    "stash": {
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
    "stash": {
      "url": "http://localhost:8080/sse"
    }
  }
}
```

Works with any agent that supports MCP over SSE — Claude Desktop, Cursor, Windsurf, Cline, Continue, OpenAI Agents, Ollama, OpenRouter, Atlas Cloud-backed setups, and more.

## What It Does

Stash is a cognitive layer between your AI agent and the world. Episodes become facts. Facts become relationships. Relationships become patterns. Patterns become wisdom.

A 9-stage consolidation pipeline turns raw observations into structured knowledge — facts, relationships, causal links, patterns, contradictions, goal tracking, failure patterns, and hypothesis verification. Each stage only processes new data since the last run.

## Stash Cloud (Beta — Free)

A hosted, multi-tenant version of Stash is available at **[usestash.io](https://usestash.io/)** and is currently free while in beta.

The cloud version is written from scratch — it shares no code with this repository. It is designed from the ground up for scalability, multi-tenancy, and long-term sustainability as a product. Feature sets differ in both directions: some things available here aren't in the cloud, and vice versa.

## Learn More

**[alash3al.github.io/stash →](https://alash3al.github.io/stash/)**

## License

Apache 2.0
