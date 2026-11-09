package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"svoyshef/internal/store"
	"svoyshef/plugins/aidabaran"
)

func exportDay(y int, m time.Month, d int) *time.Time {
	t := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	return &t
}

// TestAN04_ParseExportRange_Valid: флаги опциональны по отдельности (нет — граница
// открыта), формат строго YYYY-MM-DD, --to → полуинтервал (день входит включительно).
func TestAN04_ParseExportRange_Valid(t *testing.T) {
	cases := []struct {
		name   string
		args   []string
		from   *time.Time
		toExcl *time.Time
	}{
		{"no flags", nil, nil, nil},
		{"from only", []string{"--from", "2026-11-01"}, exportDay(2026, 11, 1), nil},
		{"to only", []string{"--to", "2026-11-30"}, nil, exportDay(2026, 12, 1)},
		{"both", []string{"--from", "2026-11-10", "--to", "2026-11-20"}, exportDay(2026, 11, 10), exportDay(2026, 11, 21)},
		{"to month end rolls over", []string{"--to", "2026-11-30"}, nil, exportDay(2026, 12, 1)},
		{"to year end rolls over", []string{"--to", "2026-12-31"}, nil, exportDay(2027, 1, 1)},
		{"leap day", []string{"--from", "2028-02-29", "--to", "2028-02-29"}, exportDay(2028, 2, 29), exportDay(2028, 3, 1)},
		{"equals form", []string{"--from=2026-11-01"}, exportDay(2026, 11, 1), nil},
		{"single dash form", []string{"-from", "2026-11-01"}, exportDay(2026, 11, 1), nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			from, toExcl, err := parseExportRange(tc.args)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !timePtrEqual(from, tc.from) {
				t.Fatalf("from: want %v, got %v", tc.from, from)
			}
			if !timePtrEqual(toExcl, tc.toExcl) {
				t.Fatalf("toExcl: want %v, got %v", tc.toExcl, toExcl)
			}
			if from != nil && from.Location() != time.UTC {
				t.Fatalf("from must be UTC, got %v", from.Location())
			}
			if toExcl != nil && toExcl.Location() != time.UTC {
				t.Fatalf("toExcl must be UTC, got %v", toExcl.Location())
			}
		})
	}
}

func timePtrEqual(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

// TestAN04_ParseExportRange_Invalid: всё, кроме строго YYYY-MM-DD, — ошибка
// (сообщение уходит в stderr через main, stdout остаётся пуст).
func TestAN04_ParseExportRange_Invalid(t *testing.T) {
	bad := [][]string{
		{"--from", ""},
		{"--to", ""},
		{"--from="},
		{"--to="},
		{"--from", "2026-11-21", "--to", "2026-11-20"},
		{"--from", "2026-11-1"},
		{"--from", "2026-1-10"},
		{"--from", "2026/11/10"},
		{"--from", "10.11.2026"},
		{"--from", "2026-13-01"},
		{"--from", "2026-00-10"},
		{"--from", "2026-02-30"},
		{"--from", "2026-11-10T00:00:00Z"},
		{"--from", "2026-11-10 "},
		{"--from", "garbage"},
		{"--to", "2026-11-1x"},
		{"--to", "26-11-10"},
		{"--from"},
		{"--to"},
		{"--limit", "10"},
		{"extra-positional"},
		{"--from", "2026-11-01", "--to"},
	}
	for _, args := range bad {
		from, toExcl, err := parseExportRange(args)
		if err == nil {
			t.Fatalf("parseExportRange(%q): expected error, got from=%v to=%v", args, from, toExcl)
		}
		if !strings.Contains(err.Error(), "export-events") && !strings.Contains(err.Error(), "YYYY-MM-DD") && !strings.Contains(err.Error(), "--") {
			t.Fatalf("error must mention the flag/format: %v", err)
		}
	}
}

// TestAN04_WriteEventsCSV_Format: заголовок обязателен; ts — RFC 3339 UTC с
// микросекундами; table_id NULL → пустое поле; payload в кавычках по RFC 4180
// (кавычки удваиваются — csv.Reader читает обратно без потерь); LF, UTF-8, без BOM.
func TestAN04_WriteEventsCSV_Format(t *testing.T) {
	tbl := 12
	msk := time.FixedZone("MSK", 3*60*60)
	rows := []store.ExportedEvent{
		{
			TS:        time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC),
			SessionID: "11111111-1111-1111-1111-111111111111",
			TableID:   &tbl,
			Name:      "rec_impression",
			Payload:   `{"context": "dish", "rec_source": "expert", "rec_item_id": 366609864022}`,
		},
		{
			TS:        time.Date(2026, 11, 10, 12, 34, 56, 123456000, msk), // не-UTC зона → конвертация в UTC
			SessionID: "22222222-2222-2222-2222-222222222222",
			TableID:   nil,
			Name:      "page_open",
			Payload:   "{}",
		},
	}
	var buf bytes.Buffer
	if err := writeEventsCSV(&buf, rows); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	if !strings.HasPrefix(out, "ts,session_id,table_id,name,payload\n") {
		t.Fatalf("header line wrong: %q", out)
	}
	if strings.Contains(out, "\r") {
		t.Fatal("line endings must be LF only")
	}
	if strings.HasPrefix(out, "\xef\xbb\xbf") {
		t.Fatal("BOM must not be present")
	}
	if !strings.Contains(out, `"{""context"": ""dish"", ""rec_source"": ""expert"", ""rec_item_id"": 366609864022}"`) {
		t.Fatalf("payload field must be RFC4180-quoted with doubled quotes, got:\n%s", out)
	}
	if !strings.Contains(out, ",page_open,\"{}\"\n") {
		t.Fatalf("empty object payload must also be quoted, got:\n%s", out)
	}

	recs, err := csv.NewReader(strings.NewReader(out)).ReadAll()
	if err != nil {
		t.Fatalf("csv reader (RFC 4180 round-trip): %v", err)
	}
	want := [][]string{
		{"ts", "session_id", "table_id", "name", "payload"},
		{"2026-11-10T00:00:00.000000Z", "11111111-1111-1111-1111-111111111111", "12", "rec_impression", `{"context": "dish", "rec_source": "expert", "rec_item_id": 366609864022}`},
		{"2026-11-10T09:34:56.123456Z", "22222222-2222-2222-2222-222222222222", "", "page_open", "{}"},
	}
	if len(recs) != len(want) {
		t.Fatalf("want %d records, got %d: %+v", len(want), len(recs), recs)
	}
	for i := range want {
		if strings.Join(recs[i], "|") != strings.Join(want[i], "|") {
			t.Fatalf("record %d:\nwant %q\ngot  %q", i, want[i], recs[i])
		}
	}
}

// TestAN04_WriteEventsCSV_EmptySelectionHeaderOnly: пустая выборка → только заголовок.
func TestAN04_WriteEventsCSV_EmptySelectionHeaderOnly(t *testing.T) {
	var buf bytes.Buffer
	if err := writeEventsCSV(&buf, nil); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "ts,session_id,table_id,name,payload\n" {
		t.Fatalf("empty selection must print header only, got %q", buf.String())
	}
}

// TestAN04_ExportEvents_InvalidFlagStdoutEmpty: невалидный флаг → ошибка, stdout пуст.
func TestAN04_ExportEvents_InvalidFlagStdoutEmpty(t *testing.T) {
	for _, args := range [][]string{
		{"--from", "not-a-date"},
		{"--to", "2026-11-99"},
		{"--bogus"},
	} {
		var buf bytes.Buffer
		err := runExportEvents(args, &buf)
		if err == nil {
			t.Fatalf("runExportEvents(%q): expected error", args)
		}
		if buf.Len() != 0 {
			t.Fatalf("stdout must be empty on flag error, got %q", buf.String())
		}
	}
}

// TestAN04_ExportEvents_DBUnavailableStdoutEmpty: недоступная БД → ошибка,
// stdout пуст (CSV собирается в буфер и не попадает в stdout при сбое).
func TestAN04_ExportEvents_DBUnavailableStdoutEmpty(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://127.0.0.1:1/unavailable?sslmode=disable&connect_timeout=1")
	var buf bytes.Buffer
	err := runExportEvents(nil, &buf)
	if err == nil {
		t.Fatal("expected connect error")
	}
	if buf.Len() != 0 {
		t.Fatalf("stdout must be empty on DB error, got %q", buf.String())
	}
}

// TestAN04_ExportEvents_RequiresDatabaseURL: без DATABASE_URL — ошибка до коннекта.
func TestAN04_ExportEvents_RequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	var buf bytes.Buffer
	if err := runExportEvents(nil, &buf); err == nil {
		t.Fatal("expected DATABASE_URL required error")
	}
	if buf.Len() != 0 {
		t.Fatalf("stdout must be empty, got %q", buf.String())
	}
}

// TestAN04_LoadConfig_FrontendDistDefault: env FRONTEND_DIST переопределяет дефолт.
func TestAN04_LoadConfig_FrontendDistDefault(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/db")
	t.Setenv("FRONTEND_DIST", "")
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.FrontendDist != "../frontend/dist" {
		t.Fatalf("default FRONTEND_DIST wrong: %q", cfg.FrontendDist)
	}
	t.Setenv("FRONTEND_DIST", "/srv/spa")
	cfg, err = loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.FrontendDist != "/srv/spa" {
		t.Fatalf("FRONTEND_DIST env not applied: %q", cfg.FrontendDist)
	}
}

func TestAN04_ExportEvents_CLIProcessErrors(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "server")
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, output)
	}
	cases := []struct {
		name string
		args []string
		dsn  string
		want string
	}{
		{"empty from", []string{"--from", ""}, "", "--from"},
		{"empty to", []string{"--to="}, "", "--to"},
		{"strict date", []string{"--from", "2026-1-01"}, "", "YYYY-MM-DD"},
		{"unknown flag", []string{"--bogus"}, "", "флаги"},
		{"missing value", []string{"--to"}, "", "флаги"},
		{"reverse range before config or DB", []string{"--from", "2026-11-21", "--to", "2026-11-20"}, "", "--from не может быть позже --to"},
		{"unavailable DB", nil, "postgres://127.0.0.1:1/unavailable?sslmode=disable&connect_timeout=1", "не удалось подключиться к БД"},
		{"invalid DB config", nil, "://invalid", "не удалось подключиться к БД"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", tc.dsn)
			t.Setenv("RESTAURANT", "aidabaran")
			t.Setenv("COOKIE_MAX_AGE_H", "12")
			t.Setenv("AVAILABILITY_TTL_S", "5")
			cmd := exec.CommandContext(ctx, binary, append([]string{"export-events"}, tc.args...)...)
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			err := cmd.Run()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 1 {
				t.Fatalf("want exit 1, got %v", err)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout must be empty: %q", stdout.String())
			}
			if !strings.Contains(stderr.String(), tc.want) {
				t.Fatalf("stderr must contain %q: %q", tc.want, stderr.String())
			}
		})
	}
}

// TestNFR05_SpaServedFromConfiguredDist: прод-режим раздаёт SPA из каталога
// FRONTEND_DIST (задача 2: путь конфигурируем, не хардкод). SPA-маршруты не
// ходят в БД, поэтому пул не нужен.
func TestNFR05_SpaServedFromConfiguredDist(t *testing.T) {
	dist := t.TempDir()
	if err := os.WriteFile(filepath.Join(dist, "index.html"), []byte("<!doctype html><title>spa-ok</title>"), 0o644); err != nil {
		t.Fatal(err)
	}
	loadResult, err := aidabaran.LoadMenu("../../../data", "aidabaran")
	if err != nil {
		t.Fatal(err)
	}
	cfg := config{
		Port:            "0",
		DatabaseURL:     "unused",
		Restaurant:      "aidabaran",
		DataDir:         "../../../data",
		FrontendDist:    dist,
		CookieMaxAgeH:   12,
		AvailabilityTTL: itTTL,
	}
	handler, _ := buildHandler(cfg, nil, loadResult, nil, nil)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	for _, path := range []string{"/", "/dish/655892482392", "/cart"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "spa-ok") {
			t.Fatalf("%s: want 200 with index.html from configured dist, got %d %q", path, resp.StatusCode, body)
		}
	}
}
