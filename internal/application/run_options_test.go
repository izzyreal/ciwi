package application

import (
	"context"
	"testing"
)

type runOptionsProviderStub struct {
	request RunOptionsRequest
	result  RunOptions
}

func (s *runOptionsProviderStub) GetRunOptions(_ context.Context, request RunOptionsRequest) (RunOptions, error) {
	s.request = request
	return s.result, nil
}

func TestRunOptionsQueriesValidateAndDelegate(t *testing.T) {
	provider := &runOptionsProviderStub{result: RunOptions{TargetKind: RunTargetPipeline, TargetLabel: "build"}}
	queries := NewRunOptionsQueries(provider)
	result, err := queries.GetRunOptions(t.Context(), RunOptionsRequest{PipelineDBID: 7, SourceRef: "refs/heads/main"})
	if err != nil {
		t.Fatal(err)
	}
	if result.TargetLabel != "build" || provider.request.PipelineDBID != 7 {
		t.Fatalf("unexpected result=%+v request=%+v", result, provider.request)
	}
}

func TestRunOptionsQueriesRejectAmbiguousTargets(t *testing.T) {
	queries := NewRunOptionsQueries(&runOptionsProviderStub{})
	for _, request := range []RunOptionsRequest{
		{},
		{PipelineDBID: 1, ProjectID: 2, ChainID: "release"},
		{ProjectID: 2},
		{ChainID: "release"},
	} {
		if _, err := queries.GetRunOptions(t.Context(), request); ErrorKindOf(err) != ErrorInvalidArgument {
			t.Fatalf("request %+v: expected invalid argument, got %v", request, err)
		}
	}
}
