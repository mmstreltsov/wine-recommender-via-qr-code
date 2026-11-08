# Гостевое онлайн-меню — M4

React 18.3 + TypeScript + Vite, CSS-переменные, React Router. Без UI-кита, форм,
авторизации, browser storage, polling и реальной оплаты. Тексты интерфейса на русском.

## Запуск

Node 20+ (для Vitest поддерживаются Node 20, 22, 24+), запущенный backend на `:8080`:

```sh
npm ci
npm run dev
```

Vite проксирует `/api` и `/static` на backend. Открыть `http://localhost:5173/?table=12`.
Оба proxy используют `changeOrigin: false`: исходный Host сохраняется, чтобы backend
мог сравнить его с браузерным Origin на POST/PATCH/DELETE, включая events. Подмена Host
на backend target ломает этот guard; доверие к forwarded-заголовкам не добавляется.
Не запускать `preview` как замену backend: production обслуживает backend с `frontend/dist`
и данными ресторана. Новых API не требуется.

```sh
npm run lint
npm run test
npm run build
npm ls react react-dom
npm audit
npm audit --omit=dev
```

Автопроверки реализации: **49 tests passed / 5 files**, ESLint и TypeScript/Vite build
зелёные. Это компонентные/клиентские интеграционные проверки, не браузерная приёмка.

## Явные решения

- Первый запрос документа — только `GET /api/v1/menu?table=N` с проверенным номером
  столика (1–999). Ни config, ни cart, ни events/beacon не отправляются до его успешного
  ответа и валидации. Ошибка оставляет барьер закрытым; «Повторить» повторяет startup.
- `page_open` сохраняется в очереди до успешного startup, один раз на документ.
  Ошибки аналитики не ломают интерфейс, события не ретраятся, включая `429`.
  Серверные события добавления/количества/удаления фронтенд не дублирует.
- Все ответы проверяются на границе API. UI не выводит тексты технических ошибок.
  Идентификаторы — положительные safe integers, qty — 1–20, `available` — boolean,
  суммы проверяются. Тексты отображаются средствами React без HTML-инъекций.
- Фото: только HTTPS CDN `tildacdn.com`/`tildacdn.net` без credentials/query/hash,
  ресайз 400/1200/200 → same-origin `/static/` → заглушка. Изменение URL/fallback/размера
  сбрасывает состояние. Для другого CDN нужно явно расширить политику URL.
- Имя, логотип, заголовок рекомендаций приходят из config. Ресторанные шрифты и
  токены находятся в `src/theme/`; общие компоненты не содержат названия ресторана.
- Вход на меню/карточку/корзину заново запрашивает данные; вечного кэша меню нет.
  Каждая успешная мутация публикует полный снимок корзины всем подписчикам, включая
  строки, сумму, наличие, рекомендацию и счётчик. Запросы корзины сериализованы,
  запоздавший GET не затирает более новую мутацию. Оплата не делает prepay-запроса.
  Во время мутации корзины оплата недоступна до получения нового снимка наличия.
  В ленте блюда подтверждение «✓ Добавлено» сохраняется на 1.5s, после чего добавленные
  рекомендации удаляются и выдача обновляется однократно (не polling).
- Наличие в корзине не запрещает изменение qty и удаление. При `available=false`
  оплата показывает сообщение без перехода и без `pay_click`/`stub_view`.
  Feedback расположен внутри sticky-панели оплаты, получает фокус без прокрутки
  и связан с кнопкой через `aria-describedby`; это проверяется после изменения qty.
- Показ ленты измеряется по компактному заголовку, а не длинному контейнеру.
  Порог — 50%; лента один раз на открытие блюда, карточки — по ID и scope открытия.
  Retry/rerender/смена рекомендации не сбрасывают scope, повторный вход создаёт новый.
- Выбор «Все» отправляет `category_id: 0`; начальное состояние события не создаёт.
  Апсейлы информационные, рекомендации и строки корзины не имеют перехода в блюдо.

## Трассировка реализации и автоматических проверок

Контракт и реестр вне зоны записи frontend. Эта таблица — передача QA для обновления
колонки «Проверено» в `docs/10-requirements.md`, а не заявление браузерной приёмки.

| Требования | Реализация | Проверки |
|---|---|---|
| FR-01–04, FR-25, FR-27, FR-35, FR-37 | `pages/MenuPage`, `components/Header`, `CategoryChips`, `DishCard` | `pages/pages.test.tsx`: фильтр/Все=0, повторный вход, открытие, retry |
| FR-05–07, FR-09–15, FR-28–30, FR-35 | `pages/DishPage`, `components/RecFeed`, `RecCard` | `pages/pages.test.tsx`, `components/components.test.tsx`: состав, информационные апсейлы, пустая лента, no-nav, добавление, route-ID race, 404/retry |
| FR-17–21, FR-23–24, FR-31–32, UI-04, FR-37 | `pages/CartPage`, `components/CartLine`, `QtyStepper`, `app/cart-context`, `api/client` | `pages/pages.test.tsx`, `api/client.test.ts`: пусто, полный mutation snapshot, динамический rec_cart, недоступность/qty/удаление, блок оплаты без событий, retry |
| FR-26, NFR-04 | `components/PhotoWithFallback`, `api/validation` | `components/components.test.tsx`: 3 размера, CDN→local→stub, reset URL/размера/fallback, unsafe URL |
| FR-33–34, AN-01–03, NFR-04 | `api/client`, `api/tracker`, startup в `App` | `api/client.test.ts`: startup barrier, failure/retry, no beacon до успеха, page_open один, table validation, дедуп по scope и ID, 429 |
| AN-02 | `components/RecFeed`, `RecCard`, `app/useInView` | `components/components.test.tsx`, `App.test.tsx`: компактный заголовок, 49%/50%, rerender/reopen |
| NFR-03–04, NFR-06 | React rendering, `api/validation`, отсутствие ввода | `pages/pages.test.tsx`: XSS-текст не HTML, форм нет; ошибки без внутренних деталей |
| FR-путь, UI-04, AN-02 | SPA целиком | `App.test.tsx`: настоящий клиент/трекер с HTTP mock, ошибка старта → retry → меню → блюдо → rec_feed → корзина → rec_cart → недоступность → qty/удаление → оплата → заглушка → меню |
| UI-01–03, NFR-01–02 | `theme/base.css`, `theme/aidabaran.css` | Реализованы self-host fonts, токены, 1/2/3 колонки (560/960), tap ≥44px, lazy images; нужна браузерная проверка ниже |

## Оставшиеся приёмочные проверки

- UI-03 **не принят**: нужно реальное сравнение скриншотов меню с aidabaran.ru.
- UI-01: проверить загрузку четырёх основных woff2 и дополнительных
  `unbounded-currency.woff2` / `montserrat-currency.woff2` из
  `data/aidabaran/assets/fonts/` и computed fonts в браузере. Currency faces выделены
  диапазоном `U+20BD`; их поставляет агент assets вместе с OFL license. Файлы данных
  фронтендом не изменялись. Отдельно проверить, что ₽ не использует системный fallback.
  Предупреждения Vite о runtime font URL ожидаемы: их раздаёт backend через `/static/`.
- NFR-01: браузерный путь на 375px без горизонтального overflow, размеры всех targets,
  сетка 1/2/3, iOS Safari/Android Chrome. JSDOM не подтверждает геометрию.
- NFR-02: реальный Lighthouse Slow 4G, целевой performance ≥70 и интерактивность <2.5s.
- QA: проверить события с реальной БД/HttpOnly cookie/table_id и актуальность availability
  после серверного TTL. Автотест SPA использует HTTP mock, а не живой Go/Postgres.
- Обязателен независимый security-апрув по правилам `docs/60-agents.md`.

На момент реализации React/ReactDOM и типы приведены к 18; Router и Testing Library
совместимы. Существующий Vitest обновлён с 3 до 4.1.12 для устранения CRITICAL в
dev-цепочке tinypool/mocker; новых прямых библиотек нет. Полный npm audit после обновления:
0 HIGH/CRITICAL, 7 MODERATE в dev-цепочке typescript-eslint → minimatch; обычный
`npm audit fix` их не устраняет. `npm audit --omit=dev`: 0 уязвимостей.
Повторная проверка QA-раунда: последний совместимый minimatch 10.2.6 всё ещё входит
в affected `>=7 <11.0.1` ([GHSA-g3hv-75wg-m249](https://github.com/advisories/GHSA-g3hv-75wg-m249)).
Актуальный typescript-eslint 8.72.1 через typescript-estree требует `^10.2.2`;
совместимого parent patch нет. Breaking override на 11 не применялся вслепую.
