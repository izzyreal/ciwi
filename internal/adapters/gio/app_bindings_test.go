//go:build darwin || ios || linux || windows

package gio

import (
	"testing"

	cnpv1 "github.com/izzyreal/ciwi/pkg/cnp/v1"
	sharedui "github.com/izzyreal/ciwi/ui"
)

func TestRunOptionsRoutesSelectExactlyOneRequestTarget(t *testing.T) {
	for _, test := range []struct {
		route             string
		pipeline, project int64
		chain             string
	}{
		{"/run-options/projects/2/pipelines/7", 7, 0, ""},
		{"/run-options/pipelines/7", 7, 0, ""},
		{"/run-options/projects/2/chains/release", 0, 2, "release"},
	} {
		t.Run(test.route, func(t *testing.T) {
			navigation, err := navigationForRoute(test.route)
			if err != nil {
				t.Fatal(err)
			}
			navigation.sourceRef, navigation.agentID = "main", "windows-agent"
			request := nativeRunOptionsRequest(navigation)
			if request.PipelineDbId != test.pipeline || request.ProjectId != test.project || request.ChainId != test.chain {
				t.Fatalf("unexpected target: %v", request)
			}
			if request.Selection.SourceRef != "main" || request.Selection.AgentId != "windows-agent" {
				t.Fatalf("lost selection: %v", request.Selection)
			}
		})
	}
}

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
		followLatest        bool
	}{
		{status: "failed", label: "Tailing: Off", tone: "accent", followLatest: false},
		{status: "running", label: "Tailing: On", tone: "success", followLatest: true},
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
		if root["output_follow_latest"] != test.followLatest {
			t.Errorf("status %q follow latest = %#v, want %v", test.status, root["output_follow_latest"], test.followLatest)
		}
	}
}

func TestJobDetailsBindingsCarryLiveDurationClock(t *testing.T) {
	data, err := jobDetailsBindingData(&cnpv1.JobDetailsView{
		Id: "job-1", Status: "running",
		Progress: &cnpv1.Progress{SnapshotUnixMs: 1_800_000_005_000},
		JobProperties: []*cnpv1.JobDetailRow{{
			Label: "Duration", LiveDurationStartedUnixMs: 1_800_000_000_000,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	root := data["jobDetails"].(map[string]any)
	row := root["job_properties"].([]any)[0].(map[string]any)
	if heartbeatUnixMillis(row["live_duration_started_unix_ms"]) != 1_800_000_000_000 {
		t.Fatalf("live duration row = %#v", row)
	}
	if heartbeatUnixMillis(root["duration_client_snapshot_unix_ms"]) <= 0 {
		t.Fatalf("client duration snapshot = %#v", root["duration_client_snapshot_unix_ms"])
	}
}

func TestJobLogBindingsUseLogViewEmptyState(t *testing.T) {
	data, err := jobDetailsBindingData(&cnpv1.JobDetailsView{
		Id: "job-1", OutputGroups: []*cnpv1.JobOutputGroup{{Id: "step:1", Reached: true}},
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
