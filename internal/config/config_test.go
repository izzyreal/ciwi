package config

import (
	"strings"
	"testing"
)

func TestParseValidConfig(t *testing.T) {
	cfg, err := Parse([]byte(`
version: 1
project:
  name: ciwi
pipelines:
  - id: build
    trigger: manual
    jobs:
      - id: compile
        timeout_seconds: 60
        steps:
          - run: go build ./...
`), "test-valid")
	if err != nil {
		t.Fatalf("parse valid config: %v", err)
	}
	if cfg.Project.Name != "ciwi" {
		t.Fatalf("unexpected project name: %q", cfg.Project.Name)
	}
	if len(cfg.Pipelines) != 1 || cfg.Pipelines[0].ID != "build" {
		t.Fatalf("unexpected pipelines: %+v", cfg.Pipelines)
	}
}

func TestParseTestStep(t *testing.T) {
	cfg, err := Parse([]byte(`
version: 1
project:
  name: ciwi
pipelines:
  - id: test
    jobs:
      - id: unit
        timeout_seconds: 60
        steps:
          - test:
              name: go-unit
              command: go test -json ./...
              format: go-test-json
`), "test-step")
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if len(cfg.Pipelines) != 1 || len(cfg.Pipelines[0].Jobs) != 1 || len(cfg.Pipelines[0].Jobs[0].Steps) != 1 {
		t.Fatalf("unexpected parsed structure")
	}
	step := cfg.Pipelines[0].Jobs[0].Steps[0]
	if step.Test == nil {
		t.Fatal("expected test step to be parsed")
	}
	if step.Test.Name != "go-unit" || step.Test.Command != "go test -json ./..." || step.Test.Format != "go-test-json" {
		t.Fatalf("unexpected test step: %+v", step.Test)
	}
}

func TestParseRejectsUnsupportedVersion(t *testing.T) {
	_, err := Parse([]byte(`
version: 2
project:
  name: ciwi
`), "test-version")
	if err == nil || !strings.Contains(err.Error(), "unsupported config version") {
		t.Fatalf("expected unsupported version error, got: %v", err)
	}
}

func TestParseRejectsMissingProjectName(t *testing.T) {
	_, err := Parse([]byte(`
version: 1
project:
  name: ""
`), "test-project-name")
	if err == nil || !strings.Contains(err.Error(), "project.name is required") {
		t.Fatalf("expected missing project.name error, got: %v", err)
	}
}

func TestParseRejectsInvalidYAML(t *testing.T) {
	_, err := Parse([]byte("version: ["), "test-yaml")
	if err == nil || !strings.Contains(err.Error(), "parse YAML") {
		t.Fatalf("expected parse YAML error, got: %v", err)
	}
}
