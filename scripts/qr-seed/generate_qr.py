#!/usr/bin/env python3
"""Генератор печатных QR-кодов столиков «Свое Шеф» (ops-tooling, вне реестра требований).

Ссылка QR: <base-url>/?table=N (FR-34). Выход: SVG на каждый столик + самодостаточная
HTML-страница-индекс (сетка A4, подпись «Столик N»), печатается из браузера без сервера.
"""
import argparse
import base64
import html
import io
import ipaddress
import re
import sys
from pathlib import Path
from urllib.parse import urlsplit

try:
    import qrcode
    from qrcode.image.svg import SvgPathFillImage
except ImportError:
    print(
        "Не найдена библиотека qrcode (опциональная dev-зависимость).\n"
        "Установи в venv: python -m pip install --require-hashes -r scripts/qr-seed/requirements.txt\n"
        "Скрипт ничего не скачивает сам.",
        file=sys.stderr,
    )
    sys.exit(2)

MAX_TABLE = 999


def parse_args() -> argparse.Namespace:
    p = argparse.ArgumentParser(
        description="QR-коды для столиков: SVG на каждый столик + печатная HTML-страница (A4)."
    )
    p.add_argument("--base-url", default="http://localhost:8080",
                   help="базовый URL сайта (дефолт: http://localhost:8080)")
    p.add_argument("--from", dest="from_", type=int, default=1, metavar="N",
                   help="первый столик (дефолт: 1)")
    p.add_argument("--to", dest="to_", type=int, default=20, metavar="N",
                   help="последний столик (дефолт: 20)")
    p.add_argument("--subset", choices=["all", "odd", "even"], default="all",
                   help="операторский фильтр по чётности, не продуктовая A/B-логика (дефолт: all)")
    p.add_argument("--out", default=None, metavar="DIR",
                   help="каталог вывода (дефолт: scripts/qr-seed/out/)")
    return p.parse_args()


def table_numbers(from_: int, to_: int, subset: str) -> list[int]:
    if from_ < 1 or to_ > MAX_TABLE or from_ > to_:
        raise SystemExit(f"некорректный диапазон столиков: {from_}..{to_} (допустимо 1..{MAX_TABLE})")
    if subset not in ("all", "odd", "even"):
        raise SystemExit("некорректное подмножество")
    nums = list(range(from_, to_ + 1))
    if subset == "odd":
        nums = [n for n in nums if n % 2 == 1]
    elif subset == "even":
        nums = [n for n in nums if n % 2 == 0]
    return nums


def validate_base_url(value: str) -> str:
    # Reject paths: the SPA uses root-relative routes/assets and has no subpath deployment.
    error = "base-url: требуется http(s) origin; без credentials, path, query и fragment"
    if not value or any(ord(c) <= 32 or ord(c) >= 127 for c in value):
        raise ValueError(error)
    if any(c in value for c in "?#\\%<>\"'"):
        raise ValueError(error)
    try:
        parts = urlsplit(value)
        if (parts.scheme not in ("http", "https") or not parts.netloc
                or parts.username is not None or parts.password is not None
                or parts.path not in ("", "/") or not parts.hostname
                or parts.netloc.endswith(":")):
            raise ValueError(error)
        if parts.port is not None and not 1 <= parts.port <= 65535:
            raise ValueError(error)
        host = parts.hostname
        authority = r"\[[0-9a-fA-F:.]+\](?::[0-9]+)?" if ":" in host else r"[a-zA-Z0-9.-]+(?::[0-9]+)?"
        if not re.fullmatch(authority, parts.netloc):
            raise ValueError(error)
        if ":" in host or re.fullmatch(r"[0-9.]+", host):
            ipaddress.ip_address(host)
        elif len(host) > 253 or not all(
            re.fullmatch(r"[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?", label)
            for label in (host[:-1] if host.endswith(".") else host).split(".")
        ):
            raise ValueError(error)
    except ValueError:
        raise ValueError(error) from None
    return f"{parts.scheme}://{parts.netloc}/"


def qr_svg(url: str) -> bytes:
    img = qrcode.make(url, image_factory=SvgPathFillImage,
                      error_correction=qrcode.constants.ERROR_CORRECT_M, box_size=10, border=4)
    buf = io.BytesIO()
    img.save(buf)
    return buf.getvalue()


HTML_TEMPLATE = """<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<title>QR-коды столиков — Свое Шеф</title>
<style>
  :root { --gap: 6mm; }
  * { box-sizing: border-box; }
  body { font-family: system-ui, sans-serif; margin: 0; color: #111; }
  .header { padding: 4mm 6mm 2mm; }
  .header h1 { font-size: 14pt; margin: 0 0 1mm; }
  .header p { font-size: 9pt; margin: 0; color: #444; }
  .grid {
    display: grid; grid-template-columns: repeat(3, 1fr);
    gap: var(--gap); padding: var(--gap);
  }
  .cell {
    border: 0.4mm dashed #999; border-radius: 2mm;
    padding: 4mm; text-align: center;
    display: flex; flex-direction: column; align-items: center; gap: 2mm;
  }
  .cell img { width: 100%; max-width: 52mm; height: auto; }
  .caption { font-size: 11pt; font-weight: 600; }
  .url { font-size: 7pt; overflow-wrap: anywhere; }
  @media print {
    @page { size: A4; margin: 8mm; }
    .header { display: none; }
    .grid { grid-template-columns: repeat(3, 1fr); gap: 5mm; padding: 0; }
    .cell { break-inside: avoid; page-break-inside: avoid; }
  }
</style>
</head>
<body>
  <div class="header">
  <h1>QR-коды столиков — Свое Шеф</h1>
  <p>__URL_LINE__</p>
</div>
<div class="grid">
__CELLS__
</div>
</body>
</html>
"""

CELL_TEMPLATE = """<div class="cell">
  <img src="data:image/svg+xml;base64,{b64}" alt="QR: столик {n}">
  <div class="caption">Столик {n}</div>
  <div class="url">{url}</div>
</div>"""


def build_html(base_url: str, subset: str, images: dict[int, bytes]) -> str:
    url_line = html.escape(f"Ссылка QR: {base_url}?table=N · подмножество: {subset}")
    cells = "\n".join(
        CELL_TEMPLATE.format(b64=base64.b64encode(images[n]).decode("ascii"), n=n,
                             url=html.escape(f"{base_url}?table={n}"))
        for n in sorted(images)
    )
    return (HTML_TEMPLATE
            .replace("__URL_LINE__", url_line)
            .replace("__CELLS__", cells))


def main() -> None:
    args = parse_args()
    out_dir = Path(args.out) if args.out else Path(__file__).resolve().parent / "out"
    try:
        base_url = validate_base_url(args.base_url)
    except ValueError as error:
        raise SystemExit(str(error)) from None
    nums = table_numbers(args.from_, args.to_, args.subset)
    if not nums:
        raise SystemExit(f"диапазон {args.from_}..{args.to_} с subset={args.subset} дал пустой набор")

    out_dir.mkdir(parents=True, exist_ok=True)
    images: dict[int, bytes] = {}
    for n in nums:
        data = qr_svg(f"{base_url}?table={n}")
        (out_dir / f"table-{n:03d}.svg").write_bytes(data)
        images[n] = data
    page = out_dir / "index.html"
    page.write_text(build_html(base_url, args.subset, images), encoding="utf-8")

    print(f"OK: {len(nums)} QR → {out_dir}/ (table-*.svg)")
    print(f"Печатная страница: {page} (открой в браузере и распечатай; SVG встроены)")


if __name__ == "__main__":
    main()
