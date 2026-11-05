# Ontwerp: geloof en bereikbaarheid splitsen (decay)

Status: **gebouwd (2026-11-05), standaard uit.** Punt 5 uit de volgorde van de Atlas-hertoets.

## Probleem

`confidence` betekende twee dingen: hoe goed een fact wordt ondersteund, en of hij nog in gebruik is. `DecayConfidence` vermenigvuldigde `confidence` met `DecayFactor` voor facts die niet binnen het venster zijn waargenomen of opgevraagd, en een fact onder `ExpiryThreshold` werd beëindigd. Een waar feit dat een tijd niemand nodig had, was voor de opslag niet te onderscheiden van een feit waar het bewijs zich tegen keerde.

## Wat er gebouwd is

- Migratie 00046: `facts.strength REAL NOT NULL DEFAULT 1.0`. Elk bestaand feit start op 1.0.
- `CORTEXIQ_DECAY_SPLIT` (standaard `false`). Uit: niets verandert, `DecayConfidence` doet wat het deed. Aan: verval verlaagt `strength`, verlopen kijkt naar `strength`, en `confidence` verandert niet meer door onbruik.
- Herobservatie (`reinforceFact`) zet `strength` terug op 1.0. Een recall stopt het verval, zoals voorheen.
- Recall geeft `strength` mee zodra een feit begint te vervagen, en laat het weg bij 1.

## Keuzes en waarom

- **Vlag uit als standaard.** Met de vlag aan stopt een feit met weinig steun met automatisch verlopen: alleen onbruik beëindigt nog. Dat is bedoeld, maar het verandert wat in productie verdwijnt. Aanzetten is een besluit van de eigenaar.
- **Geen terugvulling.** Bestaande facts met een al verlaagde `confidence` houden die waarde; ze krijgen `strength = 1.0`. Raden hoeveel er door onbruik en hoeveel door bewijs was gezakt zou een verzonnen record zijn.
- **Een aparte kolom, geen nieuwe tabel.** Het is een eigenschap van de fact zelf, op dezelfde rij als `confidence`.
- **Recall-rangschikking ongewijzigd.** `confidence` wordt in de rangschikking nauwelijks gebruikt (alleen als tiebreak in de graafarm); `strength` doet dat ook niet. Wie wil dat vervaagde feiten lager scoren, doet dat in een aparte stap, na meting.

## Terugkomen na onbruik (toegevoegd 2026-11-05)

Een feit dat alleen door onbruik is beëindigd, wordt weer actief als dezelfde claim opnieuw wordt gezien, in plaats van dat er een tweede feit naast komt. Het was nooit onwaar; het werd even niet gebruikt.

- Migratie 00047: `facts.expired_by_disuse_at`. Alleen de decay-sweep zet die, en alleen met de split aan. NULL op een beëindigd feit betekent een andere reden (vervangen, ongeldig gemaakt, eerder dan deze migratie), en dan blijft het beëindigd en wordt dezelfde claim een nieuw feit, zoals altijd.
- De dedup kiest dormante feiten als kandidaat naast actieve; bij gelijke score wint een actief feit. Bij een treffer wordt het feit teruggezet (`valid_until`, `known_until` en de markering leeg, `strength` 1) en daarna als gewoon dedup-treffer versterkt.
- Een afwijzing die tijdens de slaap is gedaan, geldt nog: de revival gaat door dezelfde `checkRejectedValue` als elke andere weg naar binnen, en een geweigerde revival telt als geweigerd.
- Gekozen: de periode van slaap wordt niet op de rij bewaard. Een as-of-vraag die binnen die periode valt, ziet het feit daarna als geldig. De tijdlijn is dus iets minder nauwkeurig dan met een nieuw feit; dat is de prijs van "het is hetzelfde feit".
- Een revival telt in `facts_deduplicated`, niet in een eigen teller: het is een herobservatie die in een bestaand feit opgaat.

## Aanzetten

Zet `CORTEXIQ_DECAY_SPLIT=true`. Let op: `docker-compose.prod.yml` op de server moet de variabele doorgeven (zie `docker-compose.prod.example.yml`). Terugzetten is de variabele weghalen; `strength`-waarden blijven staan en doen niets.
