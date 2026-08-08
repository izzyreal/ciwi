package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/izzyreal/ciwi/internal/config"
	"github.com/izzyreal/ciwi/internal/store"
)

func TestResolvePipelineRunContextUsesInheritedVersionAndMatchingSource(t *testing.T) {
	pipeline := store.PersistedPipeline{
		SourceRepo: " https://example.test/repo.git ",
		SourceRef:  "main",
		Versioning: config.PipelineVersioning{
			AutoBump:         "patch",
			AutoBumpVCSToken: " token ",
			AutoBumpVault: &config.StepVault{
				Connection: " release-vault ",
				Secrets: []config.StepVaultSecretRef{{
					Name: " vcs-token ", Mount: " secret ", Path: " ciwi/release ", Key: " token ", KVVersion: 2,
				}},
			},
		},
	}
	dependency := pipelineDependencyContext{
		VersionRaw:        "1.2.3",
		Version:           "v1.2.3",
		SourceRepo:        "https://example.test/repo.git",
		SourceRefRaw:      " refs/heads/release ",
		SourceRefResolved: "abc123",
	}

	got, err := resolvePipelineRunContext(pipeline, dependency)
	if err != nil {
		t.Fatalf("resolve inherited context: %v", err)
	}
	if got.Version != "v1.2.3" || got.VersionRaw != "1.2.3" || got.SourceRefRaw != "refs/heads/release" || got.SourceRefResolved != "abc123" {
		t.Fatalf("unexpected inherited context: %+v", got)
	}
	if got.VersionFile != "VERSION" || got.TagPrefix != "v" || got.AutoBump != "patch" || got.AutoBumpVCSToken != "token" || got.AutoBumpVaultConn != "release-vault" {
		t.Fatalf("expected versioning defaults and auto-bump settings, got %+v", got)
	}
	if len(got.AutoBumpSecrets) != 1 || got.AutoBumpSecrets[0].Name != "vcs-token" || got.AutoBumpSecrets[0].KVVersion != 2 {
		t.Fatalf("unexpected mapped vault secrets: %+v", got.AutoBumpSecrets)
	}

	pipeline.SourceRepo = "https://example.test/other.git"
	got, err = resolvePipelineRunContext(pipeline, dependency)
	if err != nil {
		t.Fatalf("resolve inherited context from other repo: %v", err)
	}
	if got.SourceRefRaw != "main" || got.SourceRefResolved != "" {
		t.Fatalf("source ref must not cross repository boundaries: %+v", got)
	}
}

func TestResolvePipelineRunContextInheritsMatchingSourceWithoutVersioning(t *testing.T) {
	pipeline := store.PersistedPipeline{
		SourceRepo: "https://example.test/repo.git",
		SourceRef:  "main",
	}
	dependency := pipelineDependencyContext{
		SourceRepo:        "https://example.test/repo.git",
		SourceRefRaw:      "release",
		SourceRefResolved: "0123456789abcdef0123456789abcdef01234567",
	}

	got, err := resolvePipelineRunContext(pipeline, dependency)
	if err != nil {
		t.Fatalf("resolve inherited source: %v", err)
	}
	if got.SourceRefRaw != "release" || got.SourceRefResolved != dependency.SourceRefResolved {
		t.Fatalf("unversioned pipeline did not inherit pinned source: %+v", got)
	}

	pipeline.SourceRepo = "https://example.test/other.git"
	got, err = resolvePipelineRunContext(pipeline, dependency)
	if err != nil {
		t.Fatalf("resolve source from other repo: %v", err)
	}
	if got.SourceRefRaw != "main" || got.SourceRefResolved != "" {
		t.Fatalf("source ref must not cross repository boundaries: %+v", got)
	}
}

func TestResolvePipelineRunContextOptionalAndRequiredFailures(t *testing.T) {
	var reports []string
	report := func(step, status, message string) {
		reports = append(reports, step+":"+status+":"+message)
	}
	plain := store.PersistedPipeline{SourceRef: " main "}
	got, err := resolvePipelineRunContextWithReporter(plain, pipelineDependencyContext{}, report)
	if err != nil || got.SourceRefRaw != "main" {
		t.Fatalf("unversioned pipeline should retain raw ref without failing: got=%+v err=%v", got, err)
	}
	if len(reports) != 1 || !strings.Contains(reports[0], "not configured") {
		t.Fatalf("unexpected unversioned report: %v", reports)
	}

	reports = nil
	optional := store.PersistedPipeline{
		SourceRepo: filepath.Join(t.TempDir(), "missing.git"),
		Versioning: config.PipelineVersioning{File: "VERSION"},
	}
	got, err = resolvePipelineRunContextWithReporter(optional, pipelineDependencyContext{}, report)
	if err != nil || got.Version != "" || got.VersionRaw != "" || got.SourceRefRaw != "" || got.VersionFile != "" || len(got.AutoBumpSecrets) != 0 {
		t.Fatalf("optional version resolution should degrade to an empty context: got=%+v err=%v", got, err)
	}
	if len(reports) < 2 || !strings.Contains(reports[len(reports)-1], "version not resolved") {
		t.Fatalf("expected optional failure report, got %v", reports)
	}

	optional.Versioning.AutoBump = "patch"
	if _, err := resolvePipelineRunContextWithReporter(optional, pipelineDependencyContext{}, report); err == nil {
		t.Fatalf("auto-bump must fail when the source version cannot be resolved")
	}
}

func TestReadVersionFromRepoValidatesFileAndReportsProgress(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.name", "ciwi-test")
	runGit(t, repo, "config", "user.email", "ciwi-test@local")
	if err := os.WriteFile(filepath.Join(repo, "VERSION"), []byte(" 2.4.6\n"), 0o644); err != nil {
		t.Fatalf("write version: %v", err)
	}
	runGit(t, repo, "add", "VERSION")
	runGit(t, repo, "commit", "-m", "version")
	expectedSHA := strings.TrimSpace(runGit(t, repo, "rev-parse", "HEAD"))

	var reports []string
	raw, sha, err := readVersionFromRepo(repo, "", "VERSION", func(step, status, message string) {
		reports = append(reports, step+":"+status+":"+message)
	})
	if err != nil {
		t.Fatalf("read version: %v", err)
	}
	if raw != "2.4.6" || sha != expectedSHA {
		t.Fatalf("unexpected resolved version raw=%q sha=%q", raw, sha)
	}
	if len(reports) < 4 || !strings.Contains(strings.Join(reports, "\n"), "version-file:ok") {
		t.Fatalf("expected checkout and validation progress, got %v", reports)
	}

	if _, _, err := readVersionFromRepo(repo, "", "../VERSION", nil); err == nil || !strings.Contains(err.Error(), "invalid versioning.file") {
		t.Fatalf("expected unsafe version path to fail, got %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "BAD_VERSION"), []byte("2.4"), 0o644); err != nil {
		t.Fatalf("write invalid version: %v", err)
	}
	runGit(t, repo, "add", "BAD_VERSION")
	runGit(t, repo, "commit", "-m", "invalid version fixture")
	if _, _, err := readVersionFromRepo(repo, "", "BAD_VERSION", nil); err == nil || !strings.Contains(err.Error(), "semver core") {
		t.Fatalf("expected invalid semver to fail, got %v", err)
	}
}

func TestBuildAutoBumpStepScriptUsesSafeBranchResolution(t *testing.T) {
	script := buildAutoBumpStepScript("patch")
	assertScriptContains(t, script, `RAW_REF="${CIWI_PIPELINE_SOURCE_REF_RAW:-}"`)
	assertScriptContains(t, script, "git symbolic-ref --quiet --short refs/remotes/origin/HEAD")
	assertScriptContains(t, script, "git fetch origin \"$BRANCH\"")
	assertScriptContains(t, script, "git checkout -B ciwi-auto-bump \"origin/$BRANCH\"")
	assertScriptContains(t, script, "auto bump skipped: branch $BRANCH moved from ${CIWI_PIPELINE_VERSION_RAW} to ${CURRENT_VERSION}")
	assertScriptContains(t, script, "failed to resolve target branch for auto bump push")
	assertScriptContains(t, script, "auto bump push failed; branch $BRANCH advanced during release")
	assertScriptContains(t, script, `AUTH_TOKEN="${GITHUB_TOKEN:-${GH_TOKEN:-}}"`)
	assertScriptContains(t, script, `PUSH_REMOTE="origin"`)
	assertScriptContains(t, script, `PUSH_REMOTE="https://x-access-token:${AUTH_TOKEN}@github.com/${CIWI_PIPELINE_SOURCE_REPO#https://github.com/}"`)
	assertScriptContains(t, script, `git push "$PUSH_REMOTE" "HEAD:refs/heads/${BRANCH}"`)
	if strings.Contains(script, `BRANCH="main"`) {
		t.Fatalf("auto bump script must not hardcode main fallback")
	}
}

func assertScriptContains(t *testing.T, script, needle string) {
	t.Helper()
	if !strings.Contains(script, needle) {
		t.Fatalf("expected script to contain %q", needle)
	}
}
