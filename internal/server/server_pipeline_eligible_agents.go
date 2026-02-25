package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/requirements"
	"github.com/izzyreal/ciwi/internal/store"
)

type eligibleAgentsResponse struct {
	EligibleAgentIDs []string `json:"eligible_agent_ids"`
	PendingJobs      int      `json:"pending_jobs"`
}

func (s *stateStore) pipelineEligibleAgentsHandler(w http.ResponseWriter, p store.PersistedPipeline, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var selection protocol.RunPipelineSelectionRequest
	if err := json.NewDecoder(r.Body).Decode(&selection); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	_, pending, err := s.preparePendingPipelineJobs(p, &selection, enqueuePipelineOptions{allowSelectionNeedsGap: true})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ids := s.eligibleAgentsForPendingJobs(pending)
	writeJSON(w, http.StatusOK, eligibleAgentsResponse{
		EligibleAgentIDs: ids,
		PendingJobs:      len(pending),
	})
}

func (s *stateStore) pipelineChainEligibleAgentsHandler(w http.ResponseWriter, ch store.PersistedPipelineChain, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var selection protocol.RunPipelineSelectionRequest
	if err := json.NewDecoder(r.Body).Decode(&selection); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	pending, err := s.preparePendingPipelineChainJobs(ch, &selection)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ids := s.eligibleAgentsForPendingJobs(pending)
	writeJSON(w, http.StatusOK, eligibleAgentsResponse{
		EligibleAgentIDs: ids,
		PendingJobs:      len(pending),
	})
}

func (s *stateStore) preparePendingPipelineChainJobs(ch store.PersistedPipelineChain, selection *protocol.RunPipelineSelectionRequest) ([]pendingJob, error) {
	if len(ch.Pipelines) == 0 {
		return nil, fmt.Errorf("pipeline chain has no pipelines")
	}
	pipelines := make([]store.PersistedPipeline, 0, len(ch.Pipelines))
	for _, pid := range ch.Pipelines {
		p, err := s.pipelineStore().GetPipelineByProjectAndID(ch.ProjectName, strings.TrimSpace(pid))
		if err != nil {
			return nil, fmt.Errorf("load pipeline %q in chain %q: %w", pid, ch.ChainID, err)
		}
		pipelines = append(pipelines, p)
	}
	firstDep, err := s.checkPipelineDependenciesWithReporter(pipelines[0], nil)
	if err != nil {
		return nil, err
	}
	overrideSourceRef := normalizeSourceRef(selection)
	overrideRepo := strings.TrimSpace(pipelines[0].SourceRepo)
	if overrideSourceRef != "" && overrideRepo == "" {
		return nil, fmt.Errorf("source_ref override requires first chain pipeline vcs_source.repo")
	}
	firstVersionPipeline := pipelines[0]
	if overrideSourceRef != "" && shouldApplySourceRefOverride(firstVersionPipeline.SourceRepo, overrideRepo) {
		firstVersionPipeline.SourceRef = overrideSourceRef
	}
	firstRun, err := resolvePipelineRunContextWithReporter(firstVersionPipeline, firstDep, nil)
	if err != nil {
		return nil, err
	}
	if firstRun.SourceRefResolved == "" && overrideSourceRef != "" && shouldApplySourceRefOverride(firstVersionPipeline.SourceRepo, overrideRepo) {
		resolved, err := resolveSourceRefFromRepo(strings.TrimSpace(firstVersionPipeline.SourceRepo), strings.TrimSpace(firstVersionPipeline.SourceRef))
		if err != nil {
			return nil, err
		}
		firstRun.SourceRefResolved = resolved
	}
	total := len(pipelines)
	chainPipelineSet := map[string]struct{}{}
	for _, p := range pipelines {
		chainPipelineSet[strings.TrimSpace(p.PipelineID)] = struct{}{}
	}
	out := make([]pendingJob, 0)
	for i, p := range pipelines {
		prevPipelineID := ""
		if i > 0 {
			prevPipelineID = strings.TrimSpace(pipelines[i-1].PipelineID)
		}
		chainDeps := deriveChainPipelineDependencies(p, chainPipelineSet, prevPipelineID)
		meta := map[string]string{
			"chain_run_id":            "eligible-preview",
			"pipeline_chain_id":       ch.ChainID,
			"pipeline_chain_index":    strconv.Itoa(i),
			"pipeline_chain_position": strconv.Itoa(i + 1),
			"pipeline_chain_total":    strconv.Itoa(total),
		}
		if len(chainDeps) > 0 {
			meta["chain_depends_on_pipelines"] = strings.Join(chainDeps, ",")
		}
		opts := enqueuePipelineOptions{
			metaPatch:              meta,
			blocked:                len(chainDeps) > 0,
			allowSelectionNeedsGap: true,
			sourceRefOverride:      overrideSourceRef,
			sourceRefOverrideRepo:  overrideRepo,
		}
		if i > 0 {
			opts.forcedDep = &pipelineDependencyContext{
				VersionRaw:        firstRun.VersionRaw,
				Version:           firstRun.Version,
				SourceRepo:        strings.TrimSpace(pipelines[0].SourceRepo),
				SourceRefRaw:      firstRun.SourceRefRaw,
				SourceRefResolved: firstRun.SourceRefResolved,
			}
		}
		_, pending, err := s.preparePendingPipelineJobs(p, selection, opts)
		if err != nil {
			return nil, err
		}
		out = append(out, pending...)
	}
	if selection != nil && len(out) == 0 {
		return nil, fmt.Errorf("selection matched no matrix entries")
	}
	return out, nil
}

func (s *stateStore) eligibleAgentsForPendingJobs(pending []pendingJob) []string {
	if len(pending) == 0 {
		return nil
	}
	s.mu.Lock()
	agents := make(map[string]agentState, len(s.agents))
	for id, a := range s.agents {
		agents[id] = a
	}
	s.mu.Unlock()

	eligible := make([]string, 0, len(agents))
	for agentID, agent := range agents {
		ok := true
		for _, job := range pending {
			if !agentMatchesRequiredCapabilities(agentID, agent, job.requiredCaps) {
				ok = false
				break
			}
		}
		if ok {
			eligible = append(eligible, agentID)
		}
	}
	sort.Strings(eligible)
	return eligible
}

func agentMatchesRequiredCapabilities(agentID string, agent agentState, required map[string]string) bool {
	if len(required) == 0 {
		return true
	}
	merged := cloneMap(agent.Capabilities)
	if merged == nil {
		merged = map[string]string{}
	}
	merged["os"] = strings.TrimSpace(agent.OS)
	merged["arch"] = strings.TrimSpace(agent.Arch)
	merged["agent_id"] = strings.TrimSpace(agentID)

	for key, requiredValue := range required {
		requiredValue = strings.TrimSpace(requiredValue)
		switch {
		case strings.HasPrefix(key, "requires.tool."):
			tool := strings.TrimPrefix(key, "requires.tool.")
			observed := strings.TrimSpace(merged["tool."+tool])
			if !requirements.ToolConstraintMatch(observed, requiredValue) {
				return false
			}
		case strings.HasPrefix(key, "requires.container.tool."):
			// Container tool checks happen on agent runtime probe; scheduler-side
			// eligibility follows existing lease behavior by not hard-filtering here.
			continue
		case key == "shell":
			if !requirements.ShellCapabilityMatch(merged, requiredValue) {
				return false
			}
		default:
			if strings.TrimSpace(merged[key]) != requiredValue {
				return false
			}
		}
	}
	return true
}
