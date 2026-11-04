"""CortexIQ Setup plugin backend (dashboard + desktop).

Mounted by Hermes at /api/plugins/cortexiq-setup/.

Installatie- en statusbackend voor de CortexIQ geheugen-provider:
providercheck, configuratie (cortexiq.json), token (in <home>/.env), activering
(memory.provider via `hermes config set`), verbindingstest en namespace-mgmt.
Bewust geen lees/doorzoek-endpoints: daarvoor bestaat hermes-memory-ui.

Het contract hieronder spiegel de provider in
integrations/hermes/src/cortexiq_hermes/__init__.py:
- config: <home>/cortexiq.json (of <home>/cortexiq/config.json) > env > default
- env-spiegels: CORTEXIQ_API_URL, CORTEXIQ_NAMESPACE_BASE, CORTEXIQ_RECALL_SCOPE, ...
- token: CORTEXIQ_API_TOKEN via profiel-scoped secret scope, dan <home>/.env;
  de token-waarde verlaat dit bestand nooit.
"""
from __future__ import annotations

import asyncio
import importlib.metadata
import importlib.util
import json
import os
import shutil
import subprocess
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path
from typing import Any, Dict, List, Optional

try:
    from fastapi import APIRouter, Depends, HTTPException, Query
except Exception:  # Maakt lokale import/syntax-tests buiten het dashboard mogelijk.
    class APIRouter:  # type: ignore
        def __init__(self, *_args, **_kwargs):
            pass

        def get(self, *_args, **_kwargs):
            return lambda fn: fn

        def post(self, *_args, **_kwargs):
            return lambda fn: fn

    def Depends(dependency=None, **_kwargs):  # type: ignore
        return dependency

    class HTTPException(Exception):  # type: ignore
        def __init__(self, status_code: int, detail: str):
            super().__init__(detail)
            self.status_code = status_code
            self.detail = detail

    def Query(default=None, **_kwargs):  # type: ignore
        return default


async def _profile_request_scope(profile: Optional[str] = Query(None)):
    """Bind één plugin-request aan het expliciet gekozen Hermes-profiel.

    Het dashboard plaatst ``/api/plugins`` niet zelf in een profiel-scope;
    Desktop voegt het actieve profiel al toe aan ``ctx.rest``. Beide hosts
    accepteren dezelfde query-contract en delegeren de validatie aan Hermes'
    context-lokale request-scope. Oude single-profiel-versies blijven werken
    zonder (of met ``current``); een expliciet profiel faalt afgesloten als de
    veilige host-helper ontbreekt.
    """
    requested = (profile or "").strip()
    try:
        from hermes_cli.web_server_profiles import _config_profile_scope
    except (ImportError, AttributeError) as exc:
        if requested and requested.lower() != "current":
            raise HTTPException(
                status_code=501,
                detail="Deze Hermes-versie kan plugin-requests niet veilig naar een ander profiel routeren.",
            ) from exc
        yield
        return

    with _config_profile_scope(requested or None):
        yield


router = APIRouter(dependencies=[Depends(_profile_request_scope)])

PLUGIN_VERSION = "0.1.0"
PROVIDER_ID = "cortexiq"
PROVIDER_PACKAGE = "cortexiq_hermes"
PROVIDER_DISTRIBUTION = "cortexiq-hermes"

CORTEXIQ_DEFAULT_API_URL = "http://127.0.0.1:18101"
CORTEXIQ_STATS_PATH = "/api/v1/stats"
CORTEXIQ_NAMESPACES_PATH = "/api/v1/namespaces"
CORTEXIQ_FETCH_TIMEOUT = 6.0

CORTEXIQ_ENV_KEYS: Dict[str, str] = {
    "api_url": "CORTEXIQ_API_URL",
    "base_namespace": "CORTEXIQ_NAMESPACE_BASE",
    "namespace": "CORTEXIQ_NAMESPACE",
    "ingest_namespace": "CORTEXIQ_INGEST_NAMESPACE",
    "ingest_path": "CORTEXIQ_INGEST_PATH",
    "recall_scope": "CORTEXIQ_RECALL_SCOPE",
    "recall_limit": "CORTEXIQ_RECALL_LIMIT",
    "min_score": "CORTEXIQ_MIN_SCORE",
    "max_chars": "CORTEXIQ_MAX_CHARS",
    "ground_truth": "CORTEXIQ_GROUND_TRUTH",
    "turn_importance": "CORTEXIQ_TURN_IMPORTANCE",
    "capture_assistant": "CORTEXIQ_CAPTURE_ASSISTANT",
    "assistant_max_chars": "CORTEXIQ_ASSISTANT_MAX_CHARS",
    "capture_max_chars": "CORTEXIQ_CAPTURE_MAX_CHARS",
}

# Editabare sleutels en hun volg-orde/type voor de configuratie-editor.
CONFIG_INT_KEYS = ("recall_limit", "max_chars", "assistant_max_chars", "capture_max_chars")
CONFIG_FLOAT_KEYS = ("min_score", "turn_importance")
CONFIG_BOOL_KEYS = ("ground_truth", "capture_assistant")
CONFIG_STR_KEYS = ("api_url", "base_namespace", "namespace", "ingest_namespace",
                   "ingest_path", "recall_scope")
CONFIG_ALL_KEYS = CONFIG_STR_KEYS + CONFIG_INT_KEYS + CONFIG_FLOAT_KEYS + CONFIG_BOOL_KEYS


def _safe_error(exc: Any) -> str:
    text = str(exc).strip() or "onbekende fout"
    return text[:300]


def _hermes_home() -> Path:
    """Actieve Hermes-home, rekening houdend met profielen indien beschikbaar."""
    try:
        from hermes_constants import get_hermes_home
        return Path(get_hermes_home()).expanduser()
    except Exception:
        return Path(os.environ.get("HERMES_HOME", "~/.hermes")).expanduser()


def _read_yaml(path: Path) -> Dict[str, Any]:
    if not path.exists():
        return {}
    try:
        try:
            import yaml
        except ImportError:
            import hermes_yaml as yaml
        data = yaml.safe_load(path.read_text(encoding="utf-8")) or {}
        return data if isinstance(data, dict) else {}
    except Exception:
        return {}


def _read_json(path: Path) -> Dict[str, Any]:
    if not path.exists():
        return {}
    try:
        data = json.loads(path.read_text(encoding="utf-8")) or {}
        return data if isinstance(data, dict) else {}
    except Exception:
        return {}


def _load_simple_env_file(path: Path) -> Dict[str, str]:
    values: Dict[str, str] = {}
    if not path.exists():
        return values
    try:
        for line in path.read_text(encoding="utf-8").splitlines():
            text = line.strip()
            if text.startswith("export "):
                text = text[7:].strip()
            if not text or text.startswith("#") or "=" not in text:
                continue
            key, value = text.split("=", 1)
            values[key.strip()] = value.strip().strip('"').strip("'")
    except Exception:
        return {}
    return values


def _env_value(key: str, default: str = "") -> str:
    """Lees een env-waarde uit de (profiel-scoped) secret scope van Hermes.

    ``os.environ`` hoort bij het Dashboard-proces zelf en mag nooit winnen
    wanneer het request naar een ander profiel routeert. Het lokale
    .env-formaat blijft als fallback voor oudere single-profiel-hosts.
    """
    try:
        from agent.secret_scope import get_secret
    except ImportError:
        value = os.environ.get(key)
        if value not in (None, ""):
            return str(value)
        return _load_simple_env_file(_hermes_home() / ".env").get(key, default)
    value = get_secret(key, None)
    return default if value in (None, "") else str(value)


def _truthy(value: Any, default: bool = False) -> bool:
    if isinstance(value, bool):
        return value
    if value in (None, ""):
        return default
    if isinstance(value, str):
        return value.strip().lower() not in {"0", "false", "no", "off"}
    return bool(value)


def _coerce_bool(value: Any, default: bool = False) -> bool:
    if value in (None, ""):
        return default
    if isinstance(value, bool):
        return value
    if isinstance(value, (int, float)):
        return bool(value)
    return _truthy(str(value))


def _coerce_int(value: Any, default: int, lo: int = 0, hi: int = 10 ** 9) -> int:
    try:
        return min(hi, max(lo, int(float(value))))
    except Exception:
        return default


def _coerce_float(value: Any, default: float, lo: float = 0.0, hi: float = 1.0) -> float:
    try:
        return min(hi, max(lo, float(value)))
    except Exception:
        return default


def _as_int(value: Any, default: int) -> int:
    try:
        return int(float(value))
    except Exception:
        return default


def _as_float(value: Any, default: float) -> float:
    try:
        return float(value)
    except Exception:
        return default


def _cortexiq_config_path(home: Optional[Path] = None) -> Path:
    """Eerste bestaande candidate wint, anders cortexiq.json (provider-contract)."""
    home = home if home is not None else _hermes_home()
    candidates = [home / "cortexiq.json", home / "cortexiq" / "config.json"]
    for candidate in candidates:
        if candidate.exists():
            return candidate
    return candidates[0]


def _namespace_fixup(value: str) -> str:
    value = (value or "").strip()
    if value and not value.startswith("/"):
        value = "/" + value
    return value


def _cortexiq_token() -> str:
    """Lees de API-token op aanvraag; sla hem nooit op of geef hem terug."""
    return _env_value("CORTEXIQ_API_TOKEN")


def _load_cortexiq_config() -> Dict[str, Any]:
    """Niet-geheime CortexIQ-instellingen volgens het provider-contract.

    Spiegel van het provider-gedrag: cortexiq.json wint over de
    profiel-scoped omgeving, die over de defaults. De token-sleutel
    ``CORTEXIQ_API_TOKEN`` wordt nooit geretourneerd, alleen als
    boolean ``token_present`` gemeld.
    """
    home = _hermes_home()
    config_path = _cortexiq_config_path(home)
    file_cfg = _read_json(config_path)
    profile = home.name if home.parent.name == "profiles" and home.name else "default"

    def setting(key: str, default: str = "") -> str:
        value = file_cfg.get(key)
        if isinstance(value, bool):
            return "true" if value else "false"
        if isinstance(value, (int, float)):
            return str(value)
        if isinstance(value, str) and value.strip():
            return value.strip()
        env_name = CORTEXIQ_ENV_KEYS.get(key)
        if env_name:
            return _env_value(env_name, default)
        return default

    api_url = (setting("api_url", CORTEXIQ_DEFAULT_API_URL) or CORTEXIQ_DEFAULT_API_URL).rstrip("/")
    base_namespace = "//" + setting("base_namespace", "/hermes").lstrip("/")
    single_namespace = setting("namespace", "").strip()
    explicit_namespaces_raw = file_cfg.get("namespaces")
    explicit_namespaces = (
        [str(entry) for entry in explicit_namespaces_raw]
        if isinstance(explicit_namespaces_raw, list) and explicit_namespaces_raw
        else []
    )
    recall_scope = (setting("recall_scope", "global") or "global").strip().lower()

    # Spiegel de recall-scope-resolutie van de provider:
    # expliciete namespaces > één namespace > scope.
    if explicit_namespaces:
        recall_namespaces: Optional[List[str]] = explicit_namespaces
    elif single_namespace and single_namespace != "/":
        recall_namespaces = [single_namespace]
    elif recall_scope == "profile":
        recall_namespaces = [f"{base_namespace}/{profile}"]
    elif recall_scope == "base":
        recall_namespaces = [base_namespace]
    else:
        recall_namespaces = None

    ingest_namespace = setting("ingest_namespace", "").strip() or f"{base_namespace}/{profile}"
    ingest_namespace = _namespace_fixup(ingest_namespace)
    ingest_path = setting("ingest_path", "/api/v1/remember") or "/api/v1/remember"
    if not ingest_path.startswith("/"):
        ingest_path = "/" + ingest_path

    return {
        "config_path": str(config_path),
        "config_exists": any(
            (home / p).exists() for p in ("cortexiq.json", os.path.join("cortexiq", "config.json"))
        ),
        "profile": profile,
        "hermes_home": str(home),
        "api_url": api_url,
        "token_present": bool(_cortexiq_token()),
        "recall_scope": recall_scope,
        "recall_namespaces": recall_namespaces,
        "base_namespace": base_namespace,
        "namespace": single_namespace,
        "namespaces_configured": explicit_namespaces,
        "ingest_namespace": ingest_namespace,
        "ingest_path": ingest_path,
        "recall_limit": max(1, min(_as_int(setting("recall_limit", "5"), 5), 100)),
        "min_score": _as_float(setting("min_score", "0.6"), 0.6),
        "max_chars": max(0, _as_int(setting("max_chars", "1400"), 1400)),
        "ground_truth": _truthy(setting("ground_truth", "true"), default=True),
        "turn_importance": _as_float(setting("turn_importance", "0.5"), 0.5),
        "capture_assistant": _truthy(setting("capture_assistant", "false")),
        "assistant_max_chars": max(0, _as_int(setting("assistant_max_chars", "0"), 0)),
        "capture_max_chars": max(0, _as_int(setting("capture_max_chars", "4000"), 4000)),
    }


def _provider_selected(cfg: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:
    """memory.provider zoals Hermes hem leest (config.yaml)."""
    config = cfg if cfg is not None else _read_yaml(_hermes_home() / "config.yaml")
    memory_cfg = config.get("memory", {}) if isinstance(config.get("memory"), dict) else {}
    provider = str(memory_cfg.get("provider", "") or "").strip()
    return {"provider": provider, "is_cortexiq": provider == PROVIDER_ID}


def _provider_package() -> Dict[str, Any]:
    available = importlib.util.find_spec(PROVIDER_PACKAGE) is not None
    version = None
    if available:
        try:
            version = importlib.metadata.version(PROVIDER_DISTRIBUTION)
        except Exception:
            version = None
    return {"installed": available, "version": version}


def _hermes_binary() -> Optional[str]:
    found = shutil.which("hermes")
    if found:
        return found
    home = _hermes_home()
    for suffix in (".cmd", ".exe", ".bat", ""):
        candidate = home / "bin" / f"hermes{suffix}"
        if candidate.exists():
            return str(candidate)
    return None


def _namespaces_ready(cfg: Dict[str, Any], live_namespaces: List[str]) -> Dict[str, Any]:
    """Welke namespaces moet de provider hebben en welke ontbreken er."""
    existing = [str(entry) for entry in (live_namespaces or [])]
    wanted = list(cfg.get("recall_namespaces") or [])
    ingest = cfg.get("ingest_namespace")
    if ingest and ingest not in wanted:
        wanted.append(ingest)
    existing_set = {e.lstrip("/") for e in existing}
    missing = [ns for ns in wanted if ns and ns.lstrip("/") not in existing_set]
    return {
        "existing": existing,
        "wanted": wanted,
        "missing": missing,
        "ingest_ready": bool(ingest) and ingest.lstrip("/") in existing_set,
    }


async def _cortexiq_live_check() -> Dict[str, Any]:
    """Eén live API-call (GET /api/v1/stats) + GET namespaces om schema's te checken."""
    loop = asyncio.get_running_loop()

    def _run() -> Dict[str, Any]:
        started = time.monotonic()
        stats = _cortexiq_request("GET", CORTEXIQ_STATS_PATH)
        latency_ms = round((time.monotonic() - started) * 1000.0, 1)
        namespaces: List[str] = []
        if stats.get("ok"):
            namespaces_result = _cortexiq_request("GET", CORTEXIQ_NAMESPACES_PATH)
            if namespaces_result.get("ok"):
                data = namespaces_result.get("data")
                if isinstance(data, list):
                    for entry in data:
                        if isinstance(entry, dict):
                            namespaces.append(str(entry.get("slug") or entry.get("name") or ""))
                        elif entry:
                            namespaces.append(str(entry))
                elif isinstance(data, dict):
                    for entry in data.get("namespaces") or []:
                        if isinstance(entry, dict):
                            namespaces.append(str(entry.get("slug") or entry.get("name") or ""))
                        elif entry:
                            namespaces.append(str(entry))
        return {"stats": stats, "latency_ms": latency_ms, "namespaces": namespaces}

    return await loop.run_in_executor(None, _run)


def _cortexiq_request(
    method: str,
    path: str,
    cfg: Optional[Dict[str, Any]] = None,
    *,
    payload: Optional[Dict[str, Any]] = None,
    timeout: float = CORTEXIQ_FETCH_TIMEOUT,
) -> Dict[str, Any]:
    """Eén CortexIQ HTTP-call via urllib. Retourneert {ok, error, data}."""
    token = _cortexiq_token()
    if not token:
        return {
            "ok": False,
            "error": "CORTEXIQ_API_TOKEN ontbreekt voor het actieve profiel.",
            "data": None,
        }
    base = str((cfg or _load_cortexiq_config()).get("api_url") or CORTEXIQ_DEFAULT_API_URL).rstrip("/")
    url = f"{base}{path}"
    body = None
    headers: Dict[str, str] = {
        "Authorization": f"Bearer {token}",
        "Accept": "application/json",
    }
    if payload is not None:
        body = json.dumps(payload).encode("utf-8")
        headers["Content-Type"] = "application/json"
    try:
        request_ = urllib.request.Request(url, data=body, method=method, headers=headers)
        with urllib.request.urlopen(request_, timeout=timeout) as response:
            raw = response.read()
    except urllib.error.HTTPError as exc:
        detail = ""
        try:
            detail = exc.read()[:300].decode("utf-8", errors="replace").strip()
        except Exception:
            pass
        message = f"CortexIQ API gaf HTTP {exc.code} voor {path}"
        if detail:
            message += f": {detail}"
        return {"ok": False, "error": _safe_error(message), "data": None}
    except Exception as exc:
        return {"ok": False, "error": _safe_error(exc), "data": None}
    try:
        parsed = json.loads(raw or b"{}")
    except Exception:
        parsed = {"raw": (raw or b"").decode("utf-8", errors="replace")}
    return {"ok": True, "error": None, "data": parsed}


def _steps_package() -> Dict[str, Any]:
    pkg = _provider_package()
    return {
        "id": "provider_package",
        "label": "cortexiq-hermes package",
        "ok": pkg["installed"],
        "detail": (
            f"geïnstalleerd{', versie ' + pkg['version'] if pkg['version'] else ''}"
            if pkg["installed"]
            else "installeer met: pip install cortexiq-hermes"
        ),
    }


def _steps_provider() -> Dict[str, Any]:
    selected = _provider_selected()
    return {
        "id": "provider_selected",
        "label": "memory.provider = cortexiq",
        "ok": selected["is_cortexiq"],
        "detail": (
            "gezet in config.yaml"
            if selected["is_cortexiq"]
            else (f"gezet: {selected['provider']}" if selected["provider"] else "niet gezet")
        ),
    }


def _steps_token() -> Dict[str, Any]:
    present = bool(_cortexiq_token())
    return {
        "id": "token_present",
        "label": "CORTEXIQ_API_TOKEN",
        "ok": present,
        "detail": "aanwezig in profiel/.env (waarde wordt nooit getoond)"
        if present else "opslaan via het token-veld hieronder",
    }


def _steps_server(live: Optional[Dict[str, Any]]) -> Dict[str, Any]:
    if live is None:
        return {"id": "server_live", "label": "CortexIQ API", "ok": False, "detail": "nog niet gecontroleerd"}
    stats = live["stats"]
    if not stats.get("ok"):
        return {
            "id": "server_live",
            "label": "CortexIQ API",
            "ok": False,
            "detail": stats.get("error") or "server niet bereikbaar",
        }
    data = stats.get("data") if isinstance(stats.get("data"), dict) else {}
    counts = {
        key: data.get(key)
        for key in ("episodes", "facts", "namespaces", "revisions")
        if isinstance(data, dict) and data.get(key) is not None
    }
    summary = f"bereikbaar in {live['latency_ms']} ms"
    if counts:
        summary += " · " + ", ".join(f"{k}: {v}" for k, v in counts.items())
    return {"id": "server_live", "label": "CortexIQ API", "ok": True, "detail": summary}


def _steps_namespaces(live: Optional[Dict[str, Any]], cfg: Dict[str, Any]) -> Dict[str, Any]:
    if live is None or not live["stats"].get("ok"):
        return {"id": "namespaces_ready", "label": "Namespaces", "ok": False, "detail": "test eerst de verbinding"}
    readiness = _namespaces_ready(cfg, live["namespaces"])
    missing = readiness["missing"]
    return {
        "id": "namespaces_ready",
        "label": "Namespaces",
        "ok": not missing,
        "detail": (
            "alles aanwezig (ingest: " + str(cfg.get("ingest_namespace")) + ")"
            if not missing else "ontbreekt: " + ", ".join(missing)
        ),
        "missing": missing,
        "existing": readiness["existing"],
    }


# ---------------------------------------------------------------------------
# Routes
# ---------------------------------------------------------------------------

@router.get("/status")
async def status() -> Dict[str, Any]:
    """Installatie-checklist + live serverstatus in één aanroep."""
    cfg = _load_cortexiq_config()
    selected = _provider_selected()
    live = await _cortexiq_live_check() if cfg["token_present"] else None
    steps = [
        _steps_package(),
        _steps_provider(),
        _steps_token(),
        _steps_server(live),
        _steps_namespaces(live, cfg),
    ]
    return {
        "ok": all(step["ok"] for step in steps),
        "plugin_version": PLUGIN_VERSION,
        "hermes_home": cfg["hermes_home"],
        "profile": cfg["profile"],
        "config": cfg,
        "provider_selected": selected,
        "steps": steps,
        "server": (
            {"live": True, "latency_ms": live["latency_ms"], "stats": live["stats"].get("data"),
             "restart_needed_hint": False}
            if live and live["stats"].get("ok")
            else {"live": False, "error": (live or {}).get("stats", {}).get("error") if live else "token ontbreekt",
                  "restart_needed_hint": False}
        ),
    }


@router.get("/config")
async def get_config() -> Dict[str, Any]:
    """Volledige effectieve config voor de editor, inclusief het ruwe bestand."""
    home = _hermes_home()
    config_path = _cortexiq_config_path(home)
    raw = config_path.read_text(encoding="utf-8") if config_path.exists() else ""
    return {
        "config_path": str(config_path),
        "config_exists": config_path.exists(),
        "raw": raw,
        "effective": _load_cortexiq_config(),
        "provider_selected": _provider_selected(),
        "keys": list(CONFIG_ALL_KEYS),
    }


@router.post("/config")
async def save_config(payload: Dict[str, Any] = None) -> Dict[str, Any]:
    """Valideer en schrijf de bekende sleutels naar <home>/cortexiq.json.

    Onbekende sleutels (o.a. _comment, namespaces, ground_truth-varianten)
    worden precies behouden: het ruwe JSON-bestand wordt gelezen, alleen de
    bekende sleutelen worden overschreven, en het geheel atomair teruggeschreven.
    """
    payload = payload if isinstance(payload, dict) else {}
    settings = payload.get("settings")
    if not isinstance(settings, dict):
        raise HTTPException(status_code=400, detail="Verwacht {'settings': {...}} met bekende cortexiq-sleutels.")

    home = _hermes_home()
    target = home / "cortexiq.json"
    existing = _read_json(target)
    if not existing and (home / "cortexiq" / "config.json").exists():
        # Provider-contract: eerste bestaande candidate wint.
        first_existing = _cortexiq_config_path(home)
        if first_existing.exists():
            target = first_existing
            existing = _read_json(target)

    validated: Dict[str, Any] = {}

    for key in CONFIG_STR_KEYS:
        if key not in settings:
            continue
        value = str(settings.get(key) or "").strip()
        if key == "api_url":
            if value and not value.startswith(("http://", "https://")):
                raise HTTPException(status_code=400, detail="api_url moet met http:// of https:// beginnen.")
        elif key == "base_namespace" and value:
            value = _namespace_fixup(value.lstrip("/"))
        elif key in ("namespace", "ingest_namespace") and value:
            value = _namespace_fixup(value)
        elif key == "ingest_path" and value:
            if not value.startswith("/"):
                value = "/" + value
        elif key == "recall_scope" and value and value.lower() not in ("global", "base", "profile"):
            raise HTTPException(status_code=400, detail=f"recall_scope moet global, base of profile zijn (kreeg: {value!r}).")
        validated[key] = value

    for key in CONFIG_INT_KEYS:
        if key not in settings:
            continue
        raw = settings.get(key)
        try:
            lo, hi = (1, 100) if key == "recall_limit" else (0, 10 ** 9)
            validated[key] = min(hi, max(lo, int(float(raw))))
        except (TypeError, ValueError):
            raise HTTPException(
                status_code=400,
                detail=f"{key} is geen geldig geheel getal (kreeg: {raw!r}).",
            )

    for key in CONFIG_FLOAT_KEYS:
        if key not in settings:
            continue
        raw = settings.get(key)
        try:
            validated[key] = min(1.0, max(0.0, float(raw)))
        except (TypeError, ValueError):
            raise HTTPException(
                status_code=400,
                detail=f"{key} is geen geldig getal tussen 0 en 1 (kreeg: {raw!r}).",
            )

    for key in CONFIG_BOOL_KEYS:
        if key not in settings:
            continue
        validated[key] = _coerce_bool(settings.get(key))

    # Lege optionele strings mag je de sleutel laten wegvallen; dat betekent
    # "terug naar default" — dan verwijderen we de sleutel uit het bestand.
    merged = dict(existing)
    for key, value in validated.items():
        if value in (None, "") and key in ("namespace", "ingest_namespace", "api_url", "base_namespace", "ingest_path"):
            merged.pop(key, None)
        else:
            merged[key] = value

    home.mkdir(parents=True, exist_ok=True)
    tmp = target.with_suffix(".json.tmp")
    tmp.write_text(json.dumps(merged, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    os.replace(tmp, target)
    return {
        "ok": True,
        "path": str(target),
        "saved_keys": [key for key in validated if key in validated],
        "effective": _load_cortexiq_config(),
        "note": "Herstart het Hermes-proces dat de config-template leest om de changes te activeren.",
    }


@router.post("/token")
async def save_token(payload: Dict[str, Any] = None) -> Dict[str, Any]:
    """Sla CORTEXIQ_API_TOKEN op in <home>/.env (atomair; waarde nooit retour).

    Werkt op dezelfde plek die de provider leest: eerst de bestaande regel
    vervangen, anders appenden. Alleen 'token_present' gaat terug.
    """
    payload = payload if isinstance(payload, dict) else {}
    token = str(payload.get("token") or "").strip().strip('"').strip("'")
    if not token:
        raise HTTPException(status_code=400, detail="Er is geen token meegegeven.")
    if any(ch in token for ch in ("\n", "\r", "\t")):
        raise HTTPException(status_code=400, detail="Token mag geen regeleinde of tab bevatten.")

    home = _hermes_home()
    env_path = home / ".env"
    lines: List[str] = []
    if env_path.exists():
        lines = env_path.read_text(encoding="utf-8", errors="replace").splitlines()
    replaced = False
    for index, line in enumerate(lines):
        if line.strip().split("=", 1)[0].strip() == "CORTEXIQ_API_TOKEN":
            lines[index] = f"CORTEXIQ_API_TOKEN={token}"
            replaced = True
            break
    if not replaced:
        if lines and lines[-1].strip():
            lines.append("")
        lines.append(f"CORTEXIQ_API_TOKEN={token}")
        lines.append("")

    home.mkdir(parents=True, exist_ok=True)
    tmp = env_path.with_name(".env.tmp-cortexiq-setup")
    tmp.write_text("\n".join(lines) + "\n", encoding="utf-8")
    os.replace(tmp, env_path)
    return {
        "ok": True,
        "token_present": True,
        "path": str(env_path),
        "replaced_existing": replaced,
        "note": "Herstart het Hermes-proces dat het .env-bestand leest om de token te gebruiken.",
    }


@router.post("/activate")
async def activate() -> Dict[str, Any]:
    """Zet memory.provider=cortexiq in config.yaml via de hermes CLI.

    Bewust via `hermes config set` (dotted paths): directe YAML round-trips
    slopen comments in config.yaml. In het kind-proces geven we een
    credential-dragende env mee zodat de CLI het token kan vast leggen.
    """
    binary = _hermes_binary()
    if not binary:
        raise HTTPException(status_code=500, detail="Hermes CLI niet gevonden (build `hermes` is niet op PATH).")

    def _run() -> Dict[str, Any]:
        env = os.environ.copy()
        for key, value in _load_simple_env_file(_hermes_home() / ".env").items():
            env.setdefault(key, value)
        env["HERMES_HOME"] = str(_hermes_home())
        proc = subprocess.run(
            [binary, "config", "set", "memory.provider", PROVIDER_ID],
            capture_output=True,
            text=True,
            encoding="utf-8",
            errors="replace",
            timeout=60,
            env=env,
            shell=False,
        )
        return {"returncode": proc.returncode, "stdout": proc.stdout[-2000:], "stderr": proc.stderr[-2000:]}

    try:
        result = await asyncio.get_running_loop().run_in_executor(None, _run)
    except subprocess.TimeoutExpired:
        raise HTTPException(status_code=504, detail="hermes config set duurde te lang (timeout 60s).")
    except Exception as exc:
        raise HTTPException(status_code=500, detail=_safe_error(exc))

    if result["returncode"] != 0:
        raise HTTPException(
            status_code=500,
            detail=f"hermes config set faalde (code {result['returncode']}): {result['stderr'] or result['stdout']}",
        )
    return {
        "ok": True,
        "provider_selected": _provider_selected(),
        "note": "config.yaml is aangepast; herstart het Hermes-proces dat de geheugen-provider binnenleest.",
    }


@router.post("/test")
async def test_connection() -> Dict[str, Any]:
    """Verbindingstest: GET /api/v1/stats + namespaces tegen de geconfigureerde API."""
    cfg = _load_cortexiq_config()
    if not cfg["token_present"]:
        return {"ok": False, "error": "CORTEXIQ_API_TOKEN ontbreekt; sla eerst het token op."}
    live = await _cortexiq_live_check()
    ok = live["stats"].get("ok") is True
    readiness = _namespaces_ready(cfg, live["namespaces"]) if ok else None
    payload: Dict[str, Any] = {
        "ok": ok,
        "latency_ms": live["latency_ms"],
        "api_url": cfg["api_url"],
        "error": live["stats"].get("error") if not ok else None,
        "stats": live["stats"].get("data") if ok else None,
        "namespaces": live["namespaces"] if ok else [],
    }
    if readiness is not None:
        payload["namespaces_missing"] = readiness["missing"]
        payload["namespaces_existing"] = readiness["existing"]
    return payload


@router.post("/namespaces/create")
async def create_namespace(payload: Dict[str, Any] = None) -> Dict[str, Any]:
    """Maak één namespace (slug-forwaarts). HTTP 400/409 = bestond al = ok."""
    payload = payload if isinstance(payload, dict) else {}
    slug = _namespace_fixup(str(payload.get("slug") or ""))
    if not slug or slug == "/":
        raise HTTPException(status_code=400, detail="Geef een namespace-slug op, bijv. /hermes/default.")
    if any(ch.isspace() for ch in slug):
        raise HTTPException(status_code=400, detail="Namespace mag geen spaties bevatten.")

    name = str(payload.get("name") or "").strip() or f"Hermes agent ({slug.lstrip('/')})"
    result = _cortexiq_request("POST", CORTEXIQ_NAMESPACES_PATH,
                               payload={"slug": slug, "name": name})
    data = result.get("data")
    if result.get("ok"):
        return {"ok": True, "slug": slug, "created": True, "data": data}
    error = str(result.get("error") or "")
    if "HTTP 400" in error or "HTTP 409" in error:
        return {"ok": True, "slug": slug, "created": False, "existed": True, "detail": error}
    return {"ok": False, "slug": slug, "error": error}
