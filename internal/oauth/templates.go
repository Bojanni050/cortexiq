// Modifications copyright (c) 2026 Bojan van der Heide

package oauth

import (
	"html/template"
	"net/http"
)

// The consent page is the only place a human sees the OAuth flow. It is a single
// self-contained document: no assets, no framework, no external font. That keeps
// it working behind any reverse proxy and makes it impossible for a third-party
// script to observe the password.
var consentTemplate = template.Must(template.New("consent").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex,nofollow">
<title>Connect {{.ClientName}} · CortexIQ</title>
<style>
  :root {
    color-scheme: light dark;
    --bg:#f4f4f1; --card:#ffffff; --fg:#17181a; --muted:#6a6d70; --line:#e2e2dd;
    --accent:#2f5d50; --accent-fg:#ffffff; --soft:#eef3f0; --danger:#a63333; --danger-soft:#fbeeee;
  }
  @media (prefers-color-scheme: dark) {
    :root {
      --bg:#101211; --card:#191c1b; --fg:#eceeec; --muted:#9aa09c; --line:#2c302e;
      --accent:#8cc9b2; --accent-fg:#0b1613; --soft:#1e2623; --danger:#e58c8c; --danger-soft:#2a1b1b;
    }
  }
  * { box-sizing:border-box; }
  body {
    margin:0; min-height:100vh; display:grid; place-items:center; padding:24px;
    background:var(--bg); color:var(--fg);
    font:15px/1.55 -apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,Helvetica,Arial,sans-serif;
  }
  main { width:100%; max-width:430px; background:var(--card); border:1px solid var(--line); border-radius:16px; padding:28px; }
  .brand { display:flex; align-items:baseline; gap:8px; margin-bottom:22px; }
  .brand strong { font-size:15px; letter-spacing:.02em; }
  .brand span { font-size:12px; color:var(--muted); }
  h1 { font-size:21px; line-height:1.3; margin:0 0 8px; }
  .lede { margin:0 0 20px; color:var(--muted); }
  .lede code { color:var(--fg); font-size:13px; }
  fieldset { border:0; padding:0; margin:0 0 18px; }
  legend { font-size:12px; text-transform:uppercase; letter-spacing:.08em; color:var(--muted); margin-bottom:8px; }
  .scope { display:flex; gap:10px; align-items:flex-start; padding:10px 12px; border:1px solid var(--line); border-radius:10px; margin-bottom:8px; }
  .scope input { margin:3px 0 0; accent-color:var(--accent); }
  .scope strong { display:block; font-size:14px; }
  .scope small { color:var(--muted); }
  .fixed { background:var(--soft); border-color:transparent; }
  label.field { display:block; margin-bottom:18px; }
  label.field span { display:block; font-size:13px; margin-bottom:6px; color:var(--muted); }
  input[type=password] {
    width:100%; padding:10px 12px; font:inherit; color:var(--fg); background:transparent;
    border:1px solid var(--line); border-radius:10px;
  }
  input[type=password]:focus { outline:2px solid var(--accent); outline-offset:1px; border-color:transparent; }
  .error { background:var(--danger-soft); color:var(--danger); border-radius:10px; padding:10px 12px; font-size:13px; margin-bottom:16px; }
  .actions { display:flex; gap:10px; }
  button { font:inherit; font-weight:600; border-radius:10px; padding:11px 16px; cursor:pointer; border:1px solid transparent; }
  button.approve { flex:1; background:var(--accent); color:var(--accent-fg); }
  button.deny { background:transparent; color:var(--fg); border-color:var(--line); }
  button:hover { filter:brightness(1.05); }
  footer { margin-top:20px; padding-top:14px; border-top:1px solid var(--line); font-size:11px; color:var(--muted); word-break:break-all; }
</style>
</head>
<body>
<main>
  <div class="brand"><strong>CortexIQ</strong><span>persistent memory</span></div>
  <h1>Connect {{.ClientName}}</h1>
  <p class="lede">A client at <code>{{.ClientHost}}</code> wants to use your CortexIQ memory.</p>

  {{if .Error}}<div class="error" role="alert">{{.Error}}</div>{{end}}

  <form method="post" action="/oauth/authorize">
    <input type="hidden" name="response_type" value="code">
    <input type="hidden" name="client_id" value="{{.ClientID}}">
    <input type="hidden" name="redirect_uri" value="{{.RedirectURI}}">
    <input type="hidden" name="state" value="{{.State}}">
    <input type="hidden" name="code_challenge" value="{{.Challenge}}">
    <input type="hidden" name="code_challenge_method" value="{{.Method}}">
    <input type="hidden" name="resource" value="{{.Resource}}">
    <input type="hidden" name="scope" value="{{.RequestedScope}}">
    <input type="hidden" name="csrf" value="{{.CSRF}}">

    <fieldset>
      <legend>Access</legend>
      <div class="scope fixed">
        <input type="checkbox" checked disabled>
        <div><strong>Read memory</strong><small>Recall, reflect, search facts and relationships.</small></div>
      </div>
      {{if .WriteOffered}}
      <label class="scope">
        <input type="checkbox" name="allow_write" value="1" {{if .WriteChecked}}checked{{end}}>
        <div><strong>Write memory</strong><small>Remember, consolidate, create goals and hypotheses.</small></div>
      </label>
      {{end}}
    </fieldset>

    {{if not .LoggedIn}}
    <label class="field">
      <span>Admin password</span>
      <input type="password" name="password" autocomplete="current-password" autofocus required>
    </label>
    {{end}}

    <div class="actions">
      <button class="approve" type="submit" name="decision" value="approve">Allow</button>
      <button class="deny" type="submit" name="decision" value="deny">Cancel</button>
    </div>
  </form>

  <footer>Client ID: {{.ClientID}}</footer>
</main>
</body>
</html>`))

var errorTemplate = template.Must(template.New("error").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex,nofollow">
<title>Authorization error · CortexIQ</title>
<style>
  :root { color-scheme: light dark; --bg:#f4f4f1; --card:#fff; --fg:#17181a; --muted:#6a6d70; --line:#e2e2dd; --danger:#a63333; }
  @media (prefers-color-scheme: dark) { :root { --bg:#101211; --card:#191c1b; --fg:#eceeec; --muted:#9aa09c; --line:#2c302e; --danger:#e58c8c; } }
  body { margin:0; min-height:100vh; display:grid; place-items:center; padding:24px; background:var(--bg); color:var(--fg);
    font:15px/1.55 -apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,Helvetica,Arial,sans-serif; }
  main { width:100%; max-width:430px; background:var(--card); border:1px solid var(--line); border-radius:16px; padding:28px; }
  h1 { font-size:19px; margin:0 0 10px; color:var(--danger); }
  p { margin:0 0 10px; color:var(--muted); }
  code { color:var(--fg); word-break:break-word; }
</style>
</head>
<body>
<main>
  <h1>{{.Title}}</h1>
  <p>{{.Message}}</p>
  {{if .Detail}}<p><code>{{.Detail}}</code></p>{{end}}
</main>
</body>
</html>`))

type consentData struct {
	ClientName     string
	ClientHost     string
	ClientID       string
	RedirectURI    string
	State          string
	Challenge      string
	Method         string
	Resource       string
	RequestedScope string
	WriteOffered   bool
	WriteChecked   bool
	LoggedIn       bool
	Error          string
	CSRF           string
}

type errorData struct {
	Title   string
	Message string
	Detail  string
}

func renderConsent(w http.ResponseWriter, status int, data consentData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Frame-Options", "DENY")
	// The POST is protected by sameOrigin below. Do not add form-action here:
	// Claude's embedded consent browser can report the relative form action as
	// cross-origin even when the document and endpoint share this public issuer.
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'none'")
	w.WriteHeader(status)
	_ = consentTemplate.Execute(w, data)
}

func renderError(w http.ResponseWriter, status int, data errorData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'none'")
	w.WriteHeader(status)
	_ = errorTemplate.Execute(w, data)
}
