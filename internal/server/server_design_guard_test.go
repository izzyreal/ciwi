package server

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func TestDesignGuardGoCodeAvoidsRawJobStatusLiterals(t *testing.T) {
	root := repoRootFromServerTests(t)
	files := []string{
		"internal/server/server_agents.go",
		"internal/server/server_job_views.go",
		"internal/agent/agent_exec.go",
		"internal/store/sqlite_jobs.go",
	}
	forbidden := []string{
		`"queued"`,
		`"leased"`,
		`"running"`,
		`"succeeded"`,
		`"failed"`,
	}

	for _, rel := range files {
		source := mustReadRepoFile(t, root, rel)
		for _, literal := range forbidden {
			lines := literalLineNumbers(source, literal)
			if len(lines) == 0 {
				continue
			}
			t.Errorf("%s contains raw job status literal %s at lines %v; use protocol job-status constants/helpers", rel, literal, lines)
		}
	}
}

func TestDesignGuardUIJobsUseSharedStatusHelpers(t *testing.T) {
	root := repoRootFromServerTests(t)
	files := []string{
		"internal/server/ui_job.go",
		"internal/server/ui_pages.go",
		"internal/server/ui_agent.go",
		"internal/server/ui_index_js_jobs.go",
	}
	directJobStatusComparison := regexp.MustCompile(`(?:\bstatus\b|\.status\b|normalizedJobStatus\s*\([^)]*\))\s*(?:===|==)\s*['"](queued|leased|running|succeeded|failed)['"]`)

	for _, rel := range files {
		source := mustReadRepoFile(t, root, rel)
		lines := regexLineNumbers(source, directJobStatusComparison)
		if len(lines) == 0 {
			continue
		}
		t.Errorf("%s contains direct job-status comparisons at lines %v; use shared status helpers from ui/shared.js", rel, lines)
	}
}

func TestDesignGuardAgentHandlersAvoidUntypedJSONMaps(t *testing.T) {
	root := repoRootFromServerTests(t)
	files := []string{
		"internal/server/server_agents.go",
		"internal/server/server_agents_api.go",
	}

	for _, rel := range files {
		source := mustReadRepoFile(t, root, rel)
		lines := literalLineNumbers(source, "map[string]any{")
		if len(lines) == 0 {
			continue
		}
		t.Errorf("%s contains untyped map JSON response literals at lines %v; use typed response DTOs", rel, lines)
	}
}

func TestDesignGuardProjectAndPipelineHandlersAvoidUntypedJSONMaps(t *testing.T) {
	root := repoRootFromServerTests(t)
	files := []string{
		"internal/server/server_projects.go",
		"internal/server/server_pipelines.go",
	}

	for _, rel := range files {
		source := mustReadRepoFile(t, root, rel)
		lines := literalLineNumbers(source, "map[string]any{")
		if len(lines) == 0 {
			continue
		}
		t.Errorf("%s contains untyped map JSON response literals at lines %v; use typed response DTOs", rel, lines)
	}
}

func TestDesignGuardUpdateHandlersAvoidUntypedJSONMaps(t *testing.T) {
	root := repoRootFromServerTests(t)
	source := mustReadRepoFile(t, root, "internal/server/server_update_handlers.go")
	lines := literalLineNumbers(source, "map[string]any{")
	if len(lines) == 0 {
		return
	}
	t.Errorf("internal/server/server_update_handlers.go contains untyped map JSON response literals at lines %v; use typed response DTOs", lines)
}

func TestDesignGuardVaultHandlersAvoidUntypedJSONMaps(t *testing.T) {
	root := repoRootFromServerTests(t)
	source := mustReadRepoFile(t, root, "internal/server/server_vault.go")
	lines := literalLineNumbers(source, "map[string]any{")
	if len(lines) == 0 {
		return
	}
	t.Errorf("internal/server/server_vault.go contains untyped map JSON response literals at lines %v; use typed response DTOs", lines)
}

func TestDesignGuardInfoHandlersAvoidInlineMapResponses(t *testing.T) {
	root := repoRootFromServerTests(t)
	files := []string{
		"internal/server/server_info.go",
		"internal/server/server_helpers.go",
	}
	pattern := regexp.MustCompile(`writeJSON\s*\(.*map\[[^\]]+\]`)

	for _, rel := range files {
		source := mustReadRepoFile(t, root, rel)
		lines := regexLineNumbers(source, pattern)
		if len(lines) == 0 {
			continue
		}
		t.Errorf("%s contains inline map response at lines %v; use typed response DTOs", rel, lines)
	}
}

func repoRootFromServerTests(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("resolve current test file path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func mustReadRepoFile(t *testing.T, root, rel string) string {
	t.Helper()
	path := filepath.Join(root, rel)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(data)
}

func literalLineNumbers(source, literal string) []int {
	lines := strings.Split(source, "\n")
	out := make([]int, 0, 4)
	for i, line := range lines {
		if strings.Contains(line, literal) {
			out = append(out, i+1)
		}
	}
	return out
}

func regexLineNumbers(source string, pattern *regexp.Regexp) []int {
	lines := strings.Split(source, "\n")
	out := make([]int, 0, 4)
	for i, line := range lines {
		if pattern.MatchString(line) {
			out = append(out, i+1)
		}
	}
	return out
}
