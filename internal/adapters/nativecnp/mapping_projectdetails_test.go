package nativecnp

import (
	"testing"

	"github.com/izzyreal/ciwi/internal/domain"
	"github.com/izzyreal/ciwi/internal/presentation"
)

func TestProjectDetailsMappingCarriesAuthoritativePresentationFields(t *testing.T) {
	view := presentation.ProjectDetailsView{
		Project: domain.Project{
			ID: 7, Name: "ciwi", SourceKind: "repository", RepoURL: "https://example.test/ciwi.git",
			RepoRef: "main", LoadedCommit: "1234567890abcdef",
		},
		ProjectLabels: presentation.ProjectLabels{PipelineCount: "1 pipeline", SourceMetadata: "branch: main", HasPipelineChains: true},
		Pipelines: []presentation.ProjectPipelineView{{
			ID: 11, PipelineID: "build", SummaryLabel: "1 job · depends on: none", GraphSummary: "1 job · 0 dependencies",
			Jobs: []presentation.ProjectJobView{{
				ID: "test", SummaryLabel: "1 step · runs on: linux", TimeoutLabel: "Timeout: 60s", MatrixLabel: "Matrix: none",
				Steps: []presentation.ProjectStepView{{Name: "Test", DisplayCommand: "go test ./...", EnvironmentLabel: "No environment overrides"}},
			}},
		}},
		StructureFilters: []presentation.ProjectStructureFilterView{{
			Value: "all-pipelines", Label: "All Pipelines", PipelineIDs: []string{"build"}, ShowPipelineStructure: true,
			Root: presentation.ProjectStructureRootView{ID: "project:7:all-pipelines", Label: "ciwi", Meta: "Project · 1 pipeline", ProjectID: 7},
		}},
	}

	mapped := projectDetailsToProto(view)
	if mapped.Project.GetPipelineCountLabel() != "1 pipeline" || mapped.Project.GetSourceMetadata() != "branch: main" || !mapped.Project.GetHasPipelineChains() {
		t.Fatalf("project labels = %+v", mapped.Project)
	}
	if mapped.Project.GetIsManaged() || !mapped.Project.GetCanReload() || mapped.Project.GetRepoRefLabel() != "main" || mapped.Project.GetLoadedCommitShort() != "12345678" {
		t.Fatalf("project settings semantics = %+v", mapped.Project)
	}
	if mapped.Pipelines[0].GetSummaryLabel() != view.Pipelines[0].SummaryLabel || mapped.Pipelines[0].GetGraphSummaryLabel() != view.Pipelines[0].GraphSummary {
		t.Fatalf("pipeline labels = %+v", mapped.Pipelines[0])
	}
	if mapped.Pipelines[0].Jobs[0].GetSummaryLabel() == "" || mapped.Pipelines[0].Jobs[0].Steps[0].GetEnvironmentLabel() == "" {
		t.Fatalf("job mapping = %+v", mapped.Pipelines[0].Jobs[0])
	}
	if len(mapped.StructureFilters) != 1 || mapped.StructureFilters[0].GetRoot().GetId() != "project:7:all-pipelines" {
		t.Fatalf("structure filters = %+v", mapped.StructureFilters)
	}
}
