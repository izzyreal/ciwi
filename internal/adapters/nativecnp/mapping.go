package nativecnp

import (
	"fmt"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/application"
	"github.com/izzyreal/ciwi/internal/domain"
	"github.com/izzyreal/ciwi/internal/presentation"
	"github.com/izzyreal/ciwi/internal/protocol"
	cnpv1 "github.com/izzyreal/ciwi/pkg/cnp/v1"
)

func serverInfoToProto(info domain.ServerInfo) *cnpv1.ServerInfo {
	return &cnpv1.ServerInfo{
		Name: info.Name, ApiVersion: uint32(info.APIVersion), Version: info.Version, Hostname: info.Hostname,
		InstallationId: info.InstallationID,
	}
}

func serverUpdateStatusToProto(status application.ServerUpdateStatus) *cnpv1.ServerUpdateStatus {
	return &cnpv1.ServerUpdateStatus{
		CurrentVersion: status.CurrentVersion, LatestVersion: status.LatestVersion, UpdateAvailable: status.UpdateAvailable,
		LastCheckedUtc: status.LastCheckedUTC, LastApplyStatus: status.LastApplyStatus, LastApplyUtc: status.LastApplyUTC,
		Message: status.Message, ServerMode: status.ServerMode, SelfUpdateSupported: status.SelfUpdateSupported,
		SelfUpdateReason: status.SelfUpdateReason, AgentTargetVersion: status.AgentTargetVersion,
		BlockedAgentIds: append([]string(nil), status.BlockedAgentIDs...),
	}
}

func projectDetailsToProto(view presentation.ProjectDetailsView) *cnpv1.ProjectDetailsView {
	projects := projectsToProto([]domain.Project{view.Project})
	var project *cnpv1.ProjectSummary
	if len(projects) > 0 {
		project = projects[0]
	}
	pipelines := make([]*cnpv1.ProjectPipelineDetails, 0, len(view.Pipelines))
	for _, pipeline := range view.Pipelines {
		jobs := make([]*cnpv1.ProjectJobDetails, 0, len(pipeline.Jobs))
		for _, job := range pipeline.Jobs {
			steps := make([]*cnpv1.ProjectStepDetails, 0, len(job.Steps))
			for _, step := range job.Steps {
				steps = append(steps, &cnpv1.ProjectStepDetails{
					Index: uint32(step.Index), Position: uint32(step.Position), Name: step.Name,
					Type: step.Type, Command: step.Command, SkipDryRun: step.SkipDryRun,
					Environment: append([]string{}, step.Environment...),
				})
			}
			jobs = append(jobs, &cnpv1.ProjectJobDetails{
				Id: job.ID, Needs: append([]string{}, job.Needs...), NeedsLabel: job.NeedsLabel,
				RunsOnLabel: job.RunsOnLabel, ToolsLabel: job.ToolsLabel,
				TimeoutSeconds: uint32(max(job.TimeoutSeconds, 0)), MatrixCount: uint32(max(job.MatrixCount, 0)),
				StepsCount: uint32(max(job.StepsCount, 0)), SupportsDryRun: job.SupportsDryRun, Steps: steps,
			})
		}
		pipelines = append(pipelines, &cnpv1.ProjectPipelineDetails{
			Id: pipeline.ID, PipelineId: pipeline.PipelineID, Trigger: pipeline.Trigger,
			DependsOn: append([]string{}, pipeline.DependsOn...), Dependencies: pipeline.Dependencies,
			JobsCount: uint32(max(pipeline.JobsCount, 0)), SupportsDryRun: pipeline.SupportsDryRun, Jobs: jobs,
		})
	}
	return &cnpv1.ProjectDetailsView{Project: project, Pipelines: pipelines, HistoryExecutions: executionCardsToProto(view.HistoryExecutions)}
}

func jobDetailsToProto(view presentation.JobDetailsView) *cnpv1.JobDetailsView {
	timeline := make([]*cnpv1.JobTimelineItem, 0, len(view.Timeline))
	for _, item := range view.Timeline {
		timeline = append(timeline, &cnpv1.JobTimelineItem{
			Id: item.ID, Kind: item.Kind, Title: item.Title, Description: item.Description,
			Status: item.Status, StatusLabel: item.StatusLabel, Duration: item.Duration,
			ExitCode: item.ExitCode, Error: item.Error, Progress: progressToProto(item.Progress),
		})
	}
	outputGroups := make([]*cnpv1.JobOutputGroup, 0, len(view.OutputGroups))
	for _, group := range view.OutputGroups {
		outputGroups = append(outputGroups, &cnpv1.JobOutputGroup{
			Id: group.ID, StateKey: group.StateKey, Kind: group.Kind, Title: group.Title,
			CommandSummary: group.CommandSummary, Status: group.Status, StatusLabel: group.StatusLabel,
			Reached: group.Reached, Started: group.Started, Duration: group.Duration,
			ExitCode: group.ExitCode, Error: group.Error, Details: group.Details,
			YamlLiteral: group.YAMLLiteral, ExpandedCommand: group.ExpandedCommand,
			Progress: progressToProto(group.Progress),
		})
	}
	return &cnpv1.JobDetailsView{
		Id: view.ID, ProjectId: view.ProjectID, Title: view.Title, Context: view.Context, Status: view.Status, StatusLabel: view.StatusLabel,
		CurrentStep: view.CurrentStep, Agent: view.Agent, Mode: view.Mode, Created: view.Created,
		Started: view.Started, Finished: view.Finished, Duration: view.Duration, ExitCode: view.ExitCode,
		Error: view.Error, Timeline: timeline, CanCancel: view.CanCancel, CanRerun: view.CanRerun,
		OutputGroups: outputGroups, SchedulingDiagnosis: presentedSchedulingDiagnosisToProto(view),
		Progress: progressToProto(view.Progress), JobProperties: jobDetailRowsToProto(view.JobProperties),
		CacheStatistics: jobDetailRowsToProto(view.CacheStatistics), CacheStatisticsEmpty: view.CacheStatisticsEmpty,
		HostToolRequirements:      toolRequirementsToProto(view.HostToolRequirements),
		ContainerToolRequirements: toolRequirementsToProto(view.ContainerToolRequirements),
		ReleaseSummary:            jobDetailRowsToProto(view.ReleaseSummary), HasReleaseSummary: view.HasReleaseSummary,
	}
}

func jobDetailRowsToProto(rows []presentation.JobDetailRowView) []*cnpv1.JobDetailRow {
	result := make([]*cnpv1.JobDetailRow, 0, len(rows))
	for _, row := range rows {
		result = append(result, &cnpv1.JobDetailRow{Label: row.Label, Value: row.Value, Tone: row.Tone})
	}
	return result
}

func toolRequirementsToProto(view presentation.ToolRequirementsView) *cnpv1.ToolRequirements {
	return &cnpv1.ToolRequirements{
		EmptyLabel: view.EmptyLabel, Summary: view.Summary, Tone: view.Tone,
		Issues: append([]string(nil), view.Issues...),
	}
}

func jobRunContextToProto(view protocol.JobExecutionGraphContext) *cnpv1.JobRunContext {
	pipelines := make([]*cnpv1.JobRunContextPipeline, 0, len(view.Pipelines))
	for _, pipeline := range view.Pipelines {
		jobs := make([]*cnpv1.JobRunContextJob, 0, len(pipeline.Jobs))
		for _, job := range pipeline.Jobs {
			executions := make([]*cnpv1.JobRunContextExecution, 0, len(job.Executions))
			for _, execution := range job.Executions {
				matrixLabel := strings.TrimSpace(execution.MatrixName)
				if matrixLabel == "" && strings.TrimSpace(execution.MatrixIndex) != "" {
					matrixLabel = "index-" + strings.TrimSpace(execution.MatrixIndex)
				}
				if matrixLabel == "" {
					matrixLabel = "job"
				}
				attemptLabel := "Previous attempt"
				if execution.LatestAttempt {
					attemptLabel = "Latest attempt"
				}
				executions = append(executions, &cnpv1.JobRunContextExecution{
					Id: execution.ID, Status: execution.Status, MatrixLabel: matrixLabel,
					AttemptLabel: attemptLabel, Current: execution.ID == view.CurrentExecutionID,
					LatestAttempt: execution.LatestAttempt,
				})
			}
			jobs = append(jobs, &cnpv1.JobRunContextJob{
				Id: job.PipelineJobID, Needs: append([]string(nil), job.Needs...), Status: job.Status,
				SummaryLabel: fmt.Sprintf("%s · %d execution(s)", job.Status, len(job.Executions)), Executions: executions,
			})
		}
		pipelines = append(pipelines, &cnpv1.JobRunContextPipeline{
			Id: pipeline.PipelineDBID, PipelineId: pipeline.PipelineID, DependsOn: append([]string(nil), pipeline.DependsOn...),
			Status: pipeline.Status, SummaryLabel: fmt.Sprintf("%s · %d job(s)", pipeline.Status, len(pipeline.Jobs)), Jobs: jobs,
		})
	}
	scopeLabel := strings.TrimSpace(view.Scope)
	if scopeLabel != "" {
		scopeLabel = strings.ToUpper(scopeLabel[:1]) + scopeLabel[1:] + " run"
	}
	return &cnpv1.JobRunContext{
		Available: len(pipelines) > 0, Scope: view.Scope, ScopeLabel: scopeLabel,
		CurrentExecutionId: view.CurrentExecutionID, CurrentPipelineId: view.CurrentPipelineID,
		CurrentPipelineJobId: view.CurrentPipelineJobID, Pipelines: pipelines,
	}
}

func jobOutputToProto(view presentation.JobOutputView) *cnpv1.JobOutputBatch {
	events := make([]*cnpv1.JobOutputEvent, 0, len(view.Events))
	for _, event := range view.Events {
		events = append(events, &cnpv1.JobOutputEvent{
			EventId: event.EventID, Type: event.Type, ItemId: event.ItemID, Text: event.Text,
			Error: event.Error, ExitCode: event.ExitCode,
		})
	}
	return &cnpv1.JobOutputBatch{
		JobExecutionId: view.JobExecutionID, NextEventId: view.NextEventID,
		Events: events, HasMore: view.HasMore, Terminal: view.Terminal,
	}
}

func projectsToProto(projects []domain.Project) []*cnpv1.ProjectSummary {
	out := make([]*cnpv1.ProjectSummary, 0, len(projects))
	for _, project := range projects {
		pipelines := make([]*cnpv1.PipelineSummary, 0, len(project.Pipelines))
		for _, pipeline := range project.Pipelines {
			pipelines = append(pipelines, &cnpv1.PipelineSummary{
				Id: pipeline.ID, PipelineId: pipeline.PipelineID, Trigger: pipeline.Trigger,
				DependsOn: append([]string(nil), pipeline.DependsOn...), SourceRepo: pipeline.SourceRepo,
				SourceRef: pipeline.SourceRef, SupportsDryRun: pipeline.SupportsDryRun,
			})
		}
		chains := make([]*cnpv1.PipelineChainSummary, 0, len(project.PipelineChains))
		for _, chain := range project.PipelineChains {
			chains = append(chains, &cnpv1.PipelineChainSummary{
				Id: chain.ID, Name: chain.Name, Pipelines: append([]string(nil), chain.Pipelines...),
				SupportsDryRun: chain.SupportsDryRun, VersionPipelineId: chain.VersionPipelineID,
				SequenceLabel: presentation.PipelineChainSequenceLabel(chain.Pipelines),
			})
		}
		updated := int64(0)
		if !project.UpdatedUTC.IsZero() {
			updated = project.UpdatedUTC.UnixMilli()
		}
		out = append(out, &cnpv1.ProjectSummary{
			Id: project.ID, Name: project.Name, SourceKind: project.SourceKind, ConfigPath: project.ConfigPath,
			RepoUrl: project.RepoURL, RepoRef: project.RepoRef, ConfigFile: project.ConfigFile,
			LoadedCommit: project.LoadedCommit, UpdatedUnixMs: updated, Pipelines: pipelines, PipelineChains: chains,
		})
	}
	return out
}

func executionCardsToProto(cards []domain.ExecutionCard) []*cnpv1.ExecutionCardSummary {
	out := make([]*cnpv1.ExecutionCardSummary, 0, len(cards))
	for _, card := range cards {
		out = append(out, &cnpv1.ExecutionCardSummary{
			Key: card.Key, Kind: card.Kind, Title: card.Title,
			JobExecutionIds: append([]string(nil), card.JobExecutionIDs...),
			Summary: &cnpv1.ExecutionSummary{
				TotalJobs: uint32(card.Summary.TotalJobs), Succeeded: uint32(card.Summary.Succeeded),
				Failed: uint32(card.Summary.Failed), InProgress: uint32(card.Summary.InProgress),
				Waiting: uint32(card.Summary.Waiting),
			},
			Sections: executionCardSectionsToProto(card.Sections), Progress: progressToProto(card.Progress),
		})
	}
	return out
}

func executionCardSectionsToProto(sections []domain.ExecutionCardSection) []*cnpv1.ExecutionCardSection {
	out := make([]*cnpv1.ExecutionCardSection, 0, len(sections))
	for _, section := range sections {
		jobs := make([]*cnpv1.ExecutionCardJob, 0, len(section.Jobs))
		for _, job := range section.Jobs {
			jobs = append(jobs, &cnpv1.ExecutionCardJob{
				Id: job.ID, ProjectId: job.ProjectID, Label: job.Label, Status: job.Status, CurrentStep: job.CurrentStep,
				PipelineId: job.PipelineID, BuildLabel: job.BuildLabel, AgentId: job.AgentID,
				CreatedUtc: formatProtoTime(job.CreatedUTC), StartedUtc: formatProtoTime(job.StartedUTC),
				FinishedUtc: formatProtoTime(job.FinishedUTC), Reason: job.Reason, Action: job.Action,
				SchedulingDiagnosis: schedulingDiagnosisToProto(job.SchedulingDiagnosis), Progress: progressToProto(job.Progress),
			})
		}
		out = append(out, &cnpv1.ExecutionCardSection{
			Key: section.Key, Label: section.Label, Jobs: jobs, Progress: progressToProto(section.Progress),
		})
	}
	return out
}

func progressToProto(progress domain.Progress) *cnpv1.Progress {
	return &cnpv1.Progress{
		State: progress.State, Fraction: progress.Fraction, SnapshotUnixMs: progress.SnapshotUnixMS,
		RatePerMs: progress.RatePerMS,
	}
}

func formatProtoTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func presentedSchedulingDiagnosisToProto(view presentation.JobDetailsView) *cnpv1.SchedulingDiagnosis {
	if view.SchedulingSummary == "" {
		return nil
	}
	agents := make([]*cnpv1.SchedulingAgentAssessment, 0, len(view.SchedulingAgents))
	for _, agent := range view.SchedulingAgents {
		agents = append(agents, &cnpv1.SchedulingAgentAssessment{
			AgentId: agent.AgentID, Status: agent.Status, Details: agent.Details, Tone: agent.Tone,
		})
	}
	return &cnpv1.SchedulingDiagnosis{
		State: view.SchedulingState, Summary: view.SchedulingSummary,
		RequirementsLabel: view.SchedulingRequirements, AdditionalAgentsLabel: view.SchedulingAdditional,
		Agents: agents,
	}
}

func schedulingDiagnosisToProto(diagnosis *domain.SchedulingDiagnosis) *cnpv1.SchedulingDiagnosis {
	if diagnosis == nil {
		return nil
	}
	agents := make([]*cnpv1.SchedulingAgentAssessment, 0, len(diagnosis.Agents))
	for _, agent := range diagnosis.Agents {
		status, tone := "Does not match", "danger"
		if agent.CapabilityMatch && agent.Available {
			status, tone = "Eligible", "success"
		} else if agent.CapabilityMatch {
			status, tone = "Unavailable", "warning"
		}
		details := append([]string(nil), agent.AvailabilityIssues...)
		for _, issue := range agent.CapabilityIssues {
			details = append(details, issue.Message)
		}
		agents = append(agents, &cnpv1.SchedulingAgentAssessment{
			AgentId: agent.AgentID, Status: status, Details: strings.Join(details, "; "), Tone: tone,
			CapabilityMatch: agent.CapabilityMatch, Available: agent.Available,
		})
	}
	return &cnpv1.SchedulingDiagnosis{
		State: diagnosis.State, Summary: diagnosis.Summary, Requirements: append([]string(nil), diagnosis.Requirements...),
		RequirementsLabel: strings.Join(diagnosis.Requirements, " · "), Agents: agents,
	}
}

func runPipelineRequestFromProto(request *cnpv1.RunPipelineRequest, idempotencyKey string) application.RunPipelineRequest {
	if request == nil {
		return application.RunPipelineRequest{IdempotencyKey: idempotencyKey}
	}
	result := application.RunPipelineRequest{PipelineDBID: request.PipelineDbId, IdempotencyKey: idempotencyKey}
	if selection := request.Selection; selection != nil {
		result.PipelineJobID = selection.PipelineJobId
		result.MatrixName = selection.MatrixName
		if selection.MatrixIndex != nil {
			index := int(selection.GetMatrixIndex())
			result.MatrixIndex = &index
		}
		result.DryRun = selection.DryRun
		result.SourceRef = selection.SourceRef
		result.AgentID = selection.AgentId
		result.ExecutionMode = selection.ExecutionMode
	}
	return result
}

func runPipelineChainRequestFromProto(request *cnpv1.RunPipelineChainRequest, idempotencyKey string) application.RunPipelineChainRequest {
	if request == nil {
		return application.RunPipelineChainRequest{IdempotencyKey: idempotencyKey}
	}
	result := application.RunPipelineChainRequest{
		ProjectID: request.ProjectId, ChainID: request.ChainId, IdempotencyKey: idempotencyKey,
	}
	if selection := request.Selection; selection != nil {
		result.PipelineJobID = selection.PipelineJobId
		result.MatrixName = selection.MatrixName
		if selection.MatrixIndex != nil {
			index := int(selection.GetMatrixIndex())
			result.MatrixIndex = &index
		}
		result.DryRun = selection.DryRun
		result.SourceRef = selection.SourceRef
		result.AgentID = selection.AgentId
		result.ExecutionMode = selection.ExecutionMode
	}
	return result
}

func runOptionsRequestFromProto(request *cnpv1.GetRunOptionsRequest) application.RunOptionsRequest {
	if request == nil {
		return application.RunOptionsRequest{}
	}
	result := application.RunOptionsRequest{
		PipelineDBID: request.PipelineDbId, ProjectID: request.ProjectId, ChainID: request.ChainId,
		IncludeSourceRefs: true, IncludeEligibleAgents: true, AllowMissingSourceRepo: true,
	}
	if selection := request.Selection; selection != nil {
		result.PipelineJobID = selection.PipelineJobId
		result.MatrixName = selection.MatrixName
		if selection.MatrixIndex != nil {
			index := int(selection.GetMatrixIndex())
			result.MatrixIndex = &index
		}
		result.DryRun = selection.DryRun
		result.SourceRef = selection.SourceRef
		result.AgentID = selection.AgentId
		result.ExecutionMode = selection.ExecutionMode
	}
	return result
}

func runOptionsToProto(options application.RunOptions) *cnpv1.RunOptionsView {
	return &cnpv1.RunOptionsView{
		TargetKind: options.TargetKind, TargetLabel: options.TargetLabel, PipelineDbId: options.PipelineDBID,
		ProjectId: options.ProjectID, ChainId: options.ChainID, SupportsDryRun: options.SupportsDryRun,
		SourceRepo: options.SourceRepo, DefaultSourceRef: options.DefaultSourceRef,
		SourceRefs: runOptionListToProto(options.SourceRefs), EligibleAgents: runOptionListToProto(options.EligibleAgents),
		PendingJobs: uint32(max(options.PendingJobs, 0)), SelectedSourceRef: options.SelectedSourceRef,
		SelectedAgentId: options.SelectedAgentID,
	}
}

func runOptionListToProto(options []application.RunOption) []*cnpv1.RunOption {
	result := make([]*cnpv1.RunOption, 0, len(options))
	for _, option := range options {
		result = append(result, &cnpv1.RunOption{Value: option.Value, Label: option.Label})
	}
	return result
}

func agentsViewToProto(view presentation.AgentsView) *cnpv1.AgentsView {
	agents := make([]*cnpv1.AgentSummary, 0, len(view.Agents))
	for _, agent := range view.Agents {
		agents = append(agents, agentSummaryToProto(agent))
	}
	return &cnpv1.AgentsView{Summary: view.Summary, Agents: agents}
}

func agentSummaryToProto(agent presentation.AgentView) *cnpv1.AgentSummary {
	shells := make([]*cnpv1.AgentScriptShell, 0, len(agent.ScriptShells))
	for _, shell := range agent.ScriptShells {
		shells = append(shells, &cnpv1.AgentScriptShell{Value: shell.Value, Label: shell.Label, ExampleScript: shell.ExampleScript})
	}
	return &cnpv1.AgentSummary{
		Id: agent.ID, Hostname: agent.Hostname, Platform: agent.Platform, Version: agent.Version,
		Status: agent.Status, StatusLabel: agent.StatusLabel, Authorization: agent.Authorization,
		Activation: agent.Activation, Authorized: agent.Authorized, Deactivated: agent.Deactivated,
		JobInProgress: agent.JobInProgress, CapabilitiesLabel: agent.CapabilitiesLabel,
		RunMode: agent.RunMode, LastSeen: agent.LastSeen, RecentLog: agent.RecentLog,
		UpdateLabel: agent.UpdateLabel, CanUpdate: agent.CanUpdate, CanContact: agent.CanContact,
		CanRunScript: agent.CanRunScript, ScriptShells: shells,
	}
}

func agentDetailsToProto(view presentation.AgentDetailsView) *cnpv1.AgentDetailsView {
	return &cnpv1.AgentDetailsView{Agent: agentSummaryToProto(view.Agent)}
}

func managedYAMLToProto(definition protocol.ManagedYAMLDefinition) *cnpv1.ManagedYAMLDefinition {
	return &cnpv1.ManagedYAMLDefinition{
		ProjectId: definition.ProjectID, ProjectName: definition.ProjectName,
		Yaml: definition.YAML, Revision: definition.Revision,
		Pipelines: uint32(max(definition.Pipelines, 0)), PipelineChains: uint32(max(definition.PipelineChains, 0)),
	}
}

func changeToProto(change application.Change) *cnpv1.ChangeEvent {
	topics := make([]cnpv1.ChangeTopic, 0, len(change.Topics))
	for _, topic := range change.Topics {
		topics = append(topics, changeTopicToProto(topic))
	}
	return &cnpv1.ChangeEvent{
		ServerInstanceId: change.InstanceID, Revision: change.Revision, Topics: topics,
		OccurredUnixMs: change.OccurredAt.UnixMilli(), ResyncRequired: change.Resync,
	}
}

func changeTopicToProto(topic application.ChangeTopic) cnpv1.ChangeTopic {
	switch topic {
	case application.ChangeServer:
		return cnpv1.ChangeTopic_CHANGE_TOPIC_SERVER
	case application.ChangeProjects:
		return cnpv1.ChangeTopic_CHANGE_TOPIC_PROJECTS
	case application.ChangeAgents:
		return cnpv1.ChangeTopic_CHANGE_TOPIC_AGENTS
	case application.ChangeQueue:
		return cnpv1.ChangeTopic_CHANGE_TOPIC_QUEUE
	case application.ChangeHistory:
		return cnpv1.ChangeTopic_CHANGE_TOPIC_HISTORY
	case application.ChangeUpdates:
		return cnpv1.ChangeTopic_CHANGE_TOPIC_UPDATES
	case application.ChangeVault:
		return cnpv1.ChangeTopic_CHANGE_TOPIC_VAULT
	case application.ChangeAgentEligibility:
		return cnpv1.ChangeTopic_CHANGE_TOPIC_AGENT_ELIGIBILITY
	default:
		return cnpv1.ChangeTopic_CHANGE_TOPIC_UNSPECIFIED
	}
}

func errorToProto(err error) *cnpv1.ErrorStatus {
	code := cnpv1.StatusCode_STATUS_CODE_INTERNAL
	switch application.ErrorKindOf(err) {
	case application.ErrorInvalidArgument:
		code = cnpv1.StatusCode_STATUS_CODE_INVALID_ARGUMENT
	case application.ErrorNotFound:
		code = cnpv1.StatusCode_STATUS_CODE_NOT_FOUND
	case application.ErrorConflict:
		code = cnpv1.StatusCode_STATUS_CODE_CONFLICT
	case application.ErrorFailedPrecondition:
		code = cnpv1.StatusCode_STATUS_CODE_FAILED_PRECONDITION
	case application.ErrorUnavailable:
		code = cnpv1.StatusCode_STATUS_CODE_UNAVAILABLE
	case application.ErrorUnsupported:
		code = cnpv1.StatusCode_STATUS_CODE_UNSUPPORTED
	}
	return &cnpv1.ErrorStatus{Code: code, Message: err.Error()}
}
