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
	"/ui/actions.js":                  {"assets/js/actions.js", "application/javascript; charset=utf-8", false},
	"/ui/view-state.js":               {"assets/js/view-state.js", "application/javascript; charset=utf-8", false},
	"/ui/declarative.js":              {"assets/js/declarative.js", "application/javascript; charset=utf-8", false},
	"/ui/css/chrome.css":              {"assets/css/chrome.css", "text/css; charset=utf-8", false},
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
	if r.URL.Path == "/ui/contracts/routes.json" {
		serveRouteContract(w)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/ui/contracts/screens/") && strings.HasSuffix(r.URL.Path, ".json") {
		name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/ui/contracts/screens/"), ".json")
		if name == "" || strings.Contains(name, "/") || strings.Contains(name, "..") {
			http.NotFound(w, r)
			return
		}
		serveScreenContract(w, name)
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

	if strings.HasPrefix(r.URL.Path, "/declarative-preview") {
		http.NotFound(w, r)
		return
	}
	routes, err := loadRouteContract()
	if err != nil {
		http.Error(w, "shared UI routes unavailable", http.StatusInternalServerError)
		return
	}
	if _, ok := routes.Match(r.URL.Path, "web"); !ok {
		http.NotFound(w, r)
		return
	}
	serveEmbeddedAsset(w, embeddedAsset{
		path:        "assets/pages/declarative.html",
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
