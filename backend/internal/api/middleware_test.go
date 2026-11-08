package api

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type fakeSessions struct {
	mu       sync.Mutex
	sessions map[string]*int
	created  []struct {
		id      string
		tableID *int
	}
}

func newFakeSessions() *fakeSessions {
	return &fakeSessions{sessions: map[string]*int{}}
}

func (f *fakeSessions) Create(ctx context.Context, id string, tableID *int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sessions[id] = tableID
	f.created = append(f.created, struct {
		id      string
		tableID *int
	}{id, tableID})
	return nil
}

func (f *fakeSessions) Get(ctx context.Context, id string) (bool, *int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	tid, ok := f.sessions[id]
	return ok, tid, nil
}

func (f *fakeSessions) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sessions)
}

func newTestHandler(deps RouterDeps) http.Handler {
	return NewRouter(deps)
}

func tlsInsecure() *tls.Config {
	return &tls.Config{InsecureSkipVerify: true}
}

func baseDeps(t *testing.T, sessions *fakeSessions) RouterDeps {
	t.Helper()
	deps, _, _ := fullDeps(t, sessions)
	return deps
}

func TestFR33_CookieAttributesOnFirstRequest(t *testing.T) {
	sessions := newFakeSessions()
	h := newTestHandler(baseDeps(t, sessions))
	srv := httptest.NewServer(h)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/api/v1/menu")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	cookies := resp.Cookies()
	if len(cookies) != 1 || cookies[0].Name != "session" {
		t.Fatalf("expected one session cookie, got %v", cookies)
	}
	c := cookies[0]
	if !c.HttpOnly {
		t.Fatal("cookie must be HttpOnly")
	}
	if c.SameSite != http.SameSiteLaxMode {
		t.Fatal("cookie must be SameSite=Lax")
	}
	if c.Path != "/" {
		t.Fatal("cookie Path must be /")
	}
	if c.MaxAge != 12*3600 {
		t.Fatalf("Max-Age must be 43200, got %d", c.MaxAge)
	}
	if c.Secure {
		t.Fatal("Secure must be omitted without TLS")
	}
	if !isValidUUID(c.Value) || !strings.Contains(newUUIDv4(), "-") {
		t.Fatalf("cookie value must be UUID, got %q", c.Value)
	}
	if sessions.count() != 1 {
		t.Fatalf("expected 1 session row, got %d", sessions.count())
	}
}

func TestFR33_CookieSecureOverTLS(t *testing.T) {
	sessions := newFakeSessions()
	srv := httptest.NewTLSServer(newTestHandler(baseDeps(t, sessions)))
	defer srv.Close()
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: tlsInsecure()}}
	resp, err := client.Get(srv.URL + "/api/v1/menu")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	cookies := resp.Cookies()
	if len(cookies) != 1 || !cookies[0].Secure {
		t.Fatalf("Secure flag expected over TLS, got %v", cookies)
	}
}

func TestFR33_SameSessionReusedWithCookie(t *testing.T) {
	sessions := newFakeSessions()
	srv := httptest.NewServer(newTestHandler(baseDeps(t, sessions)))
	defer srv.Close()
	resp, _ := http.Get(srv.URL + "/api/v1/menu")
	cookie := resp.Cookies()[0]
	_ = resp.Body.Close()
	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/menu", nil)
	req.AddCookie(cookie)
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp2.Body.Close() }()
	if len(resp2.Cookies()) != 0 {
		t.Fatalf("no Set-Cookie expected for known session, got %v", resp2.Cookies())
	}
	if sessions.count() != 1 {
		t.Fatalf("same session must be reused, sessions=%d", sessions.count())
	}
}

func TestFR33_UnknownCookieCreatesNewSession(t *testing.T) {
	sessions := newFakeSessions()
	srv := httptest.NewServer(newTestHandler(baseDeps(t, sessions)))
	defer srv.Close()
	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/menu", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: newUUIDv4()})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if len(resp.Cookies()) != 1 {
		t.Fatal("new Set-Cookie expected for unknown session id")
	}
	if sessions.count() != 1 {
		t.Fatalf("expected 1 recreated session, got %d", sessions.count())
	}
}

func TestFR33_HealthDoesNotCreateSession(t *testing.T) {
	sessions := newFakeSessions()
	srv := httptest.NewServer(newTestHandler(baseDeps(t, sessions)))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/api/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health status %d", resp.StatusCode)
	}
	if len(resp.Cookies()) != 0 {
		t.Fatal("health must not set session cookie")
	}
	if sessions.count() != 0 {
		t.Fatal("health must not create sessions (compose healthcheck + IP limit)")
	}
	var body struct {
		Status     string `json:"status"`
		Restaurant string `json:"restaurant"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "ok" || body.Restaurant != "testaurant" {
		t.Fatalf("health body wrong: %+v", body)
	}
}

func TestFR33_HealthDoesNotConsumeIPLimit(t *testing.T) {
	sessions := newFakeSessions()
	srv := httptest.NewServer(newTestHandler(baseDeps(t, sessions)))
	defer srv.Close()
	for i := 0; i < 50; i++ {
		resp, err := http.Get(srv.URL + "/api/v1/health")
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("health %d: status %d, want 200", i+1, resp.StatusCode)
		}
		if len(resp.Cookies()) != 0 {
			t.Fatalf("health %d: must not set session cookie", i+1)
		}
	}
	if sessions.count() != 0 {
		t.Fatalf("50 health requests must create 0 sessions, got %d", sessions.count())
	}
	resp, err := http.Get(srv.URL + "/api/v1/menu")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("new session after 50 health checks from same IP must pass IP limit (30/min), got %d", resp.StatusCode)
	}
	if len(resp.Cookies()) != 1 {
		t.Fatalf("new session cookie expected, got %v", resp.Cookies())
	}
	if sessions.count() != 1 {
		t.Fatalf("session must be created, got %d", sessions.count())
	}
}

func TestFR34_TableParamOnlyAtCreation(t *testing.T) {
	sessions := newFakeSessions()
	srv := httptest.NewServer(newTestHandler(baseDeps(t, sessions)))
	defer srv.Close()
	resp, _ := http.Get(srv.URL + "/api/v1/menu?table=12")
	c := resp.Cookies()[0]
	_ = resp.Body.Close()
	if sessions.created[0].tableID == nil || *sessions.created[0].tableID != 12 {
		t.Fatalf("table_id must be 12, got %v", sessions.created[0].tableID)
	}
	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/menu?table=99", nil)
	req.AddCookie(c)
	resp2, _ := http.DefaultClient.Do(req)
	_ = resp2.Body.Close()
	if sessions.count() != 1 {
		t.Fatal("no new session expected")
	}
	stored := sessions.created[0].tableID
	if stored == nil || *stored != 12 {
		t.Fatalf("table must stay 12, got %v", stored)
	}
}

func TestFR34_InvalidTableIgnored(t *testing.T) {
	for _, raw := range []string{"abc", "0", "1000", "-5", "12.5", ""} {
		sessions := newFakeSessions()
		srv := httptest.NewServer(newTestHandler(baseDeps(t, sessions)))
		resp, _ := http.Get(srv.URL + "/api/v1/menu?table=" + raw)
		_ = resp.Body.Close()
		srv.Close()
		if sessions.count() != 1 {
			t.Fatalf("table=%q: session must still be created", raw)
		}
		if sessions.created[0].tableID != nil {
			t.Fatalf("table=%q must be ignored (NULL), got %v", raw, *sessions.created[0].tableID)
		}
	}
}

func TestFR34_ParseTableID(t *testing.T) {
	if got := parseTableID("12"); got == nil || *got != 12 {
		t.Fatalf("parse 12: %v", got)
	}
	for _, raw := range []string{"", "abc", "0", "1000", "-1"} {
		if got := parseTableID(raw); got != nil {
			t.Fatalf("parse %q: expected nil, got %v", raw, *got)
		}
	}
}

func TestNFR04_SessionIPLimit30PerMinute(t *testing.T) {
	sessions := newFakeSessions()
	srv := httptest.NewServer(newTestHandler(baseDeps(t, sessions)))
	defer srv.Close()
	last := 0
	for i := 0; i < 31; i++ {
		resp, err := http.Get(srv.URL + "/api/v1/menu")
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		last = resp.StatusCode
		if i < 30 && resp.StatusCode != http.StatusOK {
			t.Fatalf("request %d must pass, got %d", i+1, resp.StatusCode)
		}
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("31st new session from same IP must be 429, got %d", last)
	}
}

func TestNFR04_RecoveryReturns500WithoutDetails(t *testing.T) {
	var h http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom /secret/path")
	})
	h = withRecovery(h)
	srv := httptest.NewServer(h)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.StatusCode)
	}
	var body errorEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != "internal" || body.Error.Message != "Внутренняя ошибка сервера" {
		t.Fatalf("error body wrong: %+v", body)
	}
	if strings.Contains(body.Error.Message, "secret") {
		t.Fatal("internal details leaked")
	}
}
