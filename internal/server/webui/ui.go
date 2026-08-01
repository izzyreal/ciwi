package webui

import (
	"embed"
	"net/http"
	"strings"
)

//go:embed assets
var uiAssets embed.FS

type embeddedAsset struct {
	path        string
	contentType string
	cache       bool
}

var staticRoutes = map[string]embeddedAsset{
	"/favicon.ico":              {"assets/ciwi-favicon.png", "image/png", true},
	"/ciwi-favicon.png":         {"assets/ciwi-favicon.png", "image/png", true},
	"/ciwi-logo.png":            {"assets/ciwi-logo.png", "image/png", true},
	"/ui/icons.svg":             {"assets/tabler-icons.svg", "image/svg+xml", true},
	"/ui/theme.js":              {"assets/js/theme.js", "application/javascript; charset=utf-8", true},
	"/ui/shared.js":             {"assets/js/shared.js", "application/javascript; charset=utf-8", false},
	"/ui/pages.js":              {"assets/js/pages.js", "application/javascript; charset=utf-8", false},
	"/ui/index.js":              {"assets/js/index.js", "application/javascript; charset=utf-8", false},
	"/ui/settings.js":           {"assets/js/settings.js", "application/javascript; charset=utf-8", false},
	"/ui/project.js":            {"assets/js/project.js", "application/javascript; charset=utf-8", false},
	"/ui/vault.js":              {"assets/js/vault.js", "application/javascript; charset=utf-8", false},
	"/ui/agents.js":             {"assets/js/agents.js", "application/javascript; charset=utf-8", false},
	"/ui/agent.js":              {"assets/js/agent.js", "application/javascript; charset=utf-8", false},
	"/ui/job-execution.js":      {"assets/js/job-execution.js", "application/javascript; charset=utf-8", false},
	"/ui/css/chrome.css":        {"assets/css/chrome.css", "text/css; charset=utf-8", false},
	"/ui/css/index.css":         {"assets/css/index.css", "text/css; charset=utf-8", false},
	"/ui/css/settings.css":      {"assets/css/settings.css", "text/css; charset=utf-8", false},
	"/ui/css/project.css":       {"assets/css/project.css", "text/css; charset=utf-8", false},
	"/ui/css/vault.css":         {"assets/css/vault.css", "text/css; charset=utf-8", false},
	"/ui/css/agents.css":        {"assets/css/agents.css", "text/css; charset=utf-8", false},
	"/ui/css/agent.css":         {"assets/css/agent.css", "text/css; charset=utf-8", false},
	"/ui/css/job-execution.css": {"assets/css/job-execution.css", "text/css; charset=utf-8", false},
}

// Handler serves ciwi's browser pages and embedded static assets.
func Handler(w http.ResponseWriter, r *http.Request) {
	if asset, ok := staticRoutes[r.URL.Path]; ok {
		serveEmbeddedAsset(w, asset)
		return
	}

	page := ""
	switch {
	case r.URL.Path == "/":
		page = "index"
	case r.URL.Path == "/settings":
		page = "settings"
	case strings.HasPrefix(r.URL.Path, "/projects/"):
		page = "project"
	case r.URL.Path == "/vault":
		page = "vault"
	case r.URL.Path == "/agents":
		page = "agents"
	case strings.HasPrefix(r.URL.Path, "/agents/"):
		page = "agent"
	case strings.HasPrefix(r.URL.Path, "/jobs/"):
		page = "job-execution"
	default:
		http.NotFound(w, r)
		return
	}
	serveEmbeddedAsset(w, embeddedAsset{
		path:        "assets/pages/" + page + ".html",
		contentType: "text/html; charset=utf-8",
	})
}

func serveEmbeddedAsset(w http.ResponseWriter, asset embeddedAsset) {
	data, err := uiAssets.ReadFile(asset.path)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", asset.contentType)
	if asset.cache {
		w.Header().Set("Cache-Control", "public, max-age=3600")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
