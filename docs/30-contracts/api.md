# 30.2. Контракт: API v1

Базовый префикс `/api/v1`. Формат ответов — JSON (UTF-8). Ошибки:

```json
{ "error": { "code": "not_found", "message": "Позиция не найдена" } }
```
- Коды: `bad_request` (валидация), `not_found`, `item_unavailable`, `rate_limited`, `internal`.
- `409 item_unavailable` — операция с недоступной позицией (`is_available=false`, FR-36/37):
  только `POST /cart/items`; `item_id` передаётся внутри `error`:
  `{ "error": { "code": "item_unavailable", "message": "Позиции нет в наличии", "item_id": 123 } }`.
  Серверной операции оформления заказа в v0 нет (оплата — фронтовая заглушка FR-24): фронт сам
  блокирует переход при `available=false` (UI-04), события `pay_click`/`stub_view` при этом не шлются.
- Наружу НЕ выносить стеки, пути, детали БД (NFR-04).
- HTTP-статусы: 400 / 404 / 409 / 429 / 500.

Сессия (FR-33): middleware на каждый запрос `/api/v1` (кроме `/api/v1/health` — healthcheck сессий
не создаёт и IP-лимит не расходует) проверяет cookie `session`; если действующей сессии нет — создаёт credential UUIDv4,
ставит `Set-Cookie: session=<uuid>; HttpOnly; SameSite=Lax; Max-Age=<COOKIE_MAX_AGE_H×3600>; Path=/`
(Max-Age вычисляется из env, 43200 — пример при 12ч; атрибут `Secure` добавляется всегда, когда
запрос пришёл по TLS, в деве без TLS — опускается) и создаёт запись `sessions`. `table_id`: фронт
при старте, если в URL документа есть валидный `?table=N`, добавляет `?table=N` к первому API-запросу
в рамках загрузки страницы (например, `GET /api/v1/menu?table=12`); сервер учитывает `table` только
при создании сессии, в живой сессии повторные сканы table не меняют. Фронт обязан завершить
первый API-запрос (`GET /menu`, с `?table=N` при наличии) до отправки любых других /api-запросов
(сериализация старта) — исключает гонку создания сессий и потерю table_id. Любое создание новой
сессии — IP-лимит 30 новых сессий/мин/IP (in-memory, при превышении `429`).

Auth hardening (FR-33, NFR-04; ADR-0005):
- `sessions.id` — аналитический UUIDv4, не credential; cookie содержит независимый UUIDv4.
  Оба генерируются независимо криптографическим RNG; ошибка RNG — без fallback.
- В БД хранится только `SHA256(lowercase canonical UUID credential)` (32 байта), не сам credential.
  Auth lookup — только по `credential_hash` AND `expires_at > now()`, никогда по `sessions.id`.
- `expires_at = created_at + COOKIE_MAX_AGE_H` часов — абсолютный серверный срок; повторное
  использование его не продлевает, изменение env уже созданные сроки не меняет.
- Отсутствующий, malformed, unknown или expired cookie → новая анонимная сессия, table из
  текущего запроса, пустая новая корзина и новый cookie с обычным лимитом создания сессий.
  Ошибка БД → fail-closed (`500 internal`), не создание сессии и не legacy fallback.
- Имя, UUID-форма и атрибуты cookie не меняются; credential/hash запрещены в JSON, CSV и логах.
  Legacy cookies намеренно недействительны после cutover; порядок перехода — ADR-0005.

## Эндпоинты

### `GET /api/v1/health`
`200 {"status":"ok","restaurant":"aidabaran"}` — для compose healthcheck.

### `GET /api/v1/config`
```json
{
  "restaurant": { "name": "АйДаБаран", "logo_url": "/static/aidabaran/logo.png" },
  "rec_block_title": "Хорошо сочетается с"
}
```
Значения приходят из плагина. Ничего ресторанного не хардкодится во фронте.

### `GET /api/v1/menu`
Категории с позициями одним ответом (меню целиком, 125 позиций — нормальный размер).
```json
{ "categories": [
  { "id": 1, "name": "Салаты",
    "items": [ { /* MenuItemSummary по схеме data-model §2 */ } ] }
]}
```
Порядок категорий и позиций — как на сайте ресторана (SortOrder). Позиции — краткая схема
`MenuItemSummary` (data-model §2; без описания/КБЖУ/апсейлов — они в карточке; фронту ок
показывать карточку каталога без них).
Недоступные позиции (`is_available=false`, FR-36/37) из выдачи исключаются; категории,
оставшиеся без позиций, исключаются целиком — вместе с `id` (чипы пустых категорий не
показываются).

### `GET /api/v1/menu/items/{id}`
Полный `MenuItem` (включая `description`, `kbju`, `upsell_options`) — только этот эндпоинт
возвращает полную схему (data-model §2). `upsell_options` фильтруются по доступности:
недоступные опции не возвращаются (FR-37). `404`, если позиции нет ИЛИ она недоступна
(FR-37: недоступная = несуществующая).

### `GET /api/v1/menu/items/{id}/recommendations`
Лента карточки (FR-10): по одной топ-позиции из каждой категории-кандидата (свою категорию
исключаем), уже лежащие в корзине сессии исключаются (FR-28).
```json
{ "recs": [ { "item": {…}, "reason_text": "…", "source": "expert" } ] }
```
Порядок: score DESC → ID категории кандидата ASC (порядок категорий как на сайте/в menu.json) →
SortOrder позиции ASC (полный порядок — `30.3 recommender.md` §1). `recs[].item` — `MenuItemSummary`.
Кандидаты фильтруются по доступности после каскада (30.3 «Доступность», FR-37). Пустой массив =
блок скрывается (FR-29). `404`, если позиции `{id}` нет или она недоступна (как
`GET /menu/items/{id}`).

### `GET /api/v1/cart`
Текущая корзина сессии + динамическая топ-1 рекомендация (FR-19/20) одним ответом.
```json
{
  "lines": [ { "item": {…}, "qty": 2, "added_from": "rec_feed", "available": true } ],
  "total": 2780,
  "recommendation": { "item": {…}, "reason_text": "…", "source": "expert" }  // или null
}
```
`recommendation = null`, когда корзина пуста или кандидата нет. `lines[].item` и
`recommendation.item` — `MenuItemSummary`. Позиции меню, исчезнувшие
из данных, при чтении отбрасываются (ADR-0003).
`available=false` — позиция стала недоступной после добавления (FR-37): линия остаётся
(удаление и изменение qty работают как обычно), фронт показывает её приглушённо с бейджем
«Нет в наличии» (UI-04).

### `POST /api/v1/cart/items`
```json
{ "item_id": 876201943922, "source": "menu" | "rec_feed" | "rec_cart" }
```
Добавляет 1 шт (или qty+1, если уже лежит, макс 20 — иначе `bad_request`). Несуществующий
`item_id` (валидный по формату, но отсутствующий в меню) → `404 not_found`. Недоступная
позиция → `409 item_unavailable` (в теле ошибки `item_id`, FR-37). `201` → тот же
ответ, что `GET /cart`. Сервер при `POST /api/v1/cart/items` сам записывает событие `menu_add_click`
(source=menu) или `rec_add_click` (source=rec_feed|rec_cart; payload: item_id, context).
Фронт эти события не шлёт (источник правды — 30.4).

### `PATCH /api/v1/cart/items/{item_id}`
`{ "qty": 3 }` (1..20; qty=0 запрещён — для удаления DELETE). Ответ как `GET /cart`.

### `DELETE /api/v1/cart/items/{item_id}`
Удаляет строку целиком. Ответ как `GET /cart`. `404`, если позиции в корзине нет.

### `POST /api/v1/events`
Клиентские события UX (таковые — только из таксономии 30.4):
```json
{ "name": "rec_impression", "payload": { "context": "dish", "rec_item_id": 366609864022 } }
```
`204`. Невалидное имя/payload → `400`. Rate-limit: ≤ 120 событий/мин/сессия → `429` (анти-спам
NFR-04; rec_impression — до 13 на карточку, лимит не должен резать живого гостя); счётчик —
in-memory на процесс (map session→таймстампы, чистка старше 60s); фронт при
`429` молча дропает событие без ретраев. Считаем достаточным для v0-пилота (один ресторан).
Сервер НЕ валидирует item_id из payload событий (аналитика не должна ломать UX).

### `GET /api/v1/…` — чего НЕТ и не должно появиться
- Никаких admin/export HTTP-эндпоинтов (экспорт — CLI-подкоманда AN-04; управление наличием —
  CLI `availability`, FR-36).
- Никакой оплаты (FR-24 — фронтовая заглушка), никаких форм ввода.

## Статика (прод)
- `GET /static/<restaurant>/…` → `data/<restaurant>/assets|photos` (logo, локальные фото-fallback;
  шрифты: `/static/aidabaran/assets/fonts/<file>.woff2` → `data/aidabaran/assets/fonts/`).
- `GET /*` → SPA `frontend/dist` (fallback на `index.html` для клиентских роутов).
