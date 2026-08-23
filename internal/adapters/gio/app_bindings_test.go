//go:build darwin || ios || linux || windows

package gio

import (
	"testing"

	cnpv1 "github.com/izzyreal/ciwi/pkg/cnp/v1"
	sharedui "github.com/izzyreal/ciwi/ui"
)

func TestJobDetailsBindingsExposePreExecutionFailure(t *testing.T) {
	data, err := jobDetailsBindingData(&cnpv1.JobDetailsView{
		Id: "job-failed", Status: "failed", Error: "secret resolution failed before execution",
	})
	if err != nil {
		t.Fatal(err)
	}
	screen, err := sharedui.LoadScreen("job-details")
	if err != nil {
		t.Fatal(err)
	}
	if err := validateNativeBindings(screen, data); err != nil {
		t.Fatal(err)
	}
	root := data["jobDetails"].(map[string]any)
	if root["error"] != "secret resolution failed before execution" {
		t.Fatalf("job error binding = %q", root["error"])
	}
}

func TestJobDetailsBindingsUseOrdinaryOffAndSelectedOnTailingTones(t *testing.T) {
	for _, test := range []struct {
		status, label, tone string
	}{
		{status: "failed", label: "Tailing: Off", tone: "accent"},
		{status: "running", label: "Tailing: On", tone: "success"},
	} {
		data, err := jobDetailsBindingData(&cnpv1.JobDetailsView{Id: "job-1", Status: test.status})
		if err != nil {
			t.Fatal(err)
		}
		root := data["jobDetails"].(map[string]any)
		if root["tailing_label"] != test.label || root["tailing_tone"] != test.tone {
			t.Errorf("status %q tailing bindings = %q/%q, want %q/%q", test.status,
				root["tailing_label"], root["tailing_tone"], test.label, test.tone)
		}
	}
}

func TestIndexedJobLogBindingsSuppressLegacyEmptyOutputLabel(t *testing.T) {
	data, err := jobDetailsBindingData(&cnpv1.JobDetailsView{
		Id: "job-1", InteractiveLogAvailable: true,
		OutputGroups: []*cnpv1.JobOutputGroup{{Id: "step:1", Reached: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	root := data["jobDetails"].(map[string]any)
	groups := root["output_groups"].([]any)
	group := groups[0].(map[string]any)
	if label := group["empty_output_label"]; label != "" {
		t.Fatalf("indexed empty output label = %q, want empty", label)
	}
}

func TestLegacyJobLogBindingsKeepBestEffortEmptyOutputLabel(t *testing.T) {
	data, err := jobDetailsBindingData(&cnpv1.JobDetailsView{
		Id: "job-legacy", OutputGroups: []*cnpv1.JobOutputGroup{{Id: "step:1", Reached: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	root := data["jobDetails"].(map[string]any)
	groups := root["output_groups"].([]any)
	group := groups[0].(map[string]any)
	if label := group["empty_output_label"]; label != "(no output)" {
		t.Fatalf("legacy empty output label = %q", label)
	}
}
