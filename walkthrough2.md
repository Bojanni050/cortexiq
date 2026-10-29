# Walkthrough 2

Opvolger van `walkthrough.md`, dat op 1008 regels zat en daarmee gloedde
voor de regel dat er na 1000 regels een nieuw walkthrough-bestand komt.

## 2026-10-27 (Confidence naast de zin in de Facts-tabel)

- Findings: de Facts-tabel toonde alleen de zin (plus de gestructureerde
  velden); de confidence stond uitsluitend in de context-rail. Wie door de
  lijst scrollt ziet dus geen getal, en die scheiding tussen rij en nummer
  was de derde keer aanleiding voor de vraag "waarom 53%". Bo vroeg
  vervolgens expliciet om de percentages ook in de middelste kolom, naast
  het feit zelf, náást dat ze al in de rail stonden.
- Conclusions: (1) dit is een presentatie-oplossing, geen formule-wijziging —
  `calculateConfidence` en de decay zijn niet verdacht, ze zijn in de
  vorige entry gecontroleerd. (2) Het nummer staat er nu twee keer; dat is
  bewust, want de tabel en de rail dienen verschillende leesmomenten.
- Actions: `internal/webui/static/index.html` — in `renderFacts` kwam
  `${confidence(f)}` (een gedempte span met `pct(f.confidence)`) direct na
  de zin en vóór de gestructureerde velden; de rail ongewijzigd.
- Validated — het geëxtraheerde script slaagde voor `node --check`,
  `go build ./...` groen, en de testserver op 18199 serveerde de nieuwe
  UI (bevatte `const confidence`), gepusht als `f79120e`.
