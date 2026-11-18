# Fact fidelity: fireworks-ai/gpt-oss-120b

A = production prompt and gate. B = same prompt, no gate. Read the facts against the episode: subject and object right? detail kept? anything added?

## Episode 989

> Spike 16-11-2026: CortexIQ 0.1.88 (strato-vps, REST 127.0.0.1:18101) vergeleken met Hindsight 0.9.2 (contabo, 8888) als vervanger van Hindsight in Gaia. Beslissend verschil: CortexIQ filtert op occurred_at met from/to (half-open) en herkent Nederlandse datumzinnen over REST, terwijl Hindsight temporal_window alleen rankt (een venster van 1 dag gaf 54 hits over vijf dagen) en memories/list geen datumfilter heeft. Scoping moet met namespaces (array in REST); de enkelvoudige namespace is alleen een ranking-zetje. remember bewaart content, namespace, tags, occurred_at, external_key, external_refs en importance, maar gooit vrije metadata weg; external_key maakt de write idempotent (unchanged:true, andere inhoud geweigerd tenzij supersedes), dus gaia:kairos:<episode_id> is de spiegel-sleutel. DELETE /api/v1/episodes/:id purge hard en cascadeert naar afgeleide facts; facts en namespaces zijn niet los te verwijderen via REST (404/405). Consolidatie reageert binnen twee minuten maar levert even dunne facts als Hindsight. Op twintig Nederlandse vragen: CortexIQ mediaan 1.061 ms met ruwe episodes bovenaan, Hindsight 1.372 ms met een door emotionele gaia-herinneringen verzadigde top. Rapport: docs/spike-cortexiq.md (commits 85dd48e en f329e24). Volgorde blijft: MemoryStore-poort, terugkijk-route op CortexIQ, gaia-logos rebuilden, bojan/gaia importeren, Hindsight uit.

**A production** (numbers kept 11/13, invented [], gate would reject 0)

- CortexIQ version 0.1.88 running on strato‑vps at REST 127.0.0.1:18101 is being used to replace Hindsight version 0.9.2 hosted on contabo port 8888 in the Gaia AI‑assistant; CortexIQ filters on occurred_at with half‑open from/to intervals and recognises Dutch date sentences over REST, while Hindsight relies only on a temporal_window ranking (a 1‑day window yielded 54 hits over five days) and its memories/list endpoint lacks a date filter.
- Scoping for queries must be supplied as a namespaces array in the REST API; using a single namespace provides only a ranking set.
- The remember operation stores content, namespace, tags, occurred_at, external_key, external_refs and importance while discarding free metadata; the external_key makes writes idempotent (unchanged:true, other content rejected unless superseding) and uses the mirror key format gaia:kairos:<episode_id>.
- Calling DELETE /api/v1/episodes/:id permanently purges the episode and cascades the deletion to derived facts; facts and namespaces cannot be removed via REST and return 404 or 405 errors.
- During a test of twenty Dutch questions CortexIQ returned a median response time of 1.061 ms with raw episodes ranked at the top, while Hindsight had a median of 1.372 ms and its top results were saturated by emotional Gaia memories; consolidation reacts within two minutes but produces facts as thin as those from Hindsight.
- The report for the spike is stored at docs/spike-cortexiq.md with commit hashes 85dd48e and f329e24; the remaining ordered tasks are MemoryStore port, backtrack route on CortexIQ, gaia‑logos rebuild, bojan/gaia import, and removal of Hindsight.

**B variant** (numbers kept 12/13, invented [], gate would reject 0)

- CortexIQ version 0.1.88 was deployed on a strato‑vps exposing a REST endpoint at 127.0.0.1:18101, and it was compared with Hindsight version 0.9.2 running on contabo at port 8888 as the replacement for Hindsight in the Gaia AI‑assistant.
- CortexIQ filters on the occurred_at field using half‑open from/to ranges and recognises Dutch date expressions over REST, whereas Hindsight only supports a temporal_window ranking (e.g., a 1‑day window yielded 54 hits over five days) and its memories/list endpoint lacks a date filter.
- Scoping must be done with namespaces supplied as an array in REST; a single namespace provides only a ranking set, and the remember operation stores content, namespace, tags, occurred_at, external_key, external_refs and importance while discarding free metadata, with external_key making the write idempotent (unchanged:true) using the key pattern gaia:kairos:<episode_id>.
- DELETE /api/v1/episodes/:id performs a hard purge that cascades to derived facts, while facts and namespaces cannot be removed via REST (responses 404/405).
- Consolidation reacts within two minutes and yields facts as thin as Hindsight; in a benchmark of twenty Dutch questions, CortexIQ achieved a median response time of 1.061 ms with raw episodes at the top, whereas Hindsight had a median of 1.372 ms with a top list saturated by emotional Gaia memories.
- The spike report is stored at docs/spike-cortexiq.md referencing commits 85dd48e and f329e24, and the remaining migration order is: MemoryStore port, review route on CortexIQ, Gaia‑logos rebuild, bojan/gaia import, and removal of Hindsight.

## Episode 1082

> Migratieplan Hindsight→CortexIQ in Gaia-Cloud vastgesteld op 18-11-2026 (plansessie): volgorde zoals de spike van 16-11 bepaalde (MemoryStore-poort → terugkijk-route → gaia-logos rebuild → bojan/gaia import → Hindsight uit). Twee beslissingen van Bo, beide 'meten gaat voor bouwen': (1) CortexIQ-fase 1 (quota recall over namespaces) wordt pas gebouwd nadat een fixture op de live strato-instance aantoonbaar maakt dat één luide namespace de rest verdringt — geen bewijs, geen bouw; (2) hosting van CortexIQ voor Gaia (strato via Tailscale vs eigen instance op contabo) wordt pas gekozen na een latency-meting contabo→strato, uit te voeren nadat de REST aan de tailnet-kant is gebonden (nu 127.0.0.1:18101). Fase 6 (agent_identity) voorlopig geschrapt: namespace + tag dekt de epistemic mark. Fase 0-huiskeeping: episode 980 (Mindstack, Vite-migratie) opnieuw aanleveren; oude hindsight-backups van 03-05 dienen alleen als formaat-referentie bij de verse export in fase 4.

**A production** (numbers kept 9/10, invented [], gate would reject 0)

- The migration plan from Hindsight to CortexIQ in Gaia-Cloud was established on 18-11-2026.
- The migration steps are ordered as MemoryStore port, backtrack route, gaia-logos rebuild, bojan/gaia import, then Hindsight removal, following the spike of 16-11.
- CortexIQ phase 1 (quota recall over namespaces) will only be built after a fixture on the live strato instance proves that a single loud namespace displaces the rest.
- The hosting option for CortexIQ in Gaia (Strato via Tailscale versus a dedicated Contabo instance) will be chosen after a latency measurement from Contabo to Strato, performed once the REST endpoint is bound to the tailnet side at 127.0.0.1:18101.
- CortexIQ phase 6 (agent_identity) is temporarily removed because a namespace and tag cover the epistemic mark.
- Phase 0 housekeeping involves re‑supplying episode 980 (Mindstack, Vite migration) and using old Hindsight backups from 03-05 solely as a format reference for the new export in phase 4.

**B variant** (numbers kept 9/10, invented [], gate would reject 0)

- The Gaia Cloud migration plan Hindsight→CortexIQ was established on 18-11-2026, following the spike of 16-11 with the order: MemoryStore port → retro route → gaia-logos rebuild → bojan/gaia import → Hindsight removal.
- CortexIQ phase 1 (quota recall over namespaces) will only be built after a fixture on the live strato-instance shows that one loud namespace overtakes the others; without proof, the build will not proceed.
- The hosting choice for CortexIQ in Gaia (Strato via Tailscale versus a dedicated Contabo instance) will be decided after performing a latency measurement from Contabo to Strato, which will occur once the REST endpoint is bound to the tailnet at 127.0.0.1:18101.
- CortexIQ phase 6 (agent_identity) is temporarily removed because the namespace plus tag already cover the epistemic mark.
- CortexIQ phase 0 housekeeping requires re‑supplying episode 980 (Mindstack, Vite migration), and the old Hindsight backups dated 03-05 serve solely as a format reference for the new export in phase 4.

## Episode 558

> Gap-analyse CortexIQ vs Hindsight voor Gaia-Cloud (2026-11): Gaia-Cloud hangt via src/hindsightClient.js aan Hindsight voor: (1) banks + cross-bank recall met per-bank quota (scores niet op één schaal), (2) willekeurige tags per memory-item die in recall-output terugkomen (gaia:* tags, memoryKindOf), (3) versiegelde invalidatie/superseding sync vanuit cognitionSync met reconcile, (4) provenance sources:['foundation:<id>'] voor Dual Recall, (5) async fire-and-forget retain, (6) agent_identity-enforcing. CortexIQ heeft inmiddels wél reflect, mental models, reranker, graph-arm, temporal recall en MMR — die eerdere gaten zijn gedicht.

**A production** (numbers kept 0/7, invented [], gate would reject 0)

- Gaia-Cloud integrates with Hindsight through src/hindsightClient.js to provide banks and cross-bank recall with per-bank quota, random tags per memory-item (gaia:* tags, memoryKindOf), sealed invalidation/superseding sync from cognitionSync with reconcile, provenance sources:['foundation:<id>'] for Dual Recall, asynchronous fire-and-forget retain, and agent identity enforcement.
- CortexIQ now includes reflect, mental models, a reranker, graph-arm, temporal recall and MMR, thereby closing the gaps identified in the gap analysis with Hindsight for Gaia-Cloud.

**B variant** (numbers kept 0/7, invented [], gate would reject 0)

- Gaia-Cloud connects to Hindsight through the src/hindsightClient.js client, providing banks and cross-bank recall with per-bank quota, random tags per memory item in recall output, sealed invalidation/superseding synchronization from cognitionSync with reconcile, provenance sources ['foundation:<id>'] for Dual Recall, asynchronous fire-and-forget retain, and enforcement of agent identity.
- CortexIQ has incorporated reflect, mental models, a reranker, graph-arm, temporal recall, and maximal marginal relevance (MMR), addressing previously identified gaps.

## Episode 36

> Beslissing 2026-10-08 (akkoord Bo): de provenance-prefix voor afgeleide records in Gaia-Cloud gaat van 'chronicle:' naar 'foundation:'. Canonieke id-vorm: 'foundation:<uuid>' met de kale ingest_object uuid (wat Foundation /api/ingest-logs/:id verwacht), niet Foundation's episode-vorm 'ingest:<uuid>'. Reden: 'chronicle:' is legacy (Foundation, niet Chronicle, is de rauwe laag) en is een routing key, geen label. Wordt in één migratie gedaan die ook de strip-bug in kairosRoutes.js fixt en de drie sources-kolommen (hypotheses/patterns/kairos_episodes) plus Hindsight-metadata backfillt; reconcile() wordt uitgebreid met Kairos-episodes. Plan: C:\Users\bojan\.opencode\plan\chronicle-prefix-provenance-plan.md.

**A production** (numbers kept 1/1, invented [], gate would reject 0)

- Gaia-Cloud provenance prefix for derived records was changed from 'chronicle:' to 'foundation:' on 2026-10-08.
- Gaia-Cloud canonical ID format for derived records is now 'foundation:<uuid>' using the raw ingest_object UUID expected by Foundation /api/ingest-logs/:id, not the episode form 'ingest:<uuid>'.
- 'chronicle:' was identified as a legacy routing key rather than a label, prompting its replacement.
- The migration will fix the strip-bug in kairosRoutes.js and backfill the three source columns (hypotheses, patterns, kairos_episodes) as well as Hindsight metadata.
- The reconcile() function is being extended to include Kairos episodes.

**B variant** (numbers kept 0/1, invented [], gate would reject 0)

- Gaia-Cloud provenance prefix for derived records was changed from 'chronicle:' to 'foundation:'; the canonical ID format is 'foundation:<uuid>' using the bare ingest_object UUID as required by Foundation /api/ingest-logs/:id, not the episode form 'ingest:<uuid>'.
- 'chronicle:' is considered legacy because Foundation (not Chronicle) is the raw layer; it serves as a routing key rather than a label.
- The migration will fix the strip-bug in kairosRoutes.js.
- The migration will backfill the three source columns (hypotheses, patterns, kairos_episodes) and Hindsight metadata.
- The reconcile() function will be extended to handle Kairos episodes.
- The plan for the provenance prefix change is documented at C:\Users\bojan\.opencode\plan\chronicle-prefix-provenance-plan.md.

## Episode 37

> Gaia-Cloud: de chronicle:->foundation: provenance-normalisatie is op 2026-10-08 geïmplementeerd (nieuwe seam src/foundationRef.js, synthesizer+kairosRoutes erop, cognitionSync.reconcile() pusht nu ook Kairos-episodes, migratie 010_normalize_sources_prefix.sql). Tests groen: gaia-api 1244/1244, cognition 49/49. Nog open: migratie toepassen op de VPS (npm run migrate in services/cognition) en daarna npm run reconcile:hindsight -- --apply --clear om de Hindsight-metadata te verversen.

**A production** (numbers kept 2/4, invented [], gate would reject 0)

- Gaia-Cloud chronicle foundation implemented provenance normalization on 2026-10-08, adding new seam src/foundationRef.js, integrating synthesizer and kairosRoutes, and extending cognitionSync.reconcile() to push Kairos episodes via migration 010_normalize_sources_prefix.sql.
- Gaia-Cloud test suite reported all green results with gaia-api 1244 of 1244 tests passed and cognition 49 of 49 tests passed.
- Gaia-Cloud still has pending tasks to apply migration on the VPS using npm run migrate in services/cognition and then refresh Hindsight metadata with npm run reconcile:hindsight -- --apply --clear.

**B variant** (numbers kept 2/4, invented [], gate would reject 0)

- Gaia-Cloud foundation deployed provenance normalization on 2026-10-08, introducing a new seam at src/foundationRef.js, adding synthesizer and kairosRoutes, applying migration 010_normalize_sources_prefix.sql, and extending cognitionSync.reconcile() to push Kairos episodes.
- Gaia-Cloud reported all tests green: gaia-api 1244 out of 1244 passed and cognition 49 out of 49 passed.
- Gaia-Cloud VPS still requires applying the migration with npm run migrate in services/cognition followed by npm run reconcile:hindsight -- --apply --clear to refresh Hindsight metadata.

## Episode 38

> Reconcile dry-run Gaia-Cloud 2026-10-08: Hindsight actief gaia:hypothesis=2, gaia:pattern=0, gaia:kairos_episode=0; Cognition hypotheses=4, patterns=0, kairos=0. De foundation:-prefix-backfill bleek een no-op: de oude 'chronicle:'-prefix werd alleen door de Kairos-synthesizer geproduceerd en er zijn geen Kairos-episodes (GAIA_KAIROS_ENABLED staat uit), dus --apply --clear is overgeslagen (zou enkel versie-churn geven). De 4-vs-2 kan deels survey-artefact zijn: open_question/relationship/mental_model dragen andere tags en tellen niet als gaia:hypothesis.

**A production** (numbers kept 4/5, invented [], gate would reject 0)

- Gaia-Cloud dry-run on 2026-10-08 recorded Hindsight active gaia:hypothesis count of 2, gaia:pattern count of 0, and gaia:kairos_episode count of 0.
- Gaia-Cloud dry-run on 2026-10-08 recorded Cognition hypotheses count of 4, patterns count of 0, and kairos count of 0.
- The foundation -prefix-backfill operation was a no-op because the old 'chronicle:' prefix was only produced by the Kairos synthesizer and there were no Kairos episodes (GAIA_KAIROS_ENABLED was off), so the --apply --clear command was skipped.
- During the Gaia-Cloud dry-run on 2026-10-08, the GAIA_KAIROS_ENABLED setting was disabled.
- The difference between Cognition hypotheses (4) and Hindsight gaia:hypothesis (2) may be partially due to a survey artefact where open_question, relationship, and mental_model tags are counted separately and do not contribute to gaia:hypothesis.

**B variant** (numbers kept 4/5, invented [], gate would reject 0)

- During the dry-run reconciliation of Gaia-Cloud on 2026-10-08, Hindsight reported gaia:hypothesis=2, gaia:pattern=0, gaia:kairos_episode=0, while Cognition reported hypotheses=4, patterns=0, kairos=0.
- The foundation -prefix-backfill was a no-op because the old 'chronicle:' prefix is generated only by the Kairos synthesizer and GAIA_KAIROS_ENABLED is disabled, resulting in no Kairos episodes and causing the --apply --clear step to be skipped.
- The observed difference between Cognition hypotheses (4) and Hindsight gaia:hypothesis (2) may be partly due to a survey artifact, as tags open_question, relationship, and mental_model are counted separately and do not contribute to gaia:hypothesis.

## Episode 81

> Gaia-Cloud aanvulling (commit 3f645dd, 2026-10-12): een op het verleden gerichte beurt raadpleegt nu zowel Hindsight als het transcript. recallRelevantContext (memory.js) kreeg een `pastReference`-optie die de recall-gate opent; turn.js geeft die door zodra de gesprekszoek-signalen (exactHistory/pastLookup/rememberedKnowledge) vuren. Reden: Bo's punt 'ze kijkt ook in Hindsight' — de twee gates (memoryPolicy.shouldRecall en de gesprekszoek-signalen) waren onafhankelijk, dus sommige lookup-formuleringen openden alleen het transcript. Let op: Hindsight draait sowieso al elke beurt via de mentale modellen (fetchMentalModelContext), ongeacht de gate. gaia-api 1433/1433 groen (+2).

**A production** (numbers kept 3/4, invented [], gate would reject 0)

- Gaia-Cloud (commit 3f645dd on 2026-10-12) made a past-oriented turn query both Hindsight and the transcript.
- The recallRelevantContext function in memory.js received a `pastReference` option that opens the recall gate.
- turn.js forwards the pastReference option as soon as the conversational search signals exactHistory, pastLookup, or rememberedKnowledge fire.
- Previously the two gates—memoryPolicy.shouldRecall and the conversational search signals—were independent, so some lookup formulations opened only the transcript.
- Hindsight runs every turn through the mental models via fetchMentalModelContext, irrespective of the recall gate.
- gaia-api reported a test status of 1433 passed out of 1433 (green) with a +2 change.

**B variant** (numbers kept 3/4, invented [], gate would reject 0)

- Gaia-Cloud (commit 3f645dd on 2026-10-12) changed past-oriented turns to consult both Hindsight and the transcript.
- The recallRelevantContext function in memory.js received a `pastReference` option that opens the recall gate.
- turn.js forwards the recall gate once conversation search signals (exactHistory, pastLookup, rememberedKnowledge) fire.
- The memoryPolicy.shouldRecall gate was independent from the conversation search signals, causing some lookup formulations to open only the transcript.
- Hindsight runs every turn through the mental models using fetchMentalModelContext, independent of the recall gate.
- gaia-api reported 1433 passed tests out of 1433, marked green with a +2 score.

## Episode 86

> Gaia-Cloud (2026-10-12, commit 8e26794, gedeployed): (1) recall-staart gemeten uit Hindsight's Prometheus-histogram (hindsight_http_duration_seconds, recall-endpoint): 27 recalls, mean 894ms, p50 ~1s, p90/p95 ~2,5s, p99 ~5s; 1 call (3,7%) boven 4s, 0 boven 8s. Kleine, niet-representatieve steekproef (grotendeels test-calls), buckets grof. (2) Observability: elke gefaalde Hindsight-call logt nu een content-loze regel {kind:'hindsight.call_failed', bank, op, reason:'timeout'|'unreachable', timeoutMs, ms} en throwt door — via een request()-wrapper rond fetchImpl in src/hindsightClient.js (alle 14 call-sites). Reden: memory.js slikte Hindsight-fouten stil in (best-effort -> []), dus missende context was onzichtbaar. Test test/hindsightCallLog.test.js (+3), gaia-api 1436/1436. Het is puur een logregel, geen gedragsverandering; verschijnt in de gaia-api containerlog.

**A production** (numbers kept 7/18, invented [95th 2.5 99th 3.7], gate would reject 0)

- Gaia-Cloud measured recall latency from Hindsight's Prometheus histogram (hindsight_http_duration_seconds, recall-endpoint) with 27 recalls averaging 894 ms, median around 1 s, 90‑95th percentiles near 2.5 s, 99th percentile about 5 s, and one call (3.7 %) exceeding 4 s while none exceeded 8 s.
- Gaia-Cloud now logs every failed Hindsight call as an empty JSON line with kind 'hindsight.call_failed' and relevant fields, then rethrows the error through the request() wrapper around fetchImpl in src/hindsightClient.js across all fourteen call sites; the log entries appear in the gaia-api container log and no other behavior changed.

**B variant** (numbers kept 11/18, invented [2.5 3.7], gate would reject 0)

- Gaia-Cloud measured recall latency from Hindsight's Prometheus histogram (hindsight_http_duration_seconds, recall-endpoint) showing 27 recalls with mean 894 ms, p50 ~1 s, p90/p95 ~2.5 s, p99 ~5 s, and 1 call (3.7 %) exceeding 4 s and none exceeding 8 s; the sample was small, non‑representative and comprised mostly test calls with coarse buckets.
- Gaia-Cloud updated observability so that every failed Hindsight call now logs an empty‑content JSON entry {kind:'hindsight.call_failed', bank, op, reason:'timeout'|'unreachable', timeoutMs, ms} and is re‑thrown via a request() wrapper around fetchImpl in src/hindsightClient.js covering all 14 call‑sites; this change adds logging without altering behavior and appears in the gaia-api container log, with tests added in test/hindsightCallLog.test.js (+3) achieving gaia-api coverage 1436/1436.

## Episode 560

> Beslissing: LongMemEval-harnas is uitgeschreven als docs/design-longmemeval.md in cortexiq (commit eb37c41, gepusht naar main 2026-11-02). Belangrijkste bevinding: internal/benchmark bestaat al en is eerlijk (ingest als episodes, antwoorden via reflect, beide judges) — het gat is de officiële dataset + fairness-protocol + publicatie. Kernregels: judge is een ander/sterker model dan het antwoordmodel, abstention krijgt een speciale judge-regel, geen benchmark-specifieke tuning, alleen de S-variant draaien flattert consolidatie (dus ook de M/day-variant), Hindsight-cijfers worden gequoteerd met bron als hun meting.

**A production** (numbers kept 2/2, invented [], gate would reject 0)

- LongMemEval-harnas was documented in the file docs/design-longmemeval.md in the CortexIQ repository, recorded at commit eb37c41 and pushed to the main branch on 2026-11-02.
- The internal/benchmark component already exists and is considered fair, ingesting data as episodes, providing answers via reflect, and using both judges.
- LongMemEval-harnas lacks an official dataset, a fairness protocol, and a publication.
- LongMemEval-harnas core rules require the judge to be a different or stronger model than the answer model, assign a special judge rule for abstention, and prohibit benchmark-specific tuning.
- Running only the S-variant, and also the M/day-variant, flattens consolidation in LongMemEval-harnas.
- Hindsight numbers in LongMemEval-harnas are quoted with the source indicated as their measurement.

**B variant** (numbers kept 2/2, invented [], gate would reject 0)

- LongMemEval harness was documented as docs/design-longmemeval.md in CortexIQ, with commit eb37c41 pushed to main on 2026-11-02.
- internal/benchmark already exists and is considered fair, ingesting data as episodes and providing answers via reflect with both judges.
- The LongMemEval project lacks an official dataset, a fairness protocol, and a publication.
- In LongMemEval, the judge must be a different or stronger model than the answer model.
- LongMemEval specifies that abstention is handled by a special judge rule.
- LongMemEval states that running only the S-variant flattens consolidation (including the M/day-variant) and that Hindsight numbers are quoted with their measurement source.

## Episode 364

> Gaia Cloud now retains eligible Hindsight turns as a Hermes-style JSON array of role-labelled User and Assistant messages. Existing memoryworthiness/triviality gates, asynchronous fire-and-forget behavior, and identity/provenance metadata remain; no historical Hindsight data is rewritten. Deployed as commit bddbad5 on 2026-10-23.

**A production** (numbers kept 1/2, invented [], gate would reject 0)

- Gaia Cloud stores eligible Hindsight turns in a Hermes‑style JSON array where each entry is labelled as either User or Assistant.
- Gaia Cloud keeps its existing memory‑worthiness and triviality gates, asynchronous fire‑and‑forget processing, and identity/provenance metadata unchanged.
- Gaia Cloud does not rewrite any historical Hindsight data after the update.
- The update to Gaia Cloud was deployed using commit bddbad5 on 23 October 2026.

**B variant** (numbers kept 1/2, invented [], gate would reject 1)

- Gaia Cloud now retains eligible Hindsight turns as a Hermes-style JSON array of role‑labelled User and Assistant messages.
- Gaia Cloud's existing memoryworthiness/triviality gates, asynchronous fire-and-forget behavior, and identity/provenance metadata remain unchanged.
- Gaia Cloud does not rewrite historical Hindsight data.
- Gaia Cloud was deployed with commit bddbad5 on 2026‑10‑23.

## Episode 859

> CortexIQ BM25-repetitie op de productiedump (2026-11-11): CI op 3774ea9 groen. Dagelijkse prod-dump (47 MB, checksum OK; prod = 2.944 facts, 728 episodes, 128 MB, goose 60, bge-m3/dim 1024) in verse PG17-container met deploy/postgres-image: pg_restore 5 s, db.Open inclusief migratie 00061 335 ms, bm25-indexen aanwezig, tsvector-kolommen weg. Kwaliteit: BM25 wint bij inspectie van 3 van 15 queries (IDF: zeldzame tokens als 8445 en bge-m3 komen bovenaan, ts_rank_cd gaf generieke woorden); geen gelabelde eval gedraaid, vervolgstap is cortexiq eval recall. Snelheidswinst is op dit kleine corpus nog theoretisch. VPS-toegang was read-only (backuplijst, scp van dump); dump en container na afloop verwijderd. OPEN: prod nog op PG16 en niet geüpgraded; release-job voor cortexiq-postgres en arm64 nooit gedraaid; Bo moet de ssh- en scp-allow-regels en de oude gh-release-create-regel uit .claude/settings.local.json halen (bestand is in git gevolgd, niet committen). Commit van de docs: zie git log.

**A production** (numbers kept 17/21, invented [], gate would reject 0)

- On 2026-11-11 the CortexIQ continuous integration on commit 3774ea9 reported a green status.
- A daily CortexIQ production dump of 47 MB with checksum OK contained 2.944 facts, 728 episodes, 128 MB total size, goose 60 and bge‑m3 vectors of dimension 1024; it was loaded into a fresh PostgreSQL 17 container using the deploy/postgres-image, pg_restore completed in 5 seconds, db.Open with migration 00061 took 335 ms, bm25 indexes were present and tsvector columns were removed.
- During inspection BM25 won on 3 of 15 queries; IDF ranked rare tokens such as 8445 and bge‑m3 highest, while ts_rank_cd favored generic words.
- No labelled evaluation was executed and the next planned step is the CortexIQ recall evaluation.
- VPS access for CortexIQ was read‑only; after use the dump and container were deleted.
- The CortexIQ production database remained on PostgreSQL 16 and was not upgraded; the release job for cortexiq‑postgres and arm64 had never run, and Bo was instructed to remove the ssh and scp allow rules and the old gh‑release‑create rule from .claude/settings.local.json.

**B variant** (numbers kept 13/21, invented [], gate would reject 0)

- CortexIQ CI reported a green status for commit 3774ea9.
- CortexIQ performed a daily production dump of 47 MB (checksum OK) containing 2.944 facts, 728 episodes, and other data, processed in a fresh PG17 container where pg_restore completed in 5 seconds, db.Open with migration 00061 in 335 ms, BM25 indexes were present and tsvector columns were absent.
- CortexIQ BM25 retrieval won the inspection for 3 out of 15 queries, with rare tokens such as 8445 and bge-m3 ranking highest by IDF while ts_rank_cd favored generic words.
- CortexIQ had not run a labeled evaluation, and the next planned step is to execute cortexiq eval recall.
- CortexIQ VPS access was read-only, allowing only backup list and scp of the dump, and the dump and container were deleted after use.
- CortexIQ production database remained on PostgreSQL 16 and was not upgraded, the release job for cortexiq‑postgres and arm64 had never run, and Bo is required to delete the ssh and scp‑allow rules and the old gh‑release‑create rule from .claude/settings.local.json.

## Episode 759

> De productiebug supported_by is lokaal opgelost: de background consolidator selecteerde hypotheses zonder supported_by voor pgx named scanning, en positional hypothesis/counter scans misten het veld. Drie bestanden zijn aangepast, walkthrough bijgewerkt, go test ./internal/brain en go test ./... zijn groen. Niet gedeployed; aparte expliciete goedkeuring vereist.

**A production** (numbers kept 0/0, invented [], gate would reject 0)

- CortexIQ production bug 'supported_by' was locally resolved; the background consolidator had selected hypotheses without supported_by for pgx named scanning, and positional hypothesis/counter scans missed the field.
- CortexIQ had three files modified, the walkthrough documentation was updated, and the commands go test ./internal/brain and go test ./... reported all tests as green.
- CortexIQ was not deployed after the fix and requires a separate explicit approval before deployment.

**B variant** (numbers kept 0/0, invented [], gate would reject 0)

- In CortexIQ, the background consolidator selected hypotheses lacking the supported_by field during pgx named scanning, and positional hypothesis/counter scans missed that field, resolving the production bug.
- CortexIQ had three source files modified and its walkthrough documentation updated.
- CortexIQ's go test commands for ./internal/brain and all packages returned green, indicating all tests passed.
- The changes in CortexIQ have not been deployed and require a separate explicit approval before deployment.

## Episode 864

> CortexIQ migratie-race bij opstarten gerepareerd en gepusht (2026-11-11): internal/db/db.go draait de hele schema-opzet (goose.Up, dimensie-ALTER, HNSW-indexen) nu in setupSchema onder withSchemaLock, een sessie-advisory lock (pg_advisory_lock, sleutel 994294737458 = 'cortexiq' in base 36) op een eigen verbinding, wachttijd begrensd op 5 minuten, bij annuleren wordt de sessie gesloten zodat er geen lock achterblijft; Open sluit de pool nu bij elke fout (lekte bij goose-fout). Regressietests internal/db/schemalock_integration_test.go (6 gelijktijdige Open's op een lege database, wederzijdse uitsluiting, annuleerbare wacht); mutatiecheck: zonder lock falen 5 van 6 starts met dezelfde 42P07-fout als in prod. Volledige suite groen op verse PG17-db: 21 pakketten, 153 integratietests, 0 skip. NOG NIET UITGEROLD: productie draait v0.1.74 zonder dit slot; het zit in de volgende release. Open: Bo moet de ssh-allow-regels uit .claude/settings.local.json halen (de uitrol is klaar) en de Jina-sleutel roteren; compose op de VPS wijst nog naar de lokale image cortexiq-postgres:17-bm25 (ghcr-image ...-postgres:0.1.74 bestaat).

**A production** (numbers kept 9/12, invented [], gate would reject 0)

- In CortexIQ, internal/db/db.go was changed to execute the entire schema setup (goose.Up, dimension-ALTER, HNSW indexes) within setupSchema under withSchemaLock, employing a session advisory lock (pg_advisory_lock key 994294737458 = 'cortexiq' base 36) on a dedicated connection, with a 5‑minute wait timeout and automatic session close on cancellation.
- CortexIQ’s Open component now closes its connection pool on any error, addressing a leak that occurred on goose failures.
- The integration test schemalock_integration_test.go for CortexIQ runs six simultaneous Open calls on a fresh database, confirming exclusive locking and cancellable waiting, and demonstrates that omitting the lock causes five of six starts to fail with error 42P07.
- When run against a fresh PostgreSQL 17 database, the complete CortexIQ test suite reports 21 packages, 153 integration tests, no skips, and all tests pass.
- CortexIQ production is currently operating version v0.1.74, which does not include the newly added schema lock; the lock is scheduled for the upcoming release.
- Bo is assigned to delete the ssh-allow rules from .claude/settings.local.json, rotate the Jina key, and address that the VPS compose configuration still references the local cortexiq-postgres:17-bm25 image, while the corresponding ghcr image for version 0.1.74 is available.

**B variant** (numbers kept 8/12, invented [], gate would reject 0)

- CortexIQ's internal/db/db.go executes the full schema setup (goose.Up, dimension ALTER, HNSW indexes) inside the setupSchema function protected by withSchemaLock.
- CortexIQ uses a session advisory lock via pg_advisory_lock with key 994294737458 (base‑36 'cortexiq') on a dedicated connection, limiting wait time to 5 minutes and closing the session on cancellation to prevent leftover locks.
- CortexIQ's Open now closes the connection pool on each error, fixing a prior leak that occurred on goose migration errors.
- CortexIQ's regression test internal/db/schemalock_integration_test.go runs six simultaneous Open calls on an empty database to verify mutual exclusion and cancelable waiting, and confirms that omitting the lock causes five of six starts to fail with the 42P07 error observed in production.
- CortexIQ's complete test suite ran on a fresh PostgreSQL 17 database, covering 21 packages and 153 integration tests with zero skips, and all tests passed.
- CortexIQ production is currently running version v0.1.74 without the new schema lock, which is slated to be included in the upcoming release.

## Episode 169

> CortexIQ prod (strato-vps, /opt/cortexiq) is op 2026-10-15 uitgerold naar commit 6377b4c: POST /api/v1/remember is live (401 zonder token, 201 met token) en de embedder wordt bij start op de achtergrond warm geladen (~1,5 s, logregel msg="embedder warm"); warme recall meet ~5 ms in plaats van de oude koude start van ~1,8 s. Goose-versie bleef 27.

**A production** (numbers kept 9/9, invented [1.5 1.8], gate would reject 0)

- CortexIQ prod running on strato-vps at /opt/cortexiq was deployed on 2026-10-15 to commit 6377b4c.
- The POST /api/v1/remember endpoint in CortexIQ prod is live and returns HTTP 401 when called without a token and HTTP 201 when called with a token.
- At start, the embedder in CortexIQ prod is warm‑loaded in the background taking approximately 1.5 seconds, emitting a log line with msg="embedder warm".
- Warm recall in CortexIQ prod measures about 5 ms, improving from the previous cold‑start latency of roughly 1.8 seconds.
- The Goose component of CortexIQ prod remained at version 27.

**B variant** (numbers kept 9/9, invented [1.5 1.8], gate would reject 0)

- CortexIQ prod (running on strato-vps at /opt/cortexiq) was deployed on 2026-10-15 to commit 6377b4c.
- CortexIQ POST /api/v1/remember endpoint is live and returns HTTP 401 when called without a token and HTTP 201 when called with a token.
- CortexIQ embedder is warm‑loaded in the background at start, taking approximately 1.5 seconds, as indicated by log line msg="embedder warm".
- CortexIQ warm recall latency is measured at about 5 milliseconds, compared to the previous cold start latency of approximately 1.8 seconds.
- Goose version remained at 27.

## Episode 697

> CortexIQ: RRF-sweep gedraaid op de productie-embedder (bge-m3, hybride, Jina-reranker; 60 memories, 36 vragen, k=10) via docker compose run met een override naar een wegwerpdatabase cortexiq_eval (productie niet geraakt, daarna verwijderd). Uitkomst overall: rrf-k 20 hit@1 0.75 MRR 0.850; rrf-k 60 hit@1 0.78 MRR 0.868; rrf-k 100 identiek aan 60. Verschil is één à twee vragen van 36, dus ruis. Besluit: CORTEXIQ_RRF_K blijft 60. Alle varianten missen dezelfde acht vragen op ongeveer dezelfde plaats, dus een betere recall zit in embedder/reranker, niet in de fusieconstante. Recept voor een volgende keer: op de server in /opt/cortexiq een override met CORTEXIQ_POSTGRES_DSN naar een extra database, dan docker compose -f docker-compose.prod.yml -f override run --rm --no-deps -T -v /opt/cortexiq/eval:/eval:ro cortexiq eval recall --rrf-k ... --corpus /eval/recall_corpus.jsonl --queries /eval/recall_queries.jsonl. Les: gebruik bij .env-greps nooit een patroon dat KEY-variabelen raakt (RERANK matchte CORTEXIQ_RERANKER_API_KEY en de Jina-sleutel kwam in het gesprek; Bo is aangeraden hem te roteren).

**A production** (numbers kept 11/11, invented [], gate would reject 0)

- CortexIQ executed an RRF sweep on the production embedder (bge-m3, hybrid, Jina-reranker) with 60 memories and 36 queries (k=10) via docker compose run, overriding to a disposable database named cortexiq_eval, ensuring production was not impacted and the temporary database was removed afterwards.
- CortexIQ observed RRF sweep outcomes: for rrf‑k 20 hit@1 0.75 and MRR 0.850; for rrf‑k 60 hit@1 0.78 and MRR 0.868; for rrf‑k 100 results identical to rrf‑k 60, with only one or two questions differing out of 36, considered noise.
- CortexIQ decided to keep the configuration parameter CORTEXIQ_RRF_K set to 60.
- CortexIQ determined that all evaluated variants missed the same eight questions in similar positions, indicating that better recall would stem from improvements to the embedder or reranker instead of adjusting the fusion constant.
- CortexIQ recommended for future evaluations to place an override setting CORTEXIQ_POSTGRES_DSN pointing to an additional database in /opt/cortexiq, and execute docker compose with production and override files, mounting /opt/cortexiq/eval to /eval in read‑only mode, then run the cortexiq eval recall command with appropriate parameters.
- CortexIQ advised never to use .env grep patterns that touch KEY variables, noting that a grep for RERANK matched CORTEXIQ_RERANKER_API_KEY and the Jina key surfaced in conversation, and recommended rotating the key.

**B variant** (numbers kept 11/11, invented [], gate would reject 0)

- CortexIQ performed an RRF-sweep on the production embedder (bge-m3, hybrid, Jina‑reranker) using 60 memories, 36 vragen, k=10 through a docker‑compose run that overrode the database to a throwaway cortexiq_eval instance, leaving the production database unaffected.
- CortexIQ measured recall performance showing hit@1 0.75 and MRR 0.850 at rrf‑k 20, hit@1 0.78 and MRR 0.868 at rrf‑k 60, with rrf‑k 100 yielding the same results as rrf‑k 60.
- CortexIQ decided to retain the configuration CORTEXIQ_RRF_K at the value 60.
- CortexIQ observed that every variant failed on the same eight vragen, suggesting that improvements in recall depend on the embedder or reranker rather than on adjusting the fusion constant.
- CortexIQ recommends for future evaluations to create an override pointing CORTEXIQ_POSTGRES_DSN to an additional database in /opt/cortexiq, and execute docker‑compose with the production and override files, mounting /opt/cortexiq/eval as read‑only, and running the cortexiq eval recall command with appropriate corpus and query files.
- CortexIQ learned to never use .env glob patterns that interfere with key variables—including CORTEXIQ_RERANKER_API_KEY—and to rotate the Jina API key as recommended.

## Episode 888

> CortexIQ: v0.1.80 (Timeline-vergelijking tussen twee data, commit 6f725cd, CI groen) is door Claude gepubliceerd en de Release-workflow is groen (2026-11-13). De uitrol naar productie is NIET uitgevoerd: de auto-mode classifier blokkeerde het ssh-commando voor de deploy. Productie draait vermoedelijk nog v0.1.79 (niet geverifieerd). Bo moet de deploy toestaan of zelf draaien. Let op: vaste toestemming van Bo gold voor het publiceren van releases; uitrol via ssh valt buiten wat de classifier toestaat zonder allow-regel in .claude/settings.local.json.

**A production** (numbers kept 4/4, invented [], gate would reject 0)

- CortexIQ version v0.1.80 (commit 6f725cd, feature "Timeline-vergelijking tussen twee data") was published by Claude on 2026-11-13 and the release workflow showed a green CI status.
- The auto-mode classifier blocked the ssh command required for the production deployment of CortexIQ.
- The deployment of CortexIQ to production was not executed.
- The production environment is presumed to still be running CortexIQ version v0.1.79, though this has not been verified.
- Bo must either allow the deployment or execute it manually.
- Bo has a fixed permission that only covers publishing releases; deployment via ssh is outside the classifier's allowed actions without an allow rule in .claude/settings.local.json.

**B variant** (numbers kept 4/4, invented [], gate would reject 0)

- CortexIQ version v0.1.80 (commit 6f725cd) was published by Claude on 2026-11-13 with CI green.
- CortexIQ release workflow was green on 2026-11-13.
- The auto-mode classifier blocked the ssh command for the deployment, preventing rollout to production.
- CortexIQ production is presumed to be still running version v0.1.79, though this has not been verified.
- Bo must allow the deployment or execute it manually.
- Deployment via ssh is not permitted by the classifier without an allow rule in .claude/settings.local.json, despite Bo's fixed permission for publishing releases.

## Episode 412

> CortexIQ-productie (strato-vps, /opt/cortexiq) draait sinds 2026-10-27 v0.1.6 (commit c107f8c, image sha256:61d7ed372332). Deployprocedure: GitHub-release publiceren (release-workflow bouwt binaries en image), CORTEXIQ_IMAGE_TAG in /opt/cortexiq/.env bumpen, daarna docker compose -f docker-compose.prod.yml pull en up -d. De UI is een aparte service (cortexiq-ui) op dezelfde image; UI luistert op 127.0.0.1:18101, API op 18100, beide eisen een token.

**A production** (numbers kept 6/6, invented [127.0.0.1:18100], gate would reject 0)

- CortexIQ-productie on a Strato VPS at /opt/cortexiq has been running since 2026-10-27 with version v0.1.6 (commit c107f8c) using image sha256:61d7ed372332.
- The deployment procedure for CortexIQ-productie uses a GitHub release publish workflow that builds binaries and a Docker image, updates CORTEXIQ_IMAGE_TAG in /opt/cortexiq/.env, and then executes docker compose -f docker-compose.prod.yml pull and up -d.
- The CortexIQ UI runs as a separate service named cortexiq-ui on the same Docker image; the UI listens on 127.0.0.1:18101, the API on 127.0.0.1:18100, and both require an authentication token.

**B variant** (numbers kept 6/6, invented [127.0.0.1:18100], gate would reject 0)

- CortexIQ production has been running on a Strato VPS located at /opt/cortexiq since 2026-10-27 with version v0.1.6, commit c107f8c and image sha256:61d7ed372332.
- CortexIQ is deployed by publishing a GitHub release, building binaries and a container image through a release workflow, updating CORTEXIQ_IMAGE_TAG in /opt/cortexiq/.env, and executing docker compose -f docker-compose.prod.yml pull and up -d.
- The cortexiq-ui service runs as a separate component on the same container image, listening on 127.0.0.1:18101, while the CortexIQ API listens on 127.0.0.1:18100, and both endpoints require a token.

## Episode 886

> CortexIQ werkwijze (2026-11-13): Bo geeft vaste toestemming dat Claude zelf GitHub-releases publiceert (gh release create <versie> --target main) zodra CI op main groen is; Bo hoeft het niet meer zelf te doen. Daarbij altijd versienummer en releasenotities melden in het antwoord. Uitrol naar productie volgt de vaste procedure (back-up .env, pull terwijl het draait, verified dump, tag bumpen, healthz/readyz/restarts/logs controleren). Eerder deed Bo het publiceren zelf vanaf v0.1.74; die gewoonte is nu vervangen.

**A production** (numbers kept 1/2, invented [], gate would reject 0)

- Claude was given permanent permission by Bo to publish GitHub releases using the command `gh release create <versie> --target main` as soon as continuous integration on the main branch is green.
- Claude must always include the version number and release notes when publishing a GitHub release.
- CortexIQ production deployment follows a fixed procedure consisting of backing up the .env file, pulling changes while the service is running, performing a verified dump, bumping the tag, and checking healthz, readyz, restarts, and logs.
- Bo previously published releases manually beginning with version v0.1.74, a habit that has now been replaced.
- Bo no longer has to publish releases manually because Claude now handles the publishing.

**B variant** (numbers kept 1/2, invented [], gate would reject 0)

- Claude was given fixed permission by Bo to automatically publish GitHub releases with the command gh release create <versie> --target main as soon as continuous integration on the main branch reports a green status.
- Bo no longer needs to manually publish releases himself after granting Claude the automated publishing permission.
- When publishing a GitHub release, Claude must always report the version number and release notes in its response.
- CortexIQ’s deployment to production follows a fixed procedure that includes backing up the .env file, pulling code while the service is running, performing a verified dump, bumping the tag, and checking healthz/readyz endpoints, restarts, and logs.
- Previously, Bo manually published releases himself beginning with version v0.1.74, but this practice has now been replaced by Claude’s automated publishing.

## Episode 670

> CortexIQ: v0.1.20 (e56a03b) is op 2026-11-05 gedeployed naar strato-vps; v0.1.19 is wel gebouwd maar nooit uitgerold, productie ging direct van 0.1.18 naar 0.1.20. Bevat migraties 00044 (judged), 00045 (origin/origin_ref) en 00046 (strength), de rebuild-CLI en CORTEXIQ_RRF_K. Volgorde: geverifieerde dump backups/cortexiq-20261105T002405Z.dump, release, .env gebackupt als .env.bak-pre0.1.20, tag-bump en up -d. Gecontroleerd: version v0.1.20, goose 46, containers Up, geen panic of migratiefout, 2330 facts allemaal zonder origin (oude facts, bewust niet teruggevuld). Een read-only rebuild-rapport van /projects/cortexiq in productie: 114 episodes, 476 facts, 6 facts op ingetrokken bronnen (gat C). CORTEXIQ_DECAY_SPLIT staat NIET aan en docker-compose.prod.yml op de server geeft de variabele nog niet door. Open voor Bo: de ssh- en gh-release-allow-regels uit de projectsettings halen; beslissen over DECAY_SPLIT; zelf de RRF-sweep draaien (cortexiq eval recall --rrf-k 20,60,100).

**A production** (numbers kept 16/17, invented [], gate would reject 0)

- CortexIQ version v0.1.20 (commit e56a03b) was deployed on 2026-11-05 to the strato‑vps server, while version v0.1.19 was built but never rolled out, resulting in production moving directly from v0.1.18 to v0.1.20.
- The CortexIQ release contains migrations 00044 (judged), 00045 (origin/origin_ref) and 00046 (strength), as well as the rebuild‑CLI and the CORTEXIQ_RRF_K component.
- After deployment, CortexIQ was verified to be running version v0.1.20 with Goose 46, all containers were Up, there were no panics or migration errors, and 2330 facts were present, all lacking an origin field.
- A read‑only rebuild report for CortexIQ in production at /projects/cortexiq showed 114 episodes, 476 facts, and 6 facts referencing retracted sources (gap C).
- CORTEXIQ_DECAY_SPLIT is not enabled and the server's docker‑compose.prod.yml does not pass this variable.
- Open tasks for Bo include retrieving the SSH and GitHub release allow rules from the project settings, deciding whether to enable DECAY_SPLIT, and executing the RRF‑sweep using the command "cortexiq eval recall --rrf-k 20,60,100".

**B variant** (numbers kept 15/17, invented [], gate would reject 0)

- CortexIQ version v0.1.20 (commit e56a03b) was deployed on 2026-11-05 to the strato-vps server; version v0.1.19 was built but never released, and production moved directly from version 0.1.18 to version 0.1.20.
- CortexIQ includes migration scripts 00044 (judged), 00045 (origin/origin_ref), and 00046 (strength), as well as the rebuild-CLI and the CORTEXIQ_RRF_K component.
- The deployment of CortexIQ followed the sequence: verified dump at backups/cortexiq-20261105T002405Z.dump, performed release, backed up the .env file as .env.bak-pre0.1.20, executed a tag bump, and ran up -d.
- After deployment, CortexIQ was observed with version v0.1.20, goose migration version 46, containers running, no panic or migration errors, and a total of 2330 facts all lacking an origin.
- A read‑only rebuild report for CortexIQ in production at /projects/cortexiq showed 114 episodes, 476 facts, and 6 facts on withdrawn sources (gap C).
- CortexIQ's CORTEXIQ_DECAY_SPLIT setting was observed as disabled, and the server's docker-compose.prod.yml did not yet propagate the variable.

## Episode 924

> CortexIQ besluit (2026-11-13, Bo): Jina (jina-reranker-v3.5) blijft de reranker in productie; geen eigen reranker op de VPS. Reden: lokaal (llama.cpp, bge-reranker-v2-m3) is ongeveer 2,9 s trager per recall (mogelijk 3-5 s op de VPS, gedeeld met andere workloads) en iets slechter (MRR 0,80 tegen 0,85; zie id 923). Niets gewijzigd in productie. Het risico dat Jina wegvalt (saldo, 429-limiet van 100.000 tokens per minuut, 404) wordt afgedekt door de metric recall_rerank_requests_total, de logregel per minuut, RERANK_CANDIDATES=30 en de graaf-routing als vangnet (gewogen is dan beter dan uit). Niet gedaan en niet nodig tenzij het pijn doet: fallback-keten Jina dan lokaal (codewijziging), meten met 10 kandidaten, meten op de VPS zelf.

**A production** (numbers kept 11/15, invented [2.9 0.80 0.85], gate would reject 0)

- CortexIQ decided that Jina (jina-reranker-v3.5) remains the reranker in production and no own reranker is deployed on the VPS.
- CortexIQ observed that local rerankers (llama.cpp, bge-reranker-v2-m3) are about 2.9 seconds slower per recall (possibly 3–5 seconds on the VPS) and have lower MRR (0.80 versus 0.85) according to id 923.
- CortexIQ reported that no changes were made to the production system.
- CortexIQ mitigates the risk of Jina becoming unavailable (due to balance, 429 limit of 100,000 tokens per minute, or 404 errors) using the recall_rerank_requests_total metric, a per‑minute log entry, RERANK_CANDIDATES set to 30, and graph‑routing as a fallback.
- CortexIQ has not implemented and does not need a fallback chain that switches to a local reranker after Jina, which would involve code changes and measuring with 10 candidates on the VPS.

**B variant** (numbers kept 11/15, invented [2.9 0.80 0.85], gate would reject 0)

- CortexIQ kept Jina (jina-reranker-v3.5) as the reranker in production, without deploying an own reranker on the VPS.
- CortexIQ determined that the local reranker using llama.cpp and bge-reranker-v2-m3 is about 2.9 seconds slower per recall (estimated 3–5 seconds on the VPS) and achieves a lower mean reciprocal rank (0.80) compared with Jina’s 0.85 (see id 923).
- CortexIQ reported that no changes were made to the production configuration.
- CortexIQ mitigates the risk of Jina becoming unavailable (due to balance, 429 token‑limit of 100 000 per minute, or 404 errors) using the recall_rerank_requests_total metric, a per‑minute log entry, RERANK_CANDIDATES set to 30, and weighted graph routing as a fallback.
- CortexIQ has not implemented a fallback chain that would route to a local reranker after Jina, and considers it unnecessary unless problems arise.
- CortexIQ intends to evaluate the fallback chain by measuring performance with 10 candidates directly on the VPS.

## Episode 664

> CortexIQ: v0.1.18 (afgewezen-waarde-tombstone, migratie 00043, Reflect-tab #7) is op 2026-11-05 gedeployed naar strato-vps. Volgorde: geverifieerde dump (backups/cortexiq-20261104T232239Z.dump), release gepubliceerd door Bo (de auto-mode classifier weigerde gh release create van mij), .env gebackupt als .env.bak-pre0.1.18, tag-bump en up -d. Gecontroleerd: version v0.1.18 (6c89dd3), goose 43, facts_refused in de logs. Productie had 0 retracties, dus de backfill deed niets. Het Gemma-parsefoutje bij causal links in /projects/chronicle is het bekende lijstprobleem, niet nieuw. Praktisch: SSH naar strato-vps werkt via een allow-regel in de projectsettings; die hoort na de deploy weg.

**A production** (numbers kept 5/8, invented [], gate would reject 0)

- CortexIQ version v0.1.18 was deployed to the strato-vps server on 2026-11-05.
- The deployment of CortexIQ v0.1.18 included a verified dump, a release published by Bo, an .env backup as .env.bak-pre0.1.18, a tag bump, and the execution of up -d.
- After deploying CortexIQ v0.1.18, the system reported version 6c89dd3, Goose migration 43, and logged facts_refused entries.
- CortexIQ production reported zero retractions, so the backfill operation performed no changes.
- The Gemma parse error affecting causal links in /projects/chronicle was identified as a known list problem and is not a new issue.
- SSH access to the CortexIQ strato-vps server functions through an allow‑rule configured in the project settings, which is intended to be removed after the deployment.

**B variant** (numbers kept 6/8, invented [], gate would reject 0)

- CortexIQ version v0.1.18 was deployed to the strato-vps server on 2026-11-05.
- CortexIQ deployment sequence included a verified dump (backups/cortexiq-20261104T232239Z.dump), a release published by Bo (the auto-mode classifier refused the gh release create command), a .env backup saved as .env.bak-pre0.1.18, and a tag bump followed by an up -d command.
- CortexIQ deployment verification confirmed version v0.1.18 (commit 6c89dd3), goose version 43, and recorded facts_refused entries in the logs.
- CortexIQ production recorded zero retractions, so the backfill operation made no changes.
- The Gemma parser error affecting causal links in /projects/chronicle is identified as the known list problem and is not new.
- SSH access to the strato-vps server functions via an allow rule defined in the project settings, which should be removed after the deployment.

---

episodes 21 | A: 102 facts, numbers kept 129/171, invented 10 | B: 102 facts, numbers kept 128/171, invented 8, would be rejected by the production gate 1
