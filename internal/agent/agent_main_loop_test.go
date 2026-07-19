package agent

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func TestAgentHeartbeatStateAckClearsOnlyMatchingValues(t *testing.T) {
	s := &agentHeartbeatState{
		pendingUpdateFailure: "failed once",
		updateInProgress:     true,
		pendingRestartStatus: "restart requested",
	}

	s.ack("different", "restart requested")
	updateFailure, updateInProgress, restartStatus := s.snapshot()
	if updateFailure != "failed once" {
		t.Fatalf("expected mismatched update failure to remain, got %q", updateFailure)
	}
	if !updateInProgress {
		t.Fatalf("expected update_in_progress to remain true")
	}
	if restartStatus != "" {
		t.Fatalf("expected matching restart status to clear, got %q", restartStatus)
	}

	s.ack("failed once", "")
	updateFailure, updateInProgress, restartStatus = s.snapshot()
	if updateFailure != "" {
		t.Fatalf("expected matching update failure to clear, got %q", updateFailure)
	}
	if !updateInProgress {
		t.Fatalf("expected update_in_progress to remain true")
	}
	if restartStatus != "" {
		t.Fatalf("expected restart status to stay cleared, got %q", restartStatus)
	}
}

func TestAgentLoopDepsProcessHeartbeatRefreshAndImmediateActions(t *testing.T) {
	control := &deferredControl{}
	state := &agentHeartbeatState{}
	triggered := 0
	detectCalls := 0
	setCaps := map[string]string(nil)
	updateCalls := 0
	restartCalls := 0
	cacheCalls := 0
	historyCalls := 0

	deps := &agentLoopDeps{
		ctx:              context.Background(),
		workDir:          t.TempDir(),
		restartArgs:      []string{"agent"},
		control:          control,
		heartbeatState:   state,
		triggerHeartbeat: func() { triggered++ },
		detectCapsFn: func() map[string]string {
			detectCalls++
			return map[string]string{"tool.cmake": "3.30"}
		},
		setCapsFn: func(next map[string]string) { setCaps = next },
		selfUpdateFn: func(context.Context, string, string, string, []string) error {
			updateCalls++
			return nil
		},
		requestRestartFn: func() string {
			restartCalls++
			return "restart requested"
		},
		wipeCacheFn: func(string) (string, error) {
			cacheCalls++
			return "cache wiped", nil
		},
		wipeHistoryFn: func(string) (string, error) {
			historyCalls++
			return "history wiped", nil
		},
	}

	deps.processHeartbeat(protocol.HeartbeatResponse{
		RefreshToolsRequested:    true,
		UpdateRequested:          true,
		UpdateTarget:             "v1.2.3",
		UpdateRepository:         "izzyreal/ciwi",
		UpdateAPIBase:            "https://api.github.com",
		RestartRequested:         true,
		WipeCacheRequested:       true,
		FlushJobHistoryRequested: true,
	})

	if detectCalls != 1 {
		t.Fatalf("expected one capability refresh, got %d", detectCalls)
	}
	if !reflect.DeepEqual(setCaps, map[string]string{"tool.cmake": "3.30"}) {
		t.Fatalf("unexpected set capabilities payload: %#v", setCaps)
	}
	if updateCalls != 1 || restartCalls != 1 || cacheCalls != 1 || historyCalls != 1 {
		t.Fatalf("unexpected action counts update=%d restart=%d cache=%d history=%d", updateCalls, restartCalls, cacheCalls, historyCalls)
	}
	if triggered != 3 {
		t.Fatalf("expected 3 heartbeat triggers (refresh, update, restart), got %d", triggered)
	}
	updateFailure, updateInProgress, restartStatus := state.snapshot()
	if updateFailure != "" || !updateInProgress || restartStatus != "restart requested" {
		t.Fatalf("unexpected heartbeat state after immediate actions: failure=%q in_progress=%v restart=%q", updateFailure, updateInProgress, restartStatus)
	}
}

func TestAgentLoopDepsHandleHeartbeatResultFlushesDeferredWhenIdle(t *testing.T) {
	control := &deferredControl{
		pendingRestart:        true,
		pendingCacheWipe:      true,
		pendingJobHistoryWipe: true,
	}
	state := &agentHeartbeatState{
		pendingUpdateFailure: "failed once",
		pendingRestartStatus: "restart deferred",
	}
	events := make([]string, 0, 3)
	deps := &agentLoopDeps{
		ctx:            context.Background(),
		workDir:        t.TempDir(),
		control:        control,
		heartbeatState: state,
		requestRestartFn: func() string {
			events = append(events, "restart")
			return "restart requested"
		},
		wipeCacheFn: func(string) (string, error) {
			events = append(events, "cache")
			return "cache wiped", nil
		},
		wipeHistoryFn: func(string) (string, error) {
			events = append(events, "history")
			return "history wiped", nil
		},
		triggerHeartbeat: func() {},
		detectCapsFn:     func() map[string]string { return nil },
		setCapsFn:        func(map[string]string) {},
		selfUpdateFn:     func(context.Context, string, string, string, []string) error { return nil },
	}

	deps.handleHeartbeatResult(heartbeatResult{
		resp:              protocol.HeartbeatResponse{},
		sentUpdateFailure: "failed once",
		sentRestartStatus: "restart deferred",
	})

	if !reflect.DeepEqual(events, []string{"restart", "cache", "history"}) {
		t.Fatalf("unexpected deferred flush events: got=%v", events)
	}
	updateFailure, _, restartStatus := state.snapshot()
	if updateFailure != "" {
		t.Fatalf("expected matching update failure ack to clear state, got %q", updateFailure)
	}
	if restartStatus != "restart requested" {
		t.Fatalf("expected restart status to be updated by deferred restart, got %q", restartStatus)
	}
	if control.hasDeferred() || control.jobInProgress {
		t.Fatalf("expected deferred control to be cleared after flush: %+v", control)
	}
}

func TestAgentLoopDepsHandleLeaseTickBusySkipsLease(t *testing.T) {
	control := &deferredControl{jobInProgress: true}
	calledLease := false
	deps := &agentLoopDeps{
		ctx:            context.Background(),
		control:        control,
		heartbeatState: &agentHeartbeatState{},
		getCapsFn:      func() map[string]string { return nil },
		leaseJobFn: func(context.Context, *http.Client, string, string, map[string]string) (*protocol.JobExecution, error) {
			calledLease = true
			return nil, nil
		},
	}

	deps.handleLeaseTick()
	if calledLease {
		t.Fatalf("expected busy lease tick to skip lease request")
	}
}

func TestAgentLoopDepsHandleLeaseTickFlushesDeferredInsteadOfLeasing(t *testing.T) {
	control := &deferredControl{
		pendingUpdate: &pendingUpdateRequest{target: "v1.2.3", repository: "izzyreal/ciwi", apiBase: "https://api.github.com"},
	}
	events := make([]string, 0, 2)
	deps := &agentLoopDeps{
		ctx:            context.Background(),
		workDir:        t.TempDir(),
		restartArgs:    []string{"agent"},
		control:        control,
		heartbeatState: &agentHeartbeatState{},
		triggerHeartbeat: func() {
			events = append(events, "trigger")
		},
		detectCapsFn: func() map[string]string { return nil },
		getCapsFn:    func() map[string]string { return map[string]string{"executor": "script"} },
		setCapsFn:    func(map[string]string) {},
		selfUpdateFn: func(context.Context, string, string, string, []string) error {
			events = append(events, "update")
			return nil
		},
		requestRestartFn: func() string { return "" },
		wipeCacheFn:      func(string) (string, error) { return "", nil },
		wipeHistoryFn:    func(string) (string, error) { return "", nil },
		leaseJobFn: func(context.Context, *http.Client, string, string, map[string]string) (*protocol.JobExecution, error) {
			events = append(events, "lease")
			return nil, nil
		},
		executeJobFn: func(context.Context, *http.Client, string, string, string, map[string]string, protocol.JobExecution) error {
			return nil
		},
		jobDoneCh: make(chan jobResult, 1),
	}

	deps.handleLeaseTick()
	if !reflect.DeepEqual(events, []string{"trigger", "update"}) {
		t.Fatalf("unexpected deferred+lease ordering: got=%v", events)
	}
}

func TestAgentLoopDepsHandleLeaseTickProcessesFreshHeartbeatBeforeLeasing(t *testing.T) {
	control := &deferredControl{}
	events := make([]string, 0, 4)
	deps := &agentLoopDeps{
		ctx:            context.Background(),
		workDir:        t.TempDir(),
		restartArgs:    []string{"agent"},
		control:        control,
		heartbeatState: &agentHeartbeatState{},
		triggerHeartbeat: func() {
			events = append(events, "trigger")
		},
		detectCapsFn: func() map[string]string { return nil },
		getCapsFn:    func() map[string]string { return map[string]string{"executor": "script"} },
		setCapsFn:    func(map[string]string) {},
		selfUpdateFn: func(context.Context, string, string, string, []string) error {
			events = append(events, "update")
			return nil
		},
		requestRestartFn: func() string { return "" },
		wipeCacheFn:      func(string) (string, error) { return "", nil },
		wipeHistoryFn:    func(string) (string, error) { return "", nil },
		heartbeatNowFn: func() heartbeatResult {
			events = append(events, "heartbeat")
			return heartbeatResult{
				resp: protocol.HeartbeatResponse{
					UpdateRequested:  true,
					UpdateTarget:     "v1.2.3",
					UpdateRepository: "izzyreal/ciwi",
					UpdateAPIBase:    "https://api.github.com",
				},
			}
		},
		leaseJobFn: func(context.Context, *http.Client, string, string, map[string]string) (*protocol.JobExecution, error) {
			events = append(events, "lease")
			return nil, nil
		},
		executeJobFn: func(context.Context, *http.Client, string, string, string, map[string]string, protocol.JobExecution) error {
			return nil
		},
		jobDoneCh: make(chan jobResult, 1),
	}

	deps.handleLeaseTick()
	if !reflect.DeepEqual(events, []string{"heartbeat", "trigger", "update"}) {
		t.Fatalf("unexpected pre-lease heartbeat ordering: got=%v", events)
	}
}

func TestAgentLoopDepsHandleLeaseTickSchedulesJobExecution(t *testing.T) {
	control := &deferredControl{}
	jobDoneCh := make(chan jobResult, 1)
	gotCapsCh := make(chan map[string]string, 1)
	job := &protocol.JobExecution{ID: "job-123"}
	deps := &agentLoopDeps{
		ctx:            context.Background(),
		serverURL:      "http://ciwi.local",
		agentID:        "agent-1",
		workDir:        t.TempDir(),
		control:        control,
		heartbeatState: &agentHeartbeatState{},
		getCapsFn:      func() map[string]string { return map[string]string{"executor": "script"} },
		leaseJobFn: func(context.Context, *http.Client, string, string, map[string]string) (*protocol.JobExecution, error) {
			return job, nil
		},
		executeJobFn: func(context.Context, *http.Client, string, string, string, map[string]string, protocol.JobExecution) error {
			gotCapsCh <- map[string]string{"executor": "script"}
			return errors.New("job failed")
		},
		jobDoneCh: jobDoneCh,
	}

	deps.handleLeaseTick()
	if !control.jobInProgress {
		t.Fatalf("expected control to mark job in progress once leased")
	}
	select {
	case got := <-gotCapsCh:
		if !reflect.DeepEqual(got, map[string]string{"executor": "script"}) {
			t.Fatalf("unexpected job capability snapshot: %#v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for execute job invocation")
	}
	select {
	case done := <-jobDoneCh:
		if done.jobID != "job-123" {
			t.Fatalf("unexpected done job id: %q", done.jobID)
		}
		if done.err == nil || done.err.Error() != "job failed" {
			t.Fatalf("unexpected job result error: %v", done.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for job result")
	}
}
