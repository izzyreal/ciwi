package config

import (
	"errors"
	"strings"
	"testing"
)

func TestExpandIncludesRecursivelyPreservesYAMLSemantics(t *testing.T) {
	files := map[string]string{
		"fragments/pipelines.yaml": `- id: build
  jobs:
    !include jobs/build.yaml
- id: release
  jobs:
    - id: package
      runs_on: *linux
      steps:
        - run: echo release`,
		"fragments/jobs/build.yaml": `- id: compile
  runs_on: &linux
    os: linux
  steps:
    - run: |
        echo before
        !include ../../scripts/build.sh
`,
		"scripts/build.sh": "echo one\necho two",
	}
	root := []byte(`version: 1
project:
  name: split
pipelines:
  !include fragments/pipelines.yaml
`)

	expanded, err := ExpandIncludes(root, "ciwi-project.yaml", mapIncludeReader(files))
	if err != nil {
		t.Fatalf("expand includes: %v", err)
	}
	if strings.Contains(string(expanded), "!include") {
		t.Fatalf("expanded YAML still contains a directive:\n%s", expanded)
	}

	cfg, err := Parse(expanded, "expanded ciwi-project.yaml")
	if err != nil {
		t.Fatalf("parse expanded config: %v\n%s", err, expanded)
	}
	if len(cfg.Pipelines) != 2 || cfg.Pipelines[0].ID != "build" || cfg.Pipelines[1].ID != "release" {
		t.Fatalf("unexpected pipelines: %+v", cfg.Pipelines)
	}
	buildJob := cfg.Pipelines[0].Jobs[0]
	releaseJob := cfg.Pipelines[1].Jobs[0]
	if buildJob.RunsOn["os"] != "linux" || releaseJob.RunsOn["os"] != "linux" {
		t.Fatalf("cross-file YAML anchor was not preserved: build=%v release=%v", buildJob.RunsOn, releaseJob.RunsOn)
	}
	wantRun := "echo before\necho one\necho two\n"
	if got := buildJob.Steps[0].Run; got != wantRun {
		t.Fatalf("included block scalar = %q, want %q", got, wantRun)
	}
}

func TestExpandIncludesAllowsRepeatedNonCyclicFiles(t *testing.T) {
	root := []byte("items:\n  !include fragments/items.yaml\n  !include fragments/items.yaml")
	files := map[string]string{"fragments/items.yaml": "- one\n- two"}

	expanded, err := ExpandIncludes(root, "ciwi-project.yaml", mapIncludeReader(files))
	if err != nil {
		t.Fatalf("expand repeated include: %v", err)
	}
	want := "items:\n  - one\n  - two\n  - one\n  - two"
	if got := string(expanded); got != want {
		t.Fatalf("expanded content = %q, want %q", got, want)
	}
}

func TestExpandIncludesRejectsCycleWithChain(t *testing.T) {
	files := map[string]string{
		"parts/a.yaml": "!include b.yaml\n",
		"parts/b.yaml": "!include a.yaml\n",
	}
	_, err := ExpandIncludes([]byte("!include parts/a.yaml\n"), "ciwi-project.yaml", mapIncludeReader(files))
	if err == nil {
		t.Fatal("expected include cycle error")
	}
	for _, want := range []string{"parts/b.yaml:1", "ciwi-project.yaml -> parts/a.yaml -> parts/b.yaml -> parts/a.yaml"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("cycle error %q does not contain %q", err, want)
		}
	}
}

func TestExpandIncludesReportsMissingFileAndSourceLine(t *testing.T) {
	root := []byte("version: 1\n!include missing.yaml\n")
	_, err := ExpandIncludes(root, "ciwi-project.yaml", func(string) ([]byte, error) {
		return nil, errors.New("not found")
	})
	if err == nil || !strings.Contains(err.Error(), "missing.yaml") || !strings.Contains(err.Error(), "ciwi-project.yaml:2") || !strings.Contains(err.Error(), "ciwi-project.yaml -> missing.yaml") {
		t.Fatalf("unexpected missing include error: %v", err)
	}
}

func TestExpandIncludesRejectsInvalidDirectivesAndPaths(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{name: "missing path", line: "!include", want: "expected !include"},
		{name: "path with whitespace", line: "!include two files.yaml", want: "whitespace-free path"},
		{name: "absolute path", line: "!include /tmp/file.yaml", want: "must be relative"},
		{name: "repository escape", line: "!include ../file.yaml", want: "escapes the repository"},
		{name: "backslash", line: `!include parts\file.yaml`, want: "forward slashes"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ExpandIncludes([]byte(tc.line+"\n"), "ciwi-project.yaml", func(string) ([]byte, error) {
				t.Fatal("reader must not be called for an invalid directive")
				return nil, nil
			})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func mapIncludeReader(files map[string]string) IncludeReader {
	return func(repoPath string) ([]byte, error) {
		content, ok := files[repoPath]
		if !ok {
			return nil, errors.New("not found")
		}
		return []byte(content), nil
	}
}
