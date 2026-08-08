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

func TestUIHandlerServesEveryDeclarativeRouteAndAsset(t *testing.T) {
	tests := []struct {
		path        string
		contentType string
	}{
		{"/", "text/html"}, {"/settings", "text/html"}, {"/projects/42", "text/html"},
		{"/vault", "text/html"}, {"/agents", "text/html"}, {"/agents/agent-1", "text/html"},
		{"/agents/agent-1/script", "text/html"}, {"/jobs/job-1", "text/html"},
		{"/managed-yaml/new", "text/html"}, {"/managed-yaml/42", "text/html"},
		{"/run-options/pipelines/42", "text/html"},
		{"/run-options/projects/1/pipelines/42", "text/html"},
		{"/run-options/projects/1/chains/release", "text/html"},
		{"/ciwi-logo.png", "image/png"},
	}
	for path, asset := range staticRoutes {
		tests = append(tests, struct{ path, contentType string }{path, asset.contentType})
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			Handler(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
			}
			if !strings.HasPrefix(rec.Header().Get("Content-Type"), tt.contentType) || rec.Body.Len() == 0 {
				t.Fatalf("content type = %q, body bytes = %d", rec.Header().Get("Content-Type"), rec.Body.Len())
			}
		})
	}
	for _, path := range []string{"/missing", "/projects/1/extra", "/declarative-preview", "/declarative-preview/projects/1"} {
		rec := httptest.NewRecorder()
		Handler(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s status = %d, want 404", path, rec.Code)
		}
	}
}

func TestUIHandlerServesLogoFromSharedBundle(t *testing.T) {
	want, err := sharedui.Read("assets/ciwi-logo.png")
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	Handler(rec, httptest.NewRequest(http.MethodGet, "/ciwi-logo.png", nil))
	if rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), want) {
		t.Fatal("browser logo does not match the shared UI asset")
	}
}

func TestDeclarativePageReferencesOnlyServedUIAssets(t *testing.T) {
	page := mustTestAsset("assets/pages/declarative.html")
	assetReference := regexp.MustCompile(`(?:src|href)="(/ui/[^"?]+)`)
	for _, match := range assetReference.FindAllStringSubmatch(page, -1) {
		rec := httptest.NewRecorder()
		Handler(rec, httptest.NewRequest(http.MethodGet, match[1], nil))
		if rec.Code != http.StatusOK {
			t.Errorf("page references unserved asset %s (status %d)", match[1], rec.Code)
		}
	}
}

func TestDeclarativePageBootstrapsFromVersionedCachedResources(t *testing.T) {
	recorder := httptest.NewRecorder()
	Handler(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("page status = %d: %s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	revision := currentBrowserUIRevision()
	if strings.Contains(body, "__CIWI_UI_REVISION__") || !strings.Contains(body, "?v="+revision) {
		t.Fatal("page does not reference the current browser UI revision")
	}
	if strings.Contains(body, "Loading ciwi") || !strings.Contains(body, "ciwi-bootstrap-shell") {
		t.Fatal("page does not provide an immediate skeleton bootstrap shell")
	}

	for _, path := range []string{
		"/ui/declarative.js?v=" + revision,
		"/ui/contracts/screens/front-page.json?v=" + revision,
		"/ui/contracts/themes.json?v=" + revision,
	} {
		assetRecorder := httptest.NewRecorder()
		Handler(assetRecorder, httptest.NewRequest(http.MethodGet, path, nil))
		if got := assetRecorder.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
			t.Errorf("%s Cache-Control = %q, want immutable caching", path, got)
		}
	}
	unversionedRecorder := httptest.NewRecorder()
	Handler(unversionedRecorder, httptest.NewRequest(http.MethodGet, "/ui/declarative.js", nil))
	if got := unversionedRecorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("unversioned asset Cache-Control = %q, want no-store", got)
	}
}

func TestMissingEmbeddedAssetReturnsNotFound(t *testing.T) {
	rec := httptest.NewRecorder()
	serveEmbeddedAsset(rec, embeddedAsset{path: "assets/missing.js", contentType: "application/javascript"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected missing embedded asset to return 404, got %d", rec.Code)
	}
}

func BenchmarkDeclarativePageRoute(b *testing.B) {
	request := httptest.NewRequest(http.MethodGet, "/projects/42", nil)
	b.ReportAllocs()
	for range b.N {
		recorder := httptest.NewRecorder()
		Handler(recorder, request)
		if recorder.Code != http.StatusOK {
			b.Fatal(recorder.Code)
		}
	}
}
