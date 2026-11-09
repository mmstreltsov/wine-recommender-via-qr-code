# Независимая QA-проверка гастрономических рекомендаций

Дата: 09.11.2026; первый проход 17:55–18:03 UTC, финальный DB-проход
18:08–18:10 UTC. Роль: независимый QA/backend,
не автор рекомендательных данных или шаблонов.

**Итог: QA OK — full backend normal/race с DB integration PASS: в каждом прогоне
146 top-level PASS, 2014 PASS включая subtests, 19/19 DB integration PASS,
0 FAIL / 0 test SKIP. Validator/vet/lint PASS.**
Исторические DB skips первого прохода **заменены финальными выполненными
прогонами**, а не остаются открытым DB-гейтом. Это QA-апрув проверенного snapshot
в указанном скоупе, не отдельный security-апрув или закрытие всей вехи.

## Область проверки и записи

Прочитаны `AGENTS.md`, `docs/60-agents.md`, контекст, реестр, план,
frontend-спецификация, контракты data-model/recommender, полный новый
`gastronomy-content-audit.md`, реальные matrix/heuristics, код каскада и шаблоны
`texts.go`, существующие backend-тесты. `docs/SPEC.md` и `docs/PLAN.md` отсутствуют;
использован актуальный `docs/50-plan.md`.

QA изменил только:

- `backend/cmd/server/main_integration_test.go`;
- новый `backend/plugins/aidabaran/gastronomy_regression_test.go`;
- этот новый отчёт.

Вне репозитория обновлялся только назначенный heartbeat `hb/gastronomy-qa.json`
через `apply_patch`. Данные, production-код, контракты, миграции, frontend,
параллельные `texts_test.go` и `recommender_test.go` QA не менял.
Коммитов, установки зависимостей и запуска/пересоздания Docker не было.
На финальном DB-проходе изменены только этот отчёт и heartbeat: дополнительные
правки `*_test.go` не потребовались. Четыре reason-правки по ревью внесены не QA:
`808267771773→981442240863`, `238582192523→951817158352`,
`314183598113→951817158352`, `507171197452→231821651422`.
Они вошли в финальный проверенный snapshot; fixed reasons хинкали/интеграций
и оба QA test-файла остались неизменными.

## Что зафиксировано тестами

### FR-10/11/12/28/29, О2/О3: все реальные карточки

`TestFR10_RealDataGastronomyAllCards` загружает реальные файлы штатным плагином;
отсутствие данных в этом тесте — ошибка, а не skip.

- Ровно **125** исходных карточек, **14** категорий и **210** matrix-пар.
- Для каждого исходного UID непустая лента; суммарно выбраны ровно **210 уникальных
  expert-кандидатов**, все raw-пары действительно представлены в выдаче.
- Нет неизвестных UID, self/same-category, duplicate pair/item/category или
  category override; source только `expert`, score/reason выдачи совпадают с парой.
- У всех raw reasons и всех выбранных reasons длина **60–220 Unicode-символов**,
  без контрактных стоп-слов; порядок выдачи проверен по score DESC/category ID ASC.
- Все **14/14** реальных категорий встречаются целевыми.
- Явный небольшой UID-ledger запрещает крупные/на компанию targets: большие
  салаты/ассорти, хачапури 500–1000 г, сет хинкали, ташмиджаби, Дэда-чебуреки,
  бифштекс/рёбра на несколько персон, целые дорадо/цыплята с неизвестным выходом.
  Произвольный язык меню, массы и рецептуры тест не пытается распознавать.
- Для явно перечисленных UID с включёнными соусами запрещены дополнительные
  соусы и новое блюдо с другим включённым соусом; проверено также обратное
  направление от отдельно выбранного соуса к уже соусному комплекту.
- Исключение всех выбранных UID для каждой из 125 карточек даёт пустую ленту,
  без появления fallback-замен.

### Точные направленные регрессии

`TestFR19_RealDataGastronomyDirectedAdditions`: восемь сценариев реального
`ForDish` и `ForCart`, точные ordered UID/category/source, reason bounds и
совпадение текста ForCart с выбранной expert-парой.

| Исходное блюдо | Точная лента UID в порядке выдачи | ForCart для одного блюда |
|---|---|---|
| Хинкали говядина-свинина `655892482392` | `366609864022`, `733465955822`, `231821651422`, `445870875313`, `951817158352` | гранат `445870875313` |
| Шейка с сацебели `219414720772` | картофель `573190167622`, вишнёвый сок `844016957853` | вишнёвый сок |
| Лодочка с салатом/мятным соусом `577945503202` | гранат `445870875313` | гранат |
| Дорадо гриль `759806895422` | вода `301501361763` | вода |
| Дорадо на пару `159551626192` | вода `301501361763` | вода |
| Треска с аджапсандалом/соусом `183588997633` | вода `301501361763` | вода |
| Лосось с кабачком `538899551122` | вода `301501361763` | вода |
| Рёбрышки на компанию `988436681252` | вода `301501361763` | вода |

Хинкали не получают долму или салат 600 г; grill с сацебели не получает второй
соус; рыба с включённым соусом не получает новое горячее/гарнир/соус.

### Обновлённые integration-ожидания (FR-10/19/20/28/29/37)

- `TestFR10_Integration_RecFeedRealEngine`: вместо старой шестёрки ровно пять
  expert-позиций: салат с орехом → овощи на гриле → ткемали → гранат → мороженое.
  Проверяются точные UID, category ID **1/10/11/14/12**, порядок и дословные reasons.
  После добавления граната ожидаются ровно четыре оставшиеся позиции с теми же
  полями/порядком, а не просто уменьшение длины.
- `TestFR19_Integration_CartRecommendationRealEngine`: хинкали → гранат,
  удаление хинкали при оставшемся медовике → яблочный фреш; точные
  UID/category/source/reason.
- `TestFR37_Integration_RecPostFilter`: исходная точная пятёрка, off ткемали →
  точные оставшиеся четыре без категории соусов; off гранат → ForCart null;
  off оставшихся трёх → пустая лента; POST выключенного граната → прежний 409.
- Добавлен **выполненный без БД** `TestFR10_RealDataIntegrationContentExpectations`:
  те же дословные fixtures проверяются против реального движка для полной ленты,
  исключения граната/ткемали и пары медовика. Это предотвращает ложные старые
  content-ожидания при DB skips, но **не подменяет HTTP/TTL/SQL integration**.
- Guard перед миграциями/TRUNCATE сохранён без изменений: только
  `127.0.0.1:55432/m5_integration`, без pgx fallbacks; `svoyshef` запрещена.

## Финальные команды и результаты с БД (18:08–18:10 UTC)

ОС darwin/arm64, `go version go1.27.3 darwin/arm64`, рабочий каталог `backend/`.
Перед normal-прогоном read-only `docker ps` с фильтром проекта подтвердил только
`vkr-dev-db-1`, running/healthy, `127.0.0.1:55432->5432/tcp`. SRE сообщил о
существовании disposable DB; успешное выполнение всех guarded integration-тестов
подтвердило её доступность. После прогонов running осталась только та же БД.

Normal и race выполнялись **последовательно**, не конкурируя за TRUNCATE:

```sh
# Каждый блок — отдельный subshell; значения секретов не выводятся.
(set +x; set -a; . ../.env; set +a; unset DATABASE_URL M2_IT_TRACE;
 test -n "$M2_IT_DATABASE_URL" && go test -count=1 -json ./...)
(set +x; set -a; . ../.env; set +a; unset DATABASE_URL M2_IT_TRACE;
 test -n "$M2_IT_DATABASE_URL" && go test -race -count=1 -json ./...)
```

В фактических командах включён shell `pipefail`, JSON-output передавался Python
агрегатору в stdout: только counts, имена тестов/пакетов и число race warnings;
сырые логи/ошибки не печатались и в файлы не сохранялись. Shell source разрешён
без policy deny; `.env` не читался Read-инструментом и не печатался. Credentials
передавались только через env, не в argv. `DATABASE_URL` после source удалён;
для БД использован только `M2_IT_DATABASE_URL` под существующим Go guard.

| Проверка | Финальный результат |
|---|---|
| Full `go test -count=1 -json ./...` | PASS: 146 top-level / 2014 включая subtests; 19/19 DB integrations; 0 FAIL / 0 test SKIP |
| Full `go test -race -count=1 -json ./...` | PASS: те же 146 / 2014 / 19; 0 FAIL / 0 test SKIP / 0 race warnings |
| `env -u M2_IT_DATABASE_URL -u DATABASE_URL go run ./cmd/validate-matrix` | PASS: `OK: 210 pairs, coverage 100.0%` |
| `env -u M2_IT_DATABASE_URL -u DATABASE_URL go vet ./...` | PASS |
| `env -u M2_IT_DATABASE_URL -u DATABASE_URL golangci-lint run ./...` | PASS: `0 issues.` |
| `git diff --check` | PASS |

**146 top-level входят в 2014; 19 integration входят в 146 — не суммировать.**
Шесть пакетов без test-файлов Go JSON помечает package-level `skip`; это
`[no test files]`, не пропущенные тесты. Test-level skips отсутствуют.
Counts относятся ко всему совместному worktree, включая тесты шаблонов.

Оба раза реально PASS все 19 DB integration-тестов:

```text
TestAN04_Integration_CSVContract
TestAN04_Integration_OpenBoundariesAndEmpty
TestFR33_Integration_CookieAttributes
TestFR34_Integration_TableLifecycle
TestFR37_Integration_MenuFull14x125
TestAN02_Integration_CartCycleServerEvents
TestAN03_Integration_RecAddClickContextAndSession
TestFR36_Integration_AvailabilityTTLCycle
TestNFR04_Integration_Events204And400
TestNFR04_Integration_EventsRateLimit429
TestFR32_Integration_QtyLimit20OnAdd
TestFR32_Integration_PatchQtyBounds
TestFR10_Integration_RecFeedRealEngine
TestFR19_Integration_CartRecommendationRealEngine
TestFR37_Integration_RecPostFilter
TestNFR04_Integration_CredentialIsolationExpiryAndExport
TestNFR04_Integration_LegacyDataPreservedWithoutAuthentication
TestNFR04_Integration_SessionDBFailureIsClosed
TestNFR04_Integration_MigrationPreservesRowsAndEnforcesConstraints
```

В том числе теперь выполнены HTTP-контракт через тестовый сервер, availability
TTL/post-filter, корзина и CSV/auth/migration-регрессии; прежние skips этих
сценариев больше не являются ограничением результата. Штатный deployed backend
не запускался: HTTP в integration обслуживали тестовые `httptest`-серверы.

## История первого прохода без БД (17:55–18:03 UTC)

**Ниже только история первого snapshot, не текущие результаты/блокеры.**

ОС darwin/arm64, `go version go1.27.3 darwin/arm64`. Рабочий каталог — `backend/`.
В прогонах первого прохода явно удалены `M2_IT_DATABASE_URL` и `DATABASE_URL` из окружения;
в тестовых прогонах также удалён `M2_IT_TRACE`. `.env` не читался и не загружался.

| Проверка | Результат |
|---|---|
| `go test -count=1 -json ./...` | PASS, 0 FAIL; 127 top-level PASS / 19 top-level SKIP; 1989 PASS включая subtests |
| `go test -race -count=1 -json ./...` | PASS, 0 FAIL, без race findings; те же 127/19/1989 |
| `go vet ./...` | PASS; повторён после race/focused прогона |
| `golangci-lint run ./...` | PASS, `0 issues.` |
| `go run ./cmd/validate-matrix` | PASS: `OK: 210 pairs, coverage 100.0%` |
| Focused `go test -count=1 -v ./plugins/aidabaran ./cmd/server -run 'TestFR10_RealDataIntegrationContentExpectations\|TestFR19_RealDataGastronomyDirectedAdditions'` | PASS, оба top-level и все восемь directed subtests, без skips |
| `gofmt -d` двух QA test-файлов | пустой diff |
| `git diff --check` | PASS |

Для двух новых untracked файлов дополнительно выполнен
`git diff --no-index --check /dev/null <file>`: whitespace-диагностик нет;
exit 1 означает наличие нового файла/отличий, не exit 0. Повторные blob IDs
matrix/heuristics/templates и двух QA test-файлов на тот момент совпали с первым
snapshot; matrix blob был `6ad38215ce1137f7f6eab42bdb4ec6b862e57d35`.
Финальный snapshot после четырёх reason-правок приведён в конце отчёта.

JSON test-output агрегирован в stdout без сохранения credentials/сырых логов.
Число 1989 включает 127 top-level, их **не суммировать**. Числа относятся ко всему
совместному worktree, включая параллельные тесты шаблонов, а не только новой QA-работе.
Первый локальный запуск выявил compile-ошибку нового теста (несуществующее поле
`Matrix.Version`); проверка удалена, после исправления выполнены полные зелёные
normal/race/vet/lint прогоны выше. Production-код ради теста не менялся.

Ожидаемый warning загрузчика: у мороженого `951817158352` поле `kbju` без
«Калорийность» отброшено; это существующее свойство неизменённого меню.

## История DB skips и безопасность целевой БД

В первом проходе до любых integration-попыток выполнены read-only Docker-проверки:
`docker ps --filter label=com.docker.compose.project=vkr-dev` не вернул контейнеров;
`docker compose ls --format json` вернул `[]`. TCP-проверка сокета явно показала
`127.0.0.1:55432 reachable=False`. Docker не запускался и не пересоздавался.

Тогда все **19 DB integration-тестов** явно skipped из-за незаданного
`M2_IT_DATABASE_URL`: 2 CSV, 1 cookie, 1 table lifecycle, 1 полное меню,
2 cart/events context, 1 availability TTL, 2 events validation/rate-limit,
2 qty bounds, 3 recommender/feed/post-filter, 4 credential/legacy/DB-failure/migration.
Соединений к production/funnel DB, миграций или TRUNCATE в первом проходе не было.

После старта **только db** проекта `vkr-dev` SRE финальный проход выполнил все
19/19 интеграций normal/race. Нужды в дополнительном старте стека для этого
QA-гейта **больше нет**; backend остаётся stopped по отдельному security gate.
Миграции/TRUNCATE выполнялись только самим тестовым suite на disposable
`127.0.0.1:55432/m5_integration`. Существующий guard host/port/database/no fallbacks
сохранён без изменений. `svoyshef` не использовалась тестами; QA не подключался
к ней и не выполнял Docker exec, ручной SQL, migrations CLI или очистку БД.

## Ограничения и открытые вопросы

- Зелёные тесты подтверждают структуру и перечисленные directed-сценарии, не
  истинность рецептуры/гастрономического вкуса и не одобрение кухни.
- Полные произвольные корзины не перебирались. Остаточный риск второго основного
  после покрытия всех курсов, описанный автором в `gastronomy-content-audit.md`
  §ForCart, **не закрыт** этой QA-проверкой; универсальная безопасность корзины
  не заявляется. Алгоритм и контракты не менялись.
- Историческая трассировка `docs/10-requirements.md` описывает старые шесть
  рекомендаций; нормативный AC FR-10 (≥5 категорий) выполнен точной новой пятёркой.
  Исторические документы QA не переписывал.
- Браузер/UI, API штатного deployed backend, Docker backend runtime,
  TLS/публичный пилот, физические устройства,
  сканеры зависимостей и security-аудит этим отчётом не апрувируются.
- Инструмента вызова отдельного `security` в QA-сессии нет; координатору нужен
  независимый security-review diff. Собственный read-only просмотр сохранённого
  DB guard и зоны записи не заменяет такой апрув.

## Финальный snapshot совместного worktree

Git blob IDs финального DB-прогона, повторно сверены после normal/race;
не коммит и не утверждение о чистом HEAD:

| Файл | `git hash-object` |
|---|---|
| `data/aidabaran/menu.json` | `13c08ae650fce04ba480a04c13030088e7be2e08` |
| `data/aidabaran/recommender/matrix.json` | `4df2f98662f77b8e2f019fa9c1ecaa6c59fff0bc` |
| `data/aidabaran/recommender/heuristics.json` | `4808ae1ebb5d0002713ebd4913a09b97f3af4268` |
| `backend/plugins/aidabaran/texts.go` | `95167526be6acdf7bb681f6f83d52c7fb975b505` |
| `backend/plugins/aidabaran/texts_test.go` | `d207704ad176fb6a53702bb394ea14b308af66ef` |
| `backend/cmd/server/main_integration_test.go` | `a513b3d35707bbf5ad0c8e717f10a720c241f1a1` |
| `backend/plugins/aidabaran/gastronomy_regression_test.go` | `c2c1965be6dc6941d416ea2ead46095a23b2172d` |

Параллельные изменения не откатывались. При последующих изменениях content/code
нужен повтор соответствующих проверок; этот результат относится к snapshot выше.
