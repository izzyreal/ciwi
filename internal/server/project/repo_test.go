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

func TestShouldRetryFetchWithoutGitConfig(t *testing.T) {
	tests := []struct {
		name      string
		repoURL   string
		output    string
		wantRetry bool
	}{
		{
			name:      "github https with publickey failure",
			repoURL:   "https://github.com/izzyreal/ciwi",
			output:    "Permission denied (publickey)",
			wantRetry: true,
		},
		{
			name:      "github https with ssh signing failure",
			repoURL:   "https://github.com/izzyreal/ciwi",
			output:    `sign_and_send_pubkey: signing failed for RSA "SSH Key for GitHub" from agent: communication with agent failed`,
			wantRetry: true,
		},
		{
			name:      "github https different fetch failure",
			repoURL:   "https://github.com/izzyreal/ciwi",
			output:    "fatal: could not find remote ref no-such-branch",
			wantRetry: false,
		},
		{
			name:      "ssh url should not retry",
			repoURL:   "git@github.com:izzyreal/ciwi.git",
			output:    "Permission denied (publickey)",
			wantRetry: false,
		},
		{
			name:      "other host should not retry",
			repoURL:   "https://example.com/repo.git",
			output:    "Permission denied (publickey)",
			wantRetry: false,
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := shouldRetryFetchWithoutGitConfig(tc.repoURL, tc.output)
			if got != tc.wantRetry {
				t.Fatalf("shouldRetryFetchWithoutGitConfig(%q, %q) = %v, want %v", tc.repoURL, tc.output, got, tc.wantRetry)
			}
		})
	}
}

func TestShouldPreferFetchWithoutGitConfig(t *testing.T) {
	tests := []struct {
		repoURL string
		want    bool
	}{
		{repoURL: "https://github.com/izzyreal/ciwi", want: true},
		{repoURL: "HTTPS://GITHUB.COM/izzyreal/ciwi", want: true},
		{repoURL: "http://github.com/izzyreal/ciwi", want: false},
		{repoURL: "git@github.com:izzyreal/ciwi.git", want: false},
		{repoURL: "https://gitlab.com/izzyreal/ciwi", want: false},
	}
	for _, tc := range tests {
		if got := shouldPreferFetchWithoutGitConfig(tc.repoURL); got != tc.want {
			t.Fatalf("shouldPreferFetchWithoutGitConfig(%q)=%v, want %v", tc.repoURL, got, tc.want)
		}
	}
}

func TestShouldFallbackToDefaultGitConfig(t *testing.T) {
	tests := []struct {
		output string
		want   bool
	}{
		{output: "fatal: Authentication failed for 'https://github.com/org/repo'", want: true},
		{output: "fatal: could not read Username for 'https://github.com': terminal prompts disabled", want: true},
		{output: "fatal: remote error: upload-pack: not our ref deadbeef", want: false},
	}
	for _, tc := range tests {
		if got := shouldFallbackToDefaultGitConfig(tc.output); got != tc.want {
			t.Fatalf("shouldFallbackToDefaultGitConfig(%q)=%v, want %v", tc.output, got, tc.want)
		}
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
