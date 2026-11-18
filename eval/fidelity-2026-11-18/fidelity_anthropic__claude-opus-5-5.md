# Fact fidelity: anthropic/claude-opus-5-5

A = production prompt and gate. B = rule replaced, no gate. Read the facts against the episode: subject and object right? detail kept? anything added?

## Episode 989

> Spike 16-11-2026: CortexIQ 0.1.88 (strato-vps, REST 127.0.0.1:18101) vergeleken met Hindsight 0.9.2 (contabo, 8888) als vervanger van Hindsight in Gaia. Beslissend verschil: CortexIQ filtert op occurred_at met from/to (half-open) en herkent Nederlandse datumzinnen over REST, terwijl Hindsight temporal_window alleen rankt (een venster van 1 dag gaf 54 hits over vijf dagen) en memories/list geen datumfilter heeft. Scoping moet met namespaces (array in REST); de enkelvoudige namespace is alleen een ranking-zetje. remember bewaart content, namespace, tags, occurred_at, external_key, external_refs en importance, maar gooit vrije metadata weg; external_key maakt de write idempotent (unchanged:true, andere inhoud geweigerd tenzij supersedes), dus gaia:kairos:<episode_id> is de spiegel-sleutel. DELETE /api/v1/episodes/:id purge hard en cascadeert naar afgeleide facts; facts en namespaces zijn niet los te verwijderen via REST (404/405). Consolidatie reageert binnen twee minuten maar levert even dunne facts als Hindsight. Op twintig Nederlandse vragen: CortexIQ mediaan 1.061 ms met ruwe episodes bovenaan, Hindsight 1.372 ms met een door emotionele gaia-herinneringen verzadigde top. Rapport: docs/spike-cortexiq.md (commits 85dd48e en f329e24). Volgorde blijft: MemoryStore-poort, terugkijk-route op CortexIQ, gaia-logos rebuilden, bojan/gaia importeren, Hindsight uit.

**A production** (numbers kept 13/13, invented [1-day], gate would reject 0)

- A Gaia AI-assistent spike on 16-11-2026 compared CortexIQ 0.1.88 (on strato-vps, REST at 127.0.0.1:18101) with Hindsight 0.9.2 (on contabo, port 8888) as a replacement for Hindsight. The decisive difference was date handling. CortexIQ filtered on occurred_at using a half-open from/to range and recognised Dutch date phrases over REST. Hindsight's temporal_window only influenced ranking: a 1-day window returned 54 hits spread over five days. Hindsight's memories/list also had no date filter.
- In the Gaia AI-assistent spike, scoping in CortexIQ 0.1.88 required the namespaces parameter, which is an array in REST. The singular namespace parameter acted only as a ranking boost, not as a filter.
- In the Gaia AI-assistent spike, the CortexIQ 0.1.88 remember call stored content, namespace, tags, occurred_at, external_key, external_refs and importance, but discarded free-form metadata. external_key made writes idempotent: a repeated write returned unchanged:true, and different content under the same key was rejected unless supersedes was given. For that reason gaia:kairos:<episode_id> was chosen as the mirror key.
- In the Gaia AI-assistent spike, DELETE /api/v1/episodes/:id in CortexIQ 0.1.88 performed a hard purge that cascaded to the facts derived from that episode. Facts and namespaces could not be deleted separately via REST; such attempts returned 404/405.
- In the Gaia AI-assistent spike, CortexIQ 0.1.88 consolidation responded within two minutes but produced facts as thin as Hindsight's. On twenty Dutch questions, CortexIQ had a median of 1.061 ms with raw episodes at the top of the results. Hindsight 0.9.2 had a median of 1.372 ms, with a top dominated by emotional gaia memories.
- The CortexIQ spike for the Gaia AI-assistent was documented in docs/spike-cortexiq.md (commits 85dd48e and f329e24). The planned migration order stayed unchanged: first a MemoryStore port, then the look-back route on CortexIQ, then rebuilding gaia-logos, then importing bojan/gaia, and finally switching Hindsight off.

**B variant** (numbers kept 13/13, invented [1-day], gate would reject 0)

- In the Gaia spike of 16-11-2026, CortexIQ 0.1.88 (strato-vps, REST 127.0.0.1:18101) was compared with Hindsight 0.9.2 (contabo, 8888) as a replacement for Hindsight in Gaia. The decisive difference: CortexIQ filters on occurred_at with from/to (half-open) and recognises Dutch date phrases over REST. Hindsight temporal_window only ranks (a 1-day window gave 54 hits spread over five days), and Hindsight memories/list has no date filter.
- In the Gaia spike, CortexIQ 0.1.88 scoping had to be done with namespaces (an array in REST); the singular namespace was only a ranking nudge. CortexIQ remember stored content, namespace, tags, occurred_at, external_key, external_refs and importance, but discarded free metadata. external_key made the write idempotent (unchanged:true; different content refused unless supersedes), so gaia:kairos:<episode_id> is the mirror key for Gaia.
- In the Gaia spike, CortexIQ 0.1.88 DELETE /api/v1/episodes/:id hard-purged the episode and cascaded to derived facts. Facts and namespaces could not be deleted separately via the CortexIQ REST API (404/405).
- In the Gaia spike, CortexIQ 0.1.88 consolidation reacted within two minutes but produced facts as thin as those of Hindsight.
- In the Gaia spike, on twenty Dutch questions CortexIQ 0.1.88 measured a median of 1.061 ms with raw episodes at the top, while Hindsight 0.9.2 measured 1.372 ms with a top saturated by emotional gaia memories.
- The Gaia CortexIQ spike report is docs/spike-cortexiq.md (commits 85dd48e and f329e24). The planned order for Gaia remains: MemoryStore port, look-back route on CortexIQ, rebuild gaia-logos, import bojan/gaia, then turn Hindsight off.

## Episode 1082

> Migratieplan Hindsight→CortexIQ in Gaia-Cloud vastgesteld op 18-11-2026 (plansessie): volgorde zoals de spike van 16-11 bepaalde (MemoryStore-poort → terugkijk-route → gaia-logos rebuild → bojan/gaia import → Hindsight uit). Twee beslissingen van Bo, beide 'meten gaat voor bouwen': (1) CortexIQ-fase 1 (quota recall over namespaces) wordt pas gebouwd nadat een fixture op de live strato-instance aantoonbaar maakt dat één luide namespace de rest verdringt — geen bewijs, geen bouw; (2) hosting van CortexIQ voor Gaia (strato via Tailscale vs eigen instance op contabo) wordt pas gekozen na een latency-meting contabo→strato, uit te voeren nadat de REST aan de tailnet-kant is gebonden (nu 127.0.0.1:18101). Fase 6 (agent_identity) voorlopig geschrapt: namespace + tag dekt de epistemic mark. Fase 0-huiskeeping: episode 980 (Mindstack, Vite-migratie) opnieuw aanleveren; oude hindsight-backups van 03-05 dienen alleen als formaat-referentie bij de verse export in fase 4.

**A production** (numbers kept 9/10, invented [], gate would reject 0)

- The Gaia AI-assistent plan for migrating Gaia-Cloud from Hindsight to CortexIQ was set in a planning session on 18-11-2026. It follows the order determined by the spike of 16-11: MemoryStore port, then the terugkijk-route, then the gaia-logos rebuild, then the bojan/gaia import, and finally switching Hindsight off.
- Bo decided under the principle 'meten gaat voor bouwen' that CortexIQ phase 1 for Gaia, quota recall across namespaces, will only be built once a fixture on the live strato instance demonstrates that one noisy namespace crowds out the others. Without that proof, phase 1 will not be built.
- Bo decided that the hosting choice for CortexIQ in Gaia, strato via Tailscale versus a separate instance on contabo, will only be made after a latency measurement from contabo to strato. That measurement is to be taken after the CortexIQ REST interface is bound to the tailnet side; at the 18-11-2026 planning session it was bound to 127.0.0.1:18101.
- Phase 6 (agent_identity) of the Gaia-Cloud Hindsight→CortexIQ migration plan was provisionally dropped. The stated reason is that namespace plus tag already covers the epistemic mark.
- Phase 0 housekeeping in the Gaia-Cloud Hindsight→CortexIQ migration plan includes resubmitting episode 980, which concerns the Mindstack Vite migration.
- In the Gaia-Cloud Hindsight→CortexIQ migration plan, the old Hindsight backups from 03-05 serve only as a format reference for the fresh export in phase 4.

**B variant** (numbers kept 9/10, invented [], gate would reject 0)

- The Gaia-Cloud migration plan from Hindsight to CortexIQ was set on 18-11-2026 in a planning session. It follows the order determined by the spike of 16-11: MemoryStore-poort → terugkijk-route → gaia-logos rebuild → bojan/gaia import → Hindsight uit.
- Bo decided on the principle 'meten gaat voor bouwen' for the Gaia-Cloud migration to CortexIQ. CortexIQ-fase 1 (quota recall over namespaces) will only be built after a fixture on the live strato-instance demonstrably shows that one loud namespace crowds out the rest. Without that proof, fase 1 will not be built.
- Bo decided on the principle 'meten gaat voor bouwen' for CortexIQ hosting for Gaia. The choice between strato via Tailscale and an own instance on contabo will only be made after a latency measurement from contabo to strato. That measurement is to be done after the CortexIQ REST is bound to the tailnet side; at the time of the plan the REST was bound to 127.0.0.1:18101.
- CortexIQ-fase 6 (agent_identity) was dropped for now in the Gaia-Cloud migration plan, because namespace + tag covers the epistemic mark.
- As part of fase 0 housekeeping in the Gaia-Cloud migration from Hindsight to CortexIQ, episode 980 (Mindstack, Vite-migratie) is to be resubmitted.
- In the Gaia-Cloud migration from Hindsight to CortexIQ, the old Hindsight backups from 03-05 serve only as a format reference for the fresh export in fase 4.

## Episode 558

> Gap-analyse CortexIQ vs Hindsight voor Gaia-Cloud (2026-11): Gaia-Cloud hangt via src/hindsightClient.js aan Hindsight voor: (1) banks + cross-bank recall met per-bank quota (scores niet op één schaal), (2) willekeurige tags per memory-item die in recall-output terugkomen (gaia:* tags, memoryKindOf), (3) versiegelde invalidatie/superseding sync vanuit cognitionSync met reconcile, (4) provenance sources:['foundation:<id>'] voor Dual Recall, (5) async fire-and-forget retain, (6) agent_identity-enforcing. CortexIQ heeft inmiddels wél reflect, mental models, reranker, graph-arm, temporal recall en MMR — die eerdere gaten zijn gedicht.

**A production** (numbers kept 4/7, invented [], gate would reject 0)

- A 2026-11 gap analysis of CortexIQ versus Hindsight found that Gaia-Cloud, through src/hindsightClient.js, relies on Hindsight for memory banks and for cross-bank recall with a per-bank quota, because recall scores across banks are not on a single scale.
- Gaia-Cloud relies on Hindsight, via src/hindsightClient.js, for arbitrary tags on each memory item that come back in recall output. This covers the gaia:* tags and memoryKindOf, according to the 2026-11 CortexIQ-vs-Hindsight gap analysis.
- Gaia-Cloud relies on Hindsight for versioned invalidation and superseding sync driven by cognitionSync, including a reconcile step. This dependency was identified in the 2026-11 CortexIQ-vs-Hindsight gap analysis.
- Gaia-Cloud relies on Hindsight to store provenance in the form sources:['foundation:<id>'], which the Dual Recall feature uses. This was noted in the 2026-11 CortexIQ-vs-Hindsight gap analysis.
- Gaia-Cloud relies on Hindsight, via src/hindsightClient.js, for asynchronous fire-and-forget retain and for enforcement of agent_identity, according to the 2026-11 CortexIQ-vs-Hindsight gap analysis.
- By the time of the 2026-11 gap analysis against Hindsight for Gaia-Cloud, CortexIQ had gained reflect, mental models, a reranker, a graph-arm, temporal recall and MMR, closing those earlier gaps.

**B variant** (numbers kept 4/7, invented [], gate would reject 0)

- According to the 2026-11 gap analysis of CortexIQ vs Hindsight for Gaia-Cloud, Gaia-Cloud depends on Hindsight via src/hindsightClient.js for banks and cross-bank recall with a per-bank quota, because the scores are not on one scale.
- According to the 2026-11 gap analysis of CortexIQ vs Hindsight, Gaia-Cloud depends on Hindsight via src/hindsightClient.js for arbitrary tags per memory item that come back in the recall output, such as gaia:* tags and memoryKindOf.
- According to the 2026-11 gap analysis of CortexIQ vs Hindsight, Gaia-Cloud depends on Hindsight via src/hindsightClient.js for 'versiegelde' invalidation/superseding sync from cognitionSync with reconcile.
- According to the 2026-11 gap analysis of CortexIQ vs Hindsight, Gaia-Cloud depends on Hindsight via src/hindsightClient.js for provenance sources:['foundation:<id>'] used by Dual Recall.
- According to the 2026-11 gap analysis of CortexIQ vs Hindsight, Gaia-Cloud depends on Hindsight via src/hindsightClient.js for async fire-and-forget retain and for agent_identity enforcing.
- According to the 2026-11 gap analysis of CortexIQ vs Hindsight for Gaia-Cloud, CortexIQ by then had reflect, mental models, a reranker, a graph-arm, temporal recall and MMR, so those earlier gaps were closed.

## Episode 36

> Beslissing 2026-10-08 (akkoord Bo): de provenance-prefix voor afgeleide records in Gaia-Cloud gaat van 'chronicle:' naar 'foundation:'. Canonieke id-vorm: 'foundation:<uuid>' met de kale ingest_object uuid (wat Foundation /api/ingest-logs/:id verwacht), niet Foundation's episode-vorm 'ingest:<uuid>'. Reden: 'chronicle:' is legacy (Foundation, niet Chronicle, is de rauwe laag) en is een routing key, geen label. Wordt in één migratie gedaan die ook de strip-bug in kairosRoutes.js fixt en de drie sources-kolommen (hypotheses/patterns/kairos_episodes) plus Hindsight-metadata backfillt; reconcile() wordt uitgebreid met Kairos-episodes. Plan: C:\Users\bojan\.opencode\plan\chronicle-prefix-provenance-plan.md.

**A production** (numbers kept 1/1, invented [], gate would reject 0)

- On 2026-10-08, with Bo's approval, Gaia-Cloud decided to change the provenance prefix for derived records from 'chronicle:' to 'foundation:'. The stated reasons are that 'chronicle:' is legacy, since Foundation rather than Chronicle is the raw layer, and that the prefix is a routing key, not a label.
- The canonical Gaia-Cloud provenance id form was decided as 'foundation:<uuid>' using the bare ingest_object uuid, which is what Foundation /api/ingest-logs/:id expects. Foundation's episode form 'ingest:<uuid>' is explicitly not used.
- The Gaia-Cloud switch from the 'chronicle:' to the 'foundation:' prefix is planned as a single migration. That migration also fixes the strip-bug in kairosRoutes.js and backfills three sources columns (hypotheses, patterns and kairos_episodes) plus the Hindsight metadata.
- As part of the Gaia-Cloud provenance prefix migration, the reconcile() function is to be extended to also cover Kairos-episodes.
- The plan for the Gaia-Cloud migration from the 'chronicle:' to the 'foundation:' provenance prefix is documented at C:\Users\bojan\.opencode\plan\chronicle-prefix-provenance-plan.md.

**B variant** (numbers kept 1/1, invented [], gate would reject 6)

- Op 2026-10-08 is besloten (akkoord Bo) dat de provenance-prefix voor afgeleide records in Gaia-Cloud van 'chronicle:' naar 'foundation:' gaat.
- De canonieke provenance id-vorm in Gaia-Cloud is 'foundation:<uuid>' met de kale ingest_object uuid (wat Foundation /api/ingest-logs/:id verwacht), niet Foundation's episode-vorm 'ingest:<uuid>'.
- De Gaia-Cloud provenance-prefix 'chronicle:' wordt vervangen omdat 'chronicle:' legacy is (Foundation, niet Chronicle, is de rauwe laag) en omdat de prefix een routing key is, geen label.
- De Gaia-Cloud provenance-prefix wijziging naar 'foundation:' wordt in één migratie gedaan die ook de strip-bug in kairosRoutes.js fixt en de drie sources-kolommen (hypotheses/patterns/kairos_episodes) plus Hindsight-metadata backfillt.
- Als onderdeel van de Gaia-Cloud provenance-prefix migratie naar 'foundation:' wordt reconcile() uitgebreid met Kairos-episodes.
- Het plan voor de Gaia-Cloud provenance-prefix migratie van 'chronicle:' naar 'foundation:' staat in C:\Users\bojan\.opencode\plan\chronicle-prefix-provenance-plan.md.

## Episode 37

> Gaia-Cloud: de chronicle:->foundation: provenance-normalisatie is op 2026-10-08 geïmplementeerd (nieuwe seam src/foundationRef.js, synthesizer+kairosRoutes erop, cognitionSync.reconcile() pusht nu ook Kairos-episodes, migratie 010_normalize_sources_prefix.sql). Tests groen: gaia-api 1244/1244, cognition 49/49. Nog open: migratie toepassen op de VPS (npm run migrate in services/cognition) en daarna npm run reconcile:hindsight -- --apply --clear om de Hindsight-metadata te verversen.

**A production** (numbers kept 4/4, invented [], gate would reject 0)

- In Gaia-Cloud was the provenance normalization from the chronicle: prefix to the foundation: prefix implemented on 2026-10-08, together with migration 010_normalize_sources_prefix.sql.
- Gaia-Cloud got a new seam src/foundationRef.js as part of the provenance normalization, and the synthesizer and kairosRoutes were moved onto this seam.
- In Gaia-Cloud, cognitionSync.reconcile() was changed during the provenance normalization so that it also pushes Kairos-episodes.
- After the Gaia-Cloud provenance normalization on 2026-10-08, all tests were passing: gaia-api 1244/1244 and cognition 49/49.
- As of 2026-10-08, two Gaia-Cloud steps were still open. Migration 010_normalize_sources_prefix.sql still had to be applied on the VPS via npm run migrate in services/cognition. After that, npm run reconcile:hindsight -- --apply --clear had to be run to refresh the Hindsight metadata.

**B variant** (numbers kept 4/4, invented [], gate would reject 4)

- In Gaia-Cloud is de chronicle:->foundation: provenance-normalisatie op 2026-10-08 geïmplementeerd met een nieuwe seam src/foundationRef.js, waar de synthesizer en kairosRoutes op zijn overgezet.
- In Gaia-Cloud pusht cognitionSync.reconcile() sinds de chronicle:->foundation: provenance-normalisatie van 2026-10-08 ook Kairos-episodes.
- Voor de chronicle:->foundation: provenance-normalisatie in Gaia-Cloud is de migratie 010_normalize_sources_prefix.sql toegevoegd.
- Na de chronicle:->foundation: provenance-normalisatie op 2026-10-08 waren de Gaia-Cloud-tests groen: gaia-api 1244/1244 en cognition 49/49.
- Voor de Gaia-Cloud provenance-normalisatie stond op 2026-10-08 nog open: migratie 010_normalize_sources_prefix.sql toepassen op de VPS via npm run migrate in services/cognition en daarna npm run reconcile:hindsight -- --apply --clear uitvoeren om de Hindsight-metadata te verversen.

## Episode 38

> Reconcile dry-run Gaia-Cloud 2026-10-08: Hindsight actief gaia:hypothesis=2, gaia:pattern=0, gaia:kairos_episode=0; Cognition hypotheses=4, patterns=0, kairos=0. De foundation:-prefix-backfill bleek een no-op: de oude 'chronicle:'-prefix werd alleen door de Kairos-synthesizer geproduceerd en er zijn geen Kairos-episodes (GAIA_KAIROS_ENABLED staat uit), dus --apply --clear is overgeslagen (zou enkel versie-churn geven). De 4-vs-2 kan deels survey-artefact zijn: open_question/relationship/mental_model dragen andere tags en tellen niet als gaia:hypothesis.

**A production** (numbers kept 4/5, invented [], gate would reject 0)

- During the Gaia-Cloud reconcile dry-run on 2026-10-08, Hindsight held active entries counted as gaia:hypothesis=2, gaia:pattern=0 and gaia:kairos_episode=0.
- During the Gaia-Cloud reconcile dry-run on 2026-10-08, Cognition reported 4 hypotheses, 0 patterns and 0 kairos entries.
- The Gaia-Cloud foundation:-prefix backfill turned out to be a no-op. The old 'chronicle:' prefix came only from the Kairos synthesizer, and no Kairos episodes existed. For that reason the --apply --clear run was skipped, since it would only have caused version churn.
- On 2026-10-08, GAIA_KAIROS_ENABLED was observed to be switched off in Gaia-Cloud, so no Kairos episodes had been produced.
- The mismatch between 4 Cognition hypotheses and 2 Hindsight gaia:hypothesis entries in Gaia-Cloud may partly be a survey artefact. Entries of type open_question, relationship and mental_model carry different tags and are not counted as gaia:hypothesis.

**B variant** (numbers kept 4/5, invented [], gate would reject 2)

- Bij de reconcile dry-run van Gaia-Cloud op 2026-10-08 telde Hindsight actief gaia:hypothesis=2, gaia:pattern=0 en gaia:kairos_episode=0.
- Bij de reconcile dry-run van Gaia-Cloud op 2026-10-08 telde Cognition hypotheses=4, patterns=0 en kairos=0.
- De foundation:-prefix-backfill van Gaia-Cloud bleek op 2026-10-08 een no-op: de oude 'chronicle:'-prefix werd alleen door de Kairos-synthesizer geproduceerd en er waren geen Kairos-episodes, dus --apply --clear werd overgeslagen omdat dat enkel versie-churn zou geven.
- Bij de reconcile dry-run van Gaia-Cloud op 2026-10-08 stond GAIA_KAIROS_ENABLED uit, waardoor er geen Kairos-episodes waren.
- Het verschil tussen 4 Cognition-hypotheses en 2 Hindsight gaia:hypothesis bij de Gaia-Cloud reconcile dry-run kan deels een survey-artefact zijn: open_question, relationship en mental_model dragen andere tags en tellen niet als gaia:hypothesis.

## Episode 81

> Gaia-Cloud aanvulling (commit 3f645dd, 2026-10-12): een op het verleden gerichte beurt raadpleegt nu zowel Hindsight als het transcript. recallRelevantContext (memory.js) kreeg een `pastReference`-optie die de recall-gate opent; turn.js geeft die door zodra de gesprekszoek-signalen (exactHistory/pastLookup/rememberedKnowledge) vuren. Reden: Bo's punt 'ze kijkt ook in Hindsight' — de twee gates (memoryPolicy.shouldRecall en de gesprekszoek-signalen) waren onafhankelijk, dus sommige lookup-formuleringen openden alleen het transcript. Let op: Hindsight draait sowieso al elke beurt via de mentale modellen (fetchMentalModelContext), ongeacht de gate. gaia-api 1433/1433 groen (+2).

**A production** (numbers kept 4/4, invented [], gate would reject 0)

- In Gaia-Cloud (commit 3f645dd, 2026-10-12), a conversation turn that refers to the past now consults both Hindsight and the transcript instead of sometimes only the transcript.
- In Gaia-Cloud, recallRelevantContext in memory.js gained a `pastReference` option that opens the recall gate; turn.js passes this option when the conversation-search signals exactHistory, pastLookup or rememberedKnowledge fire.
- The Gaia-Cloud pastReference change was made after Bo pointed out that Gaia also looks in Hindsight: the gate memoryPolicy.shouldRecall and the conversation-search signals were independent, so some lookup phrasings opened only the transcript.
- In Gaia-Cloud, Hindsight is already queried on every turn through the mental models (fetchMentalModelContext), independent of the recall gate.
- After Gaia-Cloud commit 3f645dd, the gaia-api test suite was observed at 1433/1433 passing, including 2 new tests.

**B variant** (numbers kept 4/4, invented [], gate would reject 0)

- In Gaia-Cloud, as of commit 3f645dd (2026-10-12), a past-oriented turn consults both Hindsight and the transcript. The change followed Bo's point 'ze kijkt ook in Hindsight': the two gates (memoryPolicy.shouldRecall and the conversation-search signals) were independent, so some lookup phrasings opened only the transcript.
- In Gaia-Cloud commit 3f645dd, recallRelevantContext (memory.js) gained a `pastReference` option that opens the recall gate. turn.js passes `pastReference` as soon as the conversation-search signals (exactHistory/pastLookup/rememberedKnowledge) fire.
- In Gaia-Cloud, Hindsight already runs on every turn through the mental models (fetchMentalModelContext), regardless of the recall gate.
- At Gaia-Cloud commit 3f645dd (2026-10-12), the gaia-api tests were observed at 1433/1433 green, with 2 tests added.

## Episode 86

> Gaia-Cloud (2026-10-12, commit 8e26794, gedeployed): (1) recall-staart gemeten uit Hindsight's Prometheus-histogram (hindsight_http_duration_seconds, recall-endpoint): 27 recalls, mean 894ms, p50 ~1s, p90/p95 ~2,5s, p99 ~5s; 1 call (3,7%) boven 4s, 0 boven 8s. Kleine, niet-representatieve steekproef (grotendeels test-calls), buckets grof. (2) Observability: elke gefaalde Hindsight-call logt nu een content-loze regel {kind:'hindsight.call_failed', bank, op, reason:'timeout'|'unreachable', timeoutMs, ms} en throwt door — via een request()-wrapper rond fetchImpl in src/hindsightClient.js (alle 14 call-sites). Reden: memory.js slikte Hindsight-fouten stil in (best-effort -> []), dus missende context was onzichtbaar. Test test/hindsightCallLog.test.js (+3), gaia-api 1436/1436. Het is puur een logregel, geen gedragsverandering; verschijnt in de gaia-api containerlog.

**A production** (numbers kept 17/18, invented [], gate would reject 0)

- Op 2026-10-12 werd voor Gaia-Cloud de recall-latency van Hindsight gemeten via het Prometheus-histogram hindsight_http_duration_seconds (recall-endpoint): 27 recalls, gemiddeld 894ms, p50 ongeveer 1s, p90/p95 ongeveer 2,5s, p99 ongeveer 5s; 1 call (3,7%) duurde langer dan 4s en geen enkele langer dan 8s. De steekproef was klein en niet-representatief (vooral test-calls) en de histogram-buckets waren grof.
- In Gaia-Cloud (commit 8e26794, gedeployed 2026-10-12) logt elke mislukte Hindsight-call een regel zonder inhoud met kind 'hindsight.call_failed' en de velden bank, op, reason ('timeout' of 'unreachable'), timeoutMs en ms, en wordt de fout daarna opnieuw gegooid. Dit gebeurt via een request()-wrapper rond fetchImpl in src/hindsightClient.js, die alle 14 call-sites dekt. De regel verschijnt in de containerlog van gaia-api en verandert het gedrag niet.
- In Gaia-Cloud ving memory.js Hindsight-fouten stil op door best-effort een lege array terug te geven, waardoor ontbrekende context onzichtbaar bleef. Dit was de reden om in src/hindsightClient.js de logregel 'hindsight.call_failed' voor mislukte Hindsight-calls toe te voegen.
- Bij commit 8e26794 van Gaia-Cloud werd de test test/hindsightCallLog.test.js toegevoegd (3 nieuwe tests) en slaagden alle 1436 van de 1436 gaia-api-tests.

**B variant** (numbers kept 18/18, invented [], gate would reject 0)

- Gaia-Cloud measured on 2026-10-12 the recall latency tail from Hindsight's Prometheus-histogram (hindsight_http_duration_seconds, recall-endpoint): 27 recalls, mean 894ms, p50 ~1s, p90/p95 ~2,5s, p99 ~5s; 1 call (3,7%) above 4s, 0 above 8s. The sample was small and not representative (largely test-calls) and the buckets were coarse.
- In Gaia-Cloud (commit 8e26794, deployed 2026-10-12) every failed Hindsight call logs a content-less line {kind:'hindsight.call_failed', bank, op, reason:'timeout'|'unreachable', timeoutMs, ms} and rethrows. The logging is implemented via a request()-wrapper around fetchImpl in src/hindsightClient.js covering all 14 call-sites. The change is purely a log line with no behavior change, and the line appears in the gaia-api container log.
- In Gaia-Cloud, memory.js silently swallowed Hindsight errors (best-effort -> []), so missing context was invisible. This was the reason for adding the hindsight.call_failed log line in src/hindsightClient.js.
- Gaia-Cloud added test/hindsightCallLog.test.js (+3 tests) for the Hindsight call-failure logging, and gaia-api passed 1436/1436 tests at commit 8e26794.

## Episode 560

> Beslissing: LongMemEval-harnas is uitgeschreven als docs/design-longmemeval.md in cortexiq (commit eb37c41, gepusht naar main 2026-11-02). Belangrijkste bevinding: internal/benchmark bestaat al en is eerlijk (ingest als episodes, antwoorden via reflect, beide judges) — het gat is de officiële dataset + fairness-protocol + publicatie. Kernregels: judge is een ander/sterker model dan het antwoordmodel, abstention krijgt een speciale judge-regel, geen benchmark-specifieke tuning, alleen de S-variant draaien flattert consolidatie (dus ook de M/day-variant), Hindsight-cijfers worden gequoteerd met bron als hun meting.

**A production** (numbers kept 2/2, invented [], gate would reject 0)

- The LongMemEval harness design for CortexIQ was written up as docs/design-longmemeval.md in the cortexiq repository, committed as eb37c41 and pushed to the main branch on 2026-11-02.
- The CortexIQ package internal/benchmark already existed and was judged fair: it ingests data as episodes, produces answers through reflect, and applies both judges.
- The remaining gap for the CortexIQ LongMemEval harness was identified as the official LongMemEval dataset, a fairness protocol, and publication of results.
- The CortexIQ LongMemEval harness rules require the judge to be a different or stronger model than the answering model, apply a special judge rule to abstention, and forbid benchmark-specific tuning.
- The CortexIQ LongMemEval harness must also run the M/day variant because running only the LongMemEval S variant would flatter CortexIQ consolidation.
- In the CortexIQ LongMemEval harness, Hindsight benchmark figures are to be quoted with their source and presented as Hindsight's own measurement.

**B variant** (numbers kept 2/2, invented [], gate would reject 0)

- The LongMemEval harness for the Gaia AI-assistent was written up as docs/design-longmemeval.md in the cortexiq repository (commit eb37c41, pushed to main on 2026-11-02).
- The main finding of the cortexiq LongMemEval design was that internal/benchmark already exists and is fair: it ingests as episodes, answers via reflect, and uses both judges. The gap for LongMemEval is the official dataset, a fairness protocol and publication.
- A core rule of the cortexiq LongMemEval harness is that the judge is a different or stronger model than the answering model, and abstention gets a special judge rule.
- A core rule of the cortexiq LongMemEval harness is that no benchmark-specific tuning is applied.
- A core rule of the cortexiq LongMemEval harness is to run the M/day variant as well, because running only the LongMemEval S-variant flatters consolidation.
- A core rule of the cortexiq LongMemEval harness is that Hindsight figures are quoted with their source, as Hindsight's own measurement.

## Episode 364

> Gaia Cloud now retains eligible Hindsight turns as a Hermes-style JSON array of role-labelled User and Assistant messages. Existing memoryworthiness/triviality gates, asynchronous fire-and-forget behavior, and identity/provenance metadata remain; no historical Hindsight data is rewritten. Deployed as commit bddbad5 on 2026-10-23.

**A production** (numbers kept 2/2, invented [], gate would reject 0)

- Gaia Cloud (Gaia AI-assistent) was changed to store eligible Hindsight turns as a Hermes-style JSON array in which each message carries a User or Assistant role label.
- After the Hermes-style format change for Hindsight retention in Gaia Cloud, the existing memoryworthiness and triviality gates, the asynchronous fire-and-forget retention behavior, and the identity/provenance metadata were all kept unchanged.
- The Gaia Cloud switch to Hermes-style Hindsight turn storage does not rewrite any previously stored Hindsight data; only newly retained turns use the new format.
- The Gaia Cloud change to Hermes-style role-labelled Hindsight turn retention was deployed as commit bddbad5 on 2026-10-23.

**B variant** (numbers kept 2/2, invented [], gate would reject 1)

- Gaia Cloud now retains eligible Hindsight turns as a Hermes-style JSON array of role-labelled User and Assistant messages.
- With the Hermes-style Hindsight retention change, Gaia Cloud keeps the existing memoryworthiness/triviality gates, asynchronous fire-and-forget behavior, and identity/provenance metadata.
- The Gaia Cloud change to Hermes-style Hindsight turn retention does not rewrite any historical Hindsight data.
- The Gaia Cloud Hermes-style Hindsight turn retention change was deployed as commit bddbad5 on 2026-10-23.

## Episode 859

> CortexIQ BM25-repetitie op de productiedump (2026-11-11): CI op 3774ea9 groen. Dagelijkse prod-dump (47 MB, checksum OK; prod = 2.944 facts, 728 episodes, 128 MB, goose 60, bge-m3/dim 1024) in verse PG17-container met deploy/postgres-image: pg_restore 5 s, db.Open inclusief migratie 00061 335 ms, bm25-indexen aanwezig, tsvector-kolommen weg. Kwaliteit: BM25 wint bij inspectie van 3 van 15 queries (IDF: zeldzame tokens als 8445 en bge-m3 komen bovenaan, ts_rank_cd gaf generieke woorden); geen gelabelde eval gedraaid, vervolgstap is cortexiq eval recall. Snelheidswinst is op dit kleine corpus nog theoretisch. VPS-toegang was read-only (backuplijst, scp van dump); dump en container na afloop verwijderd. OPEN: prod nog op PG16 en niet geüpgraded; release-job voor cortexiq-postgres en arm64 nooit gedraaid; Bo moet de ssh- en scp-allow-regels en de oude gh-release-create-regel uit .claude/settings.local.json halen (bestand is in git gevolgd, niet committen). Commit van de docs: zie git log.

**A production** (numbers kept 16/21, invented [], gate would reject 0)

- During the CortexIQ BM25 rehearsal on 2026-11-11, the CortexIQ CI pipeline passed (green) on commit 3774ea9.
- On 2026-11-11 the daily CortexIQ production dump measured 47 MB with a valid checksum. Production then held 2,944 facts and 728 episodes in 128 MB, at goose migration version 60, using bge-m3 embeddings with dimension 1024. The dump was fetched over read-only VPS access (backup list and scp), and both the dump and the test container were deleted afterwards.
- The CortexIQ production dump was restored into a fresh PostgreSQL 17 container built from the deploy/postgres-image. pg_restore took 5 seconds, and db.Open including migration 00061 took 335 ms. Afterwards the bm25 indexes existed and the tsvector columns were gone.
- Manual inspection of 15 CortexIQ queries found BM25 ranking better than ts_rank_cd in 3 of them. Because of IDF weighting, rare tokens such as 8445 and bge-m3 ranked at the top, whereas ts_rank_cd had favoured generic words. No labelled evaluation was run; the planned next step is cortexiq eval recall. Any speed gain from BM25 was still theoretical on a corpus this small.
- As of 2026-11-11, CortexIQ production was still running PostgreSQL 16 and had not been upgraded. The release job for the cortexiq-postgres image and the arm64 build had never been run.
- An open task for Bo in CortexIQ is to remove the ssh and scp allow rules and the old gh release create rule from .claude/settings.local.json. That file is tracked in git, and the changes must not be committed.

**B variant** (numbers kept 16/21, invented [], gate would reject 0)

- In the CortexIQ BM25 rehearsal on 2026-11-11, with CI green on commit 3774ea9, the daily production dump (47 MB, checksum OK) was restored into a fresh PG17 container built with deploy/postgres-image. pg_restore took 5 s, db.Open including migration 00061 took 335 ms, the bm25 indexes were present and the tsvector columns were gone. Production was observed at that time with 2,944 facts, 728 episodes, 128 MB, goose version 60 and bge-m3 embeddings with dim 1024.
- In the CortexIQ BM25 rehearsal on 2026-11-11, BM25 won 3 of 15 queries on inspection, because IDF ranked rare tokens such as 8445 and bge-m3 at the top, while ts_rank_cd had favoured generic words. No labelled eval was run, and the next step is cortexiq eval recall. The speed gain on the small CortexIQ corpus was still theoretical at that time.
- As of 2026-11-11, CortexIQ production was still on PostgreSQL 16 (PG16) and had not been upgraded to PG17.
- As of 2026-11-11, the CortexIQ release job for the cortexiq-postgres image and for arm64 had never been run.
- For CortexIQ, Bo must remove the ssh and scp allow rules and the old gh-release-create rule from .claude/settings.local.json. That file is tracked in git and must not be committed.
- During the CortexIQ BM25 rehearsal on 2026-11-11, access to the VPS was read-only (listing the backups and copying the dump with scp). The dump and the container were deleted afterwards.

## Episode 759

> De productiebug supported_by is lokaal opgelost: de background consolidator selecteerde hypotheses zonder supported_by voor pgx named scanning, en positional hypothesis/counter scans misten het veld. Drie bestanden zijn aangepast, walkthrough bijgewerkt, go test ./internal/brain en go test ./... zijn groen. Niet gedeployed; aparte expliciete goedkeuring vereist.

**A production** (numbers kept 0/0, invented [], gate would reject 0)

- The CortexIQ production bug involving supported_by was caused by the background consolidator selecting hypotheses without the supported_by column for pgx named scanning, while positional hypothesis/counter scans also lacked the supported_by field.
- The CortexIQ supported_by production bug was fixed locally by changing three files, and the walkthrough documentation was updated to reflect the fix.
- After the CortexIQ supported_by bug fix, both go test ./internal/brain and go test ./... were reported as passing (green).
- The CortexIQ supported_by bug fix had not been deployed to production at the time, and deploying it required separate explicit approval.

**B variant** (numbers kept 0/0, invented [], gate would reject 0)

- The CortexIQ supported_by production bug was caused by the background consolidator selecting hypotheses without supported_by for pgx named scanning, and by positional hypothesis/counter scans missing the supported_by field.
- The CortexIQ supported_by production bug was resolved locally by changing three files, and the walkthrough was updated.
- After the CortexIQ supported_by fix, go test ./internal/brain and go test ./... were green.
- The CortexIQ supported_by fix was not deployed at that time; deployment requires separate explicit approval.

## Episode 864

> CortexIQ migratie-race bij opstarten gerepareerd en gepusht (2026-11-11): internal/db/db.go draait de hele schema-opzet (goose.Up, dimensie-ALTER, HNSW-indexen) nu in setupSchema onder withSchemaLock, een sessie-advisory lock (pg_advisory_lock, sleutel 994294737458 = 'cortexiq' in base 36) op een eigen verbinding, wachttijd begrensd op 5 minuten, bij annuleren wordt de sessie gesloten zodat er geen lock achterblijft; Open sluit de pool nu bij elke fout (lekte bij goose-fout). Regressietests internal/db/schemalock_integration_test.go (6 gelijktijdige Open's op een lege database, wederzijdse uitsluiting, annuleerbare wacht); mutatiecheck: zonder lock falen 5 van 6 starts met dezelfde 42P07-fout als in prod. Volledige suite groen op verse PG17-db: 21 pakketten, 153 integratietests, 0 skip. NOG NIET UITGEROLD: productie draait v0.1.74 zonder dit slot; het zit in de volgende release. Open: Bo moet de ssh-allow-regels uit .claude/settings.local.json halen (de uitrol is klaar) en de Jina-sleutel roteren; compose op de VPS wijst nog naar de lokale image cortexiq-postgres:17-bm25 (ghcr-image ...-postgres:0.1.74 bestaat).

**A production** (numbers kept 10/12, invented [], gate would reject 0)

- A startup migration race in CortexIQ was fixed and pushed on 2026-11-11. In internal/db/db.go, the full schema setup (goose.Up, the dimension ALTER and the HNSW indexes) now runs in setupSchema under withSchemaLock. withSchemaLock takes a session advisory lock (pg_advisory_lock, key 994294737458, which is 'cortexiq' in base 36) on its own connection. At that time the wait was capped at 5 minutes, and cancelling closes the session so no lock is left behind.
- The CortexIQ database Open function in internal/db/db.go was changed to close the connection pool on every error. Before the change, the pool leaked when goose failed.
- CortexIQ received regression tests in internal/db/schemalock_integration_test.go. They cover 6 concurrent Open calls on an empty database, mutual exclusion and a cancellable wait. A mutation check showed that without the lock, 5 of 6 starts fail with the same 42P07 error seen in production. The full CortexIQ suite then passed on a fresh PG17 database: 21 packages, 153 integration tests, 0 skipped.
- As of 2026-11-11, CortexIQ production was running v0.1.74, which does not include the schema advisory lock fix. The fix was planned for the next CortexIQ release.
- Two open tasks for Bo remained in the CortexIQ project. Bo still had to remove the ssh allow rules from .claude/settings.local.json, since the rollout was finished. Bo also still had to rotate the Jina key.
- At the time of observation, the CortexIQ compose configuration on the VPS still pointed to the local image cortexiq-postgres:17-bm25. A ghcr image ...-postgres:0.1.74 already existed.

**B variant** (numbers kept 10/12, invented [], gate would reject 0)

- CortexIQ's startup migration race was fixed and pushed on 2026-11-11: internal/db/db.go now runs the entire schema setup (goose.Up, dimension ALTER, HNSW indexes) in setupSchema under withSchemaLock, a session advisory lock (pg_advisory_lock, key 994294737458 = 'cortexiq' in base 36) on its own connection. At that time the wait was capped at 5 minutes, and on cancellation the session is closed so no lock remains behind.
- In CortexIQ, Open in internal/db/db.go now closes the pool on every error; previously the pool leaked on a goose error.
- CortexIQ regression tests in internal/db/schemalock_integration_test.go cover 6 concurrent Open calls on an empty database, mutual exclusion and a cancellable wait. A mutation check showed that without the lock 5 of 6 starts fail with the same 42P07 error as in production. The full CortexIQ test suite was green on a fresh PG17 database: 21 packages, 153 integration tests, 0 skipped.
- As of 2026-11-11, CortexIQ production was running v0.1.74 without the schema advisory lock fix; the fix had not yet been deployed and is planned for the next release.
- Open tasks for CortexIQ as of 2026-11-11: Bo needs to remove the ssh-allow rules from .claude/settings.local.json (the deployment is finished) and rotate the Jina key.
- As of 2026-11-11, the compose configuration on the CortexIQ VPS still pointed to the local image cortexiq-postgres:17-bm25, while the ghcr image ...-postgres:0.1.74 exists.

## Episode 169

> CortexIQ prod (strato-vps, /opt/cortexiq) is op 2026-10-15 uitgerold naar commit 6377b4c: POST /api/v1/remember is live (401 zonder token, 201 met token) en de embedder wordt bij start op de achtergrond warm geladen (~1,5 s, logregel msg="embedder warm"); warme recall meet ~5 ms in plaats van de oude koude start van ~1,8 s. Goose-versie bleef 27.

**A production** (numbers kept 9/9, invented [1.5 1.8], gate would reject 0)

- On 2026-10-15 the CortexIQ production environment on strato-vps (/opt/cortexiq) was rolled out to commit 6377b4c.
- After the 2026-10-15 CortexIQ prod rollout, the POST /api/v1/remember endpoint was live, returning 401 when called without a token and 201 when called with a token.
- In the CortexIQ prod deployment of commit 6377b4c, the embedder was loaded warm in the background at startup, taking about 1.5 s and emitting the log line msg="embedder warm".
- On CortexIQ prod after the 2026-10-15 rollout, warm recall was measured at about 5 ms, compared with the earlier cold start of roughly 1.8 s.
- The Goose migration version of CortexIQ prod was observed to remain at 27 after the 2026-10-15 rollout to commit 6377b4c.

**B variant** (numbers kept 9/9, invented [1.5 1.8], gate would reject 0)

- CortexIQ prod (strato-vps, /opt/cortexiq) was rolled out on 2026-10-15 to commit 6377b4c.
- In CortexIQ prod as of the 2026-10-15 rollout of commit 6377b4c, POST /api/v1/remember was live, returning 401 without a token and 201 with a token.
- In CortexIQ prod as of commit 6377b4c, the embedder was loaded warm in the background at startup, taking ~1.5 s and logging msg="embedder warm".
- CortexIQ prod warm recall was measured at ~5 ms after the 2026-10-15 rollout of commit 6377b4c, instead of the old cold start of ~1.8 s.
- The Goose version of CortexIQ prod remained 27 after the 2026-10-15 rollout to commit 6377b4c.

## Episode 697

> CortexIQ: RRF-sweep gedraaid op de productie-embedder (bge-m3, hybride, Jina-reranker; 60 memories, 36 vragen, k=10) via docker compose run met een override naar een wegwerpdatabase cortexiq_eval (productie niet geraakt, daarna verwijderd). Uitkomst overall: rrf-k 20 hit@1 0.75 MRR 0.850; rrf-k 60 hit@1 0.78 MRR 0.868; rrf-k 100 identiek aan 60. Verschil is één à twee vragen van 36, dus ruis. Besluit: CORTEXIQ_RRF_K blijft 60. Alle varianten missen dezelfde acht vragen op ongeveer dezelfde plaats, dus een betere recall zit in embedder/reranker, niet in de fusieconstante. Recept voor een volgende keer: op de server in /opt/cortexiq een override met CORTEXIQ_POSTGRES_DSN naar een extra database, dan docker compose -f docker-compose.prod.yml -f override run --rm --no-deps -T -v /opt/cortexiq/eval:/eval:ro cortexiq eval recall --rrf-k ... --corpus /eval/recall_corpus.jsonl --queries /eval/recall_queries.jsonl. Les: gebruik bij .env-greps nooit een patroon dat KEY-variabelen raakt (RERANK matchte CORTEXIQ_RERANKER_API_KEY en de Jina-sleutel kwam in het gesprek; Bo is aangeraden hem te roteren).

**A production** (numbers kept 11/11, invented [], gate would reject 0)

- A CortexIQ RRF sweep was run on the production embedder setup (bge-m3, hybrid search, Jina reranker; 60 memories, 36 questions, k=10). It measured hit@1 0.75 / MRR 0.850 at rrf-k 20 and hit@1 0.78 / MRR 0.868 at rrf-k 60, with rrf-k 100 identical to 60. The differences amount to one or two of 36 questions and were judged to be noise.
- After the RRF sweep, CortexIQ's CORTEXIQ_RRF_K setting was kept at 60, because the sweep showed no meaningful difference between rrf-k values 20, 60 and 100.
- In the CortexIQ RRF sweep, all rrf-k variants missed the same eight questions at roughly the same rank positions. The conclusion was that better recall for CortexIQ depends on the embedder or reranker rather than on the RRF fusion constant.
- The CortexIQ recall evaluation is run on the server in /opt/cortexiq. A compose override points CORTEXIQ_POSTGRES_DSN at a separate throwaway database (cortexiq_eval was used, then dropped, leaving production untouched). The command is: docker compose -f docker-compose.prod.yml -f override run --rm --no-deps -T -v /opt/cortexiq/eval:/eval:ro cortexiq eval recall --rrf-k ... --corpus /eval/recall_corpus.jsonl --queries /eval/recall_queries.jsonl.
- Lesson for CortexIQ: never grep the .env file with a pattern that can match KEY variables. Grepping for RERANK matched CORTEXIQ_RERANKER_API_KEY and exposed the Jina API key in the conversation, and Bo was advised to rotate that key.

**B variant** (numbers kept 11/11, invented [], gate would reject 0)

- A CortexIQ RRF sweep was run on the production embedder setup (bge-m3, hybrid, Jina-reranker; 60 memories, 36 questions, k=10) via docker compose run with an override pointing to a throwaway database cortexiq_eval, which was deleted afterwards; production was not touched. Overall results: rrf-k 20 gave hit@1 0.75 and MRR 0.850, rrf-k 60 gave hit@1 0.78 and MRR 0.868, and rrf-k 100 was identical to rrf-k 60. The difference was one or two questions out of 36, so noise.
- After the RRF sweep, the decision for CortexIQ was that CORTEXIQ_RRF_K stays at 60.
- In the CortexIQ RRF sweep, all rrf-k variants missed the same eight questions at roughly the same position, so better recall for CortexIQ lies in the embedder/reranker, not in the fusion constant.
- Recipe for a future CortexIQ recall eval: on the server in /opt/cortexiq, create an override with CORTEXIQ_POSTGRES_DSN pointing to an extra database, then run docker compose -f docker-compose.prod.yml -f override run --rm --no-deps -T -v /opt/cortexiq/eval:/eval:ro cortexiq eval recall --rrf-k ... --corpus /eval/recall_corpus.jsonl --queries /eval/recall_queries.jsonl.
- Lesson for CortexIQ: when grepping the .env file, never use a pattern that matches KEY variables. The pattern RERANK matched CORTEXIQ_RERANKER_API_KEY and the Jina key ended up in the conversation; Bo was advised to rotate the Jina key.

## Episode 888

> CortexIQ: v0.1.80 (Timeline-vergelijking tussen twee data, commit 6f725cd, CI groen) is door Claude gepubliceerd en de Release-workflow is groen (2026-11-13). De uitrol naar productie is NIET uitgevoerd: de auto-mode classifier blokkeerde het ssh-commando voor de deploy. Productie draait vermoedelijk nog v0.1.79 (niet geverifieerd). Bo moet de deploy toestaan of zelf draaien. Let op: vaste toestemming van Bo gold voor het publiceren van releases; uitrol via ssh valt buiten wat de classifier toestaat zonder allow-regel in .claude/settings.local.json.

**A production** (numbers kept 4/4, invented [], gate would reject 0)

- CortexIQ v0.1.80, which adds a Timeline comparison between two dates (commit 6f725cd, CI green), was published by Claude on 2026-11-13 and the Release workflow passed.
- The production rollout of CortexIQ v0.1.80 was not carried out on 2026-11-13 because the auto-mode classifier blocked the ssh command used for the deploy; Bo has to permit the deploy or run it personally.
- At the time of the CortexIQ v0.1.80 release on 2026-11-13, CortexIQ production was presumed to still be running v0.1.79, though this was not verified.
- For CortexIQ, Bo's standing permission covered publishing releases, but deploying via ssh fell outside what the auto-mode classifier allowed without an allow rule in .claude/settings.local.json.

**B variant** (numbers kept 4/4, invented [], gate would reject 0)

- CortexIQ v0.1.80 (Timeline-vergelijking tussen twee data, commit 6f725cd, CI groen) was published by Claude and the Release-workflow was green on 2026-11-13.
- The deploy of CortexIQ v0.1.80 to production was not carried out on 2026-11-13 because the auto-mode classifier blocked the ssh command for the deploy; Bo must allow the deploy or run it himself.
- At the time of the v0.1.80 release on 2026-11-13, CortexIQ production was presumably still running v0.1.79, which was not verified.
- For CortexIQ, Bo's standing permission covered publishing releases, but deployment via ssh falls outside what the auto-mode classifier allows without an allow rule in .claude/settings.local.json.

## Episode 412

> CortexIQ-productie (strato-vps, /opt/cortexiq) draait sinds 2026-10-27 v0.1.6 (commit c107f8c, image sha256:61d7ed372332). Deployprocedure: GitHub-release publiceren (release-workflow bouwt binaries en image), CORTEXIQ_IMAGE_TAG in /opt/cortexiq/.env bumpen, daarna docker compose -f docker-compose.prod.yml pull en up -d. De UI is een aparte service (cortexiq-ui) op dezelfde image; UI luistert op 127.0.0.1:18101, API op 18100, beide eisen een token.

**A production** (numbers kept 6/6, invented [], gate would reject 0)

- Since 2026-10-27, CortexIQ production on strato-vps under /opt/cortexiq was observed running v0.1.6, built from commit c107f8c with image sha256:61d7ed372332.
- CortexIQ production deployment works by publishing a GitHub release (whose release workflow builds the binaries and the image), bumping CORTEXIQ_IMAGE_TAG in /opt/cortexiq/.env, and then running docker compose -f docker-compose.prod.yml pull followed by up -d.
- The CortexIQ UI runs as its own service named cortexiq-ui, using the same image as the CortexIQ API.
- In CortexIQ production the UI was observed listening on 127.0.0.1:18101 and the API on port 18100, with both requiring a token.

**B variant** (numbers kept 6/6, invented [], gate would reject 0)

- CortexIQ-productie (strato-vps, /opt/cortexiq) was observed running v0.1.6 (commit c107f8c, image sha256:61d7ed372332) since 2026-10-27.
- The CortexIQ deploy procedure is: publish a GitHub release (the release workflow builds binaries and image), bump CORTEXIQ_IMAGE_TAG in /opt/cortexiq/.env, then run docker compose -f docker-compose.prod.yml pull and up -d.
- The CortexIQ UI is a separate service (cortexiq-ui) running on the same image as the CortexIQ API in production.
- In CortexIQ production, the cortexiq-ui was observed listening on 127.0.0.1:18101 and the CortexIQ API on port 18100, with both requiring a token.

## Episode 886

> CortexIQ werkwijze (2026-11-13): Bo geeft vaste toestemming dat Claude zelf GitHub-releases publiceert (gh release create <versie> --target main) zodra CI op main groen is; Bo hoeft het niet meer zelf te doen. Daarbij altijd versienummer en releasenotities melden in het antwoord. Uitrol naar productie volgt de vaste procedure (back-up .env, pull terwijl het draait, verified dump, tag bumpen, healthz/readyz/restarts/logs controleren). Eerder deed Bo het publiceren zelf vanaf v0.1.74; die gewoonte is nu vervangen.

**A production** (numbers kept 2/2, invented [], gate would reject 0)

- As of 2026-11-13, Bo gave Claude standing permission in the CortexIQ workflow to publish GitHub releases itself with 'gh release create <versie> --target main' as soon as CI on the main branch is green, so Bo no longer has to publish them.
- In the CortexIQ workflow, when Claude publishes a GitHub release it must always state the version number and the release notes in its response.
- CortexIQ rollout to production follows a fixed procedure: back up the .env file, pull while the service keeps running, make a verified dump, bump the tag, then check the healthz and readyz endpoints, restarts and logs.
- Before 2026-11-13, Bo published CortexIQ GitHub releases manually, a practice in place from v0.1.74 onward; that practice was replaced by Claude publishing the releases.

**B variant** (numbers kept 2/2, invented [], gate would reject 3)

- Voor CortexIQ geeft Bo vaste toestemming dat Claude zelf GitHub-releases publiceert (gh release create <versie> --target main) zodra CI op main groen is; Bo hoeft het niet meer zelf te doen (vastgelegd 2026-11-13).
- Bij het publiceren van een CortexIQ GitHub-release door Claude moeten altijd het versienummer en de releasenotities in het antwoord gemeld worden.
- Uitrol van CortexIQ naar productie volgt de vaste procedure: back-up .env, pull terwijl het draait, verified dump, tag bumpen, en healthz/readyz/restarts/logs controleren.
- Eerder publiceerde Bo de CortexIQ GitHub-releases zelf vanaf v0.1.74; die gewoonte is op 2026-11-13 vervangen doordat Claude de releases publiceert.

## Episode 670

> CortexIQ: v0.1.20 (e56a03b) is op 2026-11-05 gedeployed naar strato-vps; v0.1.19 is wel gebouwd maar nooit uitgerold, productie ging direct van 0.1.18 naar 0.1.20. Bevat migraties 00044 (judged), 00045 (origin/origin_ref) en 00046 (strength), de rebuild-CLI en CORTEXIQ_RRF_K. Volgorde: geverifieerde dump backups/cortexiq-20261105T002405Z.dump, release, .env gebackupt als .env.bak-pre0.1.20, tag-bump en up -d. Gecontroleerd: version v0.1.20, goose 46, containers Up, geen panic of migratiefout, 2330 facts allemaal zonder origin (oude facts, bewust niet teruggevuld). Een read-only rebuild-rapport van /projects/cortexiq in productie: 114 episodes, 476 facts, 6 facts op ingetrokken bronnen (gat C). CORTEXIQ_DECAY_SPLIT staat NIET aan en docker-compose.prod.yml op de server geeft de variabele nog niet door. Open voor Bo: de ssh- en gh-release-allow-regels uit de projectsettings halen; beslissen over DECAY_SPLIT; zelf de RRF-sweep draaien (cortexiq eval recall --rrf-k 20,60,100).

**A production** (numbers kept 17/17, invented [], gate would reject 0)

- CortexIQ v0.1.20 (commit e56a03b) was deployed to strato-vps on 2026-11-05; v0.1.19 was built but never rolled out, so production jumped directly from 0.1.18 to 0.1.20.
- The CortexIQ v0.1.20 release includes database migrations 00044 (judged), 00045 (origin/origin_ref) and 00046 (strength), plus the rebuild-CLI and the CORTEXIQ_RRF_K setting.
- The CortexIQ v0.1.20 deployment followed this sequence: a verified database dump at backups/cortexiq-20261105T002405Z.dump, the release, a backup of .env as .env.bak-pre0.1.20, a tag-bump, and then up -d.
- After the v0.1.20 deployment, CortexIQ production was checked and showed version v0.1.20, goose migration level 46, containers Up, and no panic or migration error. All 2330 facts had no origin at that time because older facts were deliberately not backfilled.
- A read-only rebuild report of /projects/cortexiq in CortexIQ production counted 114 episodes and 476 facts, with 6 facts resting on revoked sources (gap C).
- At the v0.1.20 deployment, CORTEXIQ_DECAY_SPLIT was not enabled in CortexIQ production, and docker-compose.prod.yml on the server did not yet pass the variable through. Open tasks for Bo were to remove the ssh and gh-release allow rules from the project settings, decide on DECAY_SPLIT, and run the RRF sweep with 'cortexiq eval recall --rrf-k 20,60,100'.

**B variant** (numbers kept 17/17, invented [], gate would reject 4)

- CortexIQ v0.1.20 (e56a03b) is op 2026-11-05 gedeployed naar strato-vps. v0.1.19 is wel gebouwd maar nooit uitgerold, dus productie ging direct van 0.1.18 naar 0.1.20. De deploy-volgorde was: geverifieerde dump backups/cortexiq-20261105T002405Z.dump, release, .env gebackupt als .env.bak-pre0.1.20, tag-bump en up -d.
- CortexIQ v0.1.20 bevat migraties 00044 (judged), 00045 (origin/origin_ref) en 00046 (strength), de rebuild-CLI en CORTEXIQ_RRF_K.
- Na de deploy van CortexIQ v0.1.20 op strato-vps werd gecontroleerd: version v0.1.20, goose 46, containers Up, geen panic of migratiefout. Toen waren 2330 facts allemaal zonder origin; dit zijn oude facts die bewust niet zijn teruggevuld.
- Een read-only CortexIQ rebuild-rapport van /projects/cortexiq in productie gaf 114 episodes, 476 facts en 6 facts op ingetrokken bronnen (gat C).
- Bij de CortexIQ v0.1.20 deploy stond CORTEXIQ_DECAY_SPLIT niet aan in productie, en docker-compose.prod.yml op de server gaf de variabele toen nog niet door.
- Open acties voor Bo bij CortexIQ: de ssh- en gh-release-allow-regels uit de projectsettings halen, beslissen over CORTEXIQ_DECAY_SPLIT, en zelf de RRF-sweep draaien met 'cortexiq eval recall --rrf-k 20,60,100'.

## Episode 924

> CortexIQ besluit (2026-11-13, Bo): Jina (jina-reranker-v3.5) blijft de reranker in productie; geen eigen reranker op de VPS. Reden: lokaal (llama.cpp, bge-reranker-v2-m3) is ongeveer 2,9 s trager per recall (mogelijk 3-5 s op de VPS, gedeeld met andere workloads) en iets slechter (MRR 0,80 tegen 0,85; zie id 923). Niets gewijzigd in productie. Het risico dat Jina wegvalt (saldo, 429-limiet van 100.000 tokens per minuut, 404) wordt afgedekt door de metric recall_rerank_requests_total, de logregel per minuut, RERANK_CANDIDATES=30 en de graaf-routing als vangnet (gewogen is dan beter dan uit). Niet gedaan en niet nodig tenzij het pijn doet: fallback-keten Jina dan lokaal (codewijziging), meten met 10 kandidaten, meten op de VPS zelf.

**A production** (numbers kept 13/15, invented [2.9 0.80 0.85], gate would reject 0)

- On 2026-11-13 Bo decided that CortexIQ keeps Jina (jina-reranker-v3.5) as its production reranker and does not run its own reranker on the VPS; nothing was changed in production as a result of this decision.
- For CortexIQ, a local reranker (llama.cpp with bge-reranker-v2-m3) was measured at roughly 2.9 s slower per recall than Jina, possibly 3-5 s on the VPS shared with other workloads, and slightly worse in quality (MRR 0.80 versus 0.85 for Jina, see id 923), which was the stated reason for keeping Jina.
- The risk of the Jina reranker becoming unavailable for CortexIQ (exhausted balance, 429 rate limit of 100,000 tokens per minute, 404 errors) was judged covered by the recall_rerank_requests_total metric, a per-minute log line, RERANK_CANDIDATES set to 30 at the time, and graph routing as a safety net, since weighted results are better than none.
- For the CortexIQ reranker, a fallback chain from Jina to a local reranker (requiring a code change), measurements with 10 candidates, and measurements on the VPS itself were not done and were considered unnecessary unless problems arise.

**B variant** (numbers kept 13/15, invented [2.9 0.80 0.85], gate would reject 0)

- On 2026-11-13 Bo decided that Jina (jina-reranker-v3.5) remains the reranker in production for CortexIQ and that no own reranker runs on the VPS; nothing was changed in production.
- For CortexIQ, a local reranker (llama.cpp with bge-reranker-v2-m3) was measured as about 2.9 s slower per recall than Jina (possibly 3-5 s on the VPS, which is shared with other workloads) and slightly worse (MRR 0.80 versus 0.85; see id 923). These results were the stated reason for keeping Jina in production.
- The CortexIQ risk of Jina dropping out (balance, 429 limit of 100,000 tokens per minute, 404) was covered by the metric recall_rerank_requests_total, a log line per minute, RERANK_CANDIDATES configured at 30 at that time, and graph routing as a fallback (weighted results being better than no reranking).
- For the CortexIQ reranker, a fallback chain from Jina to a local reranker (requiring a code change), measuring with 10 candidates, and measuring on the VPS itself were not done and were judged not needed unless problems arise.

## Episode 664

> CortexIQ: v0.1.18 (afgewezen-waarde-tombstone, migratie 00043, Reflect-tab #7) is op 2026-11-05 gedeployed naar strato-vps. Volgorde: geverifieerde dump (backups/cortexiq-20261104T232239Z.dump), release gepubliceerd door Bo (de auto-mode classifier weigerde gh release create van mij), .env gebackupt als .env.bak-pre0.1.18, tag-bump en up -d. Gecontroleerd: version v0.1.18 (6c89dd3), goose 43, facts_refused in de logs. Productie had 0 retracties, dus de backfill deed niets. Het Gemma-parsefoutje bij causal links in /projects/chronicle is het bekende lijstprobleem, niet nieuw. Praktisch: SSH naar strato-vps werkt via een allow-regel in de projectsettings; die hoort na de deploy weg.

**A production** (numbers kept 8/8, invented [], gate would reject 0)

- CortexIQ v0.1.18, which contains the rejected-value tombstone (afgewezen-waarde-tombstone), migration 00043 and Reflect-tab #7, was deployed to strato-vps on 2026-11-05. Verification after the deploy showed version v0.1.18 (commit 6c89dd3), goose migration level 43, and facts_refused entries in the logs.
- The CortexIQ v0.1.18 deployment to strato-vps followed this order: a verified database dump (backups/cortexiq-20261104T232239Z.dump), publication of the release, a backup of .env as .env.bak-pre0.1.18, a tag bump, and then up -d.
- Bo published the CortexIQ v0.1.18 release manually because the auto-mode classifier refused to run gh release create during the deployment.
- During the CortexIQ v0.1.18 deployment, the backfill changed nothing in production because production had 0 retractions at that time.
- The Gemma parse error for causal links seen in /projects/chronicle after the CortexIQ v0.1.18 deployment is the already-known list problem and not a new regression.
- At the time of the CortexIQ v0.1.18 deployment, SSH access to strato-vps worked through an allow rule in the project settings. That rule is meant to be removed after the deploy.

**B variant** (numbers kept 8/8, invented [0.1.18-deploy], gate would reject 4)

- CortexIQ v0.1.18 (afgewezen-waarde-tombstone, migratie 00043, Reflect-tab #7) is op 2026-11-05 gedeployed naar strato-vps.
- De deploy van CortexIQ v0.1.18 naar strato-vps volgde deze volgorde: geverifieerde dump (backups/cortexiq-20261104T232239Z.dump), release gepubliceerd door Bo (de auto-mode classifier weigerde gh release create), .env gebackupt als .env.bak-pre0.1.18, daarna tag-bump en up -d.
- Na de deploy van CortexIQ v0.1.18 op strato-vps werd gecontroleerd: version v0.1.18 (6c89dd3), goose-migratieversie 43, en facts_refused verscheen in de logs.
- CortexIQ productie had bij de deploy van v0.1.18 0 retracties, dus de backfill van migratie 00043 deed niets.
- Het Gemma-parsefoutje bij causal links in /projects/chronicle van CortexIQ is het bekende lijstprobleem, niet nieuw na de v0.1.18-deploy.
- SSH naar strato-vps voor de CortexIQ-deploy werkte via een allow-regel in de projectsettings; die allow-regel hoort na de deploy verwijderd te worden.

---

episodes 21 | A: 106 facts, numbers kept 156/171, invented 6 | B: 106 facts, numbers kept 157/171, invented 7, would be rejected by the production gate 24
