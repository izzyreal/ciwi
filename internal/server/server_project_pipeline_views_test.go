package server

import "testing"

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
