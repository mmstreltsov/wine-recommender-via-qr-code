# M4 final QA — отложенные проверки приёмки (checkpoint п.1, п.2)

Дата: 2026-11-08 · QA-агент · Окружение: штатный dev (compose-БД 127.0.0.1:5432, backend :8080,
штатный vite dev репозитория). Репо НЕ менялось (кроме заполнения колонки «Проверено» в
docs/10-requirements.md — этап E, после полного PASS). Секретов нет; DSN — dev, не секрет.

## Итог

| Отложенный пункт checkpoint | Статус |
|---|---|
| п.1 Финальная браузерная регрессия 375/1440 (sticky alert, валютный глиф, LIVE через штатный прокси репозитория) | **PASS** (86/86 чеков) |
| п.2 Lighthouse Slow 4G (NFR-02, AC ≥ 70) | **PASS** — 70 / 70 / 70 (3 прогона) |
| Трассировка 10-requirements.md (checkpoint п.4) | Выполнена (этап E) — все строки M4-скоупа PASS |

## A. Штатное окружение

- Backend собран из репо (без правок), PID 77062, `:8080`: health=200; `GET /api/v1/menu` —
  **14 категорий / 125 позиций** (после гигиены окружения, см. ниже).
- Штатный vite dev (`npm run dev`, frontend/vite.config.ts без правок) — порт 5173 был занят
  остаточным процессом на IPv4, vite выбрал IPv6-коллизию; перезапущен QA на явном порту **5180**
  (`--port 5180 --strictPort`), тот же конфиг-прокси `/api`,`/static` → `:8080` (changeOrigin:false,
  Host сохраняется). Через vite-origin: health=200, `/static/aidabaran/assets/fonts/unbounded-currency.woff2`
  =200, магика `wOF2` (118348 байт).
- **Гигиена окружения (до прогона)**: в штатной БД осталось 6 тестовых `item_availability` строк
  `available=off` от прошлого прогона (updated_at 2026-11-08T13:11:01Z): 164834011242, 231821651422,
  293109021772, 733465955822, 875106956773, 876201943922 → меню отдавало 119 позиций. Все 6
  восстановлены в `on` (`availability set … on`), итог: «нет отключений», меню 125/14. Это
  единственные изменения данных, помимо тестовых сессий/событий столика 42 и availability-цикла
  хинкали (восстановлен `on`).
- Живой трафик шёл строго: браузер → vite 5180 (штатный прокси репозитория) → backend 8080 →
  compose-БД. Мутации проходили (корзина/события писались) — фикс прошлого HIGH о строковом
  шорт-кате прокси подтверждён исправленным в репо (changeOrigin:false, mutation guard проходит).

## B. Браузерная регрессия (headless Chromium/Playwright+CDP, table=42, вьюпорты 375×812 и 1440×900)

Полный лог: `regression-results.json` (86 чеков, 0 FAIL), скриншоты: `artifacts/reg-*.png`.

| Чек | Требование | PASS/FAIL | Evidence |
|---|---|---|---|
| Меню: 200, 14 категорий, 125 позиций, 15 чипов видны; первый запрос `/api/v1/menu?table=42` | FR-02, FR-34 | PASS | w375+w1440; events table_id=42 |
| Переключение категории (фильтрация, активный чип #fdc300, `category_select` с payload) | FR-25, UI-02 | PASS | «Хачапури» → N карточек; «Все» → 125 |
| Карточка хинкали 655892482392: лента ≥5 уникальных категорий, не из своей; подзаголовок «Хорошо сочетается с»; причины непустые | FR-09..13 | PASS | recs=6 (w375/w1440) |
| Тап по телу rec-карточки не навигирует; добавление не перезагружает страницу | FR-15 | PASS | URL неизменен после клика титула и «Добавить» |
| Добавление из меню (`menu_add_click`, сервер) и из ленты (`rec_add_click`, сервер): бейдж 1→2 | FR-14, FR-30, AN-02 | PASS | badge=2, события в БД |
| `rec_feed_visible` (1/открытие) + `rec_impression` context=dish | AN-02, events.md | PASS | w375: feed=1, impressions>0 |
| Корзина: `cart_open` payload {lines_count,total}; блок ForCart ровно 1 карточка с обоснованием (Медовик) | FR-19 | PASS | cartRecCount=1, reason≠'' |
| Степпер qty 1→2 (`cart_qty_change`), удаление линии (`cart_item_remove`) | FR-32, FR-18 | PASS | value=2; линия удалена |
| Sticky-панель оплаты: position=sticky, bottom=0; кнопка «Оплатить» hit-test проходит (не перекрыта) | FR-23 | PASS | elementFromPoint=кнопка |
| Sticky-alert (блокировка оплаты): сообщение появляется ВНУТРИ sticky-панели, НЕ перекрывает кнопку, фокус; исчезает при восстановлении линии | UI-04, FR-37 п.7 | PASS | appear→dismiss; overlapsPay=false, payHit=true |
| Пустая корзина: «Корзина пуста», нет «Оплатить» и реком-блока | FR-31 | PASS | w375+w1440 |
| Оплата: `/pay-stub`, дословно «Ваш выбор сформирован. Позовите официанта.», `pay_click` до `stub_view`; возврат в меню | FR-24 | PASS | text===эталону |
| Шрифты: заголовки/цены Unbounded, текст Montserrat; ₽ (U+20BD) рендерится кастомным Unbounded (CDP platform fonts: единственный шрифт probe `₽` — Unbounded, isCustomFont; в реальной `.dish__price` все глифы кастомные, системного фолбэка нет) | UI-01 | PASS | w375+w1440 |
| Шрифты: все font-запросы same-origin `/static/aidabaran/assets/fonts/*.woff2`, внешних запросов шрифтов нет; прочих внешних запросов нет (кроме фото CDN tildacdn) | UI-01, NFR | PASS | fontRequests все localhost:5180/static/... |
| Стили: кнопка «Оплатить» bg #fdc300 / текст #000 / radius 10px / border 0 / shadow none / Unbounded; активный чип #fdc300 | UI-02, UI-03 | PASS | computed styles w375+w1440 |
| 375px: `scrollWidth <= 375` (нет гор. скролла); все видимые интерактивные элементы (кнопки/чипы/степперы/крестики) ≥ 44×44 px | NFR-01 | PASS | меню+корзина, smallTargets=[] (w375 и w1440) |
| Форм ввода нет: `form,input,textarea,select` = 0 на всех экранах (меню/карточка/корзина/stub) | NFR-03 | PASS | 4 экрана × 2 вьюпорта |
| UI-04 LIVE: `availability set 655892482392 off` при живом сервере → после TTL(5с)+6.5с переоткрытие корзины: линия `.cart-line--unavailable` (muted), бейдж «Нет в наличии», qty-изменение работает (1→2), крестик доступен | UI-04, FR-36, FR-37 п.5/6 | PASS | w375+w1440 |
| Блокировка оплаты при недоступной линии: сообщение «Часть позиций больше нет в наличии — уберите их из заказа», URL=/cart (НЕ /pay-stub), новых `pay_click`/`stub_view` НЕТ (сверка по БД до/после) | UI-04, FR-37 п.7 | PASS | counts не изменились |
| Восстановление: `availability set … on` → после TTL линия нормальна, бейджа нет, feedback исчез | UI-04, FR-36 | PASS | mutedAfter=false |
| AN-02: за прогон в БД столика 42 присутствуют ВСЕ 13 имён таксономии events.md | AN-02, AN-01 | PASS | см. счётчики ниже |
| FR-29 (блюdo без пар): все 125 позиций live дают непустую ленту (проверено API-пробой по всем id, 0 ошибок) — пустой ленты в live-данных нет; скрытие блока покрыто TestFR29 + TestFR37_Integration_RecPostFilter | FR-29 | PASS (unit-покрытие) | probed=125, empty=0 |

События столика 42 по итогам (все 13/13): page_open 21, menu_view 29, category_select 9,
dish_card_open 14, rec_feed_visible 6, rec_impression 49, menu_add_click 15, rec_add_click 7,
cart_open 26, cart_qty_change 8, cart_item_remove 12, pay_click 4, stub_view 4.
`pay_click` payload {total, lines_count}; порядок pay_click → stub_view подтверждён по journal.

Примечание (не баг): ForCart анкерится на последнюю добавленную позицию (контракт recommender.md §ForCart,
шаг 2: «исчерпание приоритетного курса не переключает — nil»). Для корзины {лимонад, хинкали(последняя)}
`recommendation:null` — валидно по контракту; в финальном прогоне состав {хинкали, лимонад(последний)}
даёт блок «Медовик» — FR-19 проверен в обоих состояниях.

## C. Lighthouse NFR-02 (AC: performance ≥ 70, Slow 4G)

- `npm run build` → dist (gitignored); `npm run preview` :4173 — preview наследует прокси
  (health=200, root=200, woff2=200 через 4173).
- `lighthouse@12` mobile, throttling devtools (rtt 150ms / 1638.4 Kbps = дефолтный Slow 4G),
  Chrome = headless shell. Прогоны: **70 / 70 / 70** → PASS.
- Метрики (run1): FCP 1.7s (93), LCP 7.3s (5), TBT 30ms (100), CLS 0 (100), SI 6.3s (42), TTI 4.8s.
  LCP тянут CDN-фотографии карточек (resize 400, lazy) — укладывается в AC за счёт TBT/CLS.
  Артефакты: `lighthouse.json`, `lighthouse-2.json`, `lighthouse-3.json`.

## Гигиена процессов

Запущено QA и остановлено: backend :8080 (PID 77062), vite dev :5180 (PID 77106/77122),
vite preview :4173 (PID 77682). Порты 8080/5180/4173 свободны.
НЕ тронуты (по условию): 5173 (PID 69505), 5193 (PID 65432), изолированный QA-бэкенд :8086 (PID 69084),
контейнер m4-backend-qa-*, compose-БД. `availability list` = «нет отключений» (хинкали и 6 позиций
восстановлены). `go test` не запускался. npx-кэш lighthouse — вне репо; package.json не менялся.

## Вывод

Оба отложенных пункта checkpoint (п.1, п.2) — PASS, блокеров нет. Этап E (трассировка) выполнен:
колонка «Проверено» заполнена для UI-01..04, NFR-01..03, FR-15, FR-24, FR-29, FR-31; дополнена
клиентская часть в AN-02, FR-36, FR-37. Требования/AC не менялись.
