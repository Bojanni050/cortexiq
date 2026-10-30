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

## 2026-10-30 (Graph: audit canonical_type, model-vergelijking, guard en opruiming)

- Findings: Audit op prod (1294 relaties sinds 15-10): 723 (56%) hebben een `canonical_type`: 453 via de beoordeelde mapping, 15 eigen vocabulairewoord, 255 via suggestie van het extractiemodel. Minstens 30 daarvan zijn fout omdat het model negatie of richting negeert (`lacks`/`does_not_include` → contains, `used_by` → runs_on, `accessible_at` → hosts, `included_in` → contains). Ontdubbelen werkt (0 duplicaten onder de nieuwe rijen; 480 oude blijven). Correctie op eerdere sessie-uitspraken: het productiemodel is NIET meer `edenai/qwen3-30b-a3b` (stond zo in `.env` op 15-10), maar `mistralai/Mistral-Small-24B-Instruct-2501` via DeepInfra (gewisseld tussen 17 en 27-10), dus de audit mengt twee modellen. Sinds 27-10 hadden 59 van 92 consolidatieruns met LLM-calls fouten (89 fouten op 496 calls), vrijwel allemaal "parse json: invalid character…".
- Conclusions: Modelvergelijking (relatie-extractie, productieprompt, 100 willekeurige facts uit projectnamespaces; geen /clients, /user, /self, /infra, /adlytica, geen facts met token/wachtwoord/IP/e-mail; ~$0,25 in totaal, via DeepInfra): huidig model Mistral-Small-24B: 43 relaties, 6 van 20 batches onparsebaar (30%), 51% met type; Gemma-4-31B-it: 84 relaties, 0 parse-fouten, 87% met type, 8 s/aanroep, $0,006; DeepSeek-V4-Flash: 125 relaties (63% met type bij 4096 tokens), $0,003; Qwen3.5-397B en Qwen3.6-35B "denken" en worden afgebroken op 1024 tokens (pas bruikbaar bij 4096, dan 44 en 125 s/aanroep). Kwaliteit van de eigen suggesties handmatig beoordeeld: bij alle modellen ongeveer 4/5 (Gemma) tot 1/2 (DeepSeek) plausibel, dus geen model vervangt een guard. Conclusie: de guard in code is model-onafhankelijk nodig; en het huidige model is vooral een betrouwbaarheidsprobleem (parse-fouten), niet alleen een precisieprobleem. Modelwissel is een aparte beslissing (raakt alle consolidatiestappen) en vraagt een eigen test; niet doorgevoerd. Beperkingen: kleine steekproef, alleen de relatiestap, mijn "negatie/omgekeerd"-telling markeert ook onschuldige `_in`-types.
- Actions: `internal/reasoner/relvocab.go` (`SuggestionAllowed` + patronen `guardNegationPattern`, `guardPassivePattern`, `guardInwardPattern`, tabel `outwardCanonicalTypes`; alleen toegepast op de suggestie van het model, nooit op de beoordeelde mapping; `_of` bewust niet in het omkeerpatroon, want "consisted of" is een correct contains), `relvocab_test.go` (`TestSuggestionGuard` met de echte auditgevallen, `TestGuardPatternsMatchCleanupScript`), `scripts/clear-unsafe-canonical-types.sql` (dry-run standaard, idempotent). Validated — `go test` reasoner/brain/queries groen met Postgres; eerst op een restore van een verse prod-backup (36 rijen, `relation_type`/van/naar md5 ongewijzigd, tweede run 0), daarna op prod met backup `cortexiq-20261030T020809Z.dump`: dry-run 36 rijen, apply 36 → 2306 van 6124 relaties met type, `relation_type` onaangetast, tweede dry-run 0. Commit be0dd7d (alleen mijn bestanden; de ongecommitte `webui.go`/`status.go`/`opencode.jsonc` in de werkmap zijn van een andere sessie en niet aangeraakt). NIET uitgerold: prod draait via GitHub-release + `CORTEXIQ_IMAGE_TAG` (nu 0.1.15), dus de guard is pas actief na een release; tot dan kunnen nieuwe verdachte suggesties binnenkomen, die het opruimscript opnieuw kan wegzetten.
