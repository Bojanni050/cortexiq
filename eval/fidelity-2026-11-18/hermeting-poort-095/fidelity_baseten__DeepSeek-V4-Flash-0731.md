# Fact fidelity: baseten/DeepSeek-V4-Flash-0731

A = production prompt and gate. B = same prompt, no gate. Read the facts against the episode: subject and object right? detail kept? anything added?

## Episode 989

> Spike 16-11-2026: CortexIQ 0.1.88 (strato-vps, REST 127.0.0.1:18101) vergeleken met Hindsight 0.9.2 (contabo, 8888) als vervanger van Hindsight in Gaia. Beslissend verschil: CortexIQ filtert op occurred_at met from/to (half-open) en herkent Nederlandse datumzinnen over REST, terwijl Hindsight temporal_window alleen rankt (een venster van 1 dag gaf 54 hits over vijf dagen) en memories/list geen datumfilter heeft. Scoping moet met namespaces (array in REST); de enkelvoudige namespace is alleen een ranking-zetje. remember bewaart content, namespace, tags, occurred_at, external_key, external_refs en importance, maar gooit vrije metadata weg; external_key maakt de write idempotent (unchanged:true, andere inhoud geweigerd tenzij supersedes), dus gaia:kairos:<episode_id> is de spiegel-sleutel. DELETE /api/v1/episodes/:id purge hard en cascadeert naar afgeleide facts; facts en namespaces zijn niet los te verwijderen via REST (404/405). Consolidatie reageert binnen twee minuten maar levert even dunne facts als Hindsight. Op twintig Nederlandse vragen: CortexIQ mediaan 1.061 ms met ruwe episodes bovenaan, Hindsight 1.372 ms met een door emotionele gaia-herinneringen verzadigde top. Rapport: docs/spike-cortexiq.md (commits 85dd48e en f329e24). Volgorde blijft: MemoryStore-poort, terugkijk-route op CortexIQ, gaia-logos rebuilden, bojan/gaia importeren, Hindsight uit.

**A production** (numbers kept 5/13, invented [], gate would reject 0)

- CortexIQ 0.1.88 was compared with Hindsight 0.9.2 as a potential replacement for Hindsight in Gaia.
- CortexIQ filters episodes by occurred_at with from/to (half-open) and recognizes Dutch date phrases over REST; Hindsight's temporal_window only ranks and memories/list has no date filter.
- CortexIQ scopes using namespaces as an array in REST; a single namespace only acts as a ranking boost.
- CortexIQ's remember endpoint stores content, namespace, tags, occurred_at, external_key, external_refs and importance, discards free metadata; external_key makes writes idempotent (unchanged:true, different content rejected unless supersedes).
- CortexIQ's DELETE /api/v1/episodes/:id purges hard and cascades to derived facts; facts and namespaces are not separately removable via REST (404/405).
- On twenty Dutch questions, CortexIQ had a median response time of 1,061 ms with raw episodes on top, while Hindsight had 1,372 ms with a top saturated by emotional gaia memories; consolidation replied within two minutes but yielded as thin facts as Hindsight.

**B variant** (numbers kept 11/13, invented [1-day], gate would reject 0)

- In a spike on 16-11-2026 comparing CortexIQ 0.1.88 (strato-vps, REST 127.0.0.1:18101) and Hindsight 0.9.2 (contabo, 8888) as a replacement for Hindsight in Gaia, the decisive difference was that CortexIQ filters on occurred_at with from/to (half-open) and recognizes Dutch date sentences over REST, while Hindsight's temporal_window only ranks (a 1-day window gave 54 hits over five days) and its memories/list has no date filter.
- In the same spike, CortexIQ's scoping must use namespaces as an array in the REST API; a single namespace only acts as a ranking nudge.
- CortexIQ's remember endpoint stores content, namespace, tags, occurred_at, external_key, external_refs, and importance but discards free metadata; external_key makes writes idempotent (unchanged:true, other content rejected unless supersedes), and gaia:kairos:<episode_id> is the mirror key.
- CortexIQ's DELETE /api/v1/episodes/:id performs a hard purge and cascades to derived facts; facts and namespaces cannot be deleted separately via REST, returning 404/405.
- In the spike, CortexIQ's consolidation reacted within two minutes but yielded facts as thin as those from Hindsight.
- On twenty Dutch questions, CortexIQ had a median response time of 1.061 ms with raw episodes on top, while Hindsight had 1.372 ms with a top saturated by emotional gaia-herinneringen (memories).

## Episode 1082

> Migratieplan Hindsight→CortexIQ in Gaia-Cloud vastgesteld op 18-11-2026 (plansessie): volgorde zoals de spike van 16-11 bepaalde (MemoryStore-poort → terugkijk-route → gaia-logos rebuild → bojan/gaia import → Hindsight uit). Twee beslissingen van Bo, beide 'meten gaat voor bouwen': (1) CortexIQ-fase 1 (quota recall over namespaces) wordt pas gebouwd nadat een fixture op de live strato-instance aantoonbaar maakt dat één luide namespace de rest verdringt — geen bewijs, geen bouw; (2) hosting van CortexIQ voor Gaia (strato via Tailscale vs eigen instance op contabo) wordt pas gekozen na een latency-meting contabo→strato, uit te voeren nadat de REST aan de tailnet-kant is gebonden (nu 127.0.0.1:18101). Fase 6 (agent_identity) voorlopig geschrapt: namespace + tag dekt de epistemic mark. Fase 0-huiskeeping: episode 980 (Mindstack, Vite-migratie) opnieuw aanleveren; oude hindsight-backups van 03-05 dienen alleen als formaat-referentie bij de verse export in fase 4.

**A production** (numbers kept 9/10, invented [], gate would reject 0)

- Het migratieplan van Hindsight naar CortexIQ in de Gaia-Cloud werd vastgesteld op 18-11-2026 tijdens een plansessie.
- De volgorde van de migratie van Hindsight naar CortexIQ in de Gaia-Cloud is: eerst de MemoryStore-poort, dan de terugkijk-route, vervolgens de gaia-logos rebuild, daarna de bojan/gaia import, en tot slot het uitzetten van Hindsight.
- De hostingkeuze voor CortexIQ voor Gaia (strato via Tailscale versus eigen instance op contabo) wordt pas gemaakt na een latency-meting van contabo naar strato, uit te voeren nadat de REST aan de tailnet-kant is gebonden (nu 127.0.0.1:18101).
- Fase 6 (agent_identity) van CortexIQ is voorlopig geschrapt omdat namespace en tag de epistemic mark dekken.
- Voor fase 0-huiskeeping van de migratie moet episode 980 (Mindstack, Vite-migratie) opnieuw worden aangeleverd; de oude hindsight-backups van 03-05 dienen alleen als formaat-referentie bij de verse export in fase 4.

**B variant** (numbers kept 9/10, invented [0-huishouding], gate would reject 0)

- Het Hindsight→CortexIQ migratieplan in Gaia-Cloud werd op 18-11-2026 vastgesteld tijdens een plansessie; de volgorde, zoals bepaald door de spike van 16-11, is: MemoryStore-poort eerst, dan terugkijk-route, dan gaia-logos rebuild, dan bojan/gaia import, en als laatste Hindsight uit.
- Volgens een beslissing van Bo (meten gaat voor bouwen) wordt CortexIQ-fase 1 (quota recall over namespaces) pas gebouwd nadat een fixture op de live strato-instance aantoonbaar maakt dat één luide namespace de rest verdringt; geen bewijs, geen bouw.
- Volgens een beslissing van Bo (meten gaat voor bouwen) wordt de hosting van CortexIQ voor Gaia (strato via Tailscale vs eigen instance op contabo) pas gekozen na een latency-meting contabo→strato; de meting wordt uitgevoerd nadat de REST aan de tailnet-kant is gebonden (nu 127.0.0.1:18101).
- CortexIQ-fase 6 (agent_identity) werd voorlopig geschrapt uit het plan, omdat namespace + tag de epistemic mark dekt.
- Bij de fase 0-huishouding van de Hindsight→CortexIQ migratie in Gaia-Cloud werd afgesproken dat episode 980 (Mindstack, Vite-migratie) opnieuw wordt aangeleverd, en dat oude hindsight-backups van 03-05 alleen dienen als formaat-referentie bij de verse export in fase 4.

## Episode 558

> Gap-analyse CortexIQ vs Hindsight voor Gaia-Cloud (2026-11): Gaia-Cloud hangt via src/hindsightClient.js aan Hindsight voor: (1) banks + cross-bank recall met per-bank quota (scores niet op één schaal), (2) willekeurige tags per memory-item die in recall-output terugkomen (gaia:* tags, memoryKindOf), (3) versiegelde invalidatie/superseding sync vanuit cognitionSync met reconcile, (4) provenance sources:['foundation:<id>'] voor Dual Recall, (5) async fire-and-forget retain, (6) agent_identity-enforcing. CortexIQ heeft inmiddels wél reflect, mental models, reranker, graph-arm, temporal recall en MMR — die eerdere gaten zijn gedicht.

**A production** (numbers kept 6/7, invented [], gate would reject 0)

- Gaia-Cloud depends on Hindsight (connected via src/hindsightClient.js) for six capabilities: (1) bank and cross-bank recall with per-bank quota where scores are not on one scale, (2) arbitrary tags per memory item that appear in recall output (gaia:* tags, memoryKindOf), (3) sealed invalidation/superseding sync from cognitionSync with reconcile, (4) provenance sources:['foundation:<id>'] for Dual Recall, (5) async fire-and-forget retain, and (6) agent_identity enforcing.
- CortexIQ has since added reflect, mental models, reranker, graph-arm, temporal recall, and MMR, closing the gaps previously identified in the gap analysis against Hindsight.

**B variant** (numbers kept 0/7, invented [], gate would reject 0)

- Gaia-Cloud uses Hindsight via src/hindsightClient.js for banks + cross-bank recall with per-bank quota (scores not on one scale), random tags per memory item returned in recall output (gaia:* tags, memoryKindOf), sealed invalidation/superseding sync from cognitionSync with reconcile, provenance sources ['foundation:<id>'] for Dual Recall, async fire-and-forget retain, and agent_identity-enforcing.
- CortexIQ includes reflect, mental models, reranker, graph-arm, temporal recall and MMR, which closes the earlier gaps identified in the gap analysis against Hindsight.

## Episode 36

> Beslissing 2026-10-08 (akkoord Bo): de provenance-prefix voor afgeleide records in Gaia-Cloud gaat van 'chronicle:' naar 'foundation:'. Canonieke id-vorm: 'foundation:<uuid>' met de kale ingest_object uuid (wat Foundation /api/ingest-logs/:id verwacht), niet Foundation's episode-vorm 'ingest:<uuid>'. Reden: 'chronicle:' is legacy (Foundation, niet Chronicle, is de rauwe laag) en is een routing key, geen label. Wordt in één migratie gedaan die ook de strip-bug in kairosRoutes.js fixt en de drie sources-kolommen (hypotheses/patterns/kairos_episodes) plus Hindsight-metadata backfillt; reconcile() wordt uitgebreid met Kairos-episodes. Plan: C:\Users\bojan\.opencode\plan\chronicle-prefix-provenance-plan.md.

**A production** (numbers kept 1/1, invented [], gate would reject 0)

- On 2026-10-08, a decision (agreed by Bo) was made to change Gaia-Cloud's provenance-prefix for derived records from 'chronicle:' to 'foundation:'.
- Gaia-Cloud's canonical id form for derived records is 'foundation:<uuid>' using the bare ingest_object uuid (as Foundation /api/ingest-logs/:id expects), not Foundation's episode form 'ingest:<uuid>'.
- In Gaia-Cloud, the prefix 'chronicle:' is legacy because Foundation (not Chronicle) is the raw layer, and it is a routing key, not a label.
- Gaia-Cloud's migration for the provenance-prefix change also fixes the strip-bug in kairosRoutes.js, backfills the three sources columns (hypotheses/patterns/kairos_episodes) plus Hindsight metadata, and extends reconcile() with Kairos episodes.
- The plan for the Gaia-Cloud provenance-prefix change is stored at C:\Users\bojan\.opencode\plan\chronicle-prefix-provenance-plan.md.

**B variant** (numbers kept 1/1, invented [], gate would reject 0)

- On 2026-10-08, with Bo's agreement, it was decided that the provenance prefix for derived records in Gaia-Cloud changes from 'chronicle:' to 'foundation:'.
- The canonical id form for derived records in Gaia-Cloud is 'foundation:<uuid>' with the bare ingest_object uuid, which is what Foundation's /api/ingest-logs/:id expects, not Foundation's episode form 'ingest:<uuid>'.
- The reason for changing the provenance prefix in Gaia-Cloud is that 'chronicle:' is legacy because Foundation, not Chronicle, is the raw layer, and it is a routing key, not a label.
- The migration for the prefix change in Gaia-Cloud is done in one migration that also fixes the strip-bug in kairosRoutes.js and backfills the three sources columns (hypotheses/patterns/kairos_episodes) plus Hindsight-metadata, and extends reconcile() with Kairos-episodes.
- The plan for the provenance prefix change in Gaia-Cloud is located at C:\Users\bojan\.opencode\plan\chronicle-prefix-provenance-plan.md.

## Episode 37

> Gaia-Cloud: de chronicle:->foundation: provenance-normalisatie is op 2026-10-08 geïmplementeerd (nieuwe seam src/foundationRef.js, synthesizer+kairosRoutes erop, cognitionSync.reconcile() pusht nu ook Kairos-episodes, migratie 010_normalize_sources_prefix.sql). Tests groen: gaia-api 1244/1244, cognition 49/49. Nog open: migratie toepassen op de VPS (npm run migrate in services/cognition) en daarna npm run reconcile:hindsight -- --apply --clear om de Hindsight-metadata te verversen.

**A production** (numbers kept 2/4, invented [], gate would reject 0)

- Gaia-Cloud implemented provenance normalization on 2026-10-08: new seam src/foundationRef.js, synthesizer and kairosRoutes connected to it, cognitionSync.reconcile() now also pushes Kairos-episodes, and migration 010_normalize_sources_prefix.sql.
- Gaia-Cloud reported green tests: gaia-api passed 1244 out of 1244 tests and cognition passed 49 out of 49 tests.
- For Gaia-Cloud, the following steps remain open: apply the migration on the VPS using 'npm run migrate' in services/cognition, then run 'npm run reconcile:hindsight -- --apply --clear' to refresh the Hindsight-metadata.

**B variant** (numbers kept 2/4, invented [], gate would reject 0)

- Gaia-Cloud implemented provenance normalization on 2026-10-08, introducing a new seam src/foundationRef.js, with the synthesizer and kairosRoutes using it, cognitionSync.reconcile() now also pushing Kairos episodes, and migration 010_normalize_sources_prefix.sql.
- gaia-api test suite passed 1244 out of 1244 tests.
- cognition test suite passed 49 out of 49 tests.

## Episode 38

> Reconcile dry-run Gaia-Cloud 2026-10-08: Hindsight actief gaia:hypothesis=2, gaia:pattern=0, gaia:kairos_episode=0; Cognition hypotheses=4, patterns=0, kairos=0. De foundation:-prefix-backfill bleek een no-op: de oude 'chronicle:'-prefix werd alleen door de Kairos-synthesizer geproduceerd en er zijn geen Kairos-episodes (GAIA_KAIROS_ENABLED staat uit), dus --apply --clear is overgeslagen (zou enkel versie-churn geven). De 4-vs-2 kan deels survey-artefact zijn: open_question/relationship/mental_model dragen andere tags en tellen niet als gaia:hypothesis.

**A production** (numbers kept 5/5, invented [], gate would reject 0)

- In the Gaia-Cloud reconcile dry-run on 2026-10-08, Hindsight was active with gaia:hypothesis=2, gaia:pattern=0, and gaia:kairos_episode=0.
- In the Gaia-Cloud reconcile dry-run on 2026-10-08, Cognition had 4 hypotheses, 0 patterns, and 0 kairos.
- In Gaia-Cloud, the foundation:-prefix-backfill was a no-op: the old 'chronicle:'-prefix was only produced by the Kairos-synthesizer, and with no Kairos episodes (GAIA_KAIROS_ENABLED is off), --apply --clear was skipped to avoid version churn.
- In the Gaia-Cloud reconcile dry-run, the 4-vs-2 difference between Cognition and Hindsight may partly be a survey artifact: open_question/relationship/mental_model carry other tags and do not count as gaia:hypothesis.

**B variant** (numbers kept 4/5, invented [], gate would reject 0)

- On 2026-10-08, the Hindsight module of the Gaia AI-assistent had active counts gaia:hypothesis=2, gaia:pattern=0, and gaia:kairos_episode=0 in a reconcile dry-run.
- On 2026-10-08, the Cognition module of the Gaia AI-assistent had 4 hypotheses, 0 patterns, and 0 kairos episodes in a reconcile dry-run.
- On 2026-10-08, the foundation:-prefix backfill in the Gaia AI-assistent was a no-op: the old 'chronicle:'-prefix was only produced by the Kairos synthesizer, there are no Kairos episodes because GAIA_KAIROS_ENABLED is off, so --apply --clear was skipped to avoid version churn.
- In the Gaia AI-assistent, the discrepancy between Cognition (4 hypotheses) and Hindsight (2 hypotheses) may be partly a survey artifact, because open_question, relationship, and mental_model items carry different tags and do not count as gaia:hypothesis.

## Episode 81

> Gaia-Cloud aanvulling (commit 3f645dd, 2026-10-12): een op het verleden gerichte beurt raadpleegt nu zowel Hindsight als het transcript. recallRelevantContext (memory.js) kreeg een `pastReference`-optie die de recall-gate opent; turn.js geeft die door zodra de gesprekszoek-signalen (exactHistory/pastLookup/rememberedKnowledge) vuren. Reden: Bo's punt 'ze kijkt ook in Hindsight' — de twee gates (memoryPolicy.shouldRecall en de gesprekszoek-signalen) waren onafhankelijk, dus sommige lookup-formuleringen openden alleen het transcript. Let op: Hindsight draait sowieso al elke beurt via de mentale modellen (fetchMentalModelContext), ongeacht de gate. gaia-api 1433/1433 groen (+2).

**A production** (numbers kept 2/4, invented [], gate would reject 0)

- In Gaia AI-assistent, a past-oriented turn now consults both Hindsight and the transcript.
- In Gaia AI-assistent, recallRelevantContext (memory.js) received a pastReference option that opens the recall gate.
- In Gaia AI-assistent, turn.js passes pastReference when the conversation-search signals exactHistory/pastLookup/rememberedKnowledge fire.
- In Gaia AI-assistent, the two gates memoryPolicy.shouldRecall and the conversation-search signals were independent, so some lookup formulations only opened the transcript.
- In Gaia AI-assistent, Hindsight runs every turn via the mental models (fetchMentalModelContext), regardless of the gate.
- In Gaia AI-assistent, gaia-api reported 1433/1433 green tests, with a +2 increase.

**B variant** (numbers kept 3/4, invented [], gate would reject 0)

- Gaia AI-assistent: as of commit 3f645dd, a past-oriented turn consults both Hindsight and the transcript.
- Gaia AI-assistent: recallRelevantContext (memory.js) gained a pastReference option that opens the recall gate; turn.js passes it when conversation-search signals (exactHistory/pastLookup/rememberedKnowledge) fire.
- Gaia AI-assistent: the two recall gates (memoryPolicy.shouldRecall and conversation-search signals) were independent, so some lookup formulations only opened the transcript.
- Gaia AI-assistent: Hindsight runs every turn via the mental models (fetchMentalModelContext), regardless of the recall gate.
- Gaia AI-assistent's gaia-api reported 1433/1433 green (+2) at commit 3f645dd.

## Episode 86

> Gaia-Cloud (2026-10-12, commit 8e26794, gedeployed): (1) recall-staart gemeten uit Hindsight's Prometheus-histogram (hindsight_http_duration_seconds, recall-endpoint): 27 recalls, mean 894ms, p50 ~1s, p90/p95 ~2,5s, p99 ~5s; 1 call (3,7%) boven 4s, 0 boven 8s. Kleine, niet-representatieve steekproef (grotendeels test-calls), buckets grof. (2) Observability: elke gefaalde Hindsight-call logt nu een content-loze regel {kind:'hindsight.call_failed', bank, op, reason:'timeout'|'unreachable', timeoutMs, ms} en throwt door — via een request()-wrapper rond fetchImpl in src/hindsightClient.js (alle 14 call-sites). Reden: memory.js slikte Hindsight-fouten stil in (best-effort -> []), dus missende context was onzichtbaar. Test test/hindsightCallLog.test.js (+3), gaia-api 1436/1436. Het is puur een logregel, geen gedragsverandering; verschijnt in de gaia-api containerlog.

**A production** (numbers kept 17/18, invented [2.5s 3.7], gate would reject 0)

- On 2026-10-12, Hindsight's recall endpoint (part of Gaia-Cloud) measured 27 recalls with mean 894ms, p50 ~1s, p90/p95 ~2.5s, p99 ~5s; 1 call (3.7%) took over 4s and 0 over 8s, but the sample was small and mostly test calls, so not representative; Prometheus histogram buckets were coarse.
- On 2026-10-12, Gaia-Cloud updated src/hindsightClient.js (all 14 call-sites) so each failed Hindsight call logs a content-less rule with the given fields and then throws, via a request() wrapper around fetchImpl. This was added because memory.js silently swallowed Hindsight errors (best-effort -> []), making missing context invisible. It is purely a log rule with no behavior change and appears in the gaia-api container log. Test test/hindsightCallLog.test.js was added (+3) and gaia-api passes 1436/1436 tests.

**B variant** (numbers kept 18/18, invented [2.5s 3.7], gate would reject 0)

- On 2026-10-12 (commit 8e26794, Gaia Cloud), the Hindsight recall endpoint was measured via the hindsight_http_duration_seconds Prometheus histogram: 27 recalls, mean 894ms, p50 ~1s, p90/p95 ~2.5s, p99 ~5s, with 1 call (3.7%) above 4s and 0 above 8s; the sample was small and non-representative (mostly test calls) and the histogram buckets were coarse.
- On 2026-10-12 (commit 8e26794, Gaia Cloud), the Hindsight client (src/hindsightClient.js) was updated so every failed Hindsight call logs a content-less line with those fields and re-throws the error, via a request()-wrapper around fetchImpl at all 14 call-sites; the change is purely a log line with no behavior change and appears in the gaia-api container log; the reason given is that memory.js previously silently swallowed Hindsight errors (best-effort -> []) making missing context invisible; a new test test/hindsightCallLog.test.js (+3) was added and gaia-api passes 1436/1436.

## Episode 560

> Beslissing: LongMemEval-harnas is uitgeschreven als docs/design-longmemeval.md in cortexiq (commit eb37c41, gepusht naar main 2026-11-02). Belangrijkste bevinding: internal/benchmark bestaat al en is eerlijk (ingest als episodes, antwoorden via reflect, beide judges) — het gat is de officiële dataset + fairness-protocol + publicatie. Kernregels: judge is een ander/sterker model dan het antwoordmodel, abstention krijgt een speciale judge-regel, geen benchmark-specifieke tuning, alleen de S-variant draaien flattert consolidatie (dus ook de M/day-variant), Hindsight-cijfers worden gequoteerd met bron als hun meting.

**A production** (numbers kept 2/2, invented [], gate would reject 0)

- CortexIQ's LongMemEval-harnas is written out as the design document docs/design-longmemeval.md, committed as eb37c41 and pushed to main on 2026-11-02.
- CortexIQ's internal/benchmark already exists and is honest (ingested as episodes, answered via reflect, both judges); the identified gap is the official dataset, fairness-protocol, and publication.
- CortexIQ's LongMemEval-harnas sets judge rules: the judge must be a different/stronger model than the answer model, and abstention gets a special judge rule.
- CortexIQ's LongMemEval-harnas prohibits benchmark-specific tuning.
- CortexIQ's LongMemEval-harnas requires also running the M/day variant, because running only the S-variant flatters consolidation.
- CortexIQ's LongMemEval-harnas requires quoting Hindsight figures with the source as their measurement.

**B variant** (numbers kept 2/2, invented [], gate would reject 0)

- LongMemEval harness was documented as docs/design-longmemeval.md in CortexIQ, commit eb37c41, pushed to main on 2026-11-02.
- CortexIQ's internal/benchmark already exists and is honest (ingested as episodes, answers via reflect, both judges); the remaining gap is the official dataset, fairness protocol and publication.
- For LongMemEval, the judge must be a different and stronger model than the answer model.
- For LongMemEval, abstention receives a special judge rule.
- For LongMemEval, no benchmark-specific tuning is allowed.
- For LongMemEval, running only the S variant flatters consolidation, so the M/day variant should also be run.

## Episode 364

> Gaia Cloud now retains eligible Hindsight turns as a Hermes-style JSON array of role-labelled User and Assistant messages. Existing memoryworthiness/triviality gates, asynchronous fire-and-forget behavior, and identity/provenance metadata remain; no historical Hindsight data is rewritten. Deployed as commit bddbad5 on 2026-10-23.

**A production** (numbers kept 2/2, invented [], gate would reject 0)

- Gaia Cloud was updated to store eligible Hindsight turns as a Hermes-style JSON array containing role-labelled User and Assistant messages.
- Gaia Cloud retained its existing memoryworthiness/triviality gates, asynchronous fire-and-forget behaviour, and identity/provenance metadata, and no historical Hindsight data was rewritten.
- Gaia Cloud's new Hindsight storage behaviour was deployed as commit bddbad5 on 2026-10-23.

**B variant** (numbers kept 2/2, invented [], gate would reject 1)

- Gaia Cloud retains eligible Hindsight turns as a Hermes-style JSON array of role-labelled User and Assistant messages.
- Gaia Cloud retains its existing memoryworthiness/triviality gates, asynchronous fire-and-forget behavior, and identity/provenance metadata.
- Gaia Cloud does not rewrite any historical Hindsight data.
- Gaia Cloud was deployed as commit bddbad5 on 2026-10-23.

## Episode 859

> CortexIQ BM25-repetitie op de productiedump (2026-11-11): CI op 3774ea9 groen. Dagelijkse prod-dump (47 MB, checksum OK; prod = 2.944 facts, 728 episodes, 128 MB, goose 60, bge-m3/dim 1024) in verse PG17-container met deploy/postgres-image: pg_restore 5 s, db.Open inclusief migratie 00061 335 ms, bm25-indexen aanwezig, tsvector-kolommen weg. Kwaliteit: BM25 wint bij inspectie van 3 van 15 queries (IDF: zeldzame tokens als 8445 en bge-m3 komen bovenaan, ts_rank_cd gaf generieke woorden); geen gelabelde eval gedraaid, vervolgstap is cortexiq eval recall. Snelheidswinst is op dit kleine corpus nog theoretisch. VPS-toegang was read-only (backuplijst, scp van dump); dump en container na afloop verwijderd. OPEN: prod nog op PG16 en niet geüpgraded; release-job voor cortexiq-postgres en arm64 nooit gedraaid; Bo moet de ssh- en scp-allow-regels en de oude gh-release-create-regel uit .claude/settings.local.json halen (bestand is in git gevolgd, niet committen). Commit van de docs: zie git log.

**A production** (numbers kept 16/21, invented [], gate would reject 0)

- On 2026-11-11, the CortexIQ BM25 repetition test on the production dump passed CI with commit 3774ea9 green.
- The CortexIQ production database, as of the daily prod dump, contained 2,944 facts, 728 episodes, 128 MB, goose 60, and used bge-m3 with dimension 1024; the dump itself was 47 MB with checksum OK.
- The CortexIQ production dump was loaded into a fresh PG17 container using the deploy/postgres-image; pg_restore took 5 seconds, db.Open including migration 00061 took 335 milliseconds, bm25 indexes were present, and tsvector columns were removed.
- In the CortexIQ BM25 repetition test, BM25 won in inspection of 3 of 15 queries; IDF brought rare tokens like 8445 and bge-m3 to the top, while ts_rank_cd gave generic words; no labeled eval was run and the next step is cortexiq eval recall.
- In the CortexIQ BM25 repetition test, the speed advantage was still theoretical on the small corpus.
- Open items for CortexIQ: prod is still on PG16 and not upgraded; the release-job for cortexiq-postgres and arm64 has never run; Bo must remove the ssh and scp allow rules and the old gh-release-create rule from .claude/settings.local.json (file is git-tracked, do not commit); commit of docs is pending (see git log).

**B variant** (numbers kept 16/21, invented [], gate would reject 0)

- CortexIQ CI on commit 3774ea9 is green.
- The CortexIQ daily production dump (47 MB, checksum OK; prod contains 2,944 facts, 728 episodes, 128 MB, goose 60, bge-m3/dim 1024) was loaded into a fresh PG17 container with the deploy/postgres-image; pg_restore took 5 seconds, db.Open including migration 00061 took 335 ms, bm25 indexes are present, and tsvector columns are removed.
- In the CortexIQ BM25 rehearsal, BM25 wins on inspection of 3 of 15 queries (IDF: rare tokens like 8445 and bge-m3 appear at the top, while ts_rank_cd gave generic words); no labeled evaluation was run, and the next step is cortexiq eval recall.
- The speed gain of CortexIQ BM25 is still theoretical on this small corpus.
- The CortexIQ BM25 rehearsal used read-only VPS access (backup list, scp of dump); after the rehearsal the dump and container were removed.
- Open items from the CortexIQ BM25 rehearsal: production is still on PG16 and not upgraded; the release-job for cortexiq-postgres and arm64 has never been run; Bo must remove the ssh and scp allow rules and the old gh-release-create rule from .claude/settings.local.json (the file is tracked in git, so it should not be committed).

## Episode 759

> De productiebug supported_by is lokaal opgelost: de background consolidator selecteerde hypotheses zonder supported_by voor pgx named scanning, en positional hypothesis/counter scans misten het veld. Drie bestanden zijn aangepast, walkthrough bijgewerkt, go test ./internal/brain en go test ./... zijn groen. Niet gedeployed; aparte expliciete goedkeuring vereist.

**A production** (numbers kept 0/0, invented [], gate would reject 0)

- The production bug 'supported_by' in CortexIQ was locally resolved; the cause was that the background consolidator selected hypotheses without supported_by for pgx named scanning, and positional hypothesis/counter scans missed the field.
- As part of the fix in CortexIQ, three files were modified and the walkthrough was updated.
- After the fix, the tests 'go test ./internal/brain' and 'go test ./...' are green in CortexIQ.
- The fix for the production bug 'supported_by' in CortexIQ is not deployed; separate explicit approval is required.

**B variant** (numbers kept 0/0, invented [], gate would reject 0)

- CortexIQ had a production bug where the background consolidator selected hypotheses without supported_by for pgx named scanning, and positional hypothesis/counter scans missed the supported_by field.
- CortexIQ's supported_by bug was fixed locally by modifying three files and updating the walkthrough.
- After the fix, the commands go test ./internal/brain and go test ./... passed for CortexIQ.
- The CortexIQ fix was not deployed; separate explicit approval is required.

## Episode 864

> CortexIQ migratie-race bij opstarten gerepareerd en gepusht (2026-11-11): internal/db/db.go draait de hele schema-opzet (goose.Up, dimensie-ALTER, HNSW-indexen) nu in setupSchema onder withSchemaLock, een sessie-advisory lock (pg_advisory_lock, sleutel 994294737458 = 'cortexiq' in base 36) op een eigen verbinding, wachttijd begrensd op 5 minuten, bij annuleren wordt de sessie gesloten zodat er geen lock achterblijft; Open sluit de pool nu bij elke fout (lekte bij goose-fout). Regressietests internal/db/schemalock_integration_test.go (6 gelijktijdige Open's op een lege database, wederzijdse uitsluiting, annuleerbare wacht); mutatiecheck: zonder lock falen 5 van 6 starts met dezelfde 42P07-fout als in prod. Volledige suite groen op verse PG17-db: 21 pakketten, 153 integratietests, 0 skip. NOG NIET UITGEROLD: productie draait v0.1.74 zonder dit slot; het zit in de volgende release. Open: Bo moet de ssh-allow-regels uit .claude/settings.local.json halen (de uitrol is klaar) en de Jina-sleutel roteren; compose op de VPS wijst nog naar de lokale image cortexiq-postgres:17-bm25 (ghcr-image ...-postgres:0.1.74 bestaat).

**A production** (numbers kept 9/12, invented [], gate would reject 0)

- CortexIQ's internal/db/db.go now runs the full schema setup (goose.Up, dimension ALTER, HNSW indexes) in setupSchema under withSchemaLock, a session advisory lock (pg_advisory_lock, key 994294737458 = 'cortexiq' in base 36) on a separate connection, with wait time capped at 5 minutes and session closed on cancel so no lock remains.
- CortexIQ's Open now closes the connection pool on every error, fixing a pool leak that occurred when a goose error was raised.
- CortexIQ added regression tests in internal/db/schemalock_integration_test.go for the schema lock: 6 concurrent Open calls on an empty database, verifying mutual exclusion and cancellable waiting; the mutation check shows that without the lock, 5 of 6 starts fail with the same 42P07 error as in production.
- CortexIQ's full test suite is green on a fresh PostgreSQL 17 database, with 21 packages, 153 integration tests, and 0 skips.
- CortexIQ production is not yet deployed with the migration race fix; it runs v0.1.74 without the session lock, and the fix is scheduled for the next release.
- CortexIQ's VPS compose configuration still points to the local image cortexiq-postgres:17-bm25, while the ghcr.io image -postgres:0.1.74 exists.

**B variant** (numbers kept 9/12, invented [], gate would reject 0)

- CortexIQ's internal/db/db.go now runs the entire schema setup (goose.Up, dimension ALTER, HNSW indexes) in setupSchema under withSchemaLock, a session advisory lock (pg_advisory_lock, key 994294737458 = 'cortexiq' in base 36) on its own connection, with a wait limit of 5 minutes; on cancellation the session is closed so no lock remains. Open now closes the pool on any error (previously it leaked on goose error).
- CortexIQ added regression tests in internal/db/schemalock_integration_test.go covering 6 concurrent Open's on an empty database, mutual exclusion, and cancellable wait; without the lock, 5 of 6 starts fail with the same 42P07 error as in production. The full suite is green on a fresh PG17 database: 21 packages, 153 integration tests, 0 skip.
- CortexIQ production runs v0.1.74 without the new advisory lock; the lock is not yet deployed and is intended for the next release.
- CortexIQ's VPS compose file still points to the local image cortexiq-postgres:17-bm25; the ghcr image ...-postgres:0.1.74 exists.

## Episode 169

> CortexIQ prod (strato-vps, /opt/cortexiq) is op 2026-10-15 uitgerold naar commit 6377b4c: POST /api/v1/remember is live (401 zonder token, 201 met token) en de embedder wordt bij start op de achtergrond warm geladen (~1,5 s, logregel msg="embedder warm"); warme recall meet ~5 ms in plaats van de oude koude start van ~1,8 s. Goose-versie bleef 27.

**A production** (numbers kept 9/9, invented [1.5 1.8], gate would reject 0)

- CortexIQ prod (on strato-vps, /opt/cortexiq) was rolled out to commit 6377b4c on 2026-10-15.
- CortexIQ POST /api/v1/remember is live and returns 401 without token and 201 with token.
- CortexIQ embedder is warm-loaded in the background at startup, taking about 1.5 seconds and logging msg='embedder warm'.
- On 2026-10-15, CortexIQ recall measured about 5 ms when warm, compared to the old cold start of about 1.8 s.
- The Goose version in CortexIQ remained 27 at the time of the 2026-10-15 rollout.

**B variant** (numbers kept 9/9, invented [1.5 1.8], gate would reject 0)

- CortexIQ prod (hosted on strato-vps, path /opt/cortexiq) was rolled out on 2026-10-15 to commit 6377b4c.
- CortexIQ's POST /api/v1/remember endpoint was live and returned 401 without a token and 201 with a token.
- CortexIQ warm-loaded its embedder in the background at start, taking about 1.5 seconds, and logged msg="embedder warm".
- CortexIQ warm recall measured about 5 ms, compared to the old cold start of about 1.8 s.
- The Goose version in CortexIQ remained 27.

## Episode 697

> CortexIQ: RRF-sweep gedraaid op de productie-embedder (bge-m3, hybride, Jina-reranker; 60 memories, 36 vragen, k=10) via docker compose run met een override naar een wegwerpdatabase cortexiq_eval (productie niet geraakt, daarna verwijderd). Uitkomst overall: rrf-k 20 hit@1 0.75 MRR 0.850; rrf-k 60 hit@1 0.78 MRR 0.868; rrf-k 100 identiek aan 60. Verschil is één à twee vragen van 36, dus ruis. Besluit: CORTEXIQ_RRF_K blijft 60. Alle varianten missen dezelfde acht vragen op ongeveer dezelfde plaats, dus een betere recall zit in embedder/reranker, niet in de fusieconstante. Recept voor een volgende keer: op de server in /opt/cortexiq een override met CORTEXIQ_POSTGRES_DSN naar een extra database, dan docker compose -f docker-compose.prod.yml -f override run --rm --no-deps -T -v /opt/cortexiq/eval:/eval:ro cortexiq eval recall --rrf-k ... --corpus /eval/recall_corpus.jsonl --queries /eval/recall_queries.jsonl. Les: gebruik bij .env-greps nooit een patroon dat KEY-variabelen raakt (RERANK matchte CORTEXIQ_RERANKER_API_KEY en de Jina-sleutel kwam in het gesprek; Bo is aangeraden hem te roteren).

**A production** (numbers kept 11/11, invented [], gate would reject 0)

- CortexIQ ran an RRF sweep on the production embedder (bge-m3, hybrid, Jina-reranker) with 60 memories, 36 questions, k=10, using a throwaway database cortexiq_eval via docker compose run with an override; production was not touched and the DB was removed. Results: RRF-k 20 hit@1 0.75 MRR 0.850; RRF-k 60 hit@1 0.78 MRR 0.868; RRF-k 100 identical to 60; the difference is one to two questions out of 36, considered noise.
- After the RRF sweep, CortexIQ kept CORTEXIQ_RRF_K at 60.
- CortexIQ's RRF sweep showed all variants miss the same eight questions at roughly the same place, indicating better recall lies in the embedder/reranker, not in the fusion constant.
- For a next CortexIQ evaluation, on the server in /opt/cortexiq use an override with CORTEXIQ_POSTGRES_DSN to an extra database and run: docker compose -f docker-compose.prod.yml -f override run --rm --no-deps -T -v /opt/cortexiq/eval:/eval:ro cortexiq eval recall --rrf-k ... --corpus /eval/recall_corpus.jsonl --queries /eval/recall_queries.jsonl.
- In CortexIQ, .env-greps must never use a pattern that hits KEY-variables; the pattern RERANK matched CORTEXIQ_RERANKER_API_KEY and the Jina key came into conversation, so Bo recommended rotating it.

**B variant** (numbers kept 11/11, invented [], gate would reject 0)

- CortexIQ RRF-sweep was run on the production embedder (bge-m3, hybrid, Jina-reranker) with 60 memories, 36 questions, k=10, using docker compose run with an override to a disposable database cortexiq_eval; production was not touched and the database was removed afterwards.
- The CortexIQ RRF-sweep produced overall hit@1 and MRR: rrf-k 20 hit@1 0.75 MRR 0.850; rrf-k 60 hit@1 0.78 MRR 0.868; rrf-k 100 identical to rrf-k 60; the difference of one to two questions out of 36 is considered noise.
- After the CortexIQ RRF-sweep, CORTEXIQ_RRF_K was decided to remain at 60, as the differences between rrf-k 20, 60 and 100 were within noise.
- In the CortexIQ RRF-sweep, all RRF variants missed the same eight questions at approximately the same place, indicating that better recall lies in the embedder/reranker, not in the fusion constant.
- For a next CortexIQ evaluation run, the recipe is: on the server in /opt/cortexiq, use an override with CORTEXIQ_POSTGRES_DSN to an extra database, then run docker compose -f docker-compose.prod.yml -f override run --rm --no-deps -T -v /opt/cortexiq/eval:/eval:ro cortexiq eval recall --rrf-k ... --corpus /eval/recall_corpus.jsonl --queries /eval/recall_queries.jsonl.
- During CortexIQ work, a lesson was learned: when grepping .env files, never use a pattern that matches KEY variables; the pattern 'RERANK' matched CORTEXIQ_RERANKER_API_KEY and exposed the Jina key in the conversation; Bo is advised to rotate the key.

## Episode 888

> CortexIQ: v0.1.80 (Timeline-vergelijking tussen twee data, commit 6f725cd, CI groen) is door Claude gepubliceerd en de Release-workflow is groen (2026-11-13). De uitrol naar productie is NIET uitgevoerd: de auto-mode classifier blokkeerde het ssh-commando voor de deploy. Productie draait vermoedelijk nog v0.1.79 (niet geverifieerd). Bo moet de deploy toestaan of zelf draaien. Let op: vaste toestemming van Bo gold voor het publiceren van releases; uitrol via ssh valt buiten wat de classifier toestaat zonder allow-regel in .claude/settings.local.json.

**A production** (numbers kept 4/4, invented [], gate would reject 0)

- CortexIQ v0.1.80 (feature 'Timeline-vergelijking tussen twee data', commit 6f725cd, CI green) was published by Claude on 2026-11-13, and the Release workflow was green.
- CortexIQ's auto-mode classifier blocked the ssh command for the deploy, so the rollout of v0.1.80 to production was not executed.
- CortexIQ production was presumably still running v0.1.79 at the time; this was not verified.
- Bo is required to either allow the deployment of v0.1.80 to production or perform it himself.
- Bo's standing permission covered only the publication of releases; the rollout via ssh is outside what the classifier allows without an allow rule in .claude/settings.local.json.

**B variant** (numbers kept 4/4, invented [], gate would reject 0)

- CortexIQ v0.1.80 (Timeline comparison between two dates, commit 6f725cd, CI green) was published by Claude and the Release-workflow was green on 2026-11-13.
- CortexIQ's rollout to production was not executed because the auto-mode classifier blocked the ssh command for the deploy.
- CortexIQ production was presumably still running v0.1.79 (not verified) at the time of the event.
- Bo was required to allow the CortexIQ deployment or run it himself.
- For CortexIQ, Bo's standing permission applies to publishing releases, but ssh rollout falls outside what the classifier allows without an allow-rule in .claude/settings.local.json.

## Episode 412

> CortexIQ-productie (strato-vps, /opt/cortexiq) draait sinds 2026-10-27 v0.1.6 (commit c107f8c, image sha256:61d7ed372332). Deployprocedure: GitHub-release publiceren (release-workflow bouwt binaries en image), CORTEXIQ_IMAGE_TAG in /opt/cortexiq/.env bumpen, daarna docker compose -f docker-compose.prod.yml pull en up -d. De UI is een aparte service (cortexiq-ui) op dezelfde image; UI luistert op 127.0.0.1:18101, API op 18100, beide eisen een token.

**A production** (numbers kept 6/6, invented [], gate would reject 0)

- CortexIQ productie (on strato-vps at /opt/cortexiq) has been running since 2026-10-27 with deployment details: version v0.1.6, commit c107f8c, image sha256:61d7ed372332.
- The deploy procedure for CortexIQ productie is: publish a GitHub release (the release-workflow builds binaries and image), bump CORTEXIQ_IMAGE_TAG in /opt/cortexiq/.env, then run 'docker compose -f docker-compose.prod.yml pull' and 'up -d'.
- CortexIQ productie has a separate UI service (cortexiq-ui) on the same image; the UI listens on 127.0.0.1:18101, the API on 18100, and both require a token.

**B variant** (numbers kept 6/6, invented [127.0.0.1:18100], gate would reject 0)

- CortexIQ-productie on strato-vps at /opt/cortexiq has been running since 2026-10-27 using version v0.1.6 (commit c107f8c, image sha256:61d7ed372332).
- The CortexIQ deployment procedure consists of publishing a GitHub release (the release-workflow builds binaries and the image), bumping CORTEXIQ_IMAGE_TAG in /opt/cortexiq/.env, and then running docker compose -f docker-compose.prod.yml pull and up -d.
- The CortexIQ UI service (cortexiq-ui) is a separate service on the same image that listens on 127.0.0.1:18101 and requires a token.
- The CortexIQ API listens on 127.0.0.1:18100 and requires a token.

## Episode 886

> CortexIQ werkwijze (2026-11-13): Bo geeft vaste toestemming dat Claude zelf GitHub-releases publiceert (gh release create <versie> --target main) zodra CI op main groen is; Bo hoeft het niet meer zelf te doen. Daarbij altijd versienummer en releasenotities melden in het antwoord. Uitrol naar productie volgt de vaste procedure (back-up .env, pull terwijl het draait, verified dump, tag bumpen, healthz/readyz/restarts/logs controleren). Eerder deed Bo het publiceren zelf vanaf v0.1.74; die gewoonte is nu vervangen.

**A production** (numbers kept 1/2, invented [], gate would reject 0)

- CortexIQ has a permanent authorization for Claude to publish GitHub releases itself using 'gh release create <versie> --target main' once CI on main is green; Bo no longer needs to do it himself.
- For CortexIQ, when Claude publishes a release, it must always report the version number and release notes in the reply.
- CortexIQ production rollout follows a fixed procedure: back-up .env, pull while running, verified dump, tag bump, healthz/readyz/restarts/logs checks.
- For CortexIQ, previously Bo published releases himself starting from v0.1.74; that habit is now replaced by Claude publishing them.

**B variant** (numbers kept 1/2, invented [], gate would reject 0)

- For CortexIQ, Bo granted Claude permanent permission to publish GitHub releases automatically when CI on main is green.
- For CortexIQ, GitHub releases are published with the command 'gh release create <versie> --target main'.
- For CortexIQ, Bo no longer needs to publish GitHub releases manually.
- For CortexIQ, every release must include the version number and release notes in the reply.
- For CortexIQ, production deployment follows the fixed procedure: backup .env, pull while running, verified dump, tag bump, and check healthz/readyz/restarts/logs.
- For CortexIQ, Bo previously published releases manually from v0.1.74; that habit has now been replaced.

## Episode 670

> CortexIQ: v0.1.20 (e56a03b) is op 2026-11-05 gedeployed naar strato-vps; v0.1.19 is wel gebouwd maar nooit uitgerold, productie ging direct van 0.1.18 naar 0.1.20. Bevat migraties 00044 (judged), 00045 (origin/origin_ref) en 00046 (strength), de rebuild-CLI en CORTEXIQ_RRF_K. Volgorde: geverifieerde dump backups/cortexiq-20261105T002405Z.dump, release, .env gebackupt als .env.bak-pre0.1.20, tag-bump en up -d. Gecontroleerd: version v0.1.20, goose 46, containers Up, geen panic of migratiefout, 2330 facts allemaal zonder origin (oude facts, bewust niet teruggevuld). Een read-only rebuild-rapport van /projects/cortexiq in productie: 114 episodes, 476 facts, 6 facts op ingetrokken bronnen (gat C). CORTEXIQ_DECAY_SPLIT staat NIET aan en docker-compose.prod.yml op de server geeft de variabele nog niet door. Open voor Bo: de ssh- en gh-release-allow-regels uit de projectsettings halen; beslissen over DECAY_SPLIT; zelf de RRF-sweep draaien (cortexiq eval recall --rrf-k 20,60,100).

**A production** (numbers kept 15/17, invented [], gate would reject 0)

- CortexIQ v0.1.20 (e56a03b) was deployed to strato-vps on 2026-11-05; v0.1.19 was built but never rolled out, so production went directly from 0.1.18 to 0.1.20.
- CortexIQ v0.1.20 includes migrations 00044 (judged), 00045 (origin/origin_ref), 00046 (strength), the rebuild-CLI, and the CORTEXIQ_RRF_K environment variable.
- The CortexIQ v0.1.20 deployment to strato-vps followed the order: verified dump backups/cortexiq-20261105T002405Z.dump, performed the release, backed up .env as .env.bak-pre0.1.20, did the tag-bump, and ran up -d.
- After deployment, CortexIQ showed version v0.1.20, goose 46, containers Up, no panic or migration error, and 2330 facts all without origin (old facts deliberately not backfilled).
- A read-only rebuild report of /projects/cortexiq in production measured 114 episodes, 476 facts, and 6 facts on retracted sources (gap C).
- CORTEXIQ_DECAY_SPLIT was not enabled and docker-compose.prod.yml on the server did not pass the variable yet.

**B variant** (numbers kept 15/17, invented [], gate would reject 0)

- CortexIQ v0.1.20 (commit e56a03b) was deployed to strato-vps on 2026-11-05; version 0.1.19 was built but never rolled out, so production went directly from 0.1.18 to 0.1.20.
- CortexIQ v0.1.20 includes migrations 00044 (judged), 00045 (origin/origin_ref) and 00046 (strength), the rebuild-CLI and the CORTEXIQ_RRF_K setting.
- CortexIQ v0.1.20 deployment on 2026-11-05 followed this sequence: verified dump backups/cortexiq-20261105T002405Z.dump, release, .env backed up as .env.bak-pre0.1.20, tag-bump and up -d.
- After deployment, CortexIQ v0.1.20 was verified: version v0.1.20, goose 46, containers Up, no panic or migration error, and 2330 facts all without origin (old facts deliberately not backfilled).
- A read-only rebuild report of /projects/cortexiq in production showed 114 episodes, 476 facts, and 6 facts on withdrawn sources (gap C).
- CORTEXIQ_DECAY_SPLIT was not enabled in CortexIQ and docker-compose.prod.yml on the server does not pass the variable.

## Episode 924

> CortexIQ besluit (2026-11-13, Bo): Jina (jina-reranker-v3.5) blijft de reranker in productie; geen eigen reranker op de VPS. Reden: lokaal (llama.cpp, bge-reranker-v2-m3) is ongeveer 2,9 s trager per recall (mogelijk 3-5 s op de VPS, gedeeld met andere workloads) en iets slechter (MRR 0,80 tegen 0,85; zie id 923). Niets gewijzigd in productie. Het risico dat Jina wegvalt (saldo, 429-limiet van 100.000 tokens per minuut, 404) wordt afgedekt door de metric recall_rerank_requests_total, de logregel per minuut, RERANK_CANDIDATES=30 en de graaf-routing als vangnet (gewogen is dan beter dan uit). Niet gedaan en niet nodig tenzij het pijn doet: fallback-keten Jina dan lokaal (codewijziging), meten met 10 kandidaten, meten op de VPS zelf.

**A production** (numbers kept 13/15, invented [2.9 0.80 0.85], gate would reject 0)

- On 2026-11-13, CortexIQ decided to keep Jina (jina-reranker-v3.5) as the production reranker and not to run a custom reranker on the VPS; no changes were made in production.
- CortexIQ's local reranker (llama.cpp, bge-reranker-v2-m3) was measured as about 2.9 seconds slower per recall than Jina (possibly 3-5 seconds on the VPS, shared with other workloads) and slightly worse in MRR (0.80 vs 0.85, see id 923).
- As of 2026-11-13, CortexIQ covered the risk of the Jina reranker failing (due to balance, 429 limit of 100,000 tokens per minute, or 404) via the metric recall_rerank_requests_total, a log line per minute, RERANK_CANDIDATES=30, and graph-routing as a fallback (weighted is then better than out).
- As of 2026-11-13, CortexIQ had not implemented and did not currently need, unless it causes problems, a fallback chain of Jina then local reranker (code change), measuring with 10 candidates, and measuring on the VPS itself.

**B variant** (numbers kept 12/15, invented [2.9 0.80 0.85], gate would reject 0)

- CortexIQ kept Jina (jina-reranker-v3.5) as the production reranker and did not deploy its own reranker on the VPS.
- CortexIQ's local reranker (llama.cpp with bge-reranker-v2-m3) was measured to be about 2.9 s slower per recall (possibly 3-5 s on the VPS shared with other workloads) and slightly worse in MRR (0.80 vs 0.85) compared to Jina (see id 923).
- CortexIQ covers the risk of Jina becoming unavailable (balance, 429 limit of 100,000 tokens per minute, 404) through the metric recall_rerank_requests_total, a per-minute log line, RERANK_CANDIDATES=30, and graph routing as a fallback (weighted is better than out).
- CortexIQ did not implement a fallback chain from Jina to local reranker (code change), did not measure with 10 candidates, and did not measure on the VPS itself; these were considered not needed unless performance degradation occurs.

## Episode 664

> CortexIQ: v0.1.18 (afgewezen-waarde-tombstone, migratie 00043, Reflect-tab #7) is op 2026-11-05 gedeployed naar strato-vps. Volgorde: geverifieerde dump (backups/cortexiq-20261104T232239Z.dump), release gepubliceerd door Bo (de auto-mode classifier weigerde gh release create van mij), .env gebackupt als .env.bak-pre0.1.18, tag-bump en up -d. Gecontroleerd: version v0.1.18 (6c89dd3), goose 43, facts_refused in de logs. Productie had 0 retracties, dus de backfill deed niets. Het Gemma-parsefoutje bij causal links in /projects/chronicle is het bekende lijstprobleem, niet nieuw. Praktisch: SSH naar strato-vps werkt via een allow-regel in de projectsettings; die hoort na de deploy weg.

**A production** (numbers kept 6/8, invented [], gate would reject 0)

- CortexIQ v0.1.18 was deployed to strato-vps on 2026-11-05.
- The CortexIQ v0.1.18 deployment to strato-vps followed the order: verified dump (backups/cortexiq-20261104T232239Z.dump), release published by Bo (auto-mode classifier refused gh release create), .env backed up as .env.bak-pre0.1.18, tag-bump and up -d.
- After deployment, CortexIQ v0.1.18 was checked: version v0.1.18 (commit 6c89dd3), goose migration 43, and facts_refused entries in the logs.
- CortexIQ production had 0 retractions, so the backfill did nothing.
- The Gemma parse error on causal links in /projects/chronicle is a known list problem, not a new issue.
- SSH access to strato-vps works via an allow-rule in the project settings; that rule should be removed after the deployment.

**B variant** (numbers kept 8/8, invented [], gate would reject 0)

- CortexIQ version v0.1.18 (with afgewezen-waarde-tombstone, migratie 00043, and Reflect-tab #7) was deployed to strato-vps on 2026-11-05. The deployment steps were: verified dump (backups/cortexiq-20261104T232239Z.dump), release published by Bo (the auto-mode classifier refused to create a gh release), .env backed up as .env.bak-pre0.1.18, tag-bump and up -d.
- Post-deployment verification of CortexIQ showed version v0.1.18 (commit 6c89dd3), goose 43, and facts_refused in the logs.
- CortexIQ production had 0 retractions, so the backfill did nothing.
- The Gemma parsing error with causal links in /projects/chronicle is the known list problem, not new.
- SSH access to strato-vps was working via an allow-rule in the project settings; that rule is supposed to be removed after the deploy.

---

episodes 21 | A: 96 facts, numbers kept 141/171, invented 7 | B: 97 facts, numbers kept 143/171, invented 10, would be rejected by the production gate 1
