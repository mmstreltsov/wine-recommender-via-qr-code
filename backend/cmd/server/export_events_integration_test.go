package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Гейт интеграционных тестов — M2_IT_DATABASE_URL (как в main_integration_test.go).

func seedExportEvent(t *testing.T, pool *pgxpool.Pool, sid string, tableID *int, name, payloadJSON string, ts time.Time) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO events (session_id, table_id, name, payload, ts)
		VALUES ($1::uuid, $2, $3, $4::jsonb, $5::timestamptz)`,
		sid, tableID, name, payloadJSON, ts)
	if err != nil {
		t.Fatalf("seed event %s: %v", name, err)
	}
}

const (
	expSID1 = "11111111-1111-1111-1111-111111111111"
	expSID2 = "22222222-2222-2222-2222-222222222222"
)

// TestAN04_Integration_CSVContract: полный контракт events.md §4 на реальной БД —
// колонки/заголовок, границы полуинтервала (--from включительно, --to день входит,
// следующий день 00:00:00Z — нет), ORDER BY ts,id (тай-брейк по id при равных ts),
// ts RFC 3339 UTC с микросекундами, table_id NULL → пустое поле, payload —
// канонический текст jsonb как в БД, LF/UTF-8/без BOM.
func TestAN04_Integration_CSVContract(t *testing.T) {
	t.Setenv("DATABASE_URL", os.Getenv("M2_IT_DATABASE_URL"))
	_, pool, _ := newIntegrationServer(t)
	tbl := 12
	ts := func(s string) time.Time {
		tt, err := time.Parse(time.RFC3339Nano, s)
		if err != nil {
			t.Fatal(err)
		}
		return tt
	}
	// Порядок вставки = порядок id (BIGSERIAL).
	seedExportEvent(t, pool, expSID1, nil, "page_open", `{}`, ts("2026-11-09T23:59:59.999999Z"))                                                                 // до --from → вне выгрузки
	seedExportEvent(t, pool, expSID1, &tbl, "rec_impression", `{"rec_item_id":366609864022,"context":"dish","rec_source":"expert"}`, ts("2026-11-10T00:00:00Z")) // == from → входит
	seedExportEvent(t, pool, expSID2, nil, "cart_open", `{"lines_count":1,"total":500}`, ts("2026-11-20T12:00:00.123456Z"))
	seedExportEvent(t, pool, expSID2, nil, "menu_view", `{}`, ts("2026-11-20T12:00:00.123456Z"))                              // тот же ts → тай-брейк по id
	seedExportEvent(t, pool, expSID2, nil, "stub_view", `{}`, ts("2026-11-21T00:00:00Z"))                                     // == to+1 → вне выгрузки
	seedExportEvent(t, pool, expSID2, &tbl, "pay_click", `{"lines_count":2,"total":1390}`, ts("2026-11-20T23:59:59.999999Z")) // последний микросекундный момент дня --to → входит

	var buf bytes.Buffer
	if err := runExportEvents([]string{"--from", "2026-11-10", "--to", "2026-11-20"}, &buf); err != nil {
		t.Fatalf("export-events: %v", err)
	}
	out := buf.String()

	if strings.HasPrefix(out, "\xef\xbb\xbf") {
		t.Fatal("BOM must not be present")
	}
	if strings.Contains(out, "\r") {
		t.Fatal("line endings must be LF only")
	}
	if !strings.Contains(out, ",menu_view,\"{}\"\n") {
		t.Fatalf("empty jsonb payload must be quoted:\n%s", out)
	}

	recs, err := csv.NewReader(strings.NewReader(out)).ReadAll()
	if err != nil {
		t.Fatalf("csv parse: %v\n%s", err, out)
	}
	if len(recs) != 5 {
		t.Fatalf("want header + 4 rows, got %d:\n%s", len(recs), out)
	}
	if strings.Join(recs[0], ",") != "ts,session_id,table_id,name,payload" {
		t.Fatalf("header wrong: %q", recs[0])
	}
	// ORDER BY ts, id: 00:00:00Z → 12:00:00.123456Z (cart_open вставлен раньше menu_view) → 23:59:59.999999Z.
	want := [][]string{
		{"2026-11-10T00:00:00.000000Z", expSID1, "12", "rec_impression"},
		{"2026-11-20T12:00:00.123456Z", expSID2, "", "cart_open"},
		{"2026-11-20T12:00:00.123456Z", expSID2, "", "menu_view"},
		{"2026-11-20T23:59:59.999999Z", expSID2, "12", "pay_click"},
	}
	for i, w := range want {
		got := recs[i+1][:4]
		if strings.Join(got, "|") != strings.Join(w, "|") {
			t.Fatalf("row %d:\nwant %q\ngot  %q", i, w, got)
		}
	}

	// payload — канонический текст jsonb как хранится в БД (payload::text).
	for _, row := range recs[1:] {
		var dbPayload string
		err := pool.QueryRow(context.Background(), `
			SELECT payload::text FROM events
			WHERE session_id = $1::uuid AND name = $2 AND ts = $3::timestamptz`,
			row[1], row[3], mustParseRFC3339Micro(t, row[0])).Scan(&dbPayload)
		if err != nil {
			t.Fatalf("select db payload for %s: %v", row[3], err)
		}
		if row[4] != dbPayload {
			t.Fatalf("payload for %s: want db text %q, got %q", row[3], dbPayload, row[4])
		}
	}
}

func mustParseRFC3339Micro(t *testing.T, s string) time.Time {
	t.Helper()
	tt, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse ts %q: %v", s, err)
	}
	return tt
}

// TestAN04_Integration_OpenBoundariesAndEmpty: нет флага — граница открыта;
// каждый флаг работает по отдельности; пустая выборка — только заголовок, exit 0.
func TestAN04_Integration_OpenBoundariesAndEmpty(t *testing.T) {
	t.Setenv("DATABASE_URL", os.Getenv("M2_IT_DATABASE_URL"))
	_, pool, _ := newIntegrationServer(t)
	ts := func(s string) time.Time {
		tt, err := time.Parse(time.RFC3339Nano, s)
		if err != nil {
			t.Fatal(err)
		}
		return tt
	}
	seedExportEvent(t, pool, expSID1, nil, "page_open", `{}`, ts("2026-11-10T10:00:00Z"))
	seedExportEvent(t, pool, expSID2, nil, "menu_view", `{}`, ts("2026-11-15T10:00:00Z"))

	export := func(t *testing.T, args ...string) [][]string {
		t.Helper()
		var buf bytes.Buffer
		if err := runExportEvents(args, &buf); err != nil {
			t.Fatalf("export-events %v: %v", args, err)
		}
		recs, err := csv.NewReader(strings.NewReader(buf.String())).ReadAll()
		if err != nil {
			t.Fatalf("csv parse: %v", err)
		}
		return recs
	}

	if recs := export(t); len(recs) != 3 {
		t.Fatalf("no flags: want header + 2 rows, got %d", len(recs))
	}
	if recs := export(t, "--from", "2026-11-12"); len(recs) != 2 || recs[1][3] != "menu_view" {
		t.Fatalf("--from only: wrong rows: %+v", recs)
	}
	if recs := export(t, "--to", "2026-11-12"); len(recs) != 2 || recs[1][3] != "page_open" {
		t.Fatalf("--to only (inclusive day): wrong rows: %+v", recs)
	}
	empty := export(t, "--from", "2020-01-01", "--to", "2020-01-02")
	if len(empty) != 1 {
		t.Fatalf("empty selection: want header only, got %+v", empty)
	}
	if empty[0][0] != "ts" || empty[0][4] != "payload" {
		t.Fatalf("empty selection header wrong: %+v", empty[0])
	}
}
