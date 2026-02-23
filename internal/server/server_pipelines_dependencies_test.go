package server

import (
	"testing"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func TestVerifyDependencyRunUsesLatestSuccessfulOfLatestVersion(t *testing.T) {
	base := time.Now().UTC()
	jobs := []protocol.JobExecution{
		{
			ID:         "build-ok-a",
			Status:     protocol.JobExecutionStatusSucceeded,
			CreatedUTC: base.Add(-3 * time.Minute),
			Metadata: map[string]string{
				"project":              "ciwi",
				"pipeline_id":          "build",
				"pipeline_run_id":      "run-old",
				"pipeline_version_raw": "1.2.3",
				"pipeline_version":     "v1.2.3",
				"build_target":         "linux-amd64",
			},
			ArtifactGlobs: []string{"dist/**"},
		},
		{
			ID:         "build-failed-rerun",
			Status:     protocol.JobExecutionStatusFailed,
			CreatedUTC: base.Add(-1 * time.Minute),
			Metadata: map[string]string{
				"project":              "ciwi",
				"pipeline_id":          "build",
				"pipeline_run_id":      "run-rerun",
				"pipeline_version_raw": "1.2.3",
				"pipeline_version":     "v1.2.3",
			},
		},
	}

	ctx, err := verifyDependencyRun(jobs, "ciwi", "build")
	if err != nil {
		t.Fatalf("verify dependency run: %v", err)
	}
	if ctx.VersionRaw != "1.2.3" || ctx.Version != "v1.2.3" {
		t.Fatalf("unexpected dependency version: raw=%q tagged=%q", ctx.VersionRaw, ctx.Version)
	}
	if got := ctx.ArtifactJobIDs["linux-amd64"]; got != "build-ok-a" {
		t.Fatalf("expected artifact job from successful run, got %q", got)
	}
}

func TestVerifyDependencyRunRejectsCrossVersionSuccessfulFallback(t *testing.T) {
	base := time.Now().UTC()
	jobs := []protocol.JobExecution{
		{
			ID:         "build-ok-old",
			Status:     protocol.JobExecutionStatusSucceeded,
			CreatedUTC: base.Add(-5 * time.Minute),
			Metadata: map[string]string{
				"project":              "ciwi",
				"pipeline_id":          "build",
				"pipeline_run_id":      "run-v1",
				"pipeline_version_raw": "1.2.3",
				"pipeline_version":     "v1.2.3",
			},
		},
		{
			ID:         "build-failed-new",
			Status:     protocol.JobExecutionStatusFailed,
			CreatedUTC: base.Add(-1 * time.Minute),
			Metadata: map[string]string{
				"project":              "ciwi",
				"pipeline_id":          "build",
				"pipeline_run_id":      "run-v2",
				"pipeline_version_raw": "1.2.4",
				"pipeline_version":     "v1.2.4",
			},
		},
	}

	if _, err := verifyDependencyRun(jobs, "ciwi", "build"); err == nil {
		t.Fatalf("expected dependency verification to fail when latest version has no successful run")
	}
}

func TestVerifyDependencyRunReturnsSourceRepoAndResolvedRef(t *testing.T) {
	base := time.Now().UTC()
	jobs := []protocol.JobExecution{
		{
			ID:         "build-ok",
			Status:     protocol.JobExecutionStatusSucceeded,
			CreatedUTC: base,
			Metadata: map[string]string{
				"project":                      "ciwi",
				"pipeline_id":                  "build",
				"pipeline_run_id":              "run-1",
				"pipeline_version_raw":         "1.2.3",
				"pipeline_version":             "v1.2.3",
				"pipeline_source_repo":         "https://github.com/acme/build.git",
				"pipeline_source_ref_resolved": "deadbeef",
			},
		},
	}

	ctx, err := verifyDependencyRun(jobs, "ciwi", "build")
	if err != nil {
		t.Fatalf("verify dependency run: %v", err)
	}
	if ctx.SourceRepo != "https://github.com/acme/build.git" {
		t.Fatalf("unexpected source repo: %q", ctx.SourceRepo)
	}
	if ctx.SourceRefResolved != "deadbeef" {
		t.Fatalf("unexpected source ref resolved: %q", ctx.SourceRefResolved)
	}
}
