package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/izzyreal/ciwi/internal/domain"
	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/store"
)

func TestProjectProtocolMappingsPreserveNestedDefinitions(t *testing.T) {
	summary := projectFromProtocol(protocol.ProjectSummary{
		ID: 3, Name: "ciwi", RepoURL: "https://example.com/ciwi.git",
		Pipelines:      []protocol.PipelineSummary{{ID: 7, PipelineID: "build", DependsOn: []string{"test"}, SupportsDryRun: true}},
		PipelineChains: []protocol.PipelineChainSummary{{ID: "release", Name: "Build and release", Pipelines: []string{"build", "release"}, SupportsDryRun: true}},
	})
	if summary.ID != 3 || len(summary.Pipelines) != 1 || !summary.Pipelines[0].SupportsDryRun || len(summary.PipelineChains) != 1 {
		t.Fatalf("project = %#v", summary)
	}
	summary.Pipelines[0].DependsOn[0] = "changed"
	summary.PipelineChains[0].Pipelines[0] = "changed"

	details := projectDetailsFromProtocol(protocol.ProjectDetail{
		ID: 3, Name: "ciwi",
		Pipelines: []protocol.PipelineDetail{{
			ID: 7, PipelineID: "build", Jobs: []protocol.PipelineJobDetail{{
				ID: "unit-tests", Needs: []string{"prepare"}, RunsOn: map[string]string{"os": "linux"},
				RequiresTools: map[string]string{"go": ">=1.24"}, MatrixIncludes: []protocol.MatrixInclude{{Name: "amd64"}},
				Steps: []protocol.PipelineStep{
					{Type: "run", Name: "Build", Run: "go build ./...", Env: map[string]string{"CGO_ENABLED": "0"}},
					{Type: "test", Name: "Test", TestCommand: "go test ./...", SkipDryRun: true},
				},
			}},
		}},
		PipelineChains: []protocol.PipelineChainSummary{{ID: "release", Pipelines: []string{"build"}}},
	})
	if len(details.Pipelines) != 1 || len(details.Pipelines[0].Jobs) != 1 || len(details.Pipelines[0].Jobs[0].Steps) != 2 {
		t.Fatalf("details = %#v", details)
	}
	job := details.Pipelines[0].Jobs[0]
	if job.MatrixCount != 1 || job.Steps[0].Command != "go build ./..." || job.Steps[1].Command != "go test ./..." {
		t.Fatalf("job = %#v", job)
	}
	if !details.Project.Pipelines[0].SupportsDryRun {
		t.Fatal("skip-dry-run step did not mark pipeline as supporting dry run")
	}
	job.RunsOn["os"] = "changed"
	if got := cloneStringMap(nil); got == nil || len(got) != 0 {
		t.Fatalf("nil clone = %#v", got)
	}
}

func TestProjectRepositoryHonorsCancellationAndNotFound(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ciwi.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repository := NewProjectRepository(db)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := repository.ListProjects(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("list error = %v", err)
	}
	if _, err := repository.GetProjectDetails(context.Background(), 999); !errors.Is(err, domain.ErrProjectNotFound) {
		t.Fatalf("details error = %v", err)
	}
}
