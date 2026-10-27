# Walkthrough (vervolg)

`walkthrough.md` liep bij 996 regels tegen de grens aan; dit is het vervolg in dezelfde stijl en volgorde.

## 2026-10-27 (v0.1.6 live: driekoloms dashboard, inklapbare contextkolom)

- Findings: `main` stond na de UI-commit op `c107f8c` met CI en Benchmark groen, maar prod draaide nog `0.1.5`. De UI is een eigen compose-service (`cortexiq-ui`) op dezelfde image, dus een deploy moet beide containers vervangen — een tag-bump en `up -d` doen dat in één keer.
- Conclusions: geen migratie en geen configuratiewijziging, dus geen backup vooraf en geen extra tussenstap: release publiceren, image laten bouwen, tag in `/opt/cortexiq/.env` zetten, `pull` + `up -d`. Het image-digest is de controle, niet het versienummer in de UI: `.Image` van beide containers en de tag wijzen naar `sha256:61d7ed372332…`, en `cortexiq version` in de container geeft `v0.1.6`/`c107f8c`.
- Actions: release `v0.1.6` (release-workflow `40547905515`, alle vijf binaries plus docker groen), `.env` gebumpt met een tijdgestempelde backup ernaast, beide containers herstart. `docs/STATUS.md` bijgewerkt (release, commit, digest, registry-regel, dashboard en de embedding-scan als known rough edge). Validated — `/ui` geeft 200 en bevat de nieuwe markup, `/api/v1/episodes/1` geeft 401 zonder token zoals het hoort, consolidatie draait over alle namespaces zonder fouten in de logs.
