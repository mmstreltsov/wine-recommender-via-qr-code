package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Dev-режим (vite :5173) и CSP: CSP ставится ТОЛЬКО на ответы бэкенда — страницу
// в деве отдаёт vite без этого заголовка (React-refresh/HMR-скрипты вставляет
// vite, они CSP бэкенда не нарушают). /api в деве приходит через vite-прокси —
// same-origin запрос, connect-src 'self' соблюдается; HMR-ws подключается к
// vite-источнику (:5173), а не к бэкенду, — бэкендной CSP не регулируется.
// Проверено вручную в M4 через штатный vite-прокси; тесты ниже фиксируют прод-
// поведение (backend раздаёт SPA+статику+API, все ответы с CSP).

// TestNFR04_CSPHeaderOnAPIResponses: CSP присутствует на всех API-ответах
// (200/204/404) и равен контракту.
func TestNFR04_CSPHeaderOnAPIResponses(t *testing.T) {
	sessions := newFakeSessions()
	deps, _, _ := fullDeps(t, sessions)
	srv := httptest.NewServer(newTestHandler(deps))
	defer srv.Close()
	for _, path := range []string{"/api/v1/health", "/api/v1/config", "/api/v1/menu", "/api/v1/nope"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if got := resp.Header.Get("Content-Security-Policy"); got != contentSecurityPolicy {
			t.Fatalf("%s: CSP wrong:\nwant %q\ngot  %q", path, contentSecurityPolicy, got)
		}
	}
}

// TestNFR04_CSPHeaderOnStaticAndSPA: статику данных и SPA из dist бэкенд
// отдаёт с тем же CSP (прод-режим: все ответы — от бэкенда).
func TestNFR04_CSPHeaderOnStaticAndSPA(t *testing.T) {
	root := t.TempDir()
	assets := filepath.Join(root, "testaurant", "assets")
	if err := os.MkdirAll(assets, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assets, "logo.png"), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	dist := t.TempDir()
	if err := os.WriteFile(filepath.Join(dist, "index.html"), []byte("<!doctype html><html></html>"), 0o644); err != nil {
		t.Fatal(err)
	}

	sessions := newFakeSessions()
	deps, _, _ := fullDeps(t, sessions)
	deps.DataDir = root
	deps.FrontendDist = dist
	srv := httptest.NewServer(newTestHandler(deps))
	defer srv.Close()

	for _, path := range []string{"/static/testaurant/assets/logo.png", "/", "/dish/123"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: want 200, got %d", path, resp.StatusCode)
		}
		if got := resp.Header.Get("Content-Security-Policy"); got != contentSecurityPolicy {
			t.Fatalf("%s: CSP missing/wrong: %q", path, got)
		}
	}
	for _, path := range []string{"/api", "/api/", "/api/v1/nope", "/api/v2/menu", "/api/export-events"} {
		rec := httptest.NewRecorder()
		newTestHandler(deps).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound || rec.Header().Get("Content-Type") != "application/json; charset=utf-8" || !strings.Contains(rec.Body.String(), `"code":"not_found"`) {
			t.Fatalf("%s: want JSON 404 instead of SPA, got %d %q", path, rec.Code, rec.Body.String())
		}
	}
}

// TestNFR04_CSPNoUnsafeInline: 'unsafe-inline' в CSP не нужен и запрещён —
// фронтенд не использует инлайн-стили (frontend/src без style=-атрибутов,
// dist подключает только внешние css/js — проверено по факту в M4/M5).
// Тест фиксирует контракт: появление unsafe-inline = регрессия.
func TestNFR04_CSPNoUnsafeInline(t *testing.T) {
	if strings.Contains(contentSecurityPolicy, "unsafe-inline") {
		t.Fatalf("CSP must not contain unsafe-inline: %q", contentSecurityPolicy)
	}
	for _, directive := range []string{
		"default-src 'self'",
		"script-src 'self'",
		"style-src 'self'",
		"connect-src 'self'",
		"font-src 'self'",
		"base-uri 'self'",
		"frame-ancestors 'none'",
		"form-action 'self'",
	} {
		if !strings.Contains(contentSecurityPolicy, directive) {
			t.Fatalf("CSP missing directive %q: %q", directive, contentSecurityPolicy)
		}
	}
}

func TestNFR04_CSPExplicitImageCDNAllowlist(t *testing.T) {
	for _, directive := range strings.Split(contentSecurityPolicy, "; ") {
		if !strings.HasPrefix(directive, "img-src ") {
			continue
		}
		want := "img-src 'self' https://static.tildacdn.com https://optim.tildacdn.com https://static.tildacdn.net https://optim.tildacdn.net"
		if directive != want {
			t.Fatalf("image sources must be explicit CDN hosts accepted by safeRemoteImage, got %q", directive)
		}
		return
	}
	t.Fatal("missing img-src")
}

// TestNFR02_FontsImmutableCacheControl: self-host woff2-шрифты (UI-01) неизменны —
// Cache-Control: public, max-age=31536000, immutable; остальные ассеты кэш-
// заголовок не получают (не-woff2 в fonts/, logo, фото).
func TestNFR02_FontsImmutableCacheControl(t *testing.T) {
	root := t.TempDir()
	fonts := filepath.Join(root, "testaurant", "assets", "fonts")
	if err := os.MkdirAll(fonts, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fonts, "unbounded-700.woff2"), []byte("woff2"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fonts, "notes.css"), []byte("/* x */"), 0o644); err != nil {
		t.Fatal(err)
	}
	assets := filepath.Join(root, "testaurant", "assets")
	if err := os.WriteFile(filepath.Join(assets, "logo.png"), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}

	h := newStaticHandler(root, "testaurant")
	get := func(p string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	rec := get("/static/testaurant/assets/fonts/unbounded-700.woff2")
	if rec.Code != http.StatusOK {
		t.Fatalf("font: want 200, got %d", rec.Code)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "public, max-age=31536000, immutable" {
		t.Fatalf("font Cache-Control wrong: %q", cc)
	}

	for _, p := range []string{
		"/static/testaurant/assets/fonts/notes.css",
		"/static/testaurant/assets/logo.png",
	} {
		rec := get(p)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: want 200, got %d", p, rec.Code)
		}
		if cc := rec.Header().Get("Cache-Control"); cc != "" {
			t.Fatalf("%s: must not set Cache-Control, got %q", p, cc)
		}
	}
}
