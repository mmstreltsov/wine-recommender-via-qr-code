# 50. План работ и вехи

Правило: контракт (`30-contracts`) не меняется в ходе вехи; потребность в изменении = стоп и
пересмотр контракта до кода. После каждой вехи — security-аудит (CRITICAL/HIGH блокируют следующую).

## M1 — Данные и матрица рекомов (аналитик + backend)
Сделать:
1. `plugins/aidabaran/loader.go`: menu.json → `[]domain.MenuItem` по маппингу `30.1` (+ тесты на
   грани: позиция без цены, без фото, без КБЖУ).
2. `data/aidabaran/recommender/matrix.json` — **наполнение контентом**: пары «блюдо → дополнение»
   с текстами по чек-листу FR-12 (цель: покрытие ≥ 80% блюд, каждая категория как целевая).
   Черновики текстов генерирует LLM оффлайн → пакетное ревью по §5 `30.3` (в рантайме LLM нет —
   решение скоупа v0, ADR-0004).
3. `heuristics.json` (источник правды — файл, 14 правил). Ревью аналитиком `forcart_priorities.json`
   (приоритеты «недостающего курса» для ForCart).
4. `upsell_map.json`: перед заполнением — инвентаризация (таблица всех уникальных upsell-текстов
   × «есть ли соответствующая позиция в меню»); заполняются только реальные совпадения; пустой
   `upsell_map.json` (`"map": {}`) — валидный результат, не ошибка.
5. Валидатор матрицы (CLI `cmd/validate-matrix`).
6. Загрузка фото-fallback: `python3 data/aidabaran/parser.py photos` (локально, в git не коммитим).
DoD: валидатор зелёный (запуск: `cd backend && go run ./cmd/validate-matrix`); загрузка меню даёт
125 позиций/14 категорий; матрица проходит чек-лист текстов (ревью).

## M2 — Core backend (backend)
Сделать: goose-миграции; сессии/корзина/события (pgx, репозитории за интерфейсами); хендлеры по
`30.2` (menu из плагина, cart, events, config, health); middleware (cookie-сессия, logging,
recovery, rate-limit events); в начале M2 — интерфейс `internal/recommender` (типы контракта
`30.3`, стаб-реализация до M3 — иначе M3 не параллелится); учёт наличия (FR-36/37): миграция
`item_availability`, TTL-кэш `AVAILABILITY_TTL_S`, CLI `availability set/list/find`, фильтры
выдач (меню/карточка/рекомы/корзина), ошибка `409 item_unavailable`; интеграционные тесты API
(compose-db).
DoD: `docker compose up db` + сервер — все эндпоинты отвечают по контракту; тесты/lint зелёные;
grep-изоляция internal (нет «aidabaran»); события menu_add_click/rec_add_click пишутся сервером;
выключение позиции CLI при живом сервере исчезает из `/menu` в пределах TTL (фильтр рекомендаций
проверяется в M3), добавление её в корзину даёт `409 item_unavailable`.

## M3 — Реком-модуль АйДаБарана (backend, параллельно с M2 после фиксации контрактов)
Сделать: реализация `Recommender` (ForDish каскад, ForCart правила, приоритеты из
`forcart_priorities.json`) по `30.3`; детерминированные unit-тесты на фиксированном
fixture-меню (эксперт/upsell/heuristic/exclude/пусто); отдельная задача — wire движка в хендлеры
M2 (DoD: `/items/{id}/recommendations` и `recommendation` во всех ответах корзины
(GET/POST/PATCH/DELETE) считаются реальным движком, не стабом).
DoD: тесты покрывают все ветки каскада; ручная проверка: `/items/{хинкали}/recommendations`
возвращает осмысленные пары с текстами; фильтр наличия: выключенная CLI позиция не появляется
в рекомендациях (лента карточки и ForCart), опустевшая лента → блок скрыт.

## M4 — Frontend (frontend)
Сделать: скачать woff2 (Unbounded 300–700, Montserrat, кириллица) и закоммитить в
`data/aidabaran/assets/fonts/`; каркас Vite+React+TS, тема aidabaran, все 5 экранов/состояний
по `40-frontend`, API-клиент, трекер событий (клиентская часть AN-02), PhotoWithFallback;
UI корзины для недоступной линии — приглушение, бейдж «Нет в наличии», блокировка оплаты (UI-04).
DoD: полный путь гостя (FR-путь) проходит на мобильном вьюпорте 375px; тесты компонентов на
ключевые AC (FR-15 no-navigation, FR-29 скрытие блока, FR-31 пустая корзина, UI-04 недоступная
линия); lint/тесты зелёные;
визуальное сравнение с сайтом ресторана (UI-01…03).

## M5 — Аналитика, деплой, финал
Сделать: `export-events` CLI (AN-04; правила — 30.4 §4: границы-полуинтервал, обязательный
заголовок, LF/без BOM, ошибки в stderr + exit 1); docker-compose полный (`docker compose up`
без флагов = db+backend, backend раздаёт dist+static; env `FRONTEND_DIST` проводятся в main.go;
бинд-маунт `./data` — 20-architecture §7); seed QR: параметрический ops-скрипт в `scripts/`
(входы: base URL, диапазон столиков, подмножество A/B; генерирует страницу/CSV для печати QR
`?table=N`; вне реестра требований — ops-tooling; число столиков пилота уточнит заказчик,
дефолт 1–20); CSP-заголовок (defense-in-depth); прогон воронки вручную + сверка CSV;
ретро-трассировка: закрыть 19 пустых M4-строк (FR-01, FR-03, FR-04, FR-05, FR-06, FR-07, FR-09,
FR-13, FR-14, FR-17, FR-18, FR-23, FR-25, FR-26, FR-27 (Should), FR-30, FR-32, FR-35, NFR-06)
прогоном на прод-compose со ссылками на артефакты; финальные M4-отчёты QA/security —
в `docs/reports/`; security-аудит финальный; заполнение колонки «Проверено» в
`10-requirements.md`.
M5 security regression (FR-33, NFR-04, AN-04): независимые session_id/credential, hash-only auth,
абсолютный expiry, fail-closed БД/RNG, сохранность legacy-данных и отказ ID-auth (включая CSV UUID);
cutover с предупреждением гостей и остановкой старых бинарников — ADR-0005; DSN только env,
без секретов/sessionID/raw pgx в логах; обратный диапазон export — exit 1, stdout пуст.
Поставка M5 локальная, порты только loopback; публичный деплой/TLS вне M5 (20-architecture §7).
DoD: на чистой машине `docker compose up` → рабочий сайт на `127.0.0.1:8080` (NFR-05 обязателен для
закрытия M5, несмотря на Should в реестре); экспорт CSV валиден; аудит без CRITICAL/HIGH.

### Закрытие M5 (09.11.2026)

**M5 закрыта в локальном скоупе прототипа, все обязательные гейты закрыты; это не production/TLS approval.**
Финальные независимые ответы: requirements-reviewer **PASS / OK** закрытию M5 и security
**OK security-docs**, локальный security gate остаётся **PASS**, 0 CRITICAL/HIGH/MED/LOW.
Это подтверждение документации, не новый runtime-аудит; проверки и сканеры не повторялись.
Аналитик: OK events/CSV. [Финальный review-report](reports/m5-final-review-report.md) фиксирует
основания; координатор финализирует закрытие после передачи документации.

- [QA `m5-auth-pgx-r1/run-02`](reports/m5-final-qa-report.md): 21/21 групп на 375/1440 через backend; Go 137 top-level / 258 с subtests, 19 integration, 0 skips, normal+race; frontend 74 теста, lint/build PASS. Закрыты 19 ранее пустых M4-строк трассировки.
- [Security PASS](reports/m5-security-final-report.md): 0 CRITICAL/HIGH/MED/LOW, 3 MED устранены (credential отдельно от analytics ID, безопасные логи, DSN только env). Полный npm audit: 7 moderate одной dev-цепочки, runtime 0; это не чистый полный audit.
- Requirements-reviewer: проверены все 45 Must (32 FR / 4 UI / 5 AN / 4 NFR), пустых «Проверено» нет; сохранённые sanitized summaries совпали с QA evidence. Единственный MINOR в `20-architecture.md:61` исправлен: порядок `expert → upsell → heuristic`, без изменения контракта или кода.
- [SRE](../scripts/M5-SRE-RESULT.md): clean-source/no-cache build предыдущего pre-auth snapshot PASS; QA пересобрал текущий auth-образ обычным build, no-cache заново не повторялся. NFR-05 проверен изолированно на 18080/55432, не на новой физической ОС.
- Cutover сохранил 30 sessions / 10 carts / 2 items / 496 events и исторический CSV побайтово; QA snapshot: 710 событий, все 13 имён. Перед реальным cutover остановить все старые бинарники; legacy cookies → новая пустая корзина, история сохранена; rollback к ID-auth запрещён.
- Принят только `vkr-m5-acceptance`, loopback `127.0.0.1:18080/55432`; после локального просмотра остановлен по просьбе пользователя, volumes сохранены. Исходный стек 8080/5432 не обновлялся; прежняя публикация старого backend на всех интерфейсах известна из исторического SRE evidence, текущий статус не проверялся и вне PASS.
- QR/PDF печать проверена, физические телефоны/камера/принтер/скан не проверены. Lighthouse 70/70/70 остаётся результатом M4 preview, без повторного compose-прогона. Публичный TLS/proxy-пилот требует отдельной работы и аудита.

## Порядок и зависимости

```
M1 ──► M2 ──► M4 ──► M5
 │      ▲
 └─► M3 ┘ (M3 после M1, параллельно с M2; интеграция в M2-конце)
```

## Определение готовности прототипа (весь проект)
Все Must-требования `10-requirements.md` в колонке «Проверено» = да; funnel-прогон создаёт
полный набор событий; известные Won't — задокументированы.
