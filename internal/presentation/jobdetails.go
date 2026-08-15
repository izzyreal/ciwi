package presentation

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/izzyreal/ciwi/internal/domain"
	"github.com/izzyreal/ciwi/internal/requirements"
)

type JobDetailsView struct {
	ID                        string
	ProjectID                 int64
	Title                     string
	Context                   string
	Status                    string
	StatusLabel               string
	CurrentStep               string
	Agent                     string
	Mode                      string
	Created                   string
	Started                   string
	Finished                  string
	Duration                  string
	ExitCode                  string
	Error                     string
	CanCancel                 bool
	CanRerun                  bool
	SchedulingState           string
	SchedulingSummary         string
	SchedulingRequirements    string
	SchedulingAgents          []SchedulingAgentView
	SchedulingAdditional      string
	JobProperties             []JobDetailRowView
	CacheStatistics           []JobDetailRowView
	CacheStatisticsEmpty      string
	HostToolRequirements      ToolRequirementsView
	ContainerToolRequirements ToolRequirementsView
	ReleaseSummary            []JobDetailRowView
	HasReleaseSummary         bool
	Artifacts                 ReportDetailsView
	TestReport                ReportDetailsView
	CoverageReport            ReportDetailsView
	Progress                  domain.Progress
	Timeline                  []JobTimelineView
	OutputGroups              []JobOutputGroupView
}

type JobDetailRowView struct {
	Label string
	Value string
	Tone  string
}

type ToolRequirementsView struct {
	EmptyLabel string
	Summary    string
	Tone       string
	Issues     []string
}

type ReportDetailsView struct {
	EmptyLabel      string
	Summary         string
	Tone            string
	Rows            []JobDetailRowView
	AdditionalLabel string
	Nodes           []TreeNodeView
	Filters         []ReportFilterView
	Filter          string
	CanDownloadAll  bool
}

type ReportFilterView struct {
	Value string
	Label string
}

// TreeNodeView is the shared recursive presentation used for artifacts, test
// cases, and coverage paths. Renderers own only drawing and expansion.
type TreeNodeView struct {
	Key             string
	Label           string
	Detail          string
	Tone            string
	Link            string
	ActionLabel     string
	ActionKind      string
	ActionPath      string
	DefaultExpanded bool
	FilterValues    []string
	Children        []TreeNodeView
}

type SchedulingAgentView struct {
	AgentID string
	Status  string
	Details string
	Tone    string
}

type JobTimelineView struct {
	ID          string
	Kind        string
	Title       string
	Description string
	Status      string
	StatusLabel string
	Duration    string
	ExitCode    string
	Error       string
	Progress    domain.Progress
}

type JobOutputGroupView struct {
	ID              string
	StateKey        string
	Kind            string
	Title           string
	CommandSummary  string
	Status          string
	StatusLabel     string
	Reached         bool
	Started         string
	Duration        string
	ExitCode        string
	Error           string
	Details         string
	YAMLLiteral     string
	ExpandedCommand string
	DefaultExpanded bool
	Progress        domain.Progress
}

type JobOutputView struct {
	JobExecutionID string
	Events         []JobOutputEventView
	NextEventID    int64
	HasMore        bool
	Terminal       bool
}

type JobOutputEventView struct {
	EventID  int64
	Type     string
	ItemID   string
	Text     string
	Error    string
	ExitCode string
}

type JobDetailsQueries struct {
	executions interface {
		GetJobExecutionDetails(context.Context, string) (domain.JobExecutionDetails, error)
		GetJobOutput(context.Context, string, int64) (domain.JobOutputBatch, error)
	}
}

func NewJobDetailsQueries(executions interface {
	GetJobExecutionDetails(context.Context, string) (domain.JobExecutionDetails, error)
	GetJobOutput(context.Context, string, int64) (domain.JobOutputBatch, error)
}) *JobDetailsQueries {
	return &JobDetailsQueries{executions: executions}
}

func (q *JobDetailsQueries) GetJobOutputView(ctx context.Context, jobID string, afterEventID int64) (JobOutputView, error) {
	batch, err := q.executions.GetJobOutput(ctx, jobID, afterEventID)
	if err != nil {
		return JobOutputView{}, err
	}
	view := JobOutputView{
		JobExecutionID: batch.JobExecutionID, NextEventID: batch.NextEventID,
		HasMore: batch.HasMore, Terminal: batch.Terminal,
		Events: make([]JobOutputEventView, 0, len(batch.Events)),
	}
	for _, event := range batch.Events {
		text := outputEventText(event)
		if event.Type != "" {
			view.Events = append(view.Events, JobOutputEventView{
				EventID: event.ID, Type: event.Type, ItemID: event.ItemID, Text: text,
				Error: strings.TrimSpace(event.Error), ExitCode: formatExitCode(event.ExitCode),
			})
		}
	}
	return view, nil
}

func (q *JobDetailsQueries) GetJobDetailsView(ctx context.Context, jobID string) (JobDetailsView, error) {
	details, err := q.executions.GetJobExecutionDetails(ctx, jobID)
	if err != nil {
		return JobDetailsView{}, err
	}
	return presentJobDetails(details), nil
}

func presentJobDetails(details domain.JobExecutionDetails) JobDetailsView {
	now := time.Now().UTC()
	statusLabel := humanStatus(details.Status)
	if details.TestReport != nil {
		statusLabel = statusWithTestCounts(statusLabel, &domain.JobTestSummary{
			Total: details.TestReport.Total, Passed: details.TestReport.Passed,
		})
	}
	view := JobDetailsView{
		ID: details.ID, ProjectID: details.ProjectID, Title: jobHeaderTitle(details), Context: jobHeaderContext(details, statusLabel),
		Status: details.Status, StatusLabel: statusLabel, CurrentStep: details.CurrentStep,
		Agent: details.AgentID, Created: formatTimestamp(details.CreatedUTC), Started: formatTimestamp(details.StartedUTC),
		Finished: formatTimestamp(details.FinishedUTC), ExitCode: formatExitCode(details.ExitCode), Error: details.Error,
		CanCancel: canCancelJob(details), CanRerun: canRerunJob(details),
		Progress: progressForInput(progressInput{
			status: details.Status, waiting: details.Waiting,
			started: details.StartedUTC, finished: details.FinishedUTC,
			expectedDurationMS: details.ExpectedDurationMS,
		}, now),
	}
	if details.DryRun {
		view.Mode = "Dry run"
	} else {
		view.Mode = "Ordinary run"
	}
	applySchedulingDiagnosis(&view, details.SchedulingDiagnosis)
	if !details.StartedUTC.IsZero() && !details.FinishedUTC.IsZero() && !details.FinishedUTC.Before(details.StartedUTC) {
		view.Duration = formatDuration(details.FinishedUTC.Sub(details.StartedUTC))
	}
	view.JobProperties = presentJobProperties(details, view)
	view.CacheStatistics, view.CacheStatisticsEmpty = presentCacheStatistics(details.CacheStats)
	view.HostToolRequirements = presentToolRequirements(details, "requires.tool.", "host.tool.", "No tool requirements declared for this job.")
	view.ContainerToolRequirements = presentToolRequirements(details, "requires.container.tool.", "container.tool.", "No container tool requirements declared for this job.")
	view.ReleaseSummary, view.HasReleaseSummary = presentReleaseSummary(details)
	view.Artifacts = presentArtifacts(details.Artifacts)
	view.TestReport = presentTestReport(details.TestReport, details.Metadata)
	view.CoverageReport = presentCoverageReport(details.TestReport)
	phaseTotal, stepTotal := 0, 0
	for _, item := range details.Timeline {
		if item.Kind == "phase" {
			phaseTotal++
		} else {
			stepTotal++
		}
	}
	phaseIndex, stepIndex := 0, 0
	view.Timeline = make([]JobTimelineView, 0, len(details.Timeline))
	view.OutputGroups = make([]JobOutputGroupView, 0, len(details.Timeline))
	for _, item := range details.Timeline {
		prefix := "Job step"
		categoryIndex, categoryTotal := stepIndex+1, stepTotal
		if item.Kind == "phase" {
			prefix = "Ciwi phase"
			phaseIndex++
			categoryIndex, categoryTotal = phaseIndex, phaseTotal
		} else {
			stepIndex++
			categoryIndex = stepIndex
		}
		title := fmt.Sprintf("%s %d/%d", prefix, categoryIndex, categoryTotal)
		if name := strings.TrimSpace(item.Name); name != "" {
			title += ": " + name
		}
		reached := item.Reached || (item.Status != "" && item.Status != "pending" && item.Status != "not reached")
		itemProgress := progressForInput(progressInput{
			status: item.Status, started: item.StartedUTC, finished: item.FinishedUTC,
			expectedDurationMS: item.ExpectedDurationMS,
		}, now)
		view.Timeline = append(view.Timeline, JobTimelineView{
			ID: item.ID, Kind: item.Kind, Title: title, Description: item.Description,
			Status: item.Status, StatusLabel: humanStatus(item.Status), Duration: formatDurationMS(item.DurationMS),
			ExitCode: formatExitCode(item.ExitCode), Error: item.Error, Progress: itemProgress,
		})
		view.OutputGroups = append(view.OutputGroups, JobOutputGroupView{
			ID: item.ID, StateKey: "job-output:" + details.ID + ":" + item.ID, Kind: item.Kind, Title: title,
			CommandSummary: strings.Join(strings.Fields(item.Command), " "), Status: item.Status,
			StatusLabel: humanStatus(item.Status), Reached: reached, Started: formatTimestamp(item.StartedUTC),
			Duration: formatDurationMS(item.DurationMS), ExitCode: formatExitCode(item.ExitCode), Error: item.Error,
			Details: item.Description, YAMLLiteral: item.YAMLLiteral, ExpandedCommand: item.Command,
			DefaultExpanded: details.Metadata.Flag(domain.ExecutionMetadataAdhoc) && item.Kind != "phase",
			Progress:        itemProgress,
		})
	}
	return view
}

func presentArtifacts(artifacts []domain.JobArtifact) ReportDetailsView {
	if len(artifacts) == 0 {
		return ReportDetailsView{EmptyLabel: "No artifacts"}
	}
	items := append([]domain.JobArtifact(nil), artifacts...)
	sort.SliceStable(items, func(i, j int) bool { return items[i].Path < items[j].Path })
	return ReportDetailsView{
		Summary: fmt.Sprintf("%d artifact(s)", len(items)), Tone: "accent", CanDownloadAll: true,
		Nodes: presentArtifactTree(items),
	}
}

func presentTestReport(report *domain.JobTestReport, metadata domain.ExecutionMetadata) ReportDetailsView {
	if report == nil {
		return ReportDetailsView{EmptyLabel: "No parsed test report"}
	}
	return ReportDetailsView{
		Summary: formatTestCounts(report.Total, report.Passed, report.Failed, report.Skipped), Tone: reportTone(report.Total, report.Failed),
		Filter: "all", Filters: []ReportFilterView{{Value: "all", Label: "All"}, {Value: "fail", Label: "Failed"}, {Value: "skip", Label: "Skipped"}, {Value: "pass", Label: "Passed"}},
		Nodes: presentTestTree(report, metadata),
	}
}

func presentCoverageReport(report *domain.JobTestReport) ReportDetailsView {
	if report == nil || report.Coverage == nil {
		return ReportDetailsView{EmptyLabel: "No parsed coverage report"}
	}
	coverage := report.Coverage
	total, covered, unit := coverage.TotalStatements, coverage.CoveredStatements, "statements"
	if total == 0 {
		total, covered, unit = coverage.TotalLines, coverage.CoveredLines, "lines"
	}
	percent := coveragePercent(covered, total, coverage.Percent)
	format := strings.TrimSpace(coverage.Format)
	if format == "" {
		format = "unknown format"
	}
	view := ReportDetailsView{
		Summary: fmt.Sprintf("%.2f%% overall · %d/%d %s · %d file(s) · %s", percent, covered, total, unit, len(coverage.Files), format),
		Tone:    "accent",
	}
	view.Nodes = presentCoverageTree(coverage.Files)
	return view
}

func formatTestCounts(total, passed, failed, skipped int) string {
	return fmt.Sprintf("%d total · %d passed · %d failed · %d skipped", total, passed, failed, skipped)
}

func reportTone(total, failed int) string {
	if failed > 0 {
		return "danger"
	}
	if total > 0 {
		return "success"
	}
	return "muted"
}

func coverageFileTotals(file domain.JobCoverageFile) (int, int) {
	if file.TotalStatements > 0 {
		return file.TotalStatements, file.CoveredStatements
	}
	return file.TotalLines, file.CoveredLines
}

func coveragePercent(covered, total int, reported float64) float64 {
	if reported != 0 || total == 0 {
		return reported
	}
	return 100 * float64(covered) / float64(total)
}

func jobHeaderTitle(details domain.JobExecutionDetails) string {
	parts := make([]string, 0, 4)
	if project := strings.TrimSpace(details.ProjectName); project != "" {
		parts = append(parts, project)
	}
	if details.Metadata.Flag(domain.ExecutionMetadataAdhoc) {
		parts = append(parts, "Adhoc script")
	} else {
		for _, value := range []string{details.PipelineID, details.PipelineJobID, details.MatrixName} {
			if value = strings.TrimSpace(value); value != "" {
				parts = append(parts, value)
			}
		}
	}
	if len(parts) == 0 {
		return "Job Execution"
	}
	return strings.Join(parts, " / ")
}

func jobHeaderContext(details domain.JobExecutionDetails, statusLabel string) string {
	context := "Status: " + statusLabel
	if step := strings.TrimSpace(details.CurrentStep); step != "" {
		context += " · " + step
	}
	return context
}

func presentJobProperties(details domain.JobExecutionDetails, view JobDetailsView) []JobDetailRowView {
	build := details.Metadata.Value(domain.ExecutionMetadataBuildVersion)
	if target := details.Metadata.Value(domain.ExecutionMetadataBuildTarget); build != "" && target != "" {
		build += " (" + target + ")"
	}
	rows := []JobDetailRowView{
		{Label: "Job Execution ID", Value: details.ID},
		{Label: "Project", Value: details.ProjectName},
		{Label: "Job ID", Value: details.PipelineJobID},
		{Label: "Pipeline", Value: details.PipelineID},
		{Label: "Mode", Value: view.Mode},
		{Label: "Build", Value: build},
		{Label: "Agent", Value: details.AgentID},
		{Label: "Created", Value: view.Created},
		{Label: "Started", Value: view.Started},
		{Label: "Duration", Value: view.Duration},
		{Label: "Exit Code", Value: view.ExitCode},
	}
	return rows
}

func presentCacheStatistics(statistics []domain.JobCacheStatistics) ([]JobDetailRowView, string) {
	if len(statistics) == 0 {
		return nil, "No cache statistics reported for this job."
	}
	rows := make([]JobDetailRowView, 0, len(statistics))
	for _, stats := range statistics {
		attributes := make([]string, 0, 3)
		if stats.Environment != "" {
			attributes = append(attributes, stats.Environment)
		}
		if stats.Type != "" {
			attributes = append(attributes, stats.Type)
		}
		if stats.Source != "" {
			attributes = append(attributes, "source: "+stats.Source)
		}
		lines := make([]string, 0, 4)
		if len(attributes) > 0 {
			lines = append(lines, strings.Join(attributes, " · "))
		}
		if stats.Path != "" {
			lines = append(lines, "Path: "+stats.Path)
		}
		lines = append(lines, fmt.Sprintf("Size: %s | Files: %d | Dirs: %d", formatBytes(stats.SizeBytes), stats.Files, stats.Directories))
		keys := make([]string, 0, len(stats.ToolMetrics))
		for key := range stats.ToolMetrics {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys[:min(len(keys), 10)] {
			lines = append(lines, key+": "+stats.ToolMetrics[key])
		}
		tone := ""
		if stats.Error != "" {
			lines = append(lines, "Error: "+stats.Error)
			tone = "danger"
		}
		rows = append(rows, JobDetailRowView{Label: firstNonEmpty(stats.ID, "cache"), Value: strings.Join(lines, "\n"), Tone: tone})
	}
	return rows, ""
}

func presentToolRequirements(details domain.JobExecutionDetails, requiredPrefix, runtimePrefix, emptyLabel string) ToolRequirementsView {
	type requirement struct{ tool, constraint string }
	items := make([]requirement, 0)
	for key, constraint := range details.RequiredCapabilities {
		if strings.HasPrefix(key, requiredPrefix) {
			if tool := strings.TrimSpace(strings.TrimPrefix(key, requiredPrefix)); tool != "" {
				items = append(items, requirement{tool: tool, constraint: strings.TrimSpace(constraint)})
			}
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].tool < items[j].tool })
	if len(items) == 0 {
		return ToolRequirementsView{EmptyLabel: emptyLabel}
	}
	observed := false
	issues := make([]string, 0)
	for _, item := range items {
		actual := strings.TrimSpace(details.RuntimeCapabilities[runtimePrefix+item.tool])
		observed = observed || actual != ""
		if !requirements.ToolConstraintMatch(actual, item.constraint) {
			expected := firstNonEmpty(item.constraint, "*")
			issues = append(issues, fmt.Sprintf("%s expected %s, got %s", item.tool, expected, firstNonEmpty(actual, "missing")))
		}
	}
	if !observed {
		status := strings.ToLower(strings.TrimSpace(details.Status))
		message := "Runtime capability report unavailable for this execution."
		if status == "queued" || status == "waiting" {
			message = "No agent has leased this job yet; runtime capability data is not available."
		} else if status == "running" || status == "leased" || status == "in progress" {
			message = "Waiting for the leased agent runtime capability report."
		}
		return ToolRequirementsView{EmptyLabel: message}
	}
	if len(issues) == 0 {
		return ToolRequirementsView{Summary: "Requirements matched", Tone: "success"}
	}
	return ToolRequirementsView{Summary: "Requirements mismatch", Tone: "danger", Issues: issues}
}

func presentReleaseSummary(details domain.JobExecutionDetails) ([]JobDetailRowView, bool) {
	if details.PipelineID != "release" {
		return nil, false
	}
	mode := "live"
	if details.DryRun {
		mode = "dry-run"
	}
	rows := []JobDetailRowView{{Label: "Mode", Value: mode}}
	for _, field := range []struct{ key, label string }{
		{domain.ExecutionMetadataVersion, "Version"}, {domain.ExecutionMetadataPipelineVersionRaw, "Version"},
		{domain.ExecutionMetadataTag, "Tag"}, {domain.ExecutionMetadataPipelineVersion, "Tag"}, {domain.ExecutionMetadataArtifacts, "Assets"},
		{domain.ExecutionMetadataNextVersion, "Next version"}, {domain.ExecutionMetadataAutoBumpBranch, "Auto bump branch"},
	} {
		value := details.Metadata.Value(field.key)
		if value == "" {
			continue
		}
		already := false
		for _, row := range rows {
			already = already || row.Label == field.label
		}
		if !already {
			rows = append(rows, JobDetailRowView{Label: field.label, Value: value})
		}
	}
	if len(rows) == 1 {
		rows = append(rows, JobDetailRowView{Value: "No release metadata reported yet.", Tone: "muted"})
	}
	return rows, true
}

func formatBytes(value int64) string {
	if value < 0 {
		return "0 B"
	}
	if value < 1024 {
		return fmt.Sprintf("%d B", value)
	}
	units := []string{"KB", "MB", "GB", "TB"}
	size := float64(value) / 1024
	unitIndex := 0
	for size >= 1024 && unitIndex < len(units)-1 {
		size /= 1024
		unitIndex++
	}
	precision := 2
	if size >= 10 {
		precision = 1
	}
	formatted := fmt.Sprintf("%.*f", precision, size)
	if precision == 2 {
		formatted = strings.TrimSuffix(formatted, ".00")
		if strings.HasSuffix(formatted, "0") && strings.Contains(formatted, ".") {
			formatted = strings.TrimSuffix(formatted, "0")
		}
	}
	return formatted + " " + units[unitIndex]
}

func applySchedulingDiagnosis(view *JobDetailsView, diagnosis *domain.SchedulingDiagnosis) {
	if view == nil || diagnosis == nil {
		return
	}
	view.SchedulingState = diagnosis.State
	view.SchedulingSummary = diagnosis.Summary
	view.SchedulingRequirements = strings.Join(diagnosis.Requirements, " · ")
	incompatibleShown := 0
	incompatibleTotal := 0
	for _, agent := range diagnosis.Agents {
		if !agent.CapabilityMatch {
			incompatibleTotal++
			if incompatibleShown >= 3 {
				continue
			}
			incompatibleShown++
		}
		status, tone := "Does not match", "danger"
		details := make([]string, 0, len(agent.CapabilityIssues)+len(agent.AvailabilityIssues))
		if agent.CapabilityMatch && agent.Available {
			status, tone = "Eligible", "success"
		} else if agent.CapabilityMatch {
			status, tone = "Unavailable", "warning"
		}
		details = append(details, agent.AvailabilityIssues...)
		for _, issue := range agent.CapabilityIssues {
			details = append(details, issue.Message)
		}
		view.SchedulingAgents = append(view.SchedulingAgents, SchedulingAgentView{
			AgentID: agent.AgentID, Status: status, Details: strings.Join(details, "; "), Tone: tone,
		})
	}
	if hidden := incompatibleTotal - incompatibleShown; hidden > 0 {
		view.SchedulingAdditional = fmt.Sprintf("%d additional agent(s) do not match", hidden)
	}
}

func canCancelJob(details domain.JobExecutionDetails) bool {
	switch strings.ToLower(strings.TrimSpace(details.Status)) {
	case "queued", "leased", "running", "in progress":
		return true
	default:
		return false
	}
}

func canRerunJob(details domain.JobExecutionDetails) bool {
	if !details.StartedUTC.IsZero() {
		return true
	}
	if strings.ToLower(strings.TrimSpace(details.Status)) != "failed" {
		return false
	}
	reason := strings.ToLower(strings.TrimSpace(details.Error))
	return (strings.HasPrefix(reason, "cancelled: required job ") || strings.HasPrefix(reason, "cancelled: upstream pipeline ")) && strings.HasSuffix(reason, " failed")
}

func jobContext(details domain.JobExecutionDetails) string {
	parts := make([]string, 0, 4)
	if details.ProjectName != "" {
		parts = append(parts, details.ProjectName)
	}
	if details.PipelineID != "" {
		parts = append(parts, "pipeline "+details.PipelineID)
	}
	if details.MatrixName != "" {
		parts = append(parts, details.MatrixName)
	}
	parts = append(parts, "execution "+details.ID)
	return strings.Join(parts, " · ")
}

func formatTimestamp(value time.Time) string {
	return DeclarativeTimestamp(value)
}

func formatDuration(value time.Duration) string {
	if value < 0 {
		return ""
	}
	value = value.Round(time.Millisecond)
	return value.String()
}

func formatDurationMS(value int64) string {
	if value <= 0 {
		return ""
	}
	totalSeconds := value / 1000
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60
	if hours > 0 {
		return fmt.Sprintf("%02dh %02dm %02ds", hours, minutes, seconds)
	}
	return fmt.Sprintf("%02dm %02ds", minutes, seconds)
}

func formatExitCode(value *int) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(*value)
}

func humanStatus(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(value, "_", " "), "-", " "))
	if value == "" {
		return "Unknown"
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return "execution"
}

func outputEventText(event domain.JobOutputEvent) string {
	text := ""
	switch event.Type {
	case domain.JobOutputEventSystemMessage:
		text = event.Message
	case domain.JobOutputEventOutput:
		text = event.Output
	case domain.JobOutputEventFinished:
		return ""
	}
	text = cleanOutputText(text)
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return text
}

var outputANSIEscapeRE = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]|\x1b\][^\x07]*(?:\x07|\x1b\\)|\x1b[@-Z\\-_]`)

func cleanOutputText(text string) string {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	text = outputANSIEscapeRE.ReplaceAllString(text, "")
	var clean strings.Builder
	for len(text) > 0 {
		r, size := utf8.DecodeRuneInString(text)
		text = text[size:]
		if r == utf8.RuneError && size == 1 {
			continue
		}
		if r == '\n' || r == '\t' || r >= 0x20 {
			clean.WriteRune(r)
		}
	}
	return clean.String()
}
