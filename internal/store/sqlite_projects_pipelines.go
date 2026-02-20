package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/izzyreal/ciwi/internal/config"
	"github.com/izzyreal/ciwi/internal/protocol"
)

func parseRFC3339OrZero(raw string) time.Time {
	value := raw
	if value == "" {
		return time.Time{}
	}
	if ts, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return ts
	}
	return time.Time{}
}

func upsertProject(tx *sql.Tx, name, configPath, repoURL, repoRef, configFile, loadedCommit, now string) (int64, error) {
	res, err := tx.Exec(`
		UPDATE projects
		SET config_path = ?, repo_url = ?, repo_ref = ?, config_file = ?, loaded_commit = ?, updated_utc = ?
		WHERE name = ?
	`, configPath, repoURL, repoRef, configFile, loadedCommit, now, name)
	if err != nil {
		return 0, fmt.Errorf("update project: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("project rows affected: %w", err)
	}
	if rows == 0 {
		if _, err := tx.Exec(`
			INSERT INTO projects (name, config_path, repo_url, repo_ref, config_file, loaded_commit, created_utc, updated_utc)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, name, configPath, repoURL, repoRef, configFile, loadedCommit, now, now); err != nil {
			return 0, fmt.Errorf("insert project: %w", err)
		}
	}

	var id int64
	if err := tx.QueryRow(`SELECT id FROM projects WHERE name = ?`, name).Scan(&id); err != nil {
		return 0, fmt.Errorf("fetch project id: %w", err)
	}
	return id, nil
}

func upsertPipeline(tx *sql.Tx, projectID int64, p config.Pipeline, now string) (int64, error) {
	dependsOnJSON, _ := json.Marshal(p.DependsOn)
	versioningJSON, _ := json.Marshal(p.Versioning)
	if _, err := tx.Exec(`
		INSERT INTO pipelines (project_id, pipeline_id, trigger_mode, depends_on_json, source_repo, source_ref, versioning_json, created_utc, updated_utc)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(project_id, pipeline_id)
		DO UPDATE SET trigger_mode=excluded.trigger_mode, depends_on_json=excluded.depends_on_json, source_repo=excluded.source_repo, source_ref=excluded.source_ref, versioning_json=excluded.versioning_json, updated_utc=excluded.updated_utc
	`, projectID, p.ID, p.Trigger, string(dependsOnJSON), p.Source.Repo, p.Source.Ref, string(versioningJSON), now, now); err != nil {
		return 0, fmt.Errorf("upsert pipeline: %w", err)
	}

	var id int64
	if err := tx.QueryRow(`SELECT id FROM pipelines WHERE project_id = ? AND pipeline_id = ?`, projectID, p.ID).Scan(&id); err != nil {
		return 0, fmt.Errorf("fetch pipeline id: %w", err)
	}
	return id, nil
}

func (s *Store) ListProjects() ([]protocol.ProjectSummary, error) {
	rows, err := s.db.Query(`
		SELECT p.id, p.name, p.config_path, p.repo_url, p.repo_ref, p.config_file, p.loaded_commit, p.updated_utc, pl.id, pl.pipeline_id, pl.trigger_mode, pl.depends_on_json, pl.source_repo, pl.source_ref, pl.versioning_json
		FROM projects p
		LEFT JOIN pipelines pl ON pl.project_id = p.id
		ORDER BY p.name, pl.pipeline_id
	`)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()

	projectsByID := map[int64]*protocol.ProjectSummary{}
	order := make([]int64, 0)

	for rows.Next() {
		var projectID int64
		var projectName, configPath string
		var repoURL, repoRef, configFile, loadedCommit sql.NullString
		var updatedUTC string
		var pipelineID sql.NullInt64
		var pipelineName, trigger, dependsOnJSON, sourceRepo, sourceRef, versioningJSON sql.NullString

		if err := rows.Scan(&projectID, &projectName, &configPath, &repoURL, &repoRef, &configFile, &loadedCommit, &updatedUTC, &pipelineID, &pipelineName, &trigger, &dependsOnJSON, &sourceRepo, &sourceRef, &versioningJSON); err != nil {
			return nil, fmt.Errorf("scan project row: %w", err)
		}

		project, ok := projectsByID[projectID]
		if !ok {
			project = &protocol.ProjectSummary{
				ID:           projectID,
				Name:         projectName,
				ConfigPath:   configPath,
				RepoURL:      repoURL.String,
				RepoRef:      repoRef.String,
				ConfigFile:   configFile.String,
				LoadedCommit: loadedCommit.String,
				UpdatedUTC:   parseRFC3339OrZero(updatedUTC),
			}
			projectsByID[projectID] = project
			order = append(order, projectID)
		}

		if pipelineID.Valid {
			dependsOn := []string{}
			_ = json.Unmarshal([]byte(dependsOnJSON.String), &dependsOn)
			project.Pipelines = append(project.Pipelines, protocol.PipelineSummary{
				ID:         pipelineID.Int64,
				PipelineID: pipelineName.String,
				Trigger:    trigger.String,
				DependsOn:  dependsOn,
				SourceRepo: sourceRepo.String,
				SourceRef:  sourceRef.String,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate project rows: %w", err)
	}

	projects := make([]protocol.ProjectSummary, 0, len(order))
	for _, id := range order {
		project := projectsByID[id]
		pipelineSupports := map[string]bool{}
		pipelineIDByName := map[string]int64{}
		for i := range project.Pipelines {
			supports, err := s.pipelineSupportsDryRun(project.Pipelines[i].ID)
			if err != nil {
				return nil, err
			}
			project.Pipelines[i].SupportsDryRun = supports
			pipelineSupports[project.Pipelines[i].PipelineID] = supports
			pipelineIDByName[project.Pipelines[i].PipelineID] = project.Pipelines[i].ID
		}
		chains, err := s.listPipelineChainsByProjectID(id)
		if err != nil {
			return nil, err
		}
		project.PipelineChains = make([]protocol.PipelineChainSummary, 0, len(chains))
		for _, ch := range chains {
			supports := false
			var versionPipelineID int64
			for _, pid := range ch.Pipelines {
				if versionPipelineID == 0 {
					versionPipelineID = pipelineIDByName[pid]
				}
				if pipelineSupports[pid] {
					supports = true
				}
			}
			project.PipelineChains = append(project.PipelineChains, protocol.PipelineChainSummary{
				ID:                ch.DBID,
				ChainID:           ch.ChainID,
				Pipelines:         append([]string(nil), ch.Pipelines...),
				SupportsDryRun:    supports,
				VersionPipelineID: versionPipelineID,
			})
		}
		projects = append(projects, *project)
	}
	return projects, nil
}

func (s *Store) GetProjectByID(id int64) (protocol.ProjectSummary, error) {
	var p protocol.ProjectSummary
	row := s.db.QueryRow(`
		SELECT id, name, config_path, repo_url, repo_ref, config_file, loaded_commit, updated_utc
		FROM projects
		WHERE id = ?
	`, id)
	var updatedUTC string
	if err := row.Scan(&p.ID, &p.Name, &p.ConfigPath, &p.RepoURL, &p.RepoRef, &p.ConfigFile, &p.LoadedCommit, &updatedUTC); err != nil {
		if err == sql.ErrNoRows {
			return protocol.ProjectSummary{}, fmt.Errorf("project not found")
		}
		return protocol.ProjectSummary{}, fmt.Errorf("get project: %w", err)
	}
	p.UpdatedUTC = parseRFC3339OrZero(updatedUTC)
	return p, nil
}

func (s *Store) GetProjectByName(name string) (protocol.ProjectSummary, error) {
	var p protocol.ProjectSummary
	row := s.db.QueryRow(`
		SELECT id, name, config_path, repo_url, repo_ref, config_file, loaded_commit, updated_utc
		FROM projects
		WHERE name = ?
	`, name)
	var updatedUTC string
	if err := row.Scan(&p.ID, &p.Name, &p.ConfigPath, &p.RepoURL, &p.RepoRef, &p.ConfigFile, &p.LoadedCommit, &updatedUTC); err != nil {
		if err == sql.ErrNoRows {
			return protocol.ProjectSummary{}, fmt.Errorf("project not found")
		}
		return protocol.ProjectSummary{}, fmt.Errorf("get project: %w", err)
	}
	p.UpdatedUTC = parseRFC3339OrZero(updatedUTC)
	return p, nil
}

func (s *Store) GetProjectDetail(id int64) (protocol.ProjectDetail, error) {
	project, err := s.GetProjectByID(id)
	if err != nil {
		return protocol.ProjectDetail{}, err
	}

	rows, err := s.db.Query(`
		SELECT id, pipeline_id, trigger_mode, depends_on_json, source_repo, source_ref, versioning_json
		FROM pipelines
		WHERE project_id = ?
		ORDER BY pipeline_id
	`, id)
	if err != nil {
		return protocol.ProjectDetail{}, fmt.Errorf("list pipelines: %w", err)
	}

	detail := protocol.ProjectDetail{
		ID:           project.ID,
		Name:         project.Name,
		RepoURL:      project.RepoURL,
		RepoRef:      project.RepoRef,
		ConfigFile:   project.ConfigFile,
		LoadedCommit: project.LoadedCommit,
		UpdatedUTC:   project.UpdatedUTC,
	}

	for rows.Next() {
		var p protocol.PipelineDetail
		var dependsOnJSON, versioningJSON string
		if err := rows.Scan(&p.ID, &p.PipelineID, &p.Trigger, &dependsOnJSON, &p.SourceRepo, &p.SourceRef, &versioningJSON); err != nil {
			return protocol.ProjectDetail{}, fmt.Errorf("scan pipeline: %w", err)
		}
		_ = json.Unmarshal([]byte(dependsOnJSON), &p.DependsOn)
		_ = json.Unmarshal([]byte(versioningJSON), &p.Versioning)
		detail.Pipelines = append(detail.Pipelines, p)
	}
	if err := rows.Err(); err != nil {
		return protocol.ProjectDetail{}, fmt.Errorf("iterate pipelines: %w", err)
	}
	if err := rows.Close(); err != nil {
		return protocol.ProjectDetail{}, fmt.Errorf("close pipelines rows: %w", err)
	}

	for i := range detail.Pipelines {
		persistedJobs, err := s.listPipelineJobs(detail.Pipelines[i].ID)
		if err != nil {
			return protocol.ProjectDetail{}, err
		}
		detail.Pipelines[i].Jobs = pipelineJobDetailsFromPersisted(persistedJobs)
	}
	chains, err := s.listPipelineChainsByProjectID(id)
	if err != nil {
		return protocol.ProjectDetail{}, err
	}
	pipelineSupports := map[string]bool{}
	pipelineIDByName := map[string]int64{}
	for _, p := range detail.Pipelines {
		supports := false
		for _, j := range p.Jobs {
			for _, st := range j.Steps {
				if st.SkipDryRun {
					supports = true
					break
				}
			}
			if supports {
				break
			}
		}
		pipelineSupports[p.PipelineID] = supports
		pipelineIDByName[p.PipelineID] = p.ID
	}
	detail.PipelineChains = make([]protocol.PipelineChainSummary, 0, len(chains))
	for _, ch := range chains {
		supports := false
		var versionPipelineID int64
		for _, pid := range ch.Pipelines {
			if versionPipelineID == 0 {
				versionPipelineID = pipelineIDByName[pid]
			}
			if pipelineSupports[pid] {
				supports = true
			}
		}
		detail.PipelineChains = append(detail.PipelineChains, protocol.PipelineChainSummary{
			ID:                ch.DBID,
			ChainID:           ch.ChainID,
			Pipelines:         append([]string(nil), ch.Pipelines...),
			SupportsDryRun:    supports,
			VersionPipelineID: versionPipelineID,
		})
	}

	return detail, nil
}

func (s *Store) SetProjectLoadedCommit(projectID int64, loadedCommit string) error {
	_, err := s.db.Exec(`
		UPDATE projects
		SET loaded_commit = ?, updated_utc = ?
		WHERE id = ?
	`, loadedCommit, time.Now().UTC().Format(time.RFC3339Nano), projectID)
	if err != nil {
		return fmt.Errorf("set project loaded commit: %w", err)
	}
	return nil
}

func (s *Store) pipelineSupportsDryRun(pipelineDBID int64) (bool, error) {
	jobs, err := s.listPipelineJobs(pipelineDBID)
	if err != nil {
		return false, err
	}
	for _, job := range jobs {
		for _, step := range job.Steps {
			if step.SkipDryRun {
				return true, nil
			}
		}
	}
	return false, nil
}

func (s *Store) listPipelineChainsByProjectID(projectID int64) ([]PersistedPipelineChain, error) {
	rows, err := s.db.Query(`
		SELECT pc.id, pc.project_id, p.name, pc.chain_id, pc.pipelines_json
		FROM pipeline_chains pc
		JOIN projects p ON p.id = pc.project_id
		WHERE pc.project_id = ?
		ORDER BY pc.chain_id
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list pipeline chains: %w", err)
	}
	defer rows.Close()
	out := make([]PersistedPipelineChain, 0)
	for rows.Next() {
		var ch PersistedPipelineChain
		var pipelinesJSON string
		if err := rows.Scan(&ch.DBID, &ch.ProjectID, &ch.ProjectName, &ch.ChainID, &pipelinesJSON); err != nil {
			return nil, fmt.Errorf("scan pipeline chain: %w", err)
		}
		_ = json.Unmarshal([]byte(pipelinesJSON), &ch.Pipelines)
		out = append(out, ch)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pipeline chains: %w", err)
	}
	return out, nil
}

func pipelineJobDetailsFromPersisted(persistedJobs []PersistedPipelineJob) []protocol.PipelineJobDetail {
	out := make([]protocol.PipelineJobDetail, 0, len(persistedJobs))
	for _, j := range persistedJobs {
		d := protocol.PipelineJobDetail{
			ID:                   j.ID,
			Needs:                append([]string(nil), j.Needs...),
			TimeoutSeconds:       j.TimeoutSeconds,
			RunsOn:               cloneMap(j.RunsOn),
			RequiresTools:        cloneMap(j.RequiresTools),
			RequiresCapabilities: cloneMap(j.RequiresCaps),
			Artifacts:            append([]string(nil), j.Artifacts...),
			Caches:               cloneJobCachesFromConfig(j.Caches),
		}
		d.Steps = make([]protocol.PipelineStep, 0, len(j.Steps))
		for _, step := range j.Steps {
			if step.Test != nil {
				d.Steps = append(d.Steps, protocol.PipelineStep{
					Type:           "test",
					TestName:       step.Test.Name,
					TestCommand:    step.Test.Command,
					TestFormat:     step.Test.Format,
					TestReport:     step.Test.Report,
					CoverageFormat: step.Test.CoverageFormat,
					CoverageReport: step.Test.CoverageReport,
					SkipDryRun:     step.SkipDryRun,
					Env:            cloneMap(step.Env),
				})
				continue
			}
			d.Steps = append(d.Steps, protocol.PipelineStep{
				Type:       "run",
				Run:        step.Run,
				SkipDryRun: step.SkipDryRun,
				Env:        cloneMap(step.Env),
			})
		}
		for idx, vars := range j.MatrixInclude {
			v := cloneMap(vars)
			d.MatrixIncludes = append(d.MatrixIncludes, protocol.MatrixInclude{
				Index: idx,
				Name:  v["name"],
				Vars:  v,
			})
		}
		out = append(out, d)
	}
	return out
}

func (s *Store) GetPipelineByDBID(id int64) (PersistedPipeline, error) {
	var p PersistedPipeline
	row := s.db.QueryRow(`
		SELECT pl.id, pl.project_id, p.name, pl.pipeline_id, pl.trigger_mode, pl.depends_on_json, pl.source_repo, pl.source_ref, pl.versioning_json
		FROM pipelines pl
		JOIN projects p ON p.id = pl.project_id
		WHERE pl.id = ?
	`, id)
	var dependsOnJSON, versioningJSON string
	if err := row.Scan(&p.DBID, &p.ProjectID, &p.ProjectName, &p.PipelineID, &p.Trigger, &dependsOnJSON, &p.SourceRepo, &p.SourceRef, &versioningJSON); err != nil {
		if err == sql.ErrNoRows {
			return p, fmt.Errorf("pipeline not found")
		}
		return p, fmt.Errorf("get pipeline: %w", err)
	}
	_ = json.Unmarshal([]byte(dependsOnJSON), &p.DependsOn)
	_ = json.Unmarshal([]byte(versioningJSON), &p.Versioning)

	jobs, err := s.listPipelineJobs(p.DBID)
	if err != nil {
		return p, err
	}
	p.Jobs = jobs
	return p, nil
}

func (s *Store) GetPipelineByProjectAndID(projectName, pipelineID string) (PersistedPipeline, error) {
	var id int64
	row := s.db.QueryRow(`
		SELECT pl.id
		FROM pipelines pl
		JOIN projects p ON p.id = pl.project_id
		WHERE p.name = ? AND pl.pipeline_id = ?
	`, projectName, pipelineID)
	if err := row.Scan(&id); err != nil {
		if err == sql.ErrNoRows {
			return PersistedPipeline{}, fmt.Errorf("pipeline not found")
		}
		return PersistedPipeline{}, fmt.Errorf("find pipeline: %w", err)
	}
	return s.GetPipelineByDBID(id)
}

func (s *Store) GetPipelineChainByDBID(id int64) (PersistedPipelineChain, error) {
	var ch PersistedPipelineChain
	row := s.db.QueryRow(`
		SELECT pc.id, pc.project_id, p.name, pc.chain_id, pc.pipelines_json
		FROM pipeline_chains pc
		JOIN projects p ON p.id = pc.project_id
		WHERE pc.id = ?
	`, id)
	var pipelinesJSON string
	if err := row.Scan(&ch.DBID, &ch.ProjectID, &ch.ProjectName, &ch.ChainID, &pipelinesJSON); err != nil {
		if err == sql.ErrNoRows {
			return ch, fmt.Errorf("pipeline chain not found")
		}
		return ch, fmt.Errorf("get pipeline chain: %w", err)
	}
	_ = json.Unmarshal([]byte(pipelinesJSON), &ch.Pipelines)
	return ch, nil
}

func (s *Store) listPipelineJobs(pipelineDBID int64) ([]PersistedPipelineJob, error) {
	rows, err := s.db.Query(`
		SELECT job_id, position, needs_json, runs_on_json, requires_tools_json, requires_capabilities_json, timeout_seconds, artifacts_json, caches_json, matrix_json, steps_json
		FROM pipeline_jobs
		WHERE pipeline_id = ?
		ORDER BY position
	`, pipelineDBID)
	if err != nil {
		return nil, fmt.Errorf("list pipeline jobs: %w", err)
	}
	defer rows.Close()

	jobs := []PersistedPipelineJob{}
	for rows.Next() {
		var j PersistedPipelineJob
		var needsJSON, runsOnJSON, requiresToolsJSON, requiresCapsJSON, artifactsJSON, cachesJSON, matrixJSON, stepsJSON string
		if err := rows.Scan(&j.ID, &j.Position, &needsJSON, &runsOnJSON, &requiresToolsJSON, &requiresCapsJSON, &j.TimeoutSeconds, &artifactsJSON, &cachesJSON, &matrixJSON, &stepsJSON); err != nil {
			return nil, fmt.Errorf("scan pipeline job: %w", err)
		}
		_ = json.Unmarshal([]byte(needsJSON), &j.Needs)
		_ = json.Unmarshal([]byte(runsOnJSON), &j.RunsOn)
		_ = json.Unmarshal([]byte(requiresToolsJSON), &j.RequiresTools)
		_ = json.Unmarshal([]byte(requiresCapsJSON), &j.RequiresCaps)
		_ = json.Unmarshal([]byte(artifactsJSON), &j.Artifacts)
		_ = json.Unmarshal([]byte(cachesJSON), &j.Caches)
		_ = json.Unmarshal([]byte(matrixJSON), &j.MatrixInclude)
		if err := json.Unmarshal([]byte(stepsJSON), &j.Steps); err != nil {
			// Backward compatibility for existing rows where steps_json is []string.
			var legacy []string
			if legacyErr := json.Unmarshal([]byte(stepsJSON), &legacy); legacyErr == nil {
				j.Steps = make([]config.PipelineJobStep, 0, len(legacy))
				for _, run := range legacy {
					j.Steps = append(j.Steps, config.PipelineJobStep{Run: run})
				}
			}
		}
		jobs = append(jobs, j)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pipeline jobs: %w", err)
	}
	return jobs, nil
}
