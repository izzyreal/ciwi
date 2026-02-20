package jobexecution

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func TestPersistCoverageReportArtifactCreateAndRemove(t *testing.T) {
	root := t.TempDir()
	jobID := "job-1"

	withCoverage := protocol.JobExecutionTestReport{
		Coverage: &protocol.CoverageReport{
			Format:            "go-coverprofile",
			TotalStatements:   10,
			CoveredStatements: 8,
			Percent:           80,
		},
	}
	if err := PersistCoverageReportArtifact(root, jobID, withCoverage); err != nil {
		t.Fatalf("PersistCoverageReportArtifact create: %v", err)
	}
	coveragePath := filepath.Join(root, jobID, coverageReportArtifactPath)
	if _, err := os.Stat(coveragePath); err != nil {
		t.Fatalf("expected coverage artifact to exist: %v", err)
	}

	withoutCoverage := protocol.JobExecutionTestReport{}
	if err := PersistCoverageReportArtifact(root, jobID, withoutCoverage); err != nil {
		t.Fatalf("PersistCoverageReportArtifact remove: %v", err)
	}
	if _, err := os.Stat(coveragePath); !os.IsNotExist(err) {
		t.Fatalf("expected coverage artifact removed, stat err=%v", err)
	}
}

func TestAppendSyntheticArtifacts(t *testing.T) {
	root := t.TempDir()
	jobID := "job-1"
	base := filepath.Join(root, jobID)
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatalf("mkdir base: %v", err)
	}
	if err := os.WriteFile(filepath.Join(base, testReportArtifactPath), []byte(`{"total":1}`), 0o644); err != nil {
		t.Fatalf("write test report artifact: %v", err)
	}
	if err := os.WriteFile(filepath.Join(base, coverageReportArtifactPath), []byte(`{"percent":99}`), 0o644); err != nil {
		t.Fatalf("write coverage artifact: %v", err)
	}

	artifacts := []protocol.JobExecutionArtifact{
		{JobExecutionID: jobID, Path: "dist/app.bin", URL: jobID + "/dist/app.bin", SizeBytes: 3},
	}
	withTest := AppendSyntheticTestReportArtifact(root, jobID, artifacts)
	if len(withTest) != 2 {
		t.Fatalf("expected synthetic test report artifact appended, got %d items", len(withTest))
	}
	withBoth := AppendSyntheticCoverageReportArtifact(root, jobID, withTest)
	if len(withBoth) != 3 {
		t.Fatalf("expected synthetic coverage artifact appended, got %d items", len(withBoth))
	}

	withBothAgain := AppendSyntheticCoverageReportArtifact(root, jobID, withBoth)
	if len(withBothAgain) != len(withBoth) {
		t.Fatalf("expected duplicate synthetic coverage append to be ignored")
	}
	missingJob := AppendSyntheticTestReportArtifact(root, "job-missing", artifacts)
	if len(missingJob) != len(artifacts) {
		t.Fatalf("expected missing synthetic source to keep artifact list unchanged")
	}
}

func TestViewFromProtocolTimestamps(t *testing.T) {
	now := time.Now().UTC().Round(0)
	job := protocol.JobExecution{
		ID:         "job-1",
		Status:     protocol.JobExecutionStatusRunning,
		CreatedUTC: now,
	}
	view := ViewFromProtocol(job)
	if view.StartedUTC != nil || view.FinishedUTC != nil || view.LeasedUTC != nil {
		t.Fatalf("expected zero timestamps to remain nil in view")
	}

	started := now.Add(time.Second)
	finished := now.Add(2 * time.Second)
	leased := now.Add(3 * time.Second)
	job.StartedUTC = started
	job.FinishedUTC = finished
	job.LeasedUTC = leased
	view = ViewFromProtocol(job)
	if view.StartedUTC == nil || !view.StartedUTC.Equal(started) {
		t.Fatalf("expected started timestamp pointer, got %v", view.StartedUTC)
	}
	if view.FinishedUTC == nil || !view.FinishedUTC.Equal(finished) {
		t.Fatalf("expected finished timestamp pointer, got %v", view.FinishedUTC)
	}
	if view.LeasedUTC == nil || !view.LeasedUTC.Equal(leased) {
		t.Fatalf("expected leased timestamp pointer, got %v", view.LeasedUTC)
	}
}
