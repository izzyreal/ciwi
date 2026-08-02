package application

import (
	"context"
	"testing"
)

type pipelineChainRunnerStub struct {
	calls   int
	request RunPipelineChainRequest
}

func (s *pipelineChainRunnerStub) RunPipelineChain(_ context.Context, request RunPipelineChainRequest) (RunPipelineChainResult, error) {
	s.calls++
	s.request = request
	return RunPipelineChainResult{ProjectName: "ciwi", ChainID: request.ChainID, ChainName: "Build and release", Enqueued: 3, JobExecutionIDs: []string{"job-1", "job-2", "job-3"}}, nil
}

func TestPipelineChainCommandsDeduplicatesRun(t *testing.T) {
	runner := &pipelineChainRunnerStub{}
	hub := NewChangeHub()
	commands := NewPipelineChainCommands(runner, newReceiptRepositoryStub(), hub)
	request := RunPipelineChainRequest{ProjectID: 7, ChainID: " build+release ", DryRun: true, IdempotencyKey: "chain-1"}
	first, err := commands.RunPipelineChain(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := commands.RunPipelineChain(t.Context(), request)
	if err != nil || second.Enqueued != first.Enqueued || runner.calls != 1 {
		t.Fatalf("results = %#v, %#v; calls = %d; err = %v", first, second, runner.calls, err)
	}
	if runner.request.ChainID != "build+release" || hub.Snapshot().Revision != 1 {
		t.Fatalf("request = %#v; revision = %d", runner.request, hub.Snapshot().Revision)
	}
}

func TestPipelineChainCommandsValidateIdentity(t *testing.T) {
	commands := NewPipelineChainCommands(&pipelineChainRunnerStub{}, nil, nil)
	for _, request := range []RunPipelineChainRequest{{ChainID: "build"}, {ProjectID: 1}} {
		if _, err := commands.RunPipelineChain(t.Context(), request); ErrorKindOf(err) != ErrorInvalidArgument {
			t.Fatalf("RunPipelineChain(%#v) error = %v", request, err)
		}
	}
}
