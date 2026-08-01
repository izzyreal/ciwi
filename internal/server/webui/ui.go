package webui

import (
	"embed"
	"net/http"
	"strings"
)

//go:embed assets/ciwi-logo.png assets/ciwi-favicon.png assets/tabler-icons.svg
var uiAssets embed.FS

// Handler serves ciwi's browser pages and embedded static assets.
func Handler(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/favicon.ico" || r.URL.Path == "/ciwi-favicon.png":
		serveEmbeddedPNG(w, "assets/ciwi-favicon.png")
		return
	case r.URL.Path == "/ciwi-logo.png":
		serveEmbeddedPNG(w, "assets/ciwi-logo.png")
		return
	case r.URL.Path == "/ui/icons.svg":
		serveEmbeddedAsset(w, "assets/tabler-icons.svg", "image/svg+xml")
		return
	case r.URL.Path == "/ui/theme.js":
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_, _ = w.Write([]byte(uiThemeJS))
		return
	case r.URL.Path == "/":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(indexHTML))
		return
	case r.URL.Path == "/settings":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(settingsHTML))
		return
	case r.URL.Path == "/ui/shared.js":
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		_, _ = w.Write([]byte(uiSharedJS))
		return
	case r.URL.Path == "/ui/pages.js":
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		_, _ = w.Write([]byte(uiPagesJS))
		return
	case strings.HasPrefix(r.URL.Path, "/projects/"):
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(projectHTML))
		return
	case r.URL.Path == "/vault":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(vaultHTML))
		return
	case r.URL.Path == "/agents":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(agentsHTML))
		return
	case strings.HasPrefix(r.URL.Path, "/agents/"):
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(agentHTML))
		return
	case strings.HasPrefix(r.URL.Path, "/jobs/"):
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(jobExecutionHTML))
		return
	default:
		http.NotFound(w, r)
	}
}

func serveEmbeddedPNG(w http.ResponseWriter, path string) {
	serveEmbeddedAsset(w, path, "image/png")
}

func serveEmbeddedAsset(w http.ResponseWriter, path, contentType string) {
	data, err := uiAssets.ReadFile(path)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
