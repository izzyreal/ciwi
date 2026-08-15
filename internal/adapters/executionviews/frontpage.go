package executionviews

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/adapters/executiondiagnosis"
	"github.com/izzyreal/ciwi/internal/domain"
	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/requirements"
	"github.com/izzyreal/ciwi/internal/server/jobhistory"
)

type Store interface {
	ListJobExecutionsContext(context.Context) ([]protocol.JobExecution, error)
	ListJobExecutionTestSummaries(context.Context, []string) (map[string]protocol.JobExecutionTestSummary, error)
	GetJobExecution(string) (protocol.JobExecution, error)
	ListJobExecutionTimelineEvents(string) ([]protocol.JobExecutionEvent, error)
	ListJobExecutionEventsPageAfter(string, int64, int) ([]protocol.JobExecutionEvent, error)
	ListJobExecutionArtifacts(string) ([]protocol.JobExecutionArtifact, error)
	GetJobExecutionTestReport(string) (protocol.JobExecutionTestReport, bool, error)
}

type jobLogStore interface {
	GetJobLogDescriptor(string) (domain.JobLogDescriptor, error)
	GetJobLogPage(string, string, domain.JobLogPageMode, int64) (domain.JobLogPage, error)
	SearchJobLog(string, string, int64) (domain.JobLogSearchResult, error)
}

type SchedulingAgentSource interface {
	ListSchedulingAgents(context.Context) ([]requirements.AgentSnapshot, error)
}

type ProgressEstimator interface {
	AttachJobEstimates([]protocol.JobExecution)
	AttachDetailEstimate(*protocol.JobExecution) error
}

const (
	outputPageSize  = 128
	outputPageBytes = 512 * 1024
)

func (r *Repository) GetJobLogDescriptor(ctx context.Context, jobID string) (domain.JobLogDescriptor, error) {
	if err := ctx.Err(); err != nil {
		return domain.JobLogDescriptor{}, err
	}
	store, ok := r.store.(jobLogStore)
	if !ok {
		return domain.JobLogDescriptor{}, fmt.Errorf("job log store unavailable")
	}
	return store.GetJobLogDescriptor(jobID)
}

func (r *Repository) GetJobLogPage(ctx context.Context, jobID, itemID string, mode domain.JobLogPageMode, cursor int64) (domain.JobLogPage, error) {
	if err := ctx.Err(); err != nil {
		return domain.JobLogPage{}, err
	}
	store, ok := r.store.(jobLogStore)
	if !ok {
		return domain.JobLogPage{}, fmt.Errorf("job log store unavailable")
	}
	return store.GetJobLogPage(jobID, itemID, mode, cursor)
}

func (r *Repository) SearchJobLog(ctx context.Context, jobID, query string, selectedIndex int64) (domain.JobLogSearchResult, error) {
	if err := ctx.Err(); err != nil {
		return domain.JobLogSearchResult{}, err
	}
	store, ok := r.store.(jobLogStore)
	if !ok {
		return domain.JobLogSearchResult{}, fmt.Errorf("job log store unavailable")
	}
	return store.SearchJobLog(jobID, query, selectedIndex)
}

func (r *Repository) ListJobOutputAfter(ctx context.Context, jobID string, afterEventID int64) (domain.JobOutputBatch, error) {
	if err := ctx.Err(); err != nil {
		return domain.JobOutputBatch{}, err
	}
	job, err := r.store.GetJobExecution(jobID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return domain.JobOutputBatch{}, domain.ErrJobExecutionNotFound
		}
		return domain.JobOutputBatch{}, err
	}
	events, err := r.store.ListJobExecutionEventsPageAfter(jobID, afterEventID, outputPageSize)
	if err != nil {
		return domain.JobOutputBatch{}, err
	}
	batch := domain.JobOutputBatch{
		JobExecutionID: jobID, NextEventID: afterEventID,
		Terminal: protocol.IsTerminalJobExecutionStatus(protocol.NormalizeJobExecutionStatus(job.Status)),
		Events:   make([]domain.JobOutputEvent, 0, len(events)),
	}
	pageBytes := 0
	for eventIndex, event := range events {
		eventBytes := len(event.Output) + len(event.Message) + len(event.Error)
		if len(batch.Events) > 0 && pageBytes+eventBytes > outputPageBytes {
			batch.HasMore = true
			break
		}
		itemKind, itemName, itemIndex, itemTotal := "", "", 0, 0
		itemID := ""
		if event.Phase != nil {
			itemKind, itemName, itemIndex, itemTotal = "phase", event.Phase.Name, event.Phase.Index, event.Phase.Total
			itemID = strings.TrimSpace(event.Phase.ID)
		} else if event.Step != nil {
			itemKind, itemName, itemIndex, itemTotal = "step", event.Step.Name, event.Step.Index, event.Step.Total
			if event.Step.Index > 0 {
				itemID = fmt.Sprintf("step:%d", event.Step.Index)
			}
		}
		batch.Events = append(batch.Events, domain.JobOutputEvent{
			ID: event.ID, Type: outputEventType(event.Type), Message: event.Message, Output: event.Output,
			Error: event.Error, ExitCode: copyInt(event.ExitCode), ItemID: itemID, ItemKind: itemKind,
			ItemName: itemName, ItemIndex: itemIndex, ItemTotal: itemTotal,
		})
		if event.ID > batch.NextEventID {
			batch.NextEventID = event.ID
		}
		pageBytes += eventBytes
		if eventIndex == len(events)-1 && len(events) == outputPageSize {
			batch.HasMore = true
		}
	}
	return batch, nil
}

func outputEventType(eventType string) string {
	switch eventType {
	case protocol.JobExecutionEventTypeSystemMessage:
		return domain.JobOutputEventSystemMessage
	case protocol.JobExecutionEventTypeStepOutput, protocol.JobExecutionEventTypePhaseOutput:
		return domain.JobOutputEventOutput
	case protocol.JobExecutionEventTypeStepFinished, protocol.JobExecutionEventTypePhaseFinished:
		return domain.JobOutputEventFinished
	default:
		return ""
	}
}

func (r *Repository) GetJobExecutionDetails(ctx context.Context, jobID string) (domain.JobExecutionDetails, error) {
	if err := ctx.Err(); err != nil {
		return domain.JobExecutionDetails{}, err
	}
	job, err := r.store.GetJobExecution(jobID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return domain.JobExecutionDetails{}, domain.ErrJobExecutionNotFound
		}
		return domain.JobExecutionDetails{}, err
	}
	if r.progress != nil {
		if err := r.progress.AttachDetailEstimate(&job); err != nil {
			return domain.JobExecutionDetails{}, err
		}
	}
	jobs := []protocol.JobExecution{job}
	if err := r.attachSchedulingDiagnoses(ctx, jobs); err != nil {
		return domain.JobExecutionDetails{}, err
	}
	job = jobs[0]
	events, err := r.store.ListJobExecutionTimelineEvents(jobID)
	if err != nil {
		return domain.JobExecutionDetails{}, err
	}
	if err := ctx.Err(); err != nil {
		return domain.JobExecutionDetails{}, err
	}
	artifacts, err := r.store.ListJobExecutionArtifacts(jobID)
	if err != nil {
		return domain.JobExecutionDetails{}, err
	}
	testReport, reportFound, err := r.store.GetJobExecutionTestReport(jobID)
	if err != nil {
		return domain.JobExecutionDetails{}, err
	}
	return mapJobExecutionDetails(job, events, artifacts, testReport, reportFound), nil
}

func mapJobExecutionDetails(job protocol.JobExecution, events []protocol.JobExecutionEvent, artifacts []protocol.JobExecutionArtifact, testReport protocol.JobExecutionTestReport, reportFound bool) domain.JobExecutionDetails {
	projectID, _ := strconv.ParseInt(protocol.JobMetadataValue(job, protocol.JobMetadataProjectID), 10, 64)
	details := domain.JobExecutionDetails{
		ID: job.ID, ProjectID: projectID, ProjectName: protocol.JobMetadataValue(job, protocol.JobMetadataProject),
		PipelineID: protocol.JobMetadataValue(job, protocol.JobMetadataPipelineID), PipelineJobID: protocol.JobMetadataValue(job, protocol.JobMetadataPipelineJobID),
		MatrixName: protocol.JobMetadataValue(job, protocol.JobMetadataMatrixName), Status: protocol.NormalizeJobExecutionStatus(job.Status),
		CurrentStep: strings.TrimSpace(job.CurrentStep), AgentID: strings.TrimSpace(job.LeasedByAgentID),
		DryRun: protocol.JobMetadataValue(job, protocol.JobMetadataDryRun) == "1", CreatedUTC: job.CreatedUTC,
		StartedUTC: job.StartedUTC, FinishedUTC: job.FinishedUTC, ExitCode: copyInt(job.ExitCode), Error: strings.TrimSpace(job.Error),
		SchedulingDiagnosis: job.SchedulingDiagnosis, ExpectedDurationMS: job.ExpectedDurationMS,
		Metadata: copyStringMap(job.Metadata), RequiredCapabilities: copyStringMap(job.RequiredCapabilities),
		RuntimeCapabilities: copyStringMap(job.RuntimeCapabilities),
		Waiting: protocol.NormalizeJobExecutionStatus(job.Status) == protocol.JobExecutionStatusQueued &&
			(job.Metadata.Flag(domain.ExecutionMetadataChainBlocked) || job.Metadata.Flag(domain.ExecutionMetadataNeedsBlocked)),
	}
	for _, stats := range job.CacheStats {
		details.CacheStats = append(details.CacheStats, domain.JobCacheStatistics{
			ID: stats.ID, Environment: stats.Env, Type: stats.Type, Path: stats.Path, Source: stats.Source,
			SizeBytes: stats.SizeBytes, Files: stats.Files, Directories: stats.Directories,
			ToolMetrics: copyStringMap(stats.ToolMetrics), Error: strings.TrimSpace(stats.Error),
		})
	}
	for _, artifact := range artifacts {
		details.Artifacts = append(details.Artifacts, domain.JobArtifact{Path: strings.TrimSpace(artifact.Path), SizeBytes: artifact.SizeBytes})
	}
	if reportFound {
		details.TestReport = mapJobTestReport(testReport)
	}
	states := timelineStates(events)
	stepsByIndex := make(map[int]protocol.JobStepPlanItem, len(job.StepPlan))
	for _, step := range job.StepPlan {
		if step.Index > 0 {
			stepsByIndex[step.Index] = step
		}
	}
	terminal := protocol.IsTerminalJobExecutionStatus(details.Status)
	for _, item := range protocol.BuildJobExecutionTimeline(job) {
		state := states[item.ID]
		status := state.status
		if status == "" {
			if terminal {
				status = "not reached"
			} else {
				status = "pending"
			}
		}
		expectedDurationMS := int64(0)
		if item.Kind == "phase" {
			expectedDurationMS = job.PhaseExpectedDuration[item.ID]
		} else {
			expectedDurationMS = job.StepExpectedDuration[item.StepIndex]
		}
		timelineItem := domain.JobTimelineItem{
			ID: item.ID, Kind: item.Kind, Name: item.Name, Description: item.Description,
			Index: item.Index, Total: item.Total, Reached: state.reached, Status: status, StartedUTC: state.startedUTC, DurationMS: state.durationMS,
			FinishedUTC: state.finishedUTC, ExpectedDurationMS: expectedDurationMS,
			ExitCode: copyInt(state.exitCode), Error: state.error,
		}
		if item.Kind == "step" {
			if step, ok := stepsByIndex[item.StepIndex]; ok {
				timelineItem.YAMLLiteral = step.YAMLLiteral
				timelineItem.Command = step.Script
			}
		}
		details.Timeline = append(details.Timeline, timelineItem)
	}
	return details
}

func mapJobTestReport(report protocol.JobExecutionTestReport) *domain.JobTestReport {
	result := &domain.JobTestReport{
		Total: report.Total, Passed: report.Passed, Failed: report.Failed, Skipped: report.Skipped,
		Suites: make([]domain.JobTestSuite, 0, len(report.Suites)),
	}
	for _, suite := range report.Suites {
		presentedSuite := domain.JobTestSuite{
			Name: suite.Name, Format: suite.Format, Total: suite.Total, Passed: suite.Passed, Failed: suite.Failed, Skipped: suite.Skipped,
			Cases: make([]domain.JobTestCase, 0, len(suite.Cases)),
		}
		for _, testCase := range suite.Cases {
			presentedSuite.Cases = append(presentedSuite.Cases, domain.JobTestCase{
				Package: testCase.Package, Name: testCase.Name, File: testCase.File, Line: testCase.Line,
				Status: testCase.Status, DurationSeconds: testCase.DurationSeconds, Output: testCase.Output,
			})
		}
		result.Suites = append(result.Suites, presentedSuite)
	}
	if report.Coverage != nil {
		result.Coverage = &domain.JobCoverageReport{
			Format: report.Coverage.Format, TotalLines: report.Coverage.TotalLines, CoveredLines: report.Coverage.CoveredLines,
			TotalStatements: report.Coverage.TotalStatements, CoveredStatements: report.Coverage.CoveredStatements,
			Percent: report.Coverage.Percent, Files: make([]domain.JobCoverageFile, 0, len(report.Coverage.Files)),
		}
		for _, file := range report.Coverage.Files {
			result.Coverage.Files = append(result.Coverage.Files, domain.JobCoverageFile{
				Path: file.Path, TotalLines: file.TotalLines, CoveredLines: file.CoveredLines,
				TotalStatements: file.TotalStatements, CoveredStatements: file.CoveredStatements, Percent: file.Percent,
			})
		}
	}
	return result
}

func copyStringMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

type timelineState struct {
	reached     bool
	startedUTC  time.Time
	finishedUTC time.Time
	status      string
	durationMS  int64
	exitCode    *int
	error       string
}

func timelineStates(events []protocol.JobExecutionEvent) map[string]timelineState {
	states := make(map[string]timelineState)
	for _, event := range events {
		id := ""
		if event.Phase != nil {
			id = strings.TrimSpace(event.Phase.ID)
		} else if event.Step != nil && event.Step.Index > 0 {
			id = fmt.Sprintf("step:%d", event.Step.Index)
		}
		if id == "" {
			continue
		}
		state := states[id]
		state.reached = true
		switch event.Type {
		case protocol.JobExecutionEventTypePhaseStarted, protocol.JobExecutionEventTypeStepStarted:
			state.status = "in progress"
			state.startedUTC = event.TimestampUTC
		case protocol.JobExecutionEventTypePhaseFinished, protocol.JobExecutionEventTypeStepFinished:
			state.status = "succeeded"
			if strings.TrimSpace(event.Error) != "" || (event.ExitCode != nil && *event.ExitCode != 0) {
				state.status = "failed"
			}
			state.durationMS = event.DurationMS
			state.finishedUTC = event.TimestampUTC
			state.exitCode = copyInt(event.ExitCode)
			state.error = strings.TrimSpace(event.Error)
		}
		states[id] = state
	}
	return states
}

func copyInt(value *int) *int {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

type Repository struct {
	store      Store
	limit      int
	scheduling SchedulingAgentSource
	progress   ProgressEstimator
}

func NewRepositoryWithProgress(store Store, limit int, scheduling SchedulingAgentSource, progress ProgressEstimator) *Repository {
	repository := NewRepository(store, limit, scheduling)
	repository.progress = progress
	return repository
}

func NewRepository(store Store, limit int, scheduling ...SchedulingAgentSource) *Repository {
	if limit <= 0 {
		limit = 40
	}
	var source SchedulingAgentSource
	if len(scheduling) > 0 {
		source = scheduling[0]
	}
	return &Repository{store: store, limit: limit, scheduling: source}
}

func (r *Repository) ListFrontPageExecutionCards(ctx context.Context) ([]domain.ExecutionCard, []domain.ExecutionCard, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	jobs, err := r.store.ListJobExecutionsContext(ctx)
	if err != nil {
		return nil, nil, err
	}
	queuedSelection := jobhistory.SelectSummaryCards(jobs, true, r.limit)
	historySelection := jobhistory.SelectSummaryCards(jobs, false, r.limit)
	visibleJobIDs := append(queuedSelection.VisibleJobIDs(jobs), historySelection.VisibleJobIDs(jobs)...)
	if err := r.attachTestSummaries(ctx, jobs, visibleJobIDs); err != nil {
		return nil, nil, err
	}
	if r.progress != nil {
		r.progress.AttachJobEstimates(jobs)
	}
	if err := r.attachSelectedSchedulingDiagnoses(ctx, jobs, queuedSelection.VisibleJobIDs(jobs)); err != nil {
		return nil, nil, err
	}
	queued := mapCards(queuedSelection.Views(jobs))
	history := mapCards(historySelection.Views(jobs))
	return queued, history, nil
}

func (r *Repository) attachTestSummaries(ctx context.Context, jobs []protocol.JobExecution, jobIDs []string) error {
	summaries, err := r.store.ListJobExecutionTestSummaries(ctx, jobIDs)
	if err != nil {
		return fmt.Errorf("load front-page test summaries: %w", err)
	}
	for i := range jobs {
		if summary, found := summaries[jobs[i].ID]; found {
			copy := summary
			jobs[i].TestSummary = &copy
		}
	}
	return nil
}

func (r *Repository) attachSelectedSchedulingDiagnoses(ctx context.Context, jobs []protocol.JobExecution, jobIDs []string) error {
	if len(jobIDs) == 0 {
		return nil
	}
	wanted := make(map[string]struct{}, len(jobIDs))
	for _, jobID := range jobIDs {
		wanted[jobID] = struct{}{}
	}
	indices := make([]int, 0, len(wanted))
	selected := make([]protocol.JobExecution, 0, len(wanted))
	for i := range jobs {
		if _, ok := wanted[jobs[i].ID]; ok {
			indices = append(indices, i)
			selected = append(selected, jobs[i])
		}
	}
	if err := r.attachSchedulingDiagnoses(ctx, selected); err != nil {
		return err
	}
	for i, index := range indices {
		jobs[index] = selected[i]
	}
	return nil
}

func mapCards(cards []jobhistory.CardView) []domain.ExecutionCard {
	out := make([]domain.ExecutionCard, 0, len(cards))
	for _, card := range cards {
		out = append(out, domain.ExecutionCard{
			Key: card.Key, Kind: card.Kind, Title: card.Title,
			JobExecutionIDs: append([]string(nil), card.JobExecutionIDs...),
			Summary: domain.ExecutionSummary{
				TotalJobs: card.Summary.TotalJobs, Succeeded: card.Summary.Succeeded,
				Failed: card.Summary.Failed, InProgress: card.Summary.InProgress, Waiting: card.Summary.Waiting,
			},
			Sections: mapCardSections(card.Sections), ProgressJobs: mapProgressJobs(card.ProgressJobs),
		})
	}
	return out
}

func mapCardSections(sections []jobhistory.SectionView) []domain.ExecutionCardSection {
	out := make([]domain.ExecutionCardSection, 0, len(sections))
	for _, section := range sections {
		label := strings.TrimSpace(section.Label)
		if label == "" {
			label = "Execution"
		}
		mapped := domain.ExecutionCardSection{Key: section.Key, Label: label, ProgressJobs: mapProgressJobs(section.ProgressJobs)}
		for _, item := range section.Items {
			mapped.Jobs = append(mapped.Jobs, mapCardItemJobs(item)...)
		}
		out = append(out, mapped)
	}
	return out
}

func mapProgressJobs(jobs []jobhistory.ProgressJobView) []domain.ExecutionCardJob {
	out := make([]domain.ExecutionCardJob, 0, len(jobs))
	for _, job := range jobs {
		started, finished := time.Time{}, time.Time{}
		if job.StartedUTC != nil {
			started = *job.StartedUTC
		}
		if job.FinishedUTC != nil {
			finished = *job.FinishedUTC
		}
		out = append(out, domain.ExecutionCardJob{
			Status: job.Status, Waiting: job.Waiting, StartedUTC: started,
			FinishedUTC: finished, ExpectedDurationMS: job.ExpectedDurationMS,
		})
	}
	return out
}

func mapCardItemJobs(item jobhistory.ItemView) []domain.ExecutionCardJob {
	if item.Job != nil {
		label := strings.TrimSpace(item.MatrixLabel)
		if label == "" {
			label = domain.ExecutionMetadata(item.Job.Metadata).Value(domain.ExecutionMetadataPipelineJobID)
		}
		if label == "" {
			label = strings.TrimSpace(item.Job.ID)
		}
		status := protocol.NormalizeJobExecutionStatus(item.Job.Status)
		return []domain.ExecutionCardJob{{
			ID: item.Job.ID, ProjectID: metadataInt64(item.Job.Metadata, domain.ExecutionMetadataProjectID), Label: label, Status: status,
			PipelineID: domain.ExecutionMetadata(item.Job.Metadata).Value(domain.ExecutionMetadataPipelineID),
			BuildLabel: executionBuildLabel(item.Job.Metadata), AgentID: strings.TrimSpace(item.Job.LeasedByAgentID),
			CreatedUTC: item.Job.CreatedUTC, StartedUTC: timeValue(item.Job.StartedUTC), FinishedUTC: timeValue(item.Job.FinishedUTC),
			Reason: executionReason(item.Job), Action: executionAction(status),
			CurrentStep: strings.TrimSpace(item.Job.CurrentStep), TestSummary: mapJobTestSummary(item.Job.TestSummary), SchedulingDiagnosis: item.Job.SchedulingDiagnosis,
			ExpectedDurationMS: item.Job.ExpectedDurationMS,
			Waiting: status == protocol.JobExecutionStatusQueued &&
				(domain.ExecutionMetadata(item.Job.Metadata).Flag(domain.ExecutionMetadataChainBlocked) || domain.ExecutionMetadata(item.Job.Metadata).Flag(domain.ExecutionMetadataNeedsBlocked)),
		}}
	}
	out := make([]domain.ExecutionCardJob, 0, len(item.Items))
	for _, child := range item.Items {
		out = append(out, mapCardItemJobs(child)...)
	}
	return out
}

func mapJobTestSummary(summary *protocol.JobExecutionTestSummary) *domain.JobTestSummary {
	if summary == nil {
		return nil
	}
	return &domain.JobTestSummary{
		Total: summary.Total, Passed: summary.Passed, Failed: summary.Failed, Skipped: summary.Skipped,
	}
}

func metadataInt64(metadata domain.ExecutionMetadata, key string) int64 {
	value, _ := metadata.Int64(key)
	return value
}

func executionBuildLabel(metadata domain.ExecutionMetadata) string {
	version := metadata.Value(domain.ExecutionMetadataBuildVersion)
	if version == "" {
		return ""
	}
	if target := metadata.Value(domain.ExecutionMetadataBuildTarget); target != "" {
		return version + " (" + target + ")"
	}
	return version
}

func executionReason(job *jobhistory.JobView) string {
	parts := make([]string, 0, 2)
	if status := protocol.NormalizeJobExecutionStatus(job.Status); status == protocol.JobExecutionStatusQueued {
		metadata := domain.ExecutionMetadata(job.Metadata)
		if pipelines := metadata.CSV(domain.ExecutionMetadataChainDependsOnPipelines); metadata.Flag(domain.ExecutionMetadataChainBlocked) && len(pipelines) > 0 {
			label := "pipelines "
			if len(pipelines) == 1 {
				label = "pipeline "
			}
			parts = append(parts, "Waiting for "+label+strings.Join(pipelines, ", "))
		} else if jobs := metadata.CSV(domain.ExecutionMetadataNeedsJobIDs); len(jobs) > 0 {
			label := "jobs "
			if len(jobs) == 1 {
				label = "job "
			}
			parts = append(parts, "Waiting for "+label+strings.Join(jobs, ", "))
		} else if metadata.Flag(domain.ExecutionMetadataChainBlocked) || metadata.Flag(domain.ExecutionMetadataNeedsBlocked) {
			parts = append(parts, "Waiting for prerequisites")
		}
	}
	if job.SchedulingDiagnosis != nil {
		if summary := strings.TrimSpace(job.SchedulingDiagnosis.Summary); summary != "" && !slices.Contains(parts, summary) {
			parts = append(parts, summary)
		}
	}
	return strings.Join(parts, "; ")
}

func splitMetadataList(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func executionAction(status string) string {
	if protocol.IsPendingJobExecutionStatus(status) {
		return "remove"
	}
	if protocol.NormalizeJobExecutionStatus(status) == protocol.JobExecutionStatusRunning {
		return "cancel"
	}
	return ""
}

func timeValue(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}

func (r *Repository) attachSchedulingDiagnoses(ctx context.Context, jobs []protocol.JobExecution) error {
	if r.scheduling == nil {
		return nil
	}
	agents, err := r.scheduling.ListSchedulingAgents(ctx)
	if err != nil {
		return err
	}
	for i := range jobs {
		jobs[i].SchedulingDiagnosis = executiondiagnosis.DiagnoseQueuedJob(jobs[i], agents)
	}
	return nil
}
