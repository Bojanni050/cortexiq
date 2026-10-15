# Extractor findings: fragments and frozen config values

Observed 2026-10-16 while wiring an external memory client (Hermes Agent) to
CortexIQ over `POST /api/v1/remember`, in a namespace used only for that work
(`/hermes`, namespace_id 93).

Eight short episodes were written; the extractor consolidated them into fourteen
facts. Two classes of problem showed up. Both are trivially reproducible.

## 1. Fragments produce confident, wrong facts

These episodes are meta-talk from the session — a user typing "2", "zo",
"gedaan". Their only content is that a turn happened:

```
t=22:42:17  len=21   "zo moet het goed zijn"
t=22:44:08  len=22   "ok ay we gaan het zien"
t=22:46:13  len= 1   "2"
```

The extractor did not treat them as noise. It inferred a feature:

```
fact_id 490
"CortexIQ includes the content '2' and 'gedaan' in early responses as a
 signal or reminder, indicating a deliberate design choice for user feedback."
```

Nothing in the input supports that. The "2" was a user picking option 2 from a
numbered list. The failure mode is therefore not "empty fact" but
"self-assured fabrication", which is worse: at recall time it is
indistinguishable from real knowledge, and this one was in fact injected into
the client's context as authoritative memory before it was removed.

Suggested direction: an explicit minimum before extraction. An episode with no
sentence in it, or under roughly 80 characters, cannot yield a fact that is
worth its risk. Either skip those episodes, or instruct the extractor that
filler turns are not facts.

## 2. Runtime configuration is frozen into facts

Two further facts recorded a default that changed within the hour:

```
fact_id 487
"CortexIQ limits assistant responses to 612 characters, which is 600
 characters from the configuration CORTEXIQ_ASSISTANT_MAX_CHARS plus an
 additional …"                          [capture was truncated at 150 chars]

fact_id 488
"CortexIQ marks responses as AFGEKAPT when they exceed the 612-character
 limit, as observed in timestamps 22:47:42 and 22:50:16."
```

612 was true when written: 600 was the client's own cap plus its truncation
marker. It became false the moment that default became 0 (no cap). The facts
outlived their truth by minutes, and nothing notices.

Config-derived facts age faster than any other kind, because configuration is
what changes most often and with the least ceremony. Options: exclude them from
extraction, tag them as config-derived so a config change can invalidate them,
or give facts a validity window.

## Reproduction

Both classes are observable through the plain REST API, with no MCP client
involved:

```bash
curl -X POST .../api/v1/namespaces -H "Authorization: Bearer $TOKEN" \
     -H 'Content-Type: application/json' -d '{"slug":"/probe","name":"probe"}'

curl -X POST .../api/v1/remember -H "Authorization: Bearer $TOKEN" \
     -H 'Content-Type: application/json' \
     -d '{"content":"2","namespace":"/probe"}'

# wait for consolidation, then
curl ".../api/v1/facts?namespaces=/probe" -H "Authorization: Bearer $TOKEN"
```

Of the fourteen facts produced, eight were removed — empty, ephemeral, or
untrue after the config change. Six were kept because they recorded real
findings about the system under test.

A note on cleaning up: the typed id fields (`fact_id` / `episode_id`) are what
make this safe. Facts and episodes number their rows independently, so a bare
`id` read off a recall row cannot tell you which table it belongs to.
