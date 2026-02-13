package protocol

import "testing"

func TestNormalizeJobExecutionStatus(t *testing.T) {
	got := NormalizeJobExecutionStatus(" Running ")
	if got != JobExecutionStatusRunning {
		t.Fatalf("normalize status: got %q want %q", got, JobExecutionStatusRunning)
	}
}

func TestJobExecutionStatusPredicates(t *testing.T) {
	if !IsPendingJobExecutionStatus(JobExecutionStatusQueued) || !IsPendingJobExecutionStatus(JobExecutionStatusLeased) {
		t.Fatal("pending status predicate should include queued and leased")
	}
	if IsPendingJobExecutionStatus(JobExecutionStatusRunning) {
		t.Fatal("pending status predicate should exclude running")
	}
	if !IsActiveJobExecutionStatus(JobExecutionStatusRunning) || !IsActiveJobExecutionStatus(JobExecutionStatusQueued) || !IsActiveJobExecutionStatus(JobExecutionStatusLeased) {
		t.Fatal("active status predicate should include queued, leased and running")
	}
	if !IsTerminalJobExecutionStatus(JobExecutionStatusSucceeded) || !IsTerminalJobExecutionStatus(JobExecutionStatusFailed) {
		t.Fatal("terminal status predicate should include succeeded and failed")
	}
	if IsTerminalJobExecutionStatus(JobExecutionStatusRunning) {
		t.Fatal("terminal status predicate should exclude running")
	}
}

func TestIsValidJobExecutionUpdateStatus(t *testing.T) {
	if !IsValidJobExecutionUpdateStatus(JobExecutionStatusRunning) || !IsValidJobExecutionUpdateStatus(JobExecutionStatusSucceeded) || !IsValidJobExecutionUpdateStatus(JobExecutionStatusFailed) {
		t.Fatal("valid update status predicate should allow running/succeeded/failed")
	}
	if IsValidJobExecutionUpdateStatus(JobExecutionStatusQueued) || IsValidJobExecutionUpdateStatus(JobExecutionStatusLeased) {
		t.Fatal("valid update status predicate should reject queued/leased")
	}
}
