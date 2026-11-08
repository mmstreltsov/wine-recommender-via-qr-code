package api

import (
	"mime"
	"net/http"
	"net/url"
	"strings"
)

// NFR-04: reject browser cross-origin writes before creating or loading a session.
func withMutationGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiPath := r.URL.Path == "/api/v1" || strings.HasPrefix(r.URL.Path, "/api/v1/")
		write := r.Method == http.MethodPost || r.Method == http.MethodPatch || r.Method == http.MethodDelete
		if !apiPath || !write {
			next.ServeHTTP(w, r)
			return
		}
		if !validWriteOrigin(r) || !validWriteContentType(r) {
			writeError(w, http.StatusBadRequest, "bad_request", "Некорректный запрос")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func validWriteOrigin(r *http.Request) bool {
	if sites := r.Header.Values("Sec-Fetch-Site"); len(sites) > 1 {
		return false
	} else if len(sites) == 1 && sites[0] != "same-origin" && sites[0] != "none" {
		return false
	}
	origins := r.Header.Values("Origin")
	if len(origins) == 0 {
		return true
	}
	if len(origins) != 1 {
		return false
	}
	origin, err := url.Parse(origins[0])
	if err != nil || origin.Host == "" || origins[0] != origin.Scheme+"://"+origin.Host {
		return false
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return origin.Scheme == scheme && strings.EqualFold(origin.Host, r.Host)
}

func validWriteContentType(r *http.Request) bool {
	if r.Method == http.MethodDelete {
		return r.ContentLength == 0 && len(r.TransferEncoding) == 0
	}
	values := r.Header.Values("Content-Type")
	if len(values) != 1 {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(values[0])
	return err == nil && mediaType == "application/json"
}
