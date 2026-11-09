import base64
import subprocess
import sys
import tempfile
import unittest
import xml.etree.ElementTree as ET
from html.parser import HTMLParser
from pathlib import Path
from unittest.mock import patch

import generate_qr as seed


class PrintPage(HTMLParser):
    def __init__(self):
        super().__init__()
        self.images = []
        self.text = []

    def handle_starttag(self, tag, attrs):
        if tag == "img":
            self.images.append(dict(attrs))
        if tag in ("script", "iframe", "link"):
            raise AssertionError("print page must be self-contained")

    def handle_data(self, data):
        self.text.append(data)


class TestFR34QRSeed(unittest.TestCase):
    def test_origins(self):
        for value in ("http://localhost:18080", "https://menu.example/",
                      "http://127.0.0.1:8080/", "http://[::1]:8080"):
            with self.subTest(value=value):
                self.assertEqual(seed.validate_base_url(value), value.rstrip("/") + "/")

    def test_untrusted_url_rejected(self):
        for value in ("", "ftp://menu.example", "//menu.example", "javascript:alert(1)",
                      "http://user@menu.example", "http://user:pass@menu.example",
                      "https://menu.example?", "https://menu.example#",
                      "http://menu.example/?x=1", "http://menu.example/#x",
                      "http://menu.example/path", "http://menu.example//",
                      "http://menu.example:0", "http://menu.example:65536",
                      "http://menu.example:", "http://[broken", "http://[::1]evil",
                      "http://[::1]evil:80", "http://menu.example..", "http://-bad.example",
                      "http://menu..example", "http://999.1.1.1", "http://menu%2eexample",
                      "http://menu.example\\@other", "http://menu.example\n",
                      "http://menu.example/a\tb", 'http://menu.example/\"<script>'):
            with self.subTest(value=value), self.assertRaises(ValueError):
                seed.validate_base_url(value)

    def test_range_and_subset(self):
        self.assertEqual(seed.table_numbers(1, 5, "all"), [1, 2, 3, 4, 5])
        self.assertEqual(seed.table_numbers(1, 5, "odd"), [1, 3, 5])
        self.assertEqual(seed.table_numbers(1, 5, "even"), [2, 4])
        self.assertEqual(seed.table_numbers(999, 999, "all"), [999])
        self.assertEqual(seed.table_numbers(1, 1, "even"), [])
        for start, end, subset in ((0, 20, "all"), (1, 1000, "all"),
                                   (3, 2, "all"), (-1, -1, "all"), (1, 2, "bad")):
            with self.subTest(start=start, end=end), self.assertRaises(SystemExit):
                seed.table_numbers(start, end, subset)

    def test_quiet_zone_and_payload(self):
        original = seed.qrcode.make
        with patch.object(seed.qrcode, "make", wraps=original) as make:
            svg = seed.qr_svg("http://localhost:18080/?table=999")
        self.assertEqual(make.call_args.args, ("http://localhost:18080/?table=999",))
        self.assertGreaterEqual(make.call_args.kwargs["border"], 4)
        root = ET.fromstring(svg)
        self.assertEqual(root.tag, "{http://www.w3.org/2000/svg}svg")
        self.assertIsNotNone(root.find("{http://www.w3.org/2000/svg}path"))
        background = root.find("{http://www.w3.org/2000/svg}rect")
        self.assertEqual(background.get("fill"), "white")

    def test_printed_result_all_subsets(self):
        for subset, expected in (("all", [1, 2, 3]), ("odd", [1, 3]), ("even", [2])):
            with self.subTest(subset=subset), tempfile.TemporaryDirectory() as tmp:
                result = subprocess.run(
                    [sys.executable, seed.__file__, "--base-url", "http://localhost:18080/",
                     "--from", "1", "--to", "3", "--subset", subset, "--out", tmp],
                    capture_output=True, check=True)
                self.assertEqual(result.stderr, b"")
                page = (Path(tmp) / "index.html").read_text(encoding="utf-8")
                parsed = PrintPage()
                parsed.feed(page)
                self.assertEqual(len(parsed.images), len(expected))
                self.assertIn("@page { size: A4", page)
                self.assertIn("break-inside: avoid", page)
                for image, number in zip(parsed.images, expected):
                    self.assertIn(f"Столик {number}", parsed.text)
                    self.assertIn(f"http://localhost:18080/?table={number}", parsed.text)
                    prefix, data = image["src"].split(",", 1)
                    self.assertEqual(prefix, "data:image/svg+xml;base64")
                    svg = (Path(tmp) / f"table-{number:03d}.svg").read_bytes()
                    self.assertEqual(base64.b64decode(data, validate=True), svg)
                    self.assertEqual(svg, seed.qr_svg(f"http://localhost:18080/?table={number}"))

    def test_invalid_cli_creates_no_output(self):
        with tempfile.TemporaryDirectory() as tmp:
            out = Path(tmp) / "out"
            for flags in (["--base-url", "https://user@menu.example"],
                          ["--from", "1000"], ["--from", "1", "--to", "1", "--subset", "even"]):
                result = subprocess.run([sys.executable, seed.__file__, "--out", str(out), *flags],
                                        capture_output=True)
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(result.stdout, b"")
                self.assertNotIn(b"user@", result.stderr)
                self.assertFalse(out.exists())


if __name__ == "__main__":
    unittest.main()
