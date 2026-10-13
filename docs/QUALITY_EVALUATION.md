# Stash quality evaluation

Use this procedure before changing the reasoner model, prompts, consolidation
thresholds, or relationship batch size. It keeps model quality separate from
real user memory and produces a comparable result.

## 1. Use an isolated namespace

Create a temporary namespace such as `/eval/2026-10-13-qwen3` and use only
synthetic examples. Do not copy customer, user, or project memories into the
evaluation set.

Each example should contain one or more explicit claims, including cases with:

- names, versions, paths, ports, and numbers;
- two independent claims in one episode;
- a translation or rewording of an existing fact;
- compatible values versus a genuine contradiction;
- an explicit relationship and a statement with no relationship;
- an unsupported or ambiguous claim that should produce no fact.

Store the examples with `remember`, run one `consolidate` after the batch, then
inspect `query_facts`, `query_relationships`, and `list_contradictions`.

## 2. Score every run

Record the model, settings, date, reasoner calls, prompt tokens, completion
tokens, retries, and these scores:

| Dimension | Question | Target |
|---|---|---:|
| Fact recall | Were all explicit durable claims extracted? | >= 95% |
| Specificity | Were names, numbers, versions and paths preserved? | 100% |
| Grounding | Were unsupported claims invented? | 0 |
| Separation | Were independent claims split into separate facts? | >= 90% |
| Deduplication | Were rewordings merged without losing new details? | >= 90% |
| Relationships | Are extracted edges explicit and correctly attributed? | >= 90% |

The run fails if any unsupported claim is accepted, a critical number/name is
changed, or the reasoner exceeds the configured call budget.

## 3. Compare models safely

Run the same examples with the old and new configuration. Keep the raw JSONL
export and the score sheet for each run. Compare quality and completion tokens;
the cheapest model is not better if it drops identifiers or causes retries.

The relevant Prometheus metrics are:

```text
reasoner_requests_total{stage,status}
reasoner_prompt_tokens_total{stage}
reasoner_completion_tokens_total{stage}
```

Consolidation results also expose `llm_calls` and `llm_calls_limited`.

## 4. Clean up

After scoring, remove the isolated namespace and its exported evaluation file.
Never leave synthetic evaluation data mixed into production namespaces.
