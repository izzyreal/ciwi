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
			"server": map[string]any{"version": "test"}, "projects": []any{},
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
		"themes": []any{}, "projects": []any{}, "server_version": "test",
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

func browserClientBindingFixture() map[string]any {
	return map[string]any{
		"connected": true, "connecting": false, "offline": false, "address": "localhost",
		"status": "Connected through the browser", "tone": "success", "progress": map[string]any{"state": "none"},
	}
}
