package presentation

import (
	"strings"

	"github.com/izzyreal/ciwi/internal/domain"
)

// ProjectSettingsLabels contains renderer-neutral project semantics used by
// the Global Settings project list. Timestamp formatting remains a renderer
// concern because it is intentionally shown in the user's local timezone.
type ProjectSettingsLabels struct {
	IsManaged         bool
	CanReload         bool
	HasRepository     bool
	RepositoryRef     string
	HasLoadedCommit   bool
	LoadedCommitShort string
	LoadedCommitURL   string
	SourceLabel       string
}

func PresentProjectSettings(project domain.Project) ProjectSettingsLabels {
	managed := strings.TrimSpace(project.SourceKind) == "managed_yaml"
	repository := strings.TrimSpace(project.RepoURL)
	ref := strings.TrimSpace(project.RepoRef)
	if ref == "" {
		ref = "default"
	}
	commit := strings.TrimSpace(project.LoadedCommit)
	shortCommit := commit
	if len(shortCommit) > 8 {
		shortCommit = shortCommit[:8]
	}
	sourceLabel := "Managed YAML stored in ciwi"
	if !managed {
		sourceLabel = repository
		if ref != "default" {
			if sourceLabel != "" {
				sourceLabel += " · "
			}
			sourceLabel += ref
		}
	}
	return ProjectSettingsLabels{
		IsManaged: managed, CanReload: !managed, HasRepository: repository != "",
		RepositoryRef: ref, HasLoadedCommit: commit != "", LoadedCommitShort: shortCommit,
		LoadedCommitURL: projectCommitURL(repository, commit), SourceLabel: sourceLabel,
	}
}

func projectCommitURL(repository, commit string) string {
	if repository == "" || commit == "" {
		return ""
	}
	repository = strings.TrimSuffix(strings.TrimRight(repository, "/"), ".git")
	if !strings.HasPrefix(repository, "https://") && !strings.HasPrefix(repository, "http://") {
		return ""
	}
	return repository + "/commit/" + commit
}
