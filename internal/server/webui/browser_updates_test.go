package webui

import (
	"strings"
	"testing"

	"github.com/dop251/goja"
)

func TestBrowserSettingsRestoresPersistedUpdateCheck(t *testing.T) {
	payload, err := browserRendererSource()
	if err != nil {
		t.Fatal(err)
	}
	script := string(payload)
	start := strings.Index(script, "  function declarativeVersionOptions")
	end := strings.Index(script, "\n  function outputMatchRanges")
	if start < 0 || end < 0 || start >= end {
		t.Fatal("browser update binding implementation is unavailable")
	}
	runtime := goja.New()
	if _, err := runtime.RunString(script[start:end]); err != nil {
		t.Fatal(err)
	}

	assertions := []string{
		`(() => { const value = declarativePersistedUpdateBinding({
			update_current_version:'v0.2.25', update_latest_version:'v0.2.26', update_available:'1',
			update_last_checked_utc:'2026-08-08T12:00:00Z'
		}); return value.updateVersions.length === 1 && value.updateVersions[0].value === 'v0.2.26'
			&& value.selectedUpdateVersion === 'v0.2.26' && value.updateResult === 'Update available: v0.2.25 → v0.2.26'; })()`,
		`(() => { const value = declarativePersistedUpdateBinding({
			update_current_version:'v0.2.26', update_latest_version:'v0.2.26', update_available:'0',
			update_last_checked_utc:'2026-08-08T12:00:00Z', update_message:'already up to date'
		}); return value.updateVersions[0].label === 'No newer versions available'
			&& value.selectedUpdateVersion === '' && value.updateResult === 'Up to date (v0.2.26)'; })()`,
		`(() => { const value = declarativePersistedUpdateBinding({
			update_current_version:'v0.2.27', update_latest_version:'v0.2.26', update_available:'1'
		}); return value.updateVersions[0].label === 'Click "Check for updates"'
			&& value.selectedUpdateVersion === '' && value.updateResult === ''; })()`,
	}
	for _, assertion := range assertions {
		value, err := runtime.RunString(assertion)
		if err != nil {
			t.Fatal(err)
		}
		if !value.ToBoolean() {
			t.Errorf("browser persisted-update assertion failed: %s", assertion)
		}
	}
}
