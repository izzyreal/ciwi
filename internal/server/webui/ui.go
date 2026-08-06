package webui

import (
	"embed"
	"net/http"
	"strings"

	sharedui "github.com/izzyreal/ciwi/ui"
)

//go:embed assets
var uiAssets embed.FS

type embeddedAsset struct {
	path        string
	contentType string
	cache       bool
}

var staticRoutes = map[string]embeddedAsset{
	"/favicon.ico":                    {"assets/ciwi-favicon.png", "image/png", true},
	"/ciwi-favicon.png":               {"assets/ciwi-favicon.png", "image/png", true},
	"/ui/fonts/ciwi-mono-regular.ttf": {"assets/fonts/GeistMono-Regular.ttf", "font/ttf", true},
	"/ui/fonts/ciwi-mono-medium.ttf":  {"assets/fonts/GeistMono-Medium.ttf", "font/ttf", true},
	"/ui/fonts/ciwi-mono-bold.ttf":    {"assets/fonts/GeistMono-Bold.ttf", "font/ttf", true},
	"/ui/icons.svg":                   {"assets/tabler-icons.svg", "image/svg+xml", true},
	"/ui/theme.js":                    {"assets/js/theme.js", "application/javascript; charset=utf-8", false},
	"/ui/heartbeat.js":                {"assets/js/heartbeat.js", "application/javascript; charset=utf-8", false},
	"/ui/shared.js":                   {"assets/js/shared.js", "application/javascript; charset=utf-8", false},
	"/ui/actions.js":                  {"assets/js/actions.js", "application/javascript; charset=utf-8", false},
	"/ui/pages.js":                    {"assets/js/pages.js", "application/javascript; charset=utf-8", false},
	"/ui/index.js":                    {"assets/js/index.js", "application/javascript; charset=utf-8", false},
	"/ui/settings.js":                 {"assets/js/settings.js", "application/javascript; charset=utf-8", false},
	"/ui/project.js":                  {"assets/js/project.js", "application/javascript; charset=utf-8", false},
	"/ui/vault.js":                    {"assets/js/vault.js", "application/javascript; charset=utf-8", false},
	"/ui/agents.js":                   {"assets/js/agents.js", "application/javascript; charset=utf-8", false},
	"/ui/agent.js":                    {"assets/js/agent.js", "application/javascript; charset=utf-8", false},
	"/ui/job-execution.js":            {"assets/js/job-execution.js", "application/javascript; charset=utf-8", false},
	"/ui/declarative.js":              {"assets/js/declarative.js", "application/javascript; charset=utf-8", false},
	"/ui/css/chrome.css":              {"assets/css/chrome.css", "text/css; charset=utf-8", false},
	"/ui/css/index.css":               {"assets/css/index.css", "text/css; charset=utf-8", false},
	"/ui/css/settings.css":            {"assets/css/settings.css", "text/css; charset=utf-8", false},
	"/ui/css/project.css":             {"assets/css/project.css", "text/css; charset=utf-8", false},
	"/ui/css/vault.css":               {"assets/css/vault.css", "text/css; charset=utf-8", false},
	"/ui/css/agents.css":              {"assets/css/agents.css", "text/css; charset=utf-8", false},
	"/ui/css/agent.css":               {"assets/css/agent.css", "text/css; charset=utf-8", false},
	"/ui/css/job-execution.css":       {"assets/css/job-execution.css", "text/css; charset=utf-8", false},
	"/ui/css/declarative.css":         {"assets/css/declarative.css", "text/css; charset=utf-8", false},
}

// Handler serves ciwi's browser pages and embedded static assets.
func Handler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/ciwi-logo.png" {
		serveSharedUIAsset(w, embeddedAsset{
			path:        "assets/ciwi-logo.png",
			contentType: "image/png",
			cache:       true,
		})
		return
	}
	if r.URL.Path == "/ui/css/typography.css" {
		serveTypographyCSS(w)
		return
	}
	if r.URL.Path == "/ui/contracts/typography.json" {
		serveTypographyContract(w)
		return
	}
	if r.URL.Path == "/ui/contracts/actions.json" {
		serveActionContract(w)
		return
	}
	if r.URL.Path == "/ui/contracts/screens/front-page.json" {
		serveScreenContract(w, "front-page")
		return
	}
	if r.URL.Path == "/ui/contracts/screens/project-details.json" {
		serveScreenContract(w, "project-details")
		return
	}
	if r.URL.Path == "/ui/contracts/screens/job-details.json" {
		serveScreenContract(w, "job-details")
		return
	}
	if r.URL.Path == "/ui/contracts/screens/settings.json" {
		serveScreenContract(w, "settings")
		return
	}
	if r.URL.Path == "/ui/contracts/screens/run-options.json" {
		serveScreenContract(w, "run-options")
		return
	}
	if r.URL.Path == "/ui/contracts/screens/agents.json" {
		serveScreenContract(w, "agents")
		return
	}
	if r.URL.Path == "/ui/contracts/screens/agent-details.json" {
		serveScreenContract(w, "agent-details")
		return
	}
	if r.URL.Path == "/ui/contracts/screens/connection.json" {
		serveScreenContract(w, "connection")
		return
	}
	if r.URL.Path == "/ui/contracts/themes.json" {
		serveThemeContracts(w)
		return
	}
	if asset, ok := staticRoutes[r.URL.Path]; ok {
		serveEmbeddedAsset(w, asset)
		return
	}

	page := ""
	switch {
	case r.URL.Path == "/":
		page = "index"
	case r.URL.Path == "/declarative-preview":
		page = "declarative"
	case strings.HasPrefix(r.URL.Path, "/declarative-preview/projects/"):
		page = "declarative"
	case strings.HasPrefix(r.URL.Path, "/declarative-preview/jobs/"):
		page = "declarative"
	case strings.HasPrefix(r.URL.Path, "/declarative-preview/run-options/"):
		page = "declarative"
	case r.URL.Path == "/declarative-preview/settings" || r.URL.Path == "/declarative-preview/settings/":
		page = "declarative"
	case r.URL.Path == "/declarative-preview/agents" || r.URL.Path == "/declarative-preview/agents/":
		page = "declarative"
	case strings.HasPrefix(r.URL.Path, "/declarative-preview/agents/"):
		page = "declarative"
	case r.URL.Path == "/declarative-preview/connection" || r.URL.Path == "/declarative-preview/connection/":
		page = "declarative"
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
	writeEmbeddedAsset(w, data, asset)
}

func serveSharedUIAsset(w http.ResponseWriter, asset embeddedAsset) {
	data, err := sharedui.Read(asset.path)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	writeEmbeddedAsset(w, data, asset)
}

func writeEmbeddedAsset(w http.ResponseWriter, data []byte, asset embeddedAsset) {
	w.Header().Set("Content-Type", asset.contentType)
	if asset.cache {
		w.Header().Set("Cache-Control", "public, max-age=3600")
	} else {
		w.Header().Set("Cache-Control", "no-store")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
