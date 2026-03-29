package jobhistory

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/server/httpx"
)

type Store interface {
	ListJobExecutions() ([]protocol.JobExecution, error)
}

type HandlerDeps struct {
	Store                   Store
	AttachTestSummaries     func([]protocol.JobExecution)
	AttachUnmetRequirements func([]protocol.JobExecution)
}

type LayoutResponse struct {
	Offset     int          `json:"offset"`
	Limit      int          `json:"limit"`
	TotalCards int          `json:"total_cards"`
	Cards      []LayoutCard `json:"cards"`
}

type LayoutCard struct {
	Key               string `json:"key"`
	Kind              string `json:"kind"`
	CollapsedRowsHint int    `json:"collapsed_rows_hint"`
	ExpandedRowsHint  int    `json:"expanded_rows_hint"`
}

type CardsResponse struct {
	Offset     int        `json:"offset"`
	Limit      int        `json:"limit"`
	TotalCards int        `json:"total_cards"`
	Detail     string     `json:"detail"`
	Cards      []CardView `json:"cards"`
}

type CardView struct {
	Key      string        `json:"key"`
	Kind     string        `json:"kind"`
	Title    string        `json:"title"`
	Summary  SummaryView   `json:"summary"`
	Shape    ShapeView     `json:"shape"`
	Sections []SectionView `json:"sections,omitempty"`
}

type SummaryView struct {
	TotalJobs  int `json:"total_jobs"`
	Succeeded  int `json:"succeeded"`
	Failed     int `json:"failed"`
	InProgress int `json:"in_progress"`
}

type ShapeView struct {
	PipelineSections int `json:"pipeline_sections"`
	MatrixSections   int `json:"matrix_sections"`
	ExpandedRowsHint int `json:"expanded_rows_hint"`
}

type SectionView struct {
	Kind  string     `json:"kind"`
	Key   string     `json:"key"`
	Label string     `json:"label"`
	Items []ItemView `json:"items"`
}

type ItemView struct {
	Kind        string     `json:"kind"`
	Key         string     `json:"key,omitempty"`
	Label       string     `json:"label,omitempty"`
	MatrixLabel string     `json:"matrix_label,omitempty"`
	Job         *JobView   `json:"job,omitempty"`
	Items       []ItemView `json:"items,omitempty"`
}

type JobView struct {
	ID                   string                            `json:"id"`
	Script               string                            `json:"script"`
	Env                  map[string]string                 `json:"env,omitempty"`
	RequiredCapabilities map[string]string                 `json:"required_capabilities"`
	TimeoutSeconds       int                               `json:"timeout_seconds"`
	ArtifactGlobs        []string                          `json:"artifact_globs,omitempty"`
	Caches               []protocol.JobCacheSpec           `json:"caches,omitempty"`
	Source               *protocol.SourceSpec              `json:"source,omitempty"`
	Metadata             map[string]string                 `json:"metadata,omitempty"`
	StepPlan             []protocol.JobStepPlanItem        `json:"step_plan,omitempty"`
	CurrentStep          string                            `json:"current_step,omitempty"`
	CacheStats           []protocol.JobCacheStats          `json:"cache_stats,omitempty"`
	RuntimeCapabilities  map[string]string                 `json:"runtime_capabilities,omitempty"`
	Status               string                            `json:"status"`
	CreatedUTC           time.Time                         `json:"created_utc"`
	StartedUTC           *time.Time                        `json:"started_utc,omitempty"`
	FinishedUTC          *time.Time                        `json:"finished_utc,omitempty"`
	LeasedByAgentID      string                            `json:"leased_by_agent_id,omitempty"`
	LeasedUTC            *time.Time                        `json:"leased_utc,omitempty"`
	ExitCode             *int                              `json:"exit_code,omitempty"`
	Error                string                            `json:"error,omitempty"`
	Output               string                            `json:"output,omitempty"`
	TestSummary          *protocol.JobExecutionTestSummary `json:"test_summary,omitempty"`
	UnmetRequirements    []string                          `json:"unmet_requirements,omitempty"`
	SensitiveValues      []string                          `json:"sensitive_values,omitempty"`
}

type executionCard struct {
	Key            string
	Kind           string
	Indices        []int
	VisibleIndices []int
}

func HandleLayout(w http.ResponseWriter, r *http.Request, deps HandlerDeps) {
	if deps.Store == nil {
		http.Error(w, "job history store unavailable", http.StatusInternalServerError)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	jobs, cards, page, offset, limit, err := loadHistoryPage(r, deps)
	_ = jobs
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := make([]LayoutCard, 0, len(page))
	for _, card := range page {
		out = append(out, layoutCardView(jobs, card))
	}
	httpx.WriteJSON(w, http.StatusOK, LayoutResponse{
		Offset: offset, Limit: limit, TotalCards: len(cards), Cards: out,
	})
}

func HandleCards(w http.ResponseWriter, r *http.Request, deps HandlerDeps) {
	if deps.Store == nil {
		http.Error(w, "job history store unavailable", http.StatusInternalServerError)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	detail := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("detail")))
	if detail == "" {
		detail = "summary"
	}
	if detail != "summary" && detail != "full" {
		http.Error(w, "invalid detail", http.StatusBadRequest)
		return
	}
	jobs, cards, page, offset, limit, err := loadHistoryPage(r, deps)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if detail == "full" {
		enrichPageJobs(jobs, page, deps)
	}
	out := make([]CardView, 0, len(page))
	for _, card := range page {
		out = append(out, cardView(jobs, card, detail == "full"))
	}
	httpx.WriteJSON(w, http.StatusOK, CardsResponse{
		Offset: offset, Limit: limit, TotalCards: len(cards), Detail: detail, Cards: out,
	})
}

func HandleQueueLayout(w http.ResponseWriter, r *http.Request, deps HandlerDeps) {
	if deps.Store == nil {
		http.Error(w, "job history store unavailable", http.StatusInternalServerError)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	jobs, cards, page, offset, limit, err := loadQueuePage(r, deps)
	_ = jobs
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := make([]LayoutCard, 0, len(page))
	for _, card := range page {
		out = append(out, layoutCardView(jobs, card))
	}
	httpx.WriteJSON(w, http.StatusOK, LayoutResponse{
		Offset: offset, Limit: limit, TotalCards: len(cards), Cards: out,
	})
}

func HandleQueueCards(w http.ResponseWriter, r *http.Request, deps HandlerDeps) {
	if deps.Store == nil {
		http.Error(w, "job history store unavailable", http.StatusInternalServerError)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	detail := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("detail")))
	if detail == "" {
		detail = "summary"
	}
	if detail != "summary" && detail != "full" {
		http.Error(w, "invalid detail", http.StatusBadRequest)
		return
	}
	jobs, cards, page, offset, limit, err := loadQueuePage(r, deps)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if detail == "full" {
		enrichPageJobs(jobs, page, deps)
	}
	out := make([]CardView, 0, len(page))
	for _, card := range page {
		out = append(out, cardView(jobs, card, detail == "full"))
	}
	httpx.WriteJSON(w, http.StatusOK, CardsResponse{
		Offset: offset, Limit: limit, TotalCards: len(cards), Detail: detail, Cards: out,
	})
}

func loadHistoryPage(r *http.Request, deps HandlerDeps) ([]protocol.JobExecution, []executionCard, []executionCard, int, int, error) {
	jobs, err := deps.Store.ListJobExecutions()
	if err != nil {
		return nil, nil, nil, 0, 0, err
	}
	cards := buildExecutionCards(jobs, func(job protocol.JobExecution) bool {
		return !protocol.IsActiveJobExecutionStatus(job.Status)
	})
	offset := parseQueryInt(r, "offset", 0, 0, 1_000_000)
	limit := parseQueryInt(r, "limit", 20, 1, 200)
	page := paginateCards(cards, offset, limit)
	return jobs, cards, page, offset, limit, nil
}

func loadQueuePage(r *http.Request, deps HandlerDeps) ([]protocol.JobExecution, []executionCard, []executionCard, int, int, error) {
	jobs, err := deps.Store.ListJobExecutions()
	if err != nil {
		return nil, nil, nil, 0, 0, err
	}
	cards := buildExecutionCards(jobs, func(job protocol.JobExecution) bool {
		return protocol.IsActiveJobExecutionStatus(job.Status)
	})
	offset := parseQueryInt(r, "offset", 0, 0, 1_000_000)
	limit := parseQueryInt(r, "limit", 20, 1, 200)
	page := paginateCards(cards, offset, limit)
	return jobs, cards, page, offset, limit, nil
}

func parseQueryInt(r *http.Request, key string, fallback, min, max int) int {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func buildExecutionCards(jobs []protocol.JobExecution, visibleFn func(protocol.JobExecution) bool) []executionCard {
	byChain := map[string]*executionCard{}
	byPipeline := map[string]*executionCard{}
	singles := map[int]bool{}
	for i, job := range jobs {
		if key := chainCardKey(job); key != "" {
			card := byChain[key]
			if card == nil {
				card = &executionCard{Key: key, Kind: "chain"}
				byChain[key] = card
			}
			card.Indices = append(card.Indices, i)
			if visibleFn == nil || visibleFn(job) {
				card.VisibleIndices = append(card.VisibleIndices, i)
			}
			continue
		}
		if key := pipelineCardKey(job); key != "" {
			card := byPipeline[key]
			if card == nil {
				card = &executionCard{Key: key, Kind: "pipeline"}
				byPipeline[key] = card
			}
			card.Indices = append(card.Indices, i)
			if visibleFn == nil || visibleFn(job) {
				card.VisibleIndices = append(card.VisibleIndices, i)
			}
			continue
		}
		singles[i] = true
	}
	if visibleFn != nil {
		for idx := range singles {
			if visibleFn(jobs[idx]) {
				continue
			}
			delete(singles, idx)
		}
	}
	consumed := map[int]struct{}{}
	out := make([]executionCard, 0, len(jobs))
	for i, job := range jobs {
		if _, ok := consumed[i]; ok {
			continue
		}
		if key := chainCardKey(job); key != "" {
			card := byChain[key]
			if card == nil || (visibleFn != nil && len(card.VisibleIndices) == 0) {
				for _, idx := range card.Indices {
					consumed[idx] = struct{}{}
				}
				continue
			}
			for _, idx := range card.Indices {
				consumed[idx] = struct{}{}
			}
			out = append(out, executionCard{
				Key: key, Kind: "chain",
				Indices:        append([]int(nil), card.Indices...),
				VisibleIndices: append([]int(nil), card.VisibleIndices...),
			})
			continue
		}
		if key := pipelineCardKey(job); key != "" {
			card := byPipeline[key]
			if card == nil || (visibleFn != nil && len(card.VisibleIndices) == 0) {
				for _, idx := range card.Indices {
					consumed[idx] = struct{}{}
				}
				continue
			}
			for _, idx := range card.Indices {
				consumed[idx] = struct{}{}
			}
			out = append(out, executionCard{
				Key: key, Kind: "pipeline",
				Indices:        append([]int(nil), card.Indices...),
				VisibleIndices: append([]int(nil), card.VisibleIndices...),
			})
			continue
		}
		if visibleFn != nil && !visibleFn(job) {
			consumed[i] = struct{}{}
			continue
		}
		consumed[i] = struct{}{}
		out = append(out, executionCard{Key: "job:" + strings.TrimSpace(job.ID), Kind: "job", Indices: []int{i}, VisibleIndices: []int{i}})
	}
	return out
}

func paginateCards(cards []executionCard, offset, limit int) []executionCard {
	if offset >= len(cards) {
		return nil
	}
	end := offset + limit
	if end > len(cards) {
		end = len(cards)
	}
	return append([]executionCard(nil), cards[offset:end]...)
}

func enrichPageJobs(jobs []protocol.JobExecution, cards []executionCard, deps HandlerDeps) {
	indices := make([]int, 0)
	for _, card := range cards {
		indices = append(indices, card.VisibleIndices...)
	}
	if len(indices) == 0 {
		return
	}
	sort.Ints(indices)
	pageJobs := make([]protocol.JobExecution, len(indices))
	for i, idx := range indices {
		pageJobs[i] = jobs[idx]
	}
	if deps.AttachTestSummaries != nil {
		deps.AttachTestSummaries(pageJobs)
	}
	if deps.AttachUnmetRequirements != nil {
		deps.AttachUnmetRequirements(pageJobs)
	}
	for i, idx := range indices {
		jobs[idx] = pageJobs[i]
	}
}

func layoutCardView(jobs []protocol.JobExecution, card executionCard) LayoutCard {
	shape := cardShape(jobs, card)
	return LayoutCard{
		Key: card.Key, Kind: card.Kind, CollapsedRowsHint: 1, ExpandedRowsHint: shape.ExpandedRowsHint,
	}
}

func cardView(jobs []protocol.JobExecution, card executionCard, includeSections bool) CardView {
	shape := cardShape(jobs, card)
	out := CardView{
		Key:     card.Key,
		Kind:    card.Kind,
		Title:   cardTitle(jobs, card),
		Summary: summarizeCard(jobs, card),
		Shape:   shape,
	}
	if includeSections {
		out.Sections = buildSections(jobs, card)
	}
	return out
}

func summarizeCard(jobs []protocol.JobExecution, card executionCard) SummaryView {
	out := SummaryView{TotalJobs: len(card.Indices)}
	for _, idx := range card.Indices {
		status := protocol.NormalizeJobExecutionStatus(jobs[idx].Status)
		switch {
		case status == protocol.JobExecutionStatusSucceeded:
			out.Succeeded++
		case status == protocol.JobExecutionStatusFailed:
			out.Failed++
		case protocol.IsActiveJobExecutionStatus(status):
			out.InProgress++
		}
	}
	return out
}

func cardShape(jobs []protocol.JobExecution, card executionCard) ShapeView {
	sections := buildSections(jobs, card)
	out := ShapeView{PipelineSections: len(sections)}
	rows := 0
	for _, section := range sections {
		rows++ // section header
		for _, item := range section.Items {
			if item.Kind == "matrix" {
				out.MatrixSections++
				rows++ // matrix header
				rows += len(item.Items)
				continue
			}
			rows++
		}
	}
	if rows == 0 {
		rows = 1
	}
	out.ExpandedRowsHint = rows
	return out
}

func buildSections(jobs []protocol.JobExecution, card executionCard) []SectionView {
	switch card.Kind {
	case "job":
		job := jobs[card.VisibleIndices[0]]
		return []SectionView{{
			Kind:  "pipeline",
			Key:   "section:" + strings.TrimSpace(job.ID),
			Label: strings.TrimSpace((job.Metadata)["pipeline_id"]),
			Items: []ItemView{{Kind: "job", Job: jobView(job)}},
		}}
	default:
		return buildPipelineSections(jobs, card)
	}
}

func buildPipelineSections(jobs []protocol.JobExecution, card executionCard) []SectionView {
	type sectionState struct {
		key   string
		label string
		jobs  []protocol.JobExecution
	}
	ordered := make([]sectionState, 0)
	byKey := map[string]int{}
	for _, idx := range card.VisibleIndices {
		job := jobs[idx]
		sectionKey := pipelineSectionKey(job)
		if sectionKey == "" {
			sectionKey = "section:" + strings.TrimSpace(job.ID)
		}
		pos, ok := byKey[sectionKey]
		if !ok {
			pos = len(ordered)
			byKey[sectionKey] = pos
			ordered = append(ordered, sectionState{
				key:   sectionKey,
				label: strings.TrimSpace(job.Metadata["pipeline_id"]),
			})
		}
		ordered[pos].jobs = append(ordered[pos].jobs, job)
	}

	out := make([]SectionView, 0, len(ordered))
	for _, state := range ordered {
		out = append(out, SectionView{
			Kind:  "pipeline",
			Key:   state.key,
			Label: state.label,
			Items: buildSectionItems(state.jobs),
		})
	}
	return out
}

func buildSectionItems(jobs []protocol.JobExecution) []ItemView {
	type matrixState struct {
		key   string
		label string
		items []ItemView
	}
	out := make([]ItemView, 0)
	matrixOrder := make([]matrixState, 0)
	matrixPos := map[string]int{}
	flushMatrix := func(key string) {
		if key == "" {
			return
		}
		pos := matrixPos[key]
		state := matrixOrder[pos]
		out = append(out, ItemView{Kind: "matrix", Key: state.key, Label: state.label, Items: append([]ItemView(nil), state.items...)})
		delete(matrixPos, key)
		matrixOrder[pos] = matrixState{}
	}
	activeMatrixKey := ""
	for _, job := range jobs {
		matrixKey := matrixGroupKey(job)
		if matrixKey == "" {
			flushMatrix(activeMatrixKey)
			activeMatrixKey = ""
			out = append(out, ItemView{Kind: "job", Job: jobView(job)})
			continue
		}
		if activeMatrixKey != "" && activeMatrixKey != matrixKey {
			flushMatrix(activeMatrixKey)
			activeMatrixKey = ""
		}
		pos, ok := matrixPos[matrixKey]
		if !ok {
			pos = len(matrixOrder)
			matrixPos[matrixKey] = pos
			matrixOrder = append(matrixOrder, matrixState{
				key:   matrixKey,
				label: strings.TrimSpace(job.Metadata["pipeline_job_id"]),
			})
		}
		item := ItemView{Kind: "job", MatrixLabel: matrixEntryLabel(job), Job: jobView(job)}
		matrixOrder[pos].items = append(matrixOrder[pos].items, item)
		activeMatrixKey = matrixKey
	}
	flushMatrix(activeMatrixKey)
	return out
}

func matrixEntryLabel(job protocol.JobExecution) string {
	if name := strings.TrimSpace(job.Metadata["matrix_name"]); name != "" {
		return name
	}
	if idx := strings.TrimSpace(job.Metadata["matrix_index"]); idx != "" {
		return "idx-" + idx
	}
	return ""
}

func cardTitle(jobs []protocol.JobExecution, card executionCard) string {
	if len(card.Indices) == 0 {
		return "job"
	}
	first := jobs[card.Indices[0]]
	project := strings.TrimSpace(first.Metadata["project"])
	label := ""
	switch card.Kind {
	case "chain":
		label = strings.TrimSpace(first.Metadata["pipeline_chain_id"])
	case "pipeline":
		label = strings.TrimSpace(first.Metadata["pipeline_id"])
	default:
		label = strings.TrimSpace(first.Metadata["pipeline_job_id"])
		if label == "" {
			label = "job"
		}
	}
	buildVersion := ""
	oldest := ""
	for _, idx := range card.Indices {
		job := jobs[idx]
		if buildVersion == "" {
			buildVersion = strings.TrimSpace(job.Metadata["build_version"])
		}
		ts := job.CreatedUTC.Format(time.RFC3339Nano)
		if oldest == "" || ts < oldest {
			oldest = ts
		}
	}
	parts := make([]string, 0, 4)
	if project != "" {
		parts = append(parts, project)
	}
	if label != "" {
		parts = append(parts, label)
	}
	if oldest != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, oldest); err == nil {
			parts = append(parts, parsed.Local().Format("Mon 02 Jan, 15:04:05"))
		}
	}
	if buildVersion != "" {
		parts = append(parts, buildVersion)
	}
	if len(parts) == 0 {
		return "job"
	}
	return strings.Join(parts, " ")
}

func pipelineSectionKey(job protocol.JobExecution) string {
	if group := pipelineCardKey(job); group != "" {
		return "section:" + group
	}
	pipelineID := strings.TrimSpace(job.Metadata["pipeline_id"])
	if pipelineID == "" {
		return ""
	}
	return "section:" + pipelineID
}

func chainCardKey(job protocol.JobExecution) string {
	chainRunID := strings.TrimSpace(job.Metadata["chain_run_id"])
	if chainRunID == "" {
		return ""
	}
	return "chain:" + chainRunID
}

func pipelineCardKey(job protocol.JobExecution) string {
	runID := strings.TrimSpace(job.Metadata["pipeline_run_id"])
	if runID == "" {
		return ""
	}
	projectID := strings.TrimSpace(job.Metadata["project_id"])
	if projectID == "" {
		projectID = strings.TrimSpace(job.Metadata["project"])
	}
	pipelineID := strings.TrimSpace(job.Metadata["pipeline_id"])
	return "pipeline:" + runID + "|" + projectID + "|" + pipelineID
}

func matrixGroupKey(job protocol.JobExecution) string {
	if strings.TrimSpace(job.Metadata["matrix_name"]) == "" && strings.TrimSpace(job.Metadata["matrix_index"]) == "" {
		return ""
	}
	pipelineID := strings.TrimSpace(job.Metadata["pipeline_id"])
	pipelineJobID := strings.TrimSpace(job.Metadata["pipeline_job_id"])
	runID := strings.TrimSpace(job.Metadata["pipeline_run_id"])
	if pipelineJobID == "" {
		return ""
	}
	return fmt.Sprintf("matrix:%s|%s|%s", runID, pipelineID, pipelineJobID)
}

func jobView(job protocol.JobExecution) *JobView {
	view := &JobView{
		ID:                   job.ID,
		Script:               job.Script,
		Env:                  job.Env,
		RequiredCapabilities: job.RequiredCapabilities,
		TimeoutSeconds:       job.TimeoutSeconds,
		ArtifactGlobs:        job.ArtifactGlobs,
		Caches:               job.Caches,
		Source:               job.Source,
		Metadata:             job.Metadata,
		StepPlan:             job.StepPlan,
		CurrentStep:          job.CurrentStep,
		CacheStats:           job.CacheStats,
		RuntimeCapabilities:  job.RuntimeCapabilities,
		Status:               protocol.NormalizeJobExecutionStatus(job.Status),
		CreatedUTC:           job.CreatedUTC,
		LeasedByAgentID:      job.LeasedByAgentID,
		ExitCode:             job.ExitCode,
		Error:                job.Error,
		Output:               job.Output,
		TestSummary:          job.TestSummary,
		UnmetRequirements:    job.UnmetRequirements,
		SensitiveValues:      job.SensitiveValues,
	}
	if !job.StartedUTC.IsZero() {
		ts := job.StartedUTC
		view.StartedUTC = &ts
	}
	if !job.FinishedUTC.IsZero() {
		ts := job.FinishedUTC
		view.FinishedUTC = &ts
	}
	if !job.LeasedUTC.IsZero() {
		ts := job.LeasedUTC
		view.LeasedUTC = &ts
	}
	return view
}
