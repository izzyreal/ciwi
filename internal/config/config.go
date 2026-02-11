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
	Version   int        `yaml:"version" json:"version"`
	Project   Project    `yaml:"project" json:"project"`
	Pipelines []Pipeline `yaml:"pipelines" json:"pipelines"`
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
	ID        string   `yaml:"id" json:"id"`
	Trigger   string   `yaml:"trigger" json:"trigger"`
	DependsOn []string `yaml:"depends_on,omitempty" json:"depends_on,omitempty"`
	Source    Source   `yaml:"source" json:"source"`
	Jobs      []Job    `yaml:"jobs" json:"jobs"`
}

type Source struct {
	Repo string `yaml:"repo" json:"repo"`
	Ref  string `yaml:"ref" json:"ref"`
}

type Job struct {
	ID             string            `yaml:"id" json:"id"`
	RunsOn         map[string]string `yaml:"runs_on" json:"runs_on"`
	Requires       Requires          `yaml:"requires,omitempty" json:"requires,omitempty"`
	TimeoutSeconds int               `yaml:"timeout_seconds" json:"timeout_seconds"`
	Artifacts      []string          `yaml:"artifacts" json:"artifacts"`
	Matrix         Matrix            `yaml:"matrix" json:"matrix"`
	Steps          []Step            `yaml:"steps" json:"steps"`
}

type Requires struct {
	Tools map[string]string `yaml:"tools,omitempty" json:"tools,omitempty"`
}

type Matrix struct {
	Include []map[string]string `yaml:"include" json:"include"`
}

type Step struct {
	Run  string            `yaml:"run,omitempty" json:"run,omitempty"`
	Test *TestStep         `yaml:"test,omitempty" json:"test,omitempty"`
	Env  map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
}

type TestStep struct {
	Name    string `yaml:"name,omitempty" json:"name,omitempty"`
	Command string `yaml:"command" json:"command"`
	Format  string `yaml:"format,omitempty" json:"format,omitempty"`
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
			for k, st := range job.Steps {
				runSet := strings.TrimSpace(st.Run) != ""
				testSet := st.Test != nil
				if runSet == testSet {
					errs = append(errs, fmt.Sprintf("pipelines[%d].jobs[%d].steps[%d] must set exactly one of run or test", i, j, k))
				}
				if st.Test != nil {
					if strings.TrimSpace(st.Test.Command) == "" {
						errs = append(errs, fmt.Sprintf("pipelines[%d].jobs[%d].steps[%d].test.command is required", i, j, k))
					}
					if strings.TrimSpace(st.Test.Format) != "" && st.Test.Format != "go-test-json" {
						errs = append(errs, fmt.Sprintf("pipelines[%d].jobs[%d].steps[%d].test.format unsupported %q", i, j, k, st.Test.Format))
					}
				}
				for envK := range st.Env {
					if strings.TrimSpace(envK) == "" {
						errs = append(errs, fmt.Sprintf("pipelines[%d].jobs[%d].steps[%d].env key must not be empty", i, j, k))
					}
				}
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

	return errs
}
