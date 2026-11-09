package api

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFR33_IndependentCredentialCanonicalReuseAndFixedExpiry(t *testing.T) {
	sessions := newFakeSessions()
	var analyticID string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s, ok := SessionFrom(r.Context())
		if !ok {
			t.Fatal("missing session context")
		}
		analyticID = s.ID
		w.WriteHeader(http.StatusNoContent)
	})
	h := newSessionMiddleware(sessions, 12*time.Hour).Middleware(next)
	first := httptest.NewRecorder()
	h.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/api/v1/cart?table=12", nil))
	credential := first.Result().Cookies()[0].Value
	if credential == analyticID || !isValidUUID(credential) || !isValidUUID(analyticID) {
		t.Fatal("credential and analytics ID must be independent crypto UUIDv4")
	}
	hash := sha256.Sum256([]byte(credential))
	stored := sessions.sessions[hash]
	if stored.id != analyticID || stored.tableID == nil || *stored.tableID != 12 {
		t.Fatal("hash must resolve the independent analytics session")
	}
	h = newSessionMiddleware(sessions, time.Hour).Middleware(next)
	for _, value := range []string{credential, strings.ToUpper(credential)} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/cart?table=99", nil)
		req.AddCookie(&http.Cookie{Name: "session", Value: value})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent || len(rec.Result().Cookies()) != 0 || analyticID != stored.id || sessions.count() != 1 {
			t.Fatal("canonical credential reuse must keep the same session without reissuing cookie")
		}
		if sessions.sessions[hash] != stored {
			t.Fatal("reuse and changed configuration must not change table or expiry")
		}
	}
}

func TestNFR04_InvalidCredentialsCannotBypassCreationRateLimit(t *testing.T) {
	for _, kind := range []string{"missing", "malformed", "unknown", "analytics ID", "expired"} {
		t.Run(kind, func(t *testing.T) {
			sessions := newFakeSessions()
			id, credential := newUUIDv4(), newUUIDv4()
			hash := sha256.Sum256([]byte(credential))
			sessions.sessions[hash] = fakeSession{id: id, expiresAt: time.Now().Add(time.Hour)}
			value := credential
			switch kind {
			case "missing":
				value = ""
			case "malformed":
				value = "not-a-uuid"
			case "unknown":
				value = newUUIDv4()
			case "analytics ID":
				value = id
			case "expired":
				sessions.sessions[hash] = fakeSession{id: id, expiresAt: time.Now().Add(-time.Second)}
			}
			h := newSessionMiddleware(sessions, time.Hour).Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				s, _ := SessionFrom(r.Context())
				if s.ID == id {
					t.Fatal("invalid credential authenticated the old session")
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			for i := 0; i < 31; i++ {
				req := httptest.NewRequest(http.MethodGet, "/api/v1/cart", nil)
				if value != "" {
					req.AddCookie(&http.Cookie{Name: "session", Value: value})
				}
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, req)
				if i < 30 && (rec.Code != http.StatusNoContent || len(rec.Result().Cookies()) != 1) {
					t.Fatal("invalid credential must create a new rate-limited anonymous session")
				}
				if i == 30 && (rec.Code != http.StatusTooManyRequests || len(rec.Result().Cookies()) != 0) {
					t.Fatal("all anonymous session creation must use the same IP limit")
				}
			}
		})
	}
}

func TestNFR04_SessionDatabaseErrorsFailClosed(t *testing.T) {
	for _, operation := range []string{"lookup", "create"} {
		t.Run(operation, func(t *testing.T) {
			sessions := newFakeSessions()
			if operation == "lookup" {
				sessions.getErr = errors.New("database failure")
			} else {
				sessions.createErr = errors.New("database failure")
			}
			called := false
			h := newSessionMiddleware(sessions, time.Hour).Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
			req := httptest.NewRequest(http.MethodGet, "/api/v1/cart", nil)
			req.AddCookie(&http.Cookie{Name: "session", Value: newUUIDv4()})
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if called || rec.Code != http.StatusInternalServerError || len(rec.Result().Cookies()) != 0 || len(sessions.created) != 0 {
				t.Fatal("database error must fail closed, without fallback or new session")
			}
		})
	}
}

func TestNFR04_SessionLogsOmitCredentialHashAndAnalyticsID(t *testing.T) {
	var logs bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(previous) })
	sessions := newFakeSessions()
	h := withLogging(newSessionMiddleware(sessions, time.Hour).Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})))
	first := httptest.NewRecorder()
	h.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/api/v1/cart", nil))
	credential := first.Result().Cookies()[0].Value
	hash := sha256.Sum256([]byte(credential))
	hashText := hex.EncodeToString(hash[:])
	id := sessions.created[0].id
	sessions.getErr = errors.New(credential + " " + hashText + " " + id)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cart", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: credential})
	h.ServeHTTP(httptest.NewRecorder(), req)
	log.SetOutput(previous)
	for _, sensitive := range []string{credential, hashText, id} {
		if strings.Contains(logs.String(), sensitive) {
			t.Fatal("session credential, hash or analytical ID leaked into logs")
		}
	}
}

type failingEntropy struct{}

func (failingEntropy) Read(p []byte) (int, error) { return 0, errors.New("entropy unavailable") }

func TestNFR04_SessionRNGFailureHasNoFallback(t *testing.T) {
	for _, successfulBytes := range []int{0, 16} {
		func() {
			previous := rand.Reader
			rand.Reader = io.MultiReader(bytes.NewReader(make([]byte, successfulBytes)), failingEntropy{})
			defer func() { rand.Reader = previous }()
			sessions := newFakeSessions()
			called := false
			h := withRecovery(newSessionMiddleware(sessions, time.Hour).Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/cart", nil))
			if called || rec.Code != http.StatusInternalServerError || len(rec.Result().Cookies()) != 0 || len(sessions.created) != 0 {
				t.Fatal("failure generating either UUID must fail closed without persisting a partial session")
			}
		}()
	}
}
