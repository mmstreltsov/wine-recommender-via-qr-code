#!/usr/bin/env python3
"""NFR-05/UI-01/NFR-04: read-only loopback HTTP smoke, no guest funnel writes."""
import argparse
import json
import re
from urllib.error import URLError
from urllib.parse import urlsplit
from urllib.request import urlopen


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-url", default="http://127.0.0.1:8080")
    args = parser.parse_args()
    parts = urlsplit(args.base_url)
    if (parts.scheme != "http" or parts.hostname not in ("127.0.0.1", "localhost", "::1")
            or parts.username is not None or parts.password is not None
            or parts.path not in ("", "/") or "?" in args.base_url or "#" in args.base_url):
        parser.error("use a local HTTP origin without credentials, path, query or fragment")
    base = args.base_url.rstrip("/")

    def get(path):
        with urlopen(base + path, timeout=15) as response:
            assert response.status == 200, path
            assert response.headers.get("X-Content-Type-Options") == "nosniff", path
            csp = response.headers.get("Content-Security-Policy", "")
            for directive in ("default-src 'self'", "script-src 'self'", "font-src 'self'",
                              "connect-src 'self'", "frame-ancestors 'none'"):
                assert directive in csp, path
            assert "unsafe-inline" not in csp and "unsafe-eval" not in csp, path
            if path.startswith("/api/"):
                assert response.headers.get("Cache-Control") == "no-store", path
            return response.read(), response.headers

    body, headers = get("/api/v1/health")
    assert json.loads(body)["status"] == "ok"
    assert not headers.get("Set-Cookie"), "health must not create sessions"
    index, _ = get("/")
    assert b'<div id="root"' in index
    for route in ("/cart", "/dish/655892482392", "/pay-stub"):
        assert get(route)[0] == index, route
    assets = re.findall(rb'(?:src|href)="(/assets/[^"<>]+)"', index)
    assert len(assets) >= 2, "missing built JS/CSS"
    for path in assets:
        assert get(path.decode("ascii"))[0], "empty built asset"
    for family in ("unbounded", "montserrat"):
        for subset in ("cyrillic", "latin", "currency"):
            font, headers = get(f"/static/aidabaran/assets/fonts/{family}-{subset}.woff2")
            assert font.startswith(b"wOF2"), "invalid font"
            assert "font/woff2" in headers.get("Content-Type", ""), "invalid font MIME"
    print("PASS: health (no session), SPA/deep links, built assets, six WOFF2 fonts, CSP/nosniff/no-store")


if __name__ == "__main__":
    try:
        main()
    except (AssertionError, URLError, ValueError, KeyError):
        raise SystemExit("Smoke failed; check local service health, assets and headers.") from None
