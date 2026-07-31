package server

import (
	"net/http"
	"strings"
	"testing"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func TestManagedYAMLAPIValidationCreateReadUpdateAndConflict(t *testing.T) {
	ts, s := newTestHTTPServerWithState(t)
	defer ts.Close()

	rawInitial := managedYAMLAPIConfig("Managed project", "build")
	validateResp := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/projects/managed-yaml/validate", map[string]any{"yaml": rawInitial})
	if validateResp.StatusCode != http.StatusOK {
		t.Fatalf("expected validation success, got %d body=%s", validateResp.StatusCode, readBody(t, validateResp))
	}
	var validated protocol.ManagedYAMLDefinition
	decodeJSONBody(t, validateResp, &validated)
	if validated.ProjectName != "Managed project" || validated.Pipelines != 1 {
		t.Fatalf("unexpected validation response: %+v", validated)
	}
	projects, err := s.db.ListProjects()
	if err != nil || len(projects) != 0 {
		t.Fatalf("validation mutated projects: projects=%+v err=%v", projects, err)
	}

	createResp := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/projects/managed-yaml", map[string]any{"yaml": rawInitial})
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected create success, got %d body=%s", createResp.StatusCode, readBody(t, createResp))
	}
	var created protocol.ManagedYAMLDefinition
	decodeJSONBody(t, createResp, &created)
	if created.ProjectID <= 0 || created.Revision == "" || created.YAML != "" {
		t.Fatalf("unexpected create response: %+v", created)
	}

	getURL := ts.URL + "/api/v1/projects/" + int64ToString(created.ProjectID) + "/managed-yaml"
	getResp := mustJSONRequest(t, ts.Client(), http.MethodGet, getURL, nil)
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected managed YAML read, got %d body=%s", getResp.StatusCode, readBody(t, getResp))
	}
	var loaded protocol.ManagedYAMLDefinition
	decodeJSONBody(t, getResp, &loaded)
	if loaded.YAML != rawInitial || loaded.Revision != created.Revision {
		t.Fatalf("unexpected loaded definition: %+v", loaded)
	}

	rawUpdated := managedYAMLAPIConfig("Renamed managed project", "release")
	updateResp := mustJSONRequest(t, ts.Client(), http.MethodPut, getURL, map[string]any{"yaml": rawUpdated, "revision": loaded.Revision})
	if updateResp.StatusCode != http.StatusOK {
		t.Fatalf("expected update success, got %d body=%s", updateResp.StatusCode, readBody(t, updateResp))
	}
	var updated protocol.ManagedYAMLDefinition
	decodeJSONBody(t, updateResp, &updated)
	if updated.ProjectID != created.ProjectID || updated.ProjectName != "Renamed managed project" || updated.Revision == loaded.Revision {
		t.Fatalf("unexpected update response: %+v", updated)
	}

	staleResp := mustJSONRequest(t, ts.Client(), http.MethodPut, getURL, map[string]any{"yaml": rawInitial, "revision": loaded.Revision})
	if staleResp.StatusCode != http.StatusConflict {
		t.Fatalf("expected stale update conflict, got %d body=%s", staleResp.StatusCode, readBody(t, staleResp))
	}
	afterStale := mustJSONRequest(t, ts.Client(), http.MethodGet, getURL, nil)
	var unchanged protocol.ManagedYAMLDefinition
	decodeJSONBody(t, afterStale, &unchanged)
	if unchanged.YAML != rawUpdated || unchanged.Revision != updated.Revision {
		t.Fatalf("stale API update changed definition: %+v", unchanged)
	}

	listResp := mustJSONRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/v1/projects", nil)
	var listPayload struct {
		Projects []protocol.ProjectSummary `json:"projects"`
	}
	decodeJSONBody(t, listResp, &listPayload)
	if len(listPayload.Projects) != 1 || listPayload.Projects[0].SourceKind != protocol.ProjectSourceManagedYAML || listPayload.Projects[0].Name != "Renamed managed project" {
		t.Fatalf("unexpected managed project list: %+v", listPayload.Projects)
	}

	deleteResp := mustJSONRequest(t, ts.Client(), http.MethodDelete, ts.URL+"/api/v1/projects/"+int64ToString(created.ProjectID), nil)
	if deleteResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected managed project deletion, got %d", deleteResp.StatusCode)
	}
	missingResp := mustJSONRequest(t, ts.Client(), http.MethodGet, getURL, nil)
	if missingResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected deleted managed project to be absent, got %d", missingResp.StatusCode)
	}
}

func TestManagedYAMLAPIRejectsInvalidDuplicateWrongSourceAndOversize(t *testing.T) {
	ts, s := newTestHTTPServerWithState(t)
	defer ts.Close()

	invalid := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/projects/managed-yaml", map[string]any{"yaml": "not: [valid"})
	if invalid.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected invalid YAML rejection, got %d", invalid.StatusCode)
	}
	projectsAfterInvalid, err := s.db.ListProjects()
	if err != nil || len(projectsAfterInvalid) != 0 {
		t.Fatalf("invalid YAML mutated projects: projects=%+v err=%v", projectsAfterInvalid, err)
	}
	raw := managedYAMLAPIConfig("Unique name", "build")
	createdResp := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/projects/managed-yaml", map[string]any{"yaml": raw})
	var created protocol.ManagedYAMLDefinition
	decodeJSONBody(t, createdResp, &created)
	duplicateRaw := managedYAMLAPIConfig("unique NAME", "other")
	duplicate := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/projects/managed-yaml", map[string]any{"yaml": duplicateRaw})
	if duplicate.StatusCode != http.StatusConflict {
		t.Fatalf("expected duplicate name conflict, got %d body=%s", duplicate.StatusCode, readBody(t, duplicate))
	}

	loadPipelineTestConfig(t, s, testConfigYAML)
	vcs, err := s.db.GetProjectByName("ciwi")
	if err != nil {
		t.Fatalf("get VCS project: %v", err)
	}
	wrongSource := mustJSONRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/v1/projects/"+int64ToString(vcs.ID)+"/managed-yaml", nil)
	if wrongSource.StatusCode != http.StatusConflict {
		t.Fatalf("expected VCS managed-YAML rejection, got %d", wrongSource.StatusCode)
	}
	reloadManaged := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/projects/"+int64ToString(created.ProjectID)+"/reload", map[string]any{})
	if reloadManaged.StatusCode != http.StatusConflict {
		t.Fatalf("expected managed project VCS reload rejection, got %d", reloadManaged.StatusCode)
	}

	oversize := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/projects/managed-yaml", map[string]any{"yaml": strings.Repeat("x", managedYAMLMaxRequestBytes)})
	if oversize.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected request size rejection, got %d body=%s", oversize.StatusCode, readBody(t, oversize))
	}
}

func managedYAMLAPIConfig(projectName, pipelineID string) string {
	return `version: 1
project:
  name: ` + projectName + `
pipelines:
  - id: ` + pipelineID + `
    trigger: manual
    jobs:
      - id: run
        runs_on:
          executor: script
          shell: posix
        timeout_seconds: 60
        steps:
          - run: echo ok
`
}
