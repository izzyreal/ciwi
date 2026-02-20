package httpx

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteJSONSetsHeadersAndBody(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteJSON(rec, 201, map[string]string{"ok": "yes"})

	if rec.Code != 201 {
		t.Fatalf("status code: got %d want %d", rec.Code, 201)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content-type: got %q", got)
	}
	if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "no-store") {
		t.Fatalf("cache-control missing no-store: %q", got)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"ok":"yes"`) {
		t.Fatalf("unexpected body: %q", body)
	}
}
