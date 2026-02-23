package agent

import (
	"context"
	"fmt"
	"strings"
)

const (
	defaultRepoGitUserName  = "ciwi-agent"
	defaultRepoGitUserEmail = "ciwi-agent@local"
)

// ensureRepoGitIdentity sets a stable git identity inside the checked-out repository.
func ensureRepoGitIdentity(ctx context.Context, repoDir string) error {
	repoDir = strings.TrimSpace(repoDir)
	if repoDir == "" {
		return fmt.Errorf("empty repository directory")
	}
	if _, err := runCommandCapture(ctx, "", "git", "-C", repoDir, "config", "user.name", defaultRepoGitUserName); err != nil {
		return fmt.Errorf("set repo git user.name: %w", err)
	}
	if _, err := runCommandCapture(ctx, "", "git", "-C", repoDir, "config", "user.email", defaultRepoGitUserEmail); err != nil {
		return fmt.Errorf("set repo git user.email: %w", err)
	}
	return nil
}

func repoGitIdentitySummary() string {
	return fmt.Sprintf("[git] repo_identity user.name=%s user.email=%s",
		strings.TrimSpace(defaultRepoGitUserName),
		strings.TrimSpace(defaultRepoGitUserEmail),
	)
}
