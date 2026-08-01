package server

import (
	"strings"
	"testing"

	"github.com/izzyreal/ciwi/internal/config"
	"github.com/izzyreal/ciwi/internal/store"
)

func TestRenderInspectableRawYAMLSelectsAndValidatesJobs(t *testing.T) {
	pipeline := store.PersistedPipeline{
		PipelineID: "release",
		Trigger:    "manual",
		SourceRepo: " https://example.test/repo.git ",
		SourceRef:  " main ",
		Jobs: []store.PersistedPipelineJob{
			{ID: "package", Position: 2, Steps: []config.PipelineJobStep{{Run: "package"}}},
			{ID: "build", Position: 1, MatrixInclude: []map[string]string{{"name": "linux"}, {"name": "macos"}}, Steps: []config.PipelineJobStep{{Run: "build"}}},
		},
	}

	content, title, err := renderInspectableRawYAML(pipeline, projectInspectRequest{})
	if err != nil {
		t.Fatalf("render pipeline yaml: %v", err)
	}
	if title != "Pipeline release YAML" || !strings.Contains(content, "id: release") || strings.Index(content, "id: build") > strings.Index(content, "id: package") {
		t.Fatalf("unexpected pipeline rendering title=%q content=%s", title, content)
	}

	idx := 1
	content, title, err = renderInspectableRawYAML(pipeline, projectInspectRequest{PipelineJobID: " build ", MatrixIndex: &idx})
	if err != nil {
		t.Fatalf("render selected job yaml: %v", err)
	}
	if title != "Job build YAML" || !strings.Contains(content, "name: macos") || strings.Contains(content, "name: linux") {
		t.Fatalf("unexpected selected matrix rendering title=%q content=%s", title, content)
	}

	badIndex := 2
	if _, _, err := renderInspectableRawYAML(pipeline, projectInspectRequest{PipelineJobID: "build", MatrixIndex: &badIndex}); err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Fatalf("expected matrix bounds error, got %v", err)
	}
	if _, _, err := renderInspectableRawYAML(pipeline, projectInspectRequest{PipelineJobID: "missing"}); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected missing job error, got %v", err)
	}
}

func TestInspectableConversionDeepCopiesStoredConfiguration(t *testing.T) {
	stored := store.PersistedPipelineJob{
		ID:              "build",
		Needs:           []string{"prepare"},
		RunsOn:          map[string]string{"os": "linux"},
		ArtifactSources: []config.PipelineJobArtifactSource{{Pipeline: "prepare", Matrix: map[string]string{"arch": "amd64"}}},
		MatrixInclude:   []map[string]string{{"name": "linux"}},
		Steps: []config.PipelineJobStep{{
			Run: "build", Env: map[string]string{"MODE": "release"},
			Vault: &config.StepVault{Connection: "vault", Secrets: []config.StepVaultSecretRef{{Name: "TOKEN", Path: "ciwi", Key: "token"}}},
		}},
	}
	converted := persistedPipelineJobToConfigJob(stored)
	converted.Needs[0] = "changed"
	converted.RunsOn["os"] = "changed"
	converted.ArtifactSources[0].Matrix["arch"] = "changed"
	converted.Matrix.Include[0]["name"] = "changed"
	converted.Steps[0].Env["MODE"] = "changed"
	converted.Steps[0].Vault.Secrets[0].Name = "changed"

	if stored.Needs[0] != "prepare" || stored.RunsOn["os"] != "linux" || stored.ArtifactSources[0].Matrix["arch"] != "amd64" || stored.MatrixInclude[0]["name"] != "linux" || stored.Steps[0].Env["MODE"] != "release" || stored.Steps[0].Vault.Secrets[0].Name != "TOKEN" {
		t.Fatalf("inspection conversion mutated stored configuration: %+v", stored)
	}
}

func TestRenderInspectableExecutorScript(t *testing.T) {
	if got := renderInspectableExecutorScript(nil); got != "" {
		t.Fatalf("expected empty script, got %q", got)
	}
	if got := renderInspectableExecutorScript([]pendingJob{{script: "  echo one\n"}}); got != "echo one" {
		t.Fatalf("unexpected single script: %q", got)
	}
	got := renderInspectableExecutorScript([]pendingJob{
		{pipelineJobID: "build", metadata: map[string]string{"matrix_name": "linux-amd64"}, script: "go build"},
		{pipelineJobID: "package", metadata: map[string]string{}, script: ""},
	})
	for _, want := range []string{"# build / linux-amd64", "go build", "---", "# package", "# <empty>"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected multi-script rendering to contain %q: %s", want, got)
		}
	}
}
