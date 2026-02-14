package store

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func TestStoreLeaseJobConcurrencySingleWinner(t *testing.T) {
	s := openTestStore(t)

	job, err := s.CreateJobExecution(protocol.CreateJobExecutionRequest{
		Script:               "echo hello",
		RequiredCapabilities: map[string]string{"os": "linux", "arch": "amd64"},
		TimeoutSeconds:       30,
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	const workers = 24
	var leasedCount int32
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(i int) {
			defer wg.Done()
			agentID := "agent-concurrent-" + string(rune('a'+(i%26)))
			for attempt := 0; attempt < 8; attempt++ {
				j, leaseErr := s.LeaseJobExecution(agentID, map[string]string{"os": "linux", "arch": "amd64"})
				if leaseErr != nil {
					if strings.Contains(strings.ToLower(leaseErr.Error()), "database is locked") {
						time.Sleep(10 * time.Millisecond)
						continue
					}
					t.Errorf("lease error: %v", leaseErr)
					return
				}
				if j != nil {
					atomic.AddInt32(&leasedCount, 1)
				}
				return
			}
		}(i)
	}
	wg.Wait()

	if leasedCount != 1 {
		t.Fatalf("expected exactly one lease winner, got %d", leasedCount)
	}

	got, err := s.GetJobExecution(job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if got.Status != "leased" {
		t.Fatalf("expected job status leased, got %q", got.Status)
	}
	if got.LeasedByAgentID == "" {
		t.Fatal("expected leased_by_agent_id to be set")
	}
}

func TestCapabilitiesMatchToolConstraints(t *testing.T) {
	agentCaps := map[string]string{
		"os":         "linux",
		"arch":       "amd64",
		"executor":   "script",
		"shells":     "posix",
		"tool.go":    "1.25.7",
		"tool.git":   "2.44.0",
		"tool.cmake": "3.28.1",
	}
	req := map[string]string{
		"os":                  "linux",
		"requires.tool.go":    ">=1.24",
		"requires.tool.git":   ">=2.30",
		"requires.tool.cmake": "*",
		"requires.tool.clang": "",
	}
	if capabilitiesMatch(agentCaps, req) {
		t.Fatalf("expected missing clang tool to fail")
	}
	agentCaps["tool.clang"] = "17.0.1"
	if !capabilitiesMatch(agentCaps, req) {
		t.Fatalf("expected constraints to match")
	}
	req["requires.tool.go"] = ">1.26"
	if capabilitiesMatch(agentCaps, req) {
		t.Fatalf("expected go constraint >1.26 to fail")
	}
}

func TestCapabilitiesMatchShellsList(t *testing.T) {
	agentCaps := map[string]string{
		"os":       "windows",
		"arch":     "amd64",
		"executor": "script",
		"shells":   "cmd,powershell",
	}
	req := map[string]string{
		"os":       "windows",
		"executor": "script",
		"shell":    "powershell",
	}
	if !capabilitiesMatch(agentCaps, req) {
		t.Fatalf("expected shell requirement to match via shells list")
	}
}

func TestStoreSaveAndGetJobTestReport(t *testing.T) {
	s := openTestStore(t)

	job, err := s.CreateJobExecution(protocol.CreateJobExecutionRequest{
		Script:         "echo tests",
		TimeoutSeconds: 30,
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	report := protocol.JobExecutionTestReport{
		Total:   2,
		Passed:  1,
		Failed:  1,
		Skipped: 0,
		Suites: []protocol.TestSuiteReport{
			{
				Name:    "go-unit",
				Format:  "go-test-json",
				Total:   2,
				Passed:  1,
				Failed:  1,
				Skipped: 0,
				Cases: []protocol.TestCase{
					{Package: "p", Name: "TestA", Status: "pass"},
					{Package: "p", Name: "TestB", Status: "fail"},
				},
			},
		},
	}
	if err := s.SaveJobExecutionTestReport(job.ID, report); err != nil {
		t.Fatalf("save test report: %v", err)
	}

	got, found, err := s.GetJobExecutionTestReport(job.ID)
	if err != nil {
		t.Fatalf("get test report: %v", err)
	}
	if !found {
		t.Fatal("expected test report to be found")
	}
	if got.Total != 2 || got.Failed != 1 || len(got.Suites) != 1 {
		t.Fatalf("unexpected test report: %+v", got)
	}
}

func TestStoreIgnoresLateRunningAfterSucceeded(t *testing.T) {
	s := openTestStore(t)

	job, err := s.CreateJobExecution(protocol.CreateJobExecutionRequest{
		Script:         "echo done",
		TimeoutSeconds: 30,
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	done, err := s.UpdateJobExecutionStatus(job.ID, protocol.JobExecutionStatusUpdateRequest{
		AgentID: "agent-1",
		Status:  "succeeded",
		Output:  "final output",
	})
	if err != nil {
		t.Fatalf("mark succeeded: %v", err)
	}
	if done.Status != "succeeded" {
		t.Fatalf("expected succeeded, got %q", done.Status)
	}

	got, err := s.UpdateJobExecutionStatus(job.ID, protocol.JobExecutionStatusUpdateRequest{
		AgentID: "agent-1",
		Status:  "running",
		Output:  "late running output",
	})
	if err != nil {
		t.Fatalf("late running update: %v", err)
	}
	if got.Status != "succeeded" {
		t.Fatalf("expected status to remain succeeded, got %q", got.Status)
	}
	if got.Output != "final output" {
		t.Fatalf("expected output to remain terminal output, got %q", got.Output)
	}
}

func TestStoreAgentHasActiveJob(t *testing.T) {
	s := openTestStore(t)

	job, err := s.CreateJobExecution(protocol.CreateJobExecutionRequest{
		Script:               "echo hi",
		RequiredCapabilities: map[string]string{"os": "linux"},
		TimeoutSeconds:       30,
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	leased, err := s.LeaseJobExecution("agent-a", map[string]string{"os": "linux"})
	if err != nil {
		t.Fatalf("lease: %v", err)
	}
	if leased == nil || leased.ID != job.ID {
		t.Fatalf("expected leased job %q", job.ID)
	}

	active, err := s.AgentHasActiveJobExecution("agent-a")
	if err != nil {
		t.Fatalf("AgentHasActiveJobExecution leased: %v", err)
	}
	if !active {
		t.Fatalf("expected active job for agent-a")
	}

	if _, err := s.UpdateJobExecutionStatus(job.ID, protocol.JobExecutionStatusUpdateRequest{
		AgentID: "agent-a",
		Status:  "succeeded",
	}); err != nil {
		t.Fatalf("mark succeeded: %v", err)
	}
	active, err = s.AgentHasActiveJobExecution("agent-a")
	if err != nil {
		t.Fatalf("AgentHasActiveJobExecution succeeded: %v", err)
	}
	if active {
		t.Fatalf("expected no active job after succeeded")
	}
}

func TestStoreConcurrentRunningDoesNotOverrideTerminal(t *testing.T) {
	s := openTestStore(t)

	job, err := s.CreateJobExecution(protocol.CreateJobExecutionRequest{
		Script:         "echo hi",
		TimeoutSeconds: 30,
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	if _, err := s.UpdateJobExecutionStatus(job.ID, protocol.JobExecutionStatusUpdateRequest{
		AgentID: "agent-1",
		Status:  "running",
		Output:  "stream-1",
	}); err != nil {
		t.Fatalf("mark running: %v", err)
	}

	exitCode := 0
	var wg sync.WaitGroup
	var errSucceeded atomic.Value
	var errRunning atomic.Value
	wg.Add(2)
	go func() {
		defer wg.Done()
		for attempt := 0; attempt < 8; attempt++ {
			_, uerr := s.UpdateJobExecutionStatus(job.ID, protocol.JobExecutionStatusUpdateRequest{
				AgentID:  "agent-1",
				Status:   "succeeded",
				ExitCode: &exitCode,
				Output:   "final",
			})
			if uerr != nil && strings.Contains(strings.ToLower(uerr.Error()), "database is locked") {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			if uerr != nil {
				errSucceeded.Store(uerr)
			}
			return
		}
		errSucceeded.Store("update retries exhausted due to database lock")
	}()
	go func() {
		defer wg.Done()
		for attempt := 0; attempt < 8; attempt++ {
			_, uerr := s.UpdateJobExecutionStatus(job.ID, protocol.JobExecutionStatusUpdateRequest{
				AgentID: "agent-1",
				Status:  "running",
				Output:  "late-stream",
			})
			if uerr != nil && strings.Contains(strings.ToLower(uerr.Error()), "database is locked") {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			if uerr != nil {
				errRunning.Store(uerr)
			}
			return
		}
		errRunning.Store("update retries exhausted due to database lock")
	}()
	wg.Wait()

	if v := errSucceeded.Load(); v != nil {
		t.Fatalf("concurrent succeeded update error: %v", v)
	}
	if v := errRunning.Load(); v != nil {
		t.Fatalf("concurrent running update error: %v", v)
	}

	got, err := s.GetJobExecution(job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if got.Status != "succeeded" {
		t.Fatalf("expected succeeded, got %q", got.Status)
	}
	if got.FinishedUTC.IsZero() {
		t.Fatalf("expected finished timestamp to be set")
	}
}

func TestStoreTracksCurrentStepAndClearsOnTerminal(t *testing.T) {
	s := openTestStore(t)

	job, err := s.CreateJobExecution(protocol.CreateJobExecutionRequest{
		Script:         "echo hi",
		TimeoutSeconds: 30,
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	running, err := s.UpdateJobExecutionStatus(job.ID, protocol.JobExecutionStatusUpdateRequest{
		AgentID:     "agent-1",
		Status:      "running",
		Output:      "stream-1",
		CurrentStep: "Step 1/3: checkout source",
	})
	if err != nil {
		t.Fatalf("mark running: %v", err)
	}
	if running.CurrentStep != "Step 1/3: checkout source" {
		t.Fatalf("unexpected running current_step: %q", running.CurrentStep)
	}

	done, err := s.UpdateJobExecutionStatus(job.ID, protocol.JobExecutionStatusUpdateRequest{
		AgentID: "agent-1",
		Status:  "succeeded",
		Output:  "done",
	})
	if err != nil {
		t.Fatalf("mark succeeded: %v", err)
	}
	if done.CurrentStep != "" {
		t.Fatalf("expected current_step to clear on terminal status, got %q", done.CurrentStep)
	}
}

func TestStorePreservesOutputWhenRunningUpdateOmitsOutput(t *testing.T) {
	s := openTestStore(t)

	job, err := s.CreateJobExecution(protocol.CreateJobExecutionRequest{
		Script:         "echo hi",
		TimeoutSeconds: 30,
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	first, err := s.UpdateJobExecutionStatus(job.ID, protocol.JobExecutionStatusUpdateRequest{
		AgentID:     "agent-1",
		Status:      "running",
		Output:      "line-1\nline-2",
		CurrentStep: "Step 1/3: checkout",
	})
	if err != nil {
		t.Fatalf("mark first running: %v", err)
	}
	if first.Output == "" {
		t.Fatalf("expected initial running output to be set")
	}

	second, err := s.UpdateJobExecutionStatus(job.ID, protocol.JobExecutionStatusUpdateRequest{
		AgentID:     "agent-1",
		Status:      "running",
		CurrentStep: "Step 2/3: build",
	})
	if err != nil {
		t.Fatalf("mark second running: %v", err)
	}
	if second.Output != first.Output {
		t.Fatalf("expected running output to be preserved, got=%q want=%q", second.Output, first.Output)
	}
	if second.CurrentStep != "Step 2/3: build" {
		t.Fatalf("expected current_step to update, got %q", second.CurrentStep)
	}
}

func TestIsSQLiteBusyError(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{err: nil, want: false},
		{err: errors.New("database is locked (5) (SQLITE_BUSY)"), want: true},
		{err: errors.New("update failed: SQLITE_BUSY"), want: true},
		{err: errors.New("constraint failed"), want: false},
	}
	for _, tc := range cases {
		if got := isSQLiteBusyError(tc.err); got != tc.want {
			t.Fatalf("isSQLiteBusyError(%v)=%v want=%v", tc.err, got, tc.want)
		}
	}
}

func TestStoreRequeueStaleLeasedJobExecutions(t *testing.T) {
	s := openTestStore(t)

	job, err := s.CreateJobExecution(protocol.CreateJobExecutionRequest{
		Script:               "echo hi",
		RequiredCapabilities: map[string]string{"os": "linux"},
		TimeoutSeconds:       30,
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	leased, err := s.LeaseJobExecution("agent-a", map[string]string{"os": "linux"})
	if err != nil {
		t.Fatalf("lease: %v", err)
	}
	if leased == nil || leased.ID != job.ID {
		t.Fatalf("expected leased job %q", job.ID)
	}

	staleNow := leased.LeasedUTC.Add(2 * time.Minute)
	requeued, err := s.RequeueStaleLeasedJobExecutions(staleNow, 30*time.Second)
	if err != nil {
		t.Fatalf("requeue stale leased jobs: %v", err)
	}
	if requeued != 1 {
		t.Fatalf("expected 1 requeued job, got %d", requeued)
	}

	got, err := s.GetJobExecution(job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if got.Status != protocol.JobExecutionStatusQueued {
		t.Fatalf("expected queued status, got %q", got.Status)
	}
	if got.LeasedByAgentID != "" {
		t.Fatalf("expected lease owner to clear, got %q", got.LeasedByAgentID)
	}
}

func TestStoreFailTimedOutRunningJobExecutions(t *testing.T) {
	s := openTestStore(t)

	job, err := s.CreateJobExecution(protocol.CreateJobExecutionRequest{
		Script:               "echo hi",
		RequiredCapabilities: map[string]string{"os": "linux"},
		TimeoutSeconds:       5,
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	leased, err := s.LeaseJobExecution("agent-a", map[string]string{"os": "linux"})
	if err != nil {
		t.Fatalf("lease: %v", err)
	}
	if leased == nil || leased.ID != job.ID {
		t.Fatalf("expected leased job %q", job.ID)
	}

	startedAt := time.Now().UTC().Add(-20 * time.Second)
	if _, err := s.UpdateJobExecutionStatus(job.ID, protocol.JobExecutionStatusUpdateRequest{
		AgentID:      "agent-a",
		Status:       protocol.JobExecutionStatusRunning,
		CurrentStep:  "Checking out source",
		Output:       "[checkout] repo=example ref=abc",
		TimestampUTC: startedAt,
	}); err != nil {
		t.Fatalf("mark running: %v", err)
	}

	failed, err := s.FailTimedOutRunningJobExecutions(startedAt.Add(20*time.Second), 2*time.Second, "job timed out while running (server maintenance)")
	if err != nil {
		t.Fatalf("fail timed-out running jobs: %v", err)
	}
	if failed != 1 {
		t.Fatalf("expected 1 failed timed-out job, got %d", failed)
	}

	got, err := s.GetJobExecution(job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if got.Status != protocol.JobExecutionStatusFailed {
		t.Fatalf("expected failed status, got %q", got.Status)
	}
	if got.FinishedUTC.IsZero() {
		t.Fatalf("expected finished timestamp to be set")
	}
	if !strings.Contains(got.Error, "timed out") {
		t.Fatalf("expected timeout error, got %q", got.Error)
	}
	if !strings.Contains(got.Output, "[control] job timed out while running (server maintenance)") {
		t.Fatalf("expected timeout control marker in output, got %q", got.Output)
	}
}
