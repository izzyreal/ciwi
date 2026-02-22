package agent

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func downloadDependencyArtifacts(ctx context.Context, client *http.Client, serverURL, jobID, execDir string) (string, error) {
	var summary strings.Builder
	if strings.TrimSpace(serverURL) == "" {
		return "", fmt.Errorf("server url is required")
	}
	if strings.TrimSpace(jobID) == "" {
		return "", fmt.Errorf("dependency job id is required")
	}
	if strings.TrimSpace(execDir) == "" {
		return "", fmt.Errorf("exec dir is required")
	}

	if zipSummary, err := downloadDependencyArtifactsZIP(ctx, client, serverURL, jobID, execDir); err == nil {
		return zipSummary, nil
	} else {
		fmt.Fprintf(&summary, "[dep-artifacts] zip_fallback job=%s reason=%v\n", jobID, err)
	}

	artifactsURL := strings.TrimRight(serverURL, "/") + "/api/v1/jobs/" + jobID + "/artifacts"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, artifactsURL, nil)
	if err != nil {
		return "", fmt.Errorf("create artifacts request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request artifact list: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("artifact list rejected: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload protocol.JobExecutionArtifactsResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("decode artifact list: %w", err)
	}
	if len(payload.Artifacts) == 0 {
		fmt.Fprintf(&summary, "[dep-artifacts] none from job=%s", jobID)
		return summary.String(), nil
	}
	fmt.Fprintf(&summary, "[dep-artifacts] downloading=%d from job=%s\n", len(payload.Artifacts), jobID)
	for _, a := range payload.Artifacts {
		rel, err := safeDependencyArtifactPath(a.Path)
		if err != nil {
			fmt.Fprintf(&summary, "[dep-artifacts] skip=%s reason=%v\n", a.Path, err)
			continue
		}
		fileURL, err := resolveDependencyArtifactURL(serverURL, a.URL)
		if err != nil {
			return summary.String(), fmt.Errorf("resolve artifact url for %q: %w", a.Path, err)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
		if err != nil {
			return summary.String(), fmt.Errorf("create artifact request for %q: %w", a.Path, err)
		}
		resp, err := client.Do(req)
		if err != nil {
			return summary.String(), fmt.Errorf("download artifact %q: %w", a.Path, err)
		}
		content, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return summary.String(), fmt.Errorf("download artifact %q rejected: status=%d body=%s", a.Path, resp.StatusCode, strings.TrimSpace(string(content)))
		}
		if readErr != nil {
			return summary.String(), fmt.Errorf("read artifact %q: %w", a.Path, readErr)
		}
		if err := writeDependencyArtifact(execDir, rel, content); err != nil {
			return summary.String(), fmt.Errorf("write artifact %q: %w", a.Path, err)
		}
		fmt.Fprintf(&summary, "[dep-artifacts] restored=%s bytes=%d\n", rel, len(content))
	}
	return strings.TrimSuffix(summary.String(), "\n"), nil
}

func downloadDependencyArtifactsZIP(ctx context.Context, client *http.Client, serverURL, jobID, execDir string) (string, error) {
	var summary strings.Builder
	zipURL := strings.TrimRight(serverURL, "/") + "/api/v1/jobs/" + jobID + "/artifacts/download-all"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, zipURL, nil)
	if err != nil {
		return "", fmt.Errorf("create zip request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download zip: %w", err)
	}
	defer resp.Body.Close()
	payload, readErr := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("zip rejected: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}
	if readErr != nil {
		return "", fmt.Errorf("read zip: %w", readErr)
	}
	zr, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		return "", fmt.Errorf("open zip: %w", err)
	}
	fmt.Fprintf(&summary, "[dep-artifacts] zip_entries=%d from job=%s\n", len(zr.File), jobID)
	restored := 0
	for _, zf := range zr.File {
		if zf.FileInfo().IsDir() {
			continue
		}
		rel, err := safeDependencyArtifactPath(zf.Name)
		if err != nil {
			fmt.Fprintf(&summary, "[dep-artifacts] skip=%s reason=%v\n", zf.Name, err)
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			return summary.String(), fmt.Errorf("open zip entry %q: %w", zf.Name, err)
		}
		content, readErr := io.ReadAll(rc)
		closeErr := rc.Close()
		if readErr != nil {
			return summary.String(), fmt.Errorf("read zip entry %q: %w", zf.Name, readErr)
		}
		if closeErr != nil {
			return summary.String(), fmt.Errorf("close zip entry %q: %w", zf.Name, closeErr)
		}
		if err := writeDependencyArtifact(execDir, rel, content); err != nil {
			return summary.String(), fmt.Errorf("write artifact %q: %w", zf.Name, err)
		}
		fmt.Fprintf(&summary, "[dep-artifacts] restored=%s bytes=%d\n", rel, len(content))
		restored++
	}
	if restored == 0 {
		fmt.Fprintf(&summary, "[dep-artifacts] none from job=%s", jobID)
		return summary.String(), nil
	}
	return strings.TrimSuffix(summary.String(), "\n"), nil
}

func safeDependencyArtifactPath(path string) (string, error) {
	rel := filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
	if rel == "." || rel == "" {
		return "", fmt.Errorf("empty path")
	}
	if strings.HasPrefix(rel, "/") || strings.HasPrefix(rel, "../") || strings.Contains(rel, "/../") || rel == ".." {
		return "", fmt.Errorf("unsafe path")
	}
	return rel, nil
}

func resolveDependencyArtifactURL(serverURL, raw string) (string, error) {
	serverURL = strings.TrimSpace(serverURL)
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("empty url")
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return raw, nil
	}
	base, err := url.Parse(strings.TrimRight(serverURL, "/") + "/")
	if err != nil {
		return "", fmt.Errorf("parse server url: %w", err)
	}
	ref, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse artifact url: %w", err)
	}
	return base.ResolveReference(ref).String(), nil
}

func dependencyArtifactJobIDs(env map[string]string) []string {
	ids := make([]string, 0)
	seen := map[string]struct{}{}
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		if _, ok := seen[v]; ok {
			return
		}
		seen[v] = struct{}{}
		ids = append(ids, v)
	}
	for _, part := range strings.Split(strings.TrimSpace(env["CIWI_DEP_ARTIFACT_JOB_IDS"]), ",") {
		add(part)
	}
	add(env["CIWI_DEP_ARTIFACT_JOB_ID"])
	if len(ids) == 0 {
		return nil
	}
	return ids
}

func writeDependencyArtifact(execDir, rel string, content []byte) error {
	dst := filepath.Join(execDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, content, 0o644)
}
