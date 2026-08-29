package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func TestFlushExecutionHistoryRemovesArtifactsBeforeReturning(t *testing.T) {
	_, state := newTestHTTPServerWithState(t)
	job, err := state.db.CreateJobExecution(protocol.CreateJobExecutionRequest{Script: "echo done"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.UpdateJobExecutionStatus(job.ID, protocol.JobExecutionStatusUpdateRequest{
		AgentID: "agent-1", Status: protocol.JobExecutionStatusSucceeded,
	}); err != nil {
		t.Fatal(err)
	}
	artifactDir := filepath.Join(state.artifactsDir, job.ID)
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artifactDir, "result.txt"), []byte("done"), 0o644); err != nil {
		t.Fatal(err)
	}

	deleted, err := (executionMutatorAdapter{state: state}).FlushExecutionHistory(context.Background(), false, []string{job.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 1 || deleted[0] != job.ID {
		t.Fatalf("deleted = %v", deleted)
	}
	if _, err := os.Stat(artifactDir); !os.IsNotExist(err) {
		t.Fatalf("artifact directory still exists after successful flush: %v", err)
	}
}
