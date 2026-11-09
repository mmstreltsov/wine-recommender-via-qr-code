package api

import (
	"context"
	"crypto/sha256"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"svoyshef/internal/store"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}

func withRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Print("ERROR operation=handle_request code=panic")
				writeError(w, http.StatusInternalServerError, "internal", "Внутренняя ошибка сервера")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		if rec.status == 0 {
			rec.status = http.StatusOK
		}
		log.Printf("operation=http_request status=%d duration=%s", rec.status, time.Since(start).Round(time.Millisecond))
	})
}

type sessionCtxKey struct{}

type Session struct {
	ID      string
	TableID *int
}

func SessionFrom(ctx context.Context) (*Session, bool) {
	s, ok := ctx.Value(sessionCtxKey{}).(*Session)
	return s, ok
}

type sessionMiddleware struct {
	sessions    store.SessionStore
	cookieName  string
	cookieMaxAg int
	ipLimiter   *slidingWindowLimiter
}

func newSessionMiddleware(sessions store.SessionStore, cookieMaxAge time.Duration) *sessionMiddleware {
	return &sessionMiddleware{
		sessions:    sessions,
		cookieName:  "session",
		cookieMaxAg: int(cookieMaxAge.Seconds()),
		ipLimiter:   newLimiter(30, time.Minute),
	}
}

func parseTableID(raw string) *int {
	if raw == "" {
		return nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > 999 {
		return nil
	}
	return &n
}

func clientIP(remoteAddr string) string {
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		return host
	}
	return remoteAddr
}

func (m *sessionMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !sessionNeeded(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		ctx := r.Context()
		var sess *Session
		if c, err := r.Cookie(m.cookieName); err == nil && isValidUUID(c.Value) {
			hash := sha256.Sum256([]byte(strings.ToLower(c.Value)))
			id, tableID, err := m.sessions.GetByCredentialHash(ctx, hash)
			if err != nil {
				writeAppError(w, err)
				return
			}
			if id != "" {
				sess = &Session{ID: id, TableID: tableID}
			}
		}
		if sess == nil {
			ip := clientIP(r.RemoteAddr)
			if !m.ipLimiter.Allow(ip) {
				writeError(w, http.StatusTooManyRequests, "rate_limited", "Слишком много запросов")
				return
			}
			id := newUUIDv4()
			credential := newUUIDv4()
			hash := sha256.Sum256([]byte(credential))
			tableID := parseTableID(r.URL.Query().Get("table"))
			if err := m.sessions.Create(ctx, id, tableID, hash, time.Duration(m.cookieMaxAg)*time.Second); err != nil {
				writeAppError(w, err)
				return
			}
			http.SetCookie(w, &http.Cookie{
				Name:     m.cookieName,
				Value:    credential,
				Path:     "/",
				MaxAge:   m.cookieMaxAg,
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
				Secure:   r.TLS != nil,
			})
			sess = &Session{ID: id, TableID: tableID}
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, sessionCtxKey{}, sess)))
	})
}

func sessionNeeded(path string) bool {
	return path == "/api/v1" || (len(path) > len("/api/v1/") && path[:len("/api/v1/")] == "/api/v1/" && path != "/api/v1/health")
}
