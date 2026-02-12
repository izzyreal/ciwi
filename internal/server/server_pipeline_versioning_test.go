package server

import (
	"strings"
	"testing"
)

func TestBuildAutoBumpStepScriptUsesSafeBranchResolution(t *testing.T) {
	script := buildAutoBumpStepScript("patch")
	assertScriptContains(t, script, `RAW_REF="${CIWI_PIPELINE_SOURCE_REF_RAW:-}"`)
	assertScriptContains(t, script, "git symbolic-ref --quiet --short refs/remotes/origin/HEAD")
	assertScriptContains(t, script, "git fetch origin \"$BRANCH\"")
	assertScriptContains(t, script, "git checkout -B ciwi-auto-bump \"origin/$BRANCH\"")
	assertScriptContains(t, script, "auto bump skipped: branch $BRANCH moved from ${CIWI_PIPELINE_VERSION_RAW} to ${CURRENT_VERSION}")
	assertScriptContains(t, script, "failed to resolve target branch for auto bump push")
	assertScriptContains(t, script, "auto bump push failed; branch $BRANCH advanced during release")
	assertScriptContains(t, script, `git push origin "HEAD:refs/heads/${BRANCH}"`)
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
