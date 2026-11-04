/* CortexIQ Setup — dashboard-plugin (gewoon IIFE, geen build-stap). */
(function () {
  "use strict";

  const SDK = window.__HERMES_PLUGIN_SDK__;
  const React = SDK.React;
  const hooks = SDK.hooks;
  const components = SDK.components;
  const Card = components.Card;
  const CardHeader = components.CardHeader;
  const CardTitle = components.CardTitle;
  const CardContent = components.CardContent;
  const Badge = components.Badge;
  const Button = components.Button;
  const Input = components.Input;
  const Separator = components.Separator;
  const Checkbox = components.Checkbox;
  const useState = hooks.useState;
  const useEffect = hooks.useEffect;
  const useMemo = hooks.useMemo;
  const e = React.createElement;

  const API = "/api/plugins/cortexiq-setup";

  // Dashboard's generieke fetchJSON profileert /api/plugins niet zelf: het
  // geselecteerde profiel projecteren we in ?profile=. Niet cachen: de
  // selectie kan veranderen terwijl de pagina ongemount is. Eigen POST's via
  // fetchJSON met {method:"POST"} — dezelfde contract als kanban gebruikt.
  const STALE_PROFILE_RESPONSE = {};

  function selectedProfile() {
    const params = new URLSearchParams(window.location.search || "");
    return (params.get("profile") || "").trim() || null;
  }

  function withSelectedProfile(path, profile) {
    if (!profile) return path;
    const hashAt = path.indexOf("#");
    const hash = hashAt >= 0 ? path.slice(hashAt) : "";
    const beforeHash = hashAt >= 0 ? path.slice(0, hashAt) : path;
    const questionAt = beforeHash.indexOf("?");
    const pathname = questionAt >= 0 ? beforeHash.slice(0, questionAt) : beforeHash;
    const params = new URLSearchParams(questionAt >= 0 ? beforeHash.slice(questionAt + 1) : "");
    if (!params.has("profile")) params.set("profile", profile);
    const query = params.toString();
    return pathname + (query ? "?" + query : "") + hash;
  }

  async function fetchPluginJSON(path, options) {
    // React draait child-effects vóór de URL-sync van ProfileProvider;
    // geef één taak zodat requests daar nooit op racen.
    await new Promise(function (resolve) { setTimeout(resolve, 0); });
    const requestProfile = selectedProfile();
    return SDK.fetchJSON(withSelectedProfile(path, requestProfile), options).then(function (payload) {
      return selectedProfile() === requestProfile ? payload : STALE_PROFILE_RESPONSE;
    });
  }

  function publishFresh(setter) {
    return function (payload) {
      if (payload !== STALE_PROFILE_RESPONSE) setter(payload);
    };
  }

  function post(path, payload) {
    return fetchPluginJSON(path, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload || {}),
    });
  }

  function messageFromException(exc) {
    try {
      const data = exc && exc.data;
      if (data && data.detail) return String(data.detail);
      if (exc && exc.message) return String(exc.message);
    } catch (err) { /* laat de generieke melding zien */ }
    return "Aanroep mislukt — zie de log van de Hermes-server.";
  }

  function okBadge(ok, label) {
    // Dit SDK-surface werkt met tone:"outline"/"secondary", geen variant.
    const props = ok ? { tone: "outline" } : { tone: "secondary" };
    return e(Badge, Object.assign(props, { key: label }),
      (ok ? "✓ " : "✗ ") + (label || ""));
  }

  function Row(step) {
    return e("div", { className: "cortexiq-step" + (step.ok ? " ok" : ""), key: step.id || step.label },
      e("span", { className: "cortexiq-step-mark" }, step.ok ? "✓" : "✗"),
      e("span", { className: "cortexiq-step-label" }, step.label),
      e("span", { className: "cortexiq-step-detail" }, step.detail || ""),
    );
  }

  function Field(props) {
    const id = "cortexiq-" + props.field;
    return e("label", { className: "cortexiq-field", htmlFor: id },
      e("span", { className: "cortexiq-field-label" }, props.label),
      e(Input, {
        id: id,
        type: props.type || "text",
        value: props.value,
        onChange: function (event) { props.onChange(props.field, event.target.value); },
        placeholder: props.placeholder || "",
      }),
      props.hint ? e("span", { className: "cortexiq-field-hint" }, props.hint) : null,
    );
  }

  function Toggle(props) {
    const id = "cortexiq-" + props.field;
    return e("label", { className: "cortexiq-toggle", htmlFor: id },
      e("input", {
        id: id,
        type: "checkbox",
        className: Checkbox ? undefined : "cortexiq-checkbox-fallback",
        checked: !!props.value,
        onChange: function (event) { props.onChange(props.field, event.target.checked); },
      }),
      e("span", null, props.label),
      props.hint ? e("span", { className: "cortexiq-field-hint" }, props.hint) : null,
    );
  }

  const DEFAULT_FORM = {
    api_url: "http://127.0.0.1:18101",
    base_namespace: "/hermes",
    namespace: "",
    ingest_namespace: "",
    ingest_path: "/api/v1/remember",
    recall_scope: "global",
    recall_limit: "5",
    min_score: "0.6",
    max_chars: "1400",
    ground_truth: true,
    turn_importance: "0.5",
    capture_assistant: false,
    assistant_max_chars: "0",
    capture_max_chars: "4000",
  };

  function formFromConfig(cfg) {
    const next = Object.assign({}, DEFAULT_FORM);
    if (!cfg) return next;
    Object.keys(next).forEach(function (key) {
      const value = cfg[key];
      if (value === null || value === undefined) return;
      next[key] = typeof next[key] === "boolean" ? !!value : String(value);
    });
    return next;
  }

  function CortexIQSetupPage() {
    const [status, setStatus] = useState(null);
    const [statusError, setStatusError] = useState(null);
    const [testResult, setTestResult] = useState(null);
    const [creating, setCreating] = useState(null); // slug die net aangemaakt wordt
    const [manualSlug, setManualSlug] = useState("");
    const [manualResult, setManualResult] = useState(null);
    const [tokenValue, setTokenValue] = useState("");
    const [tokenNote, setTokenNote] = useState(null);
    const [configNote, setConfigNote] = useState(null);
    const [activateNote, setActivateNote] = useState(null);
    const [busy, setBusy] = useState(null); // appelebale bundel: 'status'|'test'|'token'|...
    const [form, setForm] = useState(DEFAULT_FORM);
    const [formLoaded, setFormLoaded] = useState(false);

    function loadStatus() {
      setBusy("status");
      setStatusError(null);
      fetchPluginJSON(API + "/status")
        .then(publishFresh(function (payload) {
          setStatus(payload);
          if (!formLoaded) {
            setForm(formFromConfig(payload && payload.config));
            setFormLoaded(true);
          }
        }))
        .catch(function (exc) {
          setStatusError(messageFromException(exc));
          setStatus(null);
        })
        .finally(function () { setBusy(null); });
    }

    useEffect(function () { loadStatus(); }, []);

    function setFormField(field, value) {
      setForm(function (prev) { return Object.assign({}, prev, { [field]: value }); });
    }

    function runTest() {
      setBusy("test");
      setTestResult(null);
      post(API + "/test")
        .then(publishFresh(function (payload) {
          setTestResult(payload);
        }))
        .catch(function (exc) { setTestResult({ ok: false, error: messageFromException(exc) }); })
        .finally(function () { setBusy(null); });
    }

    function saveConfig() {
      setBusy("config");
      setConfigNote(null);
      post(API + "/config", { settings: form })
        .then(publishFresh(function (payload) {
          setConfigNote({ ok: true, text: "Config opgeslagen in " + payload.path + ". Herstart Hermes om de provider de nieuwe waarden te laten lezen." });
          loadStatus();
        }))
        .catch(function (exc) { setConfigNote({ ok: false, text: messageFromException(exc) }); })
        .finally(function () { setBusy(null); });
    }

    function saveToken() {
      setBusy("token");
      setTokenNote(null);
      post(API + "/token", { token: tokenValue })
        .then(publishFresh(function (payload) {
          setTokenNote({ ok: true, text: "Token opgeslagen in " + payload.path + (payload.replaced_existing ? " (bestaande token vervangen)." : " (toegevoegd).") });
          setTokenValue("");
          loadStatus();
        }))
        .catch(function (exc) { setTokenNote({ ok: false, text: messageFromException(exc) }); })
        .finally(function () { setBusy(null); });
    }

    function activate() {
      setBusy("activate");
      setActivateNote(null);
      post(API + "/activate")
        .then(publishFresh(function (payload) {
          setActivateNote({ ok: true, text: "memory.provider = cortexiq gezet. Herstart Hermes om de provider te activeren." });
          loadStatus();
        }))
        .catch(function (exc) { setActivateNote({ ok: false, text: messageFromException(exc) }); })
        .finally(function () { setBusy(null); });
    }

    function createNamespace(slug) {
      const target = (slug || manualSlug || "").trim();
      if (!target) { setManualResult({ ok: false, text: "Geef eerst een namespace-slug op." }); return; }
      setBusy("namespace");
      setManualResult(null);
      setCreating(target);
      post(API + "/namespaces/create", { slug: target })
        .then(publishFresh(function (payload) {
          const text = payload.created ? ("Namespace " + payload.slug + " aangemaakt.") : ("Namespace " + payload.slug + " bestond al — niets gedaan.");
          if (slug) setTestResult(null); else setManualResult({ ok: true, text: text });
          setManualSlug("");
          loadStatus();
        }))
        .catch(function (exc) {
          const text = messageFromException(exc);
          if (slug) setTestResult(null);
          else setManualResult({ ok: false, text: text });
        })
        .finally(function () { setBusy(null); setCreating(null); });
    }

    const config = status && status.config;
    const server = status && status.server;
    const steps = (status && status.steps) || [];
    const missing = useMemo(function () {
      if (testResult && Array.isArray(testResult.namespaces_missing)) return testResult.namespaces_missing;
      const step = steps.find(function (step) { return step.id === "namespaces_ready"; });
      return (step && step.missing) || [];
    }, [status, testResult]);

    const headerKids = [
      e("span", { className: "cortexiq-page-title" }, "CortexIQ geheugen — setup"),
      e("span", { className: "cortexiq-page-sub" },
        (config ? "profiel " + config.profile + " · " + config.hermes_home : "status ophalen…")),
      e("span", { style: { flex: "1 1 auto" } }),
      e(Button, { onClick: loadStatus, disabled: busy !== null }, busy === "status" ? "Bezig…" : "Verversen"),
    ];

    const kids = [e("div", { className: "cortexiq-header" }, headerKids)];

    if (statusError) {
      kids.push(e(Card, { key: "error", className: "cortexiq-error-card" }, e(CardContent, null,
        e("div", { className: "cortexiq-error" }, "Status ophalen mislukt: " + statusError),
        e("div", { className: "cortexiq-error-hint" }, "Draait de backend? De routes worden gemount bij de start van het Hermes-serverproces — herstart na installatie."),
      )));
      return e("div", { className: "cortexiq-root" }, kids);
    }

    // --- Instalatiechecklist -----------------------------------------------
    kids.push(e(Card, { key: "steps", className: "cortexiq-card" },
      e(CardHeader, null, e(CardTitle, null, "Installatiestatus", " ",
        status && status.ok ? okBadge(true, "klaar") : okBadge(false, "niet compleet"))),
      e(CardContent, null,
        e("div", { className: "cortexiq-steps" }, steps.map(Row)),
      ),
    ));

    // --- Live server --------------------------------------------------------
    kids.push(e(Card, { key: "server", className: "cortexiq-card" },
      e(CardHeader, null, e(CardTitle, null, "CortexIQ-server")),
      e(CardContent, null,
        e("div", { className: "cortexiq-kv" },
          e("span", { className: "cortexiq-kv-key" }, "api_url"),
          e("span", null, (config && config.api_url) || "—"),
        ),
        e("div", { className: "cortexiq-kv" },
          e("span", { className: "cortexiq-kv-key" }, "verbinding"),
          e("span", null, server && server.live ? "live (" + server.latency_ms + " ms)" : (server && server.error) || "nog niet getest"),
        ),
        e("div", { style: { marginTop: 12 } },
          e(Button, { onClick: runTest, disabled: busy !== null }, busy === "test" ? "Testen…" : "Verbinding testen"),
        ),
        testResult && !testResult.ok
          ? e("div", { className: "cortexiq-note cortexiq-note-error" }, "Test mislukt: " + (testResult.error || "onbekende fout"))
          : null,
        testResult && testResult.ok
          ? e("div", { className: "cortexiq-note" }, "Bereikbaar in " + testResult.latency_ms + " ms · " + (testResult.namespaces || []).length + " namespaces gevonden")
          : null,
      ),
    ));

    // --- Namespaces ----------------------------------------------------------
    kids.push(e(Card, { key: "ns", className: "cortexiq-card" },
      e(CardHeader, null, e(CardTitle, null, "Namespaces")),
      e(CardContent, null,
        e("div", { className: "cortexiq-kv" },
          e("span", { className: "cortexiq-kv-key" }, "ingest"),
          e("span", null, (config && config.ingest_namespace) || "—"),
        ),
        missing.length
          ? e("div", { className: "cortexiq-missing" }, missing.map(function (slug) {
              return e("div", { className: "cortexiq-missing-row", key: slug },
                e("span", null, slug),
                e(Button, {
                  variant: "outline", disabled: busy !== null,
                  onClick: function () { createNamespace(slug); },
                }, creating === slug ? "…" : "Maak aan"),
              );
            }))
          : e("div", { className: "cortexiq-note" }, "Geen ontbrekende namespaces bekend."),
        e(Separator, null),
        e("div", { className: "cortexiq-manual" },
          e(Input, {
            placeholder: "/hermes/profiel",
            value: manualSlug,
            onChange: function (event) { setManualSlug(event.target.value); },
          }),
          e(Button, { variant: "outline", disabled: busy !== null, onClick: function () { createNamespace(""); } }, busy === "namespace" ? "…" : "Maak eigen"),
        ),
        manualResult
          ? e("div", { className: "cortexiq-note" + (manualResult.ok ? "" : " cortexiq-note-error") }, manualResult.text)
          : null,
      ),
    ));

    // --- Configuratie --------------------------------------------------------
    if (config) {
      kids.push(e(Card, { key: "config", className: "cortexiq-card" },
        e(CardHeader, null, e(CardTitle, null, "Configuratie", " ",
          e("span", { className: "cortexiq-page-sub" }, config.config_path))),
        e(CardContent, null,
          e("div", { className: "cortexiq-grid" },
            e(Field, { field: "api_url", label: "API-url", value: form.api_url, onChange: setFormField }),
            e(Field, { field: "base_namespace", label: "Basis-namespace", value: form.base_namespace, onChange: setFormField }),
            e(Field, {
              field: "recall_scope", label: "Recall-scope", value: form.recall_scope, onChange: setFormField,
              hint: "global | base | profile — leeg/algemeen betekent alles",
            }),
            e(Field, {
              field: "namespace", label: "Recall-namespace (één)", value: form.namespace, onChange: setFormField,
              placeholder: "leeg = volgens scope", hint: "wint van de scope",
            }),
            e(Field, {
              field: "ingest_namespace", label: "Ingest-namespace", value: form.ingest_namespace, onChange: setFormField,
              placeholder: "leeg = <basis>/<profiel>", hint: "moet bestaan om te schrijven",
            }),
            e(Field, { field: "ingest_path", label: "Ingest-pad", value: form.ingest_path, onChange: setFormField }),
            e(Field, { field: "recall_limit", label: "Recall-limit", value: form.recall_limit, onChange: setFormField, type: "number" }),
            e(Field, { field: "min_score", label: "Min-score", value: form.min_score, onChange: setFormField }),
            e(Field, { field: "max_chars", label: "Max chars", value: form.max_chars, onChange: setFormField, type: "number" }),
            e(Field, { field: "turn_importance", label: "Turn importance", value: form.turn_importance, onChange: setFormField }),
            e(Field, { field: "assistant_max_chars", label: "Assistant max chars", value: form.assistant_max_chars, onChange: setFormField, type: "number" }),
            e(Field, { field: "capture_max_chars", label: "Capture max chars", value: form.capture_max_chars, onChange: setFormField, type: "number" }),
            e(Toggle, { field: "ground_truth", label: "Ground truth-regel", value: form.ground_truth, onChange: setFormField }),
            e(Toggle, { field: "capture_assistant", label: "Ook assistent-antwoorden opslaan", value: form.capture_assistant, onChange: setFormField }),
          ),
          e("div", { style: { marginTop: 12 } },
            e(Button, { onClick: saveConfig, disabled: busy !== null }, busy === "config" ? "Opslaan…" : "Config opslaan"),
          ),
          configNote
            ? e("div", { className: "cortexiq-note" + (configNote.ok ? "" : " cortexiq-note-error") }, configNote.text)
            : null,
        ),
      ));

      // --- Token -------------------------------------------------------------
      kids.push(e(Card, { key: "token", className: "cortexiq-card" },
        e(CardHeader, null, e(CardTitle, null, "API-token")),
        e(CardContent, null,
          e("div", { className: "cortexiq-token-row" },
            e(Input, {
              type: "password",
              placeholder: config.token_present ? "Token is gezet — opnieuw invallen om te vervangen" : "CORTEXIQ_API_TOKEN",
              value: tokenValue,
              onChange: function (event) { setTokenValue(event.target.value); },
            }),
            e(Button, { onClick: saveToken, disabled: busy !== null || !tokenValue }, busy === "token" ? "Opslaan…" : "Opslaan"),
          ),
          e("div", { className: "cortexiq-field-hint" },
            "Wordt opgeslagen als CORTEXIQ_API_TOKEN in <home>/.env — de waarde wordt nooit getoond of gelogd.",
          ),
          tokenNote
            ? e("div", { className: "cortexiq-note" + (tokenNote.ok ? "" : " cortexiq-note-error") }, tokenNote.text)
            : null,
        ),
      ));

      // --- Activeren ---------------------------------------------------------
      kids.push(e(Card, { key: "activate", className: "cortexiq-card" },
        e(CardHeader, null, e(CardTitle, null, "Provider activeren")),
        e(CardContent, null,
          e("div", { className: "cortexiq-kv" },
            e("span", { className: "cortexiq-kv-key" }, "memory.provider"),
            status.provider_selected && status.provider_selected.is_cortexiq
              ? okBadge(true, "cortexiq")
              : e("span", null, status.provider_selected && status.provider_selected.provider ? "gezet: " + status.provider_selected.provider : "niet gezet"),
          ),
          status.provider_selected && status.provider_selected.is_cortexiq
            ? null
            : e("div", { style: { marginTop: 12 } },
                e(Button, { onClick: activate, disabled: busy !== null }, busy === "activate" ? "Bezig…" : "Gebruik CortexIQ als provider"),
              ),
          activateNote
            ? e("div", { className: "cortexiq-note" + (activateNote.ok ? "" : " cortexiq-note-error") }, activateNote.text)
            : null,
          e("div", { className: "cortexiq-restart-banner" },
            "Na config-, token- of provider-wijzigingen: herstart het Hermes-proces dat de geheugen-provider leest. De backend van deze plugin mount ook alleen bij het opstarten.",
          ),
        ),
      ));
    }

    return e("div", { className: "cortexiq-root" }, kids);
  }

  window.__HERMES_PLUGINS__.register("cortexiq-setup", CortexIQSetupPage);
})();
