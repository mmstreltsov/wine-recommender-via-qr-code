package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/tracelog"

	"svoyshef/internal/store"
	"svoyshef/plugins/aidabaran"
)

const itTTL = 300 * time.Millisecond

type itClient struct {
	mu   sync.Mutex
	jar  map[string]string
	http *http.Client
}

func newItClient() *itClient {
	return &itClient{jar: map[string]string{}, http: &http.Client{}}
}

func (c *itClient) sessionID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.jar["session"]
}

func (c *itClient) do(req *http.Request) (*http.Response, error) {
	c.mu.Lock()
	if v, ok := c.jar["session"]; ok {
		req.AddCookie(&http.Cookie{Name: "session", Value: v})
	}
	c.mu.Unlock()
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	for _, ck := range resp.Cookies() {
		if ck.Name == "session" {
			c.jar["session"] = ck.Value
		}
	}
	c.mu.Unlock()
	return resp, nil
}

func (c *itClient) get(t *testing.T, url string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func (c *itClient) post(t *testing.T, url, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func (c *itClient) patch(t *testing.T, url, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPatch, url, bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func (c *itClient) delete(t *testing.T, url string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func decodeBody(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decode body: %v", err)
	}
}

type itMenuView struct {
	Categories []struct {
		ID    int    `json:"id"`
		Name  string `json:"name"`
		Items []struct {
			ID            int64  `json:"id"`
			CategoryID    int    `json:"category_id"`
			Name          string `json:"name"`
			Price         int    `json:"price"`
			PhotoURL      string `json:"photo_url"`
			PhotoFallback string `json:"photo_fallback"`
		} `json:"items"`
	} `json:"categories"`
}

func (m itMenuView) totalItems() int {
	total := 0
	for _, c := range m.Categories {
		total += len(c.Items)
	}
	return total
}

func (m itMenuView) hasItem(itemID int64) bool {
	for _, c := range m.Categories {
		for _, it := range c.Items {
			if it.ID == itemID {
				return true
			}
		}
	}
	return false
}

func newIntegrationServer(t *testing.T) (*httptest.Server, *pgxpool.Pool, *aidabaran.LoadResult) {
	t.Helper()
	dsn := os.Getenv("M2_IT_DATABASE_URL")
	if dsn == "" {
		t.Skip("M2_IT_DATABASE_URL не задан: интеграционные сценарии требуют docker compose up -d db")
	}
	ctx := context.Background()
	cfg := config{
		Port:            "0",
		DatabaseURL:     dsn,
		Restaurant:      "aidabaran",
		DataDir:         "../../../data",
		CookieMaxAgeH:   12,
		AvailabilityTTL: itTTL,
	}
	pool, err := newPool(ctx, dsn)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	if os.Getenv("M2_IT_TRACE") != "" {
		cfgp, perr := pgxpool.ParseConfig(dsn)
		if perr != nil {
			t.Fatal(perr)
		}
		cfgp.MinConns = 1
		cfgp.MaxConns = 1
		cfgp.ConnConfig.Tracer = &tracelog.TraceLog{
			Logger: tracelog.LoggerFunc(func(ctx context.Context, level tracelog.LogLevel, msg string, data map[string]any) {
				fmt.Fprintf(os.Stderr, "PGX %s %s %v\n", level, msg, data)
			}),
			LogLevel: tracelog.LogLevelDebug,
		}
		tp, terr := pgxpool.NewWithConfig(ctx, cfgp)
		if terr != nil {
			t.Fatal(terr)
		}
		pool.Close()
		pool = tp
	}
	t.Cleanup(pool.Close)
	if err := runMigrations(ctx, dsn); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	if _, err := pool.Exec(ctx, `TRUNCATE cart_items, carts, events, item_availability, sessions`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	loadResult, err := aidabaran.LoadMenu(cfg.DataDir, cfg.Restaurant)
	if err != nil {
		t.Fatalf("load menu: %v", err)
	}
	upsellItem, err := aidabaran.LoadUpsellMap(cfg.DataDir, cfg.Restaurant)
	if err != nil {
		t.Fatalf("load upsell map: %v", err)
	}
	rec, err := aidabaran.LoadRecommender(cfg.DataDir, cfg.Restaurant)
	if err != nil {
		t.Fatalf("load recommender: %v", err)
	}
	handler, _ := buildHandler(cfg, pool, loadResult, upsellItem, rec)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv, pool, loadResult
}

func waitUntil(t *testing.T, timeout time.Duration, fn func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timeout waiting: %s", what)
}

func itEventRows(t *testing.T, pool *pgxpool.Pool, sessionID string) []struct {
	Name    string
	Payload map[string]any
} {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
		SELECT name, payload::text FROM events
		WHERE session_id = $1::uuid
		ORDER BY id`, sessionID)
	if err != nil {
		t.Fatalf("select events: %v", err)
	}
	defer rows.Close()
	out := make([]struct {
		Name    string
		Payload map[string]any
	}, 0)
	for rows.Next() {
		var name, raw string
		if err := rows.Scan(&name, &raw); err != nil {
			t.Fatalf("scan event: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			t.Fatalf("parse payload %q: %v", raw, err)
		}
		out = append(out, struct {
			Name    string
			Payload map[string]any
		}{name, payload})
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("events rows: %v", err)
	}
	return out
}

func number(t *testing.T, v any) float64 {
	t.Helper()
	f, ok := v.(float64)
	if !ok {
		t.Fatalf("expected number, got %T (%v)", v, v)
	}
	return f
}

func TestFR33_Integration_CookieAttributes(t *testing.T) {
	srv, pool, _ := newIntegrationServer(t)
	resp := func() *http.Response {
		c := newItClient()
		return c.get(t, srv.URL+"/api/v1/menu")
	}()
	defer func() { _ = resp.Body.Close() }()
	cookies := resp.Cookies()
	if len(cookies) != 1 || cookies[0].Name != "session" {
		t.Fatalf("expected one session cookie, got %v", cookies)
	}
	c := cookies[0]
	if !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != "/" || c.MaxAge != 12*3600 || c.Secure {
		t.Fatalf("cookie attrs wrong: %+v", c)
	}
	var tableID *int
	if err := pool.QueryRow(context.Background(), `SELECT table_id FROM sessions WHERE id = $1::uuid`, c.Value).Scan(&tableID); err != nil {
		t.Fatalf("session row missing: %v", err)
	}
	if tableID != nil {
		t.Fatalf("table_id must be NULL without ?table, got %v", *tableID)
	}
}

func TestFR34_Integration_TableLifecycle(t *testing.T) {
	srv, pool, _ := newIntegrationServer(t)
	c := newItClient()
	resp := c.get(t, srv.URL+"/api/v1/menu?table=12")
	_ = resp.Body.Close()
	sid := c.sessionID()
	var tableID *int
	if err := pool.QueryRow(context.Background(), `SELECT table_id FROM sessions WHERE id = $1::uuid`, sid).Scan(&tableID); err != nil {
		t.Fatalf("session row: %v", err)
	}
	if tableID == nil || *tableID != 12 {
		t.Fatalf("table_id must be 12, got %v", tableID)
	}
	resp2 := c.get(t, srv.URL+"/api/v1/menu?table=99")
	_ = resp2.Body.Close()
	if err := pool.QueryRow(context.Background(), `SELECT table_id FROM sessions WHERE id = $1::uuid`, sid).Scan(&tableID); err != nil {
		t.Fatalf("session row: %v", err)
	}
	if tableID == nil || *tableID != 12 {
		t.Fatalf("repeat scan must not change table, got %v", tableID)
	}
	var count int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM sessions`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 session, got %d", count)
	}
	c2 := newItClient()
	resp3 := c2.get(t, srv.URL+"/api/v1/menu?table=abc")
	_ = resp3.Body.Close()
	if err := pool.QueryRow(context.Background(), `SELECT table_id FROM sessions WHERE id = $1::uuid`, c2.sessionID()).Scan(&tableID); err != nil {
		t.Fatal(err)
	}
	if tableID != nil {
		t.Fatalf("invalid table must give NULL, got %v", *tableID)
	}
}

func TestFR37_Integration_MenuFull14x125(t *testing.T) {
	srv, _, _ := newIntegrationServer(t)
	c := newItClient()
	resp := c.get(t, srv.URL+"/api/v1/menu")
	var view itMenuView
	decodeBody(t, resp, &view)
	if len(view.Categories) != 14 {
		t.Fatalf("expected 14 categories, got %d", len(view.Categories))
	}
	if total := view.totalItems(); total != 125 {
		t.Fatalf("expected 125 items, got %d", total)
	}
	for _, cat := range view.Categories {
		if cat.Name == "" || cat.ID < 1 {
			t.Fatalf("bad category: %+v", cat)
		}
		for _, it := range cat.Items {
			if it.CategoryID != cat.ID || it.Name == "" || it.Price < 0 {
				t.Fatalf("bad item in category %d: %+v", cat.ID, it)
			}
		}
	}
}

func TestAN02_Integration_CartCycleServerEvents(t *testing.T) {
	srv, pool, loaded := newIntegrationServer(t)
	x := loaded.Items[0].ID
	y := loaded.Items[1].ID
	z := loaded.Items[2].ID
	c := newItClient()
	resp := c.get(t, srv.URL+"/api/v1/menu?table=12")
	_ = resp.Body.Close()

	resp = c.post(t, srv.URL+"/api/v1/cart/items", fmt.Sprintf(`{"item_id":%d,"source":"menu"}`, x))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("add x: %d", resp.StatusCode)
	}
	var cart struct {
		Lines []struct {
			Item struct {
				ID int64 `json:"id"`
			} `json:"item"`
			Qty       int    `json:"qty"`
			AddedFrom string `json:"added_from"`
			Available bool   `json:"available"`
		} `json:"lines"`
		Total          int `json:"total"`
		Recommendation any `json:"recommendation"`
	}
	decodeBody(t, resp, &cart)
	if len(cart.Lines) != 1 || cart.Lines[0].Item.ID != x || cart.Lines[0].Qty != 1 || !cart.Lines[0].Available {
		t.Fatalf("cart after add x wrong: %+v", cart)
	}

	resp = c.post(t, srv.URL+"/api/v1/cart/items", fmt.Sprintf(`{"item_id":%d,"source":"rec_feed"}`, y))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("add y: %d", resp.StatusCode)
	}
	_ = resp.Body.Close()

	resp = c.post(t, srv.URL+"/api/v1/cart/items", fmt.Sprintf(`{"item_id":%d,"source":"rec_cart"}`, z))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("add z: %d", resp.StatusCode)
	}
	_ = resp.Body.Close()

	resp = c.patch(t, srv.URL+fmt.Sprintf("/api/v1/cart/items/%d", x), `{"qty":3}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("patch x: %d", resp.StatusCode)
	}
	_ = resp.Body.Close()

	resp = c.delete(t, srv.URL+fmt.Sprintf("/api/v1/cart/items/%d", y))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete y: %d", resp.StatusCode)
	}
	_ = resp.Body.Close()

	rows := itEventRows(t, pool, c.sessionID())
	wantNames := []string{"menu_add_click", "rec_add_click", "rec_add_click", "cart_qty_change", "cart_item_remove"}
	if len(rows) != len(wantNames) {
		t.Fatalf("expected %d events, got %d: %+v", len(wantNames), len(rows), rows)
	}
	for i, want := range wantNames {
		if rows[i].Name != want {
			t.Fatalf("event %d: want %s, got %s", i, want, rows[i].Name)
		}
	}
	if got := number(t, rows[0].Payload["item_id"]); int64(got) != x {
		t.Fatalf("menu_add_click item_id: got %v", rows[0].Payload["item_id"])
	}
	if got := number(t, rows[1].Payload["item_id"]); int64(got) != y {
		t.Fatalf("rec_add_click(dish) item_id: got %v", rows[1].Payload["item_id"])
	}
	if rows[1].Payload["context"] != "dish" {
		t.Fatalf("rec_add_click context: got %v", rows[1].Payload["context"])
	}
	if rows[2].Payload["context"] != "cart" {
		t.Fatalf("rec_add_click context: got %v", rows[2].Payload["context"])
	}
	if got := number(t, rows[3].Payload["from"]); got != 1 {
		t.Fatalf("cart_qty_change from: got %v", rows[3].Payload["from"])
	}
	if got := number(t, rows[3].Payload["to"]); got != 3 {
		t.Fatalf("cart_qty_change to: got %v", rows[3].Payload["to"])
	}
	if got := number(t, rows[4].Payload["item_id"]); int64(got) != y {
		t.Fatalf("cart_item_remove item_id: got %v", rows[4].Payload["item_id"])
	}

	var tableID *int
	if err := pool.QueryRow(context.Background(), `SELECT DISTINCT table_id FROM events WHERE session_id = $1::uuid`, c.sessionID()).Scan(&tableID); err != nil {
		t.Fatalf("events table_id: %v", err)
	}
	if tableID == nil || *tableID != 12 {
		t.Fatalf("events must carry table_id=12, got %v", tableID)
	}
}

func TestAN03_Integration_RecAddClickContextAndSession(t *testing.T) {
	srv, pool, loaded := newIntegrationServer(t)
	x := loaded.Items[0].ID
	c := newItClient()
	resp := c.get(t, srv.URL+"/api/v1/menu?table=7")
	_ = resp.Body.Close()
	resp = c.post(t, srv.URL+"/api/v1/cart/items", fmt.Sprintf(`{"item_id":%d,"source":"rec_feed"}`, x))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("add: %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
	rows := itEventRows(t, pool, c.sessionID())
	if len(rows) != 1 || rows[0].Name != "rec_add_click" {
		t.Fatalf("expected single rec_add_click, got %+v", rows)
	}
	if rows[0].Payload["context"] != "dish" {
		t.Fatalf("context must be dish, got %v", rows[0].Payload["context"])
	}
	if got := number(t, rows[0].Payload["item_id"]); int64(got) != x {
		t.Fatalf("item_id: got %v", rows[0].Payload["item_id"])
	}
}

func TestFR36_Integration_AvailabilityTTLCycle(t *testing.T) {
	srv, pool, loaded := newIntegrationServer(t)
	x := loaded.Items[0].ID
	availability := store.NewAvailabilityStore(pool)
	c := newItClient()
	resp := c.get(t, srv.URL+"/api/v1/menu")
	_ = resp.Body.Close()

	resp = c.post(t, srv.URL+"/api/v1/cart/items", fmt.Sprintf(`{"item_id":%d,"source":"menu"}`, x))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("add while available: %d", resp.StatusCode)
	}
	_ = resp.Body.Close()

	if err := availability.Upsert(context.Background(), x, false); err != nil {
		t.Fatal(err)
	}
	menuHas := func() bool {
		r := c.get(t, srv.URL+"/api/v1/menu")
		var view itMenuView
		decodeBody(t, r, &view)
		return view.hasItem(x)
	}
	waitUntil(t, 3*time.Second, func() bool { return !menuHas() }, "item disappears from /menu within TTL")

	resp = c.get(t, srv.URL+fmt.Sprintf("/api/v1/menu/items/%d", x))
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("card of unavailable must 404, got %d", resp.StatusCode)
	}

	resp = c.post(t, srv.URL+"/api/v1/cart/items", fmt.Sprintf(`{"item_id":%d,"source":"menu"}`, x))
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("add unavailable must 409, got %d", resp.StatusCode)
	}
	var errBody struct {
		Error struct {
			Code   string `json:"code"`
			ItemID *int64 `json:"item_id"`
		} `json:"error"`
	}
	decodeBody(t, resp, &errBody)
	if errBody.Error.Code != "item_unavailable" || errBody.Error.ItemID == nil || *errBody.Error.ItemID != x {
		t.Fatalf("409 body wrong: %+v", errBody)
	}

	resp = c.get(t, srv.URL+"/api/v1/cart")
	var cart struct {
		Lines []struct {
			Item struct {
				ID int64 `json:"id"`
			} `json:"item"`
			Available bool `json:"available"`
		} `json:"lines"`
	}
	decodeBody(t, resp, &cart)
	if len(cart.Lines) != 1 || cart.Lines[0].Item.ID != x || cart.Lines[0].Available {
		t.Fatalf("line must stay with available=false: %+v", cart.Lines)
	}

	resp = c.patch(t, srv.URL+fmt.Sprintf("/api/v1/cart/items/%d", x), `{"qty":2}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("patch unavailable line must work, got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()

	if err := availability.Upsert(context.Background(), x, true); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, 3*time.Second, menuHas, "item returns to /menu within TTL after set on")

	resp = c.delete(t, srv.URL+fmt.Sprintf("/api/v1/cart/items/%d", x))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete line: %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
}

func TestNFR04_Integration_Events204And400(t *testing.T) {
	srv, pool, _ := newIntegrationServer(t)
	for _, headers := range []map[string]string{
		{"Content-Type": "text/plain"},
		{"Content-Type": "application/json", "Origin": "null"},
		{"Content-Type": "application/json", "Origin": "http://other.test"},
		{"Content-Type": "application/json", "Sec-Fetch-Site": "same-site"},
	} {
		req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/events", bytes.NewBufferString(`{"name":"page_open","payload":{}}`))
		if err != nil {
			t.Fatal(err)
		}
		for key, value := range headers {
			req.Header.Set(key, value)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest || len(resp.Cookies()) != 0 {
			t.Fatal("unsafe event write must be rejected before session creation")
		}
	}
	var sessions int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM sessions`).Scan(&sessions); err != nil || sessions != 0 {
		t.Fatalf("rejected writes created sessions: count=%d err=%v", sessions, err)
	}
	c := newItClient()
	resp := c.get(t, srv.URL+"/api/v1/menu?table=3")
	_ = resp.Body.Close()
	ok := func(body string) {
		req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/events", bytes.NewBufferString(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", srv.URL)
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		r, err := c.do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = r.Body.Close()
		if r.StatusCode != http.StatusNoContent {
			t.Fatalf("POST /events %s: want 204, got %d", body, r.StatusCode)
		}
	}
	ok(`{"name":"page_open","payload":{}}`)
	ok(`{"name":"rec_impression","payload":{"context":"dish","rec_item_id":366609864022,"rec_source":"expert"}}`)
	ok(`{"name":"rec_impression","payload":{"context":"cart","rec_item_id":999999,"rec_source":"heuristic"}}`)
	bad := func(body string) {
		r := c.post(t, srv.URL+"/api/v1/events", body)
		_ = r.Body.Close()
		if r.StatusCode != http.StatusBadRequest {
			t.Fatalf("POST /events %s: want 400, got %d", body, r.StatusCode)
		}
	}
	bad(`{"name":"nope","payload":{}}`)
	bad(`{"name":"menu_view","payload":{"x":{"y":1}}}`)
	bad(`{"name":`)
	bad(`{"name":"page_open","payload":{"note":"synthetic free text"}}`)
	bad(`{"name":"category_select","payload":{"category_id":"text"}}`)
	bad(`{"name":"rec_impression","payload":{"context":"dish","rec_item_id":999999}}`)
	bad(`{"name":"rec_impression","payload":{"context":"dish","rec_item_id":999999,"rec_source":"arbitrary"}}`)
	bad(`{"name":"rec_feed_visible","payload":{"dish_id":999999,"recs_count":14}}`)
	bad(`{"name":"menu_view","payload":{}} {"name":"stub_view","payload":{}}`)
	// events.md §1: серверные имена от клиента → 400 (ровно один источник).
	bad(`{"name":"menu_add_click","payload":{"item_id":1}}`)
	bad(`{"name":"rec_add_click","payload":{"item_id":1,"context":"dish"}}`)
	bad(`{"name":"cart_qty_change","payload":{"item_id":1,"from":1,"to":2}}`)
	bad(`{"name":"cart_item_remove","payload":{"item_id":1}}`)

	rows := itEventRows(t, pool, c.sessionID())
	if len(rows) != 3 {
		t.Fatalf("expected 3 stored events, got %d", len(rows))
	}
	for _, r := range rows {
		if r.Name != "page_open" && r.Name != "rec_impression" {
			t.Fatalf("unexpected stored name: %s", r.Name)
		}
	}
}

func TestNFR04_Integration_EventsRateLimit429(t *testing.T) {
	srv, _, _ := newIntegrationServer(t)
	c := newItClient()
	resp := c.get(t, srv.URL+"/api/v1/menu")
	_ = resp.Body.Close()
	body := `{"name":"menu_view","payload":{}}`
	for i := 1; i <= 120; i++ {
		r := c.post(t, srv.URL+"/api/v1/events", body)
		_ = r.Body.Close()
		if r.StatusCode != http.StatusNoContent {
			t.Fatalf("event %d: want 204, got %d", i, r.StatusCode)
		}
	}
	r := c.post(t, srv.URL+"/api/v1/events", body)
	_ = r.Body.Close()
	if r.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("event 121: want 429, got %d", r.StatusCode)
	}
}

// TestFR32_Integration_QtyLimit20OnAdd: реальный лимит qty=20 в SQL-апсерте —
// 20×POST одной позиции → 201, 21-й POST → 400 bad_request.
func TestFR32_Integration_QtyLimit20OnAdd(t *testing.T) {
	srv, _, loaded := newIntegrationServer(t)
	x := loaded.Items[0].ID
	c := newItClient()
	resp := c.get(t, srv.URL+"/api/v1/menu")
	_ = resp.Body.Close()
	for i := 1; i <= 20; i++ {
		r := c.post(t, srv.URL+"/api/v1/cart/items", fmt.Sprintf(`{"item_id":%d,"source":"menu"}`, x))
		if r.StatusCode != http.StatusCreated {
			t.Fatalf("POST %d/20: want 201, got %d", i, r.StatusCode)
		}
		_ = r.Body.Close()
	}
	r := c.post(t, srv.URL+"/api/v1/cart/items", fmt.Sprintf(`{"item_id":%d,"source":"menu"}`, x))
	_ = r.Body.Close()
	if r.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST 21: want 400, got %d", r.StatusCode)
	}
	cart := itGetCart(t, c, srv)
	if len(cart.Lines) != 1 || cart.Lines[0].Qty != 20 {
		t.Fatalf("после 21-го POST qty должен остаться 20: %+v", cart.Lines)
	}
}

// TestFR32_Integration_PatchQtyBounds: PATCH qty=0 и qty=21 на HTTP-уровне → 400.
func TestFR32_Integration_PatchQtyBounds(t *testing.T) {
	srv, _, loaded := newIntegrationServer(t)
	x := loaded.Items[0].ID
	c := newItClient()
	resp := c.get(t, srv.URL+"/api/v1/menu")
	_ = resp.Body.Close()
	itAddToCart(t, c, srv, x, "menu")
	for _, body := range []string{`{"qty":0}`, `{"qty":21}`, `{"qty":-1}`} {
		r := c.patch(t, srv.URL+fmt.Sprintf("/api/v1/cart/items/%d", x), body)
		_ = r.Body.Close()
		if r.StatusCode != http.StatusBadRequest {
			t.Fatalf("PATCH %s: want 400, got %d", body, r.StatusCode)
		}
	}
	r := c.patch(t, srv.URL+fmt.Sprintf("/api/v1/cart/items/%d", x), `{"qty":5}`)
	if r.StatusCode != http.StatusOK {
		t.Fatalf("PATCH валидного qty: want 200, got %d", r.StatusCode)
	}
	_ = r.Body.Close()
	cart := itGetCart(t, c, srv)
	if len(cart.Lines) != 1 || cart.Lines[0].Qty != 5 {
		t.Fatalf("после валидного PATCH qty должен быть 5: %+v", cart.Lines)
	}
}

const (
	itKhinkaliID   = int64(655892482392) // Хинкали говядина-свинина 3 шт (Хинкали)
	itTkemaliID    = int64(231821651422) // Ткемали (Соусы), expert-пара хинкали, score 0.9
	itSeasonVegID  = int64(293109021772) // Сезонные овощи с ароматной зеленью (Холодные закуски), expert 0.7
	itGrillVegID   = int64(733465955822) // Овощи на гриле (Овощи на углях и гарниры), expert 0.7
	itTarrhunID    = int64(875106956773) // Лимонад Тархун с грушей (Напитки), expert 0.65
	itDolmaID      = int64(164834011242) // Традиционная долма (Горячие закуски), expert 0.6
	itBigSaladID   = int64(876201943922) // Большой овощной салат (Салаты), heuristic-топ от хинкали
	itMedovikID    = int64(939802252842) // Медовик (Десерты)
	itCompoteID    = int64(923360279353) // Компот сезонный (Напитки), expert-пара медовика
	itCreamSauceID = int64(397344866022) // Сливочно-пряный (Соусы), heuristic-топ Соусов

	khinkaliCategoryID = 5  // «Хинкали»
	saucesCategoryID   = 11 // «Соусы»
)

type itRecView struct {
	Item struct {
		ID         int64  `json:"id"`
		CategoryID int    `json:"category_id"`
		Name       string `json:"name"`
		Price      int    `json:"price"`
	} `json:"item"`
	ReasonText string `json:"reason_text"`
	Source     string `json:"source"`
}

type itCartView struct {
	Lines []struct {
		Item struct {
			ID int64 `json:"id"`
		} `json:"item"`
		Qty       int    `json:"qty"`
		AddedFrom string `json:"added_from"`
		Available bool   `json:"available"`
	} `json:"lines"`
	Total          int `json:"total"`
	Recommendation *struct {
		Item struct {
			ID         int64  `json:"id"`
			CategoryID int    `json:"category_id"`
			Name       string `json:"name"`
		} `json:"item"`
		ReasonText string `json:"reason_text"`
		Source     string `json:"source"`
	} `json:"recommendation"`
}

func itGetRecs(t *testing.T, c *itClient, srv *httptest.Server, itemID int64) []itRecView {
	t.Helper()
	resp := c.get(t, srv.URL+fmt.Sprintf("/api/v1/menu/items/%d/recommendations", itemID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET recommendations %d: want 200, got %d", itemID, resp.StatusCode)
	}
	var v struct {
		Recs []itRecView `json:"recs"`
	}
	decodeBody(t, resp, &v)
	return v.Recs
}

func itGetCart(t *testing.T, c *itClient, srv *httptest.Server) itCartView {
	t.Helper()
	resp := c.get(t, srv.URL+"/api/v1/cart")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET cart: want 200, got %d", resp.StatusCode)
	}
	var v itCartView
	decodeBody(t, resp, &v)
	return v
}

func itAddToCart(t *testing.T, c *itClient, srv *httptest.Server, itemID int64, source string) {
	t.Helper()
	resp := c.post(t, srv.URL+"/api/v1/cart/items", fmt.Sprintf(`{"item_id":%d,"source":%q}`, itemID, source))
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST cart item %d (%s): want 201, got %d", itemID, source, resp.StatusCode)
	}
}

func recsContain(recs []itRecView, itemID int64) bool {
	for _, r := range recs {
		if r.Item.ID == itemID {
			return true
		}
	}
	return false
}

// TestFR10_Integration_RecFeedRealEngine: реальный движок (M3) в ленте карточки (FR-10/FR-11/FR-28).
func TestFR10_Integration_RecFeedRealEngine(t *testing.T) {
	srv, _, _ := newIntegrationServer(t)
	c := newItClient()
	resp := c.get(t, srv.URL+"/api/v1/menu")
	_ = resp.Body.Close()

	recs := itGetRecs(t, c, srv, itKhinkaliID)
	// Фактическая лента хинкали по данным M1 (детерминизм; порядок score DESC → catID ASC):
	// Ткемали (expert 0.9), Сезонные овощи (expert 0.7), Овощи на гриле (expert 0.7),
	// Тархун (expert 0.65), Долма (expert 0.6), Большой овощной салат (heuristic 0.4).
	// Остальные категории-кандидаты без пары пропускаются.
	want := []struct {
		id     int64
		source string
	}{
		{itTkemaliID, "expert"},
		{itSeasonVegID, "expert"},
		{itGrillVegID, "expert"},
		{itTarrhunID, "expert"},
		{itDolmaID, "expert"},
		{itBigSaladID, "heuristic"},
	}
	if len(recs) != len(want) {
		t.Fatalf("лента хинкали: want %d позиций, got %d: %+v", len(want), len(recs), recs)
	}
	cats := make(map[int]bool)
	for i, r := range recs {
		if r.Item.ID != want[i].id || r.Source != want[i].source {
			t.Fatalf("rec %d: want {%d %s}, got {%d %s}", i, want[i].id, want[i].source, r.Item.ID, r.Source)
		}
		if r.Item.CategoryID == khinkaliCategoryID {
			t.Fatalf("позиция из категории самого блюда в ленте: %+v", r)
		}
		if r.ReasonText == "" || len(r.ReasonText) < 60 {
			t.Fatalf("FR-11: пустой/короткий reason_text у %d: %q", r.Item.ID, r.ReasonText)
		}
		cats[r.Item.CategoryID] = true
	}
	if len(cats) != len(recs) {
		t.Fatalf("в ленте должна быть ровно одна позиция на категорию: %+v", recs)
	}

	// FR-28: позиция, добавленная в корзину, исчезает из ленты.
	itAddToCart(t, c, srv, itTarrhunID, "rec_feed")
	recs2 := itGetRecs(t, c, srv, itKhinkaliID)
	if recsContain(recs2, itTarrhunID) {
		t.Fatalf("FR-28: позиция из корзины осталась в ленте: %+v", recs2)
	}
	if len(recs2) != len(recs)-1 {
		t.Fatalf("после добавления в корзину want %d позиций, got %d", len(recs)-1, len(recs2))
	}
}

// TestFR19_Integration_CartRecommendationRealEngine: ForCart реального движка (FR-19/20/21).
func TestFR19_Integration_CartRecommendationRealEngine(t *testing.T) {
	srv, _, _ := newIntegrationServer(t)
	c := newItClient()
	resp := c.get(t, srv.URL+"/api/v1/menu?table=5")
	_ = resp.Body.Close()

	if cart := itGetCart(t, c, srv); cart.Recommendation != nil {
		t.Fatalf("пустая корзина: recommendation want null, got %+v", cart.Recommendation)
	}

	// Медовик (Десерты) + хинкали: первый непокрытый курс — «Напитки»,
	// последняя позиция — хинкали → expert-пара Тархун (FR-21: категории «Напитки» в корзине нет).
	itAddToCart(t, c, srv, itMedovikID, "menu")
	itAddToCart(t, c, srv, itKhinkaliID, "menu")
	cart := itGetCart(t, c, srv)
	if cart.Recommendation == nil {
		t.Fatal("FR-19: recommendation = null после добавления двух позиций")
	}
	if cart.Recommendation.Item.ID != itTarrhunID || cart.Recommendation.Source != "expert" {
		t.Fatalf("recommendation: want Тархун/expert, got %+v", cart.Recommendation)
	}
	if cart.Recommendation.ReasonText == "" {
		t.Fatal("FR-11: recommendation без reason_text")
	}

	// FR-20: состав изменился (последняя позиция — медовик) → рекомендация сменилась
	// на expert-пару медовика (Компот, Напитки).
	resp = c.delete(t, srv.URL+fmt.Sprintf("/api/v1/cart/items/%d", itKhinkaliID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete хинкали: %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
	cart = itGetCart(t, c, srv)
	if cart.Recommendation == nil || cart.Recommendation.Item.ID != itCompoteID {
		t.Fatalf("FR-20: после удаления want рекомендация Компот, got %+v", cart.Recommendation)
	}
}

// TestFR37_Integration_RecPostFilter: пост-фильтр доступности рекомендаций (FR-37 п.8, FR-29) и 409 (FR-37 п.4).
func TestFR37_Integration_RecPostFilter(t *testing.T) {
	srv, pool, _ := newIntegrationServer(t)
	availability := store.NewAvailabilityStore(pool)
	c := newItClient()
	resp := c.get(t, srv.URL+"/api/v1/menu")
	_ = resp.Body.Close()

	recs := itGetRecs(t, c, srv, itKhinkaliID)
	if !recsContain(recs, itTkemaliID) {
		t.Fatalf("исходная лента должна содержать Ткемали: %+v", recs)
	}

	// (c) лента: off топ-1 (Ткемали, «Соусы») → позиция исчезает из ленты в пределах TTL+2с,
	// замена по каскаду НЕ ищется: категория «Соусы» исключается целиком, heuristic-топ
	// «Сливочно-пряный» не появляется.
	if err := availability.Upsert(context.Background(), itTkemaliID, false); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, itTTL+2*time.Second, func() bool {
		return !recsContain(itGetRecs(t, c, srv, itKhinkaliID), itTkemaliID)
	}, "Ткемали исчезает из /recommendations в пределах TTL+2с")
	filtered := itGetRecs(t, c, srv, itKhinkaliID)
	if len(filtered) != 5 {
		t.Fatalf("после off Ткемали want 5 позиций, got %d: %+v", len(filtered), filtered)
	}
	for _, r := range filtered {
		if r.Item.CategoryID == saucesCategoryID || r.Item.ID == itCreamSauceID {
			t.Fatalf("замена недоступного кандидата не ищется, категория исключена целиком: %+v", r)
		}
	}

	// (c) корзина: [хинкали] → ForCart даёт Тархун (единственный кандидат группы «Напитки»:
	// expert-пара; upsell-маппинг пуст; heuristic «Хинкали→Напитки» отсутствует).
	itAddToCart(t, c, srv, itKhinkaliID, "menu")
	if cart := itGetCart(t, c, srv); cart.Recommendation == nil || cart.Recommendation.Item.ID != itTarrhunID {
		t.Fatalf("ForCart для [хинкали]: want Тархун, got %+v", cart.Recommendation)
	}
	if err := availability.Upsert(context.Background(), itTarrhunID, false); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, itTTL+2*time.Second, func() bool {
		return itGetCart(t, c, srv).Recommendation == nil
	}, "recommendation → null при недоступности единственного кандидата")

	// FR-29: off оставшихся кандидатов ленты → лента опустела → recs=[] (блок скрыт).
	if err := availability.Upsert(context.Background(), itSeasonVegID, false); err != nil {
		t.Fatal(err)
	}
	if err := availability.Upsert(context.Background(), itGrillVegID, false); err != nil {
		t.Fatal(err)
	}
	if err := availability.Upsert(context.Background(), itDolmaID, false); err != nil {
		t.Fatal(err)
	}
	if err := availability.Upsert(context.Background(), itBigSaladID, false); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, itTTL+2*time.Second, func() bool {
		return len(itGetRecs(t, c, srv, itKhinkaliID)) == 0
	}, "лента рекомендаций опустела → recs=[]")

	// (d) POST недоступной позиции → 409 item_unavailable (по-прежнему).
	resp = c.post(t, srv.URL+"/api/v1/cart/items", fmt.Sprintf(`{"item_id":%d,"source":"menu"}`, itTarrhunID))
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("POST недоступной позиции: want 409, got %d", resp.StatusCode)
	}
	var errBody struct {
		Error struct {
			Code   string `json:"code"`
			ItemID *int64 `json:"item_id"`
		} `json:"error"`
	}
	decodeBody(t, resp, &errBody)
	if errBody.Error.Code != "item_unavailable" || errBody.Error.ItemID == nil || *errBody.Error.ItemID != itTarrhunID {
		t.Fatalf("409 body wrong: %+v", errBody)
	}
}
