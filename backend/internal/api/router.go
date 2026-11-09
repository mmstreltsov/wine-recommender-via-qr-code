package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"svoyshef/internal/app"
	"svoyshef/internal/store"
)

type Server struct {
	restaurant    string
	config        RestaurantConfig
	recBlockTitle string
	menu          *app.MenuService
	cart          *app.CartService
	recs          *app.Recommendations
	events        *app.EventsService
	eventsLimiter *slidingWindowLimiter
}

type RouterDeps struct {
	Restaurant    string
	Config        RestaurantConfig
	RecBlockTitle string
	Menu          *app.MenuService
	Cart          *app.CartService
	Recs          *app.Recommendations
	Events        *app.EventsService
	Sessions      store.SessionStore
	CookieMaxAge  time.Duration
	DataDir       string
	FrontendDist  string
}

func NewRouter(d RouterDeps) http.Handler {
	s := &Server{
		restaurant:    d.Restaurant,
		config:        d.Config,
		recBlockTitle: d.RecBlockTitle,
		menu:          d.Menu,
		cart:          d.Cart,
		recs:          d.Recs,
		events:        d.Events,
		eventsLimiter: newLimiter(120, time.Minute),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", s.handleHealth)
	mux.HandleFunc("GET /api/v1/config", s.handleConfig)
	mux.HandleFunc("GET /api/v1/menu", s.handleMenu)
	mux.HandleFunc("GET /api/v1/menu/items/{id}", s.handleMenuItem)
	mux.HandleFunc("GET /api/v1/menu/items/{id}/recommendations", s.handleItemRecommendations)
	mux.HandleFunc("GET /api/v1/cart", s.handleCartGet)
	mux.HandleFunc("POST /api/v1/cart/items", s.handleCartAdd)
	mux.HandleFunc("PATCH /api/v1/cart/items/{item_id}", s.handleCartSetQty)
	mux.HandleFunc("DELETE /api/v1/cart/items/{item_id}", s.handleCartRemove)
	mux.HandleFunc("POST /api/v1/events", s.handleEvents)
	mux.Handle("/static/", newStaticHandler(d.DataDir, d.Restaurant))
	if FrontendDistExists(d.FrontendDist) {
		mux.Handle("/", newSpaHandler(d.FrontendDist))
	}
	var h http.Handler = mux
	h = newSessionMiddleware(d.Sessions, d.CookieMaxAge).Middleware(h)
	h = withMutationGuard(h)
	h = withLogging(h)
	h = withRecovery(h)
	h = withSecurityHeaders(h)
	return h
}

// contentSecurityPolicy (NFR-04, defense-in-depth): ограничивает источники
// контента на всех ответах бэкенда. 'unsafe-inline' в style-src НЕ нужен —
// фронтенд не использует инлайн-стили (frontend/src без style=-атрибутов,
// собранный dist подключает только внешние css/js; проверяется тестом
// TestNFR04_CSPNoUnsafeInline). img-src: фото блюд — CDN ресторана
// (static/optim на tildacdn.com/net, включая редиректы ресайза); allowlist входит
// в safeRemoteImage (frontend/src/api/validation.ts), без wildcard-поддоменов.
// connect-src 'self' — SPA ходит только в свой origin.
// Дев (vite :5173) не затрагивается: страницу отдаёт vite без этого заголовка,
// CSP действует лишь на ответы бэкенда; /api приходит через vite-прокси
// (same-origin), HMR-ws — соединение с vite-источником, не с бэкендом.
const contentSecurityPolicy = "default-src 'self'; " +
	"img-src 'self' https://static.tildacdn.com https://optim.tildacdn.com https://static.tildacdn.net https://optim.tildacdn.net; " +
	"script-src 'self'; " +
	"style-src 'self'; " +
	"connect-src 'self'; " +
	"font-src 'self'; " +
	"base-uri 'self'; " +
	"frame-ancestors 'none'; " +
	"form-action 'self'"

// withSecurityHeaders: nosniff и CSP на все ответы; Cache-Control: no-store
// для /api/v1 (JSON API — без промежуточного кэширования).
func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
		if r.URL.Path == "/api/v1" || strings.HasPrefix(r.URL.Path, "/api/v1/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

func FrontendDistExists(dist string) bool {
	st, err := os.Stat(dist)
	return err == nil && st.IsDir() && fileExists(filepath.Join(dist, "index.html"))
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
