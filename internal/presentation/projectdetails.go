package presentation

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/izzyreal/ciwi/internal/domain"
)

type ProjectDetailsView struct {
	Project           domain.Project
	Pipelines         []ProjectPipelineView
	HistoryExecutions []domain.ExecutionCard
}

type ProjectPipelineView struct {
	ID             int64
	PipelineID     string
	Trigger        string
	DependsOn      []string
	Dependencies   string
	JobsCount      int
	SupportsDryRun bool
	Jobs           []ProjectJobView
}

type ProjectJobView struct {
	ID             string
	Needs          []string
	NeedsLabel     string
	RunsOnLabel    string
	ToolsLabel     string
	TimeoutSeconds int
	MatrixCount    int
	StepsCount     int
	SupportsDryRun bool
	Steps          []ProjectStepView
}

type ProjectStepView struct {
	Index       int
	Position    int
	Name        string
	Type        string
	Command     string
	SkipDryRun  bool
	Environment []string
}

type ProjectDetailsQueries struct {
	projects interface {
		GetProjectDetails(context.Context, int64) (domain.ProjectDetails, error)
	}
	executions interface {
		ListFrontPageExecutionCards(context.Context) ([]domain.ExecutionCard, []domain.ExecutionCard, error)
	}
}

func NewProjectDetailsQueries(projects interface {
	GetProjectDetails(context.Context, int64) (domain.ProjectDetails, error)
}, executions ...interface {
	ListFrontPageExecutionCards(context.Context) ([]domain.ExecutionCard, []domain.ExecutionCard, error)
}) *ProjectDetailsQueries {
	queries := &ProjectDetailsQueries{projects: projects}
	if len(executions) > 0 {
		queries.executions = executions[0]
	}
	return queries
}

func (q *ProjectDetailsQueries) GetProjectDetailsView(ctx context.Context, projectID int64) (ProjectDetailsView, error) {
	details, err := q.projects.GetProjectDetails(ctx, projectID)
	if err != nil {
		return ProjectDetailsView{}, err
	}
	pipelineSupport := make(map[string]bool, len(details.Project.Pipelines))
	for _, pipeline := range details.Project.Pipelines {
		pipelineSupport[pipeline.PipelineID] = pipeline.SupportsDryRun
	}
	pipelines := make([]ProjectPipelineView, 0, len(details.Pipelines))
	for _, pipeline := range details.Pipelines {
		jobs := make([]ProjectJobView, 0, len(pipeline.Jobs))
		for _, job := range pipeline.Jobs {
			steps := make([]ProjectStepView, 0, len(job.Steps))
			supportsDryRun := false
			for _, step := range job.Steps {
				supportsDryRun = supportsDryRun || step.SkipDryRun
				name := strings.TrimSpace(step.Name)
				if name == "" && step.Type == "test" {
					name = "test " + strings.TrimSpace(step.TestName)
				}
				if strings.TrimSpace(name) == "test" || name == "" {
					name = fmt.Sprintf("step %d", step.Index+1)
				}
				steps = append(steps, ProjectStepView{
					Index: step.Index, Position: step.Index + 1, Name: name,
					Type: defaultValue(step.Type, "run"), Command: step.Command,
					SkipDryRun: step.SkipDryRun, Environment: sortedKeys(step.Environment),
				})
			}
			jobs = append(jobs, ProjectJobView{
				ID: job.ID, Needs: append([]string{}, job.Needs...), NeedsLabel: strings.Join(job.Needs, ", "),
				RunsOnLabel: keyValueLabel(job.RunsOn), ToolsLabel: keyValueLabel(job.RequiresTools),
				TimeoutSeconds: job.TimeoutSeconds, MatrixCount: job.MatrixCount,
				StepsCount: len(steps), SupportsDryRun: supportsDryRun, Steps: steps,
			})
		}
		dependencies := strings.Join(pipeline.DependsOn, ", ")
		if dependencies == "" {
			dependencies = "none"
		}
		pipelines = append(pipelines, ProjectPipelineView{
			ID: pipeline.ID, PipelineID: pipeline.PipelineID, Trigger: pipeline.Trigger,
			DependsOn: append([]string{}, pipeline.DependsOn...), Dependencies: dependencies,
			JobsCount: len(jobs), SupportsDryRun: pipelineSupport[pipeline.PipelineID], Jobs: jobs,
		})
	}
	history := []domain.ExecutionCard{}
	if q.executions != nil {
		_, allHistory, err := q.executions.ListFrontPageExecutionCards(ctx)
		if err != nil {
			return ProjectDetailsView{}, err
		}
		history = projectExecutionCards(allHistory, projectID)
	}
	return ProjectDetailsView{Project: details.Project, Pipelines: pipelines, HistoryExecutions: history}, nil
}

func projectExecutionCards(cards []domain.ExecutionCard, projectID int64) []domain.ExecutionCard {
	filtered := make([]domain.ExecutionCard, 0, len(cards))
	for _, card := range cards {
		matched := false
		for _, section := range card.Sections {
			for _, job := range section.Jobs {
				if job.ProjectID == projectID {
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
		if matched {
			filtered = append(filtered, card)
		}
	}
	return filtered
}

func keyValueLabel(values map[string]string) string {
	keys := sortedKeys(values)
	if len(keys) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		value := values[key]
		if value == "" {
			value = "*"
		}
		parts = append(parts, key+"="+value)
	}
	return strings.Join(parts, ", ")
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func defaultValue(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func PipelineChainSequenceLabel(pipelines []string) string {
	return strings.Join(pipelines, " → ")
}
