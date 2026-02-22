package agent

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/izzyreal/ciwi/internal/protocol"
)

const (
	artifactLogLevelNone              = "none"
	artifactLogLevelSummary           = "summary"
	artifactLogLevelVerbose           = "verbose"
	defaultArtifactLogLevel           = artifactLogLevelSummary
	defaultArtifactLogMaxIncludeLines = 25
)

type collectedArtifact struct {
	Path    string
	Content []byte
}

func collectAndUploadArtifacts(ctx context.Context, client *http.Client, serverURL, agentID, jobID, execDir string, globs []string, progress func(string)) (string, error) {
	artifacts, summary, err := collectArtifactsWithProgress(execDir, globs, progress)
	if err != nil {
		return summary, err
	}
	if len(artifacts) == 0 {
		return summary, nil
	}
	if progress != nil {
		progress(fmt.Sprintf("[artifacts] collected=%d bytes=%d", len(artifacts), totalCollectedArtifactBytes(artifacts)))
		progress(fmt.Sprintf("[artifacts] uploading=%d mode=zip", len(artifacts)))
	}

	if err := uploadArtifactsZIP(ctx, client, serverURL, agentID, jobID, artifacts); err == nil {
		if progress != nil {
			progress("[artifacts] upload complete")
		}
		return summary + "\n[artifacts] uploaded", nil
	}
	if progress != nil {
		progress("[artifacts] zip upload failed; falling back to legacy upload")
		progress(fmt.Sprintf("[artifacts] uploading=%d mode=legacy", len(artifacts)))
	}
	if err := uploadArtifactsLegacy(ctx, client, serverURL, agentID, jobID, artifacts); err != nil {
		return summary, err
	}
	if progress != nil {
		progress("[artifacts] upload complete")
	}

	return summary + "\n[artifacts] uploaded", nil
}

func uploadArtifactsLegacy(ctx context.Context, client *http.Client, serverURL, agentID, jobID string, artifacts []collectedArtifact) error {
	reqBody := protocol.UploadArtifactsRequest{
		AgentID:   agentID,
		Artifacts: toLegacyUploadArtifacts(artifacts),
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal artifact upload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL+"/api/v1/jobs/"+jobID+"/artifacts", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create artifact upload request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send artifact upload: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024))
		return fmt.Errorf("artifact upload rejected: status=%d body=%s", resp.StatusCode, bytes.TrimSpace(respBody))
	}
	return nil
}

func uploadArtifactsZIP(ctx context.Context, client *http.Client, serverURL, agentID, jobID string, artifacts []collectedArtifact) error {
	var payload bytes.Buffer
	zw := zip.NewWriter(&payload)
	for _, a := range artifacts {
		rel := filepath.ToSlash(filepath.Clean(strings.TrimSpace(a.Path)))
		if rel == "" || rel == "." || strings.HasPrefix(rel, "/") || rel == ".." || strings.HasPrefix(rel, "../") || strings.Contains(rel, "/../") {
			continue
		}
		entry, err := zw.Create(rel)
		if err != nil {
			_ = zw.Close()
			return fmt.Errorf("create zip entry %q: %w", rel, err)
		}
		if _, err := entry.Write(a.Content); err != nil {
			_ = zw.Close()
			return fmt.Errorf("write zip entry %q: %w", rel, err)
		}
	}
	if err := zw.Close(); err != nil {
		return fmt.Errorf("finalize artifact zip: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL+"/api/v1/jobs/"+jobID+"/artifacts/upload-zip", bytes.NewReader(payload.Bytes()))
	if err != nil {
		return fmt.Errorf("create zip artifact upload request: %w", err)
	}
	req.Header.Set("Content-Type", "application/zip")
	req.Header.Set("X-CIWI-Agent-ID", agentID)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send zip artifact upload: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024))
		return fmt.Errorf("zip artifact upload rejected: status=%d body=%s", resp.StatusCode, bytes.TrimSpace(respBody))
	}
	return nil
}

func toLegacyUploadArtifacts(in []collectedArtifact) []protocol.UploadArtifact {
	out := make([]protocol.UploadArtifact, 0, len(in))
	for _, a := range in {
		out = append(out, protocol.UploadArtifact{
			Path:       a.Path,
			DataBase64: base64.StdEncoding.EncodeToString(a.Content),
		})
	}
	return out
}

func totalCollectedArtifactBytes(in []collectedArtifact) int64 {
	var total int64
	for _, a := range in {
		total += int64(len(a.Content))
	}
	return total
}

func collectArtifacts(execDir string, globs []string) ([]collectedArtifact, string, error) {
	return collectArtifactsWithProgress(execDir, globs, nil)
}

func collectArtifactsWithProgress(execDir string, globs []string, progress func(string)) ([]collectedArtifact, string, error) {
	if len(globs) == 0 {
		return nil, "", nil
	}

	var summary strings.Builder
	fmt.Fprintf(&summary, "[artifacts] globs=%s\n", strings.Join(globs, ", "))
	logLevel, includeLineLimit := artifactLogConfigFromEnv()

	seen := map[string]struct{}{}
	matched := make([]string, 0)
	for _, pattern := range globs {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		ms, err := doublestar.Glob(os.DirFS(execDir), pattern)
		if err != nil {
			fmt.Fprintf(&summary, "[artifacts] invalid_glob=%q err=%v\n", pattern, err)
			continue
		}
		for _, m := range ms {
			m = filepath.ToSlash(filepath.Clean(m))
			if m == "." || strings.HasPrefix(m, "../") || strings.HasPrefix(m, "/") {
				continue
			}
			if _, ok := seen[m]; ok {
				continue
			}
			seen[m] = struct{}{}
			matched = append(matched, m)
		}
	}
	sort.Strings(matched)

	uploads := make([]collectedArtifact, 0)
	var (
		includeLines      int
		includeSuppressed int
		skipped           int
		totalBytes        int64
		scanned           int
	)
	for _, rel := range matched {
		if len(uploads) >= maxArtifactsPerJob {
			fmt.Fprintf(&summary, "[artifacts] cap_reached=%d\n", maxArtifactsPerJob)
			break
		}
		full := filepath.Join(execDir, filepath.FromSlash(rel))
		info, err := os.Stat(full)
		if err != nil {
			skipped++
			fmt.Fprintf(&summary, "[artifacts] skip=%s err=%v\n", rel, err)
			continue
		}
		if info.IsDir() {
			continue
		}
		scanned++
		if info.Size() > maxArtifactFileBytes {
			skipped++
			fmt.Fprintf(&summary, "[artifacts] skip=%s reason=size(%d>%d)\n", rel, info.Size(), maxArtifactFileBytes)
			continue
		}

		content, err := os.ReadFile(full)
		if err != nil {
			skipped++
			fmt.Fprintf(&summary, "[artifacts] skip=%s err=%v\n", rel, err)
			continue
		}
		uploads = append(uploads, collectedArtifact{Path: rel, Content: content})
		totalBytes += int64(len(content))
		if progress != nil && scanned%250 == 0 {
			progress(fmt.Sprintf("[artifacts] collecting scanned=%d included=%d bytes=%d", scanned, len(uploads), totalBytes))
		}
		if logLevel == artifactLogLevelVerbose {
			if includeLines < includeLineLimit {
				fmt.Fprintf(&summary, "[artifacts] include=%s size=%d\n", rel, len(content))
				includeLines++
			} else {
				includeSuppressed++
			}
		}
	}

	if len(uploads) == 0 {
		fmt.Fprintf(&summary, "[artifacts] none\n")
		return uploads, summary.String(), nil
	}
	if logLevel == artifactLogLevelVerbose && includeSuppressed > 0 {
		fmt.Fprintf(&summary, "[artifacts] includes_truncated=%d shown=%d total=%d\n", includeSuppressed, includeLines, len(uploads))
	}
	if logLevel != artifactLogLevelNone {
		fmt.Fprintf(&summary, "[artifacts] included=%d bytes=%d skipped=%d\n", len(uploads), totalBytes, skipped)
	}
	return uploads, summary.String(), nil
}

func artifactLogConfigFromEnv() (level string, includeLineLimit int) {
	level = strings.ToLower(strings.TrimSpace(envOrDefault("CIWI_ARTIFACT_LOG_LEVEL", defaultArtifactLogLevel)))
	switch level {
	case artifactLogLevelNone, artifactLogLevelSummary, artifactLogLevelVerbose:
	default:
		level = defaultArtifactLogLevel
	}
	includeLineLimit = defaultArtifactLogMaxIncludeLines
	if raw := strings.TrimSpace(envOrDefault("CIWI_ARTIFACT_LOG_MAX_INCLUDE_LINES", "")); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v >= 0 {
			includeLineLimit = v
		}
	}
	return level, includeLineLimit
}
