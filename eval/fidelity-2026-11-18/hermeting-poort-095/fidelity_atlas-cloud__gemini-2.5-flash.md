# Fact fidelity: atlas-cloud/gemini-2.5-flash

A = production prompt and gate. B = same prompt, no gate. Read the facts against the episode: subject and object right? detail kept? anything added?

## Episode 989

> Spike 16-11-2026: CortexIQ 0.1.88 (strato-vps, REST 127.0.0.1:18101) vergeleken met Hindsight 0.9.2 (contabo, 8888) als vervanger van Hindsight in Gaia. Beslissend verschil: CortexIQ filtert op occurred_at met from/to (half-open) en herkent Nederlandse datumzinnen over REST, terwijl Hindsight temporal_window alleen rankt (een venster van 1 dag gaf 54 hits over vijf dagen) en memories/list geen datumfilter heeft. Scoping moet met namespaces (array in REST); de enkelvoudige namespace is alleen een ranking-zetje. remember bewaart content, namespace, tags, occurred_at, external_key, external_refs en importance, maar gooit vrije metadata weg; external_key maakt de write idempotent (unchanged:true, andere inhoud geweigerd tenzij supersedes), dus gaia:kairos:<episode_id> is de spiegel-sleutel. DELETE /api/v1/episodes/:id purge hard en cascadeert naar afgeleide facts; facts en namespaces zijn niet los te verwijderen via REST (404/405). Consolidatie reageert binnen twee minuten maar levert even dunne facts als Hindsight. Op twintig Nederlandse vragen: CortexIQ mediaan 1.061 ms met ruwe episodes bovenaan, Hindsight 1.372 ms met een door emotionele gaia-herinneringen verzadigde top. Rapport: docs/spike-cortexiq.md (commits 85dd48e en f329e24). Volgorde blijft: MemoryStore-poort, terugkijk-route op CortexIQ, gaia-logos rebuilden, bojan/gaia importeren, Hindsight uit.

**A production** (numbers kept 8/13, invented [1-day], gate would reject 0)

- CortexIQ version 0.1.88 was deployed on strato-vps, exposing a REST API at 127.0.0.1:18101.
- CortexIQ filters on `occurred_at` using a `from/to` (half-open) range and recognizes Dutch date phrases transmitted over REST.
- Hindsight's `temporal_window` only ranks results (a 1-day window yielded 54 hits over five days), and its `memories/list` endpoint lacked a date filter.
- CortexIQ's `remember` function stores `content`, `namespace`, `tags`, `occurred_at`, `external_key`, `external_refs`, and `importance`, but discards free metadata; its `external_key` ensures idempotent writes (returning `unchanged:true` for identical content and refusing other content unless `supersedes`), using `gaia:kairos:<episode_id>` as the mirror key format.
- CortexIQ's `DELETE /api/v1/episodes/:id` endpoint performs a hard purge that cascades to derived facts; facts and namespaces cannot be removed separately via REST, leading to 404/405 errors.
- For twenty Dutch questions, CortexIQ achieved a median response time of 1.061 ms with raw episodes at the top, whereas Hindsight recorded 1.372 ms with its top results saturated by emotional Gaia memories.

**B variant** (numbers kept 5/13, invented [], gate would reject 0)

- CortexIQ filters on `occurred_at` using `from/to` (half-open) and recognizes Dutch date sentences via REST, unlike Hindsight whose `temporal_window` only ranks and `memories/list` has no date filter.
- CortexIQ's scoping uses namespaces (array in REST); its `remember` function stores content, namespace, tags, occurred_at, external_key, external_refs, and importance but discards free metadata; the `external_key` makes writes idempotent, using `gaia:kairos:<episode_id>` as the mirror key for Gaia episodes.
- CortexIQ's DELETE /api/v1/episodes/:id endpoint performs a hard purge that cascades to derived facts, but facts and namespaces cannot be removed separately via REST due to 404/405 errors.
- For twenty Dutch questions, CortexIQ had a median response time of 1.061 ms with raw episodes appearing at the top of results, while Hindsight had a median response time of 1.372 ms with its top results saturated by emotional Gaia memories.

## Episode 1082

> Migratieplan Hindsight→CortexIQ in Gaia-Cloud vastgesteld op 18-11-2026 (plansessie): volgorde zoals de spike van 16-11 bepaalde (MemoryStore-poort → terugkijk-route → gaia-logos rebuild → bojan/gaia import → Hindsight uit). Twee beslissingen van Bo, beide 'meten gaat voor bouwen': (1) CortexIQ-fase 1 (quota recall over namespaces) wordt pas gebouwd nadat een fixture op de live strato-instance aantoonbaar maakt dat één luide namespace de rest verdringt — geen bewijs, geen bouw; (2) hosting van CortexIQ voor Gaia (strato via Tailscale vs eigen instance op contabo) wordt pas gekozen na een latency-meting contabo→strato, uit te voeren nadat de REST aan de tailnet-kant is gebonden (nu 127.0.0.1:18101). Fase 6 (agent_identity) voorlopig geschrapt: namespace + tag dekt de epistemic mark. Fase 0-huiskeeping: episode 980 (Mindstack, Vite-migratie) opnieuw aanleveren; oude hindsight-backups van 03-05 dienen alleen als formaat-referentie bij de verse export in fase 4.

**A production** (numbers kept 6/10, invented [2026-11-18 2026-11-16], gate would reject 0)

- The Gaia AI-assistent Hindsight to CortexIQ migration plan was established on 2026-11-18 during a planning session.
- The Gaia AI-assistent Hindsight to CortexIQ migration sequence, determined by a spike on 2026-11-16, includes MemoryStore-poort, terugkijk-route, gaia-logos rebuild, bojan/gaia import, and Hindsight out.
- The building of CortexIQ phase 1 (quota recall over namespaces) is conditional on empirical evidence from a live strato-instance demonstrating that one noisy namespace displaces others.
- The selection of CortexIQ hosting for Gaia AI-assistent (strato via Tailscale vs. own instance on Contabo) will occur after a latency measurement between Contabo and strato, which will be performed after the REST interface is bound on the tailnet side, currently at 127.0.0.1:18101.
- CortexIQ phase 6 (agent_identity) was temporarily removed because namespace + tag covers the epistemic mark.
- Old Hindsight backups from 03-05 serve only as format reference for the fresh export in phase 4.

**B variant** (numbers kept 10/10, invented [], gate would reject 0)

- A migration plan from Hindsight to CortexIQ in Gaia-Cloud was established on 18-11-2026, with the sequence determined by the 16-11 spike: MemoryStore-poort, terugkijk-route, gaia-logos rebuild, bojan/gaia import, then Hindsight uit.
- CortexIQ-fase 1 (quota recall over namespaces) will only be built after a fixture on the live strato-instance demonstrably shows that one noisy namespace displaces the others.
- The hosting choice for CortexIQ for Gaia (strato via Tailscale vs. own instance on contabo) will be made after a latency measurement from contabo to strato, to be performed once the REST is bound on the tailnet side (currently 127.0.0.1:18101).
- CortexIQ Fase 6 (agent_identity) was temporarily scrapped because namespace + tag covers the epistemic mark.
- As part of CortexIQ Fase 0-huiskeeping, episode 980 (Mindstack, Vite-migratie) needs to be resubmitted.
- Old hindsight-backups from 03-05 serve only as a format reference for the fresh export in Fase 4.

## Episode 558

> Gap-analyse CortexIQ vs Hindsight voor Gaia-Cloud (2026-11): Gaia-Cloud hangt via src/hindsightClient.js aan Hindsight voor: (1) banks + cross-bank recall met per-bank quota (scores niet op één schaal), (2) willekeurige tags per memory-item die in recall-output terugkomen (gaia:* tags, memoryKindOf), (3) versiegelde invalidatie/superseding sync vanuit cognitionSync met reconcile, (4) provenance sources:['foundation:<id>'] voor Dual Recall, (5) async fire-and-forget retain, (6) agent_identity-enforcing. CortexIQ heeft inmiddels wél reflect, mental models, reranker, graph-arm, temporal recall en MMR — die eerdere gaten zijn gedicht.

**A production** (numbers kept 0/7, invented [], gate would reject 0)

- Gaia-Cloud depends on Hindsight via `src/hindsightClient.js` for banks + cross-bank recall with per-bank quota (where scores are not on a single scale), arbitrary tags per memory-item in recall-output (including `gaia:*` tags and `memoryKindOf`), sealed invalidation/superseding sync from cognitionSync with reconcile, provenance sources `['foundation:<id>']` for Dual Recall, async fire-and-forget retain, and agent_identity-enforcing.
- CortexIQ possesses capabilities including reflect, mental models, reranker, graph-arm, temporal recall, and MMR.

**B variant** (numbers kept 0/7, invented [], gate would reject 0)

- Gaia-Cloud relies on Hindsight via src/hindsightClient.js for banks and cross-bank recall with per-bank quota (scores not on one scale), arbitrary tags per memory-item returning in recall-output (including gaia:* tags and memoryKindOf), sealed invalidation/superseding sync from cognitionSync with reconcile, provenance sources ['foundation:<id>'] for Dual Recall, async fire-and-forget retain, and agent_identity-enforcing functionality.
- CortexIQ currently includes reflect, mental models, reranker, graph-arm, temporal recall, and MMR functionalities.

## Episode 36

> Beslissing 2026-10-08 (akkoord Bo): de provenance-prefix voor afgeleide records in Gaia-Cloud gaat van 'chronicle:' naar 'foundation:'. Canonieke id-vorm: 'foundation:<uuid>' met de kale ingest_object uuid (wat Foundation /api/ingest-logs/:id verwacht), niet Foundation's episode-vorm 'ingest:<uuid>'. Reden: 'chronicle:' is legacy (Foundation, niet Chronicle, is de rauwe laag) en is een routing key, geen label. Wordt in één migratie gedaan die ook de strip-bug in kairosRoutes.js fixt en de drie sources-kolommen (hypotheses/patterns/kairos_episodes) plus Hindsight-metadata backfillt; reconcile() wordt uitgebreid met Kairos-episodes. Plan: C:\Users\bojan\.opencode\plan\chronicle-prefix-provenance-plan.md.

**A production** (numbers kept 1/1, invented [], gate would reject 0)

- A decision for Gaia AI-assistent on 2026-10-08 stated the provenance-prefix for derived records in Gaia-Cloud will change from 'chronicle:' to 'foundation:'.
- A decision for Gaia AI-assistent on 2026-10-08 specified the canonical ID form for derived records will be 'foundation:<uuid>' using the raw ingest_object UUID, not Foundation's episode-form 'ingest:<uuid>'.
- The stated reason for changing the Gaia-Cloud provenance-prefix from 'chronicle:' was that 'chronicle:' is legacy (Foundation, not Chronicle, is the raw layer) and acts as a routing key, not a label.
- A planned migration for Gaia AI-assistent will include fixing a strip-bug in kairosRoutes.js, backfilling three sources-columns (hypotheses/patterns/kairos_episodes) and Hindsight-metadata, and extending reconcile() with Kairos-episodes.
- The plan for the Gaia AI-assistent chronicle-prefix provenance change is documented at C:\Users\bojan\.opencode\plan\chronicle-prefix-provenance-plan.md.

**B variant** (numbers kept 0/1, invented [], gate would reject 0)

- The provenance-prefix for derived records in Gaia-Cloud within Gaia AI-assistent was changed from 'chronicle:' to 'foundation:'.
- The canonical ID form for derived records in Gaia-Cloud within Gaia AI-assistent was updated to 'foundation:<uuid>', using the raw ingest_object uuid (which Foundation /api/ingest-logs/:id expects), not Foundation's episode-form 'ingest:<uuid>'.
- The reason for changing the provenance-prefix in Gaia AI-assistent from 'chronicle:' was that 'chronicle:' was considered legacy and a routing key rather than a label, with Foundation serving as the raw layer.
- A migration for Gaia AI-assistent included fixing a strip-bug in kairosRoutes.js, backfilling the hypotheses, patterns, and kairos_episodes sources columns along with Hindsight-metadata, and extending reconcile() with Kairos-episodes.
- The plan for the chronicle-prefix provenance migration for Gaia AI-assistent was located at C:\Users\bojan\.opencode\plan\chronicle-prefix-provenance-plan.md.

## Episode 37

> Gaia-Cloud: de chronicle:->foundation: provenance-normalisatie is op 2026-10-08 geïmplementeerd (nieuwe seam src/foundationRef.js, synthesizer+kairosRoutes erop, cognitionSync.reconcile() pusht nu ook Kairos-episodes, migratie 010_normalize_sources_prefix.sql). Tests groen: gaia-api 1244/1244, cognition 49/49. Nog open: migratie toepassen op de VPS (npm run migrate in services/cognition) en daarna npm run reconcile:hindsight -- --apply --clear om de Hindsight-metadata te verversen.

**A production** (numbers kept 2/4, invented [], gate would reject 0)

- Provenance normalization for the Gaia AI-assistent was implemented on 2026-10-08, including a new seam src/foundationRef.js, synthesizer+kairosRoutes, cognitionSync.reconcile() pushing Kairos-episodes, and migration 010_normalize_sources_prefix.sql.
- All 1244 gaia-api tests for the Gaia AI-assistent were green.
- All 49 cognition tests for the Gaia AI-assistent were green.

**B variant** (numbers kept 4/4, invented [], gate would reject 0)

- Gaia AI-assistent's chronicle:->foundation component had provenance normalization implemented on 2026-10-08. This implementation included a new seam src/foundationRef.js, synthesizer+kairosRoutes, cognitionSync.reconcile() pushing Kairos-episodes, and migration 010_normalize_sources_prefix.sql.
- Gaia AI-assistent's gaia-api tests were green (1244/1244) and cognition tests were green (49/49).

## Episode 38

> Reconcile dry-run Gaia-Cloud 2026-10-08: Hindsight actief gaia:hypothesis=2, gaia:pattern=0, gaia:kairos_episode=0; Cognition hypotheses=4, patterns=0, kairos=0. De foundation:-prefix-backfill bleek een no-op: de oude 'chronicle:'-prefix werd alleen door de Kairos-synthesizer geproduceerd en er zijn geen Kairos-episodes (GAIA_KAIROS_ENABLED staat uit), dus --apply --clear is overgeslagen (zou enkel versie-churn geven). De 4-vs-2 kan deels survey-artefact zijn: open_question/relationship/mental_model dragen andere tags en tellen niet als gaia:hypothesis.

**A production** (numbers kept 4/5, invented [], gate would reject 0)

- On 2026-10-08, Gaia AI-assistent performed a reconcile dry-run on Gaia-Cloud.
- During a reconcile dry-run on Gaia-Cloud on 2026-10-08, Gaia AI-assistent measured Hindsight active gaia:hypothesis as 2, gaia:pattern as 0, gaia:kairos_episode as 0, and Cognition hypotheses as 4, patterns as 0, kairos as 0.
- The foundation:-prefix-backfill operation on Gaia AI-assistent was a no-op because the old 'chronicle:'-prefix was only produced by the Kairos-synthesizer, there were no Kairos-episodes, and GAIA_KAIROS_ENABLED was off.
- The --apply --clear action for Gaia AI-assistent was skipped because it would only cause version-churn.
- The difference between Cognition hypotheses (4) and Hindsight active gaia:hypothesis (2) in Gaia AI-assistent might be partly due to a survey-artefact, as open_question/relationship/mental_model carry different tags and do not count as gaia:hypothesis.

**B variant** (numbers kept 4/5, invented [], gate would reject 0)

- A Reconcile dry-run for Gaia-Cloud on 2026-10-08 reported Hindsight active metrics as `gaia:hypothesis=2`, `gaia:pattern=0`, `gaia:kairos_episode=0`, and Cognition metrics as `hypotheses=4`, `patterns=0`, `kairos=0`.
- The `foundation:-prefix-backfill` for Gaia AI-assistent was a no-op because the old `chronicle:`-prefix was only produced by the Kairos-synthesizer, there were no Kairos-episodes, and `GAIA_KAIROS_ENABLED` was off.
- The `--apply --clear` command was skipped during the Gaia AI-assistent Reconcile dry-run to avoid version-churn.
- The difference between 4 Cognition hypotheses and 2 Hindsight `gaia:hypothesis` in Gaia AI-assistent might be a survey-artefact, as `open_question`, `relationship`, and `mental_model` carry different tags and are not counted as `gaia:hypothesis`.

## Episode 81

> Gaia-Cloud aanvulling (commit 3f645dd, 2026-10-12): een op het verleden gerichte beurt raadpleegt nu zowel Hindsight als het transcript. recallRelevantContext (memory.js) kreeg een `pastReference`-optie die de recall-gate opent; turn.js geeft die door zodra de gesprekszoek-signalen (exactHistory/pastLookup/rememberedKnowledge) vuren. Reden: Bo's punt 'ze kijkt ook in Hindsight' — de twee gates (memoryPolicy.shouldRecall en de gesprekszoek-signalen) waren onafhankelijk, dus sommige lookup-formuleringen openden alleen het transcript. Let op: Hindsight draait sowieso al elke beurt via de mentale modellen (fetchMentalModelContext), ongeacht de gate. gaia-api 1433/1433 groen (+2).

**A production** (numbers kept 0/4, invented [], gate would reject 0)

- A past-oriented turn in the Gaia AI-assistent now consults both Hindsight and the transcript.
- The `recallRelevantContext` function in `memory.js` for Gaia AI-assistent gained a `pastReference` option that opens the recall-gate, which `turn.js` passes when conversation search signals (exactHistory/pastLookup/rememberedKnowledge) fire.
- Previously, in Gaia AI-assistent's memory lookup mechanism, some lookup formulations only opened the transcript because the `memoryPolicy.shouldRecall` gate and conversation search signals were independent.
- Hindsight in Gaia AI-assistent runs every turn via the mental models (`fetchMentalModelContext`), regardless of the gate.

**B variant** (numbers kept 0/4, invented [], gate would reject 0)

- A past-oriented turn in Gaia AI-assistent now consults both Hindsight and the transcript.
- The recallRelevantContext function in memory.js gained a `pastReference` option that opens the recall-gate.
- turn.js passes the `pastReference` option when the conversational search signals exactHistory, pastLookup, or rememberedKnowledge fire.
- The memoryPolicy.shouldRecall and conversational search signals gates in Gaia AI-assistent were previously independent, which caused some lookup formulations to open only the transcript.
- Hindsight runs every turn via the mental models (fetchMentalModelContext), regardless of the gate.

## Episode 86

> Gaia-Cloud (2026-10-12, commit 8e26794, gedeployed): (1) recall-staart gemeten uit Hindsight's Prometheus-histogram (hindsight_http_duration_seconds, recall-endpoint): 27 recalls, mean 894ms, p50 ~1s, p90/p95 ~2,5s, p99 ~5s; 1 call (3,7%) boven 4s, 0 boven 8s. Kleine, niet-representatieve steekproef (grotendeels test-calls), buckets grof. (2) Observability: elke gefaalde Hindsight-call logt nu een content-loze regel {kind:'hindsight.call_failed', bank, op, reason:'timeout'|'unreachable', timeoutMs, ms} en throwt door — via een request()-wrapper rond fetchImpl in src/hindsightClient.js (alle 14 call-sites). Reden: memory.js slikte Hindsight-fouten stil in (best-effort -> []), dus missende context was onzichtbaar. Test test/hindsightCallLog.test.js (+3), gaia-api 1436/1436. Het is puur een logregel, geen gedragsverandering; verschijnt in de gaia-api containerlog.

**A production** (numbers kept 15/18, invented [2.5s 3.7], gate would reject 0)

- A performance measurement for Gaia AI-assistent's recall-endpoint, derived from Hindsight's Prometheus-histogram (`hindsight_http_duration_seconds`), recorded 27 recalls with a mean of 894ms, p50 at approximately 1s, p90/p95 at approximately 2.5s, and p99 at approximately 5s. One call (3.7%) exceeded 4s, and zero calls exceeded 8s. This measurement was based on a small, non-representative sample, largely test-calls, with coarse buckets.
- Gaia AI-assistent's Hindsight client now logs a content-less line, formatted as {kind:'hindsight.call_failed', bank, op, reason:'timeout'|'unreachable', timeoutMs, ms}, for every failed Hindsight call, and then rethrows the error. This functionality is implemented via a `request()`-wrapper around `fetchImpl` in `src/hindsightClient.js`, covering all 14 call-sites. The reason for this implementation was that `memory.js` silently consumed Hindsight errors, making missing context invisible. This log line appears in the `gaia-api` containerlog and represents a logging change only, not a behavioral change.

**B variant** (numbers kept 16/18, invented [2.5s 3.7], gate would reject 0)

- On 2026-10-12, the Gaia AI-assistent recall-endpoint's performance was measured from Hindsight's Prometheus-histogram (hindsight_http_duration_seconds) for 27 recalls, showing a mean of 894ms, p50 of ~1s, p90/p95 of ~2.5s, and p99 of ~5s; 1 call (3.7%) was above 4s, and 0 calls were above 8s. The sample was small, not representative (largely test-calls), and used coarse buckets.
- The Gaia AI-assistent was updated so that each failed Hindsight-call now logs a content-less line (e.g., {kind:'hindsight.call_failed', bank, op, reason:'timeout'|'unreachable', timeoutMs, ms}) and throws through. This change is implemented via a request()-wrapper around fetchImpl in src/hindsightClient.js (affecting all 14 call-sites) because memory.js previously silently swallowed Hindsight errors, making missing context invisible.

## Episode 560

> Beslissing: LongMemEval-harnas is uitgeschreven als docs/design-longmemeval.md in cortexiq (commit eb37c41, gepusht naar main 2026-11-02). Belangrijkste bevinding: internal/benchmark bestaat al en is eerlijk (ingest als episodes, antwoorden via reflect, beide judges) — het gat is de officiële dataset + fairness-protocol + publicatie. Kernregels: judge is een ander/sterker model dan het antwoordmodel, abstention krijgt een speciale judge-regel, geen benchmark-specifieke tuning, alleen de S-variant draaien flattert consolidatie (dus ook de M/day-variant), Hindsight-cijfers worden gequoteerd met bron als hun meting.

**A production** (numbers kept 2/2, invented [], gate would reject 0)

- The Gaia AI-assistent project's LongMemEval-harnas design document was written as docs/design-longmemeval.md in CortexIQ, committed with eb37c41 and pushed to main on 2026-11-02.
- The Gaia AI-assistent project's existing internal/benchmark was found to be fair, achieved by ingesting as episodes, answering via reflect, and using both judges.
- The Gaia AI-assistent project's internal/benchmark identified a gap regarding an official dataset, fairness-protocol, and publication.
- The Gaia AI-assistent project established core rules for evaluation, including that the judge must be a different/stronger model than the answer model, abstention receives a special judge rule, benchmark-specific tuning is prohibited, running only the S-variant flatters consolidation (also the M/day variant), and Hindsight figures must be quoted with their source as their measurement.

**B variant** (numbers kept 2/2, invented [], gate would reject 0)

- The Gaia AI-assistent LongMemEval-harnas was documented as docs/design-longmemeval.md in cortexiq with commit eb37c41, pushed to main on 2026-11-02.
- Gaia AI-assistent's internal/benchmark was found to already exist and be fair, as it ingested episodes, answered via reflect, and used both judges.
- The LongMemEval-harnas for Gaia AI-assistent was identified to be missing an official dataset, a fairness protocol, and a publication.
- A core rule for the Gaia AI-assistent's LongMemEval-harnas was that the judge model must be different from and stronger than the answer model.
- A core rule for the Gaia AI-assistent's LongMemEval-harnas was that abstention would receive a special judge-rule.
- A core rule for the Gaia AI-assistent's LongMemEval-harnas was that there should be no benchmark-specific tuning.

## Episode 364

> Gaia Cloud now retains eligible Hindsight turns as a Hermes-style JSON array of role-labelled User and Assistant messages. Existing memoryworthiness/triviality gates, asynchronous fire-and-forget behavior, and identity/provenance metadata remain; no historical Hindsight data is rewritten. Deployed as commit bddbad5 on 2026-10-23.

**A production** (numbers kept 2/2, invented [], gate would reject 0)

- Gaia AI-assistent's Gaia Cloud retains eligible Hindsight turns as a Hermes-style JSON array of role-labelled User and Assistant messages.
- Gaia AI-assistent's Gaia Cloud Hindsight retention process uses memoryworthiness/triviality gates, asynchronous fire-and-forget behavior, and identity/provenance metadata.
- Gaia AI-assistent's Gaia Cloud does not rewrite historical Hindsight data.
- Gaia AI-assistent's Gaia Cloud was deployed as commit bddbad5 on 2026-10-23.

**B variant** (numbers kept 2/2, invented [], gate would reject 1)

- Gaia Cloud now retains eligible Hindsight turns as a Hermes-style JSON array of role-labelled User and Assistant messages.
- For Gaia Cloud, existing memoryworthiness/triviality gates, asynchronous fire-and-forget behavior, and identity/provenance metadata remain.
- Gaia Cloud does not rewrite historical Hindsight data.
- Gaia Cloud was deployed as commit bddbad5 on 2026-10-23.

## Episode 859

> CortexIQ BM25-repetitie op de productiedump (2026-11-11): CI op 3774ea9 groen. Dagelijkse prod-dump (47 MB, checksum OK; prod = 2.944 facts, 728 episodes, 128 MB, goose 60, bge-m3/dim 1024) in verse PG17-container met deploy/postgres-image: pg_restore 5 s, db.Open inclusief migratie 00061 335 ms, bm25-indexen aanwezig, tsvector-kolommen weg. Kwaliteit: BM25 wint bij inspectie van 3 van 15 queries (IDF: zeldzame tokens als 8445 en bge-m3 komen bovenaan, ts_rank_cd gaf generieke woorden); geen gelabelde eval gedraaid, vervolgstap is cortexiq eval recall. Snelheidswinst is op dit kleine corpus nog theoretisch. VPS-toegang was read-only (backuplijst, scp van dump); dump en container na afloop verwijderd. OPEN: prod nog op PG16 en niet geüpgraded; release-job voor cortexiq-postgres en arm64 nooit gedraaid; Bo moet de ssh- en scp-allow-regels en de oude gh-release-create-regel uit .claude/settings.local.json halen (bestand is in git gevolgd, niet committen). Commit van de docs: zie git log.

**A production** (numbers kept 9/21, invented [], gate would reject 0)

- The CortexIQ production data used for the BM25 repetition had 2,944 facts, 728 episodes, a total size of 128 MB, used goose 60, and bge-m3 with dimension 1024.
- The CortexIQ production environment was running on PostgreSQL 16 and had not been upgraded.
- The release job for `cortexiq-postgres` and `arm64` had never been run.
- The `.claude/settings.local.json` file contained `ssh-` and `scp-allow-regels` and an `oude gh-release-create-regel`.
- The `.claude/settings.local.json` file was followed in git.

**B variant** (numbers kept 15/21, invented [], gate would reject 0)

- A CortexIQ daily production dump used for a BM25 rehearsal had a file size of 47 MB, and the corresponding production data contained 2,944 facts, 728 episodes, occupied 128 MB, and referenced 'goose 60' and 'bge-m3/dim 1024'.
- A CortexIQ BM25 rehearsal was conducted using a fresh PG17-container created with the deploy/postgres-image.
- During a CortexIQ BM25 rehearsal, the database successfully opened after applying migration 00061, with BM25 indexes present and tsvector columns removed.
- In a CortexIQ BM25 rehearsal, BM25 search quality won against ts_rank_cd in 3 out of 15 queries during inspection; BM25's IDF mechanism ranked rare tokens like '8445' and 'bge-m3' higher, while ts_rank_cd yielded generic words.
- The CortexIQ production environment was observed to be running PostgreSQL version PG16 and had not yet been upgraded.
- The release job for cortexiq-postgres and arm64 had never been run for CortexIQ.

## Episode 759

> De productiebug supported_by is lokaal opgelost: de background consolidator selecteerde hypotheses zonder supported_by voor pgx named scanning, en positional hypothesis/counter scans misten het veld. Drie bestanden zijn aangepast, walkthrough bijgewerkt, go test ./internal/brain en go test ./... zijn groen. Niet gedeployed; aparte expliciete goedkeuring vereist.

**A production** (numbers kept 0/0, invented [], gate would reject 0)

- The production bug `supported_by` in CortexIQ was locally resolved.
- The `supported_by` production bug in CortexIQ was caused by the background consolidator selecting hypotheses without `supported_by` for `pgx named scanning`, and by positional hypothesis/counter scans missing the `supported_by` field.
- For the fix of the `supported_by` production bug in CortexIQ, three files were adapted, the walkthrough was updated, and `go test ./internal/brain` and `go test ./...` tests were green.
- The fix for the `supported_by` production bug in CortexIQ was not deployed.
- Separate explicit approval was required for the deployment of the fix for the `supported_by` production bug in CortexIQ.

**B variant** (numbers kept 0/0, invented [], gate would reject 0)

- CortexIQ's background consolidator had a production bug where it selected hypotheses without `supported_by` for `pgx named scanning`, and `positional hypothesis/counter scans` missed the `supported_by` field.
- For CortexIQ, three files were adapted and the walkthrough was updated as part of a local fix.
- For CortexIQ, the `go test ./internal/brain` and `go test ./...` commands were green after a local fix.
- The fix for CortexIQ was not deployed and required separate explicit approval.

## Episode 864

> CortexIQ migratie-race bij opstarten gerepareerd en gepusht (2026-11-11): internal/db/db.go draait de hele schema-opzet (goose.Up, dimensie-ALTER, HNSW-indexen) nu in setupSchema onder withSchemaLock, een sessie-advisory lock (pg_advisory_lock, sleutel 994294737458 = 'cortexiq' in base 36) op een eigen verbinding, wachttijd begrensd op 5 minuten, bij annuleren wordt de sessie gesloten zodat er geen lock achterblijft; Open sluit de pool nu bij elke fout (lekte bij goose-fout). Regressietests internal/db/schemalock_integration_test.go (6 gelijktijdige Open's op een lege database, wederzijdse uitsluiting, annuleerbare wacht); mutatiecheck: zonder lock falen 5 van 6 starts met dezelfde 42P07-fout als in prod. Volledige suite groen op verse PG17-db: 21 pakketten, 153 integratietests, 0 skip. NOG NIET UITGEROLD: productie draait v0.1.74 zonder dit slot; het zit in de volgende release. Open: Bo moet de ssh-allow-regels uit .claude/settings.local.json halen (de uitrol is klaar) en de Jina-sleutel roteren; compose op de VPS wijst nog naar de lokale image cortexiq-postgres:17-bm25 (ghcr-image ...-postgres:0.1.74 bestaat).

**A production** (numbers kept 9/12, invented [], gate would reject 0)

- CortexIQ's internal/db/db.go was updated to run its entire schema setup (goose.Up, dimension ALTER, HNSW indexes) within setupSchema under withSchemaLock, which uses a session-advisory lock.
- CortexIQ's withSchemaLock utilizes a pg_advisory_lock with key 994294737458 (representing 'cortexiq' in base 36) on a dedicated connection, with a waiting time limited to 5 minutes, ensuring the session is closed upon cancellation to prevent lock persistence.
- The CortexIQ Open function was updated to close the connection pool upon every error, addressing a previous leak that occurred during goose errors.
- Regression tests for CortexIQ's schema lock were implemented in internal/db/schemalock_integration_test.go, covering scenarios like 6 concurrent Open calls on an empty database, mutual exclusion, and cancellable waiting.
- Without the schema lock, 5 out of 6 CortexIQ starts failed with a 42P07 error (as observed in production); after the fix, the full test suite passed on a fresh PostgreSQL 17 database (21 packages, 153 integration tests, 0 skips).
- CortexIQ production was running version v0.1.74 without the schema lock, which is planned for the next release. Additionally, the compose configuration on the VPS was still referencing the local image cortexiq-postgres:17-bm25, despite the existence of ghcr-image ...-postgres:0.1.74.

**B variant** (numbers kept 6/12, invented [], gate would reject 0)

- CortexIQ's schema setup process, including goose.Up, dimension ALTER operations, and HNSW index creation, runs within the setupSchema function under a withSchemaLock.
- CortexIQ's withSchemaLock is implemented as a session-advisory lock (pg_advisory_lock) using key 994294737458 (representing 'cortexiq' in base 36) on a separate database connection, with a waiting time limited to 5 minutes.
- CortexIQ's Open function was modified to close the database connection pool upon encountering any error.
- CortexIQ's schema lock feature is tested by regression tests located in internal/db/schemalock_integration_test.go, which includes scenarios like 6 simultaneous Open calls on an empty database, mutual exclusion, and cancellable waits.
- CortexIQ's production environment was running version v0.1.74, which did not include the new schema lock mechanism.

## Episode 169

> CortexIQ prod (strato-vps, /opt/cortexiq) is op 2026-10-15 uitgerold naar commit 6377b4c: POST /api/v1/remember is live (401 zonder token, 201 met token) en de embedder wordt bij start op de achtergrond warm geladen (~1,5 s, logregel msg="embedder warm"); warme recall meet ~5 ms in plaats van de oude koude start van ~1,8 s. Goose-versie bleef 27.

**A production** (numbers kept 9/9, invented [1.5 1.8], gate would reject 0)

- CortexIQ prod (running on strato-vps, located at /opt/cortexiq) was rolled out to commit 6377b4c on 2026-10-15.
- The POST /api/v1/remember endpoint in CortexIQ was live, returning a 401 status without a token and a 201 status with a token.
- The embedder in CortexIQ was warm-loaded in the background upon start, taking approximately 1.5 seconds, indicated by the log line msg="embedder warm".
- CortexIQ measured warm recall at approximately 5 ms; previously, a cold start measured approximately 1.8 seconds.
- The Goose version used in the CortexIQ setup remained 27.

**B variant** (numbers kept 9/9, invented [1.5 1.8], gate would reject 0)

- CortexIQ prod was deployed to commit `6377b4c` on `2026-10-15` at `/opt/cortexiq` on `strato-vps`.
- The CortexIQ `POST /api/v1/remember` endpoint is live, responding with `401` without a token and `201` with a token.
- The CortexIQ embedder is warm-loaded in the background during startup, taking approximately 1.5 seconds, indicated by the log line `msg="embedder warm"`.
- Warm recall in CortexIQ was measured at approximately 5 ms.
- The old cold start for CortexIQ recall was measured at approximately 1.8 seconds.
- The Goose version used by CortexIQ remained 27.

## Episode 697

> CortexIQ: RRF-sweep gedraaid op de productie-embedder (bge-m3, hybride, Jina-reranker; 60 memories, 36 vragen, k=10) via docker compose run met een override naar een wegwerpdatabase cortexiq_eval (productie niet geraakt, daarna verwijderd). Uitkomst overall: rrf-k 20 hit@1 0.75 MRR 0.850; rrf-k 60 hit@1 0.78 MRR 0.868; rrf-k 100 identiek aan 60. Verschil is één à twee vragen van 36, dus ruis. Besluit: CORTEXIQ_RRF_K blijft 60. Alle varianten missen dezelfde acht vragen op ongeveer dezelfde plaats, dus een betere recall zit in embedder/reranker, niet in de fusieconstante. Recept voor een volgende keer: op de server in /opt/cortexiq een override met CORTEXIQ_POSTGRES_DSN naar een extra database, dan docker compose -f docker-compose.prod.yml -f override run --rm --no-deps -T -v /opt/cortexiq/eval:/eval:ro cortexiq eval recall --rrf-k ... --corpus /eval/recall_corpus.jsonl --queries /eval/recall_queries.jsonl. Les: gebruik bij .env-greps nooit een patroon dat KEY-variabelen raakt (RERANK matchte CORTEXIQ_RERANKER_API_KEY en de Jina-sleutel kwam in het gesprek; Bo is aangeraden hem te roteren).

**A production** (numbers kept 11/11, invented [], gate would reject 0)

- An RRF-sweep was performed on the CortexIQ production-embedder (bge-m3, hybride, Jina-reranker) using 60 memories, 36 questions, and k=10. This sweep was executed via docker compose run with an override to a temporary database named cortexiq_eval.
- The RRF-sweep results for CortexIQ showed hit@1 0.75 and MRR 0.850 for rrf-k 20, hit@1 0.78 and MRR 0.868 for rrf-k 60, and rrf-k 100 was identical to rrf-k 60.
- It was decided that the CORTEXIQ_RRF_K configuration for CortexIQ would remain 60.
- For CortexIQ, a better recall is expected to come from improving the embedder/reranker, not the fusion constant, as all tested variants consistently missed the same eight questions.
- For future recall evaluations of CortexIQ, the recommended procedure is to use an override with CORTEXIQ_POSTGRES_DSN pointing to an extra database on the server in /opt/cortexiq, and execute the command: docker compose -f docker-compose.prod.yml -f override run --rm --no-deps -T -v /opt/cortexiq/eval:/eval:ro cortexiq eval recall --rrf-k ... --corpus /eval/recall_corpus.jsonl --queries /eval/recall_queries.jsonl.
- A lesson learned for CortexIQ was to avoid using .env-grep patterns that match KEY variables, as a 'RERANK' pattern matched 'CORTEXIQ_RERANKER_API_KEY', leading to the Jina key being exposed, and it was advised to rotate this key.

**B variant** (numbers kept 11/11, invented [], gate would reject 0)

- An RRF-sweep was executed for CortexIQ on the production embedder, which used bge-m3, hybride, Jina-reranker, and was tested with 60 memories, 36 questions, and k=10.
- The CortexIQ RRF-sweep was run against a disposable database named `cortexiq_eval`, ensuring the production database was not affected, and `cortexiq_eval` was subsequently removed.
- An RRF-sweep on CortexIQ yielded hit@1 0.75 and MRR 0.850 for rrf-k 20, hit@1 0.78 and MRR 0.868 for rrf-k 60, and identical results for rrf-k 100. Based on these outcomes, the CORTEXIQ_RRF_K setting was decided to remain 60.
- An analysis of CortexIQ RRF-sweep results indicated that all tested variants missed the same eight out of 36 questions, suggesting that improvements to recall should focus on the embedder or reranker rather than the fusion constant.
- A security lesson from CortexIQ operations highlighted the risk of using .env-grep patterns that match KEY-variables, as demonstrated when "RERANK" matched CORTEXIQ_RERANKER_API_KEY, exposing a Jina key; a recommendation was made to rotate the key.

## Episode 888

> CortexIQ: v0.1.80 (Timeline-vergelijking tussen twee data, commit 6f725cd, CI groen) is door Claude gepubliceerd en de Release-workflow is groen (2026-11-13). De uitrol naar productie is NIET uitgevoerd: de auto-mode classifier blokkeerde het ssh-commando voor de deploy. Productie draait vermoedelijk nog v0.1.79 (niet geverifieerd). Bo moet de deploy toestaan of zelf draaien. Let op: vaste toestemming van Bo gold voor het publiceren van releases; uitrol via ssh valt buiten wat de classifier toestaat zonder allow-regel in .claude/settings.local.json.

**A production** (numbers kept 4/4, invented [], gate would reject 0)

- CortexIQ version v0.1.80, commit 6f725cd, which includes 'Timeline-vergelijking tussen twee data', was published by Claude on 2026-11-13 with a green CI and a green Release-workflow.
- The rollout of CortexIQ version v0.1.80 to production was NOT performed because the auto-mode classifier blocked the ssh-command for the deploy.
- CortexIQ production was presumably still running version v0.1.79, which was not verified.
- Bo's standing permission was for publishing CortexIQ releases.
- The auto-mode classifier was configured such that rollout via ssh for CortexIQ falls outside its allowance without an allow-rule in .claude/settings.local.json.

**B variant** (numbers kept 4/4, invented [], gate would reject 0)

- CortexIQ version v0.1.80 was published by Claude on 2026-11-13, associated with commit 6f725cd, and had its CI green and Release-workflow green.
- CortexIQ v0.1.80 includes the feature "Timeline-vergelijking tussen twee data".
- The deployment of CortexIQ v0.1.80 to production was NOT executed.
- The auto-mode classifier blocked the SSH command for the deploy of CortexIQ v0.1.80.
- The CortexIQ production environment was presumed to be running version v0.1.79, which was not verified.
- For CortexIQ, Bo's standing permission applied to publishing releases, but SSH deployment is outside what the classifier allows without an allow rule in `.claude/settings.local.json`.

## Episode 412

> CortexIQ-productie (strato-vps, /opt/cortexiq) draait sinds 2026-10-27 v0.1.6 (commit c107f8c, image sha256:61d7ed372332). Deployprocedure: GitHub-release publiceren (release-workflow bouwt binaries en image), CORTEXIQ_IMAGE_TAG in /opt/cortexiq/.env bumpen, daarna docker compose -f docker-compose.prod.yml pull en up -d. De UI is een aparte service (cortexiq-ui) op dezelfde image; UI luistert op 127.0.0.1:18101, API op 18100, beide eisen een token.

**A production** (numbers kept 6/6, invented [], gate would reject 0)

- A CortexIQ-productie instance was observed running since 2026-10-27, with version v0.1.6, commit c107f8c, and image sha256:61d7ed372332, deployed on a strato-vps at /opt/cortexiq.
- The deployment procedure for CortexIQ involved publishing a GitHub-release (where a release-workflow builds binaries and an image), bumping the CORTEXIQ_IMAGE_TAG in /opt/cortexiq/.env, and then executing 'docker compose -f docker-compose.prod.yml pull' followed by 'up -d'.
- The CortexIQ UI service (cortexiq-ui) was configured as a separate service, running on the same image as the main application, listening on 127.0.0.1:18101, and required a token for access.
- The CortexIQ API was configured to listen on port 18100 and required a token for access.

**B variant** (numbers kept 6/6, invented [], gate would reject 0)

- CortexIQ-productie was running version v0.1.6 (commit c107f8c, image sha256:61d7ed372332) on strato-vps at /opt/cortexiq since 2026-10-27.
- The deployment procedure for CortexIQ involves publishing a GitHub release (where a release-workflow builds binaries and images), bumping CORTEXIQ_IMAGE_TAG in /opt/cortexiq/.env, and then running 'docker compose -f docker-compose.prod.yml pull' and 'up -d'.
- The cortexiq-ui service is a separate service running on the same image as CortexIQ and listens on 127.0.0.1:18101.
- The CortexIQ API listens on port 18100.
- Both the CortexIQ UI (listening on 127.0.0.1:18101) and the API (listening on 18100) require a token.

## Episode 886

> CortexIQ werkwijze (2026-11-13): Bo geeft vaste toestemming dat Claude zelf GitHub-releases publiceert (gh release create <versie> --target main) zodra CI op main groen is; Bo hoeft het niet meer zelf te doen. Daarbij altijd versienummer en releasenotities melden in het antwoord. Uitrol naar productie volgt de vaste procedure (back-up .env, pull terwijl het draait, verified dump, tag bumpen, healthz/readyz/restarts/logs controleren). Eerder deed Bo het publiceren zelf vanaf v0.1.74; die gewoonte is nu vervangen.

**A production** (numbers kept 2/2, invented [], gate would reject 0)

- On 2026-11-13, Bo granted Claude standing permission to publish CortexIQ GitHub releases automatically, using the command `gh release create <versie> --target main`, once CI on 'main' is green.
- On 2026-11-13, it was established that Claude must always report the version number and release notes in its response after publishing CortexIQ GitHub releases.
- CortexIQ's production deployment procedure involves backing up the `.env` file, pulling while the system is running, performing a verified dump, bumping the tag, and checking `healthz`, `readyz`, restarts, and logs.
- Before 2026-11-13, Bo was responsible for publishing CortexIQ GitHub releases, a practice that had been in place since version v0.1.74, but this custom has now been replaced.

**B variant** (numbers kept 0/2, invented [], gate would reject 0)

- Claude publishes GitHub releases for CortexIQ using the command `gh release create <versie> --target main` when CI on main is green, with fixed permission from Bo.
- When Claude publishes GitHub releases for CortexIQ, it must always report the version number and release notes in the answer.
- The production rollout for CortexIQ follows a fixed procedure: back-up .env, pull while it's running, verified dump, tag bump, and check healthz/readyz/restarts/logs.

## Episode 670

> CortexIQ: v0.1.20 (e56a03b) is op 2026-11-05 gedeployed naar strato-vps; v0.1.19 is wel gebouwd maar nooit uitgerold, productie ging direct van 0.1.18 naar 0.1.20. Bevat migraties 00044 (judged), 00045 (origin/origin_ref) en 00046 (strength), de rebuild-CLI en CORTEXIQ_RRF_K. Volgorde: geverifieerde dump backups/cortexiq-20261105T002405Z.dump, release, .env gebackupt als .env.bak-pre0.1.20, tag-bump en up -d. Gecontroleerd: version v0.1.20, goose 46, containers Up, geen panic of migratiefout, 2330 facts allemaal zonder origin (oude facts, bewust niet teruggevuld). Een read-only rebuild-rapport van /projects/cortexiq in productie: 114 episodes, 476 facts, 6 facts op ingetrokken bronnen (gat C). CORTEXIQ_DECAY_SPLIT staat NIET aan en docker-compose.prod.yml op de server geeft de variabele nog niet door. Open voor Bo: de ssh- en gh-release-allow-regels uit de projectsettings halen; beslissen over DECAY_SPLIT; zelf de RRF-sweep draaien (cortexiq eval recall --rrf-k 20,60,100).

**A production** (numbers kept 14/17, invented [], gate would reject 0)

- CortexIQ version v0.1.20 (e56a03b) was deployed to strato-vps on 2026-11-05. The deployment skipped v0.1.19, directly transitioning from v0.1.18 to v0.1.20.
- CortexIQ v0.1.20 included database migrations 00044 (judged), 00045 (origin/origin_ref), and 00046 (strength), along with the rebuild-CLI and CORTEXIQ_RRF_K.
- After deployment, the CortexIQ production environment confirmed version v0.1.20, goose 46, containers as Up, no panic or migration errors, and contained 2330 facts all without origin (which were old facts deliberately not refilled).
- A read-only rebuild report for /projects/cortexiq in the CortexIQ production environment showed 114 episodes, 476 facts, and 6 facts on withdrawn sources (gap C).
- The CORTEXIQ_DECAY_SPLIT feature was NOT enabled in CortexIQ, and the docker-compose.prod.yml on the server was not yet passing this variable.

**B variant** (numbers kept 12/17, invented [], gate would reject 0)

- CortexIQ version v0.1.20 (e56a03b) was deployed to strato-vps on 2026-11-05.
- CortexIQ v0.1.20 included migrations 00044 (judged), 00045 (origin/origin_ref), 00046 (strength), the rebuild-CLI, and CORTEXIQ_RRF_K.
- CortexIQ's database migration tool (goose) reported version 46 after deployment.
- CortexIQ had 2330 old facts without origin that were intentionally not backfilled.
- A read-only rebuild report for /projects/cortexiq in the CortexIQ production environment showed 114 episodes, 476 facts, and 6 facts on withdrawn sources (gap C).
- The CORTEXIQ_DECAY_SPLIT configuration was not active, and docker-compose.prod.yml on the server did not yet pass this variable.

## Episode 924

> CortexIQ besluit (2026-11-13, Bo): Jina (jina-reranker-v3.5) blijft de reranker in productie; geen eigen reranker op de VPS. Reden: lokaal (llama.cpp, bge-reranker-v2-m3) is ongeveer 2,9 s trager per recall (mogelijk 3-5 s op de VPS, gedeeld met andere workloads) en iets slechter (MRR 0,80 tegen 0,85; zie id 923). Niets gewijzigd in productie. Het risico dat Jina wegvalt (saldo, 429-limiet van 100.000 tokens per minuut, 404) wordt afgedekt door de metric recall_rerank_requests_total, de logregel per minuut, RERANK_CANDIDATES=30 en de graaf-routing als vangnet (gewogen is dan beter dan uit). Niet gedaan en niet nodig tenzij het pijn doet: fallback-keten Jina dan lokaal (codewijziging), meten met 10 kandidaten, meten op de VPS zelf.

**A production** (numbers kept 12/15, invented [2.9 0.80 0.85 429-limit], gate would reject 0)

- On 2026-11-13, CortexIQ decided that Jina (jina-reranker-v3.5) would remain the production reranker, and no custom reranker would be deployed on the VPS.
- A local reranker implementation (llama.cpp, bge-reranker-v2-m3) was observed to be approximately 2.9 seconds slower per recall and had an MRR of 0.80, which was slightly worse than Jina's MRR of 0.85 (see id 923).
- CortexIQ mitigated the risk of Jina reranker unavailability (due to issues like balance, a 429-limit of 100,000 tokens per minute, and 404 errors) by using the metric recall_rerank_requests_total, a log line per minute, setting RERANK_CANDIDATES to 30, and employing graph-routing as a safety net (where weighted routing was considered better than off).

**B variant** (numbers kept 9/15, invented [2.9 0.80 0.85], gate would reject 0)

- CortexIQ decided on 2026-11-13 that Jina (jina-reranker-v3.5) would remain the reranker in production.
- CortexIQ decided on 2026-11-13 not to deploy its own reranker on the VPS.
- Local rerankers (llama.cpp, bge-reranker-v2-m3) were measured to be approximately 2.9 seconds slower per recall and had an MRR of 0.80, which was worse than Jina's MRR of 0.85.
- CortexIQ mitigates the risk of Jina reranker outages using the `recall_rerank_requests_total` metric, a log line per minute, `RERANK_CANDIDATES=30`, and graph routing as a fallback mechanism.

## Episode 664

> CortexIQ: v0.1.18 (afgewezen-waarde-tombstone, migratie 00043, Reflect-tab #7) is op 2026-11-05 gedeployed naar strato-vps. Volgorde: geverifieerde dump (backups/cortexiq-20261104T232239Z.dump), release gepubliceerd door Bo (de auto-mode classifier weigerde gh release create van mij), .env gebackupt als .env.bak-pre0.1.18, tag-bump en up -d. Gecontroleerd: version v0.1.18 (6c89dd3), goose 43, facts_refused in de logs. Productie had 0 retracties, dus de backfill deed niets. Het Gemma-parsefoutje bij causal links in /projects/chronicle is het bekende lijstprobleem, niet nieuw. Praktisch: SSH naar strato-vps werkt via een allow-regel in de projectsettings; die hoort na de deploy weg.

**A production** (numbers kept 8/8, invented [], gate would reject 0)

- CortexIQ version v0.1.18 (afgewezen-waarde-tombstone, migratie 00043, Reflect-tab #7) was deployed to strato-vps on 2026-11-05. Post-deployment checks confirmed version v0.1.18 (6c89dd3), goose 43, and observed 'facts_refused' in CortexIQ's logs.
- A verified database backup for CortexIQ was created as backups/cortexiq-20261104T232239Z.dump prior to deployment.
- The .env file was backed up as .env.bak-pre0.1.18 before deployment.
- The CortexIQ production environment registered 0 retracties, indicating the backfill operation did nothing.
- Gemma experienced a parsing error in causal links within /projects/chronicle, identified as a known list problem.
- SSH access to strato-vps was enabled via an allow-rule in the projectsettings.

**B variant** (numbers kept 7/8, invented [], gate would reject 0)

- CortexIQ version v0.1.18 (6c89dd3) was deployed to strato-vps on 2026-11-05.
- CortexIQ v0.1.18 included the changes 'afgewezen-waarde-tombstone', 'migratie 00043', and 'Reflect-tab #7'.
- After deployment, CortexIQ was running goose version 43 and its logs contained 'facts_refused'.
- CortexIQ production had 0 retracties, which led to the backfill doing nothing.
- A known Gemma parser error was observed with causal links in /projects/chronicle, identified as the 'bekende lijstprobleem'.
- SSH access to strato-vps functioned via an allow-rule in the projectsettings.

---

episodes 21 | A: 95 facts, numbers kept 124/171, invented 11 | B: 96 facts, numbers kept 122/171, invented 7, would be rejected by the production gate 1
