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
	ListJobExecutionEventsForJobs(jobIDs []string, eventType string) (map[string][]protocol.JobExecutionEvent, error)
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
	matches := e.comparableSuccessfulJobs(*job, jobs)
	estimate := Estimate{ExpectedDurationMS: median(jobDurations(matches))}
	if len(matches) > 0 {
		ids := make([]string, len(matches))
		for i := range matches {
			ids[i] = matches[i].ID
		}
		eventsByJob, err := e.store.ListJobExecutionEventsForJobs(ids, "")
		if err != nil {
			return err
		}
		if len(job.StepPlan) > 0 {
			estimate.StepExpectedDuration = estimateSteps(job.StepPlan, matches, eventsByJob)
		}
		estimate.PhaseExpectedDuration = estimatePhases(protocol.BuildJobExecutionTimeline(*job), matches, eventsByJob)
	}
	e.remember(cacheKey, estimate)
	applyEstimate(job, estimate)
	return nil
}

func estimatePhases(timeline []protocol.JobExecutionTimelineItem, matches []protocol.JobExecution, eventsByJob map[string][]protocol.JobExecutionEvent) map[string]int64 {
	wanted := map[string]struct{}{}
	for _, item := range timeline {
		if item.Kind == "phase" && strings.TrimSpace(item.ID) != "" {
			wanted[item.ID] = struct{}{}
		}
	}
	samples := make(map[string][]int64, len(wanted))
	for _, match := range matches {
		for _, event := range eventsByJob[match.ID] {
			if event.Type != protocol.JobExecutionEventTypePhaseFinished || event.Phase == nil || event.DurationMS <= 0 ||
				(event.ExitCode != nil && *event.ExitCode != 0) || strings.TrimSpace(event.Error) != "" {
				continue
			}
			id := strings.TrimSpace(event.Phase.ID)
			if _, ok := wanted[id]; ok {
				samples[id] = append(samples[id], event.DurationMS)
			}
		}
	}
	out := make(map[string]int64)
	for id, durations := range samples {
		if value := median(durations); value > 0 {
			out[id] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
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

func (e *Estimator) provisionalJobKey(job protocol.JobExecution) string {
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
		strings.TrimSpace(m["dry_run"]),
		string(requiredCapabilities),
		e.jobPlanFingerprint(job),
	}
	return strings.Join(parts, "\x1f")
}

type executableStep struct {
	Index          int               `json:"index"`
	Script         string            `json:"script"`
	Kind           string            `json:"kind"`
	Env            map[string]string `json:"env,omitempty"`
	TestName       string            `json:"test_name,omitempty"`
	TestFormat     string            `json:"test_format,omitempty"`
	TestReport     string            `json:"test_report,omitempty"`
	CoverageFormat string            `json:"coverage_format,omitempty"`
	CoverageReport string            `json:"coverage_report,omitempty"`
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
	payload := struct {
		Script string           `json:"script"`
		Steps  []executableStep `json:"steps"`
	}{Script: job.Script, Steps: steps}
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
	}
}

func stepFingerprint(step protocol.JobStepPlanItem) string {
	executable := executableStepFromPlan(step)
	// Step events intentionally do not carry Env, because step environment can
	// contain sensitive values. Job-level matching still includes Env; the
	// per-step fingerprint must use the common subset present in both StepPlan
	// and step.finished events.
	executable.Env = nil
	encoded, _ := json.Marshal(executable)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func estimateSteps(plan []protocol.JobStepPlanItem, matches []protocol.JobExecution, eventsByJob map[string][]protocol.JobExecutionEvent) map[int]int64 {
	wanted := make(map[int]string, len(plan))
	for _, step := range plan {
		wanted[step.Index] = stepFingerprint(step)
	}
	samples := make(map[int][]int64, len(plan))
	for _, match := range matches {
		for _, event := range eventsByJob[match.ID] {
			if event.Type != protocol.JobExecutionEventTypeStepFinished || event.Step == nil || event.DurationMS <= 0 || (event.ExitCode != nil && *event.ExitCode != 0) || strings.TrimSpace(event.Error) != "" {
				continue
			}
			fingerprint, ok := wanted[event.Step.Index]
			if !ok || stepFingerprint(*event.Step) != fingerprint {
				continue
			}
			samples[event.Step.Index] = append(samples[event.Step.Index], event.DurationMS)
		}
	}
	out := make(map[int]int64)
	for index, durations := range samples {
		if value := median(durations); value > 0 {
			out[index] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
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
