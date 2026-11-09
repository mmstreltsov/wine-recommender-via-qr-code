# Svoy Shef: Local Prototype

QR menu, rule-based recommendations, anonymous cart and research events for the
Aidabaran pilot. Payment is a stub. This is a local educational prototype, not a
production-ready public deployment. Specifications live in `docs/`.

## Start Locally

Prerequisites: Docker Engine/Desktop with Compose v2+ and Python 3.11+ recommended.
For the completed local acceptance run and its private env path, see
`scripts/M5-SRE-RESULT.md` (no secret values are stored there).
Host Go, Node, `node_modules`, built `dist` and local photos are NOT required for
the container build. First build needs access to Docker Hub, npm and Go modules.
Run commands from the repository root.

```bash
python3 scripts/prepare-env.py
docker compose up
```

The helper creates a new `.env` with random database credentials, mode `0600`,
and project name `vkr-local`. It refuses to replace existing files. `.env` is
gitignored; never commit or print it. `.env.example` documents variables without
credentials. Manual env preparation is also possible: set all four required
`POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`, `DATABASE_URL` values. The
container DSN must target `db:5432`; URL-encode manually chosen credentials.
Never use `docker compose config` without `--quiet` in shared logs: it exposes env.
Docker access permits inspecting container env and must be treated as privileged.

After env preparation, plain `docker compose up` starts **db + backend**, without
profiles or service flags. Open http://127.0.0.1:8080/?table=12. Both published
ports bind **127.0.0.1 only**; `BACKEND_PORT` and `DB_PORT` change host ports, not
container ports. The backend applies embedded migrations at startup and serves
the SPA, `/api/v1/*` and `/static/*` from one origin.

```bash
docker compose ps
python3 scripts/smoke.py
docker compose stop
docker compose start
```

Do not use `down -v`, `volume rm` or volume pruning on research data. Changing
`POSTGRES_PASSWORD` in env does not rotate credentials in an initialized volume.
Use a new project/volume for isolated work, or perform an explicit DB rotation.
There are no fixed container names or globally named volumes in compose.

## Runtime Safety

The backend runs as UID 10001, with a read-only root filesystem, dropped Linux
capabilities and `no-new-privileges`. `./data:/app/data:ro` is a read-only bind
mount per the M5 architecture. Availability CLI writes to PostgreSQL, not files.
Tracked fonts and menu data are included in the image; compose uses the host data
tree. Make it readable by UID 10001, do not grant write access to the container.
Postgres uses the official image entrypoint for initialization, then runs the
database as its `postgres` user; its data volume must remain writable.

Photos are intentionally absent from the image/git. On a clean clone, CDN photos
work, but missing offline fallback photos degrade to the UI placeholder. FR-26
offline fallback acceptance requires the pilot's local photos. To fetch them
separately, follow `data/aidabaran/README.md`; the Docker build does not fetch photos.

**No public TLS/reverse-proxy deployment is provided in M5.** Current backend
origin checks derive the scheme from `r.TLS` and compare the origin host to `Host`;
cookie `Secure` also depends on direct TLS. Forwarded scheme/host headers are not
trusted. An arbitrary TLS-terminating proxy forwarding HTTP will reject browser
writes and will not correctly set `Secure` cookies. Do not work around this by
rewriting browser Origin or weakening guards. Trusted proxy/origin configuration,
TLS, public domain, firewall and a fresh security review are separate follow-up
work. Loopback QR links cannot be opened from a guest's phone.

## Editing Recommendations

Pairings and reasons are static content, prepared offline (no runtime LLM).
Sources: `data/aidabaran/recommender/matrix.json` (directed expert pairs with
`score` and `reason_text`), `heuristics.json` (category fallback rules),
`forcart_priorities.json` (cart course priorities), and the Go fallback templates
in `backend/plugins/aidabaran/texts.go`. The matrix is directional: `A → B` does
not imply `B → A`. Scores are editorial priorities, not purchase probabilities.
Review reports: `docs/reports/gastronomy-*.md`.

After editing JSON data, validate and restart the backend (no DB/volume removal):

```bash
cd backend && go run ./cmd/validate-matrix
docker compose restart backend
```

Changing Go templates (or any backend/frontend code) requires a rebuild:
`docker compose up -d --build --no-deps backend`.

The read-only reviewer role `.opencode/agents/georgian-gastronomy.md` is offline
tooling for content review; it does not affect guest runtime and loads only after
a full OpenCode restart.

## Isolated Acceptance

Do not operate on an existing project just to test a new build. Choose a new
project name and unused ports; verify its volume does not exist before first use.
For a fresh environment (do not overwrite an existing env):

```bash
python3 scripts/prepare-env.py --out /private/tmp/vkr-m5-acceptance.env --project vkr-m5-acceptance --db-port 55432 --backend-port 18080
export COMPOSE_ENV_FILES=/private/tmp/vkr-m5-acceptance.env
export COMPOSE_PROJECT_NAME=vkr-m5-acceptance
docker compose config --quiet
docker compose build
docker compose up -d --wait
python3 scripts/smoke.py --base-url http://127.0.0.1:18080
```

Use a **separate DB** for destructive Go integration tests, never the DB used by
the browser funnel. The generated env points `M2_IT_DATABASE_URL` to
`m5_integration`, not `svoyshef`. Create it once in the new acceptance project:

```bash
docker compose exec -T db sh -c 'createdb -U "$POSTGRES_USER" m5_integration'
```

For local Go tests, source only the trusted helper-generated env in a subshell.
The integration tests truncate tables in their target database. Keep both browser
and test databases on this separate project, not on the running pilot stack.

```bash
(set -a; . "$COMPOSE_ENV_FILES"; set +a; go -C backend test -count=1 ./...)
python3 -m unittest discover -s scripts -p 'test_*.py' -v
```

Do not point `M2_IT_DATABASE_URL` at the funnel DB or the existing pilot DB.

### Credential Cutover

Before migration 0003, warn active guests: their existing real guest cookies are
deliberately invalidated. They receive a new anonymous session and an empty cart;
historical sessions, carts, events and research exports are retained, not deleted.
Stop **all old backend versions** sharing the target database before migrating or
starting the new binary. Do not mix versions, roll back to ID-based authentication
or add a legacy-cookie fallback; exported analytics UUIDs must never authenticate.
See `docs/adr/0005-anonymity.md`. This cutover has been exercised only on the local
`vkr-m5-acceptance` project (18080/55432), not on the original deployment (8080/5432).

## Clean Build

```bash
bash scripts/clean-source-build.sh vkr-m5-clean-source
```

This streams a tar context containing current tracked and non-ignored untracked
files (`git ls-files --cached --others --exclude-standard`), not just committed
HEAD. Ignored `frontend/dist`, `frontend/node_modules`, local photos and `.env`
are physically absent from the input. It neither removes nor renames local files.
Docker additionally filters `.dockerignore`, then `npm ci` + `npm run build` and
`go mod download` + static Go build run in separate stages, with `--no-cache`.
No source or dependency bind mounts are used during image build. Runtime only
contains the server, built SPA and tracked restaurant data, not Node/Go tooling.
New ignored source dependencies would intentionally make this check fail.

Base tags were verified against Docker Hub on 2026-11-08, including amd64/arm64:
`node:24-alpine3.23` (LTS), `golang:1.26-alpine3.23` (supported Go branch),
`alpine:3.23` (security support to 2027-11), `postgres:16-alpine3.23` (PG16 support
to 2028-11). All are pinned to manifest-list digests in Dockerfile/compose.
Review and refresh digests regularly for security updates; pinning is not an
automatic patch service. Node 20, Go 1.24 and Alpine 3.20 are not used.

## Operator Commands

Use the correct env/project for every command. For the default stack `.env` is
loaded automatically; for acceptance export the two `COMPOSE_*` variables above.

```bash
docker compose exec -T backend /app/backend/server availability find 'Хинкали'
docker compose exec -T backend /app/backend/server availability list
docker compose exec -T backend /app/backend/server export-events --from 2026-11-01 --to 2026-11-30 > events.csv
docker compose exec -T db sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' > /private/tmp/svoyshef.dump
```

`availability set ITEM_ID off|on` changes live availability after the cache TTL
(default 5 seconds). Export uses inclusive UTC calendar dates (internally a
half-open timestamp range), UTF-8 CSV with a header, LF, no BOM. Redirect stdout
only, not stderr. Treat backups/events as research data: restrict file permissions
(`umask 077` before exporting), retain only as needed, do not publish in git.
Restore drills belong in another newly created project/database, never over the
pilot or funnel DB. A successful dump alone is not proof of restore readiness.

## QR Printing

See `scripts/qr-seed/README.md` for pinned/hash-checked installation, unit tests,
URL validation and offline A4 printing. `all/odd/even` subsets are operator
selection for research printouts, **not product A/B assignment or feature flags**.
Default tables are 1-20; actual pilot count is an operator input.
