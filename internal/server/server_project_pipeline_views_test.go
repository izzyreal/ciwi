package server

import (
	"strings"
	"testing"
)

func TestPipelineChainUIUsesNamesAndProjectScopedDerivedIDs(t *testing.T) {
	for _, want := range []string{
		"function pipelineChainSequence(chain)",
		"function pipelineChainDisplayName(chain)",
		"function pipelineChainDisplayHTML(chain)",
		"function pipelineChainSequenceHTML(chain)",
		"function pipelineChainAPIPath(projectID, chainID, action)",
		"pipelineChainDisplayHTML(c)",
		"pipelineChainDisplayHTML(ch)",
		"pipelineChainSequenceHTML(c)",
		"pipelineChainSequenceHTML(ch)",
		"pipelineChainAPIPath(project.id, c.id, 'run')",
		"pipelineChainAPIPath(p.id, ch.id, 'run')",
	} {
		if !strings.Contains(uiPagesJS+uiIndexProjectsJS+projectHTML, want) {
			t.Fatalf("pipeline-chain UI no longer contains %q", want)
		}
	}
	if strings.Contains(uiIndexProjectsJS+projectHTML, "/api/v1/pipeline-chains/") {
		t.Fatalf("pipeline-chain UI still contains the legacy unscoped route")
	}
	if strings.Contains(uiIndexProjectsJS+projectHTML, ".chain_id") {
		t.Fatalf("pipeline-chain UI still uses the removed chain_id response field")
	}
}

func TestBuildPipelineVersionPreviewResponses(t *testing.T) {
	errResp := buildPipelineVersionPreviewErrorResponse("boom")
	if errResp.OK {
		t.Fatalf("error response should have ok=false")
	}
	if errResp.Message != "boom" {
		t.Fatalf("unexpected error response message: %q", errResp.Message)
	}

	success := buildPipelineVersionPreviewSuccessResponse(pipelineRunContext{
		Version:           " 1.2.3 ",
		VersionRaw:        " 1.2.3+meta ",
		SourceRefResolved: " refs/heads/main ",
		VersionFile:       " VERSION ",
		TagPrefix:         " v ",
		AutoBump:          " patch ",
	})
	if !success.OK {
		t.Fatalf("success response should have ok=true")
	}
	if success.PipelineVersion != "1.2.3" || success.PipelineVersionRaw != "1.2.3+meta" {
		t.Fatalf("unexpected version fields: %+v", success)
	}
	if success.SourceRefResolved != "refs/heads/main" || success.VersionFile != "VERSION" || success.TagPrefix != "v" || success.AutoBump != "patch" {
		t.Fatalf("unexpected trimmed success fields: %+v", success)
	}
}
