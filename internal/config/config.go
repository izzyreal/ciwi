package config

import (
	"bytes"
	"fmt"
	"os"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

type File struct {
	Version        int             `yaml:"version" json:"version"`
	Project        Project         `yaml:"project" json:"project"`
	Pipelines      []Pipeline      `yaml:"pipelines" json:"pipelines"`
	PipelineChains []PipelineChain `yaml:"pipeline_chains,omitempty" json:"pipeline_chains,omitempty"`
}

type Project struct {
	Name  string        `yaml:"name" json:"name"`
	Vault *ProjectVault `yaml:"vault,omitempty" json:"vault,omitempty"`
}

type ProjectVault struct {
	Connection string               `yaml:"connection" json:"connection"`
	Secrets    []ProjectVaultSecret `yaml:"secrets,omitempty" json:"secrets,omitempty"`
}

type ProjectVaultSecret struct {
	Name      string `yaml:"name" json:"name"`
	Mount     string `yaml:"mount,omitempty" json:"mount,omitempty"`
	Path      string `yaml:"path" json:"path"`
	Key       string `yaml:"key" json:"key"`
	KVVersion int    `yaml:"kv_version,omitempty" json:"kv_version,omitempty"`
}

type Pipeline struct {
	ID         string              `yaml:"id" json:"id"`
	Trigger    string              `yaml:"trigger" json:"trigger"`
	DependsOn  []string            `yaml:"depends_on,omitempty" json:"depends_on,omitempty"`
	Source     Source              `yaml:"source" json:"source"`
	Versioning *PipelineVersioning `yaml:"versioning,omitempty" json:"versioning,omitempty"`
	Jobs       []PipelineJobSpec   `yaml:"jobs" json:"jobs"`
}

type PipelineChain struct {
	ID        string   `yaml:"id" json:"id"`
	Pipelines []string `yaml:"pipelines" json:"pipelines"`
}

type PipelineVersioning struct {
	File      string `yaml:"file,omitempty" json:"file,omitempty"`
	TagPrefix string `yaml:"tag_prefix,omitempty" json:"tag_prefix,omitempty"`
	AutoBump  string `yaml:"auto_bump,omitempty" json:"auto_bump,omitempty"`
}

type Source struct {
	Repo string `yaml:"repo" json:"repo"`
	Ref  string `yaml:"ref" json:"ref"`
}

type PipelineJobSpec struct {
	ID             string                  `yaml:"id" json:"id"`
	Needs          []string                `yaml:"needs,omitempty" json:"needs,omitempty"`
	RunsOn         map[string]string       `yaml:"runs_on" json:"runs_on"`
	Requires       PipelineJobRequirements `yaml:"requires,omitempty" json:"requires,omitempty"`
	TimeoutSeconds int                     `yaml:"timeout_seconds" json:"timeout_seconds"`
	Artifacts      []string                `yaml:"artifacts" json:"artifacts"`
	Caches         []PipelineJobCacheSpec  `yaml:"caches,omitempty" json:"caches,omitempty"`
	Matrix         PipelineJobMatrix       `yaml:"matrix" json:"matrix"`
	Steps          []PipelineJobStep       `yaml:"steps" json:"steps"`
}

type PipelineJobCacheSpec struct {
	ID  string `yaml:"id" json:"id"`
	Env string `yaml:"env,omitempty" json:"env,omitempty"`
}

type PipelineJobRequirements struct {
	Tools        map[string]string `yaml:"tools,omitempty" json:"tools,omitempty"`
	Capabilities map[string]string `yaml:"capabilities,omitempty" json:"capabilities,omitempty"`
}

type PipelineJobMatrix struct {
	Include []map[string]string `yaml:"include" json:"include"`
}

type PipelineJobStep struct {
	Run        string               `yaml:"run,omitempty" json:"run,omitempty"`
	Test       *PipelineJobTestStep `yaml:"test,omitempty" json:"test,omitempty"`
	SkipDryRun bool                 `yaml:"skip_dry_run,omitempty" json:"skip_dry_run,omitempty"`
	Env        map[string]string    `yaml:"env,omitempty" json:"env,omitempty"`
}

type PipelineJobTestStep struct {
	Name           string `yaml:"name,omitempty" json:"name,omitempty"`
	Command        string `yaml:"command" json:"command"`
	Format         string `yaml:"format,omitempty" json:"format,omitempty"`
	Report         string `yaml:"report,omitempty" json:"report,omitempty"`
	CoverageFormat string `yaml:"coverage_format,omitempty" json:"coverage_format,omitempty"`
	CoverageReport string `yaml:"coverage_report,omitempty" json:"coverage_report,omitempty"`
}

func Load(path string) (File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return File{}, fmt.Errorf("read config file %q: %w", path, err)
	}

	return Parse(data, path)
}

func Parse(data []byte, source string) (File, error) {
	var cfg File

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return cfg, fmt.Errorf("parse YAML in %q: %w", source, err)
	}

	if errs := cfg.Validate(); len(errs) > 0 {
		return cfg, fmt.Errorf("invalid config in %q: %s", source, strings.Join(errs, "; "))
	}
	return cfg, nil
}

func (cfg File) Validate() []string {
	var errs []string

	if cfg.Version != 1 {
		errs = append(errs, fmt.Sprintf("unsupported config version %d", cfg.Version))
	}
	if strings.TrimSpace(cfg.Project.Name) == "" {
		errs = append(errs, "project.name is required")
	}
	if cfg.Project.Vault != nil {
		if strings.TrimSpace(cfg.Project.Vault.Connection) == "" {
			errs = append(errs, "project.vault.connection is required when project.vault is set")
		}
		seenSecrets := map[string]struct{}{}
		for i, sec := range cfg.Project.Vault.Secrets {
			if strings.TrimSpace(sec.Name) == "" || strings.TrimSpace(sec.Path) == "" || strings.TrimSpace(sec.Key) == "" {
				errs = append(errs, fmt.Sprintf("project.vault.secrets[%d] requires name, path and key", i))
			}
			if strings.TrimSpace(sec.Name) != "" {
				if _, ok := seenSecrets[sec.Name]; ok {
					errs = append(errs, fmt.Sprintf("project.vault.secrets[%d] duplicate name %q", i, sec.Name))
				}
				seenSecrets[sec.Name] = struct{}{}
			}
		}
	}

	if len(cfg.Pipelines) == 0 {
		errs = append(errs, "pipelines must contain at least one pipeline")
		return errs
	}

	pipelineIDs := map[string]struct{}{}
	for i, p := range cfg.Pipelines {
		if strings.TrimSpace(p.ID) == "" {
			errs = append(errs, fmt.Sprintf("pipelines[%d].id is required", i))
		} else {
			if _, exists := pipelineIDs[p.ID]; exists {
				errs = append(errs, fmt.Sprintf("pipelines[%d].id duplicate %q", i, p.ID))
			}
			pipelineIDs[p.ID] = struct{}{}
		}

		if strings.TrimSpace(p.Trigger) != "" && !slices.Contains([]string{"manual", "vcs"}, p.Trigger) {
			errs = append(errs, fmt.Sprintf("pipelines[%d].trigger must be one of manual,vcs", i))
		}

		for j, dep := range p.DependsOn {
			if strings.TrimSpace(dep) == "" {
				errs = append(errs, fmt.Sprintf("pipelines[%d].depends_on[%d] must not be empty", i, j))
			}
		}

		if len(p.Jobs) == 0 {
			errs = append(errs, fmt.Sprintf("pipelines[%d].jobs must contain at least one job", i))
		}
		if p.Versioning != nil {
			if file := strings.TrimSpace(p.Versioning.File); file != "" {
				if strings.HasPrefix(file, "/") || strings.HasPrefix(file, "..") || strings.Contains(file, ".."+string(os.PathSeparator)) {
					errs = append(errs, fmt.Sprintf("pipelines[%d].versioning.file must be a relative in-repo path", i))
				}
			}
			switch strings.TrimSpace(p.Versioning.AutoBump) {
			case "", "patch", "minor", "major":
			default:
				errs = append(errs, fmt.Sprintf("pipelines[%d].versioning.auto_bump must be one of patch,minor,major", i))
			}
		}

		jobIDs := map[string]struct{}{}
		for j, job := range p.Jobs {
			if strings.TrimSpace(job.ID) == "" {
				errs = append(errs, fmt.Sprintf("pipelines[%d].jobs[%d].id is required", i, j))
			} else {
				if _, exists := jobIDs[job.ID]; exists {
					errs = append(errs, fmt.Sprintf("pipelines[%d].jobs[%d].id duplicate %q", i, j, job.ID))
				}
				jobIDs[job.ID] = struct{}{}
			}

			if job.TimeoutSeconds < 0 {
				errs = append(errs, fmt.Sprintf("pipelines[%d].jobs[%d].timeout_seconds must be >= 0", i, j))
			}
			executor := strings.ToLower(strings.TrimSpace(job.RunsOn["executor"]))
			shell := strings.ToLower(strings.TrimSpace(job.RunsOn["shell"]))
			if executor != "" && executor != "script" {
				errs = append(errs, fmt.Sprintf("pipelines[%d].jobs[%d].runs_on.executor must be \"script\"", i, j))
			}
			if shell != "" && shell != "posix" && shell != "cmd" && shell != "powershell" {
				errs = append(errs, fmt.Sprintf("pipelines[%d].jobs[%d].runs_on.shell must be one of posix,cmd,powershell", i, j))
			}
			if executor == "script" && shell == "" {
				errs = append(errs, fmt.Sprintf("pipelines[%d].jobs[%d].runs_on.shell is required when runs_on.executor=script", i, j))
			}
			if shell != "" && executor != "script" {
				errs = append(errs, fmt.Sprintf("pipelines[%d].jobs[%d].runs_on.executor must be \"script\" when runs_on.shell is set", i, j))
			}
			if len(job.Steps) == 0 {
				errs = append(errs, fmt.Sprintf("pipelines[%d].jobs[%d].steps must contain at least one step", i, j))
			}
			for tool, constraint := range job.Requires.Tools {
				if strings.TrimSpace(tool) == "" {
					errs = append(errs, fmt.Sprintf("pipelines[%d].jobs[%d].requires.tools contains empty tool name", i, j))
				}
				if strings.ContainsAny(tool, " \t\n\r") {
					errs = append(errs, fmt.Sprintf("pipelines[%d].jobs[%d].requires.tools[%q] invalid tool name", i, j, tool))
				}
				if strings.TrimSpace(constraint) == "" {
					continue
				}
			}
			for capKey, requiredValue := range job.Requires.Capabilities {
				if strings.TrimSpace(capKey) == "" {
					errs = append(errs, fmt.Sprintf("pipelines[%d].jobs[%d].requires.capabilities contains empty capability name", i, j))
				}
				if strings.ContainsAny(capKey, " \t\n\r") {
					errs = append(errs, fmt.Sprintf("pipelines[%d].jobs[%d].requires.capabilities[%q] invalid capability name", i, j, capKey))
				}
				if strings.TrimSpace(requiredValue) == "" {
					continue
				}
			}
			cacheIDs := map[string]struct{}{}
			for cIdx, c := range job.Caches {
				cacheID := strings.TrimSpace(c.ID)
				if cacheID == "" {
					errs = append(errs, fmt.Sprintf("pipelines[%d].jobs[%d].caches[%d].id is required", i, j, cIdx))
				} else {
					if _, exists := cacheIDs[cacheID]; exists {
						errs = append(errs, fmt.Sprintf("pipelines[%d].jobs[%d].caches[%d].id duplicate %q", i, j, cIdx, cacheID))
					}
					cacheIDs[cacheID] = struct{}{}
				}
				cacheEnv := strings.TrimSpace(c.Env)
				if cacheEnv == "" {
					errs = append(errs, fmt.Sprintf("pipelines[%d].jobs[%d].caches[%d].env is required", i, j, cIdx))
				}
			}
			for k, st := range job.Steps {
				runSet := strings.TrimSpace(st.Run) != ""
				testSet := st.Test != nil
				stepModeCount := 0
				if runSet {
					stepModeCount++
				}
				if testSet {
					stepModeCount++
				}
				if stepModeCount != 1 {
					errs = append(errs, fmt.Sprintf("pipelines[%d].jobs[%d].steps[%d] must set exactly one of run or test", i, j, k))
				}
				if st.Test != nil {
					if strings.TrimSpace(st.Test.Command) == "" {
						errs = append(errs, fmt.Sprintf("pipelines[%d].jobs[%d].steps[%d].test.command is required", i, j, k))
					}
					if format := strings.TrimSpace(st.Test.Format); format != "" {
						switch format {
						case "go-test-json", "junit", "junit-xml":
						default:
							errs = append(errs, fmt.Sprintf("pipelines[%d].jobs[%d].steps[%d].test.format unsupported %q", i, j, k, st.Test.Format))
						}
					}
					report := strings.TrimSpace(st.Test.Report)
					if report == "" {
						errs = append(errs, fmt.Sprintf("pipelines[%d].jobs[%d].steps[%d].test.report is required", i, j, k))
					} else if hasUnsafePath(report) {
						errs = append(errs, fmt.Sprintf("pipelines[%d].jobs[%d].steps[%d].test.report must be a relative in-repo path", i, j, k))
					}
					coverageReport := strings.TrimSpace(st.Test.CoverageReport)
					coverageFormat := strings.TrimSpace(st.Test.CoverageFormat)
					if coverageReport != "" {
						if hasUnsafePath(coverageReport) {
							errs = append(errs, fmt.Sprintf("pipelines[%d].jobs[%d].steps[%d].test.coverage_report must be a relative in-repo path", i, j, k))
						}
					}
					if coverageFormat != "" {
						switch coverageFormat {
						case "go-coverprofile", "lcov":
						default:
							errs = append(errs, fmt.Sprintf("pipelines[%d].jobs[%d].steps[%d].test.coverage_format unsupported %q", i, j, k, st.Test.CoverageFormat))
						}
					}
					if coverageReport == "" && coverageFormat != "" {
						errs = append(errs, fmt.Sprintf("pipelines[%d].jobs[%d].steps[%d].test.coverage_report is required when coverage_format is set", i, j, k))
					}
				}
				for envK := range st.Env {
					if strings.TrimSpace(envK) == "" {
						errs = append(errs, fmt.Sprintf("pipelines[%d].jobs[%d].steps[%d].env key must not be empty", i, j, k))
					}
				}
			}

			for j, job := range p.Jobs {
				seenNeeds := map[string]struct{}{}
				for k, need := range job.Needs {
					need = strings.TrimSpace(need)
					if need == "" {
						errs = append(errs, fmt.Sprintf("pipelines[%d].jobs[%d].needs[%d] must not be empty", i, j, k))
						continue
					}
					if need == job.ID {
						errs = append(errs, fmt.Sprintf("pipelines[%d].jobs[%d].needs[%d] must not reference itself", i, j, k))
						continue
					}
					if _, ok := jobIDs[need]; !ok {
						errs = append(errs, fmt.Sprintf("pipelines[%d].jobs[%d].needs[%d] references unknown job %q", i, j, k, need))
						continue
					}
					if _, dup := seenNeeds[need]; dup {
						errs = append(errs, fmt.Sprintf("pipelines[%d].jobs[%d].needs[%d] duplicate %q", i, j, k, need))
						continue
					}
					seenNeeds[need] = struct{}{}
				}
			}
			if hasPipelineJobNeedsCycle(p.Jobs) {
				errs = append(errs, fmt.Sprintf("pipelines[%d].jobs contains cyclic needs graph", i))
			}
		}
	}

	for i, p := range cfg.Pipelines {
		for j, dep := range p.DependsOn {
			dep = strings.TrimSpace(dep)
			if dep == "" {
				continue
			}
			if _, ok := pipelineIDs[dep]; !ok {
				errs = append(errs, fmt.Sprintf("pipelines[%d].depends_on[%d] references unknown pipeline %q", i, j, dep))
			}
		}
	}

	chainIDs := map[string]struct{}{}
	for i, ch := range cfg.PipelineChains {
		chainID := strings.TrimSpace(ch.ID)
		if chainID == "" {
			errs = append(errs, fmt.Sprintf("pipeline_chains[%d].id is required", i))
		} else {
			if _, exists := chainIDs[chainID]; exists {
				errs = append(errs, fmt.Sprintf("pipeline_chains[%d].id duplicate %q", i, chainID))
			}
			chainIDs[chainID] = struct{}{}
		}
		if len(ch.Pipelines) == 0 {
			errs = append(errs, fmt.Sprintf("pipeline_chains[%d].pipelines must contain at least one pipeline id", i))
			continue
		}
		seen := map[string]struct{}{}
		for j, pid := range ch.Pipelines {
			pid = strings.TrimSpace(pid)
			if pid == "" {
				errs = append(errs, fmt.Sprintf("pipeline_chains[%d].pipelines[%d] must not be empty", i, j))
				continue
			}
			if _, ok := pipelineIDs[pid]; !ok {
				errs = append(errs, fmt.Sprintf("pipeline_chains[%d].pipelines[%d] references unknown pipeline %q", i, j, pid))
			}
			if _, ok := seen[pid]; ok {
				errs = append(errs, fmt.Sprintf("pipeline_chains[%d].pipelines[%d] duplicate pipeline %q", i, j, pid))
			}
			seen[pid] = struct{}{}
		}
	}

	return errs
}

func hasUnsafePath(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" {
		return true
	}
	if strings.HasPrefix(v, "/") || strings.HasPrefix(v, "\\") {
		return true
	}
	if strings.HasPrefix(v, "..") || strings.Contains(v, "../") || strings.Contains(v, `..\`) {
		return true
	}
	return false
}

func hasPipelineJobNeedsCycle(jobs []PipelineJobSpec) bool {
	edges := make(map[string][]string, len(jobs))
	for _, job := range jobs {
		if strings.TrimSpace(job.ID) == "" {
			continue
		}
		for _, need := range job.Needs {
			need = strings.TrimSpace(need)
			if need == "" {
				continue
			}
			edges[job.ID] = append(edges[job.ID], need)
		}
	}
	visiting := map[string]bool{}
	visited := map[string]bool{}
	var visit func(id string) bool
	visit = func(id string) bool {
		if visited[id] {
			return false
		}
		if visiting[id] {
			return true
		}
		visiting[id] = true
		for _, dep := range edges[id] {
			if visit(dep) {
				return true
			}
		}
		visiting[id] = false
		visited[id] = true
		return false
	}
	for _, job := range jobs {
		id := strings.TrimSpace(job.ID)
		if id == "" {
			continue
		}
		if visit(id) {
			return true
		}
	}
	return false
}
