package api

import (
	"bytes"
	"crypto/rand"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

type failingResponseWriter struct {
	header http.Header
	err    error
}

func (w *failingResponseWriter) Header() http.Header         { return w.header }
func (w *failingResponseWriter) WriteHeader(status int)      {}
func (w *failingResponseWriter) Write(p []byte) (int, error) { return 0, w.err }

func TestNFR04_HTTPLogsOmitCredentialsAndRawErrors(t *testing.T) {
	var logs bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(previous) })
	credential := rand.Text()
	dbErr := &pgconn.PgError{Code: "23505", Message: credential, Detail: credential}
	rec := httptest.NewRecorder()
	writeAppError(rec, dbErr)
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), credential) {
		t.Fatal("database error must produce a generic 500")
	}
	writeJSON(&failingResponseWriter{header: make(http.Header), err: errors.New(credential)}, http.StatusOK, map[string]string{"status": "ok"})
	h := withRecovery(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { panic(dbErr) }))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	h = withLogging(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	req := httptest.NewRequest(http.MethodGet, "/api/"+credential+"?token="+credential, nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: credential})
	req.Header.Set("Authorization", "Bearer "+credential)
	h.ServeHTTP(httptest.NewRecorder(), req)
	log.SetOutput(previous)
	out := logs.String()
	if strings.Contains(out, credential) {
		t.Fatal("HTTP logs contain credentials or raw errors")
	}
	for _, safe := range []string{"operation=handle_request code=internal", "operation=write_response code=internal", "operation=handle_request code=panic", "operation=http_request status=204"} {
		if !strings.Contains(out, safe) {
			t.Errorf("missing safe log %s", safe)
		}
	}
}
