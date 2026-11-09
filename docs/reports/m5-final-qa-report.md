# M5 Final QA Acceptance

Версия evidence: **`m5-auth-pgx-r1/run-02`**, дата 2026-11-08, финальный browser-прогон 21:08:12 UTC.
Роль: QA + аналитик. **QA: OK в проверенном локальном скоупе. Аналитик: OK events-контракту и export CSV.**
Это явный loop-апрув аналитика для `docs/30-contracts/events.md` (включая §4), не подмена
обязательных апрувов архитектора/ревьюера требований/security и не самостоятельное закрытие M5.
Коммитов, изменений production-кода/зависимостей со стороны QA и операций с исходным стеком не выполнялось.
Повторная независимая приёмка после auth hardening ADR-0005, migration 0003 и pgx/v5 v5.11.0.

## Results

| Проверка | Фактический результат |
|---|---|
| Browser/API/auth/CSV acceptance через Go backend | **21/21 групп PASS**, 375×812 и 1440×900, Chromium; не Vite |
| Отдельный одноразовый cutover 0002 → 0003 | PASS: полная сохранность старых строк/связей и CSV, legacy replay отклонён |
| Backend `go test -count=1 ./...` | **137 top-level / 258 с subtests PASS**, 0 skip, включая 19 integration |
| Backend `go test -race -count=1 ./...` | **137 top-level / 258 с subtests PASS**, 0 skip; race findings нет |
| Frontend `npm run lint && npm run test && npm run build` | **PASS, 74 теста / 6 файлов** после параллельных frontend security-правок |
| `golangci-lint run`, `go vet ./...`, `git diff --check`, HTTP smoke | PASS, golangci-lint: 0 issues |
| `npm audit` / `npm audit --omit=dev` | 7 moderate / **0**; high/critical 0. Dev-цепочка typescript-eslint/minimatch, GHSA-g3hv-75wg-m249; `audit fix` не запускался |

Тесты/subtests не суммируются как независимые тесты. Интеграционные тесты обоих прогонов
направлены **только в disposable `m5_integration` на 55432**, с проверкой имени БД перед запуском.
Funnel и CSV проверялись в отдельной `svoyshef` isolated-проекта, без TRUNCATE/синтетических
граничных event-вставок. Auth expiry менялся только у новых тестовых сессий. Диагностические
повторы оставлены в этой тестовой БД; это не данные пилота.

## Immutable Evidence

Это фиксированный snapshot `m5-auth-pgx-r1/run-02`, не утверждение о будущих изменениях рабочей БД.
Исходные evidence не перезаписаны: новые `cutover-summary.json` и `run-02/acceptance-summary.json`
созданы с `wx`/0600 в temp `m5-final-qa/auth-pgx-r1/`. Для следующего запуска нужен новый run-каталог.
Секреты, cookie, credential hashes, raw session UUID и строки событий в summary не сохраняются.

| Артефакт snapshot | Значение |
|---|---|
| Проверенный runtime image | `sha256:347a519f4ec347b84d64f9bb35d5846d08971d66b5170e41a5fb6eb3c092014f` |
| SHA256 источника `m5-acceptance.mjs` | `41aad153512e8634afa8f016699e1722c31dd82c8367bbc00b6049448a6000c1` |
| SHA256 исторического CSV, одинаков до/после cutover | `5bc53a16c1a47ff8535b2c1ffffb58eb63ce5ccaa1c4fe3f228b87b805decdb2` |
| SHA256 полного исторического research snapshot | `aa8b7e5bdc3c5075926ba05f4b215ffb8a5707708ad9a1c9795308ef908efa14` |

Последние два digest относятся к исследовательским данным целиком, не к credential.
Долговечный sanitized summary содержится в этом отчёте; воспроизводимые assertions находятся в тестах.

## Auth Cutover

Прочитаны актуальные API/events/ADR-0005/план. [`m5-cutover.mjs`](../../frontend/tests/m5-cutover.mjs)
проверил исходную схему 0002, **остановил старый isolated-backend до build/up**, дождался healthy
новой версии и auto-migrate 0003. До миграции: **30 sessions, 10 carts, 2 cart_items, 496 events**.
Весь snapshot sessions (без новых nullable auth-колонок)/carts/cart_items/events/availability и
связи совпали побайтово; 30 legacy-сессий остались с обоими auth-полями NULL, без backfill.
Старый CSV совпал побайтово. Реальный legacy ID-cookie дал новый cookie/пустую корзину/table=24,
не удалив старые cart_items/events. Никакого rollback/down или смешивания версий не было.

Независимый HTTP-тест на собранном backend подтверждает: credential UUID ≠ analytics UUID,
в БД только 32-byte SHA256; корреляция выполняется hash-lookup в памяти теста. Настоящий cookie
(также uppercase) переиспользует исходную корзину без Set-Cookie, изменения table или expiry;
серверный lifetime равен Max-Age. Подстановка **UUID из свежего CSV в cookie** не даёт исходную
корзину; отдельно UUID исторической legacy-сессии из сохранённого CSV с непустой старой корзиной
также даёт только новую пустую сессию/table=48, старая корзина остаётся в БД. Unknown/malformed/expired cookie дают
новую пустую сессию. Expiry не продлевается; проверены прошлый срок и точное равенство
`expires_at = now()` в одной SQL-транзакции: `expires_at > now()` ложно, затем HTTP-cookie отвергнут.
Историческая корзина после expiry сохранена. Health не создаёт cookie/сессию.
На обоих viewport события сопоставлены с analytics ID через hash настоящего HttpOnly cookie,
а не через ошибочное равенство cookie=session_id. JSON/CSV/runtime logs не содержат проверенных
credential/hash, runtime logs также не содержат проверенных analytics UUID (9 credentials/9 IDs).
DB/RNG fail-closed, schema constraints и смена env без продления срока дополнительно PASS в backend-тестах.

## Acceptance

Воспроизводимый источник: [`frontend/tests/m5-acceptance.mjs`](../../frontend/tests/m5-acceptance.mjs).
Названия 21 группы содержат ID требований; assertions падают при нарушении, константных PASS нет.
Следующие 19 ранее пустых M4-строк реестра закрыты проверкой на backend-served сборке:

| ID | Проверенные AC, оба viewport, если не указано иначе |
|---|---|
| FR-01 | Только штатные ссылки/4 роута; ревью `App.tsx`; полный путь с возвратом в меню |
| FR-03 | Href/name/price всех 125 карточек = API; реальный клик хинкали открывает корректный `/dish/:id`; общий компонент и TestFR03_* |
| FR-04 | Cursor pointer; desktop hover меняет фон/рамку; все кнопки/ссылки проверенных экранов ≥44×44 px |
| FR-05 | Фото → info → состав → рекомы, DOM/геометрия; нет горизонтального переполнения |
| FR-06 | Цены всех 125 карточек = API; цена хинкали 530 ₽ на детальном экране |
| FR-07 | Live КБЖУ+description и только description = API; оба пустые/пробельные покрыты 4 TestFR07_composition_* |
| FR-09 | После состава, 6 карточек в одну колонку и в порядке API, реальные viewport events |
| FR-13 | Точный подзаголовок, визуальный разделитель/отступ; screenshots recommendations просмотрены |
| FR-14 | Rec add: badge 0→1, правильные item_id/context=dish в БД, URL и document marker сохранены; тело не навигирует |
| FR-17 | Все линии/price/qty = API, локальные превью реально декодированы, qty>1 виден |
| FR-18 | Крестик удаляет выбранную линию, итог пересчитан, cart_item_remove в БД |
| FR-23 | Total = Σprice×qty после каждого действия, sticky pay внутри viewport; empty без pay/recs |
| FR-25 | «Все» по умолчанию/возвращает 125; «Хачапури» = категория API, активный #fdc300, category_select с ID в БД |
| FR-26 | CDN 404, primary resize/400x400, прокрутка всех 125 lazy-фото: src=локальный fallback, complete/naturalWidth>0; hero/cart тоже декодированы |
| FR-27 | Название/logo src из config; badge = Σqty, клик → /cart |
| FR-30 | Верхняя кнопка рядом с ценой, добавление даёт линию и menu_add_click |
| FR-32 | Повторное add → qty2; степпер 2→1→2→20, totals/badge/event; плюс disabled на 20 |
| FR-35 | route.abort GET /cart вместо остановки сервера; дружелюбный текст без техдеталей; unroute + «Повторить» восстанавливает данные |
| NFR-06 | Ревью строк страниц/компонентов и DOM всех состояний: русский UI; Borjomi в названии товара не является UI-label |

Дополнительно PASS: ForCart ровно одна рекомендация, добавление `source=rec_cart` меняет состав/
рекомендацию без навигации; пустая корзина; точный текст заглушки; pay_click перед stub_view.
Availability off при живом сервере подхвачен TTL: линия приглушена, «Нет в наличии», remove/qty
доступны, pay не навигирует и не создаёт pay_click/stub_view. **В `finally` восстановлено исходное
состояние availability, включая отсутствие строки**, on подтверждён API/UI. В обычном потоке
выполняются только тестовые cart/event-операции; backend не останавливается.

Шрифты: по 5 same-origin font requests на viewport, внешних font requests нет; computed styles
Unbounded/Montserrat и реальные custom glyphs обоих семейств подтверждены CDP. Все 6 WOFF2
проверены HTTP smoke. На menu/dish/recs/cart/unavailable/error/empty/stub нет forms/input и overflow;
pageerror/CSP violations: 0. Скриншоты menu/dish/recommendations/cart/unavailable просмотрены.

## Analytics

**Аналитик: OK** текущей таксономии/разделению источников/схемам/экспорту по `events.md`.
Все 13 имён фактически присутствуют и в БД, и в CSV: page_open, menu_view, category_select,
dish_card_open, rec_feed_visible, rec_impression, menu_add_click, rec_add_click, cart_open,
cart_qty_change, cart_item_remove, pay_click, stub_view. В финальном пути: 50 событий на 375,
52 на 1440; все со столиком 12; rec_add_click проверен в обоих contexts. Новые сессии с `table=abc`
и без параметра работают и дают NULL. UUID/cookie/payload raw в отчёт не включены.

AN-04: **710 строк событий**, включая 496 сохранённых исторических, побайтово сверены с каноническим
`payload::text` БД в порядке `ts,id`; 5 вариантов: без флагов, только from, только to, оба,
пустая выборка. Header обязателен, UTF-8, LF/no BOM, UTC/6 знаков микросекунд, NULL → пустое поле,
payload всегда quoted (включая `"{}"`), внутренние кавычки удвоены. В disposable integration:
точные from/последняя микросекунда to/to+1, одинаковый ts с tie-break id, CSV reader roundtrip.
Контейнерный CLI: 6 невалидных комбинаций (включая пустые даты) и недоступная БД дают exit1,
stderr непуст, stdout пуст; дополнительные process cases покрыты TestAN04_ExportEvents_CLIProcessErrors.

AN-05: ревью `tracker.ts`, `internal/app/events.go`, серверных cart mutations и всех 13 схем:
только ID/числа/закрытые enum, свободного ввода/гео/устройств/ПДн нет. Все 710 реальных payload
проверены на точный набор ключей, значения и размер ≤1КБ. TestAN05_* подтверждают отклонение
неизвестных/недостающих полей, свободных строк, вложенных объектов и недопустимых enum.
**Аналитик: OK** обновлённому events.md §5: CSV содержит только analytics ID, не auth;
старые исследовательские UUID и строки сохранены, но не дают доступа к корзине после cutover.

## Deployment

Проект `vkr-m5-acceptance`, URL `http://127.0.0.1:18080`, БД 55432; private env вне репо, mode 0600.
QA выполнил isolated `stop backend`, `build backend`, `up -d --wait backend` из актуальных исходников
после auth/pgx hardening; оба сервиса healthy. Контейнерный регресс: empty date → exit1,
unknown API → 404 JSON; CSP и backend-served SPA/deep links/assets/fonts PASS.
Runtime image зафиксирован выше; frontend bundle `index-Dn1FsijV.js` (после ужесточения CDN allowlist).
NFR-05: [SRE clean-source/no-cache evidence предыдущей ревизии](../../scripts/M5-SRE-RESULT.md) +
новая сборка auth/pgx и независимый QA HTTP/browser smoke. No-cache после auth отдельно не повторялся.
Это isolated volume с сохранением существующих research rows, не новая физическая ОС;
18080 заменяет default 8080 только для изоляции. Исходные контейнеры 8080/5432 не менялись:
IDs `3a2b911d8482` / `910d73730003` совпадают с SRE handoff.

## Reproduce

Из корня репозитория, при healthy isolated-стеке SRE и уже установленном внешнем Playwright:

```bash
export COMPOSE_ENV_FILES=/var/folders/w8/25fckp8n09597vtwlt8wgnhw0000gn/T/opencode/m5-final-qa/acceptance.env
export PLAYWRIGHT_MODULE=/var/folders/w8/25fckp8n09597vtwlt8wgnhw0000gn/T/opencode/m4-qa-prep/node_modules/playwright
export PLAYWRIGHT_BROWSERS_PATH=/var/folders/w8/25fckp8n09597vtwlt8wgnhw0000gn/T/opencode/m4-qa-prep/browsers
export M5_EVIDENCE_DIR=/var/folders/w8/25fckp8n09597vtwlt8wgnhw0000gn/T/opencode/m5-final-qa/auth-pgx-r1/run-03
node frontend/tests/m5-acceptance.mjs
python3 scripts/smoke.py --base-url http://127.0.0.1:18080
(set -a; . "$COMPOSE_ENV_FILES"; set +a; [[ "${M2_IT_DATABASE_URL%%\?*}" == *:55432/m5_integration ]] && go -C backend test -race -count=1 ./...)
npm --prefix frontend run lint && npm --prefix frontend test && npm --prefix frontend run build
```

Новые зависимости не добавлены. Сценарий жёстко привязан к isolated-проекту/порту и проверяет
Compose labels. Temp evidence: `auth-pgx-r1/{cutover-summary.json,run-02/acceptance-summary.json}`,
`run-02/{375,1440}-*.png`, `auth-pgx-r1/npm-audit*.json`; backend JSONL:
`m5-final-qa/backend-auth-{integration,race}.jsonl`. Cutover-test одноразовый: на уже migrated БД
намеренно откажется запускаться; не откатывать funnel DB ради его повторения. Повторяемая migration
проверка с legacy fixtures есть в disposable backend `TestNFR04_Integration_MigrationPreservesRowsAndEnforcesConstraints`.

## Limits

- Физические iOS Safari/Android Chrome, камера/QR, принтер и скан распечатки не проверялись; Chromium viewport не заменяет device acceptance.
- Оба состава пусты проверены component tests, поскольку live-меню такого блюда не содержит; не выдаём это за live-тест.
- NFR-02 Lighthouse заново на compose не запускался; прежний M4 performance result здесь не расширяется.
- HTTP loopback acceptance не доказывает готовность публичного TLS/proxy-пилота. FR-08 явно Won't/v1, вне v0, не «да».
- Полный npm audit не чист: 7 moderate в dev-tools; runtime audit 0. Исправление зависимостей не входит в QA write-scope.
- Backend `govulncheck` в этой QA-сессии не повторялся: бинарник недоступен; результат backend-агента не выдаётся за независимый QA-прогон. Независимо повторены test/race/vet/golangci-lint.
- Security-сабагент недоступен в инструментах этой QA-сессии; независимый финальный M5 security-аудит/апрувы остаются гейтом координатора, QA их не присваивает.
