package webui

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	sharedui "github.com/izzyreal/ciwi/ui"
)

func TestUIHandlerServesEveryPublicPageAndAsset(t *testing.T) {
	tests := []struct {
		path        string
		contentType string
	}{
		{"/", "text/html"},
		{"/ciwi-logo.png", "image/png"},
		{"/settings", "text/html"},
		{"/projects/42", "text/html"},
		{"/vault", "text/html"},
		{"/agents", "text/html"},
		{"/agents/agent-1", "text/html"},
		{"/jobs/job-1", "text/html"},
	}
	for path, asset := range staticRoutes {
		tests = append(tests, struct {
			path        string
			contentType string
		}{path: path, contentType: asset.contentType})
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			Handler(rec, req)
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
	Handler(rec, httptest.NewRequest(http.MethodGet, "/missing", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected unknown UI route to return 404, got %d", rec.Code)
	}
}

func TestUIHandlerServesLogoFromSharedBundle(t *testing.T) {
	want, err := sharedui.Read("assets/ciwi-logo.png")
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	Handler(rec, httptest.NewRequest(http.MethodGet, "/ciwi-logo.png", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !bytes.Equal(rec.Body.Bytes(), want) {
		t.Fatal("browser logo does not match the shared UI asset")
	}
}

func TestEveryPageReferencesServedUIAssets(t *testing.T) {
	assetReference := regexp.MustCompile(`(?:src|href)="(/ui/[^"]+)"`)
	for pagePath, page := range map[string]string{
		"/": indexHTML, "/settings": settingsHTML, "/projects/1": projectHTML,
		"/vault": vaultHTML, "/agents": agentsHTML, "/agents/a": agentHTML, "/jobs/j": jobExecutionHTML,
	} {
		for _, match := range assetReference.FindAllStringSubmatch(page, -1) {
			assetPath := strings.SplitN(match[1], "#", 2)[0]
			rec := httptest.NewRecorder()
			Handler(rec, httptest.NewRequest(http.MethodGet, assetPath, nil))
			if rec.Code != http.StatusOK {
				t.Errorf("page %s references unserved asset %s (status %d)", pagePath, assetPath, rec.Code)
			}
		}
	}
}

func TestPagesKeepBehaviorInRealCSSAndJavaScriptAssets(t *testing.T) {
	scriptTag := regexp.MustCompile(`<script([^>]*)>`)
	for name, page := range map[string]string{
		"index": indexHTML, "settings": settingsHTML, "project": projectHTML,
		"vault": vaultHTML, "agents": agentsHTML, "agent": agentHTML, "job-execution": jobExecutionHTML,
	} {
		if strings.Contains(page, "<style>") {
			t.Errorf("%s page reintroduced an inline style block", name)
		}
		for _, match := range scriptTag.FindAllStringSubmatch(page, -1) {
			if !strings.Contains(match[1], "src=") {
				t.Errorf("%s page reintroduced an inline script block: %s", name, match[0])
			}
		}
		for _, ref := range []string{
			`href="/ui/css/chrome.css?v=2"`,
			`href="/ui/css/` + name + `.css"`,
			`src="/ui/` + name + `.js"`,
		} {
			if !strings.Contains(page, ref) {
				t.Errorf("%s page does not reference %s", name, ref)
			}
		}
	}
}

func TestMissingEmbeddedAssetReturnsNotFound(t *testing.T) {
	rec := httptest.NewRecorder()
	serveEmbeddedAsset(rec, embeddedAsset{path: "assets/missing.js", contentType: "application/javascript"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected missing embedded asset to return 404, got %d", rec.Code)
	}
}
