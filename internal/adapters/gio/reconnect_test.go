//go:build darwin || linux || windows

package gio

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	cnpv1 "github.com/izzyreal/ciwi/pkg/cnp/v1"
	"github.com/izzyreal/ciwi/pkg/cnpclient"
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

func TestNativeTargetsNormalizeExplicitEndpoint(t *testing.T) {
	targets, err := nativeTargets(context.Background(), ":8113")
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0] != "quic://127.0.0.1:8113" {
		t.Fatalf("targets = %v", targets)
	}
}

func TestDialNativeTargetsUsesFirstReachableCandidate(t *testing.T) {
	wantClient := &cnpclient.Client{}
	client, target, err := dialNativeTargetsWith(
		context.Background(),
		[]string{"tcp://192.168.56.1:8113", "tcp://192.168.1.235:8113"},
		"dev",
		func(ctx context.Context, target, _, _ string) (*cnpclient.Client, error) {
			if target == "tcp://192.168.1.235:8113" {
				return wantClient, nil
			}
			<-ctx.Done()
			return nil, ctx.Err()
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if client != wantClient || target != "tcp://192.168.1.235:8113" {
		t.Fatalf("winner = (%p, %q), want (%p, %q)", client, target, wantClient, "tcp://192.168.1.235:8113")
	}
}

func TestDialNativeTargetsReportsAllFailures(t *testing.T) {
	_, _, err := dialNativeTargetsWith(
		context.Background(),
		[]string{"tcp://one:8113", "tcp://two:8113"},
		"dev",
		func(context.Context, string, string, string) (*cnpclient.Client, error) {
			return nil, errors.New("unreachable")
		},
	)
	if err == nil || !strings.Contains(err.Error(), "tcp://one:8113") || !strings.Contains(err.Error(), "tcp://two:8113") {
		t.Fatalf("error = %v", err)
	}
}

func TestCaptureSSHHostKeyErrorRequiresExplicitDecision(t *testing.T) {
	settings := sshConnectionSettings{HostKeyFingerprint: "SHA256:trusted"}
	err := errors.Join(errors.New("connect through remote server"), &cnpclient.SSHHostKeyError{
		Address: "192.0.2.1:22", Fingerprint: "SHA256:presented", Expected: "SHA256:trusted",
	})
	if !captureSSHHostKeyError(&settings, err) {
		t.Fatal("wrapped SSH host key error was not classified as requiring verification")
	}
	if settings.PendingFingerprint != "SHA256:presented" || settings.HostKeyFingerprint != "SHA256:trusted" {
		t.Fatalf("SSH fingerprint state = %+v", settings)
	}
	if captureSSHHostKeyError(&settings, errors.New("connection refused")) {
		t.Fatal("ordinary connection failure was classified as a host key decision")
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
