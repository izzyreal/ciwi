package server

import (
	"reflect"
	"testing"

	"github.com/izzyreal/ciwi/internal/config"
	"github.com/izzyreal/ciwi/internal/protocol"
)

func TestNormalizePipelineJobNeeds(t *testing.T) {
	got := normalizePipelineJobNeeds([]string{"  a ", "", "a", "b", " b "})
	want := []string{"a", "b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizePipelineJobNeeds mismatch: got=%v want=%v", got, want)
	}
}

func TestCloneProtocolJobCaches(t *testing.T) {
	in := []protocol.JobCacheSpec{{ID: "ccache", Env: "CCACHE_DIR"}}
	got := cloneProtocolJobCaches(in)
	if !reflect.DeepEqual(got, in) {
		t.Fatalf("cloneProtocolJobCaches mismatch: got=%v want=%v", got, in)
	}
	got[0].ID = "mutated"
	if in[0].ID == "mutated" {
		t.Fatalf("cloneProtocolJobCaches should deep-copy")
	}
}

func TestResolveDependencyArtifactJobID(t *testing.T) {
	dependsOn := []string{"build"}
	depIDs := map[string]string{
		"build:linux-amd64": "job-1",
		"build:publish":     "job-2",
	}
	if got := resolveDependencyArtifactJobID(dependsOn, depIDs, "publish", map[string]string{"name": "linux-amd64"}); got != "job-1" {
		t.Fatalf("expected name candidate match, got %q", got)
	}
	if got := resolveDependencyArtifactJobID(dependsOn, depIDs, "release-publish", nil); got != "job-2" {
		t.Fatalf("expected release- prefix fallback match, got %q", got)
	}
	if got := resolveDependencyArtifactJobID([]string{"missing"}, depIDs, "publish", nil); got != "" {
		t.Fatalf("expected empty for no matches, got %q", got)
	}
}

func TestResolveDependencyArtifactJobIDs(t *testing.T) {
	all := map[string][]string{
		"build":   {"job-1", "job-2"},
		"package": {"job-2", "job-3"},
	}
	got := resolveDependencyArtifactJobIDs([]string{"build", "package"}, all, "job-0")
	want := []string{"job-0"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("resolveDependencyArtifactJobIDs mismatch: got=%v want=%v", got, want)
	}
	got = resolveDependencyArtifactJobIDs([]string{"build", "package"}, all, "")
	want = []string{"job-1", "job-2", "job-3"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("resolveDependencyArtifactJobIDs without preferred mismatch: got=%v want=%v", got, want)
	}
	if got := resolveDependencyArtifactJobIDs(nil, nil, ""); got != nil {
		t.Fatalf("expected nil when no deps and no preferred, got %v", got)
	}
}

func TestCloneJobStepPlan(t *testing.T) {
	in := []protocol.JobStepPlanItem{{
		Index:           1,
		Total:           2,
		Name:            "compile",
		Script:          "cmake --build .",
		Kind:            "run",
		Env:             map[string]string{"GITHUB_TOKEN": "{{ secret.github-secret }}"},
		VaultConnection: "home-vault",
		VaultSecrets:    []protocol.ProjectSecretSpec{{Name: "github-secret", Mount: "kv", Path: "gh", Key: "token"}},
		TestName:        "",
		TestFormat:      "",
		TestReport:      "",
	}}
	got := cloneJobStepPlan(in)
	if !reflect.DeepEqual(got, in) {
		t.Fatalf("cloneJobStepPlan mismatch: got=%v want=%v", got, in)
	}
	got[0].Name = "mutated"
	if in[0].Name == "mutated" {
		t.Fatalf("cloneJobStepPlan should deep-copy")
	}
	got[0].Env["GITHUB_TOKEN"] = "mutated"
	if in[0].Env["GITHUB_TOKEN"] == "mutated" {
		t.Fatalf("cloneJobStepPlan should deep-copy env map")
	}
	got[0].VaultSecrets[0].Key = "mutated"
	if in[0].VaultSecrets[0].Key == "mutated" {
		t.Fatalf("cloneJobStepPlan should deep-copy vault secrets")
	}
}

func TestBuildAutoBumpNextVersion(t *testing.T) {
	cases := []struct {
		version string
		mode    string
		want    string
	}{
		{"1.2.3", "patch", "1.2.4"},
		{"1.2.3", "minor", "1.3.0"},
		{"1.2.3", "major", "2.0.0"},
		{"1.2", "patch", ""},
		{"1.x.3", "patch", ""},
		{"1.2.3", "unknown", ""},
	}
	for _, tc := range cases {
		if got := buildAutoBumpNextVersion(tc.version, tc.mode); got != tc.want {
			t.Fatalf("buildAutoBumpNextVersion(%q,%q)=%q want=%q", tc.version, tc.mode, got, tc.want)
		}
	}
}

func TestDeriveAutoBumpBranch(t *testing.T) {
	cases := []struct {
		ref  string
		want string
	}{
		{"refs/heads/main", "main"},
		{"refs/tags/v1.0.0", ""},
		{"deadbeef", ""},
		{"feature/cool", "feature/cool"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := deriveAutoBumpBranch(tc.ref); got != tc.want {
			t.Fatalf("deriveAutoBumpBranch(%q)=%q want=%q", tc.ref, got, tc.want)
		}
	}
}

func TestDescribePipelineSteps(t *testing.T) {
	runStep := config.PipelineJobStep{Run: "echo hi"}
	if got := describePipelineStep(runStep, 0, "job-a"); got != "step 1" {
		t.Fatalf("unexpected run step description: %q", got)
	}

	testStepUnnamed := config.PipelineJobStep{Test: &config.PipelineJobTestStep{Command: "ctest"}}
	if got := describePipelineStep(testStepUnnamed, 1, "job-a"); got != "test job-a-test-2" {
		t.Fatalf("unexpected unnamed test description: %q", got)
	}
	testStepNamed := config.PipelineJobStep{Test: &config.PipelineJobTestStep{Name: "unit", Command: "ctest"}}
	if got := describePipelineStep(testStepNamed, 1, "job-a"); got != "test unit" {
		t.Fatalf("unexpected named test description: %q", got)
	}

	if got := describeSkippedPipelineStepLiteral(config.PipelineJobStep{Run: "  make "}, 0, "job-a"); got != "make" {
		t.Fatalf("unexpected skipped run literal: %q", got)
	}
	if got := describeSkippedPipelineStepLiteral(config.PipelineJobStep{Test: &config.PipelineJobTestStep{Command: "  ctest -R foo  "}}, 0, "job-a"); got != "ctest -R foo" {
		t.Fatalf("unexpected skipped test command literal: %q", got)
	}
	if got := describeSkippedPipelineStepLiteral(config.PipelineJobStep{}, 2, "job-a"); got != "step 3" {
		t.Fatalf("unexpected skipped fallback literal: %q", got)
	}
}

func TestCloneJobCachesFromPersisted(t *testing.T) {
	in := []config.PipelineJobCacheSpec{{ID: "fetchcontent", Env: "FETCHCONTENT_BASE_DIR"}}
	got := cloneJobCachesFromPersisted(in)
	if len(got) != 1 || got[0].ID != in[0].ID || got[0].Env != in[0].Env {
		t.Fatalf("cloneJobCachesFromPersisted mismatch: got=%v", got)
	}
	got[0].ID = "mutated"
	if in[0].ID == "mutated" {
		t.Fatalf("cloneJobCachesFromPersisted should deep-copy")
	}
}
