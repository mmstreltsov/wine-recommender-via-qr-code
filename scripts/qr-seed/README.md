# QR Seed

Ops-tooling M5, FR-34: SVG для каждого столика и автономная HTML-страница A4 с
встроенными SVG, подписями и читаемыми URL. SVG сохраняет резкость при печати;
белый фон и quiet zone = 4 модуля, коррекция M. Печать из браузера, без сервера.

## Зависимость

Python 3.11+ рекомендуется (библиотека поддерживает 3.9–3.13).
`qrcode==8.2`: BSD-3-Clause (с сохранённым upstream MIT notice), Lincoln Loop, stable; последний опубликованный релиз
на PyPI при проверке 08.11.2026, 01.05.2025, поддержка Python 3.13 добавлена в 8.1.
Это небольшая изолированная ops-зависимость, не часть runtime приложения.
Реализовывать QR-кодировщик самостоятельно рискованнее. Используем только SVG
factory на stdlib XML: без Pillow, pypng, native libraries, сетевых QR-сервисов.
Wheel закреплён версией и SHA-256 из PyPI; нет гарантии долгосрочного SLA поддержки.
Источники: https://pypi.org/project/qrcode/8.2/ и
https://github.com/lincolnloop/python-qrcode/blob/v8.2/LICENSE.

```bash
python3 -m venv scripts/qr-seed/.venv
scripts/qr-seed/.venv/bin/python -m pip install --only-binary=:all: --require-hashes -r scripts/qr-seed/requirements.txt
scripts/qr-seed/.venv/bin/python -m unittest discover -s scripts/qr-seed -v
scripts/qr-seed/.venv/bin/python scripts/qr-seed/generate_qr.py --base-url http://127.0.0.1:18080
```

Выход по умолчанию: `scripts/qr-seed/out/index.html`, `table-001.svg` … `table-020.svg`.
Для каждого запуска используй отдельный `--out DIR`: старые файлы не удаляются.
В браузере открой `index.html`, выбери A4, масштаб 100%, без колонтитулов.
Перед тиражом проверь сканирование реальной распечатки телефоном.

Дополнительный браузерный тест печати использует уже установленный Playwright QA,
не добавляя его в зависимости приложения. Он открывает страницу offline, проверяет
все изображения и print CSS, создаёт PDF A4 и screenshot. Путь к готовому модулю и
браузерам можно передать env `PLAYWRIGHT_MODULE` и `PLAYWRIGHT_BROWSERS_PATH`.

```bash
node scripts/qr-seed/print-smoke.cjs scripts/qr-seed/out/index.html scripts/qr-seed/out/qr-a4.pdf 20
```

## Параметры

| Параметр | Дефолт | Ограничения |
|---|---|---|
| `--base-url` | `http://localhost:8080` | Только http/https origin; optional завершающий `/`. Credentials, query, fragment, path запрещены. Подкаталоги SPA не поддерживает. IDN передавать в ASCII/punycode |
| `--from` / `--to` | `1` / `20` | Включительно, `1 <= from <= to <= 999` |
| `--subset` | `all` | `all`, `odd`, `even`; пустое подмножество считается ошибкой |
| `--out` | `scripts/qr-seed/out` | Новый каталог или существующий операторский каталог |

`odd/even` сохраняет предусмотренные планом подмножества для исследовательских
распечаток A/B, но НЕ реализует распределение гостей, рандомизацию или переключение
продуктовых функций. Ссылки отличаются только `?table=N`, не группой A/B.
Число столиков пилота уточняет заказчик, 1–20 является лишь дефолтом.

Генератор принимает https, но не настраивает TLS и не доказывает готовность деплоя.
Loopback URL подходит только для QA на том же компьютере, не для телефона гостя.
Публичный запуск и произвольный TLS reverse proxy вне M5; ограничения описаны в корневом README.
