package agent

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/izzyreal/ciwi/internal/domain"
	"github.com/izzyreal/ciwi/internal/protocol"
)

var commitLikeRef = regexp.MustCompile(`^[0-9a-fA-F]{7,40}$`)

func runAutoBumpStep(ctx context.Context, execDir string, spec *protocol.AutoBumpStepSpec, env []string, output io.Writer) error {
	if spec == nil {
		return fmt.Errorf("auto-bump step is missing its execution details")
	}
	if strings.TrimSpace(spec.SourceRepo) == "" {
		return fmt.Errorf("auto-bump source repository is required")
	}
	nextVersion, err := domain.NextAutoBumpVersion(spec.VersionRaw, spec.Mode)
	if err != nil {
		return err
	}
	branch, err := resolveAutoBumpBranch(ctx, execDir, env, spec.SourceRefRaw)
	if err != nil {
		return err
	}
	if _, err := runAutoBumpGit(ctx, execDir, env, "", "check-ref-format", "--branch", branch); err != nil {
		return fmt.Errorf("invalid auto-bump target branch %q: %w", branch, err)
	}
	fmt.Fprintf(output, "[auto-bump] branch=%s version=%s -> %s\n", branch, spec.VersionRaw, nextVersion)
	if err := runAutoBumpGitLogged(ctx, execDir, env, output, "", "fetch", "origin", branch); err != nil {
		return err
	}
	if err := runAutoBumpGitLogged(ctx, execDir, env, output, "", "checkout", "-B", "ciwi-auto-bump", "origin/"+branch); err != nil {
		return err
	}
	versionPath, err := autoBumpVersionPath(execDir, spec.VersionFile)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(versionPath)
	if err != nil {
		return fmt.Errorf("read auto-bump version file %q: %w", spec.VersionFile, err)
	}
	currentVersion := strings.Join(strings.Fields(string(data)), "")
	if currentVersion != spec.VersionRaw {
		return fmt.Errorf("auto bump skipped: branch %s moved from %s to %s; rerun release from latest branch head", branch, spec.VersionRaw, currentVersion)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.WriteFile(versionPath, []byte(nextVersion+"\n"), 0o644); err != nil {
		return fmt.Errorf("write auto-bump version file %q: %w", spec.VersionFile, err)
	}
	if err := runAutoBumpGitLogged(ctx, execDir, env, output, "", "add", "--", spec.VersionFile); err != nil {
		return err
	}
	message := fmt.Sprintf("chore: bump %s to %s [skip ci]", spec.VersionFile, nextVersion)
	if err := runAutoBumpGitLogged(ctx, execDir, env, output, "", "commit", "-m", message); err != nil {
		return err
	}
	remote, token := autoBumpPushRemote(spec.SourceRepo, env)
	if err := runAutoBumpGitLogged(ctx, execDir, env, output, token, "push", remote, "HEAD:refs/heads/"+branch); err != nil {
		return fmt.Errorf("auto bump push failed; branch %s may have advanced: %w", branch, err)
	}
	return nil
}

func resolveAutoBumpBranch(ctx context.Context, execDir string, env []string, rawRef string) (string, error) {
	rawRef = strings.TrimSpace(rawRef)
	if rawRef != "" {
		switch {
		case strings.HasPrefix(rawRef, "refs/heads/"):
			return strings.TrimPrefix(rawRef, "refs/heads/"), nil
		case strings.HasPrefix(rawRef, "refs/") || commitLikeRef.MatchString(rawRef):
			return "", fmt.Errorf("auto bump requires source.ref to be a branch, got %q", rawRef)
		default:
			return rawRef, nil
		}
	}
	current, currentErr := runAutoBumpGit(ctx, execDir, env, "", "rev-parse", "--abbrev-ref", "HEAD")
	if currentErr == nil && current != "" && current != "HEAD" {
		return current, nil
	}
	originHead, err := runAutoBumpGit(ctx, execDir, env, "", "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD")
	if err == nil && strings.HasPrefix(originHead, "origin/") {
		return strings.TrimPrefix(originHead, "origin/"), nil
	}
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	return "", fmt.Errorf("failed to resolve target branch for auto bump push")
}

func autoBumpVersionPath(execDir, file string) (string, error) {
	file = strings.TrimSpace(file)
	clean := filepath.Clean(file)
	if file == "" || filepath.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid auto-bump version file %q", file)
	}
	root, err := filepath.EvalSymlinks(execDir)
	if err != nil {
		return "", fmt.Errorf("resolve auto-bump workspace: %w", err)
	}
	full, err := filepath.EvalSymlinks(filepath.Join(root, clean))
	if err != nil {
		return "", fmt.Errorf("resolve auto-bump version file %q: %w", file, err)
	}
	rel, err := filepath.Rel(root, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("auto-bump version file %q escapes the workspace", file)
	}
	return full, nil
}

func autoBumpPushRemote(sourceRepo string, env []string) (string, string) {
	token := autoBumpEnvValue(env, "GITHUB_TOKEN")
	if token == "" {
		token = autoBumpEnvValue(env, "GH_TOKEN")
	}
	if token == "" {
		return "origin", ""
	}
	u, err := url.Parse(strings.TrimSpace(sourceRepo))
	if err != nil || u.Scheme != "https" || !strings.EqualFold(u.Host, "github.com") || u.Path == "" {
		return "origin", ""
	}
	u.User = url.UserPassword("x-access-token", token)
	return u.String(), token
}

func autoBumpEnvValue(env []string, key string) string {
	for i := len(env) - 1; i >= 0; i-- {
		name, value, ok := strings.Cut(env[i], "=")
		if ok && name == key {
			return value
		}
	}
	return ""
}

func runAutoBumpGitLogged(ctx context.Context, dir string, env []string, output io.Writer, secret string, args ...string) error {
	result, err := runAutoBumpGit(ctx, dir, env, secret, args...)
	if result != "" {
		fmt.Fprintln(output, result)
	}
	return err
}

func runAutoBumpGit(ctx context.Context, dir string, env []string, secret string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = mergeEnv(env, map[string]string{"GIT_TERMINAL_PROMPT": "0"})
	var captured bytes.Buffer
	cmd.Stdout = &captured
	cmd.Stderr = &captured
	prepareCommandForCancellation(cmd)
	err := runCancelableCommand(ctx, cmd)
	text := strings.TrimSpace(redactSensitive(captured.String(), []string{secret, url.PathEscape(secret)}))
	if ctx.Err() != nil {
		return text, ctx.Err()
	}
	if err != nil {
		if text != "" {
			return text, fmt.Errorf("git %s failed: %s: %w", args[0], text, err)
		}
		return text, fmt.Errorf("git %s failed: %w", args[0], err)
	}
	return text, nil
}
