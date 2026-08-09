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
	build := pipelineByID("build")
	integrationTests := jobByID(build, "integration-tests")
	buildCrossPlatform := jobByID(build, "build-cross-platform")
	if integrationTests.RunsOn["os"] != "linux" || integrationTests.RunsOn["arch"] != "amd64" || integrationTests.Requires.Tools["docker"] != "*" {
		t.Fatalf("unexpected browser integration runtime: %+v", integrationTests)
	}
	if !slices.Contains(buildCrossPlatform.Needs, "unit-tests") || !slices.Contains(buildCrossPlatform.Needs, "integration-tests") {
		t.Fatalf("cross-platform build dependencies = %+v, want both test jobs", buildCrossPlatform.Needs)
	}

	windowsPackage := pipelineByID("package-windows")
	windowsInstaller := jobByID(windowsPackage, "windows-installer")
	if !slices.Contains(windowsPackage.DependsOn, "build-desktop") || len(windowsInstaller.ArtifactSources) != 1 || windowsInstaller.ArtifactSources[0].Pipeline != "build-desktop" || windowsInstaller.ArtifactSources[0].Job != "windows-amd64" {
		t.Fatalf("unexpected Windows packaging graph: %+v %+v", windowsPackage.DependsOn, windowsInstaller.ArtifactSources)
	}
	if _, hasGo := windowsInstaller.Requires.Tools["go"]; hasGo || windowsInstaller.GoCache != nil {
		t.Fatalf("Windows packaging must not require Go: %+v", windowsInstaller)
	}

	linuxPackage := pipelineByID("package-linux")
	linuxArchive := jobByID(linuxPackage, "linux-archive")
	if !slices.Contains(linuxPackage.DependsOn, "build-desktop") || len(linuxArchive.ArtifactSources) != 1 || linuxArchive.ArtifactSources[0].Pipeline != "build-desktop" || linuxArchive.ArtifactSources[0].Job != "linux-amd64" {
		t.Fatalf("unexpected Linux packaging graph: %+v %+v", linuxPackage.DependsOn, linuxArchive.ArtifactSources)
	}

	desktopBuild := pipelineByID("build-desktop")
	jobByID(desktopBuild, "macos-unsigned")
	jobByID(desktopBuild, "windows-amd64")
	jobByID(desktopBuild, "linux-amd64")
	desktopSign := pipelineByID("codesign-desktop-macos")
	if !slices.Contains(desktopSign.DependsOn, "build-desktop") {
		t.Fatalf("unexpected macOS desktop signing dependencies: %+v", desktopSign.DependsOn)
	}

	iosBuild := pipelineByID("ios")
	iosArchive := jobByID(iosBuild, "ios-archive")
	if iosArchive.RunsOn["os"] != "darwin" || iosArchive.RunsOn["arch"] != "arm64" || iosArchive.GoCache == nil {
		t.Fatalf("unexpected iOS archive runtime: %+v", iosArchive)
	}
	iosRelease := pipelineByID("release-ios")
	testFlight := jobByID(iosRelease, "testflight")
	if !slices.Contains(iosRelease.DependsOn, "ios") || len(testFlight.ArtifactSources) != 1 || testFlight.ArtifactSources[0].Pipeline != "ios" || testFlight.ArtifactSources[0].Job != "ios-archive" {
		t.Fatalf("unexpected TestFlight publishing graph: %+v %+v", iosRelease.DependsOn, testFlight.ArtifactSources)
	}
	githubRelease := pipelineByID("release")
	if !slices.Contains(githubRelease.DependsOn, "release-ios") {
		t.Fatalf("GitHub release must wait for TestFlight upload: %+v", githubRelease.DependsOn)
	}
	coreRelease := pipelineByID("release-core")
	coreReleaseJob := jobByID(coreRelease, "github-release")
	if !slices.Equal(coreRelease.DependsOn, []string{"build", "codesign-macos-agents"}) {
		t.Fatalf("core release dependencies = %+v", coreRelease.DependsOn)
	}
	for _, source := range coreReleaseJob.ArtifactSources {
		if source.Pipeline == "build-desktop" || source.Pipeline == "package-macos" || source.Pipeline == "package-windows" || source.Pipeline == "package-linux" || source.Pipeline == "ios" || source.Pipeline == "release-ios" {
			t.Fatalf("core release includes native-client artifact source: %+v", source)
		}
	}

	wantDesktopChain := []string{"build-desktop", "codesign-desktop-macos", "package-macos", "package-windows", "package-linux"}
	foundDesktopChain := false
	for _, chain := range configuration.PipelineChains {
		if chain.Name == "Build desktop clients" {
			foundDesktopChain = true
			if !slices.Equal(chain.Pipelines, wantDesktopChain) {
				t.Fatalf("desktop chain = %+v, want %+v", chain.Pipelines, wantDesktopChain)
			}
		}
	}
	if !foundDesktopChain {
		t.Fatal("Build desktop clients chain not found")
	}

	wantIOSChain := []string{"ios", "release-ios"}
	foundIOSChain := false
	wantCoreReleaseChain := []string{"build", "codesign-macos-agents", "release-core"}
	foundCoreReleaseChain := false
	wantFullReleasePipelines := []string{"ios", "release-ios"}
	for _, chain := range configuration.PipelineChains {
		switch chain.Name {
		case "Build and publish iOS client":
			foundIOSChain = true
			if !slices.Equal(chain.Pipelines, wantIOSChain) {
				t.Fatalf("iOS chain = %+v, want %+v", chain.Pipelines, wantIOSChain)
			}
		case "Build and release":
			for _, pipelineID := range wantFullReleasePipelines {
				if !slices.Contains(chain.Pipelines, pipelineID) {
					t.Fatalf("full release chain does not include %q: %+v", pipelineID, chain.Pipelines)
				}
			}
		case "Build and release without native clients":
			foundCoreReleaseChain = true
			if !slices.Equal(chain.Pipelines, wantCoreReleaseChain) {
				t.Fatalf("core release chain = %+v, want %+v", chain.Pipelines, wantCoreReleaseChain)
			}
		}
	}
	if !foundIOSChain {
		t.Fatal("Build and publish iOS client chain not found")
	}
	if !foundCoreReleaseChain {
		t.Fatal("Build and release without native clients chain not found")
	}
}
