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
              report: out/go-unit.json
              coverage_format: go-coverprofile
              coverage_report: out/go-unit.cover
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
	if step.Test.Name != "go-unit" || step.Test.Command != "go test -json ./..." || step.Test.Format != "go-test-json" || step.Test.Report != "out/go-unit.json" || step.Test.CoverageFormat != "go-coverprofile" || step.Test.CoverageReport != "out/go-unit.cover" {
		t.Fatalf("unexpected test step: %+v", step.Test)
	}
}

func TestParseTestStepJUnitXML(t *testing.T) {
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
              name: cpp-unit
              command: ./tests --reporter junit
              format: junit-xml
              report: out/junit.xml
`), "test-step-junit")
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	step := cfg.Pipelines[0].Jobs[0].Steps[0]
	if step.Test == nil {
		t.Fatal("expected test step to be parsed")
	}
	if step.Test.Format != "junit-xml" {
		t.Fatalf("unexpected test format: %q", step.Test.Format)
	}
}

func TestParseRejectsUnsupportedCoverageFormat(t *testing.T) {
	_, err := Parse([]byte(`
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
              command: go test -json ./...
              format: go-test-json
              report: out/go-unit.json
              coverage_format: cobertura-xml
              coverage_report: out/coverage.xml
`), "test-coverage-format")
	if err == nil || !strings.Contains(err.Error(), "coverage_format unsupported") {
		t.Fatalf("expected unsupported coverage_format error, got: %v", err)
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

func TestParseProjectVault(t *testing.T) {
	cfg, err := Parse([]byte(`
version: 1
project:
  name: ciwi
  vault:
    connection: home-vault
    secrets:
      - name: github-secret
        mount: kv
        path: gh
        key: token
        kv_version: 2
pipelines:
  - id: build
    jobs:
      - id: compile
        timeout_seconds: 60
        steps:
          - run: go build ./...
`), "test-vault")
	if err != nil {
		t.Fatalf("parse vault config: %v", err)
	}
	if cfg.Project.Vault == nil {
		t.Fatal("expected project.vault to be parsed")
	}
	if cfg.Project.Vault.Connection != "home-vault" {
		t.Fatalf("unexpected vault connection: %q", cfg.Project.Vault.Connection)
	}
	if len(cfg.Project.Vault.Secrets) != 1 || cfg.Project.Vault.Secrets[0].Name != "github-secret" {
		t.Fatalf("unexpected vault secrets: %+v", cfg.Project.Vault.Secrets)
	}
}

func TestParseRejectsInvalidProjectVault(t *testing.T) {
	_, err := Parse([]byte(`
version: 1
project:
  name: ciwi
  vault:
    connection: ""
    secrets:
      - name: github-secret
        path: gh
        key: token
pipelines: []
`), "test-invalid-vault")
	if err == nil || !strings.Contains(err.Error(), "project.vault.connection is required") {
		t.Fatalf("expected project.vault.connection error, got: %v", err)
	}
}

func TestParseRejectsInvalidYAML(t *testing.T) {
	_, err := Parse([]byte("version: ["), "test-yaml")
	if err == nil || !strings.Contains(err.Error(), "parse YAML") {
		t.Fatalf("expected parse YAML error, got: %v", err)
	}
}

func TestParseRejectsEmptyDependsOnEntry(t *testing.T) {
	_, err := Parse([]byte(`
version: 1
project:
  name: ciwi
pipelines:
  - id: release
    depends_on:
      - ""
    jobs:
      - id: publish
        timeout_seconds: 30
        steps:
          - run: echo publish
`), "test-depends-on")
	if err == nil || !strings.Contains(err.Error(), "depends_on") {
		t.Fatalf("expected depends_on validation error, got: %v", err)
	}
}

func TestParseRejectsUnknownFields(t *testing.T) {
	_, err := Parse([]byte(`
version: 1
project:
  name: ciwi
  extra_field: true
pipelines:
  - id: build
    jobs:
      - id: compile
        timeout_seconds: 60
        steps:
          - run: go build ./...
`), "test-unknown-fields")
	if err == nil || !strings.Contains(err.Error(), "field extra_field not found") {
		t.Fatalf("expected unknown field error, got: %v", err)
	}
}

func TestParseAcceptsRequiresCapabilities(t *testing.T) {
	cfg, err := Parse([]byte(`
version: 1
project:
  name: ciwi
pipelines:
  - id: build
    jobs:
      - id: linux
        timeout_seconds: 60
        requires:
          capabilities:
            xorg-dev: "1"
        steps:
          - run: echo build
`), "test-requires-capabilities")
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	got := cfg.Pipelines[0].Jobs[0].Requires.Capabilities["xorg-dev"]
	if got != "1" {
		t.Fatalf("expected requires.capabilities xorg-dev=1, got %q", got)
	}
}

func TestParseRejectsInvalidRequiresCapabilitiesName(t *testing.T) {
	_, err := Parse([]byte(`
version: 1
project:
  name: ciwi
pipelines:
  - id: build
    jobs:
      - id: linux
        timeout_seconds: 60
        requires:
          capabilities:
            "bad cap": "1"
        steps:
          - run: echo build
`), "test-invalid-requires-capabilities")
	if err == nil || !strings.Contains(err.Error(), "requires.capabilities") {
		t.Fatalf("expected requires.capabilities validation error, got: %v", err)
	}
}

func TestParseRejectsInvalidStepShape(t *testing.T) {
	_, err := Parse([]byte(`
version: 1
project:
  name: ciwi
pipelines:
  - id: build
    jobs:
      - id: compile
        timeout_seconds: 60
        steps:
          - run: echo hi
            test:
              command: go test ./...
`), "test-step-shape")
	if err == nil || !strings.Contains(err.Error(), "must set exactly one of run or test") {
		t.Fatalf("expected step shape validation error, got: %v", err)
	}
}

func TestParseRejectsMetadataStep(t *testing.T) {
	_, err := Parse([]byte(`
version: 1
project:
  name: ciwi
pipelines:
  - id: release
    jobs:
      - id: publish
        timeout_seconds: 60
        steps:
          - metadata:
              version: "{{ ciwi.version_raw }}"
              tag: "{{ ciwi.version }}"
`), "test-metadata-step")
	if err == nil || !strings.Contains(err.Error(), "field metadata not found") {
		t.Fatalf("expected metadata rejection, got: %v", err)
	}
}

func TestParseStepSkipDryRun(t *testing.T) {
	cfg, err := Parse([]byte(`
version: 1
project:
  name: ciwi
pipelines:
  - id: release
    jobs:
      - id: publish
        timeout_seconds: 60
        steps:
          - run: echo always
          - run: echo live-only
            skip_dry_run: true
`), "test-step-skip-dry-run")
	if err != nil {
		t.Fatalf("parse step skip_dry_run: %v", err)
	}
	steps := cfg.Pipelines[0].Jobs[0].Steps
	if len(steps) != 2 {
		t.Fatalf("unexpected step count: %d", len(steps))
	}
	if steps[0].SkipDryRun {
		t.Fatalf("expected first step skip_dry_run=false")
	}
	if !steps[1].SkipDryRun {
		t.Fatalf("expected second step skip_dry_run=true")
	}
}

func TestParseRejectsUnsupportedTestFormat(t *testing.T) {
	_, err := Parse([]byte(`
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
              command: ./tests
              format: tap
              report: out/tests.tap
`), "test-step-format")
	if err == nil || !strings.Contains(err.Error(), "test.format unsupported") {
		t.Fatalf("expected unsupported test.format error, got: %v", err)
	}
}

func TestParseRejectsMissingTestReport(t *testing.T) {
	_, err := Parse([]byte(`
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
              command: go test -json ./...
              format: go-test-json
`), "test-step-report")
	if err == nil || !strings.Contains(err.Error(), "test.report is required") {
		t.Fatalf("expected missing test.report error, got: %v", err)
	}
}

func TestParseRejectsDuplicatePipelineAndUnknownDependency(t *testing.T) {
	_, err := Parse([]byte(`
version: 1
project:
  name: ciwi
pipelines:
  - id: build
    jobs:
      - id: compile
        timeout_seconds: 60
        steps:
          - run: go build ./...
  - id: build
    depends_on:
      - release
    jobs:
      - id: publish
        timeout_seconds: 60
        steps:
          - run: echo publish
`), "test-dup-pipeline")
	if err == nil || !strings.Contains(err.Error(), "duplicate") || !strings.Contains(err.Error(), "unknown pipeline") {
		t.Fatalf("expected duplicate/unknown dependency errors, got: %v", err)
	}
}

func TestParsePipelineVersioning(t *testing.T) {
	cfg, err := Parse([]byte(`
version: 1
project:
  name: ciwi
pipelines:
  - id: release
    source:
      repo: https://github.com/izzyreal/ciwi.git
    versioning:
      file: VERSION
      tag_prefix: v
      auto_bump: patch
    jobs:
      - id: publish
        timeout_seconds: 60
        steps:
          - run: echo publish
`), "test-versioning")
	if err != nil {
		t.Fatalf("parse versioning config: %v", err)
	}
	if len(cfg.Pipelines) != 1 || cfg.Pipelines[0].Versioning == nil {
		t.Fatalf("expected pipeline versioning to be parsed")
	}
	if got := cfg.Pipelines[0].Versioning.AutoBump; got != "patch" {
		t.Fatalf("unexpected auto_bump: %q", got)
	}
}

func TestParseRejectsInvalidAutoBumpMode(t *testing.T) {
	_, err := Parse([]byte(`
version: 1
project:
  name: ciwi
pipelines:
  - id: release
    versioning:
      auto_bump: banana
    jobs:
      - id: publish
        timeout_seconds: 60
        steps:
          - run: echo publish
`), "test-invalid-auto-bump")
	if err == nil || !strings.Contains(err.Error(), "auto_bump") {
		t.Fatalf("expected auto_bump validation error, got: %v", err)
	}
}

func TestParseRejectsInvalidExecutorAndMissingShell(t *testing.T) {
	_, err := Parse([]byte(`
version: 1
project:
  name: ciwi
pipelines:
  - id: build
    jobs:
      - id: compile
        runs_on:
          executor: shell
        timeout_seconds: 60
        steps:
          - run: go build ./...
`), "test-invalid-executor")
	if err == nil || !strings.Contains(err.Error(), `runs_on.executor must be "script"`) {
		t.Fatalf("expected runs_on.executor validation error, got: %v", err)
	}
}

func TestParseRejectsShellWithoutScriptExecutor(t *testing.T) {
	_, err := Parse([]byte(`
version: 1
project:
  name: ciwi
pipelines:
  - id: build
    jobs:
      - id: compile
        runs_on:
          shell: posix
        timeout_seconds: 60
        steps:
          - run: go build ./...
`), "test-shell-without-script-executor")
	if err == nil || !strings.Contains(err.Error(), `runs_on.executor must be "script" when runs_on.shell is set`) {
		t.Fatalf("expected runs_on shell/executor validation error, got: %v", err)
	}
}

func TestParseRejectsScriptExecutorWithoutShell(t *testing.T) {
	_, err := Parse([]byte(`
version: 1
project:
  name: ciwi
pipelines:
  - id: build
    jobs:
      - id: compile
        runs_on:
          executor: script
        timeout_seconds: 60
        steps:
          - run: go build ./...
`), "test-script-executor-without-shell")
	if err == nil || !strings.Contains(err.Error(), `runs_on.shell is required when runs_on.executor=script`) {
		t.Fatalf("expected runs_on.shell required validation error, got: %v", err)
	}
}

func TestParseAcceptsPowerShellShell(t *testing.T) {
	_, err := Parse([]byte(`
version: 1
project:
  name: ciwi
pipelines:
  - id: win
    jobs:
      - id: smoke
        runs_on:
          executor: script
          shell: powershell
        timeout_seconds: 60
        steps:
          - run: Write-Host "ok"
`), "test-powershell-shell")
	if err != nil {
		t.Fatalf("expected powershell shell to validate, got: %v", err)
	}
}

func TestParseJobCaches(t *testing.T) {
	_, err := Parse([]byte(`
version: 1
project:
  name: ciwi
pipelines:
  - id: build
    jobs:
      - id: compile
        runs_on:
          executor: script
          shell: posix
        timeout_seconds: 60
        caches:
          - id: fetchcontent
            env: CIWI_FETCHCONTENT_SOURCES_DIR
        steps:
          - run: cmake -S . -B build
`), "test-caches")
	if err != nil {
		t.Fatalf("expected caches config to validate, got: %v", err)
	}
}

func TestParseRejectsInvalidJobCache(t *testing.T) {
	_, err := Parse([]byte(`
version: 1
project:
  name: ciwi
pipelines:
  - id: build
    jobs:
      - id: compile
        runs_on:
          executor: script
          shell: posix
        timeout_seconds: 60
        caches:
          - id: ""
            env: ""
        steps:
          - run: cmake -S . -B build
`), "test-invalid-cache")
	if err == nil || !strings.Contains(err.Error(), "caches") {
		t.Fatalf("expected cache validation error, got: %v", err)
	}
}

func TestParsePipelineChainsValidation(t *testing.T) {
	_, err := Parse([]byte(`
version: 1
project:
  name: ciwi
pipelines:
  - id: build
    jobs:
      - id: compile
        runs_on:
          executor: script
          shell: posix
        timeout_seconds: 60
        steps:
          - run: go build ./...
pipeline_chains:
  - id: build-release
    pipelines:
      - build
      - release
`), "test-pipeline-chains")
	if err == nil || !strings.Contains(err.Error(), `references unknown pipeline "release"`) {
		t.Fatalf("expected pipeline chain validation error, got: %v", err)
	}
}
