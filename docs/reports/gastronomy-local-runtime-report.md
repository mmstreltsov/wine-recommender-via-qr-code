# Gastronomy Local Runtime Report

Date: 2026-11-09. Runtime verification window: 18:12-18:25 UTC
(21:12-21:25 Europe/Moscow). Role: SRE. Status: **PASS, local prototype only**.

## Authorization And Scope

The coordinator/user supplied the final prerequisites: gastronomy explicit OK,
architect OK, requirements PASS, security final PASS with 0/0/0/0 findings,
and QA normal/race PASS: 146 top-level tests / 2014 including subtests,
19 integrations, zero skips, using disposable `m5_integration`.
These are supplied gate results, not independent reruns by this SRE session.
Explicit permission was then given to start the already-built backend.

Only project `vkr-dev` in `/Users/streltsov/vkr/wine-recommender-via-qr-code`
was targeted. No other project was started. No `down`, volume removal,
manual migrations, data cleanup, public deployment, commit or push occurred.
`.env` was neither read by tools nor changed; credentials were not printed.
Sensitive logs and Docker environment fields were not inspected.

## Runtime Identity

| Component | Result |
|---|---|
| Compose validation before DB start | `docker compose config --quiet`, exit 0 |
| Project confirmation | Compose-selected DB container ID matched `docker inspect` metadata labeled `vkr-dev`; working directory/config labels matched this repository |
| DB start in preceding preparation | `docker compose up -d --wait db`, PASS |
| Existing databases | `svoyshef`, `m5_integration`; neither needed creation |
| Prior image build | `docker compose build backend`, PASS; Go compiled, frontend build stage reused valid Docker cache |
| Authorized backend start | `docker compose up -d --wait --no-deps backend`, exit 0; only backend recreated and started |
| Backend image tag | `vkr-dev-backend:latest` |
| Built image and running container `.Image` | `sha256:0a19d3f6428ff5ec867e9c1fbf5d0e9c75da54ca50461229226d4873b5ebba6e`, exact match |
| Image creation timestamp | `2026-11-09T18:06:26.452220333Z` |
| Backend final state | `running`, `healthy`, `127.0.0.1:18080 -> 8080/tcp` |
| DB final state | `running`, `healthy`, `127.0.0.1:55432 -> 5432/tcp` |
| HTTP health body | `{"status":"ok","restaurant":"aidabaran"}` |

The final four JSON reason edits are provided by the existing read-only `./data`
bind mount. No additional build was needed for those edits. The target-aware
Go text implementation was already in the image built during preparation.

Content/code Git blob IDs at verification time match the final QA snapshot:

| File | Git blob ID |
|---|---|
| `data/aidabaran/menu.json` | `13c08ae650fce04ba480a04c13030088e7be2e08` |
| `data/aidabaran/recommender/matrix.json` | `4df2f98662f77b8e2f019fa9c1ecaa6c59fff0bc` |
| `data/aidabaran/recommender/heuristics.json` | `4808ae1ebb5d0002713ebd4913a09b97f3af4268` |
| `backend/plugins/aidabaran/texts.go` | `95167526be6acdf7bb681f6f83d52c7fb975b505` |

Matrix file SHA-256:
`60a072b53eb54208af5edcbaa56f6a68599899ff5af4a59f968d4b0f865bcba2`.
This is a shared dirty-worktree verification, not a clean-source build claim.

## Commands And Evidence

Commands ran from the repository root unless an absolute temporary path is shown.
Inspect templates select only safe metadata, never `.Config.Env`:

```sh
docker compose up -d --wait --no-deps backend
docker image inspect --format 'tags={{json .RepoTags}} id={{.Id}} created={{.Created}}' vkr-dev-backend:latest
docker inspect --format '{{.Name}} project={{index .Config.Labels "com.docker.compose.project"}} image={{.Image}} status={{.State.Status}} health={{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}} ports={{json .NetworkSettings.Ports}}' vkr-dev-db-1 vkr-dev-backend-1
python3 scripts/smoke.py --base-url http://127.0.0.1:18080
node /var/folders/w8/25fckp8n09597vtwlt8wgnhw0000gn/T/opencode/gastronomy-local-runtime-check.mjs
git hash-object data/aidabaran/menu.json data/aidabaran/recommender/matrix.json data/aidabaran/recommender/heuristics.json backend/plugins/aidabaran/texts.go
```

Read-only DB aggregate checks used container-side environment references, not
literal credentials, and returned only counts:

```sh
docker compose exec -T db sh -c 'PGPASSWORD="$POSTGRES_PASSWORD" psql -X -w -U "$POSTGRES_USER" -d svoyshef -At -v ON_ERROR_STOP=1 -c "SELECT (SELECT count(*) FROM sessions) AS sessions, (SELECT count(*) FROM carts) AS carts, (SELECT count(*) FROM cart_items) AS cart_items, (SELECT count(*) FROM events) AS events" 2>/dev/null'
```

The temporary JS checker was created with `apply_patch`, outside the repository.
It parses only menu/matrix JSON for semantic comparison and performs loopback
HTTP/browser validation. No extra repository verification scripts were added.
It does not log response headers, cookies, session IDs or browser storage state.

## HTTP Results

`scripts/smoke.py` exited 0:

```text
PASS: health (no session), SPA/deep links, built assets, six WOFF2 fonts, CSP/nosniff/no-store
```

The semantic checker used one memory-only cookie jar per run, reused across all
125 item-detail requests, 125 recommendation requests and, in the successful
combined run, the browser context. It did not POST/PATCH/DELETE cart data.
Health returned `ok` without creating a session.

- Menu: 125/125 positions present.
- Full item endpoints: 125/125, exact UID and menu name.
- Recommendation endpoints: 125/125 feeds, 210/210 expert recommendations.
- Every live reason equals the current matrix string exactly.
- Every feed matches matrix score/category/menu ordering exactly; no extra
  heuristic candidates or missing expert pairs were observed.
- All recommendation names match menu titles.

For `655892482392`, the exact five UIDs in both HTTP and rendered DOM were:

```text
366609864022, 733465955822, 231821651422, 445870875313, 951817158352
```

All five reasons matched the current matrix and all five sources were `expert`.

The final four corrected reasons were confirmed in HTTP and in the browser at
both viewport sizes:

| Directed pair | Exact live reason, equal to current matrix | Source |
|---|---|---|
| `808267771773 -> 981442240863` | Апельсиновый фреш дополнит крабового барашка: цитрусовая кислинка оттеняет типичную сладковатую ноту краба, а лаваш даёт несладкую хлебную основу. | `expert` |
| `238582192523 -> 951817158352` | К газированной Borjomi — мороженое отдельным десертом: минеральный вкус и пузырьки создают контраст холодной сладкой текстуре; вкус мороженого в рекомендации не задан. | `expert` |
| `314183598113 -> 951817158352` | К фейхоа Лагидзе — мороженое отдельным десертом: типичный кисло-сладкий профиль напитка оттеняет сладость, а мягкая холодная текстура создаёт контраст питью. | `expert` |
| `507171197452 -> 231821651422` | К мясному ассорти — немного ткемали: типичная фруктовая кислинка соуса создаёт контраст несладкому мясному вкусу. Соус остаётся необязательным акцентом. | `expert` |

## Browser Results

Existing Playwright `1.65.0` and cached Chromium `158.0.8127.18` were used.
No packages or browsers were downloaded. The initial HTTP-only attempt passed,
but default browser discovery found no executable. The existing M5 runner
identified its already-installed browser directory; setting
`PLAYWRIGHT_BROWSERS_PATH` to the existing `m4-qa-prep/browsers` directory resolved
discovery. The subsequent combined HTTP/browser run exited 0.

Viewport sizes were 375x812 and 1440x900. Six dishes were checked at each size,
12 page/viewport combinations total:

| Dish UID | Scenario | Recommendation cards per viewport |
|---|---|---:|
| `655892482392` | Beef/pork khinkali, exact five | 5 |
| `759806895422` | Grilled dorado with pomegranate sauce, water only | 1 |
| `808267771773` | Crab dish, corrected bread/citrus explanation | 1 |
| `238582192523` | Borjomi, corrected unspecified ice-cream flavor | 1 |
| `314183598113` | Feijoa Lagidze, corrected dessert contrast | 1 |
| `507171197452` | Meat assortment, corrected optional tkemali | 2 |

For every combination: exact rendered dish/card names, recommendation order and
full reason strings; scrollable page; maximum horizontal overflow **0 px**;
no title/reason/description clipping or ellipsis. Recommendation buttons were
visible, enabled, passed Playwright trial-click actionability, and accepted
keyboard focus. Main add-to-cart buttons also passed trial-click actionability.
Trial clicks did not dispatch real clicks or create cart entries.

All non-loopback browser requests were blocked: 34 attempted external image
requests were aborted before network access. Existing local photo/placeholder
fallbacks loaded successfully. Zero API error responses, zero JS page errors,
zero attempted cart mutations were observed by the browser checker.

Twelve full-page screenshots remain outside the repository at:

```text
/var/folders/w8/25fckp8n09597vtwlt8wgnhw0000gn/T/opencode/gastronomy-runtime-<width>x<height>-<dish_uid>.png
```

Both khinkali screenshots and the mobile dorado screenshot were additionally
opened for visual inspection: names and reasons remain complete, all five
khinkali alternatives are present, and controls remain inside the page width.
Screenshots are temporary local evidence, not committed report attachments.

## Data Effects

Read-only aggregate snapshots in `svoyshef`:

| Snapshot | Sessions | Carts | Cart items | Events |
|---|---:|---:|---:|---:|
| Before semantic/browser checks | 1 | 1 | 0 | 248 |
| After initial HTTP-only attempt | 2 | 1 | 0 | 248 |
| After successful combined run | 3 | 1 | 0 | 306 |

Two new anonymous sessions reflect the two checker runs. Each run issued only
one cookie; the successful run shared it with the browser. Cookies stayed only
in memory and were not saved or printed. Carts and cart items did not increase.
Browser navigation generated synthetic view/impression events. The observed
event-count increase was 58 during the check window; aggregate counts in a
shared development stack are not exclusive attribution to this checker.
Existing rows and new synthetic events were not deleted or altered manually.
No destructive integration tests were run in this runtime pass.

## Limits And Handoff

This report confirms local startup, image identity, current expert content and
the stated HTTP/UI cases. It is not a new independent security audit, full QA
rerun, exhaustive cart-combination check, or a test of fallback Go text rendering
under runtime heuristic selection: the current empty-cart feeds were all expert.
The supplied final gates remain separate evidence; this SRE session has no
subagent invocation tool and does not manufacture an additional security OK.

Physical iOS/Android devices, camera/printed QR, Safari, touch interaction,
public TLS/proxy deployment, CDN availability, chef tasting/recipe validation,
restaurant pilot outcomes and research/business effects were not verified.
The prototype remains loopback-only and is not reachable by a separate phone.
Local photo/placeholder fallback success is not CDN approval.

The stack is left running and healthy for local review. Only this report was
added inside the repository by this SRE runtime task; existing shared changes
were preserved. No commit or push was made.
