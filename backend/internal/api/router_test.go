package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"svoyshef/internal/app"
	"svoyshef/internal/domain"
	"svoyshef/internal/store"
)

type memCartStore struct {
	lines []domain.CartLine
}

func (s *memCartStore) Lines(ctx context.Context, sessionID string) ([]domain.CartLine, error) {
	out := make([]domain.CartLine, len(s.lines))
	copy(out, s.lines)
	return out, nil
}

func (s *memCartStore) AddItem(ctx context.Context, sessionID string, itemID int64, addedFrom string) (int, error) {
	for i := range s.lines {
		if s.lines[i].ItemID == itemID {
			if s.lines[i].Qty >= 20 {
				return 0, store.ErrQtyLimit
			}
			s.lines[i].Qty++
			return s.lines[i].Qty, nil
		}
	}
	s.lines = append(s.lines, domain.CartLine{ItemID: itemID, Qty: 1, AddedFrom: addedFrom})
	return 1, nil
}

func (s *memCartStore) SetQty(ctx context.Context, sessionID string, itemID int64, qty int) (bool, int, error) {
	for i := range s.lines {
		if s.lines[i].ItemID == itemID {
			prev := s.lines[i].Qty
			s.lines[i].Qty = qty
			return true, prev, nil
		}
	}
	return false, 0, nil
}

func (s *memCartStore) RemoveItem(ctx context.Context, sessionID string, itemID int64) (bool, error) {
	for i := range s.lines {
		if s.lines[i].ItemID == itemID {
			s.lines = append(s.lines[:i], s.lines[i+1:]...)
			return true, nil
		}
	}
	return false, nil
}

type staticAvail struct{}

func (staticAvail) IsAvailable(ctx context.Context, itemID int64) bool { return true }

func (staticAvail) UnavailableIDs(ctx context.Context) map[int64]struct{} {
	return map[int64]struct{}{}
}

type apiFakeEventStore struct {
	events []domain.Event
}

func (f *apiFakeEventStore) Insert(ctx context.Context, e domain.Event) error {
	f.events = append(f.events, e)
	return nil
}

func (f *apiFakeEventStore) InsertBatch(ctx context.Context, events []domain.Event) error {
	f.events = append(f.events, events...)
	return nil
}

func fullDeps(t *testing.T, sessions *fakeSessions) (RouterDeps, *memCartStore, *app.EventsService) {
	t.Helper()
	cats := []domain.Category{{ID: 1, Name: "Кат"}}
	items := []domain.MenuItem{
		{ID: 11, CategoryID: 1, Name: "Блюдо", Price: 250, SortOrder: 0},
	}
	menu := app.NewMenuService(app.NewStaticMenu(cats, items), staticAvail{}, nil)
	carts := &memCartStore{}
	eventsService := app.NewEventsService(&apiFakeEventStore{})
	cart := app.NewCartService(menu, carts, eventsService, app.StubRecommender{}, staticAvail{})
	recs := app.NewRecommendations(menu, app.StubRecommender{}, carts, staticAvail{})
	return RouterDeps{
		Restaurant:    "testaurant",
		Config:        RestaurantConfig{Name: "Тест", LogoURL: "/static/testaurant/logo.png"},
		RecBlockTitle: "Заголовок",
		Menu:          menu,
		Cart:          cart,
		Recs:          recs,
		Events:        eventsService,
		Sessions:      sessions,
		CookieMaxAge:  12 * time.Hour,
		DataDir:       t.TempDir(),
	}, carts, eventsService
}

func TestFR33_ConfigEndpointReturnsPluginValues(t *testing.T) {
	sessions := newFakeSessions()
	deps, _, _ := fullDeps(t, sessions)
	srv := httptest.NewServer(newTestHandler(deps))
	defer srv.Close()
	resp, _ := http.Get(srv.URL + "/api/v1/config")
	defer func() { _ = resp.Body.Close() }()
	var body struct {
		Restaurant struct {
			Name    string `json:"name"`
			LogoURL string `json:"logo_url"`
		} `json:"restaurant"`
		RecBlockTitle string `json:"rec_block_title"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Restaurant.Name != "Тест" || body.Restaurant.LogoURL != "/static/testaurant/logo.png" || body.RecBlockTitle != "Заголовок" {
		t.Fatalf("config wrong: %+v", body)
	}
}

func TestNFR04_EventsRateLimit120PerMinutePerSession(t *testing.T) {
	sessions := newFakeSessions()
	deps, _, _ := fullDeps(t, sessions)
	srv := httptest.NewServer(newTestHandler(deps))
	defer srv.Close()
	client := newCookieClient()
	body := `{"name":"menu_view","payload":{}}`
	for i := 1; i <= 121; i++ {
		resp, err := client.Post(srv.URL+"/api/v1/events", "application/json", bytes.NewBufferString(body))
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if i <= 120 && resp.StatusCode != http.StatusNoContent {
			t.Fatalf("event %d must be 204, got %d", i, resp.StatusCode)
		}
		if i == 121 && resp.StatusCode != http.StatusTooManyRequests {
			t.Fatalf("event 121 must be 429, got %d", resp.StatusCode)
		}
	}
}

func TestNFR04_EventsBadRequests(t *testing.T) {
	sessions := newFakeSessions()
	deps, _, _ := fullDeps(t, sessions)
	srv := httptest.NewServer(newTestHandler(deps))
	defer srv.Close()
	client := newCookieClient()
	cases := []struct {
		name string
		body string
	}{
		{"unknown name", `{"name":"nope","payload":{}}`},
		{"nested payload", `{"name":"menu_view","payload":{"x":{"y":1}}}`},
		{"array payload", `{"name":"menu_view","payload":{"x":[1]}}`},
		{"broken json", `{"name":`},
		{"empty name", `{"name":"","payload":{}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := client.Post(srv.URL+"/api/v1/events", "application/json", bytes.NewBufferString(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d", resp.StatusCode)
			}
			var body errorEnvelope
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.Error.Code != "bad_request" {
				t.Fatalf("expected bad_request, got %+v", body)
			}
		})
	}
}

type cookieRoundTripper struct {
	base http.RoundTripper
	jar  map[string]string
}

func (rt *cookieRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if v, ok := rt.jar["session"]; ok {
		req.AddCookie(&http.Cookie{Name: "session", Value: v})
	}
	resp, err := rt.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	for _, c := range resp.Cookies() {
		if c.Name == "session" {
			rt.jar["session"] = c.Value
		}
	}
	return resp, nil
}

type cookieTransport struct {
	rt *cookieRoundTripper
}

func newCookieClient() *http.Client {
	rt := &cookieRoundTripper{base: http.DefaultTransport, jar: map[string]string{}}
	return &http.Client{Transport: &cookieTransport{rt: rt}}
}

func (t *cookieTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.rt.RoundTrip(req)
}

func TestFR37_CartEndpointErrors(t *testing.T) {
	sessions := newFakeSessions()
	deps, carts, _ := fullDeps(t, sessions)
	carts.lines = []domain.CartLine{{ItemID: 11, Qty: 1, AddedFrom: "menu"}}
	srv := httptest.NewServer(newTestHandler(deps))
	defer srv.Close()
	client := newCookieClient()
	post := func(body string) *http.Response {
		resp, err := client.Post(srv.URL+"/api/v1/cart/items", "application/json", bytes.NewBufferString(body))
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		return resp
	}
	if resp := post(`{"item_id":999,"source":"menu"}`); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown item must 404, got %d", resp.StatusCode)
	}
	if resp := post(`{"item_id":11,"source":"banner"}`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad source must 400, got %d", resp.StatusCode)
	}
	if resp := post(`{"source":"menu"}`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing item_id must 400, got %d", resp.StatusCode)
	}
	req, _ := http.NewRequest("PATCH", srv.URL+"/api/v1/cart/items/999", bytes.NewBufferString(`{"qty":2}`))
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("patch missing line must 404, got %d", resp.StatusCode)
	}
	req, _ = http.NewRequest("DELETE", srv.URL+"/api/v1/cart/items/999", nil)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("delete missing line must 404, got %d", resp.StatusCode)
	}
}

func TestFR37_StubRecommendationsEmptyArray(t *testing.T) {
	sessions := newFakeSessions()
	deps, _, _ := fullDeps(t, sessions)
	srv := httptest.NewServer(newTestHandler(deps))
	defer srv.Close()
	client := newCookieClient()
	resp, err := client.Get(srv.URL + "/api/v1/menu/items/11/recommendations")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	raw, _ := readAll(resp)
	if strings.TrimSpace(string(raw)) != `{"recs":[]}` {
		t.Fatalf("stub must return empty recs, got %s", raw)
	}
	resp2, err := client.Get(srv.URL + "/api/v1/menu/items/404/recommendations")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp2.Body.Close()
	if resp2.StatusCode != http.StatusNotFound {
		t.Fatalf("non-numeric id must 404, got %d", resp2.StatusCode)
	}
}

func TestFR33_StaticPathTraversalBlocked(t *testing.T) {
	root := t.TempDir()
	logoDir := filepath.Join(root, "testaurant", "assets")
	if err := os.MkdirAll(logoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(logoDir, "logo.png"), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	secretDir := filepath.Join(root, "testaurant", "recommender")
	if err := os.MkdirAll(secretDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(secretDir, "matrix.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := newStaticHandler(root, "testaurant")
	cases := []string{
		"/static/testaurant/assets/../../recommender/matrix.json",
		"/static/testaurant/photos/../../../etc/passwd",
		"/static/other/assets/logo.png",
		"/static/testaurant/recommender/matrix.json",
		"/static/testaurant/menu.json",
		"/static/testaurant/",
	}
	for _, p := range cases {
		req := httptest.NewRequest("GET", p, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s: expected 404, got %d", p, rec.Code)
		}
	}
	req := httptest.NewRequest("GET", "/static/testaurant/logo.png", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "png" {
		t.Fatalf("logo must be served from assets, got %d %q", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest("GET", "/static/testaurant/assets/logo.png", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("assets path must work, got %d", rec.Code)
	}
}

func readAll(resp *http.Response) ([]byte, error) {
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// NFR-04: nosniff на все ответы; Cache-Control: no-store на /api/v1 JSON (и 204).
func TestNFR04_SecurityHeaders(t *testing.T) {
	sessions := newFakeSessions()
	deps, _, _ := fullDeps(t, sessions)
	srv := httptest.NewServer(newTestHandler(deps))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/config")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("config: X-Content-Type-Options missing: %+v", resp.Header)
	}
	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("config: Cache-Control must be no-store, got %q", resp.Header.Get("Cache-Control"))
	}

	client := newCookieClient()
	r, err := client.Post(srv.URL+"/api/v1/events", "application/json", bytes.NewBufferString(`{"name":"menu_view","payload":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	_ = r.Body.Close()
	if r.StatusCode != http.StatusNoContent {
		t.Fatalf("events: want 204, got %d", r.StatusCode)
	}
	if r.Header.Get("X-Content-Type-Options") != "nosniff" || r.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("events 204: security headers missing: %+v", r.Header)
	}

	// health (без сессии) — тоже JSON API-ответ.
	hm, err := http.Get(srv.URL + "/api/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	_ = hm.Body.Close()
	if hm.Header.Get("X-Content-Type-Options") != "nosniff" || hm.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("health: security headers missing: %+v", hm.Header)
	}

	// статика: nosniff есть, no-store НЕ ставится (ассеты могут кэшироваться).
	root := t.TempDir()
	logoDir := filepath.Join(root, "testaurant", "assets")
	if err := os.MkdirAll(logoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(logoDir, "logo.png"), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	sh := newStaticHandler(root, "testaurant")
	req := httptest.NewRequest("GET", "/static/testaurant/logo.png", nil)
	rec := httptest.NewRecorder()
	sh.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("logo: want 200, got %d", rec.Code)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("static: X-Content-Type-Options missing, got %q", rec.Header().Get("X-Content-Type-Options"))
	}
	if rec.Header().Get("Cache-Control") == "no-store" {
		t.Fatal("static: Cache-Control no-store must not be set for assets")
	}
}
