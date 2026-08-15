//go:build darwin || linux || windows

package gio

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	cnpv1 "github.com/izzyreal/ciwi/pkg/cnp/v1"
	"github.com/izzyreal/ciwi/pkg/cnpclient"
	sharedui "github.com/izzyreal/ciwi/ui"
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

func TestExpectedNativeDisconnectSuppressesLifecycleNoise(t *testing.T) {
	for _, err := range []error{context.Canceled, io.EOF, net.ErrClosed, errors.New("use of closed network connection")} {
		if !expectedNativeDisconnect(err) {
			t.Errorf("expected disconnect %q was not classified", err)
		}
	}
	if expectedNativeDisconnect(errors.New("permission denied")) {
		t.Fatal("application error was classified as an expected disconnect")
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
	screen, err := sharedui.LoadScreen(navigation.screen)
	if err != nil {
		t.Fatal(err)
	}
	if relevantScreenChange(screen, navigation, &cnpv1.ChangeEvent{Topics: []cnpv1.ChangeTopic{cnpv1.ChangeTopic_CHANGE_TOPIC_AGENTS}}) {
		t.Fatal("ordinary agent heartbeat invalidated run options")
	}
	if !relevantScreenChange(screen, navigation, &cnpv1.ChangeEvent{Topics: []cnpv1.ChangeTopic{cnpv1.ChangeTopic_CHANGE_TOPIC_AGENT_ELIGIBILITY}}) {
		t.Fatal("agent eligibility change did not invalidate run options")
	}
}

func TestJobDetailsOnlyRefreshesForScopedNonOutputChanges(t *testing.T) {
	navigation := navigationState{screen: "job-details", jobID: "job-1"}
	screen, err := sharedui.LoadScreen(navigation.screen)
	if err != nil {
		t.Fatal(err)
	}
	if relevantScreenChange(screen, navigation, &cnpv1.ChangeEvent{Topics: []cnpv1.ChangeTopic{cnpv1.ChangeTopic_CHANGE_TOPIC_HISTORY}}) {
		t.Fatal("unscoped history invalidation refreshed job details")
	}
	if relevantScreenChange(screen, navigation, &cnpv1.ChangeEvent{
		Topics: []cnpv1.ChangeTopic{cnpv1.ChangeTopic_CHANGE_TOPIC_HISTORY}, JobExecutionIds: []string{"job-2"},
	}) {
		t.Fatal("another execution refreshed job details")
	}
	if !relevantScreenChange(screen, navigation, &cnpv1.ChangeEvent{
		Topics: []cnpv1.ChangeTopic{cnpv1.ChangeTopic_CHANGE_TOPIC_HISTORY}, JobExecutionIds: []string{"job-1"},
	}) {
		t.Fatal("current execution history change did not refresh job details")
	}
	if relevantScreenChange(screen, navigation, &cnpv1.ChangeEvent{
		Topics: []cnpv1.ChangeTopic{cnpv1.ChangeTopic_CHANGE_TOPIC_HISTORY, cnpv1.ChangeTopic_CHANGE_TOPIC_JOB_OUTPUT}, JobExecutionIds: []string{"job-1"},
	}) {
		t.Fatal("stream-owned output change refreshed job details")
	}
}

func TestNativeRefreshUsesEverySharedScreenTopic(t *testing.T) {
	tests := []struct {
		screen string
		topic  cnpv1.ChangeTopic
	}{
		{screen: "front-page", topic: cnpv1.ChangeTopic_CHANGE_TOPIC_AGENT_ELIGIBILITY},
		{screen: "project-details", topic: cnpv1.ChangeTopic_CHANGE_TOPIC_HISTORY},
		{screen: "agent-script", topic: cnpv1.ChangeTopic_CHANGE_TOPIC_AGENTS},
		{screen: "managed-yaml", topic: cnpv1.ChangeTopic_CHANGE_TOPIC_PROJECTS},
		{screen: "vault", topic: cnpv1.ChangeTopic_CHANGE_TOPIC_VAULT},
	}
	for _, test := range tests {
		t.Run(test.screen, func(t *testing.T) {
			screen, err := sharedui.LoadScreen(test.screen)
			if err != nil {
				t.Fatal(err)
			}
			if !relevantScreenChange(screen, navigationState{screen: test.screen}, &cnpv1.ChangeEvent{Topics: []cnpv1.ChangeTopic{test.topic}}) {
				t.Fatalf("%s did not refresh for its declared %q topic", test.screen, nativeChangeTopicName(test.topic))
			}
		})
	}
}

func TestFrontPageIgnoresHeartbeatOnlyAgentChanges(t *testing.T) {
	screen, err := sharedui.LoadScreen("front-page")
	if err != nil {
		t.Fatal(err)
	}
	if relevantScreenChange(screen, navigationState{screen: "front-page"}, &cnpv1.ChangeEvent{
		Topics: []cnpv1.ChangeTopic{cnpv1.ChangeTopic_CHANGE_TOPIC_AGENTS},
	}) {
		t.Fatal("front page refreshed for a heartbeat-only agent change")
	}
	if !relevantScreenChange(screen, navigationState{screen: "front-page"}, &cnpv1.ChangeEvent{
		Topics: []cnpv1.ChangeTopic{cnpv1.ChangeTopic_CHANGE_TOPIC_AGENT_ELIGIBILITY},
	}) {
		t.Fatal("front page ignored an agent eligibility change")
	}
}
