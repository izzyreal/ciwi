package project

import (
	"context"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestFetchConfigAndIconFromRepo(t *testing.T) {
	repoDir := initTestGitRepo(t, map[string]string{
		"ciwi-project.yaml": "version: 1\nproject:\n  name: test\npipelines: []\n",
		"assets/icon.png":   tinyPNGBase64(),
	}, true)

	tmpDir := t.TempDir()
	res, err := FetchConfigAndIconFromRepo(context.Background(), tmpDir, repoDir, "HEAD", "ciwi-project.yaml")
	if err != nil {
		t.Fatalf("FetchConfigAndIconFromRepo: %v", err)
	}
	if !strings.Contains(res.ConfigContent, "project:") {
		t.Fatalf("unexpected config content: %q", res.ConfigContent)
	}
	if strings.TrimSpace(res.SourceCommit) == "" {
		t.Fatalf("expected source commit")
	}
	if res.IconContentType != "image/png" {
		t.Fatalf("expected image/png icon type, got %q", res.IconContentType)
	}
	if len(res.IconContentBytes) == 0 {
		t.Fatalf("expected icon bytes")
	}
}

func TestFetchConfigFileFromRepoMissingConfig(t *testing.T) {
	repoDir := initTestGitRepo(t, map[string]string{
		"README.md": "hello",
	}, false)
	_, err := FetchConfigFileFromRepo(context.Background(), t.TempDir(), repoDir, "HEAD", "ciwi-project.yaml")
	if err == nil || !strings.Contains(err.Error(), "missing root file") {
		t.Fatalf("expected missing config error, got %v", err)
	}
}

func TestFetchProjectIconBytesNoCandidates(t *testing.T) {
	repoDir := initTestGitRepo(t, map[string]string{
		"ciwi-project.yaml": "version: 1\nproject:\n  name: test\npipelines: []\n",
	}, false)
	tmpDir := t.TempDir()
	_, err := runCmd(context.Background(), "", "git", "init", "-q", tmpDir)
	if err != nil {
		t.Fatalf("git init temp: %v", err)
	}
	_, err = runCmd(context.Background(), "", "git", "-C", tmpDir, "remote", "add", "origin", repoDir)
	if err != nil {
		t.Fatalf("git remote add: %v", err)
	}
	_, err = runCmd(context.Background(), "", "git", "-C", tmpDir, "fetch", "-q", "--depth", "1", "origin", "HEAD")
	if err != nil {
		t.Fatalf("git fetch: %v", err)
	}
	mime, raw := fetchProjectIconBytes(context.Background(), tmpDir)
	if mime != "" || len(raw) != 0 {
		t.Fatalf("expected no icon, got mime=%q bytes=%d", mime, len(raw))
	}
}

func TestFetchProjectIconBytesHandlesUnpreparedRepoState(t *testing.T) {
	tmpDir := t.TempDir()
	runGit(t, tmpDir, "init", "-q")
	// No FETCH_HEAD available in this repo state.
	mime, raw := fetchProjectIconBytes(context.Background(), tmpDir)
	if mime != "" || len(raw) != 0 {
		t.Fatalf("expected no icon for missing FETCH_HEAD, got mime=%q bytes=%d", mime, len(raw))
	}
}

func TestFetchConfigAndIconFromRepoWithEmptyRefAndBMPIcon(t *testing.T) {
	repoDir := initTestGitRepo(t, map[string]string{
		"ciwi-project.yaml": "version: 1\nproject:\n  name: test\npipelines: []\n",
		"assets/logo.bmp":   tinyBMPBase64(),
	}, true)

	tmpDir := t.TempDir()
	res, err := FetchConfigAndIconFromRepo(context.Background(), tmpDir, repoDir, "", "ciwi-project.yaml")
	if err != nil {
		t.Fatalf("FetchConfigAndIconFromRepo empty ref: %v", err)
	}
	if res.IconContentType != "image/bmp" {
		t.Fatalf("expected image/bmp icon type, got %q", res.IconContentType)
	}
	if len(res.IconContentBytes) == 0 {
		t.Fatalf("expected bmp icon bytes")
	}
}

func TestFetchConfigAndIconFromRepoFetchFailureForInvalidRemote(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := FetchConfigAndIconFromRepo(context.Background(), tmpDir, "", "HEAD", "ciwi-project.yaml")
	if err == nil || !strings.Contains(err.Error(), "git fetch failed") {
		t.Fatalf("expected git fetch failure, got %v", err)
	}
}

func initTestGitRepo(t *testing.T, files map[string]string, decodeBase64ForPNG bool) string {
	t.Helper()
	repoDir := t.TempDir()
	runGit(t, repoDir, "init", "-q")
	runGit(t, repoDir, "config", "user.email", "ciwi@test.local")
	runGit(t, repoDir, "config", "user.name", "ciwi-test")
	for rel, content := range files {
		full := filepath.Join(repoDir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %q: %v", full, err)
		}
		data := []byte(content)
		if decodeBase64ForPNG {
			lower := strings.ToLower(rel)
			isImage := strings.HasSuffix(lower, ".png") || strings.HasSuffix(lower, ".jpg") || strings.HasSuffix(lower, ".jpeg") || strings.HasSuffix(lower, ".bmp")
			if isImage {
				decoded, err := base64.StdEncoding.DecodeString(content)
				if err != nil {
					t.Fatalf("decode image base64 for %q: %v", rel, err)
				}
				data = decoded
			}
		}
		if err := os.WriteFile(full, data, 0o644); err != nil {
			t.Fatalf("write %q: %v", full, err)
		}
	}
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "init")
	return repoDir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	base := []string{"-c", "commit.gpgsign=false", "-C", dir}
	cmd := exec.Command("git", append(base, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
}

func tinyPNGBase64() string {
	// 1x1 transparent PNG
	return "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO5p8WQAAAAASUVORK5CYII="
}

func tinyBMPBase64() string {
	// 1x1 24-bit BMP
	return "Qk06AAAAAAAAADYAAAAoAAAAAQAAAAEAAAABABgAAAAAAAQAAAAAAAAAAAAAAAAAAAAAAAAA////AA=="
}
