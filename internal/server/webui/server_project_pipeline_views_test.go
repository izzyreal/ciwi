package webui

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
