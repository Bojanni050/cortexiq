# CortexIQ memory provider for Hermes Agent

Geeft Hermes een automatische geheugenlus op CortexIQ: **recall vóór elke turn**,
**capture erna**. Geen dependencies — dit is een dunne stdlib-client tegen je
HTTP-API; alle embeddings, opslag en consolidatie blijven server-side.

De **tools** lopen niet via deze package maar via de MCP-koppeling
(`mcp_servers`). Dat is bewust: MCP-tooldefinities rijden in elk request mee, dus
een tweede kopie van `recall`/`remember` kost tokens per turn zonder extra
kunnen. Deze provider levert alleen de lus die MCP niet kan.

## Installeren

Vanuit de repo:

```bash
pip install ./integrations/hermes
```

(Eenmaal gepubliceerd op PyPI is `pip install cortexiq-hermes` genoeg. Tot die
tijd kun je ook de gebouwde wheel pakken: `dist/cortexiq_hermes-0.1.0-py3-none-any.whl`.)

Zet daarna in `~/.hermes/config.yaml`:

```yaml
memory:
  provider: cortexiq
```

en herstart de gateway.

> **Twee installatieroutes, kies er één.** Hermes zoekt een provider eerst als
> map onder `$HERMES_HOME/plugins/<naam>/` en pas daarna als pip entry point.
> Heb je de mappenversie staan, dan wint die en wordt deze package nooit
> geladen. Laat dus niet beide staan.

## Configureren

Alles via omgevingsvariabelen (in `$HERMES_HOME/.env`):

| Variabele | Default | Betekenis |
| --- | --- | --- |
| `CORTEXIQ_API_URL` | `http://127.0.0.1:18101` | basis-URL van de HTTP-API |
| `CORTEXIQ_API_TOKEN` | — | **verplicht**; gebruik een token met write-rechten in één namespace |
| `CORTEXIQ_NAMESPACE` | `/` | recall-scope; `/` = **alle** namespaces |
| `CORTEXIQ_INGEST_NAMESPACE` | = `CORTEXIQ_NAMESPACE` | capture-scope; **moet bestaan**, `/` kan niet |
| `CORTEXIQ_RECALL_LIMIT` | `5` | hoeveel treffers per turn worden opgehaald |
| `CORTEXIQ_MIN_SCORE` | `0.6` | drempel op de RRF-rangscore (zie hieronder) |
| `CORTEXIQ_MAX_CHARS` | `1400` | maximum aan geïnjecteerde context per turn |
| `CORTEXIQ_INGEST_PATH` | `/api/v1/remember` | pas aan als je route anders heet |
| `CORTEXIQ_TURN_IMPORTANCE` | `0.5` | importance bij capture |
| `CORTEXIQ_CAPTURE_ASSISTANT` | `false` | ook het antwoord opslaan (rijkere episodes) |
| `CORTEXIQ_ASSISTANT_MAX_CHARS` | `0` | afkapgrens voor dat antwoord; `0` = geen |
| `CORTEXIQ_CAPTURE_MAX_CHARS` | `4000` | harde grens op een hele episode |

**Twee namespaces, en dat is expres.** `/` is de root: voor **lezen** betekent het
"zoek in alles", maar er bestaat geen namespace-rij voor — schrijven naar `/`
geeft dus 400 (`namespace not found`). Standaard leest de plugin daarom globaal
over al je namespaces, terwijl capture naar één concreet namespace gaat. Dat
namespace moet bestaan (`POST /api/v1/namespaces`); ontbreekt het, dan faalt de
write en zegt de plugin dat één keer hardop in plaats van stil data te verliezen.

## Wat er per turn wordt opgeslagen

Standaard alleen je eigen bericht — één episode per turn, `source: turn`. Dat is
goedkoop, maar elke herinnering is dan een **vraag zonder antwoord**: later vind
je terug wat er gevraagd is, niet wat er geconcludeerd is.

Zet `CORTEXIQ_CAPTURE_ASSISTANT=true` om beide kanten op te slaan, met
rol-labels zodat de extractor ze kan scheiden:

```
user: hoe staat mijn dashboard er nu voor?

assistant: Het dashboard bindt op 127.0.0.1 en zit achter Tailscale Serve…
```

Standaard wordt er **niets** afgekapt (`CORTEXIQ_ASSISTANT_MAX_CHARS=0`); de enige
rem is `CORTEXIQ_CAPTURE_MAX_CHARS` (4000) op de hele episode. Moet er toch
gekapt worden, dan gebeurt dat op een zins- of alineagrens — nooit midden in een
woord, en nooit midden in een open code-blok (dat wordt netjes gesloten). Er
blijft **kop én staart** bewaard (60/40), want de conclusie van een antwoord staat
meestal aan het eind; het weggelaten stuk wordt gemarkeerd met `…[afgekapt]…`.

Dat is geen detail. Kapte een antwoord midden in een open ```-blok af, dan kreeg
de extractor onafgesloten markdown te verwerken en kon daar een verminkte — of
zelfs misleidende — fact uit halen. En een antwoord waarvan net het slot
verdwijnt, levert een herinnering op die de verkeerde vraag beantwoordt.

## Twee dingen die je moet weten

**Filter op `score`, nooit op `similarity`.** `score` is de RRF-rangfusie
(genormaliseerd 0..1, en rangpositie is gekalibreerd); `similarity` is rauwe
cosine `1 - (embedding <=> query)` en dat is bij een embedding-model niet
gekalibreerd. Gemeten in de praktijk:

```
match      rijen  similarity-veld aanwezig
vector      29    29/29
hybrid      24    24/24
keyword      6     0/6      <- veld volledig afwezig
```

Een keyword-only treffer heeft dus **geen** similarity, en een
similarity-drempel gooit juist exacte-term-treffers weg die hoog scoren. Deze
provider leest `similarity` daarom nergens. Ontbreekt `score`, dan houdt hij de
rij: liever een keertje extra dan een exacte treffer weggooien.

**Verwijder nooit op een `id` uit een recall-response.** Elk resultaat draagt
`type` (`episode` of `fact`) plus precies één getypeerd veld (`fact_id` of
`episode_id`). Facts en episodes nummeren hun rijen **onafhankelijk**, dus
hetzelfde getal bestaat in beide tabellen — `DELETE /api/v1/facts/{id}` met een
episode-id verwijdert een willekeurige andere fact. Gebruik altijd het getypeerde
veld en de route die daarbij hoort. Een ontbrekende rij geeft 404, een fout
verzoek 400, een echte fout 500.

## Verifiëren

```bash
hermes plugins validate ./integrations/hermes   # of de mappenversie
hermes memory status                            # moet cortexiq tonen, niet "not found"
```

En dan één echte turn: vraag iets waarvan je weet dat het in CortexIQ staat. De
recall loopt in de achtergrond (`queue_prefetch`), dus de eerste turn na een
lange idle-periode kan even duren — de embedder laadt lazy bij de eerste call.
