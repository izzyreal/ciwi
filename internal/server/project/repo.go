package project

import (
	"context"
	"fmt"
	"net/http"
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

	ref := strings.TrimSpace(repoRef)
	if ref == "" {
		ref = "HEAD"
	}

	if out, err := runCmd(ctx, "", "git", "-C", tmpDir, "fetch", "-q", "--depth", "1", "origin", ref); err != nil {
		return RepoFetchResult{}, fmt.Errorf("git fetch failed: %v\n%s", err, out)
	}

	out, err := runCmd(ctx, "", "git", "-C", tmpDir, "show", "FETCH_HEAD:"+configFile)
	if err != nil {
		return RepoFetchResult{}, fmt.Errorf("repo is not a valid ciwi project: missing root file %q", configFile)
	}
	shaOut, err := runCmd(ctx, "", "git", "-C", tmpDir, "rev-parse", "FETCH_HEAD")
	if err != nil {
		return RepoFetchResult{}, fmt.Errorf("resolve source commit for fetched config: %v", err)
	}

	iconType, iconBytes := fetchProjectIconBytes(ctx, tmpDir)
	return RepoFetchResult{
		ConfigContent:    out,
		IconContentType:  iconType,
		IconContentBytes: iconBytes,
		SourceCommit:     strings.TrimSpace(shaOut),
	}, nil
}

func runCmd(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func runCmdBytes(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
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
