package nativequic

import (
	"github.com/izzyreal/ciwi/internal/application"
	"github.com/izzyreal/ciwi/internal/domain"
	"github.com/izzyreal/ciwi/internal/presentation"
	cnpv1 "github.com/izzyreal/ciwi/pkg/cnp/v1"
)

func serverInfoToProto(info domain.ServerInfo) *cnpv1.ServerInfo {
	return &cnpv1.ServerInfo{
		Name: info.Name, ApiVersion: uint32(info.APIVersion), Version: info.Version, Hostname: info.Hostname,
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
				StepsCount: uint32(max(job.StepsCount, 0)), Steps: steps,
			})
		}
		pipelines = append(pipelines, &cnpv1.ProjectPipelineDetails{
			Id: pipeline.ID, PipelineId: pipeline.PipelineID, Trigger: pipeline.Trigger,
			DependsOn: append([]string{}, pipeline.DependsOn...), Dependencies: pipeline.Dependencies,
			JobsCount: uint32(max(pipeline.JobsCount, 0)), SupportsDryRun: pipeline.SupportsDryRun, Jobs: jobs,
		})
	}
	return &cnpv1.ProjectDetailsView{Project: project, Pipelines: pipelines}
}

func jobDetailsToProto(view presentation.JobDetailsView) *cnpv1.JobDetailsView {
	timeline := make([]*cnpv1.JobTimelineItem, 0, len(view.Timeline))
	for _, item := range view.Timeline {
		timeline = append(timeline, &cnpv1.JobTimelineItem{
			Id: item.ID, Kind: item.Kind, Title: item.Title, Description: item.Description,
			Status: item.Status, StatusLabel: item.StatusLabel, Duration: item.Duration,
			ExitCode: item.ExitCode, Error: item.Error,
		})
	}
	return &cnpv1.JobDetailsView{
		Id: view.ID, Title: view.Title, Context: view.Context, Status: view.Status, StatusLabel: view.StatusLabel,
		CurrentStep: view.CurrentStep, Agent: view.Agent, Mode: view.Mode, Created: view.Created,
		Started: view.Started, Finished: view.Finished, Duration: view.Duration, ExitCode: view.ExitCode,
		Error: view.Error, Timeline: timeline,
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
		})
	}
	return out
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
