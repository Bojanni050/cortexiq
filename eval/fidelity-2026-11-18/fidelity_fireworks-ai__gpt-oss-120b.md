# Fact fidelity: fireworks-ai/gpt-oss-120b

A = production prompt and gate. B = rule replaced, no gate. Read the facts against the episode: subject and object right? detail kept? anything added?

## Episode 989

> Spike 16-11-2026: CortexIQ 0.1.88 (strato-vps, REST 127.0.0.1:18101) vergeleken met Hindsight 0.9.2 (contabo, 8888) als vervanger van Hindsight in Gaia. Beslissend verschil: CortexIQ filtert op occurred_at met from/to (half-open) en herkent Nederlandse datumzinnen over REST, terwijl Hindsight temporal_window alleen rankt (een venster van 1 dag gaf 54 hits over vijf dagen) en memories/list geen datumfilter heeft. Scoping moet met namespaces (array in REST); de enkelvoudige namespace is alleen een ranking-zetje. remember bewaart content, namespace, tags, occurred_at, external_key, external_refs en importance, maar gooit vrije metadata weg; external_key maakt de write idempotent (unchanged:true, andere inhoud geweigerd tenzij supersedes), dus gaia:kairos:<episode_id> is de spiegel-sleutel. DELETE /api/v1/episodes/:id purge hard en cascadeert naar afgeleide facts; facts en namespaces zijn niet los te verwijderen via REST (404/405). Consolidatie reageert binnen twee minuten maar levert even dunne facts als Hindsight. Op twintig Nederlandse vragen: CortexIQ mediaan 1.061 ms met ruwe episodes bovenaan, Hindsight 1.372 ms met een door emotionele gaia-herinneringen verzadigde top. Rapport: docs/spike-cortexiq.md (commits 85dd48e en f329e24). Volgorde blijft: MemoryStore-poort, terugkijk-route op CortexIQ, gaia-logos rebuilden, bojan/gaia importeren, Hindsight uit.

**A production** (numbers kept 7/13, invented [], gate would reject 0)

- CortexIQ version 0.1.88 was deployed on a Strato VPS and exposed via a REST API at 127.0.0.1:18101.
- Hindsight version 0.9.2 was running on a Contabo host and reachable through port 8888.
- CortexIQ filters episodes by the occurred_at field using half‑open from/to ranges and can interpret Dutch date expressions in its REST API.
- Hindsight’s temporal_window feature only ranks results—its 1‑day window returned 54 hits across five days—and the memories/list endpoint does not support date filtering.
- CortexIQ’s remember operation records content, namespace, tags, occurred_at, external_key, external_refs, and importance while discarding arbitrary metadata; the external_key formatted as gaia:kairos:<episode_id> enforces idempotent writes, rejecting unchanged data unless superseded.
- CortexIQ’s DELETE /api/v1/episodes/:id permanently removes an episode and cascades deletions to derived facts, while attempts to delete facts or namespaces through the REST API return 404 or 405 errors.

**B variant** (numbers kept 10/13, invented [], gate would reject 0)

- CortexIQ version 0.1.88 running on strato-vps with REST endpoint 127.0.0.1:18101 is being used as the replacement for Hindsight version 0.9.2 on contabo (port 8888) in Gaia.
- CortexIQ filters episodes by occurred_at using half‑open from/to intervals and recognises Dutch date expressions via REST, whereas Hindsight’s temporal_window only ranks episodes (a 1‑day window gave 54 hits over five days) and its memories/list endpoint lacks a date filter.
- Scoping of episodes must be performed with namespaces supplied as an array in the REST API; a single namespace is treated only as a ranking set.
- The remember operation stores content, namespace, tags, occurred_at, external_key, external_refs and importance, discarding free metadata; the external_key field makes writes idempotent (unchanged:true) and uses the pattern gaia:kairos:<episode_id> as the mirror key.
- DELETE /api/v1/episodes/:id performs a hard purge and cascades to derived facts, while facts and namespaces cannot be deleted individually via the REST API (responses 404/405).
- In a benchmark of twenty Dutch questions, CortexIQ had a median response time of 1.061 ms with raw episodes ranked highest, compared to Hindsight’s median of 1.372 ms with an emotionally saturated top; consolidation completes within two minutes but produces facts as sparse as Hindsight.

## Episode 1082

> Migratieplan Hindsight→CortexIQ in Gaia-Cloud vastgesteld op 18-11-2026 (plansessie): volgorde zoals de spike van 16-11 bepaalde (MemoryStore-poort → terugkijk-route → gaia-logos rebuild → bojan/gaia import → Hindsight uit). Twee beslissingen van Bo, beide 'meten gaat voor bouwen': (1) CortexIQ-fase 1 (quota recall over namespaces) wordt pas gebouwd nadat een fixture op de live strato-instance aantoonbaar maakt dat één luide namespace de rest verdringt — geen bewijs, geen bouw; (2) hosting van CortexIQ voor Gaia (strato via Tailscale vs eigen instance op contabo) wordt pas gekozen na een latency-meting contabo→strato, uit te voeren nadat de REST aan de tailnet-kant is gebonden (nu 127.0.0.1:18101). Fase 6 (agent_identity) voorlopig geschrapt: namespace + tag dekt de epistemic mark. Fase 0-huiskeeping: episode 980 (Mindstack, Vite-migratie) opnieuw aanleveren; oude hindsight-backups van 03-05 dienen alleen als formaat-referentie bij de verse export in fase 4.

**A production** (numbers kept 6/10, invented [], gate would reject 0)

- The migration plan from Hindsight to CortexIQ in Gaia-Cloud was established on 18‑11‑2026 during a planning session.
- The migration sequence specified on 16‑11 consists of MemoryStore port, backtrack route, gaia‑logos rebuild, bojan/gaia import, and finally decommissioning Hindsight.
- CortexIQ phase 1 (quota recall over namespaces) will be built only after a fixture on the live Strato instance demonstrates that a single dominant namespace overrides the others.
- The choice of hosting CortexIQ for Gaia—either on Strato via Tailscale or on a dedicated Contabo instance—will be made after a latency measurement from Contabo to Strato is conducted once the REST endpoint is bound to the tailnet (currently at 127.0.0.1:18101).
- Phase 6 (agent_identity) of CortexIQ has been temporarily cancelled, with namespace and tag considered sufficient for the epistemic mark.
- Phase 0 housekeeping requires re‑supplying episode 980 (Mindstack, Vite migration) and treats old Hindsight backups from 03‑05 only as a format reference for the new export in phase 4.

**B variant** (numbers kept 8/10, invented [], gate would reject 0)

- The migration plan for moving Hindsight to CortexIQ in Gaia-Cloud was established on 18-11-2026 during a planning session.
- The migration sequence, as defined by the spike on 16-11, follows the order: MemoryStore port, retro‑view route, Gaia logos rebuild, Bojan/Gaia import, then Hindsight removal.
- CortexIQ phase 1 (quota recall over namespaces) will only be built after a fixture on the live Strato instance shows that a single dominant namespace pushes out the rest; without such evidence, construction will not proceed.
- The choice between hosting CortexIQ for Gaia on Strato via Tailscale or on a dedicated Contabo instance will be made after performing a latency measurement from Contabo to Strato, once the REST endpoint is bound to the tailnet at 127.0.0.1:18101.
- CortexIQ phase 6 (agent_identity) is currently cancelled because a namespace combined with a tag already covers the epistemic mark.
- Phase 0 housekeeping requires re‑delivering episode 980 (Mindstack, Vite migration) and using the old Hindsight backups dated 03‑05 solely as a format reference for the new export in phase 4.

## Episode 558

> Gap-analyse CortexIQ vs Hindsight voor Gaia-Cloud (2026-11): Gaia-Cloud hangt via src/hindsightClient.js aan Hindsight voor: (1) banks + cross-bank recall met per-bank quota (scores niet op één schaal), (2) willekeurige tags per memory-item die in recall-output terugkomen (gaia:* tags, memoryKindOf), (3) versiegelde invalidatie/superseding sync vanuit cognitionSync met reconcile, (4) provenance sources:['foundation:<id>'] voor Dual Recall, (5) async fire-and-forget retain, (6) agent_identity-enforcing. CortexIQ heeft inmiddels wél reflect, mental models, reranker, graph-arm, temporal recall en MMR — die eerdere gaten zijn gedicht.

**A production** (numbers kept 0/7, invented [], gate would reject 0)

- Gaia-Cloud integrates with Hindsight through the src/hindsightClient.js module, providing banks and cross-bank recall with per-bank quotas (scores on separate scales), random tags per memory item that appear in recall output (gaia:* tags and memoryKindOf), sealed invalidation/superseding synchronization from cognitionSync with reconcile, provenance sources of the form ['foundation:<id>'] for Dual Recall, asynchronous fire‑and‑forget retain, and enforcement of agent identity.
- CortexIQ now includes reflect, mental models, a reranker, a graph‑arm component, temporal recall, and maximal marginal relevance (MMR), addressing the gaps identified in the earlier comparison with Hindsight.

**B variant** (numbers kept 0/7, invented [], gate would reject 0)

- Gaia-Cloud connects to Hindsight through the src/hindsightClient.js client and offers six capabilities: banks and cross‑bank recall with per‑bank quota (scores not on a single scale), random gaia:* and memoryKindOf tags per memory‑item in recall output, sealed invalidation/superseding synchronization from cognitionSync with reconcile, provenance sources ['foundation:<id>'] for Dual Recall, asynchronous fire‑and‑forget retain, and enforcement of agent identity.
- CortexIQ has newly incorporated reflect, mental models, a reranker, graph‑arm, temporal recall, and maximal marginal relevance (MMR), closing the gaps identified in the earlier gap analysis.

## Episode 36

> Beslissing 2026-10-08 (akkoord Bo): de provenance-prefix voor afgeleide records in Gaia-Cloud gaat van 'chronicle:' naar 'foundation:'. Canonieke id-vorm: 'foundation:<uuid>' met de kale ingest_object uuid (wat Foundation /api/ingest-logs/:id verwacht), niet Foundation's episode-vorm 'ingest:<uuid>'. Reden: 'chronicle:' is legacy (Foundation, niet Chronicle, is de rauwe laag) en is een routing key, geen label. Wordt in één migratie gedaan die ook de strip-bug in kairosRoutes.js fixt en de drie sources-kolommen (hypotheses/patterns/kairos_episodes) plus Hindsight-metadata backfillt; reconcile() wordt uitgebreid met Kairos-episodes. Plan: C:\Users\bojan\.opencode\plan\chronicle-prefix-provenance-plan.md.

**A production** (numbers kept 1/1, invented [], gate would reject 0)

- Gaia-Cloud changed the provenance prefix for derived records from 'chronicle:' to 'foundation:' on 2026-10-08.
- Gaia-Cloud now uses the canonical ID format 'foundation:<uuid>' based on the raw ingest_object UUID, replacing the previous 'ingest:<uuid>' episode format.
- The migration includes a fix for the strip-bug in the kairosRoutes.js file.
- During the migration, the three source columns—hypotheses, patterns, and kairos_episodes—as well as Hindsight metadata are backfilled.
- The reconcile() function is extended to handle Kairos-episodes as part of the migration.
- The migration plan is documented in the file C:\Users\bojan\.opencode\plan\chronicle-prefix-provenance-plan.md.

**B variant** (numbers kept 1/1, invented [], gate would reject 0)

- Gaia-Cloud changed the provenance prefix for derived records from 'chronicle:' to 'foundation:' (decision on 2026-10-08).
- Gaia-Cloud now uses the canonical ID format 'foundation:<uuid>' based on the raw ingest_object UUID for derived records.
- The Foundation API endpoint /api/ingest-logs/:id expects IDs in the form 'foundation:<uuid>', not the episode form 'ingest:<uuid>'.
- The 'chronicle:' prefix is considered legacy, serving as a routing key rather than a label, because Foundation is the raw layer.
- The migration will fix the strip-bug in kairosRoutes.js, backfill the hypotheses, patterns, and kairos_episodes source columns as well as Hindsight metadata, and extend reconcile() to include Kairos episodes.
- The plan for the provenance prefix change is documented in C:\Users\bojan\.opencode\plan\chronicle-prefix-provenance-plan.md.

## Episode 37

> Gaia-Cloud: de chronicle:->foundation: provenance-normalisatie is op 2026-10-08 geïmplementeerd (nieuwe seam src/foundationRef.js, synthesizer+kairosRoutes erop, cognitionSync.reconcile() pusht nu ook Kairos-episodes, migratie 010_normalize_sources_prefix.sql). Tests groen: gaia-api 1244/1244, cognition 49/49. Nog open: migratie toepassen op de VPS (npm run migrate in services/cognition) en daarna npm run reconcile:hindsight -- --apply --clear om de Hindsight-metadata te verversen.

**A production** (numbers kept 2/4, invented [], gate would reject 0)

- Gaia-Cloud implemented provenance-normalisation on 2026-10-08, adding the seam file src/foundationRef.js, integrating synthesizer and kairosRoutes, extending cognitionSync.reconcile() to push Kairos episodes, and applying the migration script 010_normalize_sources_prefix.sql.
- Gaia-Cloud reported all gaia-api tests (1244 of 1244) and all cognition tests (49 of 49) as green.
- Gaia-Cloud still needs to apply the migration on the VPS by executing npm run migrate in services/cognition followed by npm run reconcile:hindsight -- --apply --clear to refresh Hindsight metadata.

**B variant** (numbers kept 2/4, invented [], gate would reject 0)

- Gaia-Cloud foundation implemented provenance normalization on 2026-10-08, adding new seam src/foundationRef.js, adding synthesizer and kairosRoutes, and extending cognitionSync.reconcile() to push Kairos episodes; migration script 010_normalize_sources_prefix.sql was also applied.
- Gaia-Cloud tests were all green, with gaia-api passing 1244 out of 1244 tests and cognition passing 49 out of 49 tests.
- Gaia-Cloud VPS still requires applying the migration by running npm run migrate in services/cognition and subsequently refreshing Hindsight metadata with npm run reconcile:hindsight -- --apply --clear.

## Episode 38

> Reconcile dry-run Gaia-Cloud 2026-10-08: Hindsight actief gaia:hypothesis=2, gaia:pattern=0, gaia:kairos_episode=0; Cognition hypotheses=4, patterns=0, kairos=0. De foundation:-prefix-backfill bleek een no-op: de oude 'chronicle:'-prefix werd alleen door de Kairos-synthesizer geproduceerd en er zijn geen Kairos-episodes (GAIA_KAIROS_ENABLED staat uit), dus --apply --clear is overgeslagen (zou enkel versie-churn geven). De 4-vs-2 kan deels survey-artefact zijn: open_question/relationship/mental_model dragen andere tags en tellen niet als gaia:hypothesis.

**A production** (numbers kept 2/5, invented [], gate would reject 0)

- During the dry‑run reconciliation of Gaia‑Cloud on 2026‑10‑08, the Hindsight component reported two gaia:hypothesis entries, zero gaia:pattern entries, and zero gaia:kairos_episode entries.
- In the same reconciliation, the Cognition component reported four hypotheses, with no patterns or kairos episodes.
- The foundation’s -prefix-backfill operation performed no changes because the old 'chronicle:' prefix is generated only by the Kairos synthesizer and no Kairos episodes exist (GAIA_KAIROS_ENABLED is disabled), so the --apply --clear command was skipped.
- The observed difference between four and two gaia:hypothesis counts may be due to a survey artifact where tags such as open_question, relationship, and mental_model are not counted as gaia:hypothesis.

**B variant** (numbers kept 4/5, invented [], gate would reject 0)

- Gaia-Cloud performed a dry-run reconciliation on 2026-10-08.
- In Hindsight, the counts were gaia:hypothesis 2, gaia:pattern 0, gaia:kairos_episode 0.
- Cognition reported hypotheses 4, patterns 0, kairos 0.
- The foundation's -prefix-backfill operation was a no-op because the old 'chronicle:' prefix was only produced by the Kairos synthesizer and there were no Kairos episodes (GAIA_KAIROS_ENABLED disabled), so the --apply --clear command was skipped.
- The difference between 4 and 2 hypothesis counts may be partly due to a survey artifact: open_question, relationship, and mental_model tags are counted differently and do not contribute to gaia:hypothesis.

## Episode 81

> Gaia-Cloud aanvulling (commit 3f645dd, 2026-10-12): een op het verleden gerichte beurt raadpleegt nu zowel Hindsight als het transcript. recallRelevantContext (memory.js) kreeg een `pastReference`-optie die de recall-gate opent; turn.js geeft die door zodra de gesprekszoek-signalen (exactHistory/pastLookup/rememberedKnowledge) vuren. Reden: Bo's punt 'ze kijkt ook in Hindsight' — de twee gates (memoryPolicy.shouldRecall en de gesprekszoek-signalen) waren onafhankelijk, dus sommige lookup-formuleringen openden alleen het transcript. Let op: Hindsight draait sowieso al elke beurt via de mentale modellen (fetchMentalModelContext), ongeacht de gate. gaia-api 1433/1433 groen (+2).

**A production** (numbers kept 3/4, invented [], gate would reject 0)

- Gaia-Cloud (commit 3f645dd on 2026-10-12) updated past-oriented turns to consult both Hindsight and the transcript.
- The function recallRelevantContext in memory.js received a `pastReference` option that opens the recall gate.
- turn.js forwards the recall gate whenever conversation search signals exactHistory, pastLookup, or rememberedKnowledge fire.
- Prior to the change, memoryPolicy.shouldRecall and the conversation search signals operated independently, so some lookup forms opened only the transcript.
- Hindsight executes every turn through the mental models (fetchMentalModelContext) irrespective of the recall gate.
- The gaia-api test suite reported 1433 passed out of 1433 tests, marked green with a +2 indicator.

**B variant** (numbers kept 3/4, invented [], gate would reject 0)

- Gaia-Cloud (commit 3f645dd, 2026-10-12) was updated so that a past-oriented turn now consults both Hindsight and the transcript.
- The recallRelevantContext function in memory.js received a `pastReference` option that opens the recall gate.
- turn.js forwards the `pastReference` option as soon as the conversation search signals exactHistory, pastLookup, or rememberedKnowledge fire.
- The memoryPolicy.shouldRecall gate was independent of the conversation search signals, causing some lookup formulations to open only the transcript, as noted by Bo's point that Hindsight is also consulted.
- Hindsight runs every turn through the mental models using fetchMentalModelContext, irrespective of the recall gate.
- The gaia-api test suite reported 1433 passed out of 1433 tests (green), with an additional +2 metric.

## Episode 86

> Gaia-Cloud (2026-10-12, commit 8e26794, gedeployed): (1) recall-staart gemeten uit Hindsight's Prometheus-histogram (hindsight_http_duration_seconds, recall-endpoint): 27 recalls, mean 894ms, p50 ~1s, p90/p95 ~2,5s, p99 ~5s; 1 call (3,7%) boven 4s, 0 boven 8s. Kleine, niet-representatieve steekproef (grotendeels test-calls), buckets grof. (2) Observability: elke gefaalde Hindsight-call logt nu een content-loze regel {kind:'hindsight.call_failed', bank, op, reason:'timeout'|'unreachable', timeoutMs, ms} en throwt door — via een request()-wrapper rond fetchImpl in src/hindsightClient.js (alle 14 call-sites). Reden: memory.js slikte Hindsight-fouten stil in (best-effort -> []), dus missende context was onzichtbaar. Test test/hindsightCallLog.test.js (+3), gaia-api 1436/1436. Het is puur een logregel, geen gedragsverandering; verschijnt in de gaia-api containerlog.

**A production** (numbers kept 7/18, invented [95th 2.5 99th 3.7], gate would reject 0)

- Gaia-Cloud measured recall latency from Hindsight's Prometheus histogram for the recall endpoint, observing 27 recalls with a mean of 894 ms, median around 1 s, 90‑95th percentiles near 2.5 s, 99th percentile about 5 s, and one call (3.7 %) exceeding 4 s but none over 8 s; the sample was small and primarily test calls.
- gaia-api was updated so that every failed Hindsight call now logs an empty‑content line with fields kind, bank, op, reason (timeout or unreachable), timeoutMs and ms, and then re‑throws the error via a request() wrapper around fetchImpl in src/hindsightClient.js; this adds observability without changing other behavior and appears in the gaia-api container log.

**B variant** (numbers kept 10/18, invented [2.5 3.7], gate would reject 0)

- Gaia-Cloud recorded recall latency metrics from Hindsight's Prometheus histogram (hindsight_http_duration_seconds, recall-endpoint) showing 27 recalls with a mean of 894 ms, p50 around 1 s, p90/p95 around 2.5 s, p99 around 5 s, and 1 call (3.7 %) exceeding 4 s and none exceeding 8 s, based on a small non‑representative test‑call sample.
- Gaia-Cloud updated observability so that every failed Hindsight call now logs a content‑less entry {kind:'hindsight.call_failed', bank, op, reason:'timeout'|'unreachable', timeoutMs, ms} and rethrows the error via a request() wrapper around fetchImpl in src/hindsightClient.js covering all 14 call‑sites; this change is covered by three new tests in test/hindsightCallLog.test.js, with the gaia-api test suite reporting 1436 passed out of 1436, and the log entries appear in the gaia-api container log.

## Episode 560

> Beslissing: LongMemEval-harnas is uitgeschreven als docs/design-longmemeval.md in cortexiq (commit eb37c41, gepusht naar main 2026-11-02). Belangrijkste bevinding: internal/benchmark bestaat al en is eerlijk (ingest als episodes, antwoorden via reflect, beide judges) — het gat is de officiële dataset + fairness-protocol + publicatie. Kernregels: judge is een ander/sterker model dan het antwoordmodel, abstention krijgt een speciale judge-regel, geen benchmark-specifieke tuning, alleen de S-variant draaien flattert consolidatie (dus ook de M/day-variant), Hindsight-cijfers worden gequoteerd met bron als hun meting.

**A production** (numbers kept 2/2, invented [], gate would reject 0)

- The LongMemEval harness was documented in the file docs/design-longmemeval.md in the CortexIQ repository, associated with commit eb37c41 and pushed to the main branch on 2026-11-02.
- The internal/benchmark already exists and operates fairly, ingesting data as episodes, providing answers via the reflect component, and employing two judges.
- The identified gap for LongMemEval is the absence of an official dataset, a fairness protocol, and a publication.
- The core rules for the LongMemEval benchmark require that the judge be a different or stronger model than the answer model, that abstentions are evaluated with a special judge rule, and that no benchmark-specific tuning is performed.
- Running only the S-variant in the LongMemEval benchmark flattens consolidation, which also applies to the M/day-variant.
- Hindsight scores in the LongMemEval benchmark are quoted together with the source that measured them.

**B variant** (numbers kept 2/2, invented [], gate would reject 0)

- LongMemEval-harnas was documented in the file docs/design-longmemeval.md in the CortexIQ repository (commit eb37c41) and the change was pushed to the main branch on 2026-11-02.
- The internal/benchmark already exists and is considered fair, ingesting data as episodes, providing answers via the reflect component, and using both judges.
- The identified missing components for LongMemEval are an official dataset, a fairness protocol, and a publication.
- Core rules established for LongMemEval: the judge must be a different or stronger model than the answer model; abstentions are handled with a special judge rule; no benchmark-specific tuning is allowed; only the S-variant is run, which also flattens consolidation including the M/day-variant; hindsight scores are quoted with the source as their measurement.

## Episode 364

> Gaia Cloud now retains eligible Hindsight turns as a Hermes-style JSON array of role-labelled User and Assistant messages. Existing memoryworthiness/triviality gates, asynchronous fire-and-forget behavior, and identity/provenance metadata remain; no historical Hindsight data is rewritten. Deployed as commit bddbad5 on 2026-10-23.

**A production** (numbers kept 1/2, invented [], gate would reject 0)

- Gaia Cloud was updated to retain eligible Hindsight turns as a Hermes‑style JSON array containing role‑labelled User and Assistant messages.
- Gaia Cloud continued to keep its memoryworthiness and triviality gates, asynchronous fire‑and‑forget behavior, and identity/provenance metadata unchanged.
- Gaia Cloud did not rewrite any historical Hindsight data after the update.
- Gaia Cloud was deployed with commit bddbad5 on 2026‑10‑23.

**B variant** (numbers kept 2/2, invented [], gate would reject 2)

- Gaia Cloud now retains eligible Hindsight turns as a Hermes-style JSON array of role-labelled User and Assistant messages.
- Gaia Cloud continues to retain its memoryworthiness/triviality gates, asynchronous fire-and-forget behavior, and identity/provenance metadata.
- Gaia Cloud does not rewrite any historical Hindsight data.
- Gaia Cloud was deployed with commit bddbad5 on 2026-10-23.

## Episode 859

> CortexIQ BM25-repetitie op de productiedump (2026-11-11): CI op 3774ea9 groen. Dagelijkse prod-dump (47 MB, checksum OK; prod = 2.944 facts, 728 episodes, 128 MB, goose 60, bge-m3/dim 1024) in verse PG17-container met deploy/postgres-image: pg_restore 5 s, db.Open inclusief migratie 00061 335 ms, bm25-indexen aanwezig, tsvector-kolommen weg. Kwaliteit: BM25 wint bij inspectie van 3 van 15 queries (IDF: zeldzame tokens als 8445 en bge-m3 komen bovenaan, ts_rank_cd gaf generieke woorden); geen gelabelde eval gedraaid, vervolgstap is cortexiq eval recall. Snelheidswinst is op dit kleine corpus nog theoretisch. VPS-toegang was read-only (backuplijst, scp van dump); dump en container na afloop verwijderd. OPEN: prod nog op PG16 en niet geüpgraded; release-job voor cortexiq-postgres en arm64 nooit gedraaid; Bo moet de ssh- en scp-allow-regels en de oude gh-release-create-regel uit .claude/settings.local.json halen (bestand is in git gevolgd, niet committen). Commit van de docs: zie git log.

**A production** (numbers kept 15/21, invented [], gate would reject 0)

- CortexIQ CI build 3774ea9 was green on 2026-11-11.
- The daily CortexIQ production dump measured 47 MB with a verified checksum, containing 2.944 facts across 728 episodes totaling 128 MB, using goose version 60 and bge‑m3 vectors of dimension 1024.
- In the new PostgreSQL 17 container for CortexIQ, BM25 indexes were present and the tsvector columns had been removed.
- BM25 quality assessment for CortexIQ showed it winning 3 of 15 queries, with rare tokens like 8445 and bge‑m3 ranking highest, while ts_rank_cd favored generic words; no labeled evaluation was executed and the next step is the CortexIQ evaluation recall.
- CortexIQ VPS access was read‑only, permitting only backup listing and scp of the dump, and both the dump and the container were removed after the test.
- Open issues for CortexIQ include the production database remaining on PostgreSQL 16 without upgrade, the release job for cortexiq‑postgres and arm64 never having run, and a task for Bo to remove ssh and scp allow rules and the old gh‑release‑create rule from .claude/settings.local.json, which is tracked in git and should not be committed.

**B variant** (numbers kept 16/21, invented [], gate would reject 0)

- CortexIQ continuous integration on commit 3774ea9 reported a green (passing) status.
- CortexIQ performed a daily production dump of 47 MB (checksum OK) containing 2.944 facts, 728 episodes, 128 MB of data, goose 60 and bge‑m3 vectors of dimension 1024 in a fresh PostgreSQL 15 container using the deploy/postgres‑image; pg_restore completed in 5 seconds, db.Open with migration 00061 took 335 ms, BM25 indexes were present and tsvector columns were removed.
- CortexIQ BM25 evaluation showed BM25 winning on 3 of 15 inspected queries; rare tokens such as 8445 and bge‑m3 ranked highest via IDF, while ts_rank_cd returned generic words; no labeled evaluation was run and the next step is a CortexIQ recall evaluation; speed gains remain theoretical on this small corpus.
- CortexIQ VPS access was read‑only, allowing only a backup list and scp of the dump; after use, the dump and the container were removed.
- CortexIQ production database remained on PostgreSQL 16 and had not been upgraded; the release job for cortexiq‑postgres and the arm64 architecture had never been executed.
- Bo must delete the ssh‑allow and scp‑allow rules and the old gh‑release‑create rule from .claude/settings.local.json, a file tracked in git that should not be committed; documentation changes are referenced in the git log.

## Episode 759

> De productiebug supported_by is lokaal opgelost: de background consolidator selecteerde hypotheses zonder supported_by voor pgx named scanning, en positional hypothesis/counter scans misten het veld. Drie bestanden zijn aangepast, walkthrough bijgewerkt, go test ./internal/brain en go test ./... zijn groen. Niet gedeployed; aparte expliciete goedkeuring vereist.

**A production** (numbers kept 0/0, invented [], gate would reject 0)

- CortexIQ resolved the production bug concerning the supported_by field locally by correcting the background consolidator's hypothesis selection for pgx named scanning and fixing missing fields in positional hypothesis/counter scans.
- CortexIQ updated three source files and the walkthrough documentation, after which the Go tests (./internal/brain and ./...) all passed successfully.
- The production bug fix has not been deployed and requires a separate explicit approval before deployment.

**B variant** (numbers kept 0/0, invented [], gate would reject 0)

- The production bug related to the supported_by field in the CortexIQ background consolidator was resolved locally; the consolidator had selected hypotheses without supported_by for pgx named scanning and positional hypothesis/counter scans missed the field.
- Three files in the CortexIQ codebase were modified, the walkthrough documentation was updated, and the commands go test ./internal/brain and go test ./... both succeeded (green).
- The changes for the CortexIQ production bug have not been deployed and require a separate explicit approval before deployment.

## Episode 864

> CortexIQ migratie-race bij opstarten gerepareerd en gepusht (2026-11-11): internal/db/db.go draait de hele schema-opzet (goose.Up, dimensie-ALTER, HNSW-indexen) nu in setupSchema onder withSchemaLock, een sessie-advisory lock (pg_advisory_lock, sleutel 994294737458 = 'cortexiq' in base 36) op een eigen verbinding, wachttijd begrensd op 5 minuten, bij annuleren wordt de sessie gesloten zodat er geen lock achterblijft; Open sluit de pool nu bij elke fout (lekte bij goose-fout). Regressietests internal/db/schemalock_integration_test.go (6 gelijktijdige Open's op een lege database, wederzijdse uitsluiting, annuleerbare wacht); mutatiecheck: zonder lock falen 5 van 6 starts met dezelfde 42P07-fout als in prod. Volledige suite groen op verse PG17-db: 21 pakketten, 153 integratietests, 0 skip. NOG NIET UITGEROLD: productie draait v0.1.74 zonder dit slot; het zit in de volgende release. Open: Bo moet de ssh-allow-regels uit .claude/settings.local.json halen (de uitrol is klaar) en de Jina-sleutel roteren; compose op de VPS wijst nog naar de lokale image cortexiq-postgres:17-bm25 (ghcr-image ...-postgres:0.1.74 bestaat).

**A production** (numbers kept 9/12, invented [], gate would reject 0)

- In CortexIQ, the file internal/db/db.go executes the full database schema setup (goose.Up, dimension ALTER, HNSW indexes) inside the setupSchema function under withSchemaLock, acquiring a session advisory lock (pg_advisory_lock) with key 994294737458 ('cortexiq' in base‑36) on its own connection, limiting the wait to five minutes and closing the session on cancellation to avoid leftover locks.
- CortexIQ's Open function was updated to close its connection pool on any error, addressing a previous leak that happened during goose migration failures.
- The integration test internal/db/schemalock_integration_test.go runs six concurrent Open calls on a fresh database to verify mutual exclusion and cancellable waiting, and a mutation check shows that omitting the advisory lock causes five out of six starts to fail with the same 42P07 error observed in production.
- On a fresh PostgreSQL 17 database, CortexIQ's full integration test suite passed completely, covering 21 packages and 153 integration tests with no tests skipped.
- CortexIQ production is currently running version v0.1.74, which lacks the newly added advisory lock and will receive it in an upcoming release.
- The Docker Compose setup on the VPS still references the local image cortexiq-postgres:17-bm25, while the corresponding GitHub Container Registry image ...-postgres:0.1.74 is available.

**B variant** (numbers kept 6/12, invented [], gate would reject 0)

- internal/db/db.go runs the full database schema setup (goose.Up, dimension ALTER, HNSW indexes) within setupSchema under withSchemaLock, acquiring a pg_advisory_lock with key 994294737458, limiting wait time to 5 minutes, closing the session on cancellation, and ensuring Open closes the connection pool on any error.
- The regression test schemalock_integration_test.go confirmed mutual exclusion by running six concurrent Open calls on an empty database, and demonstrated that omitting the lock caused five of six starts to fail with error code 42P07.
- The full CortexIQ test suite passed on a fresh PostgreSQL 17 database, covering 21 packages and 153 integration tests with no tests skipped.
- The production environment is currently running CortexIQ version v0.1.74, which does not include the newly added schema lock; the lock will be part of the next release.
- Bo must delete the ssh‑allow rules from .claude/settings.local.json and rotate the Jina authentication key.
- Compose on the VPS continues to reference the local image cortexiq‑postgres:17‑bm25 even though the corresponding ghcr.io image cortexiq‑postgres:0.1.74 is available.

## Episode 169

> CortexIQ prod (strato-vps, /opt/cortexiq) is op 2026-10-15 uitgerold naar commit 6377b4c: POST /api/v1/remember is live (401 zonder token, 201 met token) en de embedder wordt bij start op de achtergrond warm geladen (~1,5 s, logregel msg="embedder warm"); warme recall meet ~5 ms in plaats van de oude koude start van ~1,8 s. Goose-versie bleef 27.

**A production** (numbers kept 9/9, invented [1.5 1.8], gate would reject 0)

- CortexIQ prod (hosted on strato-vps at /opt/cortexiq) was rolled out on 2026-10-15 to commit 6377b4c.
- The POST /api/v1/remember endpoint of CortexIQ prod is live and returns HTTP 401 when called without a token and HTTP 201 when called with a token.
- At startup, the embedder of CortexIQ prod is warm‑loaded in the background, taking about 1.5 seconds and emitting the log message msg="embedder warm".
- Warm recall operations in CortexIQ prod measure roughly 5 ms, compared with the previous cold‑start latency of about 1.8 seconds.
- The Goose component in CortexIQ prod remained at version 27.

**B variant** (numbers kept 9/9, invented [1.5 1.8], gate would reject 0)

- CortexIQ production instance (strato-vps, /opt/cortexiq) was deployed on 2026-10-15 to commit 6377b4c.
- The POST /api/v1/remember endpoint in CortexIQ production returns 401 when no token is provided and 201 when a token is provided.
- At startup, the CortexIQ embedder is warm‑loaded in the background, taking approximately 1.5 seconds, and logs the message msg="embedder warm".
- Warm recall in CortexIQ measures a latency of about 5 ms.
- Previously, cold‑start recall latency in CortexIQ was about 1.8 seconds.
- The Goose component in CortexIQ remained at version 27.

## Episode 697

> CortexIQ: RRF-sweep gedraaid op de productie-embedder (bge-m3, hybride, Jina-reranker; 60 memories, 36 vragen, k=10) via docker compose run met een override naar een wegwerpdatabase cortexiq_eval (productie niet geraakt, daarna verwijderd). Uitkomst overall: rrf-k 20 hit@1 0.75 MRR 0.850; rrf-k 60 hit@1 0.78 MRR 0.868; rrf-k 100 identiek aan 60. Verschil is één à twee vragen van 36, dus ruis. Besluit: CORTEXIQ_RRF_K blijft 60. Alle varianten missen dezelfde acht vragen op ongeveer dezelfde plaats, dus een betere recall zit in embedder/reranker, niet in de fusieconstante. Recept voor een volgende keer: op de server in /opt/cortexiq een override met CORTEXIQ_POSTGRES_DSN naar een extra database, dan docker compose -f docker-compose.prod.yml -f override run --rm --no-deps -T -v /opt/cortexiq/eval:/eval:ro cortexiq eval recall --rrf-k ... --corpus /eval/recall_corpus.jsonl --queries /eval/recall_queries.jsonl. Les: gebruik bij .env-greps nooit een patroon dat KEY-variabelen raakt (RERANK matchte CORTEXIQ_RERANKER_API_KEY en de Jina-sleutel kwam in het gesprek; Bo is aangeraden hem te roteren).

**A production** (numbers kept 11/11, invented [], gate would reject 0)

- CortexIQ performed an RRF sweep on the production embedder (bge-m3, hybrid, Jina-reranker) with 60 memories, 36 queries and k=10 via Docker Compose using a temporary database cortexiq_eval, which was removed afterward without affecting production.
- The RRF sweep yielded a hit@1 of 0.75 and MRR of 0.850 at k=20, and a hit@1 of 0.78 and MRR of 0.868 at k=60, with the k=100 configuration matching the k=60 results; the variation was attributed to one or two of the 36 queries.
- CORTEXIQ_RRF_K was kept at 60 following the evaluation.
- All variants missed the same eight questions, indicating that improving recall requires changes to the embedder or reranker rather than adjusting the fusion constant.
- For a subsequent evaluation, the suggested method is to set CORTEXIQ_POSTGRES_DSN to an additional database under /opt/cortexiq, then run Docker Compose with production and override files, mounting /opt/cortexiq/eval into the container and invoking cortexiq eval recall with the appropriate corpus and query files.
- The lesson learned is to never use .env grep patterns that match key variables, as this exposed CORTEXIQ_RERANKER_API_KEY and a Jina key, and Bo was advised to rotate the exposed keys.

**B variant** (numbers kept 11/11, invented [], gate would reject 0)

- CortexIQ executed an RRF sweep on the production embedder bge-m3 (hybrid Jina reranker) with 60 memories, 36 queries, k=10 using docker compose run and a temporary database cortexiq_eval, leaving production untouched and then removing the temporary database
- CortexIQ recorded RRF sweep results: for rrf‑k 20 hit@1 0.75 and MRR 0.850; for rrf‑k 60 hit@1 0.78 and MRR 0.868; rrf‑k 100 produced the same results as rrf‑k 60
- CORTEXIQ_RRF_K was decided to remain set to 60 after the evaluation
- CortexIQ observed that every variant missed the same eight questions, indicating that better recall would come from improving the embedder or reranker, not adjusting the fusion constant
- CortexIQ recommends for future runs to override CORTEXIQ_POSTGRES_DSN to an extra database in /opt/cortexiq and run a docker‑compose command mounting /opt/cortexiq/eval as read‑only to evaluate recall with specified parameters
- CortexIQ learned to never use .env patterns that affect key variables, noting that the RERANK pattern matched CORTEXIQ_RERANKER_API_KEY and exposed the Jina key, and Bo recommended rotating the key

## Episode 888

> CortexIQ: v0.1.80 (Timeline-vergelijking tussen twee data, commit 6f725cd, CI groen) is door Claude gepubliceerd en de Release-workflow is groen (2026-11-13). De uitrol naar productie is NIET uitgevoerd: de auto-mode classifier blokkeerde het ssh-commando voor de deploy. Productie draait vermoedelijk nog v0.1.79 (niet geverifieerd). Bo moet de deploy toestaan of zelf draaien. Let op: vaste toestemming van Bo gold voor het publiceren van releases; uitrol via ssh valt buiten wat de classifier toestaat zonder allow-regel in .claude/settings.local.json.

**A production** (numbers kept 4/4, invented [], gate would reject 0)

- CortexIQ version v0.1.80, identified by commit 6f725cd, was published by Claude on 2026-11-13 and its continuous‑integration status was green.
- The deployment of CortexIQ version v0.1.80 to the production environment was not performed because the auto‑mode classifier blocked the SSH command used for the deploy.
- The production instance of CortexIQ is believed to still be running version v0.1.79, although this has not been verified.
- Bo must either grant permission for the deployment to proceed or carry out the deployment himself.
- Bo's fixed permission covers publishing releases, but the classifier does not permit deployment via SSH unless an allow rule is added to .claude/settings.local.json.

**B variant** (numbers kept 4/4, invented [], gate would reject 0)

- CortexIQ version v0.1.80 (Timeline-vergelijking tussen twee data, commit 6f725cd) was published by Claude on 2026-11-13 with CI status green.
- The release workflow for CortexIQ v0.1.80 was green on 2026-11-13.
- The auto-mode classifier blocked the ssh command for the deployment of CortexIQ v0.1.80, preventing rollout to production.
- The production environment is presumed to still be running CortexIQ version v0.1.79, though this is not verified.
- Bo needs to allow the deployment of CortexIQ v0.1.80 or execute it manually.
- An allow rule in .claude/settings.local.json is required for the classifier to permit ssh deployment, as Bo's fixed permission only covers publishing releases.

## Episode 412

> CortexIQ-productie (strato-vps, /opt/cortexiq) draait sinds 2026-10-27 v0.1.6 (commit c107f8c, image sha256:61d7ed372332). Deployprocedure: GitHub-release publiceren (release-workflow bouwt binaries en image), CORTEXIQ_IMAGE_TAG in /opt/cortexiq/.env bumpen, daarna docker compose -f docker-compose.prod.yml pull en up -d. De UI is een aparte service (cortexiq-ui) op dezelfde image; UI luistert op 127.0.0.1:18101, API op 18100, beide eisen een token.

**A production** (numbers kept 6/6, invented [127.0.0.1:18100], gate would reject 0)

- CortexIQ production on strato-vps at /opt/cortexiq has been running since 2026-10-27 with version v0.1.6 (commit c107f8c, image sha256:61d7ed372332).
- CortexIQ is deployed by publishing a GitHub release, using a release workflow that builds binaries and a Docker image, updating CORTEXIQ_IMAGE_TAG in /opt/cortexiq/.env, followed by executing docker compose -f docker-compose.prod.yml pull and up -d.
- cortexiq-ui is a separate service that runs on the same Docker image as the CortexIQ production instance.
- cortexiq-ui listens on 127.0.0.1:18101.
- CortexIQ API listens on 127.0.0.1:18100.
- Both the CortexIQ UI and API require a token for access.

**B variant** (numbers kept 5/6, invented [127.0.0.1:18100], gate would reject 0)

- CortexIQ-productie has been running on strato‑vps at /opt/cortexiq since 2026‑10‑27 with version v0.1.6 (commit c107f8c, image sha256:61d7ed372332).
- CortexIQ‑productie is deployed by publishing a GitHub release, building binaries and image via a release workflow, bumping CORTEXIQ_IMAGE_TAG in /opt/cortexiq/.env, and running docker compose -f docker-compose.prod.yml pull and up -d.
- cortexiq-ui is a separate service that runs on the same Docker image as the main CortexIQ‑productie application.
- cortexiq-ui (CortexIQ UI) listens on 127.0.0.1:18101.
- CortexIQ API listens on 127.0.0.1:18100.
- Both the UI and the API require a token for access.

## Episode 886

> CortexIQ werkwijze (2026-11-13): Bo geeft vaste toestemming dat Claude zelf GitHub-releases publiceert (gh release create <versie> --target main) zodra CI op main groen is; Bo hoeft het niet meer zelf te doen. Daarbij altijd versienummer en releasenotities melden in het antwoord. Uitrol naar productie volgt de vaste procedure (back-up .env, pull terwijl het draait, verified dump, tag bumpen, healthz/readyz/restarts/logs controleren). Eerder deed Bo het publiceren zelf vanaf v0.1.74; die gewoonte is nu vervangen.

**A production** (numbers kept 1/2, invented [], gate would reject 0)

- Bo gave permanent permission for Claude to publish GitHub releases for CortexIQ.
- Claude publishes CortexIQ GitHub releases with the command 'gh release create <versie> --target main'.
- Claude automatically creates a CortexIQ release when the continuous integration on the main branch reports a green status.
- Claude must include the version number and release notes in each CortexIQ release response.
- CortexIQ production deployment follows a fixed procedure: backing up the .env file, pulling updates while the service runs, performing a verified dump, bumping the tag, and checking healthz/readyz endpoints, restarts, and logs.
- Bo previously published CortexIQ releases manually starting with version v0.1.74, a practice that has now been replaced.

**B variant** (numbers kept 1/2, invented [], gate would reject 0)

- CortexIQ: Claude was given permanent permission to publish GitHub releases using `gh release create <versie> --target main` as soon as CI on the main branch is green.
- CortexIQ: Bo no longer needs to publish GitHub releases himself.
- CortexIQ: Claude must always include the version number and release notes in the response when publishing a release.
- CortexIQ: Production deployment follows a fixed procedure that includes backing up the .env file, pulling while the service is running, performing a verified dump, bumping the tag, and checking healthz, readyz, restarts, and logs.
- CortexIQ: Bo previously published releases himself starting with version v0.1.74, but this practice has now been replaced.

## Episode 670

> CortexIQ: v0.1.20 (e56a03b) is op 2026-11-05 gedeployed naar strato-vps; v0.1.19 is wel gebouwd maar nooit uitgerold, productie ging direct van 0.1.18 naar 0.1.20. Bevat migraties 00044 (judged), 00045 (origin/origin_ref) en 00046 (strength), de rebuild-CLI en CORTEXIQ_RRF_K. Volgorde: geverifieerde dump backups/cortexiq-20261105T002405Z.dump, release, .env gebackupt als .env.bak-pre0.1.20, tag-bump en up -d. Gecontroleerd: version v0.1.20, goose 46, containers Up, geen panic of migratiefout, 2330 facts allemaal zonder origin (oude facts, bewust niet teruggevuld). Een read-only rebuild-rapport van /projects/cortexiq in productie: 114 episodes, 476 facts, 6 facts op ingetrokken bronnen (gat C). CORTEXIQ_DECAY_SPLIT staat NIET aan en docker-compose.prod.yml op de server geeft de variabele nog niet door. Open voor Bo: de ssh- en gh-release-allow-regels uit de projectsettings halen; beslissen over DECAY_SPLIT; zelf de RRF-sweep draaien (cortexiq eval recall --rrf-k 20,60,100).

**A production** (numbers kept 15/17, invented [], gate would reject 0)

- CortexIQ version v0.1.20 (commit e56a03b) was deployed to the strato-vps server on 2026-11-05.
- CortexIQ version 0.1.19 was built but never released, so production moved directly from version 0.1.18 to version 0.1.20.
- CortexIQ version 0.1.20 includes migrations 00044 (judged), 00045 (origin/origin_ref), 00046 (strength), the rebuild-CLI, and the CORTEXIQ_RRF_K component.
- The deployment of CortexIQ version 0.1.20 followed steps: verified the dump backup at backups/cortexiq-20261105T002405Z.dump, backed up .env as .env.bak-pre0.1.20, performed a tag bump, and executed docker compose up -d.
- After deployment, CortexIQ was observed running version 0.1.20 with Goose migration 46, all containers up, no panic or migration errors, and contained 2330 facts that all lacked an origin.
- A read‑only rebuild report for CortexIQ in production showed 114 episodes, 476 facts, and 6 facts linked to retracted sources, while the CORTEXIQ_DECAY_SPLIT setting was not enabled and the docker‑compose.prod.yml file did not yet pass this variable.

**B variant** (numbers kept 14/17, invented [], gate would reject 0)

- CortexIQ version v0.1.20 (commit e56a03b) was deployed on 2026-11-05 to the strato-vps server.
- CortexIQ production released version v0.1.18 and then jumped directly to v0.1.20, bypassing the built but never deployed version v0.1.19.
- CortexIQ release included database migrations 00044 (judged), 00045 (origin/origin_ref) and 00046 (strength).
- CortexIQ verification confirmed version v0.1.20, Goose schema version 46, all containers running, no panic or migration errors, and a total of 2330 facts stored without origin information.
- CortexIQ read‑only rebuild report in production showed 114 episodes, 476 facts, and 6 facts referencing withdrawn sources (gate C).
- CortexIQ had the CORTEXIQ_DECAY_SPLIT setting disabled, and the docker-compose.prod.yml file on the server did not propagate this variable.

## Episode 924

> CortexIQ besluit (2026-11-13, Bo): Jina (jina-reranker-v3.5) blijft de reranker in productie; geen eigen reranker op de VPS. Reden: lokaal (llama.cpp, bge-reranker-v2-m3) is ongeveer 2,9 s trager per recall (mogelijk 3-5 s op de VPS, gedeeld met andere workloads) en iets slechter (MRR 0,80 tegen 0,85; zie id 923). Niets gewijzigd in productie. Het risico dat Jina wegvalt (saldo, 429-limiet van 100.000 tokens per minuut, 404) wordt afgedekt door de metric recall_rerank_requests_total, de logregel per minuut, RERANK_CANDIDATES=30 en de graaf-routing als vangnet (gewogen is dan beter dan uit). Niet gedaan en niet nodig tenzij het pijn doet: fallback-keten Jina dan lokaal (codewijziging), meten met 10 kandidaten, meten op de VPS zelf.

**A production** (numbers kept 10/15, invented [2.9 0.80 0.85], gate would reject 0)

- CortexIQ kept jina-reranker-v3.5 as the reranker in production and did not deploy a custom reranker on the VPS.
- CortexIQ observed that the local reranker using llama.cpp with bge-reranker-v2-m3 is about 2.9 seconds slower per recall (potentially 3–5 seconds on the VPS) and achieves a lower MRR of 0.80 compared with 0.85 for Jina.
- CortexIQ reported no changes to the production environment.
- CortexIQ mitigates the risk of Jina becoming unavailable (due to balance, a 429 token‑per‑minute limit of 100 000, or 404 errors) by monitoring the metric recall_rerank_requests_total, a minute‑level log entry, setting RERANK_CANDIDATES to 30, and using weighted graph routing as a fallback.
- CortexIQ has not implemented a fallback chain to run Jina locally, and it will be considered only if problems occur, with plans to measure using 10 candidates on the VPS after code changes.

**B variant** (numbers kept 11/15, invented [2.9 0.80 0.85], gate would reject 0)

- CortexIQ decided on 2026-11-13 that the production reranker remains Jina (jina-reranker-v3.5) and no custom reranker is deployed on the VPS.
- CortexIQ observed that the local reranker using llama.cpp and bge-reranker-v2-m3 is about 2.9 seconds slower per recall than Jina, possibly 3–5 seconds on the VPS when sharing resources.
- CortexIQ measured the local reranker’s mean reciprocal rank as 0.80, which is lower than Jina’s 0.85.
- CortexIQ made no changes to the production reranker configuration.
- CortexIQ mitigates the risk of Jina outage (balance, 429 token limit of 100 000 per minute, 404 errors) using the recall_rerank_requests_total metric, a per‑minute log entry, RERANK_CANDIDATES set to 30, and graph routing as a fallback.
- CortexIQ has not implemented a fallback chain that runs Jina locally; it will be added only if necessary, with measurements using 10 candidates on the VPS.

## Episode 664

> CortexIQ: v0.1.18 (afgewezen-waarde-tombstone, migratie 00043, Reflect-tab #7) is op 2026-11-05 gedeployed naar strato-vps. Volgorde: geverifieerde dump (backups/cortexiq-20261104T232239Z.dump), release gepubliceerd door Bo (de auto-mode classifier weigerde gh release create van mij), .env gebackupt als .env.bak-pre0.1.18, tag-bump en up -d. Gecontroleerd: version v0.1.18 (6c89dd3), goose 43, facts_refused in de logs. Productie had 0 retracties, dus de backfill deed niets. Het Gemma-parsefoutje bij causal links in /projects/chronicle is het bekende lijstprobleem, niet nieuw. Praktisch: SSH naar strato-vps werkt via een allow-regel in de projectsettings; die hoort na de deploy weg.

**A production** (numbers kept 6/8, invented [], gate would reject 0)

- CortexIQ version v0.1.18 was deployed to the strato-vps host on 2026-11-05.
- The deployment of CortexIQ v0.1.18 followed a sequence that included a verified dump (backups/cortexiq-20261104T232239Z.dump), a release published by Bo, a .env backup saved as .env.bak-pre0.1.18, a tag bump, and a Docker Compose up -d.
- After deployment, CortexIQ was confirmed to be running version v0.1.18 with commit hash 6c89dd3, Goose migration number 43, and the logs contained a facts_refused entry.
- CortexIQ production recorded zero retractions during the deployment, indicating that the backfill operation performed no changes.
- The Gemma parser reported an error with causal links in /projects/chronicle, identified as the known list problem and not a new issue.
- SSH access to the strato-vps host works through an allow-rule defined in the project settings, which should be removed after the deployment.

**B variant** (numbers kept 7/8, invented [], gate would reject 1)

- CortexIQ version v0.1.18 (afgewezen-waarde-tombstone, migratie 00043, Reflect-tab #7) was deployed to strato-vps on 2026-11-05.
- The deployment of CortexIQ followed the sequence: verified dump, release published by Bo, .env backed up as .env.bak-pre0.1.18, tag bump, and up -d.
- CortexIQ was confirmed to be at version v0.1.18 (commit 6c89dd3) with goose version 43, and the logs contained facts_refused.
- CortexIQ production recorded 0 retractions, so the backfill performed no actions.
- The Gemma parser error for causal links in /projects/chronicle was identified as the known list problem and was not new.
- SSH to strato-vps operated via an allow rule in the projectsettings, which is intended to be removed after the deployment.

---

episodes 21 | A: 105 facts, numbers kept 117/171, invented 10 | B: 106 facts, numbers kept 126/171, invented 8, would be rejected by the production gate 3
