package webui

import (
	"strings"
	"testing"
)

func TestManagedYAMLUIIncludesEditorAndSourceAwareRendering(t *testing.T) {
	combined := settingsHTML + settingsJS + settingsJS + pagesJS + indexJS + projectHTML + projectJS
	for _, want := range []string{
		"Add Managed YAML",
		"function openCreateManagedYAML()",
		"function openEditManagedYAML(project)",
		"function validateManagedYAML()",
		"function saveManagedYAML()",
		"Load YAML file",
		"Reload latest",
		"function isManagedYAMLProject(project)",
		"function projectSourceMetadataHTML(project)",
		"projectSourceMetadataHTML(p)",
		"projectSourceMetadataHTML(project)",
	} {
		if !strings.Contains(combined, want) {
			t.Fatalf("managed YAML UI no longer contains %q", want)
		}
	}
	if !strings.Contains(settingsJS, "definitionBtn.textContent = 'Edit YAML'") || !strings.Contains(settingsJS, "definitionBtn.textContent = 'Reload project definition from VCS'") {
		t.Fatalf("settings project controls are not source-aware")
	}
}
