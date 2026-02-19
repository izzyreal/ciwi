package agent

import (
	"strings"
	"testing"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func TestWorkspaceDirForJobStableWithSameIdentity(t *testing.T) {
	workDir := t.TempDir()
	job := protocol.JobExecution{
		ID: "job-a",
		Metadata: map[string]string{
			"project_id":      "42",
			"pipeline_job_id": "windows-build",
			"matrix_name":     "windows10-x86",
		},
		RequiredCapabilities: map[string]string{
			"os":       "windows",
			"arch":     "amd64",
			"shell":    "cmd",
			"executor": "script",
		},
	}

	one := workspaceDirForJob(workDir, job)
	two := workspaceDirForJob(workDir, job)
	if one != two {
		t.Fatalf("workspace dir not stable: %q != %q", one, two)
	}
}

func TestWorkspaceDirForJobChangesAcrossMatrixIdentity(t *testing.T) {
	workDir := t.TempDir()
	base := protocol.JobExecution{
		ID: "job-a",
		Metadata: map[string]string{
			"project_id":      "42",
			"pipeline_job_id": "windows-build",
			"matrix_name":     "windows10-x86",
		},
		RequiredCapabilities: map[string]string{
			"os":       "windows",
			"arch":     "amd64",
			"shell":    "cmd",
			"executor": "script",
		},
	}
	other := base
	other.Metadata = map[string]string{
		"project_id":      "42",
		"pipeline_job_id": "windows-build",
		"matrix_name":     "windows7-x86",
	}
	one := workspaceDirForJob(workDir, base)
	two := workspaceDirForJob(workDir, other)
	if one == two {
		t.Fatalf("expected different workspace dirs for different matrix identities: %q", one)
	}
}

func TestWorkspaceDirForJobSingleWhenNoMatrix(t *testing.T) {
	workDir := t.TempDir()
	job := protocol.JobExecution{
		ID: "job-a",
		Metadata: map[string]string{
			"project_id":      "42",
			"pipeline_job_id": "linux-amd64",
		},
		RequiredCapabilities: map[string]string{
			"os":       "linux",
			"arch":     "amd64",
			"shell":    "posix",
			"executor": "script",
		},
	}
	got := workspaceDirForJob(workDir, job)
	if strings.Contains(got, "_single_") || strings.Contains(got, "single_") {
		t.Fatalf("expected no single matrix marker in workspace path, got %q", got)
	}
	if strings.Contains(got, "idx-") {
		t.Fatalf("expected no index matrix marker in non-matrix workspace path, got %q", got)
	}
}
