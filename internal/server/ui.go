package server

import (
	"embed"
	"net/http"
	"strings"
)

//go:embed assets/ciwi-logo.png assets/ciwi-favicon.png
var uiAssets embed.FS

func (s *stateStore) uiHandler(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/favicon.ico" || r.URL.Path == "/ciwi-favicon.png":
		serveEmbeddedPNG(w, "assets/ciwi-favicon.png")
		return
	case r.URL.Path == "/ciwi-logo.png":
		serveEmbeddedPNG(w, "assets/ciwi-logo.png")
		return
	case r.URL.Path == "/":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(indexHTML))
		return
	case r.URL.Path == "/ui/shared.js":
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		_, _ = w.Write([]byte(uiSharedJS))
		return
	case strings.HasPrefix(r.URL.Path, "/projects/"):
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(projectHTML))
		return
	case strings.HasPrefix(r.URL.Path, "/jobs/"):
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(jobHTML))
		return
	default:
		http.NotFound(w, r)
	}
}

func serveEmbeddedPNG(w http.ResponseWriter, path string) {
	data, err := uiAssets.ReadFile(path)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
