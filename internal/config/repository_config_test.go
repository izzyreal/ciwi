package config

import (
	"path/filepath"
	"testing"
)

func TestRepositoryCiwiProjectConfigurationIsValid(t *testing.T) {
	configuration, err := Load(filepath.Join("..", "..", "ciwi-project.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if configuration.Project.Name != "ciwi" {
		t.Fatalf("project name = %q", configuration.Project.Name)
	}
}
