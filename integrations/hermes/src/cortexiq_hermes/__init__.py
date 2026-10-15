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
  CORTEXIQ_NAMESPACE       default "/"   recall-scope; '/' = alle namespaces
  CORTEXIQ_INGEST_NAMESPACE default = CORTEXIQ_NAMESPACE; moet BESTAAN om te kunnen schrijven
  CORTEXIQ_RECALL_LIMIT  default 5
  CORTEXIQ_MIN_SCORE     default 0.6    (gemeten: ruis plafonneert ~0.43,
                                         relevante treffers beginnen ~0.70)
  CORTEXIQ_MAX_CHARS     default 1400   (cap op wat er per turn in de prompt gaat)
  CORTEXIQ_INGEST_PATH   default /api/v1/remember
"""

from __future__ import annotations

import json
import logging
import os
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
    return (os.environ.get(name) or "").strip() or default


def _token() -> str:
    """CORTEXIQ_API_TOKEN uit de omgeving, anders uit $HERMES_HOME/.env."""
    if "v" in _TOKEN_CACHE:
        return _TOKEN_CACHE["v"]
    tok = _env("CORTEXIQ_API_TOKEN")
    if not tok:
        home = Path(_env("HERMES_HOME") or str(Path.home() / ".hermes"))
        try:
            for line in (home / ".env").read_text(encoding="utf-8").splitlines():
                if line.startswith("CORTEXIQ_API_TOKEN="):
                    tok = line.split("=", 1)[1].strip().strip('"').strip("'")
                    break
        except OSError:
            pass
    _TOKEN_CACHE["v"] = tok
    return tok


def _namespace() -> str:
    """Namespace, genormaliseerd naar een leading '/'.

    CortexIQ weigert paden zonder leading slash ("brain: namespace path must
    start with /"), en zo'n fout komt als HTTP 500 terug. Normaliseren hier
    voorkomt dat een typefout in .env stil elke write laat mislukken.
    """
    ns = _env("CORTEXIQ_NAMESPACE", "/").strip()
    if not ns.startswith("/"):
        ns = "/" + ns
    return ns or "/"


def _ingest_namespace() -> str:
    """Namespace voor capture — moet een BESTAAND namespace zijn.

    Recall mag '/': dat is de root en betekent "zoek in alles". Schrijven kan
    daar niet, want '/' heeft geen rij in de namespaces-tabel; remember geeft
    dan 400 "namespace not found". Vandaar een eigen variabele, met
    CORTEXIQ_NAMESPACE als terugval. Zo kun je globaal blijven lezen terwijl
    capture naar een concreet namespace gaat.
    """
    ns = (_env("CORTEXIQ_INGEST_NAMESPACE", "").strip()
          or _env("CORTEXIQ_NAMESPACE", "/"))
    if not ns.startswith("/"):
        ns = "/" + ns
    return ns or "/"


def _post(path: str, payload: dict[str, Any], *, timeout: float) -> dict[str, Any]:
    base = _env("CORTEXIQ_API_URL", _DEFAULT_API).rstrip("/")
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
        logger.info("%s: gestart (home=%s, context=%s, recall-namespace=%s, capture-namespace=%s)",
                    PROVIDER_NAME, self._home, self._agent_context,
                    _namespace(), _ingest_namespace())
        if _ingest_namespace() == "/":
            logger.warning("%s: capture-namespace staat op '/' en daar kan niet in geschreven "
                           "worden ('/' is de root, geen namespace-rij -> remember geeft 400). "
                           "Recall blijft werken; capture staat uit tot je een bestaande namespace "
                           "opgeeft, bijv. CORTEXIQ_INGEST_NAMESPACE=/hermes.",
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
        max_chars = int(_env("CORTEXIQ_MAX_CHARS", "1400"))
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
        """Capture de turn als episode. Non-blocking; niet-primary contexten overslaan."""
        if self._agent_context != "primary" or not user_content or not self._ingest_ok:
            return
        spawn_context_thread(self._store, name=f"{PROVIDER_NAME}-sync",
                             args=(user_content, "turn", session_id or self._session_id)).start()

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
        limit = limit or int(_env("CORTEXIQ_RECALL_LIMIT", "5"))
        namespace = _namespace()
        payload: dict[str, Any] = {"query": query, "limit": limit}
        if namespace != "/":
            payload["namespaces"] = [namespace]

        data = _post(_RECALL_PATH, payload, timeout=_RECALL_TIMEOUT)
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
        min_score = float(_env("CORTEXIQ_MIN_SCORE", "0.6"))
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
            "namespace": _ingest_namespace(),
            "importance": float(_env("CORTEXIQ_TURN_IMPORTANCE", "0.5")),
        }
        path = _env("CORTEXIQ_INGEST_PATH", "/api/v1/remember")
        try:
            _post(path, payload, timeout=_INGEST_TIMEOUT)
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
            elif not self._ingest_warned:
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
