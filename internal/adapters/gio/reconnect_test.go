//go:build darwin

package gio

import (
	"testing"
	"time"

	cnpv1 "github.com/izzyreal/ciwi/pkg/cnp/v1"
)

func TestNextReconnectDelayBacksOffAndCaps(t *testing.T) {
	tests := []struct {
		current time.Duration
		want    time.Duration
	}{
		{0, time.Second},
		{time.Second, 2 * time.Second},
		{4 * time.Second, 8 * time.Second},
		{8 * time.Second, 8 * time.Second},
	}
	for _, test := range tests {
		if got := nextReconnectDelay(test.current); got != test.want {
			t.Errorf("nextReconnectDelay(%s) = %s, want %s", test.current, got, test.want)
		}
	}
}

func TestRunOptionsIgnoresHeartbeatOnlyAgentInvalidations(t *testing.T) {
	navigation := navigationState{screen: "run-options", pipelineDBID: 1}
	if relevantScreenChange(navigation, &cnpv1.ChangeEvent{Topics: []cnpv1.ChangeTopic{cnpv1.ChangeTopic_CHANGE_TOPIC_AGENTS}}) {
		t.Fatal("ordinary agent heartbeat invalidated run options")
	}
	if !relevantScreenChange(navigation, &cnpv1.ChangeEvent{Topics: []cnpv1.ChangeTopic{cnpv1.ChangeTopic_CHANGE_TOPIC_AGENT_ELIGIBILITY}}) {
		t.Fatal("agent eligibility change did not invalidate run options")
	}
}
