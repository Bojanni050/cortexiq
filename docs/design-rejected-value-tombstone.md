# Ontwerpvoorstel: afgewezen-waarde-tombstone

Status: **voorstel, niets gebouwd.** Besluiten van de eigenaar staan in "Wat ik van jou nodig heb".

## Waarom

De hertoets van de Agent Memory Atlas (punt 1) noemde dit gat, en de CI-run van 2026-11-04 heeft het op echte Postgres bevestigd (`TestIntegrationRejectedValueStaysRejected`, `KNOWN GAP`): een via `RetractFact` afgewezen waarde komt terug als actieve fact zodra hij opnieuw wordt beweerd.

Oorzaak: `retractions` is gesleuteld op de rij (`target_type`, `target_id`) en wordt op geen enkel schrijfpad gelezen. `nearestActiveFacts` slaat beëindigde facts bovendien met opzet over (`dedup.go:58-62`), zodat een opnieuw geobserveerde waarde niet door een oude fact wordt ingeslikt. Voor dedup klopt dat. Voor correctie is het precies verkeerd.

Sinds `00c7815` schrijven alle facts via `Brain.insertFact` (`fact_insert.go`). De check kan dus op één plek.

## Wat het is

Een afwijzing wordt een record op de **waarde**, niet op de rij. Voordat `insertFact` een fact schrijft, kijkt het of de genormaliseerde waarde in dezelfde namespace is afgewezen en niet is opgeheven. Zo ja, dan wordt de fact niet geschreven.

Een tombstone ontstaat alleen bij `retract` (die al een reden afdwingt). `forget` zonder oordeel en `invalidate` zonder reden maken er geen.

## De keuzes

### 1. Wat telt als "dezelfde waarde"

| | Sleutel | Vangt | Mist | Kosten |
|---|---|---|---|---|
| **A** | genormaliseerde tekst van de fact | exact, hoofdletters, spaties, leestekens, unicode-lookalikes | parafrasen ("Kafka is onze queue") | één indexlookup |
| **B** | A, plus (entity, property, value) als de extractor die gaf | dezelfde claim in andere woorden, mits de extractor structuur leverde | alles zonder structuur; hangt af van extractorkwaliteit | idem |
| **C** | vectorgelijkenis met de afgewezen claim, met LLM-review in de dedup-band | parafrasen | kan een legitieme update blokkeren ("de queue is weer kafka"); LLM-kosten | LLM-call per treffer |

**Aanbeveling: A nu, B als aparte stap erna, C niet.** De Atlas laat zien dat normalisatie het echte werk is (te streng en de tombstone vuurt nooit, te los en hij blokkeert updates; vier red-teamrondes). Begin met de strengste variant die meetbaar is. Het restrisico bij A (parafrasen) is bekend en wordt hier uitgesproken, niet verstopt.

### 2. Normalisatie (bij A)

NFKC, daarna casefold, whitespace samenvouwen, leestekens aan de randen weg. Diakrieten blijven staan, anders botsen verschillende woorden. De sleutel is de sha256 van de genormaliseerde tekst.

Twee bewaakte randgevallen uit de Atlas:
- Een genormaliseerde tekst die leeg wordt, maakt **geen** tombstone. De Atlas beschrijft een systeem dat alles in niet-Latijns schrift tot de lege string reduceerde en alles identiek hashte.
- De eigen rij matcht nooit zichzelf, en een andere voorgestelde rij blokkeert nooit.

### 3. Wat er bij een treffer gebeurt

| Optie | Effect |
|---|---|
| **Weigeren** | de fact wordt niet geschreven; de episode telt als verwerkt zodat het checkpoint doorgaat; een teller `facts_refused` in de run |
| Als kandidaat bewaren | de fact bestaat, maar is meteen beëindigd; meer rommel |
| Als contradictie markeren | past slecht, een tombstone is geen tegenspraak tussen twee facts |

**Aanbeveling: weigeren, met teller.** De Atlas waarschuwt dat een tombstone het enige mechanisme is waarvan het stille falen eruitziet als succes. Een telling in `consolidation_runs` maakt het zichtbaar.

### 4. Terugweg

Zonder opheffen zit een gebruiker die van gedachten verandert vast. Voorstel: `lifted_at`, `lifted_by`, `lift_reason` op de retractie; een tombstone is actief zolang `lifted_at IS NULL`. Het retractie-record blijft staan als geschiedenis.

**Aanbeveling: ja, in dezelfde migratie**, met een CLI-commando; de dashboardknop mag later.

### 5. Scope

De exacte namespace, niet kinderen of ouders. Een afwijzing in project A geldt niet voor B. Open punt: een afwijzing in `/` geldt dan alleen voor `/`.

### 6. Welke paden

Alle origins van `insertFact`, behalve `eval` (een harnas met eigen namespace). Ook `import`: een backup hoort een afgewezen waarde niet terug te brengen. Export neemt geen retracties mee, dus in een verse namespace is er niets om te blokkeren.

### 7. Dependency

`golang.org/x/text/unicode/norm` staat al als *indirecte* dependency in `go.mod` (v0.34.0). Gebruiken betekent dat hij **direct** wordt: geen nieuwe module, wel een wijziging in `go.mod`. Het alternatief is alleen stdlib (`strings.ToLower`, `unicode.SimpleFold`), maar dan glippen unicode-lookalikes erdoor. Dat is precies de bypass van ronde 9 in de Atlas (`strip().lower()` zonder NFKC).

**Aanbeveling: `norm` gebruiken.** Dat vraagt jouw akkoord, want het is een dependency-wijziging.

## Schema (migratie 00043)

- `retractions`: `value_key TEXT NULL`, `lifted_at TIMESTAMPTZ NULL`, `lifted_by TEXT NULL`, `lift_reason TEXT NULL`.
- Partial index op `(namespace_id, value_key) WHERE lifted_at IS NULL AND value_key IS NOT NULL`.
- `consolidation_runs`: `facts_refused INT NOT NULL DEFAULT 0`.
- Backfill van bestaande retracties: de sleutel kan niet in SQL (NFKC), dus een idempotente Go-stap bij opstart (`WHERE value_key IS NULL`).

## Gedrag dat je moet kennen

- **Purge heft de tombstone op.** `purge.go:369-384` verwijdert de retractie als het doel hard wordt verwijderd. Purge is "laat geen spoor na", dus dat is consistent, maar het is de ronde-8-vraag van de Atlas (een opruimactie die de wasserij heropent). Voorstel: zo laten en hier vastleggen.
- **Een afwijzing van een episode** maakt een tombstone op de episodetekst. Dat blokkeert alleen een fact die letterlijk dezelfde tekst heeft. De afgeleide facts van een ingetrokken episode worden er niet door geraakt (dat is gat C uit de hertoets, een apart punt).
- **Parafrasen** vallen bij A buiten de tombstone (zie keuze 1).

## Stappen (elk een kleine commit)

1. Migratie 00043, normalisatiefunctie en unit-tests (casefold, whitespace, NFKC, fullwidth, niet-Latijns, leeg).
2. `recordRetraction` schrijft `value_key`; Go-backfill voor bestaande rijen.
3. Check in `insertFact` (`ErrRejectedValue`), teller in de run, en `tombstoneImplemented = true`. De wasserijtest wordt dan strikt.
4. Opheffen (CLI), met test.
5. Extra tests: hoofdletter- en spatievariant, fullwidth `ｋａｆｋａ`, niet-Latijns, andere namespace niet geblokkeerd, opgeheven tombstone laat door, eigen rij matcht nooit zichzelf.

Stap 2 en 3 horen bij elkaar in release-volgorde: pas als de backfill gedraaid heeft, vuurt de check voor oude retracties.

## Wat ik van jou nodig heb

1. **Sleutel:** A nu (aanbevolen), of A en B samen?
2. **Treffer:** weigeren met teller (aanbevolen)?
3. **`golang.org/x/text` direct gebruiken** (aanbevolen), of alleen stdlib?
4. **Opheffen** in dezelfde migratie (aanbevolen)?
5. **Import** ook toetsen (aanbevolen)?
