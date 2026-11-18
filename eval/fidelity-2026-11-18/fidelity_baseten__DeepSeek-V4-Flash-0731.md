# Fact fidelity: baseten/DeepSeek-V4-Flash-0731

A = production prompt and gate. B = rule replaced, no gate. Read the facts against the episode: subject and object right? detail kept? anything added?

## Episode 989

> Spike 16-11-2026: CortexIQ 0.1.88 (strato-vps, REST 127.0.0.1:18101) vergeleken met Hindsight 0.9.2 (contabo, 8888) als vervanger van Hindsight in Gaia. Beslissend verschil: CortexIQ filtert op occurred_at met from/to (half-open) en herkent Nederlandse datumzinnen over REST, terwijl Hindsight temporal_window alleen rankt (een venster van 1 dag gaf 54 hits over vijf dagen) en memories/list geen datumfilter heeft. Scoping moet met namespaces (array in REST); de enkelvoudige namespace is alleen een ranking-zetje. remember bewaart content, namespace, tags, occurred_at, external_key, external_refs en importance, maar gooit vrije metadata weg; external_key maakt de write idempotent (unchanged:true, andere inhoud geweigerd tenzij supersedes), dus gaia:kairos:<episode_id> is de spiegel-sleutel. DELETE /api/v1/episodes/:id purge hard en cascadeert naar afgeleide facts; facts en namespaces zijn niet los te verwijderen via REST (404/405). Consolidatie reageert binnen twee minuten maar levert even dunne facts als Hindsight. Op twintig Nederlandse vragen: CortexIQ mediaan 1.061 ms met ruwe episodes bovenaan, Hindsight 1.372 ms met een door emotionele gaia-herinneringen verzadigde top. Rapport: docs/spike-cortexiq.md (commits 85dd48e en f329e24). Volgorde blijft: MemoryStore-poort, terugkijk-route op CortexIQ, gaia-logos rebuilden, bojan/gaia importeren, Hindsight uit.

**A production** (numbers kept 11/13, invented [], gate would reject 0)

- On 16-11-2026, CortexIQ 0.1.88 (strato-vps, REST 127.0.0.1:18101) was compared with Hindsight 0.9.2 (contabo, port 8888) as a proposed replacement for Hindsight within the Gaia project. The comparison results were recorded in docs/spike-cortexiq.md with commits 85dd48e and f329e24.
- CortexIQ filters episodes on the occurred_at field using from/to parameters which are half-open, and it recognizes Dutch date phrases over its REST API. In contrast, Hindsight's temporal_window only re-ranks results (a one-day window returned 54 hits over five days), and its memories/list endpoint has no date filter.
- CortexIQ requires scoping via namespaces provided as an array in REST calls; a single namespace only influences ranking, not filtering.
- The CortexIQ remember endpoint stores content, namespace, tags, occurred_at, external_key, external_refs, and importance, while discarding free-form metadata. The external_key makes writes idempotent: identical writes return unchanged:true, and different content is rejected unless supersedes is used. The mirror key used is gaia:kairos:<episode_id>.
- CortexIQ's DELETE /api/v1/episodes/:id performs a hard purge that cascades to derived facts; facts and namespaces cannot be deleted separately via REST, returning 404/405.
- In a test on twenty Dutch questions, CortexIQ had a median response time of 1,061 ms with raw episodes at the top, while Hindsight had 1,372 ms with a top saturated by emotional gaia memories. CortexIQ's consolidation reacted within two minutes but produced facts as thin as Hindsight's.

**B variant** (numbers kept 6/13, invented [1-day], gate would reject 0)

- CortexIQ 0.1.88 was compared with Hindsight 0.9.2 as a replacement for Hindsight in Gaia; the decisive difference is that CortexIQ filters on occurred_at with half-open from/to and recognizes Dutch date phrases over REST, whereas Hindsight only ranks with a temporal_window (a 1-day window gave 54 hits over five days) and memories/list has no date filter.
- CortexIQ scoping requires namespaces as an array in REST; providing a single namespace only affects ranking.
- CortexIQ's remember stores content, namespace, tags, occurred_at, external_key, external_refs and importance, but discards free metadata; external_key makes the write idempotent (unchanged:true, other content rejected unless supersedes), and gaia:kairos:<episode_id> is the suggested mirror key.
- In CortexIQ, DELETE /api/v1/episodes/:id purges hard and cascades to derived facts; facts and namespaces cannot be deleted separately via REST (404/405).
- CortexIQ's consolidation reacted within two minutes but yielded thin facts similar to Hindsight.
- On twenty Dutch questions, CortexIQ had a median of 1,061 ms with raw episodes on top, while Hindsight had 1,372 ms with the top saturated by emotional gaia memories.

## Episode 1082

> Migratieplan Hindsight→CortexIQ in Gaia-Cloud vastgesteld op 18-11-2026 (plansessie): volgorde zoals de spike van 16-11 bepaalde (MemoryStore-poort → terugkijk-route → gaia-logos rebuild → bojan/gaia import → Hindsight uit). Twee beslissingen van Bo, beide 'meten gaat voor bouwen': (1) CortexIQ-fase 1 (quota recall over namespaces) wordt pas gebouwd nadat een fixture op de live strato-instance aantoonbaar maakt dat één luide namespace de rest verdringt — geen bewijs, geen bouw; (2) hosting van CortexIQ voor Gaia (strato via Tailscale vs eigen instance op contabo) wordt pas gekozen na een latency-meting contabo→strato, uit te voeren nadat de REST aan de tailnet-kant is gebonden (nu 127.0.0.1:18101). Fase 6 (agent_identity) voorlopig geschrapt: namespace + tag dekt de epistemic mark. Fase 0-huiskeeping: episode 980 (Mindstack, Vite-migratie) opnieuw aanleveren; oude hindsight-backups van 03-05 dienen alleen als formaat-referentie bij de verse export in fase 4.

**A production** (numbers kept 9/10, invented [], gate would reject 0)

- The migration plan from Hindsight to CortexIQ in Gaia-Cloud was established on 18-11-2026, with the step order (as determined by the spike of 16-11): MemoryStore-poort, terugkijk-route, gaia-logos rebuild, bojan/gaia import, Hindsight uit.
- CortexIQ fase 1 (quota recall over namespaces) will be built only after a fixture on the live strato-instance demonstrates that one loud namespace displaces the rest; without such proof, no build occurs.
- The hosting choice for CortexIQ for Gaia (strato via Tailscale vs an own instance on Contabo) will be decided only after a latency measurement from Contabo to Strato, which requires that the REST API is first bound to the tailnet side (currently at 127.0.0.1:18101).
- Fase 6 (agent_identity) of the CortexIQ migration plan was temporarily scrapped because the namespace plus tag combination covers the epistemic mark.
- In fase 0 of the CortexIQ migration (housekeeping), episode 980 (Mindstack, Vite-migratie) must be provided again; the old hindsight-backups from 03-05 will be used only as a format reference for the fresh export in fase 4.

**B variant** (numbers kept 8/10, invented [], gate would reject 0)

- The Hindsight-to-CortexIQ migration plan in Gaia-Cloud was established on 18-11-2026 during a planning session, with the sequence: MemoryStore-poort, terugkijk-route, gaia-logos rebuild, bojan/gaia import, Hindsight uit.
- CortexIQ phase 1 (quota recall over namespaces) will only be built after a fixture on the live strato-instance demonstrates that one loud namespace displaces others; no evidence, no build.
- The hosting choice for CortexIQ (strato via Tailscale vs own instance on contabo) will only be made after a latency measurement contabo→strato, to be run after the REST is bound to the tailnet side (currently 127.0.0.1:18101).
- CortexIQ phase 6 (agent_identity) was temporarily scrapped; namespace + tag covers the epistemic mark.
- For the Hindsight→CortexIQ migration in Gaia-Cloud, phase 0 housekeeping requires re-delivering episode 980 (Mindstack, Vite-migratie) and using old hindsight-backups of 03-05 only as format reference for the fresh export in phase 4.

## Episode 558

> Gap-analyse CortexIQ vs Hindsight voor Gaia-Cloud (2026-11): Gaia-Cloud hangt via src/hindsightClient.js aan Hindsight voor: (1) banks + cross-bank recall met per-bank quota (scores niet op één schaal), (2) willekeurige tags per memory-item die in recall-output terugkomen (gaia:* tags, memoryKindOf), (3) versiegelde invalidatie/superseding sync vanuit cognitionSync met reconcile, (4) provenance sources:['foundation:<id>'] voor Dual Recall, (5) async fire-and-forget retain, (6) agent_identity-enforcing. CortexIQ heeft inmiddels wél reflect, mental models, reranker, graph-arm, temporal recall en MMR — die eerdere gaten zijn gedicht.

**A production** (numbers kept 2/7, invented [], gate would reject 0)

- As of November 2026, Gaia-Cloud relied on Hindsight through src/hindsightClient.js for six capabilities: recall with per-bank quotas (scores not on a unified scale), arbitrary tags per memory item that surface in recall output (including gaia:* tags and memoryKindOf), sealed invalidation and superseding synchronization from cognitionSync with reconcile, provenance sources ['foundation:<id>'] for Dual Recall, asynchronous fire-and-forget retain, and agent_identity enforcement.
- As of November 2026, CortexIQ had gained the capabilities reflect, mental models, reranker, graph-arm, temporal recall, and MMR, which had previously been gaps relative to Hindsight.

**B variant** (numbers kept 4/7, invented [], gate would reject 0)

- As of 2026-11, Gaia-Cloud depends on Hindsight via src/hindsightClient.js for: banks and cross-bank recall with per-bank quota (scores not on a single scale); arbitrary tags per memory item that appear in recall output (gaia:* tags, memoryKindOf); sealed invalidation/superseding sync from cognitionSync with reconcile; provenance sources ['foundation:<id>'] for Dual Recall; async fire-and-forget retain; and agent_identity enforcement.
- As of 2026-11, CortexIQ has reflect, mental models, reranker, graph-arm, temporal recall and MMR, closing earlier gaps compared to Hindsight.

## Episode 36

> Beslissing 2026-10-08 (akkoord Bo): de provenance-prefix voor afgeleide records in Gaia-Cloud gaat van 'chronicle:' naar 'foundation:'. Canonieke id-vorm: 'foundation:<uuid>' met de kale ingest_object uuid (wat Foundation /api/ingest-logs/:id verwacht), niet Foundation's episode-vorm 'ingest:<uuid>'. Reden: 'chronicle:' is legacy (Foundation, niet Chronicle, is de rauwe laag) en is een routing key, geen label. Wordt in één migratie gedaan die ook de strip-bug in kairosRoutes.js fixt en de drie sources-kolommen (hypotheses/patterns/kairos_episodes) plus Hindsight-metadata backfillt; reconcile() wordt uitgebreid met Kairos-episodes. Plan: C:\Users\bojan\.opencode\plan\chronicle-prefix-provenance-plan.md.

**A production** (numbers kept 1/1, invented [], gate would reject 0)

- On 2026-10-08, a decision was made to change the provenance-prefix for derived records in Gaia-Cloud from 'chronicle:' to 'foundation:'.
- The canonical ID form for derived records in Gaia-Cloud is 'foundation:<uuid>', where <uuid> is the raw ingest_object uuid (the one expected by Foundation /api/ingest-logs/:id), not Foundation's episode form 'ingest:<uuid>'.
- The reason for changing the provenance-prefix in Gaia-Cloud is that 'chronicle:' was considered legacy because Foundation, not Chronicle, is the raw layer, and 'chronicle:' functions as a routing key rather than a label.
- The prefix change in Gaia-Cloud is implemented in a single migration that also fixes the strip-bug in kairosRoutes.js, backfills the three sources columns (hypotheses, patterns, kairos_episodes) plus Hindsight metadata, and extends reconcile() to include Kairos episodes.
- The plan for the provenance-prefix change in Gaia-Cloud is documented in the file at C:\Users\bojan\.opencode\plan\chronicle-prefix-provenance-plan.md.

**B variant** (numbers kept 1/1, invented [], gate would reject 0)

- Gaia-Cloud changed the provenance-prefix for derived records from 'chronicle:' to 'foundation:' (decision of 2026-10-08, agreed by Bo).
- The canonical id form for derived records in Gaia-Cloud is now 'foundation:<uuid>' using the bare ingest_object uuid as expected by Foundation /api/ingest-logs/:id, not Foundation's episode form 'ingest:<uuid>'.
- The prefix change from 'chronicle:' to 'foundation:' in Gaia-Cloud was made because 'chronicle:' is legacy (Foundation, not Chronicle, is the raw layer) and is a routing key, not a label.
- The Gaia-Cloud migration that changes the provenance-prefix also fixes the strip-bug in kairosRoutes.js.
- The Gaia-Cloud migration also backfills the three sources columns (hypotheses/patterns/kairos_episodes) plus Hindsight metadata, and extends reconcile() with Kairos episodes.
- The plan for the Gaia-Cloud provenance-prefix migration is at C:\Users\bojan\.opencode\plan\chronicle-prefix-provenance-plan.md.

## Episode 37

> Gaia-Cloud: de chronicle:->foundation: provenance-normalisatie is op 2026-10-08 geïmplementeerd (nieuwe seam src/foundationRef.js, synthesizer+kairosRoutes erop, cognitionSync.reconcile() pusht nu ook Kairos-episodes, migratie 010_normalize_sources_prefix.sql). Tests groen: gaia-api 1244/1244, cognition 49/49. Nog open: migratie toepassen op de VPS (npm run migrate in services/cognition) en daarna npm run reconcile:hindsight -- --apply --clear om de Hindsight-metadata te verversen.

**A production** (numbers kept 2/4, invented [], gate would reject 0)

- Gaia Chronicle implemented provenance normalization on 2026-10-08, introducing a new seam at src/foundationRef.js, wiring synthesizer and kairosRoutes to it, updating cognitionSync.reconcile() to also push Kairos episodes, and adding migration file 010_normalize_sources_prefix.sql.
- The gaia-api test suite for Gaia Chronicle passed all 1244 tests.
- The cognition test suite for Gaia Chronicle passed all 49 tests.
- For Gaia Chronicle, the migration still needs to be applied on the VPS via 'npm run migrate' in services/cognition, after which 'npm run reconcile:hindsight -- --apply --clear' must be run to refresh Hindsight metadata.

**B variant** (numbers kept 4/4, invented [], gate would reject 0)

- Gaia-Cloud implemented provenance normalization on 2026-10-08, including a new seam at src/foundationRef.js, integrating synthesizer and kairosRoutes, updating cognitionSync.reconcile() to also push Kairos episodes, and adding migration 010_normalize_sources_prefix.sql.
- Gaia-Cloud passed all tests: gaia-api 1244/1244 and cognition 49/49.
- Gaia-Cloud still needs to apply the migration on the VPS (npm run migrate in services/cognition) and then run npm run reconcile:hindsight -- --apply --clear to refresh the Hindsight metadata.

## Episode 38

> Reconcile dry-run Gaia-Cloud 2026-10-08: Hindsight actief gaia:hypothesis=2, gaia:pattern=0, gaia:kairos_episode=0; Cognition hypotheses=4, patterns=0, kairos=0. De foundation:-prefix-backfill bleek een no-op: de oude 'chronicle:'-prefix werd alleen door de Kairos-synthesizer geproduceerd en er zijn geen Kairos-episodes (GAIA_KAIROS_ENABLED staat uit), dus --apply --clear is overgeslagen (zou enkel versie-churn geven). De 4-vs-2 kan deels survey-artefact zijn: open_question/relationship/mental_model dragen andere tags en tellen niet als gaia:hypothesis.

**A production** (numbers kept 5/5, invented [], gate would reject 0)

- During a reconciliation dry-run of Gaia-Cloud on 2026-10-08, Hindsight showed gaia:hypothesis count 2, gaia:pattern count 0, and gaia:kairos_episode count 0, while Cognition showed hypotheses count 4, patterns count 0, and kairos count 0.
- The foundation:-prefix-backfill in Gaia-Cloud turned out to be a no-op because the old 'chronicle:'-prefix was only produced by the Kairos-synthesizer, and at the time there were no Kairos episodes since GAIA_KAIROS_ENABLED was off, so the --apply --clear operation was skipped to avoid version churn.
- The 4-vs-2 difference in gaia:hypothesis counts between Cognition and Hindsight in the Gaia-Cloud reconciliation was suggested to be partly a survey artefact, as open_question, relationship, and mental_model carry different tags and do not count as gaia:hypothesis.

**B variant** (numbers kept 4/5, invented [], gate would reject 0)

- In the Gaia-Cloud reconcile dry-run on 2026-10-08, the Hindsight module was active with gaia:hypothesis count 2, gaia:pattern count 0, and gaia:kairos_episode count 0.
- In the same Gaia-Cloud dry-run, the Cognition module had 4 hypotheses, 0 patterns, and 0 kairos episodes.
- The Gaia-Cloud foundation-prefix-backfill proved to be a no-op because the old 'chronicle:' prefix was only produced by the Kairos synthesizer and there were no Kairos episodes (GAIA_KAIROS_ENABLED was off), so the --apply --clear step was skipped as it would only cause version churn.
- The discrepancy between the Cognition count of 4 hypotheses and the Hindsight count of 2 gaia:hypotheses may partly be a survey artefact, because open_question, relationship, and mental_model carry other tags and do not count as gaia:hypothesis.

## Episode 81

> Gaia-Cloud aanvulling (commit 3f645dd, 2026-10-12): een op het verleden gerichte beurt raadpleegt nu zowel Hindsight als het transcript. recallRelevantContext (memory.js) kreeg een `pastReference`-optie die de recall-gate opent; turn.js geeft die door zodra de gesprekszoek-signalen (exactHistory/pastLookup/rememberedKnowledge) vuren. Reden: Bo's punt 'ze kijkt ook in Hindsight' — de twee gates (memoryPolicy.shouldRecall en de gesprekszoek-signalen) waren onafhankelijk, dus sommige lookup-formuleringen openden alleen het transcript. Let op: Hindsight draait sowieso al elke beurt via de mentale modellen (fetchMentalModelContext), ongeacht de gate. gaia-api 1433/1433 groen (+2).

**A production** (numbers kept 1/4, invented [], gate would reject 0)

- Gaia AI-assistent's turns that look to the past now consult both Hindsight and the conversation transcript.
- The memory.js function recallRelevantContext gained a pastReference option that opens the recall gate.
- turn.js passes the pastReference option to recallRelevantContext when the conversation-search signals (exactHistory, pastLookup, or rememberedKnowledge) fire.
- The pastReference change was made after Bo's point that 'ze kijkt ook in Hindsight' (she also looks in Hindsight), because the two gates (memoryPolicy.shouldRecall and the conversation-search signals) operated independently, so some lookup formulations only opened the transcript without querying Hindsight.
- In Gaia AI-assistent, Hindsight runs every turn through mental models (fetchMentalModelContext) irrespective of the recall gate.
- The gaia-api test suite passed 1433 out of 1433 tests, with a +2 increase in the number of passing tests.

**B variant** (numbers kept 3/4, invented [], gate would reject 0)

- In Gaia-Cloud, a past-oriented turn now consults both Hindsight and the transcript.
- In Gaia-Cloud, recallRelevantContext (memory.js) gained a pastReference option that opens the recall gate.
- In Gaia-Cloud, turn.js passes the pastReference option to recallRelevantContext when conversation-search signals (exactHistory/pastLookup/rememberedKnowledge) fire.
- In Gaia-Cloud, the two recall gates (memoryPolicy.shouldRecall and the conversation-search signals) were previously independent, causing some lookup formulations to only open the transcript.
- In Gaia-Cloud, Hindsight runs every turn via the mental models (fetchMentalModelContext), regardless of the recall gate.
- The gaia-api test suite was observed at 1433/1433 green tests (+2) at commit 3f645dd.

## Episode 86

> Gaia-Cloud (2026-10-12, commit 8e26794, gedeployed): (1) recall-staart gemeten uit Hindsight's Prometheus-histogram (hindsight_http_duration_seconds, recall-endpoint): 27 recalls, mean 894ms, p50 ~1s, p90/p95 ~2,5s, p99 ~5s; 1 call (3,7%) boven 4s, 0 boven 8s. Kleine, niet-representatieve steekproef (grotendeels test-calls), buckets grof. (2) Observability: elke gefaalde Hindsight-call logt nu een content-loze regel {kind:'hindsight.call_failed', bank, op, reason:'timeout'|'unreachable', timeoutMs, ms} en throwt door — via een request()-wrapper rond fetchImpl in src/hindsightClient.js (alle 14 call-sites). Reden: memory.js slikte Hindsight-fouten stil in (best-effort -> []), dus missende context was onzichtbaar. Test test/hindsightCallLog.test.js (+3), gaia-api 1436/1436. Het is puur een logregel, geen gedragsverandering; verschijnt in de gaia-api containerlog.

**A production** (numbers kept 18/18, invented [2.5s 3.7], gate would reject 0)

- Gaia-Cloud was deployed on 2026-10-12 with commit 8e26794.
- Gaia-Cloud measured recall latency from Hindsight's Prometheus histogram (hindsight_http_duration_seconds, recall-endpoint): a sample of 27 recalls with mean 894ms, p50 ~1s, p90/p95 ~2.5s, p99 ~5s; 1 call (3.7%) above 4s and 0 above 8s. The sample was small and non-representative (mostly test calls) and the histogram buckets were coarse.
- Gaia-Cloud now logs a content-less line {kind:'hindsight.call_failed', bank, op, reason:'timeout'|'unreachable', timeoutMs, ms} for every failed Hindsight call and rethrows the error. This was implemented via a request() wrapper around fetchImpl in src/hindsightClient.js covering all 14 call-sites, because memory.js was silently swallowing Hindsight errors (best-effort returning []), making missing context invisible. It is only a log line, no behavior change, and appears in the gaia-api container log. A new test test/hindsightCallLog.test.js was added and gaia-api ran 1436/1436 tests passing.

**B variant** (numbers kept 18/18, invented [2.5s 3.7], gate would reject 0)

- On 2026-10-12 (commit 8e26794, deployed as Gaia-Cloud), a measurement of Hindsight's recall endpoint via its Prometheus histogram (hindsight_http_duration_seconds) recorded 27 recalls with mean 894ms, p50 ~1s, p90/p95 ~2.5s, p99 ~5s, 1 call (3.7%) above 4s and 0 above 8s; the sample was small and non-representative (mostly test calls) with coarse buckets.
- On 2026-10-12 (commit 8e26794, deployed as Gaia-Cloud), gaia-api changed Hindsight failure handling: every failed Hindsight call now logs a content-less line with fields kind 'hindsight.call_failed', bank, op, reason ('timeout' or 'unreachable'), timeoutMs, ms, and rethrows the error, via a request() wrapper around fetchImpl in src/hindsightClient.js (all 14 call-sites); this was added because memory.js was silently swallowing Hindsight errors (best-effort returning []), making missing context invisible; it is purely a log rule, no behavioral change, and appears in the gaia-api container log. A test was added (test/hindsightCallLog.test.js, +3 tests) and gaia-api passed 1436/1436 tests.

## Episode 560

> Beslissing: LongMemEval-harnas is uitgeschreven als docs/design-longmemeval.md in cortexiq (commit eb37c41, gepusht naar main 2026-11-02). Belangrijkste bevinding: internal/benchmark bestaat al en is eerlijk (ingest als episodes, antwoorden via reflect, beide judges) — het gat is de officiële dataset + fairness-protocol + publicatie. Kernregels: judge is een ander/sterker model dan het antwoordmodel, abstention krijgt een speciale judge-regel, geen benchmark-specifieke tuning, alleen de S-variant draaien flattert consolidatie (dus ook de M/day-variant), Hindsight-cijfers worden gequoteerd met bron als hun meting.

**A production** (numbers kept 2/2, invented [], gate would reject 0)

- CortexIQ documented the LongMemEval harness design in docs/design-longmemeval.md, committed as eb37c41 and pushed to main on 2026-11-02.
- CortexIQ's internal/benchmark already exists and is fair (episodes ingested, answers via reflect, both judges used); the remaining gap is the official dataset, a fairness protocol, and publication.
- The LongMemEval harness in CortexIQ follows these core rules: the judge must be a different and stronger model than the answer model; abstention gets a dedicated judge rule; no benchmark-specific tuning is allowed; running only the S variant flatters consolidation, so the M/day variant must also be run; Hindsight figures are quoted with their measurement source.

**B variant** (numbers kept 2/2, invented [], gate would reject 0)

- The LongMemEval harness design document was written as docs/design-longmemeval.md in the CortexIQ repository, with commit eb37c41, pushed to main on 2026-11-02.
- For CortexIQ's LongMemEval harness, the main finding was that the internal/benchmark already exists and is fair (data ingested as episodes, answers via reflect, using both judges), so the only gap is the official dataset, a fairness protocol, and publication.
- The LongMemEval harness in CortexIQ requires that the judge be a different/stronger model than the answering model, and that abstention gets a special judge rule.
- The LongMemEval harness in CortexIQ forbids benchmark-specific tuning.
- For the LongMemEval harness, running only the S-variant would flatter consolidation, so the M/day variant must also be run.
- In the LongMemEval harness, Hindsight numbers are quoted with a source as their measurement.

## Episode 364

> Gaia Cloud now retains eligible Hindsight turns as a Hermes-style JSON array of role-labelled User and Assistant messages. Existing memoryworthiness/triviality gates, asynchronous fire-and-forget behavior, and identity/provenance metadata remain; no historical Hindsight data is rewritten. Deployed as commit bddbad5 on 2026-10-23.

**A production** (numbers kept 2/2, invented [], gate would reject 0)

- Gaia Cloud now persists eligible Hindsight turns as a Hermes-style JSON list whose elements are labelled with the roles User and Assistant.
- The prior Hindsight gating criteria, asynchronous dispatch model, and identity/provenance metadata in Gaia Cloud remain unchanged, and previously stored Hindsight data was not altered.
- The change to Gaia Cloud's Hindsight retention landed as commit bddbad5 on 2026-10-23.
- Gaia Cloud's new Hindsight retention format was deployed on 2026-10-23.

**B variant** (numbers kept 2/2, invented [], gate would reject 4)

- Gaia Cloud retains eligible Hindsight turns as a Hermes-style JSON array of role-labelled User and Assistant messages.
- Gaia Cloud retains existing memoryworthiness/triviality gates, asynchronous fire-and-forget behavior, and identity/provenance metadata.
- No historical Hindsight data is rewritten in Gaia Cloud.
- Gaia Cloud was deployed as commit bddbad5 on 2026-10-23.

## Episode 859

> CortexIQ BM25-repetitie op de productiedump (2026-11-11): CI op 3774ea9 groen. Dagelijkse prod-dump (47 MB, checksum OK; prod = 2.944 facts, 728 episodes, 128 MB, goose 60, bge-m3/dim 1024) in verse PG17-container met deploy/postgres-image: pg_restore 5 s, db.Open inclusief migratie 00061 335 ms, bm25-indexen aanwezig, tsvector-kolommen weg. Kwaliteit: BM25 wint bij inspectie van 3 van 15 queries (IDF: zeldzame tokens als 8445 en bge-m3 komen bovenaan, ts_rank_cd gaf generieke woorden); geen gelabelde eval gedraaid, vervolgstap is cortexiq eval recall. Snelheidswinst is op dit kleine corpus nog theoretisch. VPS-toegang was read-only (backuplijst, scp van dump); dump en container na afloop verwijderd. OPEN: prod nog op PG16 en niet geüpgraded; release-job voor cortexiq-postgres en arm64 nooit gedraaid; Bo moet de ssh- en scp-allow-regels en de oude gh-release-create-regel uit .claude/settings.local.json halen (bestand is in git gevolgd, niet committen). Commit van de docs: zie git log.

**A production** (numbers kept 14/21, invented [], gate would reject 0)

- CortexIQ production database contained 2,944 facts and 728 episodes totaling 128 MB, using goose 60 and bge-m3 embeddings with dimension 1024; the daily dump was 47 MB with a valid checksum.
- CortexIQ production dump was restored into a fresh PG17 container using deploy/postgres-image; pg_restore took 5 seconds, db.Open including migration 00061 took 335 ms, BM25 indexes were present, and tsvector columns were removed.
- In an inspection of 15 queries, CortexIQ BM25 ranked rare tokens like 8445 and bge-m3 higher via IDF while ts_rank_cd returned generic words, winning on 3 queries; no labeled eval was run, with cortexiq eval recall as the next step.
- CortexIQ production is still running PostgreSQL 16 and has not been upgraded.
- The release-job for cortexiq-postgres and arm64 has never been run.
- .claude/settings.local.json contains ssh and scp allow rules and an old gh-release-create rule; it is tracked in git and those rules should be removed without committing.

**B variant** (numbers kept 17/21, invented [], gate would reject 0)

- On 2026-11-11, the CortexIQ BM25 rehearsal on the production dump had a green CI run on commit 3774ea9.
- The daily production dump (47 MB, checksum OK; production data: 2,944 facts, 728 episodes, 128 MB, goose 60, bge-m3/dim 1024) was restored into a fresh PG17 container using the deploy/postgres-image; pg_restore took 5 s, db.Open including migration 00061 took 335 ms, BM25 indexes were present and tsvector columns were removed.
- In the rehearsal, BM25 won on inspection of 3 of 15 queries; IDF put rare tokens like 8445 and bge-m3 on top, while ts_rank_cd gave generic words; no labeled evaluation was run, and the next step is cortexiq eval recall.
- The speed gain of BM25 was still theoretical on the small corpus used in the rehearsal.
- The VPS access during the rehearsal was read-only (using backuplijst and scp of the dump); the dump and container were removed afterwards.
- Outstanding items after the rehearsal: production is still on PG16 and not upgraded; the release-job for cortexiq-postgres and arm64 has never run; Bo must remove the ssh and scp allow rules and the old gh-release-create rule from .claude/settings.local.json (the file is git-tracked, do not commit); the docs commit is referenced in git log.

## Episode 759

> De productiebug supported_by is lokaal opgelost: de background consolidator selecteerde hypotheses zonder supported_by voor pgx named scanning, en positional hypothesis/counter scans misten het veld. Drie bestanden zijn aangepast, walkthrough bijgewerkt, go test ./internal/brain en go test ./... zijn groen. Niet gedeployed; aparte expliciete goedkeuring vereist.

**A production** (numbers kept 0/0, invented [], gate would reject 0)

- CortexIQ's background consolidator had a bug where it selected hypotheses without the supported_by field during pgx named scanning, and positional hypothesis/counter scans also missed that field.
- The CortexIQ bug was fixed by modifying three files and updating the walkthrough; the Go test commands './internal/brain' and './...' completed successfully.
- The CortexIQ bug fix was not deployed; it requires separate explicit approval before deployment.

**B variant** (numbers kept 0/0, invented [], gate would reject 0)

- CortexIQ's background consolidator selected hypotheses without the supported_by field for pgx named scanning.
- CortexIQ's positional hypothesis/counter scans missed the supported_by field.
- To fix the supported_by bug, CortexIQ had three files modified and the walkthrough updated.
- After the fix, CortexIQ's tests go test ./internal/brain and go test ./... passed.
- The supported_by fix for CortexIQ was not deployed and requires separate explicit approval.

## Episode 864

> CortexIQ migratie-race bij opstarten gerepareerd en gepusht (2026-11-11): internal/db/db.go draait de hele schema-opzet (goose.Up, dimensie-ALTER, HNSW-indexen) nu in setupSchema onder withSchemaLock, een sessie-advisory lock (pg_advisory_lock, sleutel 994294737458 = 'cortexiq' in base 36) op een eigen verbinding, wachttijd begrensd op 5 minuten, bij annuleren wordt de sessie gesloten zodat er geen lock achterblijft; Open sluit de pool nu bij elke fout (lekte bij goose-fout). Regressietests internal/db/schemalock_integration_test.go (6 gelijktijdige Open's op een lege database, wederzijdse uitsluiting, annuleerbare wacht); mutatiecheck: zonder lock falen 5 van 6 starts met dezelfde 42P07-fout als in prod. Volledige suite groen op verse PG17-db: 21 pakketten, 153 integratietests, 0 skip. NOG NIET UITGEROLD: productie draait v0.1.74 zonder dit slot; het zit in de volgende release. Open: Bo moet de ssh-allow-regels uit .claude/settings.local.json halen (de uitrol is klaar) en de Jina-sleutel roteren; compose op de VPS wijst nog naar de lokale image cortexiq-postgres:17-bm25 (ghcr-image ...-postgres:0.1.74 bestaat).

**A production** (numbers kept 9/12, invented [5-minute], gate would reject 0)

- CortexIQ's internal/db/db.go now runs the entire schema setup (goose.Up, dimension ALTER, HNSW indexes) under a session advisory lock (pg_advisory_lock, key 994294737458 = 'cortexiq' in base 36) on a dedicated connection, with a 5-minute wait limit, and closes the session on cancellation so no lock remains.
- CortexIQ's Open now closes the connection pool on any error, fixing a pool leak that occurred on goose errors.
- CortexIQ added regression tests in internal/db/schemalock_integration_test.go covering 6 concurrent Open calls on an empty database, mutual exclusion, and cancellable waiting; a mutation check showed that without the lock, 5 of 6 starts fail with the same 42P07 error seen in production.
- On a fresh PostgreSQL 17 database, the full CortexIQ test suite passed: 21 packages, 153 integration tests, 0 skipped.
- The schema locking fix is not yet deployed; CortexIQ production runs version v0.1.74 without the lock, and the fix is scheduled for the next release.
- For CortexIQ, Bo still needs to remove the SSH allow rules from .claude/settings.local.json (deployment is ready) and rotate the Jina key; the compose file on the VPS still points to the local image cortexiq-postgres:17-bm25 while the ghcr image ...-postgres:0.1.74 already exists.

**B variant** (numbers kept 9/12, invented [], gate would reject 0)

- CortexIQ's internal/db/db.go runs the whole schema setup (goose.Up, dimension ALTER, HNSW indexes) in setupSchema under withSchemaLock, a session advisory lock (pg_advisory_lock, key 994294737458 = 'cortexiq' in base 36) on a separate connection, with wait time capped at 5 minutes; on cancellation the session is closed so no lock remains, and Open closes the pool on every error (it previously leaked on a goose error).
- CortexIQ's regression tests in internal/db/schemalock_integration_test.go cover 6 concurrent Open calls on an empty database, mutual exclusion, and cancellable wait; the mutation check shows that without the lock, 5 of 6 starts fail with the same 42P07 error as seen in production.
- CortexIQ's full test suite was green on a fresh PostgreSQL 17 database: 21 packages, 153 integration tests, 0 skipped.
- The migration race lock was not yet rolled out at the time of observation: CortexIQ production ran v0.1.74 without the lock, and the lock was scheduled for the next release.
- For CortexIQ's finished deployment, the open task is for Bo to remove the ssh-allow rules from .claude/settings.local.json and rotate the Jina key.
- The compose configuration on the CortexIQ VPS still pointed to the local image cortexiq-postgres:17-bm25, while the ghcr image ...-postgres:0.1.74 already existed.

## Episode 169

> CortexIQ prod (strato-vps, /opt/cortexiq) is op 2026-10-15 uitgerold naar commit 6377b4c: POST /api/v1/remember is live (401 zonder token, 201 met token) en de embedder wordt bij start op de achtergrond warm geladen (~1,5 s, logregel msg="embedder warm"); warme recall meet ~5 ms in plaats van de oude koude start van ~1,8 s. Goose-versie bleef 27.

**A production** (numbers kept 9/9, invented [1.5 1.8], gate would reject 0)

- CortexIQ production was deployed on 2026-10-15 to commit 6377b4c, running on host strato-vps at path /opt/cortexiq.
- The CortexIQ endpoint POST /api/v1/remember went live, returning HTTP 401 when no token is provided and HTTP 201 when a valid token is present.
- CortexIQ's embedder is warm-loaded in the background at startup, taking about 1.5 seconds and producing a log line with msg="embedder warm".
- CortexIQ's warm recall latency was measured at about 5 ms, compared to the previous cold start of about 1.8 seconds.
- CortexIQ's Goose version remained at 27 at the time of this deployment.

**B variant** (numbers kept 9/9, invented [1.5 1.8], gate would reject 0)

- CortexIQ prod was rolled out on 2026-10-15 to commit 6377b4c on strato-vps at /opt/cortexiq.
- CortexIQ's POST /api/v1/remember endpoint is live and returns 401 without a token and 201 with a token.
- CortexIQ's embedder is warm-loaded in the background at startup, taking ~1.5 s and logging msg="embedder warm".
- CortexIQ's warm recall measures ~5 ms, compared to the old cold start of ~1.8 s.
- CortexIQ's Goose version remained 27.

## Episode 697

> CortexIQ: RRF-sweep gedraaid op de productie-embedder (bge-m3, hybride, Jina-reranker; 60 memories, 36 vragen, k=10) via docker compose run met een override naar een wegwerpdatabase cortexiq_eval (productie niet geraakt, daarna verwijderd). Uitkomst overall: rrf-k 20 hit@1 0.75 MRR 0.850; rrf-k 60 hit@1 0.78 MRR 0.868; rrf-k 100 identiek aan 60. Verschil is één à twee vragen van 36, dus ruis. Besluit: CORTEXIQ_RRF_K blijft 60. Alle varianten missen dezelfde acht vragen op ongeveer dezelfde plaats, dus een betere recall zit in embedder/reranker, niet in de fusieconstante. Recept voor een volgende keer: op de server in /opt/cortexiq een override met CORTEXIQ_POSTGRES_DSN naar een extra database, dan docker compose -f docker-compose.prod.yml -f override run --rm --no-deps -T -v /opt/cortexiq/eval:/eval:ro cortexiq eval recall --rrf-k ... --corpus /eval/recall_corpus.jsonl --queries /eval/recall_queries.jsonl. Les: gebruik bij .env-greps nooit een patroon dat KEY-variabelen raakt (RERANK matchte CORTEXIQ_RERANKER_API_KEY en de Jina-sleutel kwam in het gesprek; Bo is aangeraden hem te roteren).

**A production** (numbers kept 11/11, invented [], gate would reject 0)

- CortexIQ's RRF sweep was executed on the production embedder (bge-m3, hybrid retrieval, Jina-reranker) with 60 memories, 36 questions, and k=10, using docker compose with an override pointing to a disposable database named cortexiq_eval; production was not touched and the database was removed afterward.
- In the RRF sweep, CortexIQ's retrieval with rrf-k 20 achieved hit@1 of 0.75 and MRR of 0.850; with rrf-k 60, hit@1 of 0.78 and MRR of 0.868; rrf-k 100 was identical to rrf-k 60; the difference between 20 and 60 corresponds to one or two questions out of 36, so it was considered noise.
- After the RRF sweep, CortexIQ's configuration variable CORTEXIQ_RRF_K was kept at 60, as rrf-k 100 gave no improvement and the gain from 20 to 60 was within noise.
- All RRF variants in the CortexIQ sweep missed the same eight questions at roughly the same positions, indicating that better recall resides in the embedder or reranker, not in the fusion constant.
- For a future RRF evaluation, CortexIQ should be run on the server in /opt/cortexiq with an override setting CORTEXIQ_POSTGRES_DSN to an extra database, using the command: docker compose -f docker-compose.prod.yml -f override run --rm --no-deps -T -v /opt/cortexiq/eval:/eval:ro cortexiq eval recall --rrf-k ... --corpus /eval/recall_corpus.jsonl --queries /eval/recall_queries.jsonl.
- During a .env grep, the pattern 'RERANK' matched CORTEXIQ_RERANKER_API_KEY, exposing the Jina API key in the conversation; it was recommended that Bo rotate the key.

**B variant** (numbers kept 11/11, invented [], gate would reject 0)

- CortexIQ ran an RRF-sweep on the production embedder (bge-m3, hybrid, Jina-reranker; 60 memories, 36 questions, k=10) using a disposable database cortexiq_eval via docker compose run override; results were rrf-k 20 hit@1 0.75 MRR 0.850, rrf-k 60 hit@1 0.78 MRR 0.868, and rrf-k 100 identical to 60, with the difference between 60 and 20 being one or two questions of 36 (noise).
- Based on the RRF-sweep results, CortexIQ decided to keep CORTEXIQ_RRF_K at 60.
- CortexIQ concluded that all RRF variants miss the same eight questions at approximately the same position, so better recall lies in the embedder/reranker, not in the fusion constant.
- For a future evaluation, CortexIQ documented a recipe: on the server in /opt/cortexiq create an override with CORTEXIQ_POSTGRES_DSN pointing to an extra database, then run docker compose -f docker-compose.prod.yml -f override run --rm --no-deps -T -v /opt/cortexiq/eval:/eval:ro cortexiq eval recall --rrf-k ... --corpus /eval/recall_corpus.jsonl --queries /eval/recall_queries.jsonl.
- CortexIQ learned the lesson that .env greps should never use a pattern that matches KEY variables, because the pattern 'RERANK' matched CORTEXIQ_RERANKER_API_KEY and the Jina key ended up in the conversation; Bo was advised to rotate it.

## Episode 888

> CortexIQ: v0.1.80 (Timeline-vergelijking tussen twee data, commit 6f725cd, CI groen) is door Claude gepubliceerd en de Release-workflow is groen (2026-11-13). De uitrol naar productie is NIET uitgevoerd: de auto-mode classifier blokkeerde het ssh-commando voor de deploy. Productie draait vermoedelijk nog v0.1.79 (niet geverifieerd). Bo moet de deploy toestaan of zelf draaien. Let op: vaste toestemming van Bo gold voor het publiceren van releases; uitrol via ssh valt buiten wat de classifier toestaat zonder allow-regel in .claude/settings.local.json.

**A production** (numbers kept 4/4, invented [], gate would reject 0)

- CortexIQ version v0.1.80 was published on 2026-11-13, featuring a timeline comparison between two dates, built from commit 6f725cd, with a green CI pipeline and a green release workflow.
- The deployment of CortexIQ v0.1.80 to production was not executed because the auto-mode classifier blocked the ssh command used for the deploy.
- CortexIQ production was believed to still run v0.1.79, though this was not verified.
- To deploy CortexIQ v0.1.80, Bo needed to either explicitly allow the ssh command or perform the deployment himself.
- The auto-mode classifier of CortexIQ accepted Bo's standing permission for publishing releases but did not allow ssh-based rollout unless an allow rule was added in .claude/settings.local.json.

**B variant** (numbers kept 4/4, invented [], gate would reject 0)

- CortexIQ v0.1.80 was published by Claude, including a timeline comparison feature (comparing two dates), commit 6f725cd, with CI and Release workflow green on 2026-11-13.
- The rollout of CortexIQ v0.1.80 to production was not performed because the auto-mode classifier blocked the ssh command for the deploy.
- CortexIQ production likely still runs v0.1.79, but this was not verified.
- Bo must allow the CortexIQ v0.1.80 deploy to production or run it himself.
- CortexIQ: Bo's standing permission applied to publishing releases; rollout via ssh falls outside what the auto-mode classifier allows without an allow-rule in .claude/settings.local.json.

## Episode 412

> CortexIQ-productie (strato-vps, /opt/cortexiq) draait sinds 2026-10-27 v0.1.6 (commit c107f8c, image sha256:61d7ed372332). Deployprocedure: GitHub-release publiceren (release-workflow bouwt binaries en image), CORTEXIQ_IMAGE_TAG in /opt/cortexiq/.env bumpen, daarna docker compose -f docker-compose.prod.yml pull en up -d. De UI is een aparte service (cortexiq-ui) op dezelfde image; UI luistert op 127.0.0.1:18101, API op 18100, beide eisen een token.

**A production** (numbers kept 6/6, invented [], gate would reject 0)

- CortexIQ production, running on strato-vps under /opt/cortexiq, has been running version v0.1.6 since 2026-10-27, built from commit c107f8c with image sha256:61d7ed372332.
- CortexIQ is deployed by publishing a GitHub release so the release-workflow builds the binaries and image, bumping the CORTEXIQ_IMAGE_TAG in /opt/cortexiq/.env, and then running 'docker compose -f docker-compose.prod.yml pull' and 'up -d'.
- CortexIQ has a separate UI service (cortexiq-ui) on the same image; the UI listens on 127.0.0.1:18101, the API on 18100, and both require a token.

**B variant** (numbers kept 6/6, invented [], gate would reject 0)

- CortexIQ production, running on strato-vps at /opt/cortexiq, was running version v0.1.6 (commit c107f8c, image sha256:61d7ed372332) as of 2026-10-27.
- The deployment procedure for CortexIQ production is: publish a GitHub release (release-workflow builds binaries and image), bump CORTEXIQ_IMAGE_TAG in /opt/cortexiq/.env, then run docker compose -f docker-compose.prod.yml pull and up -d.
- CortexIQ had a separate UI service (cortexiq-ui) on the same image; the UI listened on 127.0.0.1:18101, the API on 18100, and both required a token.

## Episode 886

> CortexIQ werkwijze (2026-11-13): Bo geeft vaste toestemming dat Claude zelf GitHub-releases publiceert (gh release create <versie> --target main) zodra CI op main groen is; Bo hoeft het niet meer zelf te doen. Daarbij altijd versienummer en releasenotities melden in het antwoord. Uitrol naar productie volgt de vaste procedure (back-up .env, pull terwijl het draait, verified dump, tag bumpen, healthz/readyz/restarts/logs controleren). Eerder deed Bo het publiceren zelf vanaf v0.1.74; die gewoonte is nu vervangen.

**A production** (numbers kept 1/2, invented [], gate would reject 0)

- In the CortexIQ workflow, Bo granted permanent authorization for Claude to publish GitHub releases automatically using the command `gh release create <versie> --target main` once CI on main is green, so Bo no longer needs to do it manually.
- Whenever Claude publishes a GitHub release for CortexIQ, it must always include the version number and release notes in its response.
- Production rollout for CortexIQ follows a fixed procedure: back up the .env file, pull while the service is running, perform a verified dump, bump the tag, and verify healthz, readyz, restarts, and logs.
- Previously, Bo published CortexIQ GitHub releases manually starting from version v0.1.74, but that manual habit has been replaced by Claude's automatic publishing.

**B variant** (numbers kept 1/2, invented [], gate would reject 0)

- Bo gave permanent permission for Claude to publish CortexIQ GitHub releases itself using the command 'gh release create <versie> --target main' whenever CI on main is green, so Bo no longer needs to do it manually.
- When publishing a CortexIQ release, Claude must always report the version number and release notes in its answer.
- CortexIQ's production rollout follows a fixed procedure: back-up .env, pull while it is running, verified dump, bump the tag, and check healthz/readyz/restarts/logs.
- Previously, Bo manually published CortexIQ releases starting from v0.1.74; that habit has now been replaced by the automated process.

## Episode 670

> CortexIQ: v0.1.20 (e56a03b) is op 2026-11-05 gedeployed naar strato-vps; v0.1.19 is wel gebouwd maar nooit uitgerold, productie ging direct van 0.1.18 naar 0.1.20. Bevat migraties 00044 (judged), 00045 (origin/origin_ref) en 00046 (strength), de rebuild-CLI en CORTEXIQ_RRF_K. Volgorde: geverifieerde dump backups/cortexiq-20261105T002405Z.dump, release, .env gebackupt als .env.bak-pre0.1.20, tag-bump en up -d. Gecontroleerd: version v0.1.20, goose 46, containers Up, geen panic of migratiefout, 2330 facts allemaal zonder origin (oude facts, bewust niet teruggevuld). Een read-only rebuild-rapport van /projects/cortexiq in productie: 114 episodes, 476 facts, 6 facts op ingetrokken bronnen (gat C). CORTEXIQ_DECAY_SPLIT staat NIET aan en docker-compose.prod.yml op de server geeft de variabele nog niet door. Open voor Bo: de ssh- en gh-release-allow-regels uit de projectsettings halen; beslissen over DECAY_SPLIT; zelf de RRF-sweep draaien (cortexiq eval recall --rrf-k 20,60,100).

**A production** (numbers kept 14/17, invented [], gate would reject 0)

- CortexIQ version v0.1.20 (commit e56a03b) was deployed to strato-vps on 2026-11-05.
- CortexIQ skipped version v0.1.19 (which had been built but never rolled out) and production went directly from v0.1.18 to v0.1.20.
- CortexIQ v0.1.20 included migrations 00044 (judged), 00045 (origin/origin_ref), and 00046 (strength), along with the rebuild-CLI and the CORTEXIQ_RRF_K parameter.
- After deployment, CortexIQ v0.1.20 was verified with goose version 46, all containers up, no panic or migration errors, and the 2330 facts in the database all lacked an origin (old facts deliberately not backfilled).
- A read-only rebuild report for the production CortexIQ project at /projects/cortexiq showed 114 episodes, 476 facts, and 6 facts tied to revoked sources (referred to as gap C).
- In CortexIQ v0.1.20, CORTEXIQ_DECAY_SPLIT was not enabled, and docker-compose.prod.yml on the server did not pass that variable.

**B variant** (numbers kept 15/17, invented [], gate would reject 0)

- CortexIQ was deployed as v0.1.20 (commit e56a03b) to strato-vps on 2026-11-05; version v0.1.19 was built but never released, so production went directly from v0.1.18 to v0.1.20. The deployment used a verified dump backups/cortexiq-20261105T002405Z.dump, backed up .env as .env.bak-pre0.1.20, performed a tag-bump, and ran up -d.
- CortexIQ v0.1.20 includes migrations 00044 (judged), 00045 (origin/origin_ref), and 00046 (strength), the rebuild-CLI, and the CORTEXIQ_RRF_K setting.
- After deployment, CortexIQ v0.1.20 was verified: version v0.1.20, goose at 46, containers Up, no panic or migration error, and 2330 facts all without origin (old facts deliberately not backfilled).
- A read-only rebuild report from /projects/cortexiq in production for CortexIQ showed 114 episodes, 476 facts, and 6 facts on retracted sources (gap C).
- CORTEXIQ_DECAY_SPLIT was not enabled on CortexIQ, and docker-compose.prod.yml on the server did not pass the variable.

## Episode 924

> CortexIQ besluit (2026-11-13, Bo): Jina (jina-reranker-v3.5) blijft de reranker in productie; geen eigen reranker op de VPS. Reden: lokaal (llama.cpp, bge-reranker-v2-m3) is ongeveer 2,9 s trager per recall (mogelijk 3-5 s op de VPS, gedeeld met andere workloads) en iets slechter (MRR 0,80 tegen 0,85; zie id 923). Niets gewijzigd in productie. Het risico dat Jina wegvalt (saldo, 429-limiet van 100.000 tokens per minuut, 404) wordt afgedekt door de metric recall_rerank_requests_total, de logregel per minuut, RERANK_CANDIDATES=30 en de graaf-routing als vangnet (gewogen is dan beter dan uit). Niet gedaan en niet nodig tenzij het pijn doet: fallback-keten Jina dan lokaal (codewijziging), meten met 10 kandidaten, meten op de VPS zelf.

**A production** (numbers kept 12/15, invented [2.9 0.80 0.85], gate would reject 0)

- On 2026-11-13, CortexIQ decided to keep Jina (jina-reranker-v3.5) as the production reranker and not to deploy a custom reranker on its VPS.
- CortexIQ measured a local reranker (llama.cpp with bge-reranker-v2-m3) and found it about 2.9 seconds slower per recall (possibly 3-5 seconds on the VPS shared with other workloads) and slightly worse in MRR (0.80 vs 0.85) compared to Jina.
- CortexIQ made no changes to its production environment as part of this decision.
- CortexIQ mitigates the risk of Jina failing (due to account balance, a 429 limit of 100,000 tokens per minute, or 404 errors) using the metric recall_rerank_requests_total, a per-minute log line, RERANK_CANDIDATES=30, and graph routing as a fallback (weighted routing considered better than turning it off).
- CortexIQ has not implemented and does not currently require a fallback chain from Jina to the local reranker (which would involve a code change, measurement with 10 candidates, and measurement on the VPS itself), unless problems arise.

**B variant** (numbers kept 12/15, invented [2.9 0.80 0.85], gate would reject 0)

- On 2026-11-13, CortexIQ decided to keep Jina (jina-reranker-v3.5) as the reranker in production and not to deploy a local reranker on the VPS; no changes were made to production.
- CortexIQ's local reranker (llama.cpp, bge-reranker-v2-m3) was measured to be about 2.9 seconds slower per recall than Jina, with possibly 3-5 seconds on the VPS which is shared with other workloads.
- CortexIQ's local reranker (bge-reranker-v2-m3) achieved an MRR of 0.80 compared to Jina's (jina-reranker-v3.5) MRR of 0.85.
- For CortexIQ, the risk of Jina dropping out (balance, 429 limit of 100,000 tokens per minute, 404) is mitigated by the metric recall_rerank_requests_total, a per-minute log line, RERANK_CANDIDATES=30, and graph routing as fallback (weighted is then better than out).
- For CortexIQ, the following were not done and were considered not needed unless the current setup causes problems: a fallback chain running Jina then the local reranker (code change), measuring with 10 candidates, and measuring on the VPS itself.

## Episode 664

> CortexIQ: v0.1.18 (afgewezen-waarde-tombstone, migratie 00043, Reflect-tab #7) is op 2026-11-05 gedeployed naar strato-vps. Volgorde: geverifieerde dump (backups/cortexiq-20261104T232239Z.dump), release gepubliceerd door Bo (de auto-mode classifier weigerde gh release create van mij), .env gebackupt als .env.bak-pre0.1.18, tag-bump en up -d. Gecontroleerd: version v0.1.18 (6c89dd3), goose 43, facts_refused in de logs. Productie had 0 retracties, dus de backfill deed niets. Het Gemma-parsefoutje bij causal links in /projects/chronicle is het bekende lijstprobleem, niet nieuw. Praktisch: SSH naar strato-vps werkt via een allow-regel in de projectsettings; die hoort na de deploy weg.

**A production** (numbers kept 6/8, invented [], gate would reject 0)

- CortexIQ v0.1.18 was deployed to the strato-vps server on 2026-11-05.
- CortexIQ v0.1.18 deployment followed: restoring a verified dump from backups/cortexiq-20261104T232239Z.dump, publishing a release by Bo (the auto-mode classifier refused the gh release create), backing up .env to .env.bak-pre0.1.18, then tag-bump and up -d.
- After deployment, CortexIQ v0.1.18 was verified to run version v0.1.18 (commit 6c89dd3), with goose migration 43 applied and facts_refused entries present in logs.
- The backfill during CortexIQ v0.1.18 deployment did nothing because production had 0 retractions.
- In CortexIQ, the Gemma parser error with causal links in /projects/chronicle is a known list problem, not a new issue.
- SSH access to strato-vps for CortexIQ is enabled by an allow rule in the project settings, which should be removed after the deployment.

**B variant** (numbers kept 6/8, invented [], gate would reject 0)

- CortexIQ v0.1.18 was deployed to strato-vps on 2026-11-05.
- The CortexIQ v0.1.18 deployment to strato-vps followed the sequence: verified dump (backups/cortexiq-20261104T232239Z.dump), release published by Bo (the auto-mode classifier refused 'gh release create'), .env backed up as .env.bak-pre0.1.18, then tag-bump and 'up -d'.
- After deployment, CortexIQ v0.1.18 was verified to show version v0.1.18 (commit 6c89dd3), goose 43, and facts_refused entries in the logs.
- CortexIQ production had 0 retractions at the time of deployment, so the backfill performed no action.
- In CortexIQ, the Gemma parse error with causal links in /projects/chronicle is a known list problem, not new.
- SSH access to strato-vps works via an allow rule in the project settings; this rule should be removed after the deployment.

---

episodes 21 | A: 96 facts, numbers kept 139/171, invented 8 | B: 99 facts, numbers kept 142/171, invented 8, would be rejected by the production gate 4
