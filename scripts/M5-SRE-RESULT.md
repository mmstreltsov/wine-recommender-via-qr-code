# M5 SRE Handoff

Verified 2026-11-08. Infra implementation and SRE self-check complete; this is NOT
an independent security approval or closure of the whole M5 milestone.

## Running Stack

- Project: `vkr-m5-acceptance`.
- Env: `/var/folders/w8/25fckp8n09597vtwlt8wgnhw0000gn/T/opencode/m5-final-qa/acceptance.env`.
- Env created with random secret, file mode `0600`, outside the repository. Do not print it.
- Browser origin: `http://127.0.0.1:18080`.
- PostgreSQL: `127.0.0.1:55432`.
- New volume: `vkr-m5-acceptance_postgres-data`, created `2026-11-08T20:05:25Z`.
- Browser/funnel DB: `svoyshef`; destructive-test DB: **`m5_integration`**.
- Container `DATABASE_URL`, host `LOCAL_DATABASE_URL`, and test `M2_IT_DATABASE_URL` are in env.
- Both acceptance services left running and healthy.
- Original `wine-recommender-via-qr-code` containers were NOT restarted/recreated;
  original container IDs remained `3a2b911d8482` / `910d73730003` during checks.
- No existing volumes deleted, reused for acceptance or altered by these commands.

## Reproduce

From the repository root, reusing the existing private env (do not regenerate it):

```bash
export COMPOSE_ENV_FILES=/var/folders/w8/25fckp8n09597vtwlt8wgnhw0000gn/T/opencode/m5-final-qa/acceptance.env
export COMPOSE_PROJECT_NAME=vkr-m5-acceptance
docker compose config --quiet
docker compose ps
docker compose up -d --wait
python3 scripts/smoke.py --base-url http://127.0.0.1:18080
(set -a; . "$COMPOSE_ENV_FILES"; set +a; go -C backend test -count=1 ./...)
python3 -m unittest discover -s scripts -p 'test_*.py' -v
scripts/qr-seed/.venv/bin/python -m unittest discover -s scripts/qr-seed -v
bash scripts/clean-source-build.sh vkr-m5-clean-source
```

`m5_integration` already exists. Do not run `createdb` again here. No secrets are
needed on the command line. Do not substitute a URL targeting the funnel or pilot
database. Normal `docker compose up` includes both services, with no profiles.

## Checks Passed

- Docker Hub manifest inspection confirmed all four supported tags and their
  amd64/arm64 manifests. Multi-arch index digests pinned in source.
- `docker compose build backend` and full isolated `up -d --wait`: PASS.
- HTTP smoke: health creates no cookie; SPA/deep links, generated JS/CSS, six
  WOFF2 font signatures and MIME, CSP/nosniff, API no-store: PASS.
- Runtime inspection: backend UID 10001; read-only root; `/app/data` mount RW=false;
  cap-drop ALL; no-new-privileges. PG server processes run as UID 70, not root.
- Port inspection: acceptance backend/db bound only to 127.0.0.1:18080/55432.
- Python env tests: 2 PASS. QR validation/range/subsets/quiet-zone/CLI/print tests: 6 PASS.
- `go -C backend test -count=1 ./...`: PASS, with destructive integration tests
  enabled against `m5_integration` in the new acceptance DB container.
- Clean-source build with `--no-cache --pull`: PASS. Context streamed from current
  tracked/non-ignored source files, physically excluding ignored node_modules,
  dist, local photos and env. No files in the working tree renamed/deleted.
- Clean image was also started WITHOUT a data bind mount on temporary loopback
  port 18081, with the integration DB (not the browser DB). Runtime preconditions
  checked no photos/node_modules, built index, UID 10001. Same HTTP smoke PASS.
  Only this disposable smoke container was stopped/removed afterward; no volumes
  were removed. Clean image remains `vkr-m5-clean-source:latest`.
- Offline Chromium print test: 20 SVG QR loaded, print CSS applied, PDF generated.
  PDF reviewed: 3 A4 pages, tables 1-9 / 10-18 / 19-20, no split/cropped cards.
- `bash -n scripts/clean-source-build.sh`, `git diff --check`: PASS.

Print artifacts:
`/var/folders/w8/25fckp8n09597vtwlt8wgnhw0000gn/T/opencode/m5-final-qa/sre-print/`
contains `index.html`, 20 SVG files, `qr-a4.pdf`, `qr-a4.pdf.png`.

Browser print check used the existing QA Playwright installation, not a new app
dependency. Re-run with the same installed browser/module:

```bash
export PLAYWRIGHT_MODULE=/var/folders/w8/25fckp8n09597vtwlt8wgnhw0000gn/T/opencode/m4-qa-prep/node_modules/playwright
export PLAYWRIGHT_BROWSERS_PATH=/var/folders/w8/25fckp8n09597vtwlt8wgnhw0000gn/T/opencode/m4-qa-prep/browsers
node scripts/qr-seed/print-smoke.cjs /var/folders/w8/25fckp8n09597vtwlt8wgnhw0000gn/T/opencode/m5-final-qa/sre-print/index.html /var/folders/w8/25fckp8n09597vtwlt8wgnhw0000gn/T/opencode/m5-final-qa/sre-print/qr-a4.pdf 20
```

## Limits And Review

- No changes made by this SRE task to backend/frontend/docs. Their concurrent
  M5 edits were included in builds/tests, not reverted or committed.
- No commits or pushes performed.
- No full browser funnel/CSV reconciliation performed here; QA owns that step.
  HTTP smoke does not POST events or mutate carts.
- No physical printer/phone scan performed. Verify an actual print before rollout.
- Clean-source smoke proves the intentional missing-photo degradation, not FR-26
  offline photo fallback acceptance on the pilot data tree.
- Local-only deployment, not public production. TLS/reverse proxy is outside M5;
  current direct-TLS origin/cookie behavior is explicitly documented in README.
- Existing original stack still exposes backend :8080 on all interfaces; this task
  deliberately did not change/restart it. New compose defaults are loopback-only.
- `npm ci` reports **7 moderate vulnerabilities** in the existing frontend lockfile
  and deprecated ESLint 9.39.5. SRE did not run audit fix or change dependencies.
  Route findings to frontend/security; do not treat this as a clean dependency audit.
- QR dependency verified: qrcode 8.2 wheel hash, BSD-3-Clause with inherited MIT
  notice, stable PyPI release; SVG factory avoids Pillow/pypng/native dependencies.
- SRE self-review found no remaining HIGH/CRITICAL in these infra changes. No
  security subagent tool is available in this session, so independent mandatory
  security approval remains a coordinator gate before closing M5.
