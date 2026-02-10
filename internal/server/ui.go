package server

import (
	"net/http"
	"strings"
)

func (s *stateStore) uiHandler(w http.ResponseWriter, r *http.Request) {
	switch {
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
