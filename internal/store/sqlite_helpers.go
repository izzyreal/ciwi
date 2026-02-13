package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/config"
	"github.com/izzyreal/ciwi/internal/protocol"
	"golang.org/x/mod/semver"
)

func scanJobExecution(scanner interface{ Scan(dest ...any) error }) (protocol.JobExecution, error) {
	var (
		job                                                                protocol.JobExecution
		envJSON, requiredJSON, artifactGlobsJSON, cachesJSON, metadataJSON string
		sourceRepo, sourceRef                                              sql.NullString
		createdUTC                                                         string
		startedUTC, finishedUTC                                            sql.NullString
		leasedByAgentID, leasedUTC                                         sql.NullString
		exitCode                                                           sql.NullInt64
		errorText, outputText, currentStepText                             sql.NullString
	)

	if err := scanner.Scan(
		&job.ID, &job.Script, &envJSON, &requiredJSON, &job.TimeoutSeconds, &artifactGlobsJSON, &cachesJSON, &sourceRepo, &sourceRef, &metadataJSON,
		&job.Status, &createdUTC, &startedUTC, &finishedUTC, &leasedByAgentID, &leasedUTC, &exitCode, &errorText, &outputText, &currentStepText,
	); err != nil {
		return protocol.JobExecution{}, err
	}

	_ = json.Unmarshal([]byte(envJSON), &job.Env)
	_ = json.Unmarshal([]byte(requiredJSON), &job.RequiredCapabilities)
	_ = json.Unmarshal([]byte(artifactGlobsJSON), &job.ArtifactGlobs)
	_ = json.Unmarshal([]byte(cachesJSON), &job.Caches)
	_ = json.Unmarshal([]byte(metadataJSON), &job.Metadata)

	if sourceRepo.Valid && sourceRepo.String != "" {
		job.Source = &protocol.SourceSpec{Repo: sourceRepo.String, Ref: sourceRef.String}
	}
	if createdUTC != "" {
		if t, err := time.Parse(time.RFC3339Nano, createdUTC); err == nil {
			job.CreatedUTC = t
		}
	}
	if startedUTC.Valid {
		if t, err := time.Parse(time.RFC3339Nano, startedUTC.String); err == nil {
			job.StartedUTC = t
		}
	}
	if finishedUTC.Valid {
		if t, err := time.Parse(time.RFC3339Nano, finishedUTC.String); err == nil {
			job.FinishedUTC = t
		}
	}
	if leasedByAgentID.Valid {
		job.LeasedByAgentID = leasedByAgentID.String
	}
	if leasedUTC.Valid {
		if t, err := time.Parse(time.RFC3339Nano, leasedUTC.String); err == nil {
			job.LeasedUTC = t
		}
	}
	if exitCode.Valid {
		v := int(exitCode.Int64)
		job.ExitCode = &v
	}
	if errorText.Valid {
		job.Error = errorText.String
	}
	if outputText.Valid {
		job.Output = outputText.String
	}
	if currentStepText.Valid {
		job.CurrentStep = strings.TrimSpace(currentStepText.String)
	}

	return job, nil
}

func capabilitiesMatch(agentCapabilities, requiredCapabilities map[string]string) bool {
	if len(requiredCapabilities) == 0 {
		return true
	}
	for k, requiredValue := range requiredCapabilities {
		if strings.HasPrefix(k, "requires.tool.") {
			tool := strings.TrimPrefix(k, "requires.tool.")
			agentValue := strings.TrimSpace(agentCapabilities["tool."+tool])
			if !toolConstraintMatch(agentValue, strings.TrimSpace(requiredValue)) {
				return false
			}
			continue
		}
		if k == "shell" {
			if !shellCapabilityMatch(agentCapabilities, requiredValue) {
				return false
			}
			continue
		}
		if agentCapabilities[k] != requiredValue {
			return false
		}
	}
	return true
}

func shellCapabilityMatch(agentCapabilities map[string]string, requiredValue string) bool {
	required := strings.ToLower(strings.TrimSpace(requiredValue))
	if required == "" {
		return true
	}
	for _, s := range strings.Split(agentCapabilities["shells"], ",") {
		if strings.EqualFold(strings.TrimSpace(s), required) {
			return true
		}
	}
	return false
}

func toolConstraintMatch(agentValue, constraint string) bool {
	agentValue = strings.TrimSpace(agentValue)
	constraint = strings.TrimSpace(constraint)
	if agentValue == "" {
		return false
	}
	if constraint == "" || constraint == "*" {
		return true
	}
	op := ""
	val := constraint
	for _, candidate := range []string{">=", "<=", ">", "<", "==", "="} {
		if strings.HasPrefix(constraint, candidate) {
			op = candidate
			val = strings.TrimSpace(strings.TrimPrefix(constraint, candidate))
			break
		}
	}
	if val == "" {
		return true
	}
	if op == "" {
		return agentValue == val
	}
	av, aok := normalizeSemver(agentValue)
	vv, vok := normalizeSemver(val)
	if !aok || !vok {
		switch op {
		case "=", "==":
			return agentValue == val
		default:
			return false
		}
	}
	cmp := semver.Compare(av, vv)
	switch op {
	case ">":
		return cmp > 0
	case ">=":
		return cmp >= 0
	case "<":
		return cmp < 0
	case "<=":
		return cmp <= 0
	case "=", "==":
		return cmp == 0
	default:
		return false
	}
}

func normalizeSemver(v string) (string, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", false
	}
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	if !semver.IsValid(v) {
		return "", false
	}
	return v, true
}

func nullableTime(t time.Time) sql.NullString {
	if t.IsZero() {
		return sql.NullString{}
	}
	return sql.NullString{String: t.Format(time.RFC3339Nano), Valid: true}
}

func nullableInt(v *int) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*v), Valid: true}
}

func nullStringValue(v sql.NullString) any {
	if !v.Valid {
		return nil
	}
	return v.String
}

func nullIntValue(v sql.NullInt64) any {
	if !v.Valid {
		return nil
	}
	return v.Int64
}

func cloneMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneJobCaches(in []protocol.JobCacheSpec) []protocol.JobCacheSpec {
	if len(in) == 0 {
		return nil
	}
	out := make([]protocol.JobCacheSpec, 0, len(in))
	for _, c := range in {
		out = append(out, protocol.JobCacheSpec{
			ID:          c.ID,
			Env:         c.Env,
			Key:         cloneJobCacheKey(c.Key),
			RestoreKeys: append([]string(nil), c.RestoreKeys...),
			Policy:      c.Policy,
			TTLDays:     c.TTLDays,
			MaxSizeMB:   c.MaxSizeMB,
		})
	}
	return out
}

func cloneJobCachesFromConfig(in []config.PipelineJobCacheSpec) []protocol.JobCacheSpec {
	if len(in) == 0 {
		return nil
	}
	out := make([]protocol.JobCacheSpec, 0, len(in))
	for _, c := range in {
		out = append(out, protocol.JobCacheSpec{
			ID:  c.ID,
			Env: c.Env,
			Key: protocol.JobCacheKey{
				Prefix:  c.Key.Prefix,
				Files:   append([]string(nil), c.Key.Files...),
				Runtime: append([]string(nil), c.Key.Runtime...),
				Tools:   append([]string(nil), c.Key.Tools...),
				Env:     append([]string(nil), c.Key.Env...),
			},
			RestoreKeys: append([]string(nil), c.RestoreKeys...),
			Policy:      c.Policy,
			TTLDays:     c.TTLDays,
			MaxSizeMB:   c.MaxSizeMB,
		})
	}
	return out
}

func cloneJobCacheKey(in protocol.JobCacheKey) protocol.JobCacheKey {
	return protocol.JobCacheKey{
		Prefix:  in.Prefix,
		Files:   append([]string(nil), in.Files...),
		Runtime: append([]string(nil), in.Runtime...),
		Tools:   append([]string(nil), in.Tools...),
		Env:     append([]string(nil), in.Env...),
	}
}

func cloneSource(in *protocol.SourceSpec) *protocol.SourceSpec {
	if in == nil {
		return nil
	}
	return &protocol.SourceSpec{Repo: in.Repo, Ref: in.Ref}
}

func (p PersistedPipeline) SortedJobs() []PersistedPipelineJob {
	jobs := make([]PersistedPipelineJob, len(p.Jobs))
	copy(jobs, p.Jobs)
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].Position < jobs[j].Position })
	return jobs
}

func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}
