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
  events              └─ Recommender: ForDish / ForCart
```

Композиция — **build-time**: `cmd/server/main.go` читает env `RESTAURANT=aidabaran` и собирает
сервер с соответствующим плагином. Никаких рантайм-плагинов Go (`plugin` package) — это хрупко
и не нужно: добавление ресторана = новый пакет в `plugins/` + строка в registry + его data-каталог.

## 2. Статика vs динамика (ключевое решение)

| Что | Где живёт | Почему |
|---|---|---|
| Меню, категории, матрица пар, тексты рекомов | Файлы `data/<restaurant>/`, в памяти процесса при старте | Статика прототипа; обновление = перезапуск. Нет лишней БД-синхронизации |
| Сессии, корзины, события | PostgreSQL | Динамика; данные исследования |
| Фотографии | CDN ресторана (primary) + локальные копии (fallback через `onerror`) | Ноль хранения и трафика; см. `data/aidabaran/README.md` |

## 3. Структура backend

```
backend/
├── cmd/server/main.go          # композиция: config → store → plugin → http server; подкоманда export-events
├── internal/
│   ├── api/                    # http-хендлеры по контрактам docs/30-contracts/api.md, middleware (сессия, logging, recovery, rate-limit)
│   ├── domain/                 # чистые типы ядра: Category, MenuItem, Cart, CartLine, Rec, Event (без ресторанной специфики)
│   ├── recommender/recommender.go  # interface Recommender + типы Rec/RecSource
│   ├── store/                  # интерфейсы репозиториев + pgx-реализации (sessions, carts, events)
│   └── app/                    # сервисы: MenuService, CartService, EventsService
└── plugins/
    └── aidabaran/
        ├── loader.go           # menu.json → []domain.MenuItem (маппинг полей см. data-model.md)
        ├── recommender.go      # реализация interface: матрица → эвристики → апсейлы
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
```

Валидация `item_id` против меню — в сервисном слое (меню в памяти), не FK: меню не в БД.

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
| `COOKIE_MAX_AGE_H` | `12` | жизнь анонимной сессии |

`.env` в git не попадает; `.env.example` — в корне репо (создаётся в M2).

Инженерные решения (scaffolding):

- Go-модуль — `svoyshef`.
- В `backend/` — Makefile с мишенями `build`, `test`, `lint`, `migrate-up`, `migrate-down`, `run`.
- goose — без глобальной установки: `go run github.com/pressly/goose/v3/cmd/goose …`.
- Миграции — `backend/migrations/`, именование sequential `NNNN_name.sql`.
- `docker-compose.yml` — в корне репо, создаётся в M2 (сервис `db`: `postgres:16-alpine` +
  healthcheck `pg_isready`); прод-профиль — в M5.
- Node 20+ (frontend).

## 7. Деплой

- Дева: `docker compose up` (postgres, backend с air/live — по желанию, vite dev отдельно).
- Прод (прототип): `docker compose up -d` на VPS; backend слушает :8080 и раздаёт
  `frontend/dist` статикой + `/api/v1`. Наличие HTTPS/домена — на этапе выезда на VPS.
- Health-check: `GET /api/v1/health`.

## 8. ADR

См. `docs/adr/` — краткие записи по значимым решениям (стек, плагин-модель, статика/динамика,
без-ML рекомы, анонимность).
