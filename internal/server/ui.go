package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func (s *stateStore) uiHandler(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/favicon.ico" || r.URL.Path == "/ciwi-favicon.png":
		servePNGFile(w, "ciwi-favicon.png")
		return
	case r.URL.Path == "/ciwi-logo.png":
		servePNGFile(w, "ciwi-logo.png")
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

func servePNGFile(w http.ResponseWriter, path string) {
	resolved, err := resolveAssetPath(path)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func resolveAssetPath(name string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	// Try current dir and a few parents so tests running from package dirs still find repo-root assets.
	dir := cwd
	for i := 0; i < 6; i++ {
		candidate := filepath.Join(dir, name)
		if st, statErr := os.Stat(candidate); statErr == nil && !st.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", os.ErrNotExist
}
