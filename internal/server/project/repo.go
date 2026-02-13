package project

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

func FetchConfigFileFromRepo(ctx context.Context, tmpDir, repoURL, repoRef, configFile string) (string, error) {
	if out, err := runCmd(ctx, "", "git", "init", "-q", tmpDir); err != nil {
		return "", fmt.Errorf("git init failed: %v\n%s", err, out)
	}
	if out, err := runCmd(ctx, "", "git", "-C", tmpDir, "remote", "add", "origin", repoURL); err != nil {
		return "", fmt.Errorf("git remote add failed: %v\n%s", err, out)
	}

	ref := strings.TrimSpace(repoRef)
	if ref == "" {
		ref = "HEAD"
	}

	if out, err := runCmd(ctx, "", "git", "-C", tmpDir, "fetch", "-q", "--depth", "1", "origin", ref); err != nil {
		return "", fmt.Errorf("git fetch failed: %v\n%s", err, out)
	}

	out, err := runCmd(ctx, "", "git", "-C", tmpDir, "show", "FETCH_HEAD:"+configFile)
	if err != nil {
		return "", fmt.Errorf("repo is not a valid ciwi project: missing root file %q", configFile)
	}

	return out, nil
}

func runCmd(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}
