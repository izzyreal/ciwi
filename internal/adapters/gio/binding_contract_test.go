//go:build darwin || ios || linux || windows

package gio

import (
	"testing"

	sharedui "github.com/izzyreal/ciwi/ui"
)

func TestLoadingViewModelsSatisfySharedScreenBindings(t *testing.T) {
	tests := []navigationState{
		{screen: "front-page"},
		{screen: "project-details", projectID: 1},
		{screen: "job-details", jobID: "job-1"},
		{screen: "settings"},
		{screen: "managed-yaml"},
		{screen: "run-options", pipelineDBID: 1},
		{screen: "agents"},
		{screen: "agent-details", agentDetailsID: "agent-1"},
		{screen: "agent-script", agentScriptID: "agent-1"},
		{screen: "vault"},
	}
	for _, navigation := range tests {
		t.Run(navigation.screen, func(t *testing.T) {
			screen, err := sharedui.LoadScreen(navigation.screen)
			if err != nil {
				t.Fatal(err)
			}
			data, err := screenLoadingData(navigation, "test", "default", connectionModeDiscover, "", sshConnectionSettings{})
			if err != nil {
				t.Fatal(err)
			}
			if err := validateNativeBindings(screen, data); err != nil {
				t.Fatal(err)
			}
			root, _ := data[screenBindingRoot(navigation.screen)].(map[string]any)
			if root["loading"] != true || root["ready"] != false || root["load_error"] != "" {
				t.Fatalf("loading lifecycle = %#v", root)
			}
		})
	}
}

func TestProjectDetailsDecorationSuppliesMissingIconBinding(t *testing.T) {
	screen, err := sharedui.LoadScreen("project-details")
	if err != nil {
		t.Fatal(err)
	}
	data, err := screenLoadingBindingData(navigationState{screen: "project-details", projectID: 1}, "test", "default", connectionModeDiscover, "", sshConnectionSettings{})
	if err != nil {
		t.Fatal(err)
	}
	root := data["projectDetails"].(map[string]any)
	project := root["project"].(map[string]any)
	delete(project, "project_icon")
	delete(project, "project_icon_content_type")
	decorateProjectDetails(root)
	if err := validateNativeBindings(screen, data); err != nil {
		t.Fatal(err)
	}
	if _, exists := project["project_icon"]; !exists {
		t.Fatal("project_icon binding is missing")
	}
}
