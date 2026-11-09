# M4 Final Security Audit — «Свое Шеф»

Дата: 2026-11-08 · Аудитор: security-агент (read-only, репо не изменялось)
Скоуп: `git diff b173e51..92df2bd` (frontend/ целиком; backend/internal/api/mutation_guard*, handlers_cart, router; internal/app/events*; cmd/server/main_integration_test; data/aidabaran/assets/fonts/*; docs/reports/m4-checkpoint.md) ПЛЮС незакоммиченный diff docs/10-requirements.md.
Опорные доки: AGENTS.md (правила безопасности), docs/30-contracts/api.md, events.md, docs/10-requirements.md NFR-04.

## VERDICT: FINDINGS (блокеров нет)

| Severity | Кол-во |
|---|---|
| CRITICAL | 0 |
| HIGH | 0 |
| MEDIUM | 0 |
| LOW | 1 |
| INFO | 7 |

**Гейт M4 (отсутствие CRITICAL/HIGH) пройден → закрытие M4: OK.**

---

## Findings

### [LOW] frontend/package-lock.json — minimatch (GHSA-g3hv-75wg-m249, moderate) в dev-цепочке
- Цепочка: typescript-eslint → @typescript-eslint/* → typescript-estree → minimatch 7.0.0–11.0.0 (DoS через globparts amplification). 7 записей аудита — одна корневая причина.
- Эксплуатация: невозможно в проде — пакеты не входят в production-цепочку; подтверждено: `npm audit --omit=dev` → **0 vulnerabilities** (runtime-deps только react, react-dom, react-router-dom).
- Статус совпадает с задокументированным остаточным риском в m4-checkpoint.md (п. «Апрувы и безопасность»); потенциально ломающий override намеренно не применялся — решение корректное.
- Исправление: в M5 поднять typescript-eslint/minimatch фиксом минорных версий, когда выйдет patched релиз; не применять `npm audit fix --force`.

### [INFO] Отсутствует заголовок Content-Security-Policy
- backend/internal/api/router.go:76-84 (withSecurityHeaders: только nosniff + no-store).
- Риск XSS практически исчерпан другими слоями: React экранирует вывод, dangerouslySetInnerHTML/innerHTML в src отсутствуют (grep = 0, только негативные тесты), URL картинок строго вайтлистятся (validation.ts:25-46 — /static только same-origin без ../ и управляющих символов; remote — только https://*.tildacdn.com|.net без портов/кредов/query). CSP — defense-in-depth.
- Рекомендация M5: добавить CSP (script-src 'self'; style-src 'self'; img-src 'self' https://*.tildacdn.com; font-src 'self'; connect-src 'self'; frame-ancestors 'none'; object-src 'none'; base-uri 'self') на reverse proxy или в withSecurityHeaders.

### [INFO] In-memory rate-limiters (события 120/мин/сессия, 30 сессий/мин/IP) — на процесс
- backend/internal/api/limiter.go:8-58; чистка stale-ключей каждые 1024 вызова (утечек нет, mutex — race-free).
- Для v0-пилота (один инстанс, один ресторан) прямо признано достаточным контрактом api.md §events. Ограничение становится дырой только при горизонтальном масштабировании — в скоупе v0 неактуально.
- Рекомендация M5+: при >1 инстансе перейти на разделяемый стор (Redis) — фиксировать решением до масштабирования.

### [INFO] TLS reverse proxy + Secure cookie отложены на M5 (задокументировано)
- docs/reports/m4-checkpoint.md:29, 37; cookie: middleware.go:134-142 (HttpOnly, SameSite=Lax, Secure только при r.TLS != nil).
- Оценка: для v0-пилота в локальном/dev-окружении (localhost, compose-БД на 127.0.0.1) — НЕ CRITICAL/HIGH. Важно: без TLS cookie уходит по http, но трафик Loopback/LAN пилота; публичный запуск без TLS был бы HIGH.
- Существенно (проверено): слепого доверия X-Forwarded-* в коде НЕТ — clientIP() использует только RemoteAddr (middleware.go:97-102), Origin-гард сверяет scheme с r.TLS, а не с заголовком (mutation_guard.go:44-48). За некорректно сконфигурированным TLS-прокси запись упадёт в 400 (fail-closed), а не в обход. Направление отказа безопасное.
- Рекомендация M5 (обязателен до публичного пилота): terminate TLS на reverse proxy, пробрасывать Proto осознанно (один доверенный хоп), включить Secure-cookie всегда за прокси, добавить HSTS; прогнать интеграционный тест за прокси (образец уже есть: TestNFR04_ProxyMustPreserveHost).

### [INFO] /static без Cache-Control для woff2/фото
- backend/internal/api/static.go:20-61 — только nosniff; заголовок кэширования не ставится.
- Влияние только на производительность (QA Lighthouse 70/70/70 прошёл и так). Не безопасность.
- Рекомендация M5: `Cache-Control: public, max-age=31536000, immutable` для /static/*/assets/fonts/*.

### [INFO] Теоретический symlink-escape в static-роутинге
- static.go:55-60: os.Stat следует symlink — symlink внутри data/<restaurant>/ мог бы указывать вне корня. Проверка префикса делается по строке пути, не по resolved-цели.
- Не эксплуатируется в v0: каталог data/ полностью репо-контролируем, пути загрузки файлов от пользователей нет. Фиксировать при появлении upload-функционала (v1+) — тогда EvalSymlinks на корень и цель.

### [INFO] Mutation guard пропускает запросы без Origin и без Sec-Fetch-Site
- mutation_guard.go:27-49 — допущено сознательно (не-браузерные клиенты: curl/CLI/интеграционные тесты). CSRF-модель про браузер: браузер всегда шлёт Origin на cross-origin POST и Sec-Fetch-Site на всех запросах; вторая линия — SameSite=Lax cookie, не прикрепляемая на cross-site POST. Проверки жёсткие: null/malformed/duplicate Origin, порт/scheme mismatch, userinfo/path/query/fragment в Origin, multiple Sec-Fetch-Site, «none» допустим только как user-initiated; Content-Type ровно один application/json (suffix-типы отклонены); DELETE — строго без тела; гард стоит РАНЬШЕ session-middleware — отклонённый write не создаёт сессию и не расходует IP-лимит (покрыто TestNFR04_WriteGuardBeforeSession, интеграционным тестом со сверкой count(*) FROM sessions = 0).
- Находок нет; записано для полноты модели угроз.

### [INFO] SPA-fallback отдаёт index.html (200) на неизвестные GET /api/v1/*-пути
- static.go:63-88 + router.go:62-64: GET /api/v1/nonexistent не матчится API-роутами и уходит в SPA-хендлер. Семантика API (200 вместо 404) — косметика; данных/поверхности атаки не добавляет (JSON-клиенты валидируют parseMenu → throw).
- Рекомендация M5 (опционально): в SPA-хендлере возвращать 404 JSON для префикса /api/v1/.

---

## Проверки по чек-листу задания (подтверждения)

1. **XSS**: dangerouslySetInnerHTML/innerHTML/document.write/eval — 0 в frontend/src (совпадения только в тестах, негативные: `javascript:alert(1)` отклоняется валидатором — client.test.ts:141-143, components.test.tsx:36). Вся отрисовка — JSX-escaping. URL изображений: safeLocalImage (только /static, same-origin, без ../, пробелов, control-chars, query/hash; двойное декодирование %2e%2e/%5c/%00 покрыто тестами client.test.ts:143) и safeRemoteImage (только https, хост-суффикс-матч tildacdn.com/.net, без port/user:pass/query). Тексты ≤10000 симв., массивы ≤2000, числа — safe integer, total корзины пересчитывается и сверяется (validation.ts:91).
2. **Валидация ответов API**: строгие runtime-парсеры на каждый эндпоинт (validation.ts), ApiError наружу без внутренних деталей (generic «Что-то пошло не так…»; негативный тест pages.test.tsx:81-84 — строка с SQL/secret не показывается). client.ts: integer()-гварды на id/qty (1..20), source-enum, сериализация старта (bootstrap до любых запросов — нет гонки сессий), table из URL — регэксп ^\d{1,3}$ + диапазон 1..999, в запрос идёт Number(raw) (инъекции query нет).
3. **Секреты/токены во фронте**: grep secret/password/apiKey/token/bearer/authorization/postinstall по frontend — только названия пакетов в lock-файле и тестовые фикстуры. console.*/process.env/import.meta.env в src — 0. Креденшелов нет; аутентификации нет по NFR-03.
4. **Tracker** (tracker.ts): только 9 имён таксономии events.md, payload — фиксированные скаляры из кода (никакого пользовательского ввода, ПДн нет); очередь ≤200; при 429 — тихий drop без ретраев (контракт api.md); page_open шлётся после старта (сессия уже существует). Отправка: fetch keepalive / sendBeacon c Blob type application/json — проходит mutation guard.
5. **Кэширование**: writeJSON — Cache-Control: no-store + nosniff на все /api/v1 (respond.go:12-20, router.go:76-84). /static и SPA — no-store не нужен (см. INFO про immutable-кэш шрифтов).
6. **vite.config.ts / vite.config.test.ts**: прокси /api и /static → http://localhost:8080, changeOrigin:false (Host браузера сохраняется — Origin-гард бэкенда валиден), xfwd отсутствует и закреплён тестом (vite.config.test.ts:14 — not.toHaveProperty('xfwd', true)). Плюс backend-тест TestNFR04_ProxyMustPreserveHost: подмена Host прокси → 400 (fail-closed). Dev-прокси не ослабляет Origin-гард; X-Forwarded-* бэкендом не доверяется нигде.
7. **Backend M4**:
   - mutation_guard.go — см. INFO выше; гард до session-middleware (router.go:66-67), порядок middlewares корректный (guard → session → logging → recovery → security headers).
   - handlers_cart.go decodeJSON: MaxBytesReader 64KB + DisallowUnknownFields + проверка «ровно один JSON-документ» (EOF-after-JSON) — двойной {} / trailing-контент → 400 до мутации (TestNFR04_DecodeSingleJSONBeforeMutation).
   - events.go: белый список 9 клиентских имён; для каждого — точный набор полей (len(payload)==len(fields): лишние/недостающие → 400); типы-валидаторы: integer c диапазонами и Trunc-проверкой (NaN/Inf безопасно отбрасываются сравнениями), enum-строки; повторный marshal с лимитом 1KB; серверные имена (menu_add_click и пр.) от клиента → 400 (ровно один источник истины — фикс прошлого HIGH удержан). Событие пишется только после полной валидации (fake-store тесты: invalid event must not be stored).
   - SQL: конкатенаций нет (grep по fmt.Sprintf/«+» с SELECT/INSERT/UPDATE/DELETE — 0); store вне diff M4, ранее проверен.
   - Race: лимитеры под mutex; integration-тесты с -race зелёные (checkpoint: Backend race PASS).
8. **Шрифты**: data/aidabaran/assets/fonts/ — ровно 6×.woff2 (у всех магика wOF2 подтверждена), 2×OFL, README с SHA-256 и провенансом; ничего лишнего. CSS @font-face — только локальные /static/... URL, внешних запросов шрифтов нет (подтверждено QA: fontRequests все same-origin). Лицензии OFL приложены — редистрибуция легальна.
9. **Static-маршрут**: листинга нет (IsDir → 404), path traversal закрыт тройно: явный запрет сегмента «..», path/filepath.Clean, префикс-проверка root+separator (static.go:43-53); URL уже percent-decoded в r.URL.Path, закодированные варианты покрыты фронтовыми негативными тестами. SPA-хендлер — Clean + префикс-проверка (static.go:77-83).
10. **package.json**: runtime-deps — react, react-dom, react-router-dom (3 шт., все официальные, caret-пиннинг в lock); install-скриптов нет (postinstall/preinstall — 0 в package.json и lock); подозрительных пакетов/typosquatting нет. `npm audit --omit=dev` → 0; полный аудит — только описанный LOW (dev-цепочка minimatch).
11. **QA-отчёт** (/var/.../m4-final-qa-report.md): секретов нет — PID/порты/item_id/table_id, dev-DSN упомянут как не-секрет; cookie-значений и паролей нет.
12. **Остаточные риски checkpoint** (TLS proxy / Secure cookie → M5): подтверждено — не CRITICAL/HIGH для v0-пилота в локальном окружении (детали в INFO №4); обязательное условие публичного запуска.
13. **Незакоммиченный diff docs/10-requirements.md**: только заполнение колонки «Проверено» (UI-01..04, NFR-01..03, FR-15/24/29/31, AN-02, FR-36/37) — security-контента нет, секретов нет. Безопасно к коммиту.

## Рекомендации для M5 (сводно, по приоритету)

1. TLS reverse proxy + Secure cookie (всегда за прокси) + HSTS; осознанная передача Proto одним доверенным хопом; интеграционный тест за прокси по образцу TestNFR04_ProxyMustPreserveHost. **Блокер публичного пилота.**
2. Добавить Content-Security-Policy (значение — в INFO №2) и, вместе с ним, X-Frame-Options: DENY / frame-ancestors 'none' (сейчас кликджекинг не закрыт заголовком; SameSite=Lax cookie частично компенсирует).
3. Cache-Control immutable для /static/*/assets/fonts/*.woff2.
4. Обновить dev-цепочку typescript-eslint при появлении patched minimatch (LOW, непрод-влияние).
5. 404-JSON для неизвестных GET /api/v1/* вместо SPA index.html (косметика API-семантики).
6. При планах на >1 инстанс бэкенда — вынести rate-limit в разделяемый стор (решение зафиксировать в ADR до масштабирования).
7. Появление upload-функционала (v1+) — EvalSymlinks в static-хендлере.

## Сводка

| Severity | Кол-во |
|---|---|
| CRITICAL | 0 |
| HIGH | 0 |
| MEDIUM | 0 |
| LOW | 1 |
| INFO | 7 |

**M4 закрытие: OK** (гейт «нет CRITICAL/HIGH» пройден; LOW не блокирует). Репо не изменялось; коммитов/push не выполнялось.
