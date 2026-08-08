package webui

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"net/http"
	"strings"
	"sync"

	sharedui "github.com/izzyreal/ciwi/ui"
)

//go:embed assets
var uiAssets embed.FS

type embeddedAsset struct {
	path        string
	contentType string
	cache       bool
	immutable   bool
}

var staticRoutes = map[string]embeddedAsset{
	"/favicon.ico":                    {"assets/ciwi-favicon.png", "image/png", true, false},
	"/ciwi-favicon.png":               {"assets/ciwi-favicon.png", "image/png", true, false},
	"/ui/fonts/ciwi-mono-regular.ttf": {"assets/fonts/GeistMono-Regular.ttf", "font/ttf", true, false},
	"/ui/fonts/ciwi-mono-medium.ttf":  {"assets/fonts/GeistMono-Medium.ttf", "font/ttf", true, false},
	"/ui/fonts/ciwi-mono-bold.ttf":    {"assets/fonts/GeistMono-Bold.ttf", "font/ttf", true, false},
	"/ui/icons.svg":                   {"assets/tabler-icons.svg", "image/svg+xml", true, false},
	"/ui/theme.js":                    {"assets/js/theme.js", "application/javascript; charset=utf-8", true, true},
	"/ui/heartbeat.js":                {"assets/js/heartbeat.js", "application/javascript; charset=utf-8", true, true},
	"/ui/actions.js":                  {"assets/js/actions.js", "application/javascript; charset=utf-8", true, true},
	"/ui/view-state.js":               {"assets/js/view-state.js", "application/javascript; charset=utf-8", true, true},
	"/ui/change-refresh.js":           {"assets/js/change-refresh.js", "application/javascript; charset=utf-8", true, true},
	"/ui/declarative.js":              {"assets/js/declarative.js", "application/javascript; charset=utf-8", true, true},
	"/ui/css/chrome.css":              {"assets/css/chrome.css", "text/css; charset=utf-8", true, true},
	"/ui/css/declarative.css":         {"assets/css/declarative.css", "text/css; charset=utf-8", true, true},
}

var (
	browserUIRevisionOnce sync.Once
	browserUIRevision     string
)

func currentBrowserUIRevision() string {
	browserUIRevisionOnce.Do(func() {
		hash := sha256.New()
		_ = fs.WalkDir(uiAssets, "assets", func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			payload, err := uiAssets.ReadFile(path)
			if err != nil {
				return err
			}
			_, _ = hash.Write([]byte(path))
			_, _ = hash.Write([]byte{0})
			_, _ = hash.Write(payload)
			_, _ = hash.Write([]byte{0})
			return nil
		})
		_, _ = hash.Write([]byte(sharedui.Revision()))
		browserUIRevision = hex.EncodeToString(hash.Sum(nil))[:16]
	})
	return browserUIRevision
}

func cacheVersionedUIResource(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("v") == currentBrowserUIRevision() {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
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
		serveTypographyCSS(w, r)
		return
	}
	if r.URL.Path == "/ui/contracts/typography.json" {
		serveTypographyContract(w, r)
		return
	}
	if r.URL.Path == "/ui/contracts/actions.json" {
		serveActionContract(w, r)
		return
	}
	if r.URL.Path == "/ui/contracts/routes.json" {
		serveRouteContract(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/ui/contracts/screens/") && strings.HasSuffix(r.URL.Path, ".json") {
		name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/ui/contracts/screens/"), ".json")
		if name == "" || strings.Contains(name, "/") || strings.Contains(name, "..") {
			http.NotFound(w, r)
			return
		}
		serveScreenContract(w, r, name)
		return
	}
	if r.URL.Path == "/ui/contracts/themes.json" {
		serveThemeContracts(w, r)
		return
	}
	if asset, ok := staticRoutes[r.URL.Path]; ok {
		if asset.immutable && r.URL.Query().Get("v") != currentBrowserUIRevision() {
			asset.cache = false
			asset.immutable = false
		}
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
	data, err := uiAssets.ReadFile("assets/pages/declarative.html")
	if err != nil {
		http.Error(w, "browser UI unavailable", http.StatusInternalServerError)
		return
	}
	data = []byte(strings.ReplaceAll(string(data), "__CIWI_UI_REVISION__", currentBrowserUIRevision()))
	writeEmbeddedAsset(w, data, embeddedAsset{contentType: "text/html; charset=utf-8"})
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
		if asset.immutable {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=3600")
		}
	} else {
		w.Header().Set("Cache-Control", "no-store")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
