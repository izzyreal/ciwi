package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUIHandlerServesEveryPublicPageAndAsset(t *testing.T) {
	tests := []struct {
		path        string
		contentType string
	}{
		{"/", "text/html"},
		{"/settings", "text/html"},
		{"/projects/42", "text/html"},
		{"/vault", "text/html"},
		{"/agents", "text/html"},
		{"/agents/agent-1", "text/html"},
		{"/jobs/job-1", "text/html"},
		{"/ui/shared.js", "application/javascript"},
		{"/ui/pages.js", "application/javascript"},
		{"/ui/theme.js", "application/javascript"},
		{"/ui/icons.svg", "image/svg+xml"},
		{"/ciwi-logo.png", "image/png"},
		{"/ciwi-favicon.png", "image/png"},
		{"/favicon.ico", "image/png"},
	}
	server := &stateStore{}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			server.uiHandler(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d", rec.Code)
			}
			if !strings.HasPrefix(rec.Header().Get("Content-Type"), tt.contentType) {
				t.Fatalf("expected content type %q, got %q", tt.contentType, rec.Header().Get("Content-Type"))
			}
			if rec.Body.Len() == 0 {
				t.Fatalf("expected a non-empty response")
			}
		})
	}

	rec := httptest.NewRecorder()
	server.uiHandler(rec, httptest.NewRequest(http.MethodGet, "/missing", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected unknown UI route to return 404, got %d", rec.Code)
	}
}
