# CortexIQ Setup Plugin

Standalone Hermes-plugin die de CortexIQ geheugen-provider in Hermes installeert en beheert:

- **Status-dashboard**: checklist van de installatiestappen (pip-package, provider-keuze, API-token, live verbinding, namespaces).
- **Configuratie**:ernet bewerken van `cortexiq.json` (api_url, namespaces, recall-opties) met validatie.
- **Token**: veilig opslaan van `CORTEXIQ_API_TOKEN` in `<home>/.env` (wordt nooit teruggegeven of gelogd).
- **Activeren**: zet `memory.provider=cortexiq` in `config.yaml` via `hermes config set`.
- **Namespaces**: ontbrekende namespaces (zoals `/hermes/<profile>`) met één klik aanmaken.

De setup-wizard bewust read/write beperkt tot installatie + status; voor het
*lezen* van herinneringen bestaat al het aparte
[hermes-memory-ui](https://github.com/Bojanni050/hermes-memory-ui)-plugin.

## Installatie

```bash
# vanuit deze repo, map integrations/cortexiq-setup
hermes plugins validate integrations/cortexiq-setup
hermes plugins install integrations/cortexiq-setup

# of handmatig
cp -r integrations/cortexiq-setup "$HERMES_HOME/plugins/cortexiq-setup"
```

Herstart daarna de Hermes-server waar het dashboard draait: backends worden
 gemount bij het opstarten van het Hermes-proces.

> **Belangrijk**: de plugin heet bewust `cortexiq-setup` (niet `cortexiq`):
> `$HERMES_HOME/plugins/<naam>/` wordt door Hermes ook gebruikt om
> geheugen-providers op naam te vinden, dus een map `cortexiq` zou de echte
> provider kunnen ondermijnen.

## Wat de plugin NIET doet

- Lees-/doorzoek-UI voor herinneringen → gebruik `hermes-memory-ui`.
- Provider-implementatie zelf → die zit in `integrations/hermes`
  (cortexiq-hermes pip-package).

## Routes (dashboard-API)

Ge mounted op `/api/plugins/cortexiq-setup/`:

| Methode | Pad | Doel |
|---|---|---|
| GET | `/status` | Installatiechecklist + live serverinfo |
| POST | `/config` | Config schrijven naar `<home>/cortexiq.json` |
| POST | `/token` | `CORTEXIQ_API_TOKEN` opslaan in `<home>/.env` |
| POST | `/activate` | `hermes config set memory.provider cortexiq` |
| POST | `/test` | Verbindingstest naar API (`/api/v1/stats`) |
| POST | `/namespaces/create` | Namespace aanmaken (400/409 = bestond al) |
