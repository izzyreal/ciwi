package config

import (
	"fmt"
	"os"

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
	ID      string `yaml:"id" json:"id"`
	Trigger string `yaml:"trigger" json:"trigger"`
	Source  Source `yaml:"source" json:"source"`
	Jobs    []Job  `yaml:"jobs" json:"jobs"`
}

type Source struct {
	Repo string `yaml:"repo" json:"repo"`
	Ref  string `yaml:"ref" json:"ref"`
}

type Job struct {
	ID             string            `yaml:"id" json:"id"`
	RunsOn         map[string]string `yaml:"runs_on" json:"runs_on"`
	TimeoutSeconds int               `yaml:"timeout_seconds" json:"timeout_seconds"`
	Artifacts      []string          `yaml:"artifacts" json:"artifacts"`
	Matrix         Matrix            `yaml:"matrix" json:"matrix"`
	Steps          []Step            `yaml:"steps" json:"steps"`
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

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse YAML in %q: %w", source, err)
	}

	if cfg.Version != 1 {
		return cfg, fmt.Errorf("unsupported config version %d", cfg.Version)
	}
	if cfg.Project.Name == "" {
		return cfg, fmt.Errorf("project.name is required")
	}
	if cfg.Project.Vault != nil {
		if cfg.Project.Vault.Connection == "" {
			return cfg, fmt.Errorf("project.vault.connection is required when project.vault is set")
		}
		for i, sec := range cfg.Project.Vault.Secrets {
			if sec.Name == "" || sec.Path == "" || sec.Key == "" {
				return cfg, fmt.Errorf("project.vault.secrets[%d] requires name, path and key", i)
			}
		}
	}

	return cfg, nil
}
