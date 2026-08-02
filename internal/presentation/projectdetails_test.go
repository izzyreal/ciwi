package presentation

import (
	"context"
	"testing"

	"github.com/izzyreal/ciwi/internal/domain"
)

type projectDetailsSourceStub struct{}

func (projectDetailsSourceStub) GetProjectDetails(context.Context, int64) (domain.ProjectDetails, error) {
	return domain.ProjectDetails{
		Project: domain.Project{ID: 1, Name: "ciwi", Pipelines: []domain.Pipeline{{PipelineID: "build", SupportsDryRun: true}}},
		Pipelines: []domain.PipelineDetails{{
			ID: 2, PipelineID: "build", DependsOn: []string{"prepare"},
			Jobs: []domain.PipelineJobDetails{{
				ID: "compile", RunsOn: map[string]string{"os": "darwin", "arch": "arm64"},
				Steps: []domain.PipelineStepDetails{{Index: 0, Type: "run"}, {Index: 1, Type: "test", TestName: "unit"}},
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
	if pipeline.Dependencies != "prepare" || !pipeline.SupportsDryRun {
		t.Fatalf("pipeline = %+v", pipeline)
	}
	job := pipeline.Jobs[0]
	if job.RunsOnLabel != "arch=arm64, os=darwin" || job.Steps[0].Name != "step 1" || job.Steps[1].Name != "test unit" {
		t.Fatalf("job = %+v", job)
	}
}
