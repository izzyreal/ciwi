package store

import (
	"context"
	"testing"
	"time"

	"github.com/izzyreal/ciwi/internal/config"
	"github.com/izzyreal/ciwi/internal/protocol"
)

func TestStoreAppStateRoundTrip(t *testing.T) {
	s := openTestStore(t)

	if err := s.SetAppState("update.status", "running"); err != nil {
		t.Fatalf("SetAppState running: %v", err)
	}
	if err := s.SetAppState("update.status", "success"); err != nil {
		t.Fatalf("SetAppState success: %v", err)
	}
	v, found, err := s.GetAppState("update.status")
	if err != nil {
		t.Fatalf("GetAppState: %v", err)
	}
	if !found || v != "success" {
		t.Fatalf("unexpected app state found=%v value=%q", found, v)
	}
	_, found, err = s.GetAppState("missing")
	if err != nil {
		t.Fatalf("GetAppState missing: %v", err)
	}
	if found {
		t.Fatalf("missing key should not be found")
	}
	all, err := s.ListAppState()
	if err != nil {
		t.Fatalf("ListAppState: %v", err)
	}
	if all["update.status"] != "success" {
		t.Fatalf("unexpected list app state payload: %v", all)
	}

	if err := s.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestStoreArtifactsAndEventsRoundTrip(t *testing.T) {
	s := openTestStore(t)
	job, err := s.CreateJobExecution(protocol.CreateJobExecutionRequest{Script: "echo hi", TimeoutSeconds: 30})
	if err != nil {
		t.Fatalf("CreateJobExecution: %v", err)
	}

	artifacts := []protocol.JobExecutionArtifact{
		{JobExecutionID: job.ID, Path: "dist/a.txt", URL: job.ID + "/dist/a.txt", SizeBytes: 10},
		{JobExecutionID: job.ID, Path: "dist/b.txt", URL: job.ID + "/dist/b.txt", SizeBytes: 20},
	}
	if err := s.SaveJobExecutionArtifacts(job.ID, artifacts); err != nil {
		t.Fatalf("SaveJobExecutionArtifacts: %v", err)
	}
	gotArtifacts, err := s.ListJobExecutionArtifacts(job.ID)
	if err != nil {
		t.Fatalf("ListJobExecutionArtifacts: %v", err)
	}
	if len(gotArtifacts) != 2 || gotArtifacts[0].Path != "dist/a.txt" || gotArtifacts[1].Path != "dist/b.txt" {
		t.Fatalf("unexpected artifacts: %+v", gotArtifacts)
	}

	ts := time.Now().UTC().Add(-time.Minute)
	events := []protocol.JobExecutionEvent{
		{Type: "step.started", TimestampUTC: ts, Step: &protocol.JobStepPlanItem{Index: 1, Name: "build"}},
		{Type: "step.finished", TimestampUTC: ts.Add(time.Second)},
		{Type: "", TimestampUTC: ts.Add(2 * time.Second)},
	}
	if err := s.AppendJobExecutionEvents(job.ID, events); err != nil {
		t.Fatalf("AppendJobExecutionEvents: %v", err)
	}
	gotEvents, err := s.ListJobExecutionEvents(job.ID)
	if err != nil {
		t.Fatalf("ListJobExecutionEvents: %v", err)
	}
	if len(gotEvents) != 2 {
		t.Fatalf("expected 2 non-empty events, got %d (%+v)", len(gotEvents), gotEvents)
	}
	if gotEvents[0].Step == nil || gotEvents[0].Step.Name != "build" {
		t.Fatalf("expected step payload roundtrip, got %+v", gotEvents[0])
	}
}

func TestStoreMergeJobExecutionEnvAndMetadata(t *testing.T) {
	s := openTestStore(t)
	job, err := s.CreateJobExecution(protocol.CreateJobExecutionRequest{
		Script:         "echo hi",
		TimeoutSeconds: 30,
		Env:            map[string]string{"A": "1", "B": "2"},
		Metadata:       map[string]string{"pipeline_id": "build", "matrix": "linux"},
	})
	if err != nil {
		t.Fatalf("CreateJobExecution: %v", err)
	}

	env, err := s.MergeJobExecutionEnv(job.ID, map[string]string{"B": "22", "C": "3", "A": ""})
	if err != nil {
		t.Fatalf("MergeJobExecutionEnv: %v", err)
	}
	if env["B"] != "22" || env["C"] != "3" {
		t.Fatalf("unexpected merged env: %v", env)
	}
	if _, ok := env["A"]; ok {
		t.Fatalf("expected empty patch value to delete key A")
	}

	meta, err := s.MergeJobExecutionMetadata(job.ID, map[string]string{"pipeline_id": "release", "matrix": ""})
	if err != nil {
		t.Fatalf("MergeJobExecutionMetadata: %v", err)
	}
	if meta["pipeline_id"] != "release" {
		t.Fatalf("expected metadata key update, got %v", meta)
	}
	if _, ok := meta["matrix"]; ok {
		t.Fatalf("expected empty patch value to delete metadata key")
	}
}

func TestStorePipelineLookupAndLoadedCommit(t *testing.T) {
	s := openTestStore(t)
	cfg, err := config.Parse([]byte(`
version: 1
project:
  name: ciwi
pipelines:
  - id: build
    trigger: manual
    source:
      repo: https://github.com/izzyreal/ciwi.git
      ref: main
    jobs:
      - id: compile
        timeout_seconds: 30
        steps:
          - run: go build ./...
pipeline_chains:
  - id: build-and-release
    pipelines:
      - build
`), "pipeline-lookup")
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if err := s.LoadConfig(cfg, "ciwi-project.yaml", "https://github.com/izzyreal/ciwi.git", "main", "ciwi-project.yaml"); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	project, err := s.GetProjectByName("ciwi")
	if err != nil {
		t.Fatalf("GetProjectByName: %v", err)
	}
	projects, err := s.ListProjects()
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(projects) != 1 || len(projects[0].Pipelines) != 1 {
		t.Fatalf("expected one listed project with one pipeline, got %+v", projects)
	}
	pipeline, err := s.GetPipelineByProjectAndID("ciwi", "build")
	if err != nil {
		t.Fatalf("GetPipelineByProjectAndID: %v", err)
	}
	if pipeline.PipelineID != "build" || len(pipeline.Jobs) != 1 {
		t.Fatalf("unexpected persisted pipeline: %+v", pipeline)
	}

	byID, err := s.GetPipelineByDBID(pipeline.DBID)
	if err != nil {
		t.Fatalf("GetPipelineByDBID: %v", err)
	}
	if byID.PipelineID != "build" {
		t.Fatalf("unexpected pipeline-by-id result: %+v", byID)
	}

	detail, err := s.GetProjectDetail(project.ID)
	if err != nil {
		t.Fatalf("GetProjectDetail: %v", err)
	}
	if len(detail.PipelineChains) != 1 {
		t.Fatalf("expected one pipeline chain, got %+v", detail.PipelineChains)
	}
	chain, err := s.GetPipelineChainByDBID(detail.PipelineChains[0].ID)
	if err != nil {
		t.Fatalf("GetPipelineChainByDBID: %v", err)
	}
	if chain.ChainID != "build-and-release" || len(chain.Pipelines) != 1 || chain.Pipelines[0] != "build" {
		t.Fatalf("unexpected pipeline chain: %+v", chain)
	}

	if err := s.SetProjectLoadedCommit(project.ID, "abcdef123"); err != nil {
		t.Fatalf("SetProjectLoadedCommit: %v", err)
	}
	updatedProject, err := s.GetProjectByID(project.ID)
	if err != nil {
		t.Fatalf("GetProjectByID after commit update: %v", err)
	}
	if updatedProject.LoadedCommit != "abcdef123" {
		t.Fatalf("expected loaded_commit update, got %q", updatedProject.LoadedCommit)
	}

	if _, err := s.GetPipelineByProjectAndID("ciwi", "missing"); err == nil {
		t.Fatalf("expected missing pipeline lookup to fail")
	}
	if _, err := s.GetPipelineChainByDBID(-1); err == nil {
		t.Fatalf("expected missing pipeline chain lookup to fail")
	}
}

func TestStoreVaultConnectionLifecycle(t *testing.T) {
	s := openTestStore(t)
	cfg, err := config.Parse([]byte(`
version: 1
project:
  name: ciwi
pipelines:
  - id: build
    jobs:
      - id: compile
        timeout_seconds: 30
        steps:
          - run: echo build
`), "vault-lifecycle")
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if err := s.LoadConfig(cfg, "ciwi-project.yaml", "https://github.com/izzyreal/ciwi.git", "main", "ciwi-project.yaml"); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	project, err := s.GetProjectByName("ciwi")
	if err != nil {
		t.Fatalf("GetProjectByName: %v", err)
	}

	conn, err := s.UpsertVaultConnection(protocol.UpsertVaultConnectionRequest{
		Name:         "ciwi-vault",
		URL:          "http://vault.local:8200",
		AuthMethod:   "approle",
		AppRoleMount: "approle",
		RoleID:       "role",
		SecretIDEnv:  "CIWI_VAULT_SECRET_ID",
	})
	if err != nil {
		t.Fatalf("UpsertVaultConnection: %v", err)
	}
	list, err := s.ListVaultConnections()
	if err != nil {
		t.Fatalf("ListVaultConnections: %v", err)
	}
	if len(list) != 1 || list[0].Name != "ciwi-vault" {
		t.Fatalf("unexpected vault connection list: %+v", list)
	}

	settings, err := s.UpdateProjectVaultSettings(project.ID, protocol.UpdateProjectVaultRequest{
		VaultConnectionID: conn.ID,
		Secrets: []protocol.ProjectSecretSpec{{
			Name: "github_token",
			Path: "kv/data/ciwi",
			Key:  "token",
		}},
	})
	if err != nil {
		t.Fatalf("UpdateProjectVaultSettings: %v", err)
	}
	if settings.VaultConnectionID != conn.ID || settings.VaultConnectionName != "ciwi-vault" {
		t.Fatalf("unexpected vault settings after update: %+v", settings)
	}
	if len(settings.Secrets) != 1 || settings.Secrets[0].Name != "github_token" {
		t.Fatalf("unexpected project secrets in settings: %+v", settings.Secrets)
	}

	if err := s.DeleteVaultConnection(conn.ID); err != nil {
		t.Fatalf("DeleteVaultConnection: %v", err)
	}
	if _, err := s.GetVaultConnectionByID(conn.ID); err == nil {
		t.Fatalf("expected deleted vault connection lookup to fail")
	}
	if err := s.DeleteVaultConnection(conn.ID); err == nil {
		t.Fatalf("expected deleting missing vault connection to fail")
	}
}

func TestPersistedPipelineSortedJobs(t *testing.T) {
	p := PersistedPipeline{Jobs: []PersistedPipelineJob{{ID: "b", Position: 2}, {ID: "a", Position: 1}}}
	sorted := p.SortedJobs()
	if len(sorted) != 2 || sorted[0].ID != "a" || sorted[1].ID != "b" {
		t.Fatalf("unexpected sorted jobs: %+v", sorted)
	}
	if p.Jobs[0].ID != "b" {
		t.Fatalf("SortedJobs should not mutate original order")
	}
}
