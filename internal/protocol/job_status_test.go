package protocol

import "testing"

func TestNormalizeJobStatus(t *testing.T) {
	got := NormalizeJobStatus(" Running ")
	if got != JobStatusRunning {
		t.Fatalf("normalize status: got %q want %q", got, JobStatusRunning)
	}
}

func TestStatusPredicates(t *testing.T) {
	if !IsPendingJobStatus(JobStatusQueued) || !IsPendingJobStatus(JobStatusLeased) {
		t.Fatal("pending status predicate should include queued and leased")
	}
	if IsPendingJobStatus(JobStatusRunning) {
		t.Fatal("pending status predicate should exclude running")
	}
	if !IsActiveJobStatus(JobStatusRunning) || !IsActiveJobStatus(JobStatusQueued) || !IsActiveJobStatus(JobStatusLeased) {
		t.Fatal("active status predicate should include queued, leased and running")
	}
	if !IsTerminalJobStatus(JobStatusSucceeded) || !IsTerminalJobStatus(JobStatusFailed) {
		t.Fatal("terminal status predicate should include succeeded and failed")
	}
	if IsTerminalJobStatus(JobStatusRunning) {
		t.Fatal("terminal status predicate should exclude running")
	}
}

func TestIsValidJobUpdateStatus(t *testing.T) {
	if !IsValidJobUpdateStatus(JobStatusRunning) || !IsValidJobUpdateStatus(JobStatusSucceeded) || !IsValidJobUpdateStatus(JobStatusFailed) {
		t.Fatal("valid update status predicate should allow running/succeeded/failed")
	}
	if IsValidJobUpdateStatus(JobStatusQueued) || IsValidJobUpdateStatus(JobStatusLeased) {
		t.Fatal("valid update status predicate should reject queued/leased")
	}
}
