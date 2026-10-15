# Third-party notices

CortexIQ is a derivative work of **Stash** and uses several third-party components
at build time and at runtime. This file records them and their licenses. It
complements [`LICENSE`](LICENSE), which holds the full Apache License 2.0 text that
covers this project and the upstream code it is based on.

## Upstream project: Stash

| | |
| --- | --- |
| Project | Stash — https://github.com/alash3al/stash |
| Copyright | Copyright 2024 Mohamed Al Ashaal |
| License | Apache License 2.0 — full text in [`LICENSE`](LICENSE) |
| Relationship | Copied and modified |

CortexIQ started as a copy of the Stash repository, and the upstream code is still
the larger part of this codebase. The fork renames the project (CortexIQ), the Go
module, the binary, the environment prefix, the MCP server name and the Docker
image, and adds features on top; apart from that the upstream code is unchanged.

Upstream ships **no `NOTICE` file**, and its sources carry **no per-file copyright
headers** — so no notice was removed or altered. Instead, every file taken from
upstream that this fork has changed carries a prominent modification notice
(Apache License 2.0, section 4(b)):

```
Modifications copyright (c) 2026 Bojan van der Heide
```

The files carrying it are exactly the upstream files that this fork changed; list
them with (the command also matches this file, which quotes the notice):

```sh
grep -rl 'Modifications copyright' --exclude-dir=.git . | grep -v LICENSE-THIRD-PARTY.md
```

The Apache-2.0 license text in `LICENSE` is kept verbatim, including its copyright
line, and must not be changed.

## Build dependencies

Fetched by the Go toolchain; **no third-party source code is vendored in this
repository**. Licenses as stated by each project at the time of writing:

| Module | License |
| --- | --- |
| github.com/alash3al/sqltmpl | MIT (Copyright (c) 2024 Mohammed Al Ashaal) |
| github.com/caarlos0/env/v11 | MIT |
| github.com/jackc/pgx/v5 | MIT |
| github.com/jedib0t/go-pretty/v6 | MIT |
| github.com/joho/godotenv | MIT |
| github.com/mark3labs/mcp-go | MIT |
| github.com/openai/openai-go | Apache License 2.0 |
| github.com/pgvector/pgvector-go | MIT |
| github.com/pressly/goose/v3 | MIT |
| github.com/prometheus/client_golang | Apache License 2.0 |
| github.com/urfave/cli/v3 | MIT |

Indirect dependencies are pulled in transitively — see `go.mod` and `go.sum` — and
each ships its own license.

## Runtime images

| Image | License |
| --- | --- |
| pgvector/pgvector:pg16 | PostgreSQL License (PostgreSQL and pgvector) |
| ollama/ollama | MIT |

## Own files

Everything this fork added — the web UI under `internal/webui/`, the `scope` and
`auth` packages, the deduplication, export, runs, stats and LLM-budget code, the
migrations from `00021` onward, the deploy units and scripts under `deploy/` and
`scripts/`, and the documentation this fork added — is original work of Bojan van
der Heide, distributed under the same Apache License 2.0 as the rest of the
repository. Those files carry no modification notice, because there is no upstream
file to modify.
