package config

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestRepositoryCiwiProjectConfigurationIsValid(t *testing.T) {
	configuration, err := Load(filepath.Join("..", "..", "ciwi-project.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if configuration.Project.Name != "ciwi" {
		t.Fatalf("project name = %q", configuration.Project.Name)
	}
	pipelineByID := func(id string) Pipeline {
		t.Helper()
		for _, pipeline := range configuration.Pipelines {
			if pipeline.ID == id {
				return pipeline
			}
		}
		t.Fatalf("pipeline %q not found", id)
		return Pipeline{}
	}
	jobByID := func(pipeline Pipeline, id string) PipelineJobSpec {
		t.Helper()
		for _, job := range pipeline.Jobs {
			if job.ID == id {
				return job
			}
		}
		t.Fatalf("job %q not found in pipeline %q", id, pipeline.ID)
		return PipelineJobSpec{}
	}

	windowsPackage := pipelineByID("package-windows")
	windowsInstaller := jobByID(windowsPackage, "windows-installer")
	if !slices.Contains(windowsPackage.DependsOn, "build") || len(windowsInstaller.ArtifactSources) != 1 || windowsInstaller.ArtifactSources[0].Job != "desktop-windows-amd64" {
		t.Fatalf("unexpected Windows packaging graph: %+v %+v", windowsPackage.DependsOn, windowsInstaller.ArtifactSources)
	}
	if _, hasGo := windowsInstaller.Requires.Tools["go"]; hasGo || windowsInstaller.GoCache != nil {
		t.Fatalf("Windows packaging must not require Go: %+v", windowsInstaller)
	}

	linuxPackage := pipelineByID("package-linux")
	linuxArchive := jobByID(linuxPackage, "linux-archive")
	if !slices.Contains(linuxPackage.DependsOn, "build") || len(linuxArchive.ArtifactSources) != 1 || linuxArchive.ArtifactSources[0].Job != "desktop-linux-amd64" {
		t.Fatalf("unexpected Linux packaging graph: %+v %+v", linuxPackage.DependsOn, linuxArchive.ArtifactSources)
	}
}
