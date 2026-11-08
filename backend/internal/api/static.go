package api

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type staticHandler struct {
	dataDir    string
	restaurant string
}

func newStaticHandler(dataDir, restaurant string) *staticHandler {
	return &staticHandler{dataDir: dataDir, restaurant: restaurant}
}

func (h *staticHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rel := strings.TrimPrefix(r.URL.Path, "/static/")
	segs := strings.Split(rel, "/")
	if len(segs) == 0 || segs[0] != h.restaurant {
		http.NotFound(w, r)
		return
	}
	sub := segs[1:]
	var fsPath string
	switch {
	case len(sub) == 1 && sub[0] == "logo.png":
		fsPath = path.Join(h.dataDir, h.restaurant, "assets", "logo.png")
	case len(sub) >= 1 && (sub[0] == "assets" || sub[0] == "photos"):
		fsPath = path.Join(h.dataDir, h.restaurant, path.Join(sub...))
	default:
		http.NotFound(w, r)
		return
	}
	for _, seg := range sub {
		if seg == ".." {
			http.NotFound(w, r)
			return
		}
	}
	root := filepath.Join(h.dataDir, h.restaurant)
	abs := filepath.Clean(fsPath)
	if !strings.HasPrefix(abs, filepath.Clean(root)+string(os.PathSeparator)) {
		http.NotFound(w, r)
		return
	}
	st, err := os.Stat(abs)
	if err != nil || st.IsDir() {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, abs)
}

type spaHandler struct {
	dir string
}

func newSpaHandler(dir string) *spaHandler {
	return &spaHandler{dir: dir}
}

func (h *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.NotFound(w, r)
		return
	}
	rel := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	full := filepath.Join(h.dir, filepath.FromSlash(rel))
	root := filepath.Clean(h.dir)
	if full != root && !strings.HasPrefix(full, root+string(os.PathSeparator)) {
		http.NotFound(w, r)
		return
	}
	if st, err := os.Stat(full); err == nil && !st.IsDir() {
		http.ServeFile(w, r, full)
		return
	}
	http.ServeFile(w, r, filepath.Join(h.dir, "index.html"))
}
