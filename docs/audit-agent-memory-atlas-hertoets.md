# CortexIQ getoetst aan de Agent Memory Atlas — hertoets op HEAD

- Gelezen: `main` @ `f620a20` (2026-11-03, v0.1.17). Eerdere audit: `f8847f3` (2026-10-26). 72 commits ertussen.
- Methode: statisch, bestand + regel. **Niets gedraaid**: geen test, geen recall, geen consolidatie. Uitspraken gaan over code, niet over gedrag.
- Bron van de acht punten: sectie 11 van `Agent-Memory-Atlas-analysev2pdf.pdf`.

## Uitslag

| # | Punt | Eerder | Nu | Verandering |
|---|------|--------|----|-------------|
| 1 | Tombstone op afgewezen waarden | gedeeltelijk | **gedeeltelijk, ongewijzigd** | `invalidate` toegevoegd, maar rij-gesleuteld |
| 2 | Scope als sleutel | gedeeltelijk | **gedeeltelijk, schrijfkant beter** | schrijven zonder namespace wordt geweigerd |
| 3 | Bewijs voor geloof | gedeeltelijk | **gedeeltelijk, gat C deels dicht** | supersede/invalidate sluiten afgeleide facts |
| 4 | Eén schrijfpoort | open | **open, ongewijzigd** | nog vijf `INSERT INTO facts` |
| 5 | Trust-toestand | bijna | **bijna, ongewijzigd** | decay mengt nog steeds geloof en bereikbaarheid |
| 6 | Telemetrie is geen waarheid | in orde | **in orde** | `rate_fact` onaangeroerd |
| 7 | Hybride retrieval | in orde | **in orde** | `rrfK = 60` nog een default, geen meting |
| 8 | Gate de dure weg | gedeeltelijk | **gedeeltelijk, ongewijzigd** | recall nog steeds zonder poort; `AddEpisode` embedt nog synchroon |

## Per punt

### 1. Tombstone — gat blijft, `invalidate` is geen oplossing

- `retractions` wordt op het schrijfpad nergens gelezen. Schrijvers: `retract.go:152`, `invalidate.go:171`. Lezers: `purge.go:372-384`, `retract.go:133,195` (telling en listing), CLI, webui. Geen enkel `INSERT`-pad raadpleegt de tabel.
- `nearestActiveFacts` (`dedup.go:58-62`) filtert nog op `deleted_at IS NULL AND valid_until IS NULL`, met dezelfde comment. Een afgewezen waarde is dus onzichtbaar voor dedup en wordt bij herbeweren een nieuwe actieve fact.
- `InvalidateKeyed` (`invalidate.go`) is een verbetering, maar op een andere as. De sleutel is een `external_key` die de **caller** kiest, bedoeld voor een spiegel (Gaia). Het is geen sleutel op `(scope, subject, predicate, genormaliseerde waarde)`. Een caller die dezelfde waarde onder een andere key of zonder key opnieuw schrijft, wordt niet tegengehouden: `insertEpisode` (`episode.go:~235-290`) kijkt alleen naar `external_key` bij live rijen (`deleted_at IS NULL`).
- De reden is bij `invalidate` optioneel (`invalidate.go`, `reasonPtr`). Zonder reden is er geen retractie-record. Dat is een stap terug ten opzichte van `retract`, dat een reden afdwingt (`validateReason`).
- Wat wel goed is: retirement, fact-closing en retractie gebeuren in één transactie, en een herhaling geeft `AlreadyInvalidated` in plaats van een fout.
- Testdekking: `external_test.go` heeft retire + reason + key-vrijheid. Er is nog **geen test** die een afgewezen waarde opnieuw aanbiedt en eist dat hij inactief blijft.

**Oordeel:** eerdere bevinding blijft staan. `invalidate` lost "een spiegel kan een claim intrekken" op, niet "een volgende schrijver kan hem niet terugzetten".

### 2. Scope — betere schrijfkant, open leeskant

- Nieuw sinds `caec750`: `remember` en de vier record-tools weigeren een write zonder namespace (`mcp.go:71-92`, `requiredNamespace`), met een foutmelding die `list_namespaces` noemt. Een expliciete `/` blijft toegestaan. Dit was een bewuste ontwerpkeuze (zie commitbericht), geen vergeten gat.
- Onveranderd: de read-tools defaulten nog op `"/"` (`mcp.go:315, 391, 428, 482, 655, 864, …`). Een recall zonder argument leest alles. `forget` ook (`mcp.go:428`).
- De per-token namespace-allowlist is weg, `Token.LegacyNamespaces` bestaat alleen nog voor een startupwaarschuwing (`auth.go:22-26`). `caec750` heeft ook `authz.go` en `scope.go` verder uitgekleed.
- Audit-nuance: de eerdere audit noemde de verwijderde allowlist een gezakt punt. De commit onderbouwt juist waarom de binding op de verkeerde as zat (apps vs projecten). Dat is geen fout, maar `scope.caller_cannot_widen` zakt daardoor nog steeds. Voor één gebruiker is dit laag risico, bij een tweede client wordt het reëel.

**Oordeel:** gedeeltelijk, met een expliciete en gedocumenteerde keuze. Alleen relevant zodra er een tweede caller komt.

### 3. Bewijs voor geloof

- Gat A (niet elke fact herleidbaar): **ongewijzigd.** `fact_sources` wordt alleen geschreven door `consolidate.go:641,675` en `dedup.go:374`. `hypothesis.go:246` en `consolidate_failure.go:118` schrijven nog facts zonder bronlink.
- Gat B (geen rebuild-pad): **ongewijzigd.** Geen reindex, rebuild of re-extract. `invalidate.go` noemt "mirror rebuild", maar dat betreft een extern spiegelsysteem, niet de afgeleide laag.
- Gat C (soft delete bereikt de afgeleiden niet): **deels dicht.** Nieuw `endFactsOfEpisode` (`episode.go:~320`) sluit facts waarvan de ingetrokken episode de enige live bron was, bij supersede en bij `invalidate`. `ForgetEpisode` en `RetractEpisode` doen dat nog niet: ze zetten alleen `deleted_at` op de episode (`episode.go:381-390`). Er zijn dus nu drie verwijderwoorden met verschillend bereik.

### 4. Eén schrijfpoort — open

Nog vijf `INSERT INTO facts`: `consolidate.go:628`, `hypothesis.go:246`, `consolidate_failure.go:118`, `export.go:326`, `eval_graph.go:216`. Alleen de eerste doet dedup, conflictcheck en bronlinks.

Nieuw is een **tweede poort aan de episode-kant**: `write_filter.go` draait vóór elke schrijfweg, maar staat standaard op `off` (`WriteFilterOff`). Dat is een goede stap voor secrets en ruis, en het bestaat voor episodes. Het raakt de fact-insertwegen niet.

### 5. Trust-toestand — ongewijzigd

`decay.go:27-40` vermenigvuldigt nog `confidence` voor facts die niet recent zijn gezien, met `GREATEST(updated_at, COALESCE(last_recalled_at, updated_at))`. Onder de drempel beëindigt hetzelfde veld het feit. Geloof en bereikbaarheid delen dus nog een veld.

### 6. Telemetrie — in orde

`feedback.go` is onaangeroerd: stem verandert alleen rang, nooit confidence, geldigheid of decay.

### 7. Hybride retrieval — in orde

`rrfK = 60` staat nog op `recall.go:91` als default uit de paper. De per-arm `Match` en `RecallTrace` bestaan nog. Open meting: RRF-constante op eigen corpus sweepen.

### 8. Gate de dure weg — ongewijzigd

- Capture en consolidatie hebben een poort (social filter, minimumlengte, checkpoint). Nieuw: `reasoner/triage.go` laat een goedkope triage een cluster weigeren vóór extractie.
- Recall embedt nog elke keer de query (`episode.go:347`), zonder poort ervoor.
- `AddEpisode` embedt nog synchroon (`episode.go:196`). Geen model-vrije capture.

## Wat de hertoets veranderde

- **Verbeterd:** schrijven zonder namespace faalt nu hard; supersede en invalidate sluiten afgeleide facts atomair; er is een write filter en een triage-poort.
- **Niet verbeterd:** gat 1 (waarde-sleutel), gat 4 (vijf insertwegen), rebuild, bronplicht, decay-splitsing, recall-poort.
- **Nieuwe observatie:** `invalidate` zonder reden laat geen sporen na. Dat is hetzelfde probleem als gat 3 (`forget` zonder mode), nu ook voor het nieuwe pad.

## Wat deze hertoets niet bewijst

Statisch, één commit. De retractietest (waarde afwijzen, opnieuw beweren, kijken of er een actieve fact komt) is nog steeds niet uitgevoerd en is de goedkoopste manier om gat 1 van vermoeden naar feit te brengen. De Hermes-plugin en andere clients zijn niet gelezen.

## Aanbevolen volgorde (ongewijzigd, met correcties)

1. Vijf insertwegen naar één functie (`gateway.no_bypass_path`). Maakt de rest goedkoper.
2. Waarde-sleutel op de retractie plus consultatie op het schrijfpad, met een test die de wasserijsequentie draait. Bij `invalidate` een reden verplicht maken, of expliciet "geen oordeel" met actor vastleggen.
3. Bronplicht voor facts uit hypothese en failure-patronen.
4. `forget` zonder mode: reden en actor vastleggen of weigeren.
5. Geloof en bereikbaarheid splitsen (decay op een eigen sterkteveld).
6. Rebuild-pad.
7. Recall-poort en RRF-sweep.
8. Namespace-autorisatie per token: alleen zodra er een tweede caller komt.

## Bijgewerkt na de hertoets

- **Punt 4 (één schrijfpoort), 2026-11-04:** de vijf `INSERT INTO facts` lopen nu via `Brain.insertFact` (`internal/brain/fact_insert.go`). `TestFactsAreInsertedInOnePlace` faalt zodra een ander bestand zelf in `facts` schrijft. Dat is `gateway.no_bypass_path` in de goedkoopste vorm, zonder database. Het is nog geen volledige gateway: `insertFact` doet zelf geen dedup, conflictcheck of bronplicht. Het is het punt waar die checks nu op één plek kunnen komen.
- **Punt 1 (tombstone), 2026-11-04:** de wasserijtest bevestigde het gat op echte Postgres (de afgewezen waarde kwam terug als actieve fact). Daarna gebouwd volgens `docs/design-rejected-value-tombstone.md`: `value_key` op `retractions`, een check in `insertFact` (`ErrRejectedValue`), `facts_refused` in de run en opheffen via `cortexiq lift`. De wasserijtest is strikt. Open: parafrasen vallen buiten de sleutel (bewust), en de structuurvariant (entity, property, value) is niet gebouwd. Het resultaat in CI is bij het schrijven hiervan nog niet gezien.
- **Punt 4 uit de volgorde (`forget` zonder mode) en de `invalidate`-observatie, 2026-11-05:** `forget` en `invalidate` zonder reden schrijven nu een auditrij in `retractions` met `judged = false` (migratie 00044): actor, tijd en de vergeten tekst. De rij krijgt nooit een `value_key` (de tabel dwingt dat af met een CHECK) en blokkeert dus niets: een forget is geen oordeel, en de waarde mag terugkomen. Een latere `retract` van dezelfde episode kan nog steeds; de auditrij telt niet als "al ingetrokken". `lift` weigert een auditrij (`ErrNotJudged`) en de backfill slaat hem over. Bewust niet gekozen: een reden verplichten, omdat dat het contract van `forget` en `invalidate` voor alle clients breekt. De integratietests voor dit pad draaien alleen in CI.
- **Punt 3 uit de volgorde (bronplicht), 2026-11-05:** `facts` kreeg `origin` en `origin_ref` (migratie 00045; NULL op oude facts, bewust niet teruggevuld). `insertFact` schrijft de fact en zijn `fact_sources` in één statement (een CTE), dus geen fact zonder de bronnen die de aanroeper noemde, en een bron die niet bestaat laat de hele write falen. Het weggooien van fouten bij het koppelen van bronnen in `storeFactWithConfig` is daarmee weg. `requireSource` weigert (`ErrNoSource`) een geconsolideerde fact zonder bronepisode en een hypothese- of failure-fact zonder `origin_ref`; import en eval zijn vrijgesteld. Een bevestigde hypothese krijgt `hypothesis:<id>` en de episodes achter zijn `source_fact_ids`; een failure-patroon krijgt `failure:<ids>` en de episodes uit zijn batch. Recall geeft `origin` en `origin_ref` mee. Open: `reinforceFact` (dedup) koppelt nog best-effort buiten een transactie, en oude facts blijven zonder herkomst. Anders dan de vorige punten is dit lokaal tegen Postgres gedraaid (zie hieronder).
