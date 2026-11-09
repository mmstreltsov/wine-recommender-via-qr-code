package api

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"
	"time"

	"svoyshef/internal/app"
	"svoyshef/internal/domain"
)

type guardSessionStore struct {
	*fakeSessions
	lookups int
}

func (s *guardSessionStore) GetByCredentialHash(ctx context.Context, hash [32]byte) (string, *int, error) {
	s.lookups++
	return s.fakeSessions.GetByCredentialHash(ctx, hash)
}

func TestNFR04_WriteGuardBeforeSession(t *testing.T) {
	cases := []struct {
		name, origin, site, contentType string
		tls, allowed                    bool
	}{
		{name: "CLI JSON", contentType: "application/json", allowed: true},
		{name: "same origin", origin: "http://example.test:8080", site: "same-origin", contentType: "application/json", allowed: true},
		{name: "same origin without metadata", origin: "http://example.test:8080", contentType: "application/json", allowed: true},
		{name: "TLS same origin", origin: "https://example.test:8080", contentType: "application/json", tls: true, allowed: true},
		{name: "case insensitive host", origin: "http://EXAMPLE.test:8080", contentType: "application/json; charset=utf-8", allowed: true},
		{name: "foreign origin", origin: "http://other.test:8080", contentType: "application/json"},
		{name: "null origin", origin: "null", contentType: "application/json"},
		{name: "sibling origin", origin: "http://sibling.example.test:8080", site: "same-site", contentType: "application/json"},
		{name: "cross site without origin", site: "cross-site", contentType: "application/json"},
		{name: "same site without origin", site: "same-site", contentType: "application/json"},
		{name: "same origin cannot override foreign origin", origin: "http://other.test:8080", site: "same-origin", contentType: "application/json"},
		{name: "metadata cannot override text plain", site: "same-origin", contentType: "text/plain"},
		{name: "unknown metadata", site: "unknown", contentType: "application/json"},
		{name: "origin port mismatch", origin: "http://example.test:8081", contentType: "application/json"},
		{name: "origin scheme mismatch", origin: "https://example.test:8080", contentType: "application/json"},
		{name: "TLS scheme mismatch", origin: "http://example.test:8080", contentType: "application/json", tls: true},
		{name: "origin path", origin: "http://example.test:8080/", contentType: "application/json"},
		{name: "origin userinfo", origin: "http://other@example.test:8080", contentType: "application/json"},
		{name: "origin query", origin: "http://example.test:8080?", contentType: "application/json"},
		{name: "origin fragment", origin: "http://example.test:8080#", contentType: "application/json"},
		{name: "multiple origin values", origin: "http://example.test:8080 http://other.test", contentType: "application/json"},
		{name: "malformed origin", origin: "http://[", contentType: "application/json"},
		{name: "text plain", contentType: "text/plain"},
		{name: "form encoded", contentType: "application/x-www-form-urlencoded"},
		{name: "multipart", contentType: "multipart/form-data; boundary=boundary"},
		{name: "missing content type"},
		{name: "malformed content type", contentType: "application/json; charset"},
		{name: "JSON suffix is not JSON media type", contentType: "application/problem+json"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sessions := &guardSessionStore{fakeSessions: newFakeSessions()}
			deps, cart, _ := fullDeps(t, sessions.fakeSessions)
			deps.Sessions = sessions
			req := httptest.NewRequest(http.MethodPost, "http://example.test:8080/api/v1/cart/items?table=12", strings.NewReader(`{"item_id":11,"source":"menu"}`))
			req.AddCookie(&http.Cookie{Name: "session", Value: newUUIDv4()})
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if tc.site != "" {
				req.Header.Set("Sec-Fetch-Site", tc.site)
			}
			if tc.contentType != "" {
				req.Header.Set("Content-Type", tc.contentType)
			}
			req.Header.Set("X-Forwarded-Host", "other.test:8080")
			req.Header.Set("X-Forwarded-Proto", "https")
			if tc.tls {
				req.TLS = &tls.ConnectionState{}
			}
			rec := httptest.NewRecorder()
			NewRouter(deps).ServeHTTP(rec, req)
			if tc.allowed {
				if rec.Code != http.StatusCreated || sessions.count() != 1 || len(cart.lines) != 1 {
					t.Fatalf("valid JSON write rejected: status=%d sessions=%d", rec.Code, sessions.count())
				}
				return
			}
			if rec.Code != http.StatusBadRequest || sessions.count() != 0 || sessions.lookups != 0 || len(cart.lines) != 0 || len(rec.Result().Cookies()) != 0 {
				t.Fatalf("unsafe request reached session/cart: status=%d sessions=%d lookups=%d", rec.Code, sessions.count(), sessions.lookups)
			}
			var body errorEnvelope
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Error.Code != "bad_request" || body.Error.Message != "Некорректный запрос" {
				t.Fatal("guard must return a generic contract error")
			}
		})
	}
}

func TestNFR04_WriteGuardAllMethodsAndDuplicateHeaders(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPatch, http.MethodDelete} {
		for _, header := range []string{"Origin", "Sec-Fetch-Site", "Content-Type"} {
			t.Run(method+"/"+header, func(t *testing.T) {
				req := httptest.NewRequest(method, "http://example.test/api/v1/cart/items/11", nil)
				req.Header.Set("Origin", "http://example.test")
				req.Header.Set("Sec-Fetch-Site", "same-origin")
				req.Header.Set("Content-Type", "application/json")
				req.Header.Add(header, req.Header.Get(header))
				called := false
				h := withMutationGuard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, req)
				// Bodyless DELETE does not use Content-Type.
				if method == http.MethodDelete && header == "Content-Type" {
					if !called {
						t.Fatal("bodyless DELETE rejected")
					}
				} else if called || rec.Code != http.StatusBadRequest {
					t.Fatal("duplicate security header accepted")
				}
			})
		}
	}
}

func TestNFR04_WriteGuardProtectsExistingCartAndEvents(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPatch, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			sessions := newFakeSessions()
			id := newUUIDv4()
			credential := newUUIDv4()
			if err := sessions.Create(context.Background(), id, nil, sha256.Sum256([]byte(credential)), time.Hour); err != nil {
				t.Fatal(err)
			}
			deps, cart, _ := fullDeps(t, sessions)
			cart.lines = []domain.CartLine{{ItemID: 11, Qty: 1, AddedFrom: "menu"}}
			for _, path := range []string{"/api/v1/cart/items/11", "/api/v1/events"} {
				req := httptest.NewRequest(method, path, strings.NewReader(`{"qty":2}`))
				req.AddCookie(&http.Cookie{Name: "session", Value: credential})
				req.Header.Set("Origin", "http://sibling.example.test")
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()
				NewRouter(deps).ServeHTTP(rec, req)
				if rec.Code != http.StatusBadRequest || len(cart.lines) != 1 || cart.lines[0].Qty != 1 {
					t.Fatal("cross-origin write affected existing session cart")
				}
			}
		})
	}
}

func TestNFR04_WriteGuardBodylessDeleteAndReadRoutes(t *testing.T) {
	for _, tc := range []struct {
		method, path, body string
		want               int
	}{
		{http.MethodDelete, "/api/v1/cart/items/11", "", http.StatusOK},
		{http.MethodDelete, "/api/v1/cart/items/11", "{}", http.StatusBadRequest},
		{http.MethodPatch, "/api/v1/cart/items/11", `{"qty":2}`, http.StatusOK},
		{http.MethodPost, "/api/v1/events", `{"name":"menu_view","payload":{}}`, http.StatusNoContent},
		{http.MethodGet, "/api/v1/health", "", http.StatusOK},
		{http.MethodGet, "/api/v1/menu?table=12", "", http.StatusOK},
	} {
		t.Run(tc.method+tc.path+tc.body, func(t *testing.T) {
			sessions := newFakeSessions()
			deps, cart, _ := fullDeps(t, sessions)
			cart.lines = []domain.CartLine{{ItemID: 11, Qty: 1, AddedFrom: "menu"}}
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			if tc.method == http.MethodPatch || tc.method == http.MethodPost {
				req.Header.Set("Content-Type", "application/json")
			}
			if tc.method == http.MethodGet {
				req.Header.Set("Origin", "null")
				req.Header.Set("Sec-Fetch-Site", "cross-site")
			}
			rec := httptest.NewRecorder()
			NewRouter(deps).ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status=%d, want %d", rec.Code, tc.want)
			}
			if tc.path == "/api/v1/health" && sessions.count() != 0 {
				t.Fatal("health created a session")
			}
			if strings.Contains(tc.path, "table=12") && (sessions.count() != 1 || sessions.created[0].tableID == nil || *sessions.created[0].tableID != 12) {
				t.Fatal("query startup lost table ID")
			}
		})
	}
}

func TestNFR04_ProxyMustPreserveHost(t *testing.T) {
	for _, changeOrigin := range []bool{false, true} {
		t.Run(map[bool]string{false: "preserve Host", true: "reject rewritten Host"}[changeOrigin], func(t *testing.T) {
			sessions := newFakeSessions()
			backend := httptest.NewServer(NewRouter(baseDeps(t, sessions)))
			defer backend.Close()
			target, err := url.Parse(backend.URL)
			if err != nil {
				t.Fatal(err)
			}
			proxy := httputil.NewSingleHostReverseProxy(target)
			director := proxy.Director
			proxy.Director = func(r *http.Request) {
				director(r)
				if changeOrigin {
					r.Header.Set("X-Forwarded-Host", r.Host)
					r.Host = target.Host
				}
			}
			frontend := httptest.NewServer(proxy)
			defer frontend.Close()
			req, err := http.NewRequest(http.MethodPost, frontend.URL+"/api/v1/cart/items", strings.NewReader(`{"item_id":11,"source":"menu"}`))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Origin", frontend.URL)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Sec-Fetch-Site", "same-origin")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = resp.Body.Close() }()
			want, count := http.StatusCreated, 1
			if changeOrigin {
				want, count = http.StatusBadRequest, 0
			}
			if resp.StatusCode != want || sessions.count() != count {
				t.Fatalf("proxy status=%d sessions=%d", resp.StatusCode, sessions.count())
			}
		})
	}
}

func TestNFR04_DecodeSingleJSONBeforeMutation(t *testing.T) {
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/v1/cart/items", `{"item_id":11,"source":"menu"}`},
		{http.MethodPatch, "/api/v1/cart/items/11", `{"qty":2}`},
		{http.MethodPost, "/api/v1/events", `{"name":"menu_view","payload":{}}`},
	} {
		for _, suffix := range []string{" {}", " null", " true", " trailing", strings.Repeat(" ", maxBodyBytes)} {
			sessions := newFakeSessions()
			deps, cart, _ := fullDeps(t, sessions)
			cart.lines = []domain.CartLine{{ItemID: 11, Qty: 1, AddedFrom: "menu"}}
			events := &apiFakeEventStore{}
			deps.Events = app.NewEventsService(events)
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body+suffix))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			NewRouter(deps).ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest || len(cart.lines) != 1 || cart.lines[0].Qty != 1 || len(events.events) != 0 {
				t.Fatalf("trailing JSON affected mutation: method=%s status=%d", tc.method, rec.Code)
			}
		}
	}
}

func TestNFR04_DecodeJSONWhitespaceAndUnknownFields(t *testing.T) {
	for _, tc := range []struct {
		body string
		want int
	}{
		{"{\"item_id\":11,\"source\":\"menu\"} \n\t", http.StatusCreated},
		{`{"item_id":11,"source":"menu","note":"synthetic free text"}`, http.StatusBadRequest},
	} {
		sessions := newFakeSessions()
		deps, cart, _ := fullDeps(t, sessions)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/cart/items", strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		NewRouter(deps).ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Fatalf("status=%d want=%d", rec.Code, tc.want)
		}
		if tc.want == http.StatusBadRequest && len(cart.lines) != 0 {
			t.Fatal("unknown body fields must not mutate cart")
		}
	}
}
