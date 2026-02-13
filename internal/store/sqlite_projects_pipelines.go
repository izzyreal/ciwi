package store

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/izzyreal/ciwi/internal/config"
	"github.com/izzyreal/ciwi/internal/protocol"
)

func upsertProject(tx *sql.Tx, name, configPath, repoURL, repoRef, configFile, now string) (int64, error) {
	if _, err := tx.Exec(`
		INSERT INTO projects (name, config_path, repo_url, repo_ref, config_file, created_utc, updated_utc)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			config_path=excluded.config_path,
			repo_url=excluded.repo_url,
			repo_ref=excluded.repo_ref,
			config_file=excluded.config_file,
			updated_utc=excluded.updated_utc
	`, name, configPath, repoURL, repoRef, configFile, now, now); err != nil {
		return 0, fmt.Errorf("upsert project: %w", err)
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
		SELECT p.id, p.name, p.config_path, p.repo_url, p.repo_ref, p.config_file, pl.id, pl.pipeline_id, pl.trigger_mode, pl.depends_on_json, pl.source_repo, pl.source_ref, pl.versioning_json
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
		var repoURL, repoRef, configFile sql.NullString
		var pipelineID sql.NullInt64
		var pipelineName, trigger, dependsOnJSON, sourceRepo, sourceRef, versioningJSON sql.NullString

		if err := rows.Scan(&projectID, &projectName, &configPath, &repoURL, &repoRef, &configFile, &pipelineID, &pipelineName, &trigger, &dependsOnJSON, &sourceRepo, &sourceRef, &versioningJSON); err != nil {
			return nil, fmt.Errorf("scan project row: %w", err)
		}

		project, ok := projectsByID[projectID]
		if !ok {
			project = &protocol.ProjectSummary{
				ID:         projectID,
				Name:       projectName,
				ConfigPath: configPath,
				RepoURL:    repoURL.String,
				RepoRef:    repoRef.String,
				ConfigFile: configFile.String,
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
		projects = append(projects, *projectsByID[id])
	}
	return projects, nil
}

func (s *Store) GetProjectByID(id int64) (protocol.ProjectSummary, error) {
	var p protocol.ProjectSummary
	row := s.db.QueryRow(`
		SELECT id, name, config_path, repo_url, repo_ref, config_file
		FROM projects
		WHERE id = ?
	`, id)
	if err := row.Scan(&p.ID, &p.Name, &p.ConfigPath, &p.RepoURL, &p.RepoRef, &p.ConfigFile); err != nil {
		if err == sql.ErrNoRows {
			return protocol.ProjectSummary{}, fmt.Errorf("project not found")
		}
		return protocol.ProjectSummary{}, fmt.Errorf("get project: %w", err)
	}
	return p, nil
}

func (s *Store) GetProjectByName(name string) (protocol.ProjectSummary, error) {
	var p protocol.ProjectSummary
	row := s.db.QueryRow(`
		SELECT id, name, config_path, repo_url, repo_ref, config_file
		FROM projects
		WHERE name = ?
	`, name)
	if err := row.Scan(&p.ID, &p.Name, &p.ConfigPath, &p.RepoURL, &p.RepoRef, &p.ConfigFile); err != nil {
		if err == sql.ErrNoRows {
			return protocol.ProjectSummary{}, fmt.Errorf("project not found")
		}
		return protocol.ProjectSummary{}, fmt.Errorf("get project: %w", err)
	}
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
	defer rows.Close()

	detail := protocol.ProjectDetail{
		ID:         project.ID,
		Name:       project.Name,
		RepoURL:    project.RepoURL,
		RepoRef:    project.RepoRef,
		ConfigFile: project.ConfigFile,
	}

	for rows.Next() {
		var p protocol.PipelineDetail
		var dependsOnJSON, versioningJSON string
		if err := rows.Scan(&p.ID, &p.PipelineID, &p.Trigger, &dependsOnJSON, &p.SourceRepo, &p.SourceRef, &versioningJSON); err != nil {
			return protocol.ProjectDetail{}, fmt.Errorf("scan pipeline: %w", err)
		}
		_ = json.Unmarshal([]byte(dependsOnJSON), &p.DependsOn)
		_ = json.Unmarshal([]byte(versioningJSON), &p.Versioning)
		persistedJobs, err := s.listPipelineJobs(p.ID)
		if err != nil {
			return protocol.ProjectDetail{}, err
		}
		p.Jobs = make([]protocol.PipelineJobDetail, 0, len(persistedJobs))
		for _, j := range persistedJobs {
			d := protocol.PipelineJobDetail{
				ID:             j.ID,
				TimeoutSeconds: j.TimeoutSeconds,
				RunsOn:         cloneMap(j.RunsOn),
				RequiresTools:  cloneMap(j.RequiresTools),
				Artifacts:      append([]string(nil), j.Artifacts...),
				Caches:         cloneJobCachesFromConfig(j.Caches),
			}
			d.Steps = make([]protocol.PipelineStep, 0, len(j.Steps))
			for _, step := range j.Steps {
				if step.Test != nil {
					d.Steps = append(d.Steps, protocol.PipelineStep{
						Type:        "test",
						TestName:    step.Test.Name,
						TestCommand: step.Test.Command,
						TestFormat:  step.Test.Format,
						TestReport:  step.Test.Report,
						Env:         cloneMap(step.Env),
					})
					continue
				}
				d.Steps = append(d.Steps, protocol.PipelineStep{
					Type: "run",
					Run:  step.Run,
					Env:  cloneMap(step.Env),
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
			p.Jobs = append(p.Jobs, d)
		}
		detail.Pipelines = append(detail.Pipelines, p)
	}
	if err := rows.Err(); err != nil {
		return protocol.ProjectDetail{}, fmt.Errorf("iterate pipelines: %w", err)
	}

	return detail, nil
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

func (s *Store) listPipelineJobs(pipelineDBID int64) ([]PersistedPipelineJob, error) {
	rows, err := s.db.Query(`
		SELECT job_id, position, runs_on_json, requires_tools_json, timeout_seconds, artifacts_json, caches_json, matrix_json, steps_json
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
		var runsOnJSON, requiresToolsJSON, artifactsJSON, cachesJSON, matrixJSON, stepsJSON string
		if err := rows.Scan(&j.ID, &j.Position, &runsOnJSON, &requiresToolsJSON, &j.TimeoutSeconds, &artifactsJSON, &cachesJSON, &matrixJSON, &stepsJSON); err != nil {
			return nil, fmt.Errorf("scan pipeline job: %w", err)
		}
		_ = json.Unmarshal([]byte(runsOnJSON), &j.RunsOn)
		_ = json.Unmarshal([]byte(requiresToolsJSON), &j.RequiresTools)
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
