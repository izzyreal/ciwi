package agent

import (
	"context"
	"strings"
	"testing"
)

func TestEnsureRepoGitIdentity(t *testing.T) {
	repo := t.TempDir()
	if _, err := runCommandCapture(context.Background(), "", "git", "init", repo); err != nil {
		t.Fatalf("git init repo: %v", err)
	}
	if err := ensureRepoGitIdentity(context.Background(), repo); err != nil {
		t.Fatalf("ensureRepoGitIdentity: %v", err)
	}

	gotName, err := runCommandCapture(context.Background(), "", "git", "-C", repo, "config", "--get", "user.name")
	if err != nil {
		t.Fatalf("read repo user.name: %v", err)
	}
	if got := stringTrimLine(gotName); got != defaultRepoGitUserName {
		t.Fatalf("unexpected repo user.name: %q", got)
	}

	gotEmail, err := runCommandCapture(context.Background(), "", "git", "-C", repo, "config", "--get", "user.email")
	if err != nil {
		t.Fatalf("read repo user.email: %v", err)
	}
	if got := stringTrimLine(gotEmail); got != defaultRepoGitUserEmail {
		t.Fatalf("unexpected repo user.email: %q", got)
	}
}

func TestEnsureRepoGitIdentityRejectsEmptyRepoDir(t *testing.T) {
	if err := ensureRepoGitIdentity(context.Background(), " \t "); err == nil || !strings.Contains(err.Error(), "empty repository directory") {
		t.Fatalf("expected empty repository directory error, got %v", err)
	}
}

func TestEnsureRepoGitIdentityReturnsGitConfigFailure(t *testing.T) {
	err := ensureRepoGitIdentity(context.Background(), "/definitely/missing/ciwi-repo")
	if err == nil || !strings.Contains(err.Error(), "set repo git user.name") {
		t.Fatalf("expected git user.name failure, got %v", err)
	}
}

func TestRepoGitIdentitySummary(t *testing.T) {
	got := repoGitIdentitySummary()
	want := "[git] repo_identity user.name=ciwi-agent user.email=ciwi-agent@local"
	if got != want {
		t.Fatalf("unexpected summary: got=%q want=%q", got, want)
	}
}

func stringTrimLine(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}
