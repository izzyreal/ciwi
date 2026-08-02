package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/izzyreal/ciwi/internal/config"
)

func TestProjectDetailsViewUsesApplicationPresentationShape(t *testing.T) {
	ts, state := newTestHTTPServerWithState(t)
	defer ts.Close()
	cfg, err := config.Parse([]byte(testConfigYAML), "ciwi-project.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := state.db.LoadConfig(cfg, "ciwi-project.yaml", "https://github.com/izzyreal/ciwi.git", "main", "ciwi-project.yaml"); err != nil {
		t.Fatal(err)
	}
	project, err := state.db.GetProjectByName("ciwi")
	if err != nil {
		t.Fatal(err)
	}
	response := mustJSONRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/v1/views/projects/"+int64ToString(project.ID), nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", response.StatusCode, readBody(t, response))
	}
	defer response.Body.Close()
	var view projectDetailsViewResponse
	if err := json.NewDecoder(response.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	if view.Project.Name != "ciwi" || len(view.Pipelines) != 1 || len(view.Pipelines[0].Jobs) != 1 {
		t.Fatalf("view = %+v", view)
	}
	if steps := view.Pipelines[0].Jobs[0].Steps; len(steps) == 0 || steps[0].Name == "" {
		t.Fatalf("steps = %+v", steps)
	}
}
