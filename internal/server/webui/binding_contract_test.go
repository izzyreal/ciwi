package webui

import (
	"testing"

	"github.com/izzyreal/ciwi/pkg/uidsl"
	sharedui "github.com/izzyreal/ciwi/ui"
)

func TestBrowserFrontPageViewModelSatisfiesSharedBindings(t *testing.T) {
	screen, err := sharedui.LoadScreen("front-page")
	if err != nil {
		t.Fatal(err)
	}
	data := map[string]any{
		"frontPage": map[string]any{
			"server": map[string]any{"version": "test"}, "projects": []any{map[string]any{
				"id": 1, "project_icon": "", "name": "example", "source_kind": "managed_yaml",
				"repo_url": "", "repo_ref": "", "config_file": "", "pipeline_count_label": "0 pipelines",
				"pipeline_chains": []any{}, "pipelines": []any{},
			}},
			"queued_executions": []any{}, "history_executions": []any{},
			"loading": false, "queued_empty": true, "history_empty": true,
		},
		"client": browserClientBindingFixture(),
	}
	if err := uidsl.ValidateBindings(screen, data, "web"); err != nil {
		t.Fatal(err)
	}
}

func TestBrowserSettingsViewModelSatisfiesWebOverrides(t *testing.T) {
	screen, err := sharedui.LoadScreen("settings")
	if err != nil {
		t.Fatal(err)
	}
	settings := map[string]any{
		"server": map[string]any{"name": "ciwi", "version": "test", "hostname": "host", "api_version": 1},
		"themes": []any{}, "projects": []any{map[string]any{
			"id": 1, "name": "example", "is_managed": false, "has_repo": true,
			"repo_url": "https://example.invalid/repo", "repo_ref_label": "main", "config_file": "ciwi-project.yaml",
			"updated_label": "now", "has_loaded_commit": true, "loaded_commit_short": "12345678",
			"loaded_commit_url": "https://example.invalid/repo/commit/12345678", "can_reload": true,
			"action_status": "", "action_tone": "muted",
		}}, "server_version": "test",
		"selected_theme": "default", "selected_theme_description": "",
		"import_repo_url": "", "import_repo_ref": "", "import_config_file": "ciwi-project.yaml",
		"update_supported": false, "update_capability_notice": "", "update_status_label": "", "blocked_agent_notice": "",
		"update_versions": []any{}, "selected_update_version": "", "update_result": "", "update_result_tone": "muted",
		"rollback_versions": []any{}, "selected_rollback_version": "", "rollback_result": "", "rollback_result_tone": "muted",
	}
	if err := uidsl.ValidateBindings(screen, map[string]any{"settings": settings, "client": browserClientBindingFixture()}, "web"); err != nil {
		t.Fatal(err)
	}
}

func TestBrowserRoutedViewFixturesSatisfySharedBindings(t *testing.T) {
	progress := map[string]any{"state": "none", "fraction": 0.0, "snapshot_unix_ms": 0, "rate_per_ms": 0.0}
	reportNode := map[string]any{
		"key": "file:binary", "label": "binary", "detail": "stored", "tone": "success", "link": "",
		"action_label": "Download", "action_kind": "file", "action_path": "binary", "default_expanded": false,
		"filter_values": []any{"all", "pass"}, "children": []any{},
	}
	step := map[string]any{
		"index": 0, "position": 1, "name": "Compile", "type": "run", "command": "go test ./...",
		"skip_dry_run": false, "environment_label": "CI=true",
	}
	job := map[string]any{
		"id": "test", "needs": []any{}, "needs_label": "none", "tools_label": "go", "summary_label": "1 step · runs on: linux",
		"timeout_label": "Timeout: 60s", "matrix_label": "Matrix: none", "supports_dry_run": true, "steps": []any{step},
	}
	pipeline := map[string]any{
		"id": 7, "pipeline_id": "build", "summary_label": "1 job · depends on: none",
		"graph_summary_label": "1 job · 0 dependencies", "depends_on": []any{}, "supports_dry_run": true, "jobs": []any{job},
	}
	project := map[string]any{
		"id": 1, "name": "example", "project_icon": "", "repo_url": "https://example.invalid/repo",
		"repo_ref": "main", "config_file": "ciwi-project.yaml", "has_pipeline_chains": true,
		"pipeline_chains": []any{map[string]any{
			"id": "release", "name": "Release", "sequence_label": "build", "supports_dry_run": true, "pipelines": []any{"build"},
		}},
	}
	jobDetails := map[string]any{
		"id": "job-1", "project_id": 1, "title": "example build", "project_icon": "", "progress": progress,
		"status": "running", "status_label": "Running", "current_step": "Compile", "can_rerun": false, "can_cancel": true,
		"job_properties":         []any{map[string]any{"label": "Created", "value": "now"}},
		"cache_statistics_empty": "", "cache_statistics": []any{map[string]any{"label": "Cache", "value": "Hit", "tone": "success"}},
		"scheduling_diagnosis": map[string]any{
			"summary": "Waiting", "requirements_label": "linux", "additional_agents_label": "",
			"agents": []any{map[string]any{"agent_id": "agent-1", "status": "eligible", "details": "ready"}},
		},
		"host_tool_requirements":      map[string]any{"empty_label": "", "summary": "Available", "tone": "success", "issues": []any{"none"}},
		"container_tool_requirements": map[string]any{"empty_label": "", "summary": "Available", "tone": "success", "issues": []any{"none"}},
		"has_release_summary":         true,
		"release_summary":             []any{map[string]any{"label": "Version", "value": "v1", "tone": "success"}},
		"run_context": map[string]any{
			"available": true, "scope_label": "one pipeline", "current_execution_id": "job-1", "pipelines": []any{map[string]any{
				"id": 7, "pipeline_id": "build", "summary_label": "1 job", "depends_on": []any{}, "jobs": []any{map[string]any{
					"id": "test", "summary_label": "1 execution", "needs": []any{}, "executions": []any{map[string]any{
						"id": "job-1", "matrix_label": "default", "status": "running", "attempt_label": "Attempt 1",
					}},
				}},
			}},
		},
		"output_search": "test", "output_search_count": "1/1", "tailing_label": "Tailing: On", "tailing_tone": "success",
		"timeline":      []any{map[string]any{"id": "step:0", "title": "Compile", "status": "running", "status_label": "Running", "progress": progress}},
		"system_output": "starting", "output_groups": []any{map[string]any{
			"id": "step:0", "title": "Compile", "state_key": "job-output:job-1:step:0", "status": "running", "progress": progress,
			"reached": true, "started": "now", "duration": "1s", "exit_code": "", "error": "", "is_phase": false, "is_step": true,
			"details": "details", "yaml_literal": "run: go test", "expanded_command": "go test ./...", "output": "ok", "empty_output_label": "",
		}},
		"artifacts": map[string]any{
			"empty_label": "", "summary": "1 artifact", "tone": "success", "additional_label": "", "rows": []any{},
			"nodes": []any{reportNode}, "filters": []any{}, "filter": "", "can_download_all": true,
		},
		"test_report": map[string]any{
			"empty_label": "", "summary": "1 passed", "tone": "success", "additional_label": "", "rows": []any{},
			"nodes": []any{reportNode}, "filters": []any{map[string]any{"value": "all", "label": "All"}}, "filter": "all", "can_download_all": false,
		},
		"coverage_report": map[string]any{
			"empty_label": "", "summary": "90%", "tone": "success", "additional_label": "", "rows": []any{},
			"nodes": []any{reportNode}, "filters": []any{}, "filter": "", "can_download_all": false,
		},
	}
	fixtures := map[string]map[string]any{
		"project-details": {"projectDetails": map[string]any{
			"project": project, "pipelines": []any{pipeline}, "visible_pipelines": []any{pipeline},
			"structure_root":   map[string]any{"id": "project:1:all-pipelines", "label": "example", "meta": "Project · 1 pipeline", "runnable": false, "project_id": "1", "chain_id": ""},
			"structure_filter": "all-pipelines", "structure_filters": []any{map[string]any{"value": "all-pipelines", "label": "All Pipelines"}},
			"loading": false, "ready": true, "load_error": "", "show_chain_structure": false, "show_pipeline_structure": true,
			"history_empty": true, "history_executions": []any{},
		}},
		"job-details": {"jobDetails": jobDetails},
		"run-options": {"runOptions": map[string]any{
			"project_id": 1, "pipeline_db_id": 7, "chain_id": "", "target_kind": "pipeline", "target_label": "build",
			"source_repo": "https://example.invalid/repo", "pending_jobs": 1, "selected_source_ref": "main", "selected_agent_id": "agent-1",
			"source_refs":     []any{map[string]any{"value": "main", "label": "main"}},
			"eligible_agents": []any{map[string]any{"value": "agent-1", "label": "agent-1"}}, "supports_dry_run": true,
		}},
		"agents": {"agents": map[string]any{"summary": "1 agent", "agents": []any{map[string]any{
			"id": "agent-1", "hostname": "host", "platform": "linux", "version": "v1", "last_seen": "now",
			"last_seen_unix_ms": 1, "status": "online", "status_label": "Online", "run_mode": "service",
		}}}},
		"agent-details": {"agentDetails": map[string]any{"agent": map[string]any{
			"id": "agent-1", "hostname": "host", "platform": "linux", "version": "v1", "last_seen": "now", "status": "online",
			"status_label": "Online", "run_mode": "service", "authorization": "Authorized", "activation": "Active", "authorized": true,
			"deactivated": false, "can_contact": true, "can_update": true, "can_run_script": true, "capabilities_label": "go", "update_label": "Current", "recent_log": "ready",
		}}},
		"agent-script": {"agentScript": map[string]any{
			"agent_id": "agent-1", "agent_label": "host", "selected_shell": "sh", "script": "echo ok", "can_run": true,
			"result": "", "result_tone": "muted", "shells": []any{map[string]any{"value": "sh", "label": "POSIX shell"}},
		}},
		"managed-yaml": {"managedYAML": map[string]any{
			"title": "Edit Managed YAML", "project_id": 1, "project_name": "example", "yaml": "name: example", "revision": "1", "editing": true,
			"result": "Valid", "result_tone": "success",
		}},
		"vault": {"vault": map[string]any{
			"connections_empty": false, "name": "home", "url": "https://vault.invalid", "role_id": "role", "approle_mount": "approle",
			"secret_id_env": "VAULT_SECRET_ID", "example": "vault://home/secret/data/build#token", "result": "OK", "result_tone": "success",
			"connections": []any{map[string]any{"id": "1", "name": "home", "url": "https://vault.invalid", "role_id": "role", "approle_mount": "approle", "secret_id_env": "VAULT_SECRET_ID"}},
		}},
	}
	for screenName, data := range fixtures {
		screen, err := sharedui.LoadScreen(screenName)
		if err != nil {
			t.Fatal(err)
		}
		if err := uidsl.ValidateBindings(screen, data, "web"); err != nil {
			t.Errorf("%s: %v", screenName, err)
		}
	}
}

func browserClientBindingFixture() map[string]any {
	return map[string]any{
		"connected": true, "connecting": false, "offline": false, "address": "localhost",
		"status": "Connected through the browser", "tone": "success", "progress": map[string]any{"state": "none"},
	}
}
