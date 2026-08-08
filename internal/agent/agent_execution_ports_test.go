package agent

import (
	"context"
	"testing"

	"github.com/izzyreal/ciwi/internal/protocol"
)

type stubSourceCheckout struct{}

func (stubSourceCheckout) Checkout(context.Context, string, protocol.SourceSpec) (string, error) {
	return "stub", nil
}

type stubScriptRunner struct{}

func (stubScriptRunner) Run(context.Context, scriptRunRequest) error { return nil }

func TestExecutionDependenciesDefaultOnlyMissingPorts(t *testing.T) {
	sources := stubSourceCheckout{}
	scripts := stubScriptRunner{}
	dependencies := (executionDependencies{sources: sources, scripts: scripts}).withDefaults()
	if _, ok := dependencies.sources.(stubSourceCheckout); !ok {
		t.Fatalf("source checkout was replaced: %T", dependencies.sources)
	}
	if _, ok := dependencies.scripts.(stubScriptRunner); !ok {
		t.Fatalf("script runner was replaced: %T", dependencies.scripts)
	}

	defaults := (executionDependencies{}).withDefaults()
	if _, ok := defaults.sources.(gitSourceCheckout); !ok {
		t.Fatalf("default source checkout = %T", defaults.sources)
	}
	if _, ok := defaults.scripts.(shellScriptRunner); !ok {
		t.Fatalf("default script runner = %T", defaults.scripts)
	}
}
