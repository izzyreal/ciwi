package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/config"
	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/store"
	"gopkg.in/yaml.v3"
)

type projectInspectRequest struct {
	PipelineDBID  int64  `json:"pipeline_db_id"`
	PipelineJobID string `json:"pipeline_job_id,omitempty"`
	MatrixIndex   *int   `json:"matrix_index,omitempty"`
	DryRun        bool   `json:"dry_run,omitempty"`
	TestSecrets   bool   `json:"test_secrets,omitempty"`
	View          string `json:"view,omitempty"`
}

type projectInspectResponse struct {
	View    string `json:"view"`
	Title   string `json:"title,omitempty"`
	Content string `json:"content"`
}

func (s *stateStore) projectInspectHandler(w http.ResponseWriter, r *http.Request, projectID int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req projectInspectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if req.PipelineDBID <= 0 {
		http.Error(w, "pipeline_db_id is required", http.StatusBadRequest)
		return
	}
	if req.MatrixIndex != nil && strings.TrimSpace(req.PipelineJobID) == "" {
		http.Error(w, "matrix_index requires pipeline_job_id", http.StatusBadRequest)
		return
	}
	view := strings.ToLower(strings.TrimSpace(req.View))
	if view == "" {
		view = "raw_yaml"
	}
	if view != "raw_yaml" && view != "executor_script" && view != "secret_mappings" {
		http.Error(w, "view must be one of raw_yaml,executor_script,secret_mappings", http.StatusBadRequest)
		return
	}

	p, err := s.pipelineStore().GetPipelineByDBID(req.PipelineDBID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if p.ProjectID != projectID {
		http.Error(w, "pipeline does not belong to project", http.StatusBadRequest)
		return
	}

	if view == "raw_yaml" {
		content, title, err := renderInspectableRawYAML(p, req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, projectInspectResponse{
			View:    view,
			Title:   title,
			Content: content,
		})
		return
	}

	selection := (*protocol.RunPipelineSelectionRequest)(nil)
	if strings.TrimSpace(req.PipelineJobID) != "" || req.MatrixIndex != nil || req.DryRun {
		selection = &protocol.RunPipelineSelectionRequest{
			PipelineJobID: strings.TrimSpace(req.PipelineJobID),
			MatrixIndex:   req.MatrixIndex,
			DryRun:        req.DryRun,
		}
	}
	_, pending, err := s.preparePendingPipelineJobs(p, selection, enqueuePipelineOptions{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if view == "secret_mappings" {
		ctx := r.Context()
		if req.TestSecrets {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
		}
		content, title, err := s.renderInspectableSecretMappings(ctx, p, pending, req.TestSecrets)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, projectInspectResponse{
			View:    view,
			Title:   title,
			Content: content,
		})
		return
	}
	content := renderInspectableExecutorScript(pending)
	if strings.TrimSpace(content) == "" {
		http.Error(w, "no executable scripts after rendering", http.StatusBadRequest)
		return
	}
	title := "Pipeline " + strings.TrimSpace(p.PipelineID) + " script"
	if strings.TrimSpace(req.PipelineJobID) != "" {
		title = "Job " + strings.TrimSpace(req.PipelineJobID) + " script"
	}
	writeJSON(w, http.StatusOK, projectInspectResponse{
		View:    view,
		Title:   title,
		Content: content,
	})
}

func (s *stateStore) renderInspectableSecretMappings(ctx context.Context, p store.PersistedPipeline, pending []pendingJob, testSecrets bool) (string, string, error) {
	title := "Pipeline " + strings.TrimSpace(p.PipelineID) + " secret mappings"
	lines := make([]string, 0, 64)
	if testSecrets {
		lines = append(lines, "mode: live Vault secret test")
	} else {
		lines = append(lines, "mode: mapping inspection only")
	}
	lines = append(lines, "pipeline: "+strings.TrimSpace(p.PipelineID), "")

	type secretCacheKey struct {
		connID int64
		spec   string
	}
	connByName := map[string]protocol.VaultConnection{}
	secretTestCache := map[secretCacheKey]string{}
	totalMappings := 0

	for _, spec := range pending {
		jobLabel := strings.TrimSpace(spec.pipelineJobID)
		if matrix := strings.TrimSpace(spec.metadata["matrix_name"]); matrix != "" {
			jobLabel += " / " + matrix
		}
		if jobLabel == "" {
			jobLabel = "<job>"
		}
		jobHeaderAdded := false
		for stepIdx, step := range spec.stepPlan {
			hasVaultConfig := strings.TrimSpace(step.VaultConnection) != "" || len(step.VaultSecrets) > 0
			referenced := map[string]struct{}{}
			for _, rawVal := range step.Env {
				matches := secretPlaceholderRE.FindAllStringSubmatch(rawVal, -1)
				for _, m := range matches {
					if len(m) < 2 {
						continue
					}
					name := strings.TrimSpace(m[1])
					if name != "" {
						referenced[name] = struct{}{}
					}
				}
			}
			if !hasVaultConfig && len(referenced) == 0 {
				continue
			}
			if !jobHeaderAdded {
				lines = append(lines, "job: "+jobLabel)
				jobHeaderAdded = true
			}
			stepName := strings.TrimSpace(step.Name)
			if stepName == "" {
				stepName = fmt.Sprintf("step %d", stepIdx+1)
			}
			lines = append(lines, fmt.Sprintf("  step: %s", stepName))
			connName := strings.TrimSpace(step.VaultConnection)
			if connName == "" {
				lines = append(lines, "    vault_connection: <missing>")
			} else {
				lines = append(lines, "    vault_connection: "+connName)
			}
			secretByName := map[string]protocol.ProjectSecretSpec{}
			for _, sec := range step.VaultSecrets {
				name := strings.TrimSpace(sec.Name)
				if name == "" {
					continue
				}
				secretByName[name] = sec
			}

			refNames := make([]string, 0, len(referenced))
			for name := range referenced {
				refNames = append(refNames, name)
			}
			sort.Strings(refNames)
			if len(refNames) == 0 && len(secretByName) > 0 {
				declared := make([]string, 0, len(secretByName))
				for name := range secretByName {
					declared = append(declared, name)
				}
				sort.Strings(declared)
				lines = append(lines, "    referenced_placeholders: (none)")
				lines = append(lines, "    declared_secrets: "+strings.Join(declared, ", "))
			}

			var conn protocol.VaultConnection
			connResolved := false
			if testSecrets && connName != "" {
				if cached, ok := connByName[connName]; ok {
					conn = cached
					connResolved = true
				} else {
					c, err := s.vaultStore().GetVaultConnectionByName(connName)
					if err != nil {
						lines = append(lines, "    test_connection: failed ("+err.Error()+")")
					} else {
						conn = c
						connByName[connName] = c
						connResolved = true
						lines = append(lines, "    test_connection: ok")
					}
				}
			}

			for _, secretName := range refNames {
				totalMappings++
				sec, ok := secretByName[secretName]
				if !ok {
					lines = append(lines, "    - "+secretName+": missing in step.vault.secrets")
					continue
				}
				mount := strings.TrimSpace(sec.Mount)
				path := strings.TrimSpace(sec.Path)
				key := strings.TrimSpace(sec.Key)
				kvVer := sec.KVVersion
				specLine := fmt.Sprintf("    - %s: mount=%s path=%s key=%s kv_version=%d", secretName, mount, path, key, kvVer)
				if !testSecrets {
					lines = append(lines, specLine)
					continue
				}
				if !connResolved {
					lines = append(lines, specLine+" | test=skipped (no connection)")
					continue
				}
				cacheKey := secretCacheKey{
					connID: conn.ID,
					spec:   fmt.Sprintf("%s|%s|%s|%d", mount, path, key, kvVer),
				}
				testResult, ok := secretTestCache[cacheKey]
				if !ok {
					_, err := s.readVaultSecret(ctx, conn, sec)
					if err != nil {
						testResult = "error: " + err.Error()
					} else {
						testResult = "ok"
					}
					secretTestCache[cacheKey] = testResult
				}
				lines = append(lines, specLine+" | test="+testResult)
			}
		}
	}

	if totalMappings == 0 {
		lines = append(lines, "no secret mappings found")
	}
	return strings.Join(lines, "\n"), title, nil
}

func renderInspectableRawYAML(p store.PersistedPipeline, req projectInspectRequest) (content string, title string, err error) {
	if strings.TrimSpace(req.PipelineJobID) == "" {
		pipe := persistedPipelineToConfigPipeline(p)
		data, mErr := yaml.Marshal(pipe)
		if mErr != nil {
			return "", "", fmt.Errorf("marshal pipeline yaml: %w", mErr)
		}
		return strings.TrimSpace(string(data)), "Pipeline " + strings.TrimSpace(p.PipelineID) + " YAML", nil
	}
	jobID := strings.TrimSpace(req.PipelineJobID)
	job, ok := findPersistedPipelineJobByID(p, jobID)
	if !ok {
		return "", "", fmt.Errorf("pipeline job %q not found", jobID)
	}
	spec := persistedPipelineJobToConfigJob(job)
	if req.MatrixIndex != nil {
		idx := *req.MatrixIndex
		if idx < 0 || idx >= len(spec.Matrix.Include) {
			return "", "", fmt.Errorf("matrix index %d out of range", idx)
		}
		spec.Matrix.Include = []map[string]string{cloneMap(spec.Matrix.Include[idx])}
	}
	data, mErr := yaml.Marshal(spec)
	if mErr != nil {
		return "", "", fmt.Errorf("marshal job yaml: %w", mErr)
	}
	return strings.TrimSpace(string(data)), "Job " + jobID + " YAML", nil
}

func persistedPipelineToConfigPipeline(p store.PersistedPipeline) config.Pipeline {
	out := config.Pipeline{
		ID:        p.PipelineID,
		Trigger:   p.Trigger,
		DependsOn: append([]string(nil), p.DependsOn...),
		Jobs:      make([]config.PipelineJobSpec, 0, len(p.Jobs)),
	}
	if strings.TrimSpace(p.SourceRepo) != "" || strings.TrimSpace(p.SourceRef) != "" {
		out.VCSSource = &config.Source{
			Repo: strings.TrimSpace(p.SourceRepo),
			Ref:  strings.TrimSpace(p.SourceRef),
		}
	}
	if v := p.Versioning; strings.TrimSpace(v.File) != "" || strings.TrimSpace(v.TagPrefix) != "" || strings.TrimSpace(v.AutoBump) != "" {
		vCopy := v
		out.Versioning = &vCopy
	}
	for _, j := range p.SortedJobs() {
		out.Jobs = append(out.Jobs, persistedPipelineJobToConfigJob(j))
	}
	return out
}

func persistedPipelineJobToConfigJob(j store.PersistedPipelineJob) config.PipelineJobSpec {
	spec := config.PipelineJobSpec{
		ID:             strings.TrimSpace(j.ID),
		Needs:          append([]string(nil), j.Needs...),
		RunsOn:         cloneMap(j.RunsOn),
		TimeoutSeconds: j.TimeoutSeconds,
		Artifacts:      append([]string(nil), j.Artifacts...),
		Caches:         append([]config.PipelineJobCacheSpec(nil), j.Caches...),
		Steps:          clonePipelineJobSteps(j.Steps),
	}
	if len(j.MatrixInclude) > 0 {
		spec.Matrix.Include = make([]map[string]string, 0, len(j.MatrixInclude))
		for _, row := range j.MatrixInclude {
			spec.Matrix.Include = append(spec.Matrix.Include, cloneMap(row))
		}
	}
	if len(j.RequiresTools) > 0 || len(j.RequiresContainerTools) > 0 {
		spec.Requires = config.PipelineJobRequirements{
			Tools: cloneMap(j.RequiresTools),
			Container: config.PipelineJobContainerRequirements{
				Tools: cloneMap(j.RequiresContainerTools),
			},
		}
	}
	return spec
}

func clonePipelineJobSteps(in []config.PipelineJobStep) []config.PipelineJobStep {
	if len(in) == 0 {
		return nil
	}
	out := make([]config.PipelineJobStep, 0, len(in))
	for _, step := range in {
		item := step
		item.Env = cloneMap(step.Env)
		if step.Vault != nil {
			v := *step.Vault
			if len(step.Vault.Secrets) > 0 {
				v.Secrets = append([]config.StepVaultSecretRef(nil), step.Vault.Secrets...)
			}
			item.Vault = &v
		}
		out = append(out, item)
	}
	return out
}

func findPersistedPipelineJobByID(p store.PersistedPipeline, jobID string) (store.PersistedPipelineJob, bool) {
	jobID = strings.TrimSpace(jobID)
	for _, j := range p.SortedJobs() {
		if strings.TrimSpace(j.ID) == jobID {
			return j, true
		}
	}
	return store.PersistedPipelineJob{}, false
}

func renderInspectableExecutorScript(pending []pendingJob) string {
	if len(pending) == 0 {
		return ""
	}
	if len(pending) == 1 {
		return strings.TrimSpace(pending[0].script)
	}
	parts := make([]string, 0, len(pending))
	for _, spec := range pending {
		name := strings.TrimSpace(spec.pipelineJobID)
		if matrix := strings.TrimSpace(spec.metadata["matrix_name"]); matrix != "" {
			name += " / " + matrix
		}
		header := "# " + strings.TrimSpace(name)
		body := strings.TrimSpace(spec.script)
		if body == "" {
			body = "# <empty>"
		}
		parts = append(parts, header+"\n"+body)
	}
	return strings.Join(parts, "\n\n---\n\n")
}
