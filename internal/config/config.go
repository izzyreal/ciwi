package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type File struct {
	Version   int        `yaml:"version"`
	Project   Project    `yaml:"project"`
	Pipelines []Pipeline `yaml:"pipelines"`
}

type Project struct {
	Name string `yaml:"name"`
}

type Pipeline struct {
	ID      string `yaml:"id"`
	Trigger string `yaml:"trigger"`
	Source  Source `yaml:"source"`
	Jobs    []Job  `yaml:"jobs"`
}

type Source struct {
	Repo string `yaml:"repo"`
	Ref  string `yaml:"ref"`
}

type Job struct {
	ID             string            `yaml:"id"`
	RunsOn         map[string]string `yaml:"runs_on"`
	TimeoutSeconds int               `yaml:"timeout_seconds"`
	Artifacts      []string          `yaml:"artifacts"`
	Matrix         Matrix            `yaml:"matrix"`
	Steps          []Step            `yaml:"steps"`
}

type Matrix struct {
	Include []map[string]string `yaml:"include"`
}

type Step struct {
	Run string `yaml:"run"`
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

	return cfg, nil
}
