# Fact fidelity: anthropic/claude-opus-5-5

A = production prompt and gate. B = same prompt, no gate. Read the facts against the episode: subject and object right? detail kept? anything added?

## Episode 989

> Spike 16-11-2026: CortexIQ 0.1.88 (strato-vps, REST 127.0.0.1:18101) vergeleken met Hindsight 0.9.2 (contabo, 8888) als vervanger van Hindsight in Gaia. Beslissend verschil: CortexIQ filtert op occurred_at met from/to (half-open) en herkent Nederlandse datumzinnen over REST, terwijl Hindsight temporal_window alleen rankt (een venster van 1 dag gaf 54 hits over vijf dagen) en memories/list geen datumfilter heeft. Scoping moet met namespaces (array in REST); de enkelvoudige namespace is alleen een ranking-zetje. remember bewaart content, namespace, tags, occurred_at, external_key, external_refs en importance, maar gooit vrije metadata weg; external_key maakt de write idempotent (unchanged:true, andere inhoud geweigerd tenzij supersedes), dus gaia:kairos:<episode_id> is de spiegel-sleutel. DELETE /api/v1/episodes/:id purge hard en cascadeert naar afgeleide facts; facts en namespaces zijn niet los te verwijderen via REST (404/405). Consolidatie reageert binnen twee minuten maar levert even dunne facts als Hindsight. Op twintig Nederlandse vragen: CortexIQ mediaan 1.061 ms met ruwe episodes bovenaan, Hindsight 1.372 ms met een door emotionele gaia-herinneringen verzadigde top. Rapport: docs/spike-cortexiq.md (commits 85dd48e en f329e24). Volgorde blijft: MemoryStore-poort, terugkijk-route op CortexIQ, gaia-logos rebuilden, bojan/gaia importeren, Hindsight uit.

**A production** (numbers kept 13/13, invented [1-day], gate would reject 0)

- In a spike on 16-11-2026, CortexIQ 0.1.88 (strato-vps, REST 127.0.0.1:18101) was compared with Hindsight 0.9.2 (contabo, port 8888) as a replacement for Hindsight in Gaia AI-assistent. The report is docs/spike-cortexiq.md (commits 85dd48e and f329e24). The decisive difference observed was that CortexIQ filters on occurred_at with from/to (half-open) and recognizes Dutch date phrases over REST. Hindsight temporal_window only ranks (a 1-day window gave 54 hits spread over five days), and Hindsight memories/list has no date filter.
- In the Gaia AI-assistent spike, CortexIQ 0.1.88 scoping had to be done with namespaces (an array in REST); the singular namespace parameter was observed to act only as a ranking nudge.
- In the Gaia AI-assistent spike, CortexIQ 0.1.88 remember was observed to store content, namespace, tags, occurred_at, external_key, external_refs and importance, but to discard free metadata. external_key makes the write idempotent: a repeated write returns unchanged:true, and different content is refused unless supersedes is given. gaia:kairos:<episode_id> is therefore the mirror key for Gaia.
- In the Gaia AI-assistent spike, DELETE /api/v1/episodes/:id in CortexIQ 0.1.88 was observed to hard purge the episode and cascade to its derived facts. Facts and namespaces could not be deleted separately via REST (404/405).
- In the Gaia AI-assistent spike, CortexIQ 0.1.88 consolidation reacted within two minutes but produced facts as thin as Hindsight's. On twenty Dutch questions, CortexIQ had a median of 1.061 ms with raw episodes at the top of the results. Hindsight 0.9.2 had a median of 1.372 ms, with a top saturated by emotional gaia memories.
- The planned migration order for Gaia AI-assistent from Hindsight to CortexIQ remains: MemoryStore-poort, terugkijk-route op CortexIQ, rebuild gaia-logos, import bojan/gaia, then switch Hindsight off.

**B variant** (numbers kept 13/13, invented [1-day], gate would reject 0)

- In the Gaia AI-assistent spike of 16-11-2026, CortexIQ 0.1.88 (strato-vps, REST 127.0.0.1:18101) was compared with Hindsight 0.9.2 (contabo, 8888) as a replacement for Hindsight in Gaia. The decisive difference was that CortexIQ filters on occurred_at with from/to (half-open) and recognizes Dutch date phrases over REST. Hindsight temporal_window only ranks (a 1-day window gave 54 hits over five days), and Hindsight memories/list has no date filter.
- In the Gaia AI-assistent spike, CortexIQ 0.1.88 scoping had to be done with namespaces (an array in REST). The singular namespace was only a ranking nudge.
- In the Gaia AI-assistent spike, CortexIQ 0.1.88 remember stored content, namespace, tags, occurred_at, external_key, external_refs and importance, but discarded free metadata. external_key made the write idempotent: identical content returned unchanged:true, and different content was refused unless supersedes was given. gaia:kairos:<episode_id> was chosen as the mirror key.
- In the Gaia AI-assistent spike, CortexIQ 0.1.88 DELETE /api/v1/episodes/:id did a hard purge and cascaded to derived facts. Facts and namespaces could not be deleted separately via REST, which returned 404/405.
- In the Gaia AI-assistent spike, CortexIQ 0.1.88 consolidation reacted within two minutes but produced facts as thin as Hindsight's. On twenty Dutch questions, CortexIQ had a median of 1.061 ms with raw episodes at the top. Hindsight 0.9.2 had a median of 1.372 ms with a top saturated by emotional gaia memories.
- The CortexIQ spike report for Gaia AI-assistent is docs/spike-cortexiq.md (commits 85dd48e and f329e24). The planned order for Gaia remained: MemoryStore-poort, terugkijk-route on CortexIQ, rebuild gaia-logos, import bojan/gaia, then switch Hindsight off.

## Episode 1082

> Migratieplan Hindsight→CortexIQ in Gaia-Cloud vastgesteld op 18-11-2026 (plansessie): volgorde zoals de spike van 16-11 bepaalde (MemoryStore-poort → terugkijk-route → gaia-logos rebuild → bojan/gaia import → Hindsight uit). Twee beslissingen van Bo, beide 'meten gaat voor bouwen': (1) CortexIQ-fase 1 (quota recall over namespaces) wordt pas gebouwd nadat een fixture op de live strato-instance aantoonbaar maakt dat één luide namespace de rest verdringt — geen bewijs, geen bouw; (2) hosting van CortexIQ voor Gaia (strato via Tailscale vs eigen instance op contabo) wordt pas gekozen na een latency-meting contabo→strato, uit te voeren nadat de REST aan de tailnet-kant is gebonden (nu 127.0.0.1:18101). Fase 6 (agent_identity) voorlopig geschrapt: namespace + tag dekt de epistemic mark. Fase 0-huiskeeping: episode 980 (Mindstack, Vite-migratie) opnieuw aanleveren; oude hindsight-backups van 03-05 dienen alleen als formaat-referentie bij de verse export in fase 4.

**A production** (numbers kept 9/10, invented [], gate would reject 0)

- The Gaia-Cloud migration plan from Hindsight to CortexIQ was set on 18-11-2026 in a planning session, following the order determined by the spike of 16-11: MemoryStore-poort → terugkijk-route → gaia-logos rebuild → bojan/gaia import → Hindsight uit.
- Bo decided ('meten gaat voor bouwen') that CortexIQ fase 1 (quota recall over namespaces) for the Gaia-Cloud migration will only be built after a fixture on the live strato-instance demonstrably shows that one loud namespace crowds out the rest — no proof, no build.
- Bo decided ('meten gaat voor bouwen') that hosting of CortexIQ for Gaia (strato via Tailscale vs. an own instance on contabo) will only be chosen after a latency measurement contabo→strato, to be done after the CortexIQ REST is bound to the tailnet side; on 18-11-2026 the REST was bound to 127.0.0.1:18101.
- Fase 6 (agent_identity) of the Gaia-Cloud Hindsight→CortexIQ migration plan was provisionally scrapped because namespace + tag covers the epistemic mark.
- As fase 0 housekeeping of the Gaia-Cloud Hindsight→CortexIQ migration, episode 980 (Mindstack, Vite-migratie) is to be resubmitted.
- In the Gaia-Cloud Hindsight→CortexIQ migration plan, the old Hindsight backups of 03-05 serve only as a format reference for the fresh export in fase 4.

**B variant** (numbers kept 10/10, invented [], gate would reject 0)

- Het migratieplan Hindsight→CortexIQ in Gaia-Cloud is op 18-11-2026 in een plansessie vastgesteld met de volgorde die de spike van 16-11 bepaalde: MemoryStore-poort → terugkijk-route → gaia-logos rebuild → bojan/gaia import → Hindsight uit.
- Bo besliste in het migratieplan Hindsight→CortexIQ voor Gaia ('meten gaat voor bouwen') dat CortexIQ-fase 1 (quota recall over namespaces) pas wordt gebouwd nadat een fixture op de live strato-instance aantoonbaar maakt dat één luide namespace de rest verdringt — geen bewijs, geen bouw.
- Bo besliste ('meten gaat voor bouwen') dat de hosting van CortexIQ voor Gaia (strato via Tailscale vs eigen instance op contabo) pas wordt gekozen na een latency-meting contabo→strato, uit te voeren nadat de CortexIQ REST aan de tailnet-kant is gebonden; op 18-11-2026 was de REST gebonden aan 127.0.0.1:18101.
- Fase 6 (agent_identity) van het migratieplan Hindsight→CortexIQ in Gaia-Cloud is voorlopig geschrapt, omdat namespace + tag de epistemic mark dekt.
- Als fase 0-huiskeeping van het migratieplan Hindsight→CortexIQ in Gaia-Cloud moet episode 980 (Mindstack, Vite-migratie) opnieuw worden aangeleverd.
- In het migratieplan Hindsight→CortexIQ in Gaia-Cloud dienen de oude hindsight-backups van 03-05 alleen als formaat-referentie bij de verse export in fase 4.

## Episode 558

> Gap-analyse CortexIQ vs Hindsight voor Gaia-Cloud (2026-11): Gaia-Cloud hangt via src/hindsightClient.js aan Hindsight voor: (1) banks + cross-bank recall met per-bank quota (scores niet op één schaal), (2) willekeurige tags per memory-item die in recall-output terugkomen (gaia:* tags, memoryKindOf), (3) versiegelde invalidatie/superseding sync vanuit cognitionSync met reconcile, (4) provenance sources:['foundation:<id>'] voor Dual Recall, (5) async fire-and-forget retain, (6) agent_identity-enforcing. CortexIQ heeft inmiddels wél reflect, mental models, reranker, graph-arm, temporal recall en MMR — die eerdere gaten zijn gedicht.

**A production** (numbers kept 4/7, invented [], gate would reject 0)

- In the 2026-11 gap analysis of CortexIQ vs Hindsight, Gaia-Cloud depends on Hindsight via src/hindsightClient.js for banks and cross-bank recall with per-bank quota, because the scores are not on one scale.
- In the 2026-11 gap analysis of CortexIQ vs Hindsight, Gaia-Cloud depends on Hindsight via src/hindsightClient.js for arbitrary tags per memory item that come back in recall output (gaia:* tags, memoryKindOf).
- In the 2026-11 gap analysis of CortexIQ vs Hindsight, Gaia-Cloud depends on Hindsight via src/hindsightClient.js for versioned invalidation/superseding sync from cognitionSync with reconcile.
- In the 2026-11 gap analysis of CortexIQ vs Hindsight, Gaia-Cloud depends on Hindsight via src/hindsightClient.js for provenance sources:['foundation:<id>'] used for Dual Recall.
- In the 2026-11 gap analysis of CortexIQ vs Hindsight, Gaia-Cloud depends on Hindsight via src/hindsightClient.js for async fire-and-forget retain and for agent_identity-enforcing.
- According to the 2026-11 gap analysis for Gaia-Cloud, CortexIQ by then had reflect, mental models, reranker, graph-arm, temporal recall and MMR, closing those earlier gaps compared to Hindsight.

**B variant** (numbers kept 4/7, invented [], gate would reject 0)

- In the gap analysis of CortexIQ vs Hindsight for Gaia-Cloud (2026-11), Gaia-Cloud depends via src/hindsightClient.js on Hindsight for banks and cross-bank recall with a per-bank quota, because the scores are not on one scale.
- In the gap analysis of CortexIQ vs Hindsight for Gaia-Cloud (2026-11), Gaia-Cloud depends via src/hindsightClient.js on Hindsight for arbitrary tags per memory-item that come back in recall output (gaia:* tags, memoryKindOf).
- In the gap analysis of CortexIQ vs Hindsight for Gaia-Cloud (2026-11), Gaia-Cloud depends via src/hindsightClient.js on Hindsight for versioned invalidation/superseding sync from cognitionSync with reconcile.
- In the gap analysis of CortexIQ vs Hindsight for Gaia-Cloud (2026-11), Gaia-Cloud depends via src/hindsightClient.js on Hindsight for provenance sources:['foundation:<id>'] used by Dual Recall.
- In the gap analysis of CortexIQ vs Hindsight for Gaia-Cloud (2026-11), Gaia-Cloud depends via src/hindsightClient.js on Hindsight for async fire-and-forget retain and for agent_identity enforcement.
- According to the gap analysis of CortexIQ vs Hindsight for Gaia-Cloud (2026-11), CortexIQ now has reflect, mental models, reranker, graph-arm, temporal recall and MMR, closing those earlier gaps.

## Episode 36

> Beslissing 2026-10-08 (akkoord Bo): de provenance-prefix voor afgeleide records in Gaia-Cloud gaat van 'chronicle:' naar 'foundation:'. Canonieke id-vorm: 'foundation:<uuid>' met de kale ingest_object uuid (wat Foundation /api/ingest-logs/:id verwacht), niet Foundation's episode-vorm 'ingest:<uuid>'. Reden: 'chronicle:' is legacy (Foundation, niet Chronicle, is de rauwe laag) en is een routing key, geen label. Wordt in één migratie gedaan die ook de strip-bug in kairosRoutes.js fixt en de drie sources-kolommen (hypotheses/patterns/kairos_episodes) plus Hindsight-metadata backfillt; reconcile() wordt uitgebreid met Kairos-episodes. Plan: C:\Users\bojan\.opencode\plan\chronicle-prefix-provenance-plan.md.

**A production** (numbers kept 1/1, invented [], gate would reject 0)

- On 2026-10-08, with Bo's approval, Gaia-Cloud decided to change the provenance prefix on derived records from 'chronicle:' to 'foundation:'. The stated reason is that 'chronicle:' is a legacy prefix, since Foundation rather than Chronicle is the raw layer, and that the prefix serves as a routing key rather than a label.
- The canonical Gaia-Cloud provenance id was decided as 'foundation:<uuid>', where <uuid> is the bare ingest_object uuid that Foundation /api/ingest-logs/:id expects. Foundation's episode-style 'ingest:<uuid>' form was not chosen.
- The Gaia-Cloud switch from 'chronicle:' to 'foundation:' is planned as a single migration. That migration also fixes the strip-bug in kairosRoutes.js and backfills both the sources columns of hypotheses, patterns and kairos_episodes and the Hindsight metadata.
- As part of the Gaia-Cloud 'foundation:' provenance migration, the reconcile() function is planned to be extended to cover Kairos-episodes.
- The Gaia-Cloud plan for migrating the provenance prefix from 'chronicle:' to 'foundation:' is documented at C:\Users\bojan\.opencode\plan\chronicle-prefix-provenance-plan.md.

**B variant** (numbers kept 1/1, invented [], gate would reject 3)

- Beslissing 2026-10-08 (akkoord Bo): de provenance-prefix voor afgeleide records in Gaia-Cloud gaat van 'chronicle:' naar 'foundation:'.
- De canonieke provenance-id-vorm in Gaia-Cloud is 'foundation:<uuid>' met de kale ingest_object uuid (wat Foundation /api/ingest-logs/:id verwacht), niet Foundation's episode-vorm 'ingest:<uuid>'.
- De Gaia-Cloud provenance-prefix 'chronicle:' wordt vervangen omdat 'chronicle:' legacy is (Foundation, niet Chronicle, is de rauwe laag) en een routing key is, geen label.
- De Gaia-Cloud prefix-wijziging van 'chronicle:' naar 'foundation:' wordt in één migratie gedaan die ook de strip-bug in kairosRoutes.js fixt en de drie sources-kolommen (hypotheses/patterns/kairos_episodes) plus Hindsight-metadata backfillt.
- Bij de Gaia-Cloud provenance-migratie naar 'foundation:' wordt reconcile() uitgebreid met Kairos-episodes.
- Het plan voor de Gaia-Cloud provenance-prefix-migratie van 'chronicle:' naar 'foundation:' staat in C:\Users\bojan\.opencode\plan\chronicle-prefix-provenance-plan.md.

## Episode 37

> Gaia-Cloud: de chronicle:->foundation: provenance-normalisatie is op 2026-10-08 geïmplementeerd (nieuwe seam src/foundationRef.js, synthesizer+kairosRoutes erop, cognitionSync.reconcile() pusht nu ook Kairos-episodes, migratie 010_normalize_sources_prefix.sql). Tests groen: gaia-api 1244/1244, cognition 49/49. Nog open: migratie toepassen op de VPS (npm run migrate in services/cognition) en daarna npm run reconcile:hindsight -- --apply --clear om de Hindsight-metadata te verversen.

**A production** (numbers kept 4/4, invented [], gate would reject 0)

- In Gaia-Cloud (Gaia AI-assistent) is de chronicle:->foundation: provenance-normalisatie op 2026-10-08 geïmplementeerd, met een nieuwe seam src/foundationRef.js waar synthesizer en kairosRoutes op zijn aangesloten.
- In Gaia-Cloud pusht cognitionSync.reconcile() sinds de chronicle:->foundation: provenance-normalisatie van 2026-10-08 ook Kairos-episodes.
- Voor de Gaia-Cloud chronicle:->foundation: provenance-normalisatie is de migratie 010_normalize_sources_prefix.sql toegevoegd.
- Na de Gaia-Cloud chronicle:->foundation: provenance-normalisatie op 2026-10-08 waren de tests groen: gaia-api 1244/1244 en cognition 49/49.
- Op 2026-10-08 stond voor Gaia-Cloud nog open: de migratie 010_normalize_sources_prefix.sql toepassen op de VPS met npm run migrate in services/cognition, en daarna npm run reconcile:hindsight -- --apply --clear uitvoeren om de Hindsight-metadata te verversen.

**B variant** (numbers kept 4/4, invented [], gate would reject 0)

- In Gaia-Cloud (Gaia AI-assistent) is de chronicle:->foundation: provenance-normalisatie op 2026-10-08 geïmplementeerd met een nieuwe seam src/foundationRef.js, waar synthesizer en kairosRoutes op aangesloten zijn.
- In Gaia-Cloud pusht cognitionSync.reconcile() sinds de chronicle:->foundation: provenance-normalisatie van 2026-10-08 ook Kairos-episodes.
- De chronicle:->foundation: provenance-normalisatie in Gaia-Cloud omvat de migratie 010_normalize_sources_prefix.sql.
- Na de chronicle:->foundation: provenance-normalisatie op 2026-10-08 waren de Gaia-Cloud-tests groen: gaia-api 1244/1244 en cognition 49/49.
- Voor de Gaia-Cloud chronicle:->foundation: provenance-normalisatie stond nog open: de migratie op de VPS toepassen (npm run migrate in services/cognition) en daarna npm run reconcile:hindsight -- --apply --clear uitvoeren om de Hindsight-metadata te verversen.

## Episode 38

> Reconcile dry-run Gaia-Cloud 2026-10-08: Hindsight actief gaia:hypothesis=2, gaia:pattern=0, gaia:kairos_episode=0; Cognition hypotheses=4, patterns=0, kairos=0. De foundation:-prefix-backfill bleek een no-op: de oude 'chronicle:'-prefix werd alleen door de Kairos-synthesizer geproduceerd en er zijn geen Kairos-episodes (GAIA_KAIROS_ENABLED staat uit), dus --apply --clear is overgeslagen (zou enkel versie-churn geven). De 4-vs-2 kan deels survey-artefact zijn: open_question/relationship/mental_model dragen andere tags en tellen niet als gaia:hypothesis.

**A production** (numbers kept 4/5, invented [], gate would reject 0)

- In the Gaia-Cloud reconcile dry-run of 2026-10-08, Gaia AI-assistent's Hindsight had active gaia:hypothesis=2, gaia:pattern=0 and gaia:kairos_episode=0.
- In the Gaia-Cloud reconcile dry-run of 2026-10-08, Gaia AI-assistent's Cognition had hypotheses=4, patterns=0 and kairos=0.
- In the Gaia-Cloud reconcile of 2026-10-08, the Gaia AI-assistent foundation:-prefix-backfill turned out to be a no-op, because the old 'chronicle:'-prefix was only produced by the Kairos-synthesizer and there were no Kairos-episodes.
- At the Gaia-Cloud reconcile of 2026-10-08, GAIA_KAIROS_ENABLED was off in Gaia AI-assistent, so there were no Kairos-episodes.
- In the Gaia-Cloud reconcile of 2026-10-08, --apply --clear was skipped for the Gaia AI-assistent foundation:-prefix-backfill because running it would only cause version churn.
- In the Gaia-Cloud reconcile of 2026-10-08, the gap of 4 Cognition hypotheses versus 2 Hindsight gaia:hypothesis entries in Gaia AI-assistent may be partly a survey artefact, because open_question, relationship and mental_model carry other tags and do not count as gaia:hypothesis.

**B variant** (numbers kept 5/5, invented [], gate would reject 0)

- The Gaia-Cloud reconcile dry-run of 2026-10-08 measured in Hindsight active gaia:hypothesis=2, gaia:pattern=0 and gaia:kairos_episode=0.
- The Gaia-Cloud reconcile dry-run of 2026-10-08 measured in Cognition hypotheses=4, patterns=0 and kairos=0.
- The Gaia foundation:-prefix-backfill turned out to be a no-op because the old 'chronicle:'-prefix was only produced by the Kairos-synthesizer and there were no Kairos-episodes, so --apply --clear was skipped since it would only cause version-churn.
- At the Gaia-Cloud reconcile dry-run of 2026-10-08, GAIA_KAIROS_ENABLED was observed to be off, and there were no Kairos-episodes.
- The Gaia-Cloud 4-vs-2 hypothesis discrepancy between Cognition and Hindsight may be partly a survey-artefact, because open_question, relationship and mental_model carry other tags and do not count as gaia:hypothesis.

## Episode 81

> Gaia-Cloud aanvulling (commit 3f645dd, 2026-10-12): een op het verleden gerichte beurt raadpleegt nu zowel Hindsight als het transcript. recallRelevantContext (memory.js) kreeg een `pastReference`-optie die de recall-gate opent; turn.js geeft die door zodra de gesprekszoek-signalen (exactHistory/pastLookup/rememberedKnowledge) vuren. Reden: Bo's punt 'ze kijkt ook in Hindsight' — de twee gates (memoryPolicy.shouldRecall en de gesprekszoek-signalen) waren onafhankelijk, dus sommige lookup-formuleringen openden alleen het transcript. Let op: Hindsight draait sowieso al elke beurt via de mentale modellen (fetchMentalModelContext), ongeacht de gate. gaia-api 1433/1433 groen (+2).

**A production** (numbers kept 4/4, invented [], gate would reject 0)

- In Gaia-Cloud (commit 3f645dd, 2026-10-12) a past-referencing turn now consults both Hindsight and the transcript.
- In Gaia-Cloud, recallRelevantContext (memory.js) gained a `pastReference` option that opens the recall gate; turn.js passes it as soon as the conversation-search signals (exactHistory/pastLookup/rememberedKnowledge) fire.
- The reason for the Gaia-Cloud pastReference change was Bo's point 'ze kijkt ook in Hindsight': the two gates (memoryPolicy.shouldRecall and the conversation-search signals) were independent, so some lookup phrasings opened only the transcript.
- In Gaia-Cloud, Hindsight already runs every turn via the mental models (fetchMentalModelContext), regardless of the recall gate.
- At commit 3f645dd (2026-10-12), the gaia-api test suite was measured at 1433/1433 passing (+2 tests).

**B variant** (numbers kept 4/4, invented [], gate would reject 2)

- In Gaia-Cloud (Gaia AI-assistent) raadpleegt een op het verleden gerichte beurt sinds commit 3f645dd (2026-10-12) zowel Hindsight als het transcript.
- In Gaia-Cloud kreeg recallRelevantContext (memory.js) in commit 3f645dd een `pastReference`-optie die de recall-gate opent; turn.js geeft die optie door zodra de gesprekszoek-signalen (exactHistory/pastLookup/rememberedKnowledge) vuren.
- De Gaia-Cloud-wijziging in commit 3f645dd volgde op Bo's punt 'ze kijkt ook in Hindsight': de twee gates (memoryPolicy.shouldRecall en de gesprekszoek-signalen exactHistory/pastLookup/rememberedKnowledge) waren onafhankelijk, dus sommige lookup-formuleringen openden alleen het transcript.
- In Gaia-Cloud draait Hindsight sowieso al elke beurt via de mentale modellen (fetchMentalModelContext), ongeacht de recall-gate.
- Na commit 3f645dd (2026-10-12) in Gaia-Cloud was de gaia-api-testsuite 1433/1433 groen, 2 tests meer dan voorheen.

## Episode 86

> Gaia-Cloud (2026-10-12, commit 8e26794, gedeployed): (1) recall-staart gemeten uit Hindsight's Prometheus-histogram (hindsight_http_duration_seconds, recall-endpoint): 27 recalls, mean 894ms, p50 ~1s, p90/p95 ~2,5s, p99 ~5s; 1 call (3,7%) boven 4s, 0 boven 8s. Kleine, niet-representatieve steekproef (grotendeels test-calls), buckets grof. (2) Observability: elke gefaalde Hindsight-call logt nu een content-loze regel {kind:'hindsight.call_failed', bank, op, reason:'timeout'|'unreachable', timeoutMs, ms} en throwt door — via een request()-wrapper rond fetchImpl in src/hindsightClient.js (alle 14 call-sites). Reden: memory.js slikte Hindsight-fouten stil in (best-effort -> []), dus missende context was onzichtbaar. Test test/hindsightCallLog.test.js (+3), gaia-api 1436/1436. Het is puur een logregel, geen gedragsverandering; verschijnt in de gaia-api containerlog.

**A production** (numbers kept 17/18, invented [2.5s 3.7], gate would reject 0)

- For Gaia-Cloud on 2026-10-12, the recall-endpoint tail latency of Hindsight was measured from the Prometheus histogram hindsight_http_duration_seconds. Across 27 recalls the mean was 894ms, p50 about 1s, p90/p95 about 2.5s and p99 about 5s. One call (3.7%) exceeded 4s and none exceeded 8s. The sample was small and not representative, consisting largely of test calls, and the histogram buckets were coarse.
- In Gaia-Cloud commit 8e26794 (deployed 2026-10-12), src/hindsightClient.js gained a request() wrapper around fetchImpl that covers all 14 call-sites. On every failed Hindsight call the wrapper writes a log line without content, {kind:'hindsight.call_failed', bank, op, reason:'timeout'|'unreachable', timeoutMs, ms}, and then rethrows the error. The change only adds a log line and does not alter behaviour. The line shows up in the gaia-api container log.
- The Gaia-Cloud hindsight.call_failed logging was added because memory.js swallowed Hindsight errors without any trace, returning [] on a best-effort basis. As a result, missing context could not be seen.
- For the Gaia-Cloud Hindsight failed-call logging, test/hindsightCallLog.test.js added 3 tests. At commit 8e26794 the gaia-api test suite passed 1436 of 1436 tests.

**B variant** (numbers kept 18/18, invented [], gate would reject 1)

- Op 2026-10-12 (Gaia-Cloud, commit 8e26794) werd de recall-staart van Hindsight gemeten uit het Prometheus-histogram hindsight_http_duration_seconds (recall-endpoint): 27 recalls, mean 894ms, p50 ~1s, p90/p95 ~2,5s, p99 ~5s; 1 call (3,7%) boven 4s, 0 boven 8s. Het was een kleine, niet-representatieve steekproef (grotendeels test-calls) met grove buckets.
- In Gaia-Cloud (commit 8e26794, gedeployed 2026-10-12) logt elke gefaalde Hindsight-call een content-loze regel {kind:'hindsight.call_failed', bank, op, reason:'timeout'|'unreachable', timeoutMs, ms} en throwt door, via een request()-wrapper rond fetchImpl in src/hindsightClient.js (alle 14 call-sites). Het is puur een logregel zonder gedragsverandering en verschijnt in de gaia-api containerlog.
- In Gaia-Cloud slikte memory.js Hindsight-fouten stil in (best-effort -> []), waardoor missende context onzichtbaar was; dat was de reden voor de hindsight.call_failed-logregel in src/hindsightClient.js.
- Voor de Gaia-Cloud Hindsight-call-logging werd test/hindsightCallLog.test.js toegevoegd (+3 tests); gaia-api slaagde toen voor 1436/1436 tests.

## Episode 560

> Beslissing: LongMemEval-harnas is uitgeschreven als docs/design-longmemeval.md in cortexiq (commit eb37c41, gepusht naar main 2026-11-02). Belangrijkste bevinding: internal/benchmark bestaat al en is eerlijk (ingest als episodes, antwoorden via reflect, beide judges) — het gat is de officiële dataset + fairness-protocol + publicatie. Kernregels: judge is een ander/sterker model dan het antwoordmodel, abstention krijgt een speciale judge-regel, geen benchmark-specifieke tuning, alleen de S-variant draaien flattert consolidatie (dus ook de M/day-variant), Hindsight-cijfers worden gequoteerd met bron als hun meting.

**A production** (numbers kept 2/2, invented [], gate would reject 0)

- The LongMemEval harness for cortexiq was written up as docs/design-longmemeval.md in cortexiq (commit eb37c41, pushed to main on 2026-11-02).
- The main finding of the cortexiq LongMemEval design is that internal/benchmark already exists and is fair (ingest as episodes, answers via reflect, both judges). The gap is the official LongMemEval dataset, a fairness protocol and publication.
- A core rule of the cortexiq LongMemEval harness design is that the judge is a different or stronger model than the answer model. Abstention gets a special judge rule.
- A core rule of the cortexiq LongMemEval harness design is that no benchmark-specific tuning is allowed.
- The cortexiq LongMemEval harness design states that running only the LongMemEval S-variant flatters consolidation, so the M/day-variant is also run.
- A core rule of the cortexiq LongMemEval harness design is that Hindsight figures are quoted with their source, as Hindsight's own measurement.

**B variant** (numbers kept 2/2, invented [], gate would reject 1)

- Het LongMemEval-harnas voor CortexIQ is uitgeschreven als docs/design-longmemeval.md in de cortexiq-repository (commit eb37c41, gepusht naar main op 2026-11-02).
- In CortexIQ bestaat internal/benchmark al en is eerlijk (ingest als episodes, antwoorden via reflect, beide judges); het gat voor LongMemEval is de officiële dataset, een fairness-protocol en publicatie.
- Kernregel van het CortexIQ LongMemEval-harnas: de judge is een ander/sterker model dan het antwoordmodel, en abstention krijgt een speciale judge-regel.
- Kernregel van het CortexIQ LongMemEval-harnas: geen benchmark-specifieke tuning.
- Kernregel van het CortexIQ LongMemEval-harnas: alleen de S-variant draaien flattert consolidatie, dus wordt ook de M/day-variant gedraaid.
- Kernregel van het CortexIQ LongMemEval-harnas: Hindsight-cijfers worden gequoteerd met bron als hun meting.

## Episode 364

> Gaia Cloud now retains eligible Hindsight turns as a Hermes-style JSON array of role-labelled User and Assistant messages. Existing memoryworthiness/triviality gates, asynchronous fire-and-forget behavior, and identity/provenance metadata remain; no historical Hindsight data is rewritten. Deployed as commit bddbad5 on 2026-10-23.

**A production** (numbers kept 2/2, invented [], gate would reject 0)

- In Gaia Cloud Hindsight retention, existing memoryworthiness/triviality gates, asynchronous fire-and-forget behavior, and identity/provenance metadata remain.
- With the Gaia Cloud Hindsight turn format change, no historical Hindsight data is rewritten.
- The Gaia Cloud Hindsight turn format change was deployed as commit bddbad5 on 2026-10-23.

**B variant** (numbers kept 2/2, invented [], gate would reject 1)

- Gaia Cloud now retains eligible Hindsight turns as a Hermes-style JSON array of role-labelled User and Assistant messages.
- With the Hermes-style Hindsight retention change, Gaia Cloud keeps its existing memoryworthiness/triviality gates, asynchronous fire-and-forget behavior, and identity/provenance metadata.
- The Gaia Cloud change to Hermes-style Hindsight retention does not rewrite any historical Hindsight data.
- The Gaia Cloud Hermes-style Hindsight retention change was deployed as commit bddbad5 on 2026-10-23.

## Episode 859

> CortexIQ BM25-repetitie op de productiedump (2026-11-11): CI op 3774ea9 groen. Dagelijkse prod-dump (47 MB, checksum OK; prod = 2.944 facts, 728 episodes, 128 MB, goose 60, bge-m3/dim 1024) in verse PG17-container met deploy/postgres-image: pg_restore 5 s, db.Open inclusief migratie 00061 335 ms, bm25-indexen aanwezig, tsvector-kolommen weg. Kwaliteit: BM25 wint bij inspectie van 3 van 15 queries (IDF: zeldzame tokens als 8445 en bge-m3 komen bovenaan, ts_rank_cd gaf generieke woorden); geen gelabelde eval gedraaid, vervolgstap is cortexiq eval recall. Snelheidswinst is op dit kleine corpus nog theoretisch. VPS-toegang was read-only (backuplijst, scp van dump); dump en container na afloop verwijderd. OPEN: prod nog op PG16 en niet geüpgraded; release-job voor cortexiq-postgres en arm64 nooit gedraaid; Bo moet de ssh- en scp-allow-regels en de oude gh-release-create-regel uit .claude/settings.local.json halen (bestand is in git gevolgd, niet committen). Commit van de docs: zie git log.

**A production** (numbers kept 17/21, invented [], gate would reject 0)

- The CortexIQ production database, per the daily prod dump of 2026-11-11 (47 MB, checksum OK), contained 2.944 facts, 728 episodes and 128 MB, was at goose version 60 and used bge-m3 embeddings with dim 1024.
- In the CortexIQ BM25 rehearsal of 2026-11-11, with CI on commit 3774ea9 green, the prod dump was restored into a fresh PG17 container built from deploy/postgres-image: pg_restore took 5 s, db.Open including migration 00061 took 335 ms, the bm25 indexes were present and the tsvector columns were gone.
- In the CortexIQ BM25 rehearsal on the production dump (2026-11-11), BM25 won on inspection in 3 of 15 queries because IDF ranks rare tokens such as 8445 and bge-m3 at the top, whereas ts_rank_cd favoured generic words. No labelled eval was run; the next step is cortexiq eval recall, and the speed gain on the small CortexIQ corpus is still theoretical.
- During the CortexIQ BM25 rehearsal of 2026-11-11, access to the CortexIQ VPS was read-only (backup listing and scp of the dump), and the dump and the PG17 container were deleted afterwards.
- As of 2026-11-11, CortexIQ production was still on PG16 and had not been upgraded, and the release-job for the cortexiq-postgres image and arm64 had never been run.
- As of 2026-11-11, Bo still had to remove the ssh and scp allow rules and the old gh-release-create rule from the CortexIQ .claude/settings.local.json; the file is tracked in git and must not be committed.

**B variant** (numbers kept 16/21, invented [], gate would reject 0)

- On 2026-11-11 the CortexIQ production database contained 2,944 facts and 728 episodes, measured 128 MB, was at goose migration version 60 and used bge-m3 embeddings with dimension 1024. The daily prod dump was 47 MB and its checksum was OK.
- In the CortexIQ BM25 rehearsal on 2026-11-11, the daily prod dump was restored into a fresh PG17 container built from deploy/postgres-image. pg_restore took 5 s and db.Open, including migration 00061, took 335 ms. Afterwards the bm25 indexes were present and the tsvector columns were gone. CI on commit 3774ea9 was green.
- In the CortexIQ BM25 rehearsal on 2026-11-11, inspection showed BM25 winning 3 of 15 queries. Through IDF, rare tokens such as 8445 and bge-m3 ranked at the top, while ts_rank_cd had favoured generic words. No labelled eval was run, and the next step is cortexiq eval recall. The speed gain of BM25 was still theoretical on the small CortexIQ corpus.
- During the CortexIQ BM25 rehearsal on 2026-11-11, VPS access was read-only (listing backups and scp of the dump). The dump and the container were removed afterwards.
- As of 2026-11-11, CortexIQ production was still on PG16 and had not been upgraded. The release job for the cortexiq-postgres image and arm64 had never been run.
- As of 2026-11-11, Bo still had to remove the ssh and scp allow rules and the old gh-release-create rule from the CortexIQ file .claude/settings.local.json. The file is tracked in git and the changes must not be committed.

## Episode 759

> De productiebug supported_by is lokaal opgelost: de background consolidator selecteerde hypotheses zonder supported_by voor pgx named scanning, en positional hypothesis/counter scans misten het veld. Drie bestanden zijn aangepast, walkthrough bijgewerkt, go test ./internal/brain en go test ./... zijn groen. Niet gedeployed; aparte expliciete goedkeuring vereist.

**A production** (numbers kept 0/0, invented [], gate would reject 0)

- The cause of the CortexIQ supported_by production bug was that the background consolidator selected hypotheses without supported_by for pgx named scanning, and the positional hypothesis/counter scans missed the supported_by field.
- The CortexIQ supported_by production bug was fixed locally by changing three files, and the walkthrough was updated.
- After the CortexIQ supported_by bug fix, go test ./internal/brain and go test ./... both passed (green).
- At the time of reporting, the CortexIQ supported_by bug fix was not deployed, and deploying it required separate explicit approval.

**B variant** (numbers kept 0/0, invented [], gate would reject 0)

- The CortexIQ production bug involving supported_by was fixed locally. Cause: the background consolidator selected hypotheses without supported_by for pgx named scanning, and the positional hypothesis/counter scans were missing the supported_by field.
- The CortexIQ supported_by bug fix modified three files, and the walkthrough was updated.
- After the CortexIQ supported_by bug fix, both go test ./internal/brain and go test ./... passed (green).
- The CortexIQ supported_by bug fix had not been deployed at the time; deployment required separate explicit approval.

## Episode 864

> CortexIQ migratie-race bij opstarten gerepareerd en gepusht (2026-11-11): internal/db/db.go draait de hele schema-opzet (goose.Up, dimensie-ALTER, HNSW-indexen) nu in setupSchema onder withSchemaLock, een sessie-advisory lock (pg_advisory_lock, sleutel 994294737458 = 'cortexiq' in base 36) op een eigen verbinding, wachttijd begrensd op 5 minuten, bij annuleren wordt de sessie gesloten zodat er geen lock achterblijft; Open sluit de pool nu bij elke fout (lekte bij goose-fout). Regressietests internal/db/schemalock_integration_test.go (6 gelijktijdige Open's op een lege database, wederzijdse uitsluiting, annuleerbare wacht); mutatiecheck: zonder lock falen 5 van 6 starts met dezelfde 42P07-fout als in prod. Volledige suite groen op verse PG17-db: 21 pakketten, 153 integratietests, 0 skip. NOG NIET UITGEROLD: productie draait v0.1.74 zonder dit slot; het zit in de volgende release. Open: Bo moet de ssh-allow-regels uit .claude/settings.local.json halen (de uitrol is klaar) en de Jina-sleutel roteren; compose op de VPS wijst nog naar de lokale image cortexiq-postgres:17-bm25 (ghcr-image ...-postgres:0.1.74 bestaat).

**A production** (numbers kept 10/12, invented [], gate would reject 0)

- On 2026-11-11, CortexIQ fixed and pushed a migration race at startup. In internal/db/db.go, the complete schema setup (goose.Up, the dimension ALTER and the HNSW indexes) now runs in setupSchema under withSchemaLock. withSchemaLock takes a session advisory lock (pg_advisory_lock, key 994294737458, which is 'cortexiq' in base 36) on its own dedicated connection. The wait for the lock is capped at 5 minutes, and on cancellation the session is closed so no lock remains held.
- On 2026-11-11, the Open function in CortexIQ internal/db/db.go was changed to close the connection pool on any error. Previously the pool leaked when goose failed.
- CortexIQ added regression tests in internal/db/schemalock_integration_test.go. The tests cover 6 concurrent Open calls against an empty database, mutual exclusion and a cancellable wait. A mutation check showed that without the advisory lock, 5 of 6 startups fail with the same 42P07 error that occurred in production.
- On 2026-11-11, the full CortexIQ test suite passed on a fresh PostgreSQL 17 database. The run covered 21 packages and 153 integration tests, with 0 skipped.
- As of 2026-11-11, CortexIQ production was running v0.1.74, which does not include the schema advisory lock fix. The fix was planned for the next release.
- As of 2026-11-11, CortexIQ had open follow-up items: Bo needs to remove the ssh allow rules from .claude/settings.local.json now that the deployment is complete, and Bo needs to rotate the Jina key. In addition, the compose configuration on the VPS was still pointing to the local image cortexiq-postgres:17-bm25, although a ghcr image ...-postgres:0.1.74 exists.

**B variant** (numbers kept 10/12, invented [], gate would reject 0)

- CortexIQ fixed a migration race at startup (2026-11-11): internal/db/db.go now runs the whole schema setup (goose.Up, dimension ALTER, HNSW indexes) in setupSchema under withSchemaLock, a session advisory lock (pg_advisory_lock, key 994294737458 = 'cortexiq' in base 36) on its own connection. At that time the wait was capped at 5 minutes, and on cancellation the session is closed so no lock remains behind.
- In CortexIQ, Open in internal/db/db.go now closes the pool on every error; previously the pool leaked on a goose error.
- CortexIQ added regression tests in internal/db/schemalock_integration_test.go (6 concurrent Open calls on an empty database, mutual exclusion, cancellable wait). A mutation check showed that without the lock 5 of 6 starts fail with the same 42P07 error as in production. The full CortexIQ suite was green on a fresh PG17 database: 21 packages, 153 integration tests, 0 skipped.
- On 2026-11-11 CortexIQ production was running v0.1.74 without the schema advisory lock. The migration race fix had not yet been deployed and was planned for the next release.
- For CortexIQ, Bo still had to remove the ssh-allow rules from .claude/settings.local.json because the deployment was finished, and had to rotate the Jina key.
- On 2026-11-11 the compose configuration on the CortexIQ VPS still pointed to the local image cortexiq-postgres:17-bm25, although the ghcr image ...-postgres:0.1.74 existed.

## Episode 169

> CortexIQ prod (strato-vps, /opt/cortexiq) is op 2026-10-15 uitgerold naar commit 6377b4c: POST /api/v1/remember is live (401 zonder token, 201 met token) en de embedder wordt bij start op de achtergrond warm geladen (~1,5 s, logregel msg="embedder warm"); warme recall meet ~5 ms in plaats van de oude koude start van ~1,8 s. Goose-versie bleef 27.

**A production** (numbers kept 9/9, invented [1.5 1.8], gate would reject 0)

- CortexIQ prod (strato-vps, /opt/cortexiq) was rolled out to commit 6377b4c on 2026-10-15.
- In the CortexIQ prod rollout of 2026-10-15 (commit 6377b4c), POST /api/v1/remember was live, returning 401 without a token and 201 with a token.
- As of the CortexIQ prod rollout of 2026-10-15 (commit 6377b4c), the CortexIQ embedder was warm-loaded in the background at start, taking ~1.5 s and logging msg="embedder warm".
- After the CortexIQ prod rollout of 2026-10-15 (commit 6377b4c), warm recall in CortexIQ measured ~5 ms instead of the old cold start of ~1.8 s.
- The Goose version of CortexIQ prod remained 27 during the rollout of 2026-10-15 to commit 6377b4c.

**B variant** (numbers kept 9/9, invented [1.5 1.8], gate would reject 0)

- CortexIQ prod (strato-vps, /opt/cortexiq) was rolled out to commit 6377b4c on 2026-10-15.
- In the CortexIQ prod rollout of 2026-10-15 (commit 6377b4c), the POST /api/v1/remember endpoint was live, returning 401 without a token and 201 with a token.
- As of the CortexIQ prod rollout of 2026-10-15 (commit 6377b4c), the CortexIQ embedder was warm-loaded in the background at start, taking ~1.5 s and logging msg="embedder warm".
- After the CortexIQ prod rollout of 2026-10-15 (commit 6377b4c), warm recall in CortexIQ measured ~5 ms instead of the old cold start of ~1.8 s.
- The Goose version of CortexIQ prod remained 27 after the rollout of 2026-10-15 to commit 6377b4c.

## Episode 697

> CortexIQ: RRF-sweep gedraaid op de productie-embedder (bge-m3, hybride, Jina-reranker; 60 memories, 36 vragen, k=10) via docker compose run met een override naar een wegwerpdatabase cortexiq_eval (productie niet geraakt, daarna verwijderd). Uitkomst overall: rrf-k 20 hit@1 0.75 MRR 0.850; rrf-k 60 hit@1 0.78 MRR 0.868; rrf-k 100 identiek aan 60. Verschil is één à twee vragen van 36, dus ruis. Besluit: CORTEXIQ_RRF_K blijft 60. Alle varianten missen dezelfde acht vragen op ongeveer dezelfde plaats, dus een betere recall zit in embedder/reranker, niet in de fusieconstante. Recept voor een volgende keer: op de server in /opt/cortexiq een override met CORTEXIQ_POSTGRES_DSN naar een extra database, dan docker compose -f docker-compose.prod.yml -f override run --rm --no-deps -T -v /opt/cortexiq/eval:/eval:ro cortexiq eval recall --rrf-k ... --corpus /eval/recall_corpus.jsonl --queries /eval/recall_queries.jsonl. Les: gebruik bij .env-greps nooit een patroon dat KEY-variabelen raakt (RERANK matchte CORTEXIQ_RERANKER_API_KEY en de Jina-sleutel kwam in het gesprek; Bo is aangeraden hem te roteren).

**A production** (numbers kept 11/11, invented [], gate would reject 0)

- A CortexIQ RRF-sweep was run on the production embedder (bge-m3, hybrid, Jina-reranker; 60 memories, 36 questions, k=10) via docker compose run with an override pointing to a throwaway database cortexiq_eval. Production was not touched, and cortexiq_eval was deleted afterwards. Overall results: rrf-k 20 gave hit@1 0.75 and MRR 0.850; rrf-k 60 gave hit@1 0.78 and MRR 0.868; rrf-k 100 was identical to rrf-k 60.
- After the CortexIQ RRF-sweep, the decision was that CORTEXIQ_RRF_K stays at 60. The difference between the rrf-k variants was one or two questions out of 36, which was judged to be noise.
- In the CortexIQ RRF-sweep, all rrf-k variants missed the same eight questions at roughly the same place. Better recall for CortexIQ therefore lies in the embedder/reranker, not in the RRF fusion constant.
- Recipe for a future CortexIQ recall eval: on the server in /opt/cortexiq, create an override with CORTEXIQ_POSTGRES_DSN pointing to an extra database. Then run: docker compose -f docker-compose.prod.yml -f override run --rm --no-deps -T -v /opt/cortexiq/eval:/eval:ro cortexiq eval recall --rrf-k ... --corpus /eval/recall_corpus.jsonl --queries /eval/recall_queries.jsonl.
- Lesson for CortexIQ: never use a grep pattern on .env that matches KEY variables. The pattern RERANK matched CORTEXIQ_RERANKER_API_KEY, which exposed the Jina key in the conversation, and Bo was advised to rotate the Jina key.

**B variant** (numbers kept 11/11, invented [], gate would reject 4)

- Voor CortexIQ is een RRF-sweep gedraaid op de productie-embedder (bge-m3, hybride, Jina-reranker; 60 memories, 36 vragen, k=10) via docker compose run met een override naar een wegwerpdatabase cortexiq_eval (productie niet geraakt, daarna verwijderd). Uitkomst overall: rrf-k 20 hit@1 0.75 MRR 0.850; rrf-k 60 hit@1 0.78 MRR 0.868; rrf-k 100 identiek aan 60. Het verschil is één à twee vragen van 36, dus ruis.
- Na de RRF-sweep is voor CortexIQ besloten dat CORTEXIQ_RRF_K op 60 blijft staan.
- In de CortexIQ RRF-sweep missen alle rrf-k-varianten dezelfde acht vragen op ongeveer dezelfde plaats, dus een betere recall voor CortexIQ zit in de embedder/reranker, niet in de fusieconstante.
- Recept om een CortexIQ recall-eval te draaien: op de server in /opt/cortexiq een override met CORTEXIQ_POSTGRES_DSN naar een extra database, dan docker compose -f docker-compose.prod.yml -f override run --rm --no-deps -T -v /opt/cortexiq/eval:/eval:ro cortexiq eval recall --rrf-k ... --corpus /eval/recall_corpus.jsonl --queries /eval/recall_queries.jsonl.
- Les bij CortexIQ: gebruik bij .env-greps nooit een patroon dat KEY-variabelen raakt; het patroon RERANK matchte CORTEXIQ_RERANKER_API_KEY waardoor de Jina-sleutel in het gesprek kwam, en Bo is aangeraden de Jina-sleutel te roteren.

## Episode 888

> CortexIQ: v0.1.80 (Timeline-vergelijking tussen twee data, commit 6f725cd, CI groen) is door Claude gepubliceerd en de Release-workflow is groen (2026-11-13). De uitrol naar productie is NIET uitgevoerd: de auto-mode classifier blokkeerde het ssh-commando voor de deploy. Productie draait vermoedelijk nog v0.1.79 (niet geverifieerd). Bo moet de deploy toestaan of zelf draaien. Let op: vaste toestemming van Bo gold voor het publiceren van releases; uitrol via ssh valt buiten wat de classifier toestaat zonder allow-regel in .claude/settings.local.json.

**A production** (numbers kept 4/4, invented [], gate would reject 0)

- CortexIQ v0.1.80 (Timeline-vergelijking tussen twee data, commit 6f725cd, CI groen) was published by Claude and the Release-workflow was green on 2026-11-13.
- The production deploy of CortexIQ v0.1.80 was not executed on 2026-11-13 because the auto-mode classifier blocked the ssh command for the deploy; Bo must allow the deploy or run it himself.
- CortexIQ production was presumed to still be running v0.1.79 on 2026-11-13, which was not verified.
- For CortexIQ, Bo's standing permission covered publishing releases, but deployment via ssh falls outside what the auto-mode classifier allows without an allow-rule in .claude/settings.local.json.

**B variant** (numbers kept 4/4, invented [], gate would reject 0)

- CortexIQ v0.1.80 (Timeline-vergelijking tussen twee data, commit 6f725cd, CI groen) was published by Claude and the Release-workflow was green on 2026-11-13.
- The deploy of CortexIQ v0.1.80 to production was not executed on 2026-11-13 because the auto-mode classifier blocked the ssh command for the deploy; Bo must allow the deploy or run it himself.
- On 2026-11-13 CortexIQ production was presumably still running v0.1.79, which was not verified.
- For CortexIQ, Bo's standing permission covered publishing releases, but deploying via ssh falls outside what the auto-mode classifier allows without an allow rule in .claude/settings.local.json.

## Episode 412

> CortexIQ-productie (strato-vps, /opt/cortexiq) draait sinds 2026-10-27 v0.1.6 (commit c107f8c, image sha256:61d7ed372332). Deployprocedure: GitHub-release publiceren (release-workflow bouwt binaries en image), CORTEXIQ_IMAGE_TAG in /opt/cortexiq/.env bumpen, daarna docker compose -f docker-compose.prod.yml pull en up -d. De UI is een aparte service (cortexiq-ui) op dezelfde image; UI luistert op 127.0.0.1:18101, API op 18100, beide eisen een token.

**A production** (numbers kept 6/6, invented [], gate would reject 0)

- CortexIQ-productie (strato-vps, /opt/cortexiq) was observed running v0.1.6 (commit c107f8c, image sha256:61d7ed372332) since 2026-10-27.
- CortexIQ deploy procedure: publish a GitHub release (the release workflow builds binaries and image), bump CORTEXIQ_IMAGE_TAG in /opt/cortexiq/.env, then run docker compose -f docker-compose.prod.yml pull and up -d.
- The CortexIQ UI runs as a separate service (cortexiq-ui) on the same image as the CortexIQ API.
- CortexIQ production was observed with the UI (cortexiq-ui) listening on 127.0.0.1:18101 and the API on port 18100, both requiring a token.

**B variant** (numbers kept 6/6, invented [], gate would reject 0)

- CortexIQ production (strato-vps, /opt/cortexiq) was observed running v0.1.6 (commit c107f8c, image sha256:61d7ed372332), deployed since 2026-10-27.
- The CortexIQ production deploy procedure is: publish a GitHub release (the release workflow builds the binaries and image), bump CORTEXIQ_IMAGE_TAG in /opt/cortexiq/.env, then run docker compose -f docker-compose.prod.yml pull and up -d.
- The CortexIQ UI runs as a separate service (cortexiq-ui) on the same image in CortexIQ production.
- In CortexIQ production, the CortexIQ UI (cortexiq-ui) was observed listening on 127.0.0.1:18101 and the CortexIQ API on port 18100, with both requiring a token.

## Episode 886

> CortexIQ werkwijze (2026-11-13): Bo geeft vaste toestemming dat Claude zelf GitHub-releases publiceert (gh release create <versie> --target main) zodra CI op main groen is; Bo hoeft het niet meer zelf te doen. Daarbij altijd versienummer en releasenotities melden in het antwoord. Uitrol naar productie volgt de vaste procedure (back-up .env, pull terwijl het draait, verified dump, tag bumpen, healthz/readyz/restarts/logs controleren). Eerder deed Bo het publiceren zelf vanaf v0.1.74; die gewoonte is nu vervangen.

**A production** (numbers kept 2/2, invented [], gate would reject 0)

- Voor CortexIQ gaf Bo op 2026-11-13 vaste toestemming dat Claude zelf GitHub-releases publiceert (gh release create <versie> --target main) zodra CI op main groen is. Bo hoeft het publiceren niet meer zelf te doen. Dit vervangt de eerdere gewoonte waarbij Bo vanaf v0.1.74 zelf publiceerde.
- Bij het publiceren van CortexIQ GitHub-releases moet Claude in het antwoord altijd het versienummer en de releasenotities melden.
- De uitrol van CortexIQ naar productie volgt een vaste procedure: back-up van .env, pull terwijl het draait, verified dump, tag bumpen, en daarna healthz, readyz, restarts en logs controleren.

**B variant** (numbers kept 1/2, invented [], gate would reject 1)

- Voor CortexIQ geeft Bo vaste toestemming dat Claude zelf GitHub-releases publiceert (gh release create <versie> --target main) zodra CI op main groen is; Bo hoeft het niet meer zelf te doen. Dit vervangt de eerdere gewoonte waarbij Bo vanaf v0.1.74 het publiceren zelf deed.
- Bij het publiceren van een CortexIQ GitHub-release meldt Claude altijd het versienummer en de releasenotities in het antwoord.
- Uitrol van CortexIQ naar productie volgt de vaste procedure: back-up van .env, pull terwijl het draait, verified dump, tag bumpen, en healthz/readyz/restarts/logs controleren.

## Episode 670

> CortexIQ: v0.1.20 (e56a03b) is op 2026-11-05 gedeployed naar strato-vps; v0.1.19 is wel gebouwd maar nooit uitgerold, productie ging direct van 0.1.18 naar 0.1.20. Bevat migraties 00044 (judged), 00045 (origin/origin_ref) en 00046 (strength), de rebuild-CLI en CORTEXIQ_RRF_K. Volgorde: geverifieerde dump backups/cortexiq-20261105T002405Z.dump, release, .env gebackupt als .env.bak-pre0.1.20, tag-bump en up -d. Gecontroleerd: version v0.1.20, goose 46, containers Up, geen panic of migratiefout, 2330 facts allemaal zonder origin (oude facts, bewust niet teruggevuld). Een read-only rebuild-rapport van /projects/cortexiq in productie: 114 episodes, 476 facts, 6 facts op ingetrokken bronnen (gat C). CORTEXIQ_DECAY_SPLIT staat NIET aan en docker-compose.prod.yml op de server geeft de variabele nog niet door. Open voor Bo: de ssh- en gh-release-allow-regels uit de projectsettings halen; beslissen over DECAY_SPLIT; zelf de RRF-sweep draaien (cortexiq eval recall --rrf-k 20,60,100).

**A production** (numbers kept 17/17, invented [], gate would reject 0)

- CortexIQ v0.1.20 (e56a03b) was deployed to strato-vps on 2026-11-05. v0.1.19 was built but never rolled out, so production went directly from 0.1.18 to 0.1.20. CortexIQ v0.1.20 contains migrations 00044 (judged), 00045 (origin/origin_ref) and 00046 (strength), the rebuild-CLI and CORTEXIQ_RRF_K.
- The CortexIQ v0.1.20 deployment to strato-vps followed this order: a verified dump backups/cortexiq-20261105T002405Z.dump, the release, a backup of .env as .env.bak-pre0.1.20, a tag-bump, then up -d.
- After the CortexIQ v0.1.20 deployment on strato-vps, checks showed version v0.1.20, goose 46, containers Up, and no panic or migration error. All 2330 facts were without origin; these are old facts that were deliberately not backfilled.
- A read-only CortexIQ rebuild report of /projects/cortexiq in production showed 114 episodes, 476 facts, and 6 facts on revoked (ingetrokken) sources (gat C).
- At the time of the CortexIQ v0.1.20 deployment, CORTEXIQ_DECAY_SPLIT was not enabled, and docker-compose.prod.yml on the strato-vps server did not yet pass the variable through.
- Open tasks for Bo after the CortexIQ v0.1.20 deployment: remove the ssh and gh-release allow rules from the project settings, decide on DECAY_SPLIT, and run the RRF sweep personally (cortexiq eval recall --rrf-k 20,60,100).

**B variant** (numbers kept 17/17, invented [], gate would reject 0)

- CortexIQ v0.1.20 (e56a03b) was deployed to strato-vps on 2026-11-05. v0.1.19 was built but never rolled out, so CortexIQ production went directly from 0.1.18 to 0.1.20. The deployment order was: a verified dump backups/cortexiq-20261105T002405Z.dump, the release, a backup of .env as .env.bak-pre0.1.20, a tag-bump, and up -d.
- CortexIQ v0.1.20 contains migrations 00044 (judged), 00045 (origin/origin_ref) and 00046 (strength), the rebuild-CLI and CORTEXIQ_RRF_K.
- After the CortexIQ v0.1.20 deployment on strato-vps, the checks showed version v0.1.20, goose 46, containers Up, and no panic or migration error. All 2330 facts in CortexIQ production had no origin, because the old facts were deliberately not backfilled.
- A read-only rebuild report of /projects/cortexiq in CortexIQ production showed 114 episodes, 476 facts, and 6 facts on revoked sources (gat C).
- At the time of the CortexIQ v0.1.20 deployment, CORTEXIQ_DECAY_SPLIT was not enabled in production, and docker-compose.prod.yml on the server did not yet pass the variable through.
- Open tasks for Bo after the CortexIQ v0.1.20 deployment: remove the ssh and gh-release allow rules from the project settings, decide on CORTEXIQ_DECAY_SPLIT, and run the RRF sweep personally (cortexiq eval recall --rrf-k 20,60,100).

## Episode 924

> CortexIQ besluit (2026-11-13, Bo): Jina (jina-reranker-v3.5) blijft de reranker in productie; geen eigen reranker op de VPS. Reden: lokaal (llama.cpp, bge-reranker-v2-m3) is ongeveer 2,9 s trager per recall (mogelijk 3-5 s op de VPS, gedeeld met andere workloads) en iets slechter (MRR 0,80 tegen 0,85; zie id 923). Niets gewijzigd in productie. Het risico dat Jina wegvalt (saldo, 429-limiet van 100.000 tokens per minuut, 404) wordt afgedekt door de metric recall_rerank_requests_total, de logregel per minuut, RERANK_CANDIDATES=30 en de graaf-routing als vangnet (gewogen is dan beter dan uit). Niet gedaan en niet nodig tenzij het pijn doet: fallback-keten Jina dan lokaal (codewijziging), meten met 10 kandidaten, meten op de VPS zelf.

**A production** (numbers kept 15/15, invented [], gate would reject 0)

- Voor CortexIQ is op 2026-11-13 (Bo) besloten dat Jina (jina-reranker-v3.5) de reranker in productie blijft en dat er geen eigen reranker op de VPS komt; in productie is niets gewijzigd.
- Voor CortexIQ werd een lokale reranker (llama.cpp, bge-reranker-v2-m3) afgewezen omdat die gemeten ongeveer 2,9 s trager per recall was (mogelijk 3-5 s op de VPS, gedeeld met andere workloads) en iets slechter scoorde dan Jina (MRR 0,80 tegen 0,85; zie id 923).
- Het risico dat de Jina-reranker van CortexIQ wegvalt (saldo, 429-limiet van 100.000 tokens per minuut, 404) wordt afgedekt door de metric recall_rerank_requests_total, de logregel per minuut, RERANK_CANDIDATES (op dat moment ingesteld op 30) en de graaf-routing als vangnet, omdat gewogen dan beter is dan uit.
- Voor de CortexIQ-reranker zijn een fallback-keten Jina dan lokaal (codewijziging), meten met 10 kandidaten en meten op de VPS zelf niet gedaan en niet nodig geacht tenzij het pijn doet.

**B variant** (numbers kept 13/15, invented [2.9 0.80 0.85], gate would reject 0)

- On 2026-11-13 Bo decided that Jina (jina-reranker-v3.5) remains the reranker in production for CortexIQ, with no own reranker on the VPS; nothing was changed in production.
- For CortexIQ, a local reranker (llama.cpp with bge-reranker-v2-m3) was measured as about 2.9 s slower per recall than Jina (possibly 3-5 s on the VPS, which is shared with other workloads) and slightly worse (MRR 0.80 against 0.85; see id 923). This was the stated reason for keeping Jina in production.
- For CortexIQ, the risk that the Jina reranker drops out (balance, 429 limit of 100,000 tokens per minute, 404) is covered by the metric recall_rerank_requests_total, the per-minute log line, RERANK_CANDIDATES set to 30 at the time of the decision, and the graph routing as a safety net, because weighted results are better than none.
- For the CortexIQ reranker, the following were not done and are considered unnecessary unless problems arise: a fallback chain of Jina then local (requires a code change), measuring with 10 candidates, and measuring on the VPS itself.

## Episode 664

> CortexIQ: v0.1.18 (afgewezen-waarde-tombstone, migratie 00043, Reflect-tab #7) is op 2026-11-05 gedeployed naar strato-vps. Volgorde: geverifieerde dump (backups/cortexiq-20261104T232239Z.dump), release gepubliceerd door Bo (de auto-mode classifier weigerde gh release create van mij), .env gebackupt als .env.bak-pre0.1.18, tag-bump en up -d. Gecontroleerd: version v0.1.18 (6c89dd3), goose 43, facts_refused in de logs. Productie had 0 retracties, dus de backfill deed niets. Het Gemma-parsefoutje bij causal links in /projects/chronicle is het bekende lijstprobleem, niet nieuw. Praktisch: SSH naar strato-vps werkt via een allow-regel in de projectsettings; die hoort na de deploy weg.

**A production** (numbers kept 8/8, invented [], gate would reject 0)

- CortexIQ v0.1.18 (afgewezen-waarde-tombstone, migration 00043, Reflect-tab #7) was deployed to strato-vps on 2026-11-05.
- The CortexIQ v0.1.18 deployment to strato-vps followed this order: a verified dump (backups/cortexiq-20261104T232239Z.dump), the release published by Bo because the auto-mode classifier refused to run gh release create, .env backed up as .env.bak-pre0.1.18, then tag-bump and up -d.
- After the CortexIQ v0.1.18 deployment on 2026-11-05, production on strato-vps was checked and showed version v0.1.18 (6c89dd3), goose migration version 43, and facts_refused appearing in the logs.
- When CortexIQ v0.1.18 was deployed, CortexIQ production had 0 retractions, so the backfill of migration 00043 did nothing.
- The Gemma parse error in causal links in CortexIQ's /projects/chronicle is the known list problem and is not new.
- During the CortexIQ v0.1.18 deployment, SSH to strato-vps worked via an allow rule in the project settings; that allow rule should be removed after the deploy.

**B variant** (numbers kept 8/8, invented [], gate would reject 0)

- CortexIQ v0.1.18 (afgewezen-waarde-tombstone, migratie 00043, Reflect-tab #7) was deployed to strato-vps on 2026-11-05.
- The CortexIQ v0.1.18 deployment to strato-vps followed this order: a verified dump (backups/cortexiq-20261104T232239Z.dump), the release published by Bo because the auto-mode classifier refused gh release create from the assistant, .env backed up as .env.bak-pre0.1.18, then tag-bump and up -d.
- After the CortexIQ v0.1.18 deployment on strato-vps, the checks showed version v0.1.18 (6c89dd3), goose 43, and facts_refused in the logs.
- During the CortexIQ v0.1.18 deployment, CortexIQ production had 0 retracties, so the backfill did nothing.
- The Gemma parse error for causal links in /projects/chronicle of CortexIQ is the known list problem, not a new issue.
- At the time of the CortexIQ v0.1.18 deployment, SSH to strato-vps worked via an allow-regel in the projectsettings, which is meant to be removed after the deploy.

---

episodes 21 | A: 105 facts, numbers kept 159/171, invented 5 | B: 106 facts, numbers kept 158/171, invented 6, would be rejected by the production gate 13
