package presentation

import (
	"testing"

	"github.com/izzyreal/ciwi/internal/domain"
)

func TestPresentProjectSettingsRepositoryProject(t *testing.T) {
	got := PresentProjectSettings(domain.Project{
		SourceKind: "repository", RepoURL: "https://example.test/team/repo.git/",
		RepoRef: "main", LoadedCommit: "1234567890abcdef",
	})
	if got.IsManaged || !got.CanReload || !got.HasRepository || !got.HasLoadedCommit {
		t.Fatalf("unexpected flags: %+v", got)
	}
	if got.RepositoryRef != "main" || got.LoadedCommitShort != "12345678" {
		t.Fatalf("unexpected labels: %+v", got)
	}
	if got.LoadedCommitURL != "https://example.test/team/repo/commit/1234567890abcdef" {
		t.Fatalf("unexpected commit URL: %q", got.LoadedCommitURL)
	}
	if got.SourceLabel != "https://example.test/team/repo.git/ · main" {
		t.Fatalf("unexpected source label: %q", got.SourceLabel)
	}
}

func TestPresentProjectSettingsManagedProject(t *testing.T) {
	got := PresentProjectSettings(domain.Project{SourceKind: "managed_yaml"})
	if !got.IsManaged || got.CanReload || got.HasRepository || got.HasLoadedCommit {
		t.Fatalf("unexpected flags: %+v", got)
	}
	if got.RepositoryRef != "default" || got.SourceLabel != "Managed YAML stored in ciwi" {
		t.Fatalf("unexpected labels: %+v", got)
	}
}
