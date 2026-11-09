# 20. Архитектура

## 1. Обзор

```
Гость сканирует QR (?table=N)
        │
        ▼
 [SPA Frontend]  Vite+React+TS, статика — в проде раздаётся backend'ом
        │  /api/v1/*  (same-origin в проде; vite-proxy в деве)
        ▼
 [Core Backend]  Go, stdlib net/http        ← НЕ знает специфики ресторана
   ├─ menu      (модуль домена: категории/позиции через интерфейс источника)
   ├─ cart      (корзина сессии в Postgres)
   ├─ session   (анонимный UUID-cookie, table_id)
   ├─ events    (воронка в Postgres)
   ├─ recommender (ИНТЕРФЕЙС, реализации — плагины)
   └─ store     (pgx-репозитории за интерфейсами)
        │                    ▲
        ▼                    │ реализует
 [PostgreSQL]      [plugins/aidabaran]  ← СПЕЦИФИКА ресторана:
  sessions,           ├─ загрузка data/aidabaran/menu.json → доменные типы ядра
   carts, cart_items,  ├─ матрица пар + тексты (data/aidabaran/recommender/)
   events, item_availability  └─ Recommender: ForDish / ForCart
```

Композиция — **build-time**: `cmd/server/main.go` читает env `RESTAURANT=aidabaran` и собирает
сервер с соответствующим плагином. Никаких рантайм-плагинов Go (`plugin` package) — это хрупко
и не нужно: добавление ресторана = новый пакет в `plugins/` + строка в registry + его data-каталог.

Наличие блюд (FR-36): сервер держит in-memory множество недоступных `item_id` — TTL-кэш над
таблицей `item_availability` (env `AVAILABILITY_TTL_S`, по умолчанию 5 с). Фильтр применяется ко
всем выдачам — меню, карточки, рекомендации (после каскада) и корзина (FR-37); изменения,
внесённые CLI при живом сервере, подхватываются без рестарта в пределах TTL (ADR-0006). При
ошибке обновления кэша (БД недоступна) сервер работает по последнему успешно загруженному срезу
и логирует ERROR (не fail-open, не 500).

## 2. Статика vs динамика (ключевое решение)

| Что | Где живёт | Почему |
|---|---|---|
| Меню, категории, матрица пар, тексты рекомов | Файлы `data/<restaurant>/`, в памяти процесса при старте | Статика прототипа; обновление = перезапуск. Нет лишней БД-синхронизации |
| Сессии, корзины, события | PostgreSQL | Динамика; данные исследования |
| Наличие блюд (отключения) | PostgreSQL `item_availability`, чтение — TTL-кэш в памяти сервера | Операционное состояние зала, меняется без релиза; default-available (пустая таблица = всё доступно, FR-36) |
| Фотографии | CDN ресторана (primary) + локальные копии (fallback через `onerror`) | Ноль хранения и трафика; см. `data/aidabaran/README.md` |

## 3. Структура backend

```
backend/
├── cmd/server/main.go          # композиция: config → store → plugin → http server; подкоманды export-events, availability (set/list/find)
├── internal/
│   ├── api/                    # http-хендлеры по контрактам docs/30-contracts/api.md, middleware (сессия, logging, recovery, rate-limit)
│   ├── domain/                 # чистые типы ядра: Category, MenuItem, Cart, CartLine, Rec, Event (без ресторанной специфики)
│   ├── recommender/recommender.go  # interface Recommender + типы Rec/RecSource
│   ├── store/                  # интерфейсы репозиториев + pgx-реализации (sessions, carts, events, availability)
│   └── app/                    # сервисы: MenuService, CartService, EventsService; TTL-кэш наличия (FR-36)
└── plugins/
    └── aidabaran/
        ├── loader.go           # menu.json → []domain.MenuItem (маппинг полей см. data-model.md)
        ├── recommender.go      # реализация interface: expert → upsell → heuristic
        ├── texts.go            # шаблоны текстов для эвристических пар
        └── testdata/           # fixtures для тестов плагина
```

Правило изоляции: в `internal/` запрещены упоминания «АйДаБаран», грузинской кухни, конкретных
категорий. Всё такое — в `plugins/aidabaran/` (проверяется grep'ом в DoD вехи).

### Интерфейс recommender (единственная точка специфики)

```go
// internal/recommender/recommender.go
type Rec struct {
    ItemID     int64
    ReasonText string   // готовый текст обоснования (FR-11/12)
    Score      float64
    Source     string   // "expert" | "upsell" | "heuristic"
}
type Recommender interface {
    ForDish(ctx context.Context, itemID int64, excludeInCart []int64) ([]Rec, error) // лента карточки (FR-10)
    ForCart(ctx context.Context, lines []CartLine, excludeInCart []int64) (*Rec, error) // топ-1 корзины (FR-19)
}
```

## 4. Схема БД (миграции goose)

```sql
-- 0001_init.sql
CREATE TABLE sessions (
    id          UUID PRIMARY KEY,          -- генерирует сервер
    table_id    INT NULL CHECK (table_id BETWEEN 1 AND 999),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE carts (
    session_id  UUID PRIMARY KEY REFERENCES sessions(id) ON DELETE CASCADE,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE cart_items (
    session_id  UUID NOT NULL REFERENCES carts(session_id) ON DELETE CASCADE,
    item_id     BIGINT NOT NULL,           -- валидируется по меню в памяти
    qty         INT NOT NULL CHECK (qty BETWEEN 1 AND 20),
    added_from  TEXT NOT NULL CHECK (added_from IN ('menu','rec_feed','rec_cart')),
    added_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (session_id, item_id)
);
CREATE TABLE events (
    id          BIGSERIAL PRIMARY KEY,
    session_id  UUID NOT NULL,           -- FK на sessions отсутствует НАМЕРЕННО:
                                         -- чистка sessions не трогает events (retention, adr/0005)
    table_id    INT NULL,
    name        TEXT NOT NULL,             -- таксономия events.md
    payload     JSONB NOT NULL DEFAULT '{}',
    ts          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_events_name_ts ON events (name, ts);
CREATE INDEX idx_events_session ON events (session_id);

-- 0002_item_availability.sql (FR-36, ADR-0006)
CREATE TABLE item_availability (
    item_id      BIGINT PRIMARY KEY,         -- без FK: меню — статика в памяти;
                                             -- item_id валидируется CLI против меню из DATA_DIR
    is_available BOOLEAN NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Семантика default-available: отсутствующая строка = позиция доступна.

-- 0003: additive auth hardening (FR-33, NFR-04, ADR-0005)
ALTER TABLE sessions
    ADD COLUMN credential_hash BYTEA NULL UNIQUE,
    ADD COLUMN expires_at TIMESTAMPTZ NULL,
    ADD CONSTRAINT sessions_credential_expiry_check CHECK (
        (credential_hash IS NULL AND expires_at IS NULL) OR
        (credential_hash IS NOT NULL AND expires_at IS NOT NULL
         AND octet_length(credential_hash) = 32));
```

Валидация `item_id` против меню — в сервисном слое (меню в памяти), не FK: меню не в БД.
`sessions.id` остаётся аналитическим UUIDv4; независимый cookie-credential хранится только как
SHA256 канонического lowercase UUID. Auth — hash AND `expires_at > now()`, никогда id.
Миграция 0003 не backfill'ит hash из id и не удаляет sessions/carts/events или их связи;
оба NULL — legacy без доступа. Абсолютный expiry и fail-closed — `30-contracts/api.md`.

## 5. Frontend

```
frontend/src/
├── app/            # роуты: / (меню), /dish/:id, /cart, /pay-stub; провайдеры сессии
├── components/     # generic: DishCard, RecFeed, RecCard, CartLine, QtyStepper, CategoryChips, PhotoWithFallback, ErrorRetry
├── pages/          # MenuPage, DishPage, CartPage, PayStubPage
├── api/            # типизированный клиент по api.md
├── theme/
│   ├── base.css    # структура: переменные-плейсхолдеры, layout, типографика
│   └── aidabaran.css  # значения токенов (из data/aidabaran/assets/tokens.css)
```

Тема = CSS-переменные (`--c-accent`, `--font-display`, …). Общие компоненты используют только
переменные; конкретные значения — в теме ресторана. Выбор темы: сборка через import в точке входа
(v0 — aidabaran; другие рестораны = другой css-файл).

## 6. Конфигурация и env

| Переменная | Пример | Назначение |
|---|---|---|
| `PORT` | `8080` | порт HTTP |
| `DATABASE_URL` | `postgres://…` | DSN Postgres |
| `RESTAURANT` | `aidabaran` | выбор плагина и data-каталога |
| `DATA_DIR` | `../data` | корень данных ресторанов; по умолчанию `../data` — запуск из `backend/`, как в AGENTS.md |
| `FRONTEND_DIST` | `../frontend/dist` | каталог собранного SPA, который backend раздаёт статикой (прод, см. §7) |
| `COOKIE_MAX_AGE_H` | `12` | жизнь анонимной сессии |
| `AVAILABILITY_TTL_S` | `5` | TTL (сек) in-memory кэша наличия блюд (FR-36): задержка подхвата изменений, внесённых CLI |

`.env` в git не попадает; `.env.example` — в корне репо (создаётся в M2).

Инженерные решения (scaffolding):

- Go-модуль — `svoyshef`.
- В `backend/` — Makefile с мишенями `build`, `test`, `lint`, `migrate-up`, `migrate-down`, `run`.
  DSN для миграций — только env, без echo и argv; sessionID, credential/hash и raw pgx-ошибки не логируются.
- goose — без глобальной установки: `go run github.com/pressly/goose/v3/cmd/goose …`.
  Миграции применяются автоматически при старте `serve` и CLI-подкомандах (`runMigrations`
  в `cmd/server/main.go`); ручной `make migrate-up` — дев-удобство, для прода не требуется.
- Миграции — `backend/migrations/`, именование sequential `NNNN_name.sql`.
- `docker-compose.yml` — в корне репо, создаётся в M2 (сервис `db`: `postgres:16-alpine` +
  healthcheck `pg_isready`); прод-профиль — в M5.
- Node 20+ (frontend).

## 7. Деплой

- Дева: `docker compose up -d db` + `go run ./cmd/server` (backend :8080) + `npm run dev`
  (vite :5173, проксирует `/api` на :8080).
- Локальная поставка (прототип, M5): `docker compose up` без флагов поднимает полный стек db+backend;
  backend на :8080 раздаёт `/api/v1` + SPA из `FRONTEND_DIST` + `/static` из `DATA_DIR`.
  Миграции применяются автоматически при старте `serve` (см. §6).
- Фото: прод-образ НЕ содержит `data/aidabaran/photos` (вне git, ~89 МБ); compose
  бинд-маунтит `./data:/app/data` — на машине с полным `data/` работают фолбэк-фото;
  на чистом клоне сайт работает с CDN-фото и деградацией фолбэка (осознанное решение;
  FR-26 проверяется на пилотной машине).
- Публикуемые порты M5 — только loopback (`127.0.0.1`); публичный деплой/TLS/домен — этап VPS
  после M5, в M5 не входит; CSP-заголовок добавляется в M5
  (defense-in-depth).
- Health-check: `GET /api/v1/health`.

## 8. ADR

См. `docs/adr/` — краткие записи по значимым решениям (стек, плагин-модель, статика/динамика,
без-ML рекомы, анонимность, наличие блюд).
