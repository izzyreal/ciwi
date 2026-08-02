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

func TestProjectDetailsDeclarativeScreenContractRoute(t *testing.T) {
	recorder := httptest.NewRecorder()
	Handler(recorder, httptest.NewRequest("GET", "/ui/contracts/screens/project-details.json", nil))
	if recorder.Code != 200 {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var screen uidsl.ScreenDocument
	if err := json.Unmarshal(recorder.Body.Bytes(), &screen); err != nil {
		t.Fatal(err)
	}
	if screen.Metadata.Name != "project-details" {
		t.Fatalf("screen = %#v", screen)
	}
}

func TestJobDetailsDeclarativeScreenContractRoute(t *testing.T) {
	recorder := httptest.NewRecorder()
	Handler(recorder, httptest.NewRequest("GET", "/ui/contracts/screens/job-details.json", nil))
	if recorder.Code != 200 {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var screen uidsl.ScreenDocument
	if err := json.Unmarshal(recorder.Body.Bytes(), &screen); err != nil {
		t.Fatal(err)
	}
	if screen.Metadata.Name != "job-details" {
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

func TestDeclarativeProjectPreviewUsesSharedRenderer(t *testing.T) {
	recorder := httptest.NewRecorder()
	Handler(recorder, httptest.NewRequest("GET", "/declarative-preview/projects/7", nil))
	if recorder.Code != 200 || !strings.Contains(recorder.Body.String(), "declarativeRoot") {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestDeclarativeJobPreviewUsesSharedRenderer(t *testing.T) {
	recorder := httptest.NewRecorder()
	Handler(recorder, httptest.NewRequest("GET", "/declarative-preview/jobs/job-1", nil))
	if recorder.Code != 200 || !strings.Contains(recorder.Body.String(), "declarativeRoot") {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
}
