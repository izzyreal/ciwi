package project

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type RepoFetchResult struct {
	ConfigContent    string
	IconContentType  string
	IconContentBytes []byte
	SourceCommit     string
	ResolvedRef      string
}

func FetchConfigFileFromRepo(ctx context.Context, tmpDir, repoURL, repoRef, configFile string) (string, error) {
	res, err := FetchConfigAndIconFromRepo(ctx, tmpDir, repoURL, repoRef, configFile)
	if err != nil {
		return "", err
	}
	return res.ConfigContent, nil
}

func FetchConfigAndIconFromRepo(ctx context.Context, tmpDir, repoURL, repoRef, configFile string) (RepoFetchResult, error) {
	if out, err := runCmd(ctx, "", "git", "init", "-q", tmpDir); err != nil {
		return RepoFetchResult{}, fmt.Errorf("git init failed: %v\n%s", err, out)
	}
	if out, err := runCmd(ctx, "", "git", "-C", tmpDir, "remote", "add", "origin", repoURL); err != nil {
		return RepoFetchResult{}, fmt.Errorf("git remote add failed: %v\n%s", err, out)
	}

	requestedRef := strings.TrimSpace(repoRef)
	ref := requestedRef
	if ref == "" {
		ref = "HEAD"
	}

	fetchArgs := []string{"-C", tmpDir, "fetch", "-q", "--depth", "1", "origin", ref}
	if out, err := runGitFetchWithFallback(ctx, repoURL, fetchArgs...); err != nil {
		return RepoFetchResult{}, fmt.Errorf("git fetch failed: %v\n%s", err, out)
	}
	configOut, err := runCmd(ctx, "", "git", "-C", tmpDir, "show", "FETCH_HEAD:"+configFile)
	if err != nil {
		return RepoFetchResult{}, fmt.Errorf("repo is not a valid ciwi project: missing root file %q", configFile)
	}
	shaOut, err := runCmd(ctx, "", "git", "-C", tmpDir, "rev-parse", "FETCH_HEAD")
	if err != nil {
		return RepoFetchResult{}, fmt.Errorf("resolve source commit for fetched config: %v", err)
	}

	iconType, iconBytes := fetchProjectIconBytes(ctx, tmpDir)
	resolvedRef := requestedRef
	if resolvedRef == "" {
		resolvedRef = resolveDefaultBranchFromRemoteHead(ctx, tmpDir)
	}
	return RepoFetchResult{
		ConfigContent:    configOut,
		IconContentType:  iconType,
		IconContentBytes: iconBytes,
		SourceCommit:     strings.TrimSpace(shaOut),
		ResolvedRef:      strings.TrimSpace(resolvedRef),
	}, nil
}

func runGitFetchWithFallback(ctx context.Context, repoURL string, fetchArgs ...string) (string, error) {
	if shouldPreferFetchWithoutGitConfig(repoURL) {
		out, err := runCmdWithEnv(ctx, "", gitFetchNoConfigEnv(), "git", fetchArgs...)
		if err == nil {
			return out, nil
		}
		if shouldFallbackToDefaultGitConfig(out) {
			return runCmd(ctx, "", "git", fetchArgs...)
		}
		return out, err
	}

	out, err := runCmd(ctx, "", "git", fetchArgs...)
	if err != nil && shouldRetryFetchWithoutGitConfig(repoURL, out) {
		retryOut, retryErr := runCmdWithEnv(ctx, "", gitFetchNoConfigEnv(), "git", fetchArgs...)
		if retryErr == nil {
			return retryOut, nil
		}
	}
	return out, err
}

func resolveDefaultBranchFromRemoteHead(ctx context.Context, tmpDir string) string {
	out, err := runCmd(ctx, "", "git", "-C", tmpDir, "ls-remote", "--symref", "origin", "HEAD")
	if err != nil {
		return ""
	}
	for _, raw := range strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if !strings.HasPrefix(line, "ref: ") || !strings.HasSuffix(line, "\tHEAD") {
			continue
		}
		ref := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "ref: "), "\tHEAD"))
		const headsPrefix = "refs/heads/"
		if strings.HasPrefix(ref, headsPrefix) {
			return strings.TrimSpace(strings.TrimPrefix(ref, headsPrefix))
		}
	}
	return ""
}

func runCmd(ctx context.Context, dir, name string, args ...string) (string, error) {
	return runCmdWithEnv(ctx, dir, nil, name, args...)
}

func runCmdWithEnv(ctx context.Context, dir string, env map[string]string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	if len(env) > 0 {
		cmd.Env = mergeEnvWithOverrides(env)
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func runCmdBytes(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}

func gitFetchNoConfigEnv() map[string]string {
	return map[string]string{
		"GIT_CONFIG_GLOBAL":   "/dev/null",
		"GIT_CONFIG_SYSTEM":   "/dev/null",
		"GIT_TERMINAL_PROMPT": "0",
	}
}

func mergeEnvWithOverrides(overrides map[string]string) []string {
	env := map[string]string{}
	for _, kv := range os.Environ() {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) != 2 {
			continue
		}
		env[parts[0]] = parts[1]
	}
	for k, v := range overrides {
		env[k] = v
	}
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	sort.Strings(out)
	return out
}

func shouldRetryFetchWithoutGitConfig(repoURL, fetchOutput string) bool {
	url := strings.ToLower(strings.TrimSpace(repoURL))
	if !strings.HasPrefix(url, "https://github.com/") {
		return false
	}
	text := strings.ToLower(fetchOutput)
	return strings.Contains(text, "permission denied (publickey)") ||
		strings.Contains(text, "sign_and_send_pubkey") ||
		strings.Contains(text, "could not read from remote repository")
}

func shouldPreferFetchWithoutGitConfig(repoURL string) bool {
	url := strings.ToLower(strings.TrimSpace(repoURL))
	return strings.HasPrefix(url, "https://github.com/")
}

func shouldFallbackToDefaultGitConfig(fetchOutput string) bool {
	text := strings.ToLower(fetchOutput)
	return strings.Contains(text, "authentication failed") ||
		strings.Contains(text, "could not read username") ||
		strings.Contains(text, "terminal prompts disabled")
}

func fetchProjectIconBytes(ctx context.Context, tmpDir string) (string, []byte) {
	list, err := runCmd(ctx, "", "git", "-C", tmpDir, "ls-tree", "-r", "--name-only", "FETCH_HEAD")
	if err != nil {
		return "", nil
	}
	type candidate struct {
		path  string
		size  int64
		depth int
	}
	candidates := make([]candidate, 0)
	for _, line := range strings.Split(strings.ReplaceAll(list, "\r\n", "\n"), "\n") {
		path := strings.TrimSpace(line)
		if path == "" {
			continue
		}
		base := strings.ToLower(filepath.Base(path))
		if !strings.Contains(base, "icon") && !strings.Contains(base, "logo") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(base))
		if ext != ".png" && ext != ".jpg" && ext != ".jpeg" && ext != ".bmp" {
			continue
		}
		sizeOut, sizeErr := runCmd(ctx, "", "git", "-C", tmpDir, "cat-file", "-s", "FETCH_HEAD:"+path)
		if sizeErr != nil {
			continue
		}
		size, parseErr := strconv.ParseInt(strings.TrimSpace(sizeOut), 10, 64)
		if parseErr != nil || size <= 0 || size > 500*1024 {
			continue
		}
		depth := strings.Count(strings.ReplaceAll(path, "\\", "/"), "/")
		candidates = append(candidates, candidate{path: path, size: size, depth: depth})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].size != candidates[j].size {
			return candidates[i].size > candidates[j].size
		}
		if candidates[i].depth != candidates[j].depth {
			return candidates[i].depth < candidates[j].depth
		}
		return candidates[i].path < candidates[j].path
	})
	for _, c := range candidates {
		raw, readErr := runCmdBytes(ctx, "", "git", "-C", tmpDir, "show", "FETCH_HEAD:"+c.path)
		if readErr != nil || len(raw) == 0 {
			continue
		}
		mime := strings.ToLower(strings.TrimSpace(http.DetectContentType(raw)))
		if strings.HasPrefix(mime, "image/png") {
			return "image/png", raw
		}
		if strings.HasPrefix(mime, "image/jpeg") {
			return "image/jpeg", raw
		}
		if strings.HasPrefix(mime, "image/bmp") || strings.HasPrefix(mime, "image/x-ms-bmp") {
			return "image/bmp", raw
		}
	}
	return "", nil
}
