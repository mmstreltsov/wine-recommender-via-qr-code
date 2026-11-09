# M5 Final Security Report

Дата документа: 09.11.2026; QA/SRE evidence: 08.11.2026.
Техписатель фиксирует переданное координатором финальное заключение security-агента и review loop.
Это сводный отчёт, не новый аудит или повторный прогон команд техписателем.

## Вердикт

**Security: PASS для локального прототипа M5. Открытых CRITICAL / HIGH / MEDIUM / LOW: 0 / 0 / 0 / 0.**
Три MEDIUM предыдущего раунда устранены и сняты финальным security-ревью.
Вердикт относится к текущим M5-изменениям рабочего дерева, не к старому запущенному стеку
и не разрешает публичный production/TLS-деплой.

## Устранённые Находки

| Находка | Исправление и подтверждение |
|---|---|
| MED: аналитический `session_id` из CSV использовался как bearer credential | Независимый UUIDv4 credential в HttpOnly cookie; в БД только SHA256 канонического credential и абсолютный `expires_at`. Lookup по hash и `expires_at > now()`, никогда по analytics ID. Legacy/CSV ID не аутентифицируют, DB/RNG errors fail-closed. [ADR-0005](../adr/0005-anonymity.md), [QA auth/cutover](m5-final-qa-report.md#auth-cutover) |
| MED: идентификаторы и raw errors могли попасть в логи | Убраны session IDs и сырые ошибки БД; диагностика sanitised, без credential/hash/DSN. QA проверил отсутствие проверенных credentials/hashes/analytics UUID в runtime logs, credentials/hashes в JSON/CSV. [QA](m5-final-qa-report.md#auth-cutover) |
| MED: DSN в Makefile мог раскрыться через echo/argv | `DATABASE_URL` передаётся goose через env, не аргументы; рецепт не печатается, raw tool output подавлен, ошибка обезличена. [Makefile](../../backend/Makefile), регрессия `migrations_makefile_test.go` |

Срок credential абсолютный, reuse и смена env его не продлевают. Сессии без действующего
credential получают новую пустую корзину; исторические данные не удаляются.
Схема и формат исследовательского CSV не изменены.

## Источники Проверок

| Источник | Результат и границы |
|---|---|
| Финальный security-агент | Read-only аудит production-кода и изменений; Go tests и Docker не запускал. Независимо выполнил npm audit: 7 moderate в одной dev-цепочке, runtime 0 |
| Backend-агент | После pgx/v5 v5.11.0 и x/text v0.41.0: `govulncheck` production + tests clean. Это переданный результат backend, не независимый запуск security или QA |
| [Финальный QA](m5-final-qa-report.md) | `m5-auth-pgx-r1/run-02`: 21/21 browser/API/auth/CSV групп, Chromium 375×812 и 1440×900; backend 137 top-level / 258 с subtests, 19 integration, 0 skips, normal + race; frontend 74 теста / 6 файлов, lint/build PASS |
| [SRE handoff](../../scripts/M5-SRE-RESULT.md) | Clean-source/no-cache build и smoke PASS на предыдущем pre-auth snapshot; loopback ports, non-root/read-only backend и read-only data mount проверены |
| QA после auth hardening | Актуальный образ пересобран обычным build и проверен через Go backend без Vite; no-cache после auth отдельно не повторялся. Точный image digest и immutable evidence находятся в [QA snapshot](m5-final-qa-report.md#immutable-evidence) |
| Docker bases | Поддерживаемые теги/manifest-list digests проверены и закреплены; это не полный CVE scan образов |

Полный npm audit **не чист**: 7 moderate относятся к одной dev-цепочке
typescript-eslint/minimatch 10.2.6, GHSA-g3hv-75wg-m249; `npm audit --omit=dev` даёт 0.
Это отдельно раскрытый остаточный dependency risk, не семь независимых находок и не
новый LOW финального security-вердикта. Breaking override / `audit fix --force` не применялись.

## Cutover И Данные

QA проверил migration 0002 → 0003 с остановкой старого isolated-backend перед запуском нового:
сохранены **30 sessions, 10 carts, 2 cart_items, 496 events**, связи и исторический CSV побайтово.
Итоговый QA snapshot содержит **710 событий / все 13 имён**; это тестовая воронка, не данные пилота.
Подробности и digest исторического CSV: [QA](m5-final-qa-report.md#auth-cutover).

Для любого следующего cutover необходимо предупредить активных гостей и остановить **все** старые
бинарники, использующие целевую БД, до миграции/запуска новой версии. Старые cookies намеренно
недействительны: новая сессия и пустая новая корзина, историческое исследование сохранено.
Смешивание версий, rollback к ID-auth и legacy-cookie fallback запрещены.

## Среда И Ограничения

- Принят только isolated `vkr-m5-acceptance`: `http://127.0.0.1:18080`, PostgreSQL `127.0.0.1:55432`; funnel DB `svoyshef`, destructive integration DB `m5_integration`. После локального просмотра стек остановлен по просьбе пользователя, volumes сохранены; это не утверждение о текущем healthy/running. Private env вне репо, mode `0600`; значения не публикуются.
- Исходный стек на 8080/5432 не трогали. Публикация старого backend :8080 на всех интерфейсах известна из исторического SRE evidence; текущий статус не проверялся. Он не обновлён и не покрыт этим PASS; новые loopback defaults не исправляют старый стек.
- HTTP loopback не является production/TLS acceptance. Trusted proxy/origin, TLS, Secure cookie за прокси, HSTS и публичная сеть требуют отдельной реализации и security-ревью; обход Origin guards недопустим. [Runtime safety](../../README.md#runtime-safety).
- Физические iOS Safari/Android Chrome, камера/QR, принтер и скан распечатки не проверены. SRE проверил offline Chromium PDF-печать QR: 20 SVG, 3 страницы A4 без обрезки карточек.
- Lighthouse M4: 70/70/70 на preview; повторного Lighthouse на compose не было. Clean clone без локальных фото деградирует до placeholder, а не подтверждает offline photo fallback.

## Review Loop

- Архитектор: **OK**, 0 BLOCKER/HIGH/MED; два LOW про устаревшие комментарии (Docker Go ≥1.25 и README о test skips) исправлены координатором.
- Ревьюер требований: предстартовый **PASS**, auth/docs согласованы; финальный независимый **PASS / OK** закрытию локальной M5 получен 09.11.2026. Проверены 45 Must без пустых «Проверено» и совпадение sanitized summaries; MINOR о порядке каскада в `20-architecture.md:61` исправлен на `expert → upsell → heuristic`.
- Security: отдельный явный ответ **«OK security-docs. Security gate local M5 remains PASS 0CRIT/HIGH/MED/LOW»** получен 09.11.2026. Это read-only подтверждение документации и сохранения прежнего PASS, не новый runtime-аудит: runtime-проверки и сканеры не повторялись. Requirements-reviewer не заменял security; получены оба независимых ответа.
- Аналитик: **OK** `events.md` и QA/CSV evidence; QA: **OK** локальному скоупу; security: **PASS**, как указано выше.

**M5 закрыта в локальном скоупе прототипа, все обязательные гейты закрыты.**
Основания финальных docs-апрувов: [review-report](m5-final-review-report.md); координатор
финализирует закрытие после передачи документации. Код после принятия не менялся;
коммит/push в рамках документирования не выполнялись.
