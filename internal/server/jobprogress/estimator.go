package jobprogress

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

const (
	maxSamples      = 10
	maxCache        = 256
	maxFingerprints = 4096
)

type Store interface {
	ListJobExecutions() ([]protocol.JobExecution, error)
	ListJobExecutionDurationEventsForJobs(jobIDs []string) (map[string][]protocol.JobExecutionEvent, error)
}

type Estimate struct {
	ExpectedDurationMS    int64
	StepExpectedDuration  map[int]int64
	PhaseExpectedDuration map[string]int64
}

type Estimator struct {
	store            Store
	mu               sync.Mutex
	cache            map[string]Estimate
	order            []string
	fingerprints     map[string]string
	fingerprintOrder []string
}

func New(store Store) *Estimator {
	return &Estimator{store: store, cache: make(map[string]Estimate), fingerprints: make(map[string]string)}
}

// AttachJobEstimates uses only the already-loaded execution records. Queue
// cards do not need the more expensive per-step event history.
func (e *Estimator) AttachJobEstimates(jobs []protocol.JobExecution) {
	exactHistory := make(map[string][]protocol.JobExecution)
	provisionalHistory := make(map[string][]protocol.JobExecution)
	completed := append([]protocol.JobExecution(nil), jobs...)
	sort.Slice(completed, func(i, j int) bool { return completed[i].CreatedUTC.After(completed[j].CreatedUTC) })
	for _, job := range completed {
		if protocol.NormalizeJobExecutionStatus(job.Status) != protocol.JobExecutionStatusSucceeded || job.StartedUTC.IsZero() || job.FinishedUTC.IsZero() || !job.FinishedUTC.After(job.StartedUTC) {
			continue
		}
		if key := e.comparableJobKey(job); key != "" && len(exactHistory[key]) < maxSamples {
			exactHistory[key] = append(exactHistory[key], job)
		}
		if key := e.provisionalJobKey(job); key != "" && len(provisionalHistory[key]) < maxSamples {
			provisionalHistory[key] = append(provisionalHistory[key], job)
		}
	}
	for i := range jobs {
		if !protocol.IsActiveJobExecutionStatus(jobs[i].Status) {
			continue
		}
		previous := previousJobExecutions(exactHistory[e.comparableJobKey(jobs[i])], jobs[i].CreatedUTC)
		if len(previous) == 0 {
			previous = previousJobExecutions(provisionalHistory[e.provisionalJobKey(jobs[i])], jobs[i].CreatedUTC)
		}
		jobs[i].ExpectedDurationMS = median(jobDurations(previous))
	}
}

func previousJobExecutions(candidates []protocol.JobExecution, before time.Time) []protocol.JobExecution {
	out := make([]protocol.JobExecution, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.CreatedUTC.Before(before) {
			out = append(out, candidate)
		}
	}
	return out
}

func (e *Estimator) AttachDetailEstimate(job *protocol.JobExecution) error {
	if job == nil || e == nil || e.store == nil {
		return nil
	}
	cacheKey := strings.TrimSpace(job.ID) + "|" + strings.TrimSpace(job.LeasedByAgentID)
	if estimate, ok := e.cached(cacheKey); ok {
		applyEstimate(job, estimate)
		return nil
	}

	jobs, err := e.store.ListJobExecutions()
	if err != nil {
		return err
	}
	jobMatches := e.comparableSuccessfulJobs(*job, jobs)
	exactUnitMatches, fallbackUnitMatches := e.comparableCompletedExecutionUnits(*job, jobs)
	estimate := Estimate{ExpectedDurationMS: median(jobDurations(jobMatches))}
	if len(exactUnitMatches)+len(fallbackUnitMatches) > 0 {
		ids := make([]string, 0, len(exactUnitMatches)+len(fallbackUnitMatches))
		for _, match := range exactUnitMatches {
			ids = append(ids, match.ID)
		}
		for _, match := range fallbackUnitMatches {
			ids = append(ids, match.ID)
		}
		eventsByJob, err := e.store.ListJobExecutionDurationEventsForJobs(ids)
		if err != nil {
			return err
		}
		if len(job.StepPlan) > 0 {
			estimate.StepExpectedDuration = estimateSteps(job.StepPlan, exactUnitMatches, fallbackUnitMatches, eventsByJob)
		}
		estimate.PhaseExpectedDuration = estimatePhases(*job, exactUnitMatches, fallbackUnitMatches, eventsByJob)
	}
	e.remember(cacheKey, estimate)
	applyEstimate(job, estimate)
	return nil
}

func estimatePhases(target protocol.JobExecution, exactMatches, fallbackMatches []protocol.JobExecution, eventsByJob map[string][]protocol.JobExecutionEvent) map[string]int64 {
	wanted := map[string]struct{}{}
	for _, item := range protocol.BuildJobExecutionTimeline(target) {
		if item.Kind == "phase" && strings.TrimSpace(item.ID) != "" {
			wanted[item.ID] = struct{}{}
		}
	}
	exactSamples := collectPhaseSamples(target, wanted, exactMatches, eventsByJob)
	fallbackSamples := collectPhaseSamples(target, wanted, fallbackMatches, eventsByJob)
	out := make(map[string]int64)
	for id := range wanted {
		durations := exactSamples[id]
		if len(durations) == 0 {
			durations = fallbackSamples[id]
		}
		if value := median(durations); value > 0 {
			out[id] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func collectPhaseSamples(target protocol.JobExecution, wanted map[string]struct{}, matches []protocol.JobExecution, eventsByJob map[string][]protocol.JobExecutionEvent) map[string][]int64 {
	samples := make(map[string][]int64, len(wanted))
	for _, match := range matches {
		for _, event := range eventsByJob[match.ID] {
			if event.Type != protocol.JobExecutionEventTypePhaseFinished || event.Phase == nil || event.DurationMS <= 0 ||
				(event.ExitCode != nil && *event.ExitCode != 0) || strings.TrimSpace(event.Error) != "" {
				continue
			}
			id := strings.TrimSpace(event.Phase.ID)
			if _, ok := wanted[id]; ok && len(samples[id]) < maxSamples && phaseDefinitionFingerprint(target, id) == phaseDefinitionFingerprint(match, id) {
				samples[id] = append(samples[id], event.DurationMS)
			}
		}
	}
	return samples
}

func phaseDefinitionFingerprint(job protocol.JobExecution, phaseID string) string {
	var definition any
	switch phaseID {
	case protocol.JobExecutionPhaseWorkspace:
		definition = phaseID
	case protocol.JobExecutionPhaseCheckout:
		repo := ""
		if job.Source != nil {
			repo = strings.TrimSpace(job.Source.Repo)
		}
		definition = struct {
			ID   string `json:"id"`
			Repo string `json:"repo"`
		}{phaseID, repo}
	case protocol.JobExecutionPhaseDependencies:
		definition = struct {
			ID      string `json:"id"`
			Enabled bool   `json:"enabled"`
		}{phaseID, timelineHasPhase(job, phaseID)}
	case protocol.JobExecutionPhaseEnvironment:
		definition = struct {
			ID     string                  `json:"id"`
			Caches []protocol.JobCacheSpec `json:"caches,omitempty"`
		}{phaseID, job.Caches}
	case protocol.JobExecutionPhaseArtifacts:
		definition = struct {
			ID    string   `json:"id"`
			Globs []string `json:"globs,omitempty"`
		}{phaseID, job.ArtifactGlobs}
	case protocol.JobExecutionPhaseTests:
		type reportDefinition struct {
			TestFormat     string `json:"test_format,omitempty"`
			TestReport     string `json:"test_report,omitempty"`
			CoverageFormat string `json:"coverage_format,omitempty"`
			CoverageReport string `json:"coverage_report,omitempty"`
		}
		reports := make([]reportDefinition, 0, len(job.StepPlan))
		for _, step := range job.StepPlan {
			if strings.TrimSpace(step.TestReport) == "" && strings.TrimSpace(step.CoverageReport) == "" {
				continue
			}
			reports = append(reports, reportDefinition{
				TestFormat: step.TestFormat, TestReport: step.TestReport,
				CoverageFormat: step.CoverageFormat, CoverageReport: step.CoverageReport,
			})
		}
		definition = struct {
			ID      string             `json:"id"`
			Reports []reportDefinition `json:"reports,omitempty"`
		}{phaseID, reports}
	default:
		definition = phaseID
	}
	encoded, _ := json.Marshal(definition)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func timelineHasPhase(job protocol.JobExecution, phaseID string) bool {
	_, ok := protocol.TimelinePhase(protocol.BuildJobExecutionTimeline(job), phaseID)
	return ok
}

func (e *Estimator) comparableSuccessfulJobs(target protocol.JobExecution, jobs []protocol.JobExecution) []protocol.JobExecution {
	if key := e.comparableJobKey(target); key != "" {
		if exact := e.comparableSuccessfulJobsByKey(target, jobs, key, e.comparableJobKey); len(exact) > 0 {
			return exact
		}
	}
	key := e.provisionalJobKey(target)
	return e.comparableSuccessfulJobsByKey(target, jobs, key, e.provisionalJobKey)
}

func (e *Estimator) comparableCompletedExecutionUnits(target protocol.JobExecution, jobs []protocol.JobExecution) ([]protocol.JobExecution, []protocol.JobExecution) {
	exactKey := e.comparableExecutionUnitKey(target)
	fallbackKey := e.provisionalExecutionUnitKey(target)
	targetAgent := strings.TrimSpace(target.LeasedByAgentID)
	exact := make([]protocol.JobExecution, 0)
	fallback := make([]protocol.JobExecution, 0)
	for _, candidate := range jobs {
		if candidate.ID == target.ID || !protocol.IsTerminalJobExecutionStatus(candidate.Status) || !candidate.CreatedUTC.Before(target.CreatedUTC) {
			continue
		}
		if candidate.StartedUTC.IsZero() || candidate.FinishedUTC.IsZero() || !candidate.FinishedUTC.After(candidate.StartedUTC) {
			continue
		}
		if exactKey != "" && e.comparableExecutionUnitKey(candidate) == exactKey {
			exact = append(exact, candidate)
			continue
		}
		if fallbackKey != "" && e.provisionalExecutionUnitKey(candidate) == fallbackKey {
			if targetAgent == "" || strings.TrimSpace(candidate.LeasedByAgentID) != targetAgent {
				fallback = append(fallback, candidate)
			}
		}
	}
	sort.Slice(exact, func(i, j int) bool { return exact[i].CreatedUTC.After(exact[j].CreatedUTC) })
	sort.Slice(fallback, func(i, j int) bool { return fallback[i].CreatedUTC.After(fallback[j].CreatedUTC) })
	return exact, fallback
}

func (e *Estimator) comparableSuccessfulJobsByKey(target protocol.JobExecution, jobs []protocol.JobExecution, key string, keyFor func(protocol.JobExecution) string) []protocol.JobExecution {
	if key == "" {
		return nil
	}
	out := make([]protocol.JobExecution, 0, maxSamples)
	for _, candidate := range jobs {
		if candidate.ID == target.ID || protocol.NormalizeJobExecutionStatus(candidate.Status) != protocol.JobExecutionStatusSucceeded {
			continue
		}
		if !candidate.CreatedUTC.Before(target.CreatedUTC) || keyFor(candidate) != key {
			continue
		}
		if candidate.StartedUTC.IsZero() || candidate.FinishedUTC.IsZero() || !candidate.FinishedUTC.After(candidate.StartedUTC) {
			continue
		}
		out = append(out, candidate)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedUTC.After(out[j].CreatedUTC) })
	if len(out) > maxSamples {
		out = out[:maxSamples]
	}
	return out
}

func (e *Estimator) comparableJobKey(job protocol.JobExecution) string {
	agent := strings.TrimSpace(job.LeasedByAgentID)
	provisional := e.provisionalJobKey(job)
	if agent == "" || provisional == "" {
		return ""
	}
	return agent + "\x1f" + provisional
}

func (e *Estimator) comparableExecutionUnitKey(job protocol.JobExecution) string {
	agent := strings.TrimSpace(job.LeasedByAgentID)
	provisional := e.provisionalExecutionUnitKey(job)
	if agent == "" || provisional == "" {
		return ""
	}
	return agent + "\x1f" + provisional
}

func (e *Estimator) provisionalJobKey(job protocol.JobExecution) string {
	base := e.provisionalExecutionUnitKey(job)
	if base == "" {
		return ""
	}
	parts := []string{
		base,
		e.jobPlanFingerprint(job),
	}
	return strings.Join(parts, "\x1f")
}

// Execution-unit history intentionally omits dry-run mode and the complete job
// plan. Individual step and phase fingerprints decide whether a sample is safe
// to share, while aggregate job duration matching remains strict.
func (e *Estimator) provisionalExecutionUnitKey(job protocol.JobExecution) string {
	m := job.Metadata
	project := strings.TrimSpace(m["project"])
	pipeline := strings.TrimSpace(m["pipeline_id"])
	pipelineJob := strings.TrimSpace(m["pipeline_job_id"])
	if project == "" && pipeline == "" && pipelineJob == "" {
		return ""
	}
	requiredCapabilities, _ := json.Marshal(job.RequiredCapabilities)
	parts := []string{
		project,
		pipeline,
		pipelineJob,
		strings.TrimSpace(m["matrix_name"]),
		strings.TrimSpace(m["matrix_index"]),
		string(requiredCapabilities),
	}
	return strings.Join(parts, "\x1f")
}

type executableStep struct {
	Index           int                          `json:"index"`
	Script          string                       `json:"script"`
	Kind            string                       `json:"kind"`
	Env             map[string]string            `json:"env,omitempty"`
	TestName        string                       `json:"test_name,omitempty"`
	TestFormat      string                       `json:"test_format,omitempty"`
	TestReport      string                       `json:"test_report,omitempty"`
	CoverageFormat  string                       `json:"coverage_format,omitempty"`
	CoverageReport  string                       `json:"coverage_report,omitempty"`
	VaultConnection string                       `json:"vault_connection,omitempty"`
	VaultSecrets    []protocol.ProjectSecretSpec `json:"vault_secrets,omitempty"`
}

func (e *Estimator) jobPlanFingerprint(job protocol.JobExecution) string {
	id := strings.TrimSpace(job.ID)
	if id != "" {
		e.mu.Lock()
		fingerprint, ok := e.fingerprints[id]
		e.mu.Unlock()
		if ok {
			return fingerprint
		}
	}
	steps := make([]executableStep, len(job.StepPlan))
	for i := range job.StepPlan {
		steps[i] = executableStepFromPlan(job.StepPlan[i])
	}
	sourceRepo := ""
	if job.Source != nil {
		sourceRepo = strings.TrimSpace(job.Source.Repo)
	}
	payload := struct {
		Script        string                  `json:"script"`
		Steps         []executableStep        `json:"steps"`
		SourceRepo    string                  `json:"source_repo,omitempty"`
		Caches        []protocol.JobCacheSpec `json:"caches,omitempty"`
		ArtifactGlobs []string                `json:"artifact_globs,omitempty"`
	}{
		Script: job.Script, Steps: steps, SourceRepo: sourceRepo,
		Caches: job.Caches, ArtifactGlobs: job.ArtifactGlobs,
	}
	encoded, _ := json.Marshal(payload)
	sum := sha256.Sum256(encoded)
	fingerprint := hex.EncodeToString(sum[:])
	if id != "" {
		e.rememberFingerprint(id, fingerprint)
	}
	return fingerprint
}

func executableStepFromPlan(step protocol.JobStepPlanItem) executableStep {
	return executableStep{
		Index: step.Index, Script: step.Script, Kind: step.Kind, Env: step.Env,
		TestName: step.TestName, TestFormat: step.TestFormat, TestReport: step.TestReport,
		CoverageFormat: step.CoverageFormat, CoverageReport: step.CoverageReport,
		VaultConnection: step.VaultConnection, VaultSecrets: step.VaultSecrets,
	}
}

func stepPlanFingerprint(step protocol.JobStepPlanItem) string {
	executable := executableStepFromPlan(step)
	// A step's position is not part of its executable identity. Keeping it out
	// lets unchanged steps retain history when an earlier step is inserted or
	// removed. The complete job fingerprint remains position-sensitive.
	executable.Index = 0
	encoded, _ := json.Marshal(executable)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func stepEventFingerprint(step protocol.JobStepPlanItem) string {
	executable := executableStepFromPlan(step)
	// Step events omit environment and Vault configuration to avoid carrying
	// sensitive data. Candidate and target plans are compared separately.
	executable.Env = nil
	executable.VaultConnection = ""
	executable.VaultSecrets = nil
	encoded, _ := json.Marshal(executable)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func estimateSteps(plan []protocol.JobStepPlanItem, exactMatches, fallbackMatches []protocol.JobExecution, eventsByJob map[string][]protocol.JobExecutionEvent) map[int]int64 {
	wanted := make(map[int]protocol.JobStepPlanItem, len(plan))
	for _, step := range plan {
		wanted[step.Index] = step
	}
	exactSamples := collectStepSamples(plan, exactMatches, eventsByJob)
	fallbackSamples := collectStepSamples(plan, fallbackMatches, eventsByJob)
	out := make(map[int]int64)
	for index := range wanted {
		durations := exactSamples[index]
		if len(durations) == 0 {
			durations = fallbackSamples[index]
		}
		if value := median(durations); value > 0 {
			out[index] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func collectStepSamples(wanted []protocol.JobStepPlanItem, matches []protocol.JobExecution, eventsByJob map[string][]protocol.JobExecutionEvent) map[int][]int64 {
	samples := make(map[int][]int64, len(wanted))
	for _, match := range matches {
		candidateSteps := make(map[int]protocol.JobStepPlanItem, len(match.StepPlan))
		for _, step := range match.StepPlan {
			candidateSteps[step.Index] = step
		}
		targetIndices := alignStepPlans(match.StepPlan, wanted)
		for _, event := range eventsByJob[match.ID] {
			if event.Type != protocol.JobExecutionEventTypeStepFinished || event.Step == nil || event.DurationMS <= 0 || (event.ExitCode != nil && *event.ExitCode != 0) || strings.TrimSpace(event.Error) != "" {
				continue
			}
			candidateStep, candidateOK := candidateSteps[event.Step.Index]
			targetIndex, aligned := targetIndices[event.Step.Index]
			if !aligned || !candidateOK || len(samples[targetIndex]) >= maxSamples ||
				stepEventFingerprint(*event.Step) != stepEventFingerprint(candidateStep) {
				continue
			}
			samples[targetIndex] = append(samples[targetIndex], event.DurationMS)
		}
	}
	return samples
}

// alignStepPlans associates historical steps with the current plan by
// executable identity and order. A longest-common-subsequence alignment keeps
// unchanged steps attached across insertions and removals while refusing to
// reuse history across command or configuration changes.
func alignStepPlans(candidate, target []protocol.JobStepPlanItem) map[int]int {
	candidateFingerprints := make([]string, len(candidate))
	for i := range candidate {
		candidateFingerprints[i] = stepPlanFingerprint(candidate[i])
	}
	targetFingerprints := make([]string, len(target))
	for i := range target {
		targetFingerprints[i] = stepPlanFingerprint(target[i])
	}

	lengths := make([][]int, len(candidate)+1)
	for i := range lengths {
		lengths[i] = make([]int, len(target)+1)
	}
	for i := len(candidate) - 1; i >= 0; i-- {
		for j := len(target) - 1; j >= 0; j-- {
			if candidateFingerprints[i] == targetFingerprints[j] {
				lengths[i][j] = 1 + lengths[i+1][j+1]
			} else {
				lengths[i][j] = max(lengths[i+1][j], lengths[i][j+1])
			}
		}
	}

	aligned := make(map[int]int, lengths[0][0])
	for i, j := 0, 0; i < len(candidate) && j < len(target); {
		if candidateFingerprints[i] == targetFingerprints[j] {
			aligned[candidate[i].Index] = target[j].Index
			i++
			j++
		} else if lengths[i+1][j] > lengths[i][j+1] {
			i++
		} else {
			j++
		}
	}
	return aligned
}

func jobDurations(jobs []protocol.JobExecution) []int64 {
	out := make([]int64, 0, len(jobs))
	for _, job := range jobs {
		ms := job.FinishedUTC.Sub(job.StartedUTC).Milliseconds()
		if ms > 0 {
			out = append(out, ms)
		}
	}
	return out
}

func median(values []int64) int64 {
	if len(values) == 0 {
		return 0
	}
	copyValues := append([]int64(nil), values...)
	sort.Slice(copyValues, func(i, j int) bool { return copyValues[i] < copyValues[j] })
	middle := len(copyValues) / 2
	if len(copyValues)%2 == 1 {
		return copyValues[middle]
	}
	return copyValues[middle-1] + (copyValues[middle]-copyValues[middle-1])/2
}

func applyEstimate(job *protocol.JobExecution, estimate Estimate) {
	job.ExpectedDurationMS = estimate.ExpectedDurationMS
	if len(estimate.PhaseExpectedDuration) == 0 {
		job.PhaseExpectedDuration = nil
	} else {
		job.PhaseExpectedDuration = make(map[string]int64, len(estimate.PhaseExpectedDuration))
		for id, duration := range estimate.PhaseExpectedDuration {
			job.PhaseExpectedDuration[id] = duration
		}
	}
	if len(estimate.StepExpectedDuration) == 0 {
		job.StepExpectedDuration = nil
		return
	}
	job.StepExpectedDuration = make(map[int]int64, len(estimate.StepExpectedDuration))
	for index, duration := range estimate.StepExpectedDuration {
		job.StepExpectedDuration[index] = duration
	}
}

func (e *Estimator) cached(key string) (Estimate, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	estimate, ok := e.cache[key]
	return estimate, ok
}

func (e *Estimator) remember(key string, estimate Estimate) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, exists := e.cache[key]; exists {
		return
	}
	e.cache[key] = estimate
	e.order = append(e.order, key)
	if len(e.order) <= maxCache {
		return
	}
	delete(e.cache, e.order[0])
	e.order = e.order[1:]
}

func (e *Estimator) rememberFingerprint(id, fingerprint string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, exists := e.fingerprints[id]; exists {
		return
	}
	e.fingerprints[id] = fingerprint
	e.fingerprintOrder = append(e.fingerprintOrder, id)
	if len(e.fingerprintOrder) <= maxFingerprints {
		return
	}
	delete(e.fingerprints, e.fingerprintOrder[0])
	e.fingerprintOrder = e.fingerprintOrder[1:]
}
