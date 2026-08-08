package agent

import (
	"context"
	"net/http"

	"github.com/izzyreal/ciwi/internal/protocol"
)

// sourceCheckout is the agent-owned seam for materializing a job source. Git
// is the only implementation today; authenticated or non-Git sources can be
// added without coupling the execution workflow to their transport details.
type sourceCheckout interface {
	Checkout(context.Context, string, protocol.SourceSpec) (string, error)
}

type gitSourceCheckout struct{}

func (gitSourceCheckout) Checkout(ctx context.Context, destination string, source protocol.SourceSpec) (string, error) {
	return checkoutSource(ctx, destination, source)
}

// scriptRunner is deliberately shaped around ciwi's current execution unit.
// It is a dispatch seam for future runner kinds, not a generic process API.
type scriptRunner interface {
	Run(context.Context, scriptRunRequest) error
}

type scriptRunRequest struct {
	Client             *http.Client
	ServerURL          string
	AgentID            string
	JobID              string
	Shell              string
	ExecDir            string
	Script             string
	Container          *executionContainerContext
	Environment        []string
	Output             *syncBuffer
	StepEvent          *protocol.JobStepPlanItem
	Progress           *outputReportState
	DefaultCurrentStep string
	SensitiveValues    []string
	TraceShell         bool
}

type shellScriptRunner struct{}

func (shellScriptRunner) Run(ctx context.Context, request scriptRunRequest) error {
	return runJobScript(
		ctx, request.Client, request.ServerURL, request.AgentID, request.JobID,
		request.Shell, request.ExecDir, request.Script, request.Container,
		request.Environment, request.Output, request.StepEvent, request.Progress,
		request.DefaultCurrentStep, request.SensitiveValues, request.TraceShell,
	)
}

type executionDependencies struct {
	sources sourceCheckout
	scripts scriptRunner
}

func defaultExecutionDependencies() executionDependencies {
	return executionDependencies{sources: gitSourceCheckout{}, scripts: shellScriptRunner{}}
}

func (d executionDependencies) withDefaults() executionDependencies {
	defaults := defaultExecutionDependencies()
	if d.sources == nil {
		d.sources = defaults.sources
	}
	if d.scripts == nil {
		d.scripts = defaults.scripts
	}
	return d
}
