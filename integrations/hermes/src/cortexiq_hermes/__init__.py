"""CortexIQ — memory provider voor Hermes Agent.

Automatische lus bovenop CortexIQ: recall vóór elke turn, capture erna.
De *tools* (goals, hypotheses, causal chains, ...) lopen via de MCP-koppeling die
al in `mcp_servers` staat — deze plugin voegt daar bewust GEEN tools aan toe, want
dat zou elke tool-definitie dubbel in elke request proppen.

Transport:
  - lezen    : POST http://127.0.0.1:18101/api/v1/recall    (recall-lus, hybrid RRF)
  - schrijven: POST <CORTEXIQ_INGEST_PATH> = /api/v1/remember (maakt een episode; de
               extractor consolideert die later tot facts, dus niet synchroon)

Over de namespace: recall mag '/' — dat is de root en betekent "zoek in alles".
Schrijven vereist een BESTAAND namespace, want '/' heeft geen rij in de
namespaces-tabel (remember geeft dan 400). Recall en capture hebben daarom elk
hun eigen variabele, zodat je globaal kunt blijven lezen.

Configuratie via omgevingsvariabelen (Hermes laadt ~/.hermes/.env):
  CORTEXIQ_API_TOKEN       verplicht
  CORTEXIQ_API_URL         default http://127.0.0.1:18101
  CORTEXIQ_RECALL_SCOPE    global | base | profile   (default global = alles)
  CORTEXIQ_NAMESPACE_BASE  default /hermes  waaronder de agent-profielen leven
  CORTEXIQ_NAMESPACE       zet één recall-namespace (wint van de scope)
  CORTEXIQ_INGEST_NAMESPACE default <base>/<profiel>; moet BESTAAN om te schrijven

Per profiel kan dit ook in <hermes_home>/cortexiq.json (of cortexiq/config.json),
wat `hermes profile clone` mee-copieert. De JSON wint van de omgeving:

  {"recall_scope": "profile", "base_namespace": "/hermes",
   "recall_limit": 8, "min_score": 0.65, "capture_assistant": true}

Zo bedient één proces meerdere profielen zonder dat ze in elkaars geheugen
schrijven: capture gaat naar <base>/<profiel>, en recall bepaalt de scope.
  CORTEXIQ_RECALL_LIMIT  default 5
  CORTEXIQ_MIN_SCORE     default 0.6    (gemeten: ruis plafonneert ~0.43,
                                         relevante treffers beginnen ~0.70)
  CORTEXIQ_MAX_CHARS     default 1400   (cap op wat er per turn in de prompt gaat)
  CORTEXIQ_INGEST_PATH   default /api/v1/remember
  CORTEXIQ_CAPTURE_ASSISTANT  default false  ook het antwoord opslaan (rijkere episodes)
  CORTEXIQ_ASSISTANT_MAX_CHARS default 0     afkapgrens voor dat antwoord (0 = geen)
  CORTEXIQ_CAPTURE_MAX_CHARS  default 4000   harde grens op een hele episode
                                             (afkappen gebeurt op een zins- of
                                             paragraafgrens, bewaart kop en staart,
                                             en sluit een open code-blok)
"""

from __future__ import annotations

import json
import logging
import os
import re
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any, Optional

from agent.memory_provider import MemoryProvider, RecallStatus, spawn_context_thread

logger = logging.getLogger(__name__)

PROVIDER_NAME = "cortexiq"

# 18101 = de `serve`-http-listener (dashboard + /api/v1). NIET 18100: dat is de
# MCP-listener, en die heeft geen /api/v1-routes (daar kreeg ik 404).
_DEFAULT_API = "http://127.0.0.1:18101"
_RECALL_PATH = "/api/v1/recall"
_RECALL_TIMEOUT = 5.0   # warm ~5 ms, koud ~1800 ms (embedder); ruim genoeg
_INGEST_TIMEOUT = 8.0

_TOKEN_CACHE: dict[str, str] = {}


def _env(name: str, default: str = "") -> str:
    """Env-waarde, via de profiel-scope van Hermes als die er is.

    Een multiplex-gateway bedient meerdere profielen uit ÉÉN proces en zet de
    .env van een profiel bewust niet in os.environ — anders zouden de sleutels
    van profiel A in de turns van B lekken. get_secret() leest die context-local
    scope en valt bij één profiel gewoon terug op os.environ, dus dit is voor
    beide situaties de juiste route.
    """
    value = None
    try:
        from agent.secret_scope import get_secret

        value = get_secret(name)
    except Exception:
        pass                      # geen scope actief, of een oudere Hermes
    if value is None:
        value = os.environ.get(name)
    return (value or "").strip() or default


def _truthy(value: str) -> bool:
    """Env-vlag -> bool. Alleen deze waarden gelden als aan."""
    return value.strip().lower() in ("1", "true", "yes", "on")


# -- configuratie per profiel ------------------------------------------------
#
# Hermes' eigen conventie: provider-config in <hermes_home>/cortexiq.json (of
# <hermes_home>/cortexiq/config.json). `hermes profile clone` copieert dat mee,
# dus een gekloond profiel neemt zijn instellingen mee. Volgorde: JSON ->
# omgeving (profiel-scoped) -> default.

_JSON_KEYS: dict[str, str] = {
    "api_url": "CORTEXIQ_API_URL",
    "base_namespace": "CORTEXIQ_NAMESPACE_BASE",
    "namespace": "CORTEXIQ_NAMESPACE",
    "ingest_namespace": "CORTEXIQ_INGEST_NAMESPACE",
    "ingest_path": "CORTEXIQ_INGEST_PATH",
    "recall_scope": "CORTEXIQ_RECALL_SCOPE",
    "recall_limit": "CORTEXIQ_RECALL_LIMIT",
    "min_score": "CORTEXIQ_MIN_SCORE",
    "max_chars": "CORTEXIQ_MAX_CHARS",
    "turn_importance": "CORTEXIQ_TURN_IMPORTANCE",
    "capture_assistant": "CORTEXIQ_CAPTURE_ASSISTANT",
    "assistant_max_chars": "CORTEXIQ_ASSISTANT_MAX_CHARS",
    "capture_max_chars": "CORTEXIQ_CAPTURE_MAX_CHARS",
}

_CONFIG_CACHE: dict[str, dict] = {}


def _home(home: str | Path | None = None) -> Path:
    """De home van dit profiel. Bij voorkeur de geïnjecteerde, niet HERMES_HOME:
    in een multiplex-gateway is die laatste de home van het proces, niet van het
    profiel dat deze turn bedient."""
    return Path(home or _env("HERMES_HOME") or (Path.home() / ".hermes"))


def _config(home: str | Path | None = None) -> dict:
    """De cortexiq.json van dit profiel, of {}."""
    base = _home(home)
    key = str(base)
    if key in _CONFIG_CACHE:
        return _CONFIG_CACHE[key]
    cfg: dict = {}
    for candidate in (base / "cortexiq.json", base / "cortexiq" / "config.json"):
        try:
            if not candidate.is_file():
                continue
            data = json.loads(candidate.read_text(encoding="utf-8"))
            if isinstance(data, dict):
                cfg = data
                logger.info("%s: config gelezen uit %s", PROVIDER_NAME, candidate)
            else:
                logger.warning("%s: %s is geen JSON-object — genegeerd", PROVIDER_NAME, candidate)
        except Exception as exc:
            logger.warning("%s: %s onleesbaar (%s) — genegeerd", PROVIDER_NAME, candidate, exc)
        break
    _CONFIG_CACHE[key] = cfg
    return cfg


def _setting(name: str, default: str = "", *, home: str | Path | None = None) -> str:
    """Eén instelling: JSON -> omgeving -> default."""
    value = _config(home).get(name)
    if isinstance(value, bool):
        return "true" if value else "false"
    if isinstance(value, (int, float)):
        return str(value)
    if isinstance(value, str) and value.strip():
        return value.strip()
    env_name = _JSON_KEYS.get(name)
    return _env(env_name, default) if env_name else default


def _profile(home: str | Path | None = None) -> str:
    """Profielnaam: <...>/profiles/<naam> -> <naam>, anders 'default'."""
    base = _home(home)
    return base.name if base.parent.name == "profiles" and base.name else "default"


def _base_namespace(home: str | Path | None = None) -> str:
    """Namespace waaronder alle agent-profielen leven (default /hermes)."""
    ns = (_setting("base_namespace", "/hermes", home=home) or "/hermes").strip()
    return ns if ns.startswith("/") else "/" + ns


def _recall_namespaces(home: str | Path | None = None) -> Optional[list[str]]:
    """Wat recall doorzoekt. None = geen filter, dus de root: alles.

    recall_scope:
      global  (default) -> None              alles, je hele corpus
      base              -> ['/hermes']       de subtree: alle agent-profielen
      profile           -> ['/hermes/<prof>'] strikt dit profiel

    Het namespace-filter van CortexIQ is hiërarchisch: '/projects' levert ook
    '/projects/stash' en '/projects/gaia' op. Daarom is 'base' één pad.

    Een expliciete `namespaces`-lijst in de JSON wint van de scope, en een
    expliciete `namespace` zet één namespace (het gedrag van vóór de scopes).
    """
    explicit = _config(home).get("namespaces")
    if isinstance(explicit, list) and explicit:
        return [str(x) for x in explicit]
    single = _setting("namespace", "", home=home).strip()
    if single and single != "/":
        return [single]
    scope = _setting("recall_scope", "global", home=home).strip().lower()
    if scope == "profile":
        return [f"{_base_namespace(home)}/{_profile(home)}"]
    if scope == "base":
        return [_base_namespace(home)]
    if scope not in ("global", "/", ""):
        logger.warning("%s: onbekende recall_scope %r — val terug op 'global'",
                       PROVIDER_NAME, scope)
    return None


def _ingest_namespace(home: str | Path | None = None) -> str:
    """Capture-namespace: expliciet, anders <base>/<profiel>.

    Moet een BESTAAND namespace zijn. '/' is de root: lezen mag daar, schrijven
    niet (geen rij in de namespaces-tabel -> 400). Bestaat het nog niet, dan
    maakt de provider hem bij de eerste write zelf aan.
    """
    explicit = _setting("ingest_namespace", "", home=home).strip()
    ns = explicit or f"{_base_namespace(home)}/{_profile(home)}"
    return ns if ns.startswith("/") else "/" + ns


def _ensure_namespace(ns: str) -> bool:
    """Maak het namespace aan als het ontbreekt. True als het nu bestaat."""
    base = _env("CORTEXIQ_API_URL", _DEFAULT_API).rstrip("/")
    req = urllib.request.Request(
        f"{base}/api/v1/namespaces",
        data=json.dumps({"slug": ns, "name": f"Hermes agent ({ns.lstrip('/')})"}).encode(),
        method="POST",
        headers={"Authorization": f"Bearer {_token()}",
                 "Content-Type": "application/json"},
    )
    try:
        with urllib.request.urlopen(req, timeout=_INGEST_TIMEOUT) as resp:
            logger.info("%s: namespace %s aangemaakt (HTTP %s)", PROVIDER_NAME, ns, resp.status)
        return True
    except urllib.error.HTTPError as exc:
        if exc.code in (400, 409):              # bestaat al: prima
            return True
        logger.warning("%s: kon namespace %s niet aanmaken (HTTP %s)", PROVIDER_NAME, ns, exc.code)
    except Exception as exc:
        logger.warning("%s: kon namespace %s niet aanmaken (%s)", PROVIDER_NAME, ns, exc)
    return False


# Grenzen waarop afkappen netjes is: zinseinde, regelovergang, puntkomma.
_BOUNDARY = re.compile(r"(?:[.!?]\s|\n\n|\n|;\s)")


def _trim_to_boundary(chunk: str, *, from_end: bool) -> str:
    """Snijd een chunk terug naar de dichtstbijzijnde nette grens.

    Neemt de LAATSTE grens voor de kop en de EERSTE voor de staart; zo valt er
    nooit een half woord of een halve zin in de opgeslagen herinnering.
    """
    best = None
    for m in _BOUNDARY.finditer(chunk):
        best = m
        if from_end:
            break
    if best is not None:
        return chunk[:best.end()] if not from_end else chunk[best.start():]
    space = chunk.rfind(" ") if not from_end else chunk.find(" ")
    if space <= 0:
        return chunk
    return chunk[:space] if not from_end else chunk[space + 1:]


def _clip(text: str, cap: int) -> str:
    """Kap af op een nette grens; bewaar kop EN staart.

    cap <= 0 betekent: niet afkappen. Bij afkappen krijgt de kop 60% en de
    staart 40% van het budget, want de conclusie van een antwoord staat
    doorgaans aan het eind. Een open code-blok wordt gesloten, zodat een
    extractor geen onafgesloten markdown te verwerken krijgt.
    """
    if cap <= 0 or len(text) <= cap:
        return text
    marker = " …[afgekapt]… "
    budget = max(cap - len(marker), 40)
    head_len = int(budget * 0.6)
    tail_len = budget - head_len

    head = _trim_to_boundary(text[: head_len + 1], from_end=False)
    tail = _trim_to_boundary(text[len(text) - tail_len - 1:], from_end=True)
    out = (head.rstrip() + marker + tail.lstrip()).strip()
    if out.count("```") % 2:
        out += "\n```"
    return out


def _token(home: str | Path | None = None) -> str:
    """CORTEXIQ_API_TOKEN: eerst de (profiel-scoped) omgeving, dan <home>/.env.

    De cache is per home: één proces kan meerdere profielen bedienen, en dan mag
    het token van profiel A niet voor profiel B blijven hangen.
    """
    tok = _env("CORTEXIQ_API_TOKEN")
    if tok:
        return tok
    base = _home(home)
    key = str(base)
    if key not in _TOKEN_CACHE:
        found = ""
        try:
            for line in (base / ".env").read_text(encoding="utf-8").splitlines():
                if line.startswith("CORTEXIQ_API_TOKEN="):
                    found = line.split("=", 1)[1].strip().strip('"').strip("'")
                    break
        except OSError:
            pass
        _TOKEN_CACHE[key] = found
    return _TOKEN_CACHE[key]


def _post(path: str, payload: dict[str, Any], *, timeout: float,
          home: str | Path | None = None) -> dict[str, Any]:
    base = _setting("api_url", _DEFAULT_API, home=home).rstrip("/")
    req = urllib.request.Request(
        f"{base}{path}",
        data=json.dumps(payload).encode(),
        method="POST",
        headers={
            "Authorization": f"Bearer {_token()}",
            "Content-Type": "application/json",
            "Accept": "application/json",
        },
    )
    with urllib.request.urlopen(req, timeout=timeout) as resp:
        body = resp.read()
    return json.loads(body or b"{}")


class CortexIQProvider(MemoryProvider):
    """Zie agent/memory_provider.py voor het volledige contract."""

    def __init__(self) -> None:
        # TRIVIAAL HOUDEN. __init__ draait ook tijdens discovery, buiten een sessie.
        # Een exception hier = Hermes gooit de provider stilzwijgend uit de lijst.
        self._home: Optional[Path] = None
        self._session_id = ""
        self._agent_context = "primary"
        self._cache = ""
        self._cache_count = 0
        self._ingest_ok = True   # uitgezet zodra het ingest-endpoint ontbreekt
        # Eenmalige waarschuwingen: een mislukte write of recall mag niet in de
        # DEBUG-ruis verdwijnen — dan verlies je stil je data.
        self._ingest_warned = False
        self._recall_warned = False

    # -- identiteit ---------------------------------------------------------

    @property
    def name(self) -> str:
        return PROVIDER_NAME

    def identity_signature(self) -> dict[str, Any]:
        return {}

    # -- levensloop ---------------------------------------------------------

    def is_available(self) -> bool:
        """Alleen config checken — geen netwerk. Zonder token heeft activeren geen zin."""
        return bool(_token())

    def unavailable_reason(self) -> str:
        if _token():
            return ""
        return "CORTEXIQ_API_TOKEN ontbreekt — zet hem in ~/.hermes/.env"

    def initialize(self, session_id: str, **kwargs) -> None:
        home = kwargs.get("hermes_home") or _env("HERMES_HOME") or str(Path.home() / ".hermes")
        self._home = Path(home)
        self._session_id = session_id or ""
        self._agent_context = kwargs.get("agent_context") or "primary"
        scope = _setting("recall_scope", "global", home=self._home)
        logger.info("%s: gestart (home=%s, profiel=%s, context=%s, recall=%s%s, capture=%s)",
                    PROVIDER_NAME, self._home, _profile(self._home), self._agent_context,
                    scope, f" {_recall_namespaces(self._home)}" if _recall_namespaces(self._home) else " (alles)",
                    _ingest_namespace(self._home))
        if _ingest_namespace(self._home) == "/":
            logger.warning("%s: capture-namespace staat op '/' en daar kan niet in geschreven "
                           "worden ('/' is de root, geen namespace-rij -> remember geeft 400). "
                           "Recall blijft werken; capture staat uit tot je een bestaande namespace "
                           "opgeeft, bijv. CORTEXIQ_INGEST_NAMESPACE=/hermes, of zet \"ingest_namespace\" in cortexiq.json.",
                           PROVIDER_NAME)

    def shutdown(self) -> None:
        return

    # -- prompt & recall ----------------------------------------------------

    def system_prompt_block(self) -> str:
        return ""

    def prefetch(self, query: str, *, session_id: str = "") -> str:
        """MOET snel zijn: alleen de cache teruggeven die queue_prefetch() vulde."""
        return self._cache

    def queue_prefetch(self, query: str, *, session_id: str = "") -> None:
        """Plan de recall ná deze turn, in de achtergrond."""
        if not query or not _token():
            return
        if not self._session_id and session_id:
            self._session_id = session_id
        spawn_context_thread(self._recall_into_cache, name=f"{PROVIDER_NAME}-prefetch",
                             args=(query,)).start()

    def recall_status(self) -> Optional[RecallStatus]:
        if not self._cache:
            return None
        return RecallStatus(provider_label="CortexIQ", count=self._cache_count)

    def _recall_into_cache(self, query: str) -> None:
        try:
            rows = self._search(query)
        except Exception as exc:            # nooit een turn laten klappen
            if not self._recall_warned:
                self._recall_warned = True
                logger.warning("%s: recall mislukt (%s) — controleer CORTEXIQ_API_URL/"
                               "CORTEXIQ_NAMESPACE en het token. Verdere fouten blijven stil.",
                               PROVIDER_NAME, exc)
            return
        if not rows:
            self._cache, self._cache_count = "", 0
            return
        max_chars = int(_setting("max_chars", "1400", home=self._home))
        lines, used, kept = [], 0, 0
        for content in rows:
            if used + len(content) > max_chars:
                break
            lines.append(f"- {content}")
            used += len(content)
            kept += 1
        if not lines:
            self._cache, self._cache_count = "", 0
            return
        self._cache = "Uit mijn geheugen:\n" + "\n".join(lines)
        self._cache_count = kept

    # -- opslag -------------------------------------------------------------

    def sync_turn(self, user_content: str, assistant_content: str, *,
                  session_id: str = "", messages: Optional[list[dict[str, Any]]] = None,
                  turn_author: Optional[dict[str, Any]] = None) -> None:
        """Capture de turn als episode. Non-blocking; niet-primary contexten overslaan.

        Standaard gaat alleen het gebruikersbericht mee. Met
        CORTEXIQ_CAPTURE_ASSISTANT=true gaat het antwoord er ook in (afgekapt op
        CORTEXIQ_ASSISTANT_MAX_CHARS) — anders is elke herinnering een vraag
        zonder antwoord, en kun je later wel terugvinden wat er gevraagd is maar
        niet wat er geconcludeerd is. Beide kanten krijgen een rol-label, zodat
        de extractor ze kan onderscheiden.
        """
        if self._agent_context != "primary" or not user_content or not self._ingest_ok:
            return

        content = user_content
        if _truthy(_setting("capture_assistant", "false", home=self._home)):
            answer = (assistant_content or "").strip()
            if answer:
                # 0 = geen aparte assistant-grens; de harde episode-grens hieronder
                # is dan de enige rem.
                answer = _clip(answer, int(_setting("assistant_max_chars", "0", home=self._home)))
                content = f"user: {user_content}\n\nassistant: {answer}"

        content = _clip(content, int(_setting("capture_max_chars", "4000", home=self._home)))

        spawn_context_thread(self._store, name=f"{PROVIDER_NAME}-sync",
                             args=(content, "turn", session_id or self._session_id)).start()

    def on_session_switch(self, new_session_id: str, *, parent_session_id: str = "",
                          reset: bool = False, rewound: bool = False, **kwargs) -> None:
        self._session_id = new_session_id or ""
        if reset:
            self._cache, self._cache_count = "", 0

    def on_pre_compress(self, messages: list[dict[str, Any]]) -> str:
        return ""

    def on_memory_write(self, action: str, target: str, content: str,
                        metadata: Optional[dict[str, Any]] = None) -> None:
        """Spiegel writes van het ingebouwde memory-tool naar CortexIQ."""
        if action == "add" and content and self._ingest_ok:
            spawn_context_thread(self._store, name=f"{PROVIDER_NAME}-mirror",
                                 args=(content, f"builtin:{target}", self._session_id)).start()

    def backup_paths(self) -> list[str]:
        """Wij hebben geen state buiten HERMES_HOME — CortexIQ zelf staat in Postgres.
        Let op: `hermes backup` dekt die database dus NIET. Gebruik je eigen
        scripts/backup-prod.sh, of exporteer periodiek naar JSONL in de home."""
        return []

    # -- tools --------------------------------------------------------------

    def get_tool_schemas(self) -> list[dict[str, Any]]:
        """Bewust LEEG.

        Hermes heeft de 29 CortexIQ-tools al via de MCP-koppeling
        (`mcp_cortexiq_*`). Hier nog eens `recall`/`remember` declareren zet elke
        tool-definitie dubbel in elke request — tokenkosten per turn, precies
        waar je zuinig wilt zijn. Deze plugin doet alleen de automatische lus.
        """
        return []

    def handle_tool_call(self, tool_name: str, args: dict[str, Any], **kwargs) -> str:
        return json.dumps({"error": f"cortexiq-provider heeft geen tools: {tool_name}"})

    def get_config_schema(self) -> list[dict[str, Any]]:
        """Leeg: alles gaat via omgevingsvariabelen (zie de module-docstring)."""
        return []

    # -- transport ----------------------------------------------------------

    def _search(self, query: str, *, limit: Optional[int] = None) -> list[str]:
        limit = limit or int(_setting("recall_limit", "5", home=self._home))
        payload: dict[str, Any] = {"query": query, "limit": limit}
        namespaces = _recall_namespaces(self._home)
        if namespaces:
            payload["namespaces"] = namespaces

        data = _post(_RECALL_PATH, payload, timeout=_RECALL_TIMEOUT, home=self._home)
        rows = data.get("results") or []

        # WAAROM OP `score` FILTEREN EN NOOIT OP `similarity`:
        #   score      = RRF-rangfusie (hybrid recall staat default aan),
        #                genormaliseerd naar 0..1 — rangpositie is gekalibreerd,
        #                dus dit scheidt relevant van ruis.
        #   similarity = rauwe cosine `1 - (embedding <=> query)`. Cosine van een
        #                embedding-model is NIET gekalibreerd, en bij een
        #                keyword-only treffer is het veld zelfs helemaal afwezig
        #                (`match: "keyword"`, gemeten: 0/6 rijen had het veld).
        #                Een similarity-drempel gooit dus juist exacte-term-treffers
        #                weg die hoog scoren — precies de verkeerde fout.
        #
        # Ontbreekt `score` (geen numerieke waarde), dan houden we de rij: liever
        # een keertje extra dan een exacte treffer weggooien.
        min_score = float(_setting("min_score", "0.6", home=self._home))
        out: list[str] = []
        for row in rows:
            content = str(row.get("content") or "").strip()
            if not content:
                continue
            score = row.get("score")
            if isinstance(score, (int, float)) and score < min_score:
                continue        # drempel: liever niets dan ruis
            out.append(content)
        return out

    def _store(self, content: str, source: str, session_id: str) -> None:
        """POST naar het ingest-endpoint. Vorm volgt de MCP-tool `remember`:
        {content, namespace, importance}. Onbekende velden sturen we niet mee, en
        `source`/`session_id` laten we voorlopig ongebruikt — daar is nog geen veld voor."""
        if not self._ingest_ok:
            return
        payload = {
            "content": content,
            "namespace": _ingest_namespace(self._home),
            "importance": float(_setting("turn_importance", "0.5", home=self._home)),
        }
        path = _setting("ingest_path", "/api/v1/remember", home=self._home)
        try:
            _post(path, payload, timeout=_INGEST_TIMEOUT, home=self._home)
        except urllib.error.HTTPError as exc:
            try:
                detail = exc.read()[:200].decode(errors="replace").strip()
            except Exception:
                detail = ""
            if exc.code in (404, 405):
                self._ingest_ok = False
                logger.info("%s: geen ingest-endpoint op %s (HTTP %s) — capture staat uit "
                            "tot je die route toevoegt. Recall blijft werken.",
                            PROVIDER_NAME, path, exc.code)
            elif exc.code == 400 and "namespace not found" in detail.lower():
                # Per-profiel namespaces kun je niet vooraf voor elk profiel
                # aanmaken; doe het bij de eerste write en probeer één keer opnieuw.
                if _ensure_namespace(payload["namespace"]):
                    try:
                        _post(path, payload, timeout=_INGEST_TIMEOUT, home=self._home)
                        logger.info("%s: namespace %s aangemaakt en eerste capture gelukt",
                                    PROVIDER_NAME, payload["namespace"])
                        return
                    except Exception as retry_exc:
                        detail = f"{detail} | retry: {retry_exc}"
            if not self._ingest_warned:
                # Bijv. 500 door een niet-bestaande namespace: dat mag niet stil zijn.
                self._ingest_warned = True
                logger.warning("%s: capture faalt (HTTP %s) %s — maak de namespace aan "
                               "(POST /api/v1/namespaces) of zet CORTEXIQ_INGEST_NAMESPACE goed. "
                               "Verdere fouten blijven stil.",
                               PROVIDER_NAME, exc.code, detail)
        except Exception as exc:
            if not self._ingest_warned:
                self._ingest_warned = True
                logger.warning("%s: capture faalt (%s). Verdere fouten blijven stil.",
                               PROVIDER_NAME, exc)


def register(ctx) -> None:
    """De loader importeert dit en vangt ctx.register_memory_provider() op."""
    ctx.register_memory_provider(CortexIQProvider())
