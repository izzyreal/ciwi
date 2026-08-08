package presentation

import (
	"context"
	"testing"

	"github.com/izzyreal/ciwi/internal/domain"
)

type projectDetailsSourceStub struct{}

func (projectDetailsSourceStub) GetProjectDetails(context.Context, int64) (domain.ProjectDetails, error) {
	return domain.ProjectDetails{
		Project: domain.Project{
			ID: 1, Name: "ciwi", RepoRef: "main", ConfigFile: "ciwi-project.yaml",
			Pipelines:      []domain.Pipeline{{PipelineID: "build", SupportsDryRun: true}, {PipelineID: "prepare"}},
			PipelineChains: []domain.PipelineChain{{ID: "prepare+build", Name: "Build", Pipelines: []string{"prepare", "build"}}},
		},
		Pipelines: []domain.PipelineDetails{{
			ID: 2, PipelineID: "build", DependsOn: []string{"prepare"},
			Jobs: []domain.PipelineJobDetails{{
				ID: "compile", RunsOn: map[string]string{"os": "darwin", "arch": "arm64"},
				Steps: []domain.PipelineStepDetails{{Index: 0, Type: "run"}, {Index: 1, Type: "test", TestName: "unit", SkipDryRun: true}},
			}},
		}},
	}, nil
}

func TestProjectDetailsViewDerivesStableLabels(t *testing.T) {
	view, err := NewProjectDetailsQueries(projectDetailsSourceStub{}).GetProjectDetailsView(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	pipeline := view.Pipelines[0]
	if pipeline.Dependencies != "prepare" || !pipeline.SupportsDryRun || pipeline.SummaryLabel == "" || pipeline.GraphSummary == "" {
		t.Fatalf("pipeline = %+v", pipeline)
	}
	job := pipeline.Jobs[0]
	if job.RunsOnLabel != "arch=arm64, os=darwin" || job.NeedsLabel != "none" || job.SummaryLabel == "" || job.TimeoutLabel == "" || job.MatrixLabel == "" || job.Steps[0].Name != "step 1" || job.Steps[1].Name != "test unit" || !job.SupportsDryRun {
		t.Fatalf("job = %+v", job)
	}
	if view.ProjectLabels.PipelineCount != "2 pipelines" || view.ProjectLabels.SourceMetadata != "branch: main · ciwi-project.yaml" || !view.ProjectLabels.HasPipelineChains {
		t.Fatalf("project labels = %+v", view.ProjectLabels)
	}
	if len(view.StructureFilters) != 3 {
		t.Fatalf("structure filters = %+v", view.StructureFilters)
	}
	chain := view.StructureFilters[2]
	if chain.Value != "chain:prepare+build" || chain.Root.ChainID != "prepare+build" || !chain.Root.Runnable || len(chain.PipelineIDs) != 2 {
		t.Fatalf("chain structure filter = %+v", chain)
	}
}

func TestProjectExecutionCardsFiltersByStableProjectID(t *testing.T) {
	cards := []domain.ExecutionCard{
		{Key: "ciwi", Sections: []domain.ExecutionCardSection{{Jobs: []domain.ExecutionCardJob{{ProjectID: 41}}}}},
		{Key: "other", Sections: []domain.ExecutionCardSection{{Jobs: []domain.ExecutionCardJob{{ProjectID: 7}}}}},
	}
	filtered := projectExecutionCards(cards, 41)
	if len(filtered) != 1 || filtered[0].Key != "ciwi" {
		t.Fatalf("filtered project history = %#v", filtered)
	}
}

func TestPipelineChainSequenceLabel(t *testing.T) {
	if got := PipelineChainSequenceLabel([]string{"build", "codesign-macos", "release"}); got != "build → codesign-macos → release" {
		t.Fatalf("sequence label = %q", got)
	}
}
