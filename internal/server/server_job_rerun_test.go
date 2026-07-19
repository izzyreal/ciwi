package server

import (
	"testing"

	"github.com/izzyreal/ciwi/internal/config"
	"github.com/izzyreal/ciwi/internal/protocol"
)

func TestPrepareJobExecutionRerunRebindsLatestUpstreamAttemptArtifacts(t *testing.T) {
	db := openPipelineChainRuntimeStore(t)
	s := &stateStore{db: db}
	cfg, err := config.Parse([]byte(`
version: 1
project:
  name: ciwi
pipelines:
  - id: codesign
    jobs:
      - id: codesign-job
        runs_on: {os: darwin}
        artifacts: [dist/codesigned/**]
        steps:
          - run: echo codesign
  - id: package
    depends_on: [codesign]
    jobs:
      - id: package-job
        runs_on: {os: darwin}
        steps:
          - run: echo package
`), "rerun-healing")
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if err := db.LoadConfig(cfg, "ciwi-project.yaml", "", "", "ciwi-project.yaml"); err != nil {
		t.Fatalf("load config: %v", err)
	}

	originalUpstream, err := db.CreateJobExecution(protocol.CreateJobExecutionRequest{
		Script: "echo codesign", ArtifactGlobs: []string{"dist/codesigned/**"},
		Metadata: map[string]string{
			"project": "ciwi", "pipeline_id": "codesign", "pipeline_run_id": "run-codesign",
			"pipeline_job_id": "codesign-job", "chain_run_id": "chain-1",
		},
	})
	if err != nil {
		t.Fatalf("create original upstream: %v", err)
	}
	originalUpstream, err = db.UpdateJobExecutionStatus(originalUpstream.ID, protocol.JobExecutionStatusUpdateRequest{AgentID: "agent-1", Status: protocol.JobExecutionStatusFailed})
	if err != nil {
		t.Fatalf("fail original upstream: %v", err)
	}
	retriedUpstream, err := db.CreateJobExecution(protocol.CreateJobExecutionRequest{
		Script: "echo codesign", ArtifactGlobs: []string{"dist/codesigned/**"},
		Metadata: map[string]string{
			"project": "ciwi", "pipeline_id": "codesign", "pipeline_run_id": "run-codesign",
			"pipeline_job_id": "codesign-job", "chain_run_id": "chain-1",
			protocol.JobMetadataAttemptRootJobID: originalUpstream.ID,
			protocol.JobMetadataRerunOfJobID:     originalUpstream.ID,
		},
	})
	if err != nil {
		t.Fatalf("create retried upstream: %v", err)
	}
	retriedUpstream, err = db.UpdateJobExecutionStatus(retriedUpstream.ID, protocol.JobExecutionStatusUpdateRequest{AgentID: "agent-1", Status: protocol.JobExecutionStatusSucceeded})
	if err != nil {
		t.Fatalf("succeed retried upstream: %v", err)
	}

	originalPackage := protocol.JobExecution{
		ID:  "package-original",
		Env: map[string]string{"CIWI_DEP_ARTIFACT_JOB_ID": originalUpstream.ID, "CIWI_DEP_ARTIFACT_JOB_IDS": originalUpstream.ID},
		Metadata: map[string]string{
			"project": "ciwi", "pipeline_id": "package", "pipeline_run_id": "run-package",
			"pipeline_job_id": "package-job", "chain_run_id": "chain-1",
		},
	}
	req := protocol.CreateJobExecutionRequest{Env: map[string]string{
		"CIWI_DEP_ARTIFACT_JOB_ID": originalUpstream.ID, "CIWI_DEP_ARTIFACT_JOB_IDS": originalUpstream.ID,
	}, Metadata: map[string]string{}}
	if err := s.prepareJobExecutionRerun(originalPackage, &req); err != nil {
		t.Fatalf("prepare package rerun: %v", err)
	}
	if got := req.Env["CIWI_DEP_ARTIFACT_JOB_IDS"]; got != retriedUpstream.ID {
		t.Fatalf("expected artifacts to rebind to %q, got %q", retriedUpstream.ID, got)
	}
	if terminated, succeeded, exists := pipelineChainStatus([]protocol.JobExecution{originalUpstream, retriedUpstream}, "chain-1", "codesign"); !exists || !terminated || !succeeded {
		t.Fatalf("expected retried pipeline to heal chain status, exists=%v terminated=%v succeeded=%v", exists, terminated, succeeded)
	}
}
