package webui

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/izzyreal/ciwi/pkg/uidsl"
)

func TestDeclarativeScreenContractRoute(t *testing.T) {
	recorder := httptest.NewRecorder()
	Handler(recorder, httptest.NewRequest("GET", "/ui/contracts/screens/front-page.json", nil))
	if recorder.Code != 200 {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var screen uidsl.ScreenDocument
	if err := json.Unmarshal(recorder.Body.Bytes(), &screen); err != nil {
		t.Fatal(err)
	}
	if screen.APIVersion != uidsl.APIVersion || screen.Metadata.Name != "front-page" {
		t.Fatalf("screen = %#v", screen)
	}
}

func TestDeclarativePreviewUsesSharedContractRenderer(t *testing.T) {
	recorder := httptest.NewRecorder()
	Handler(recorder, httptest.NewRequest("GET", "/declarative-preview", nil))
	if recorder.Code != 200 {
		t.Fatalf("status = %d", recorder.Code)
	}
	page := recorder.Body.String()
	for _, expected := range []string{"declarativeRoot", "/ui/declarative.js", "/ui/css/declarative.css"} {
		if !strings.Contains(page, expected) {
			t.Errorf("preview page does not contain %q", expected)
		}
	}
}
