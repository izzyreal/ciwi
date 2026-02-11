package agent

import (
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
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/izzyreal/ciwi/internal/protocol"
)

func collectAndUploadArtifacts(ctx context.Context, client *http.Client, serverURL, agentID, jobID, execDir string, globs []string, progress func(string)) (string, error) {
	artifacts, summary, err := collectArtifacts(execDir, globs)
	if err != nil {
		return summary, err
	}
	if len(artifacts) == 0 {
		return summary, nil
	}
	if progress != nil {
		progress(fmt.Sprintf("[artifacts] uploading=%d", len(artifacts)))
	}

	reqBody := protocol.UploadArtifactsRequest{AgentID: agentID, Artifacts: artifacts}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return summary, fmt.Errorf("marshal artifact upload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL+"/api/v1/jobs/"+jobID+"/artifacts", bytes.NewReader(body))
	if err != nil {
		return summary, fmt.Errorf("create artifact upload request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return summary, fmt.Errorf("send artifact upload: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024))
		return summary, fmt.Errorf("artifact upload rejected: status=%d body=%s", resp.StatusCode, bytes.TrimSpace(respBody))
	}
	if progress != nil {
		progress("[artifacts] upload complete")
	}

	return summary + "\n[artifacts] uploaded", nil
}

func collectArtifacts(execDir string, globs []string) ([]protocol.UploadArtifact, string, error) {
	if len(globs) == 0 {
		return nil, "", nil
	}

	var summary strings.Builder
	fmt.Fprintf(&summary, "[artifacts] globs=%s\n", strings.Join(globs, ", "))

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

	uploads := make([]protocol.UploadArtifact, 0)
	for _, rel := range matched {
		if len(uploads) >= maxArtifactsPerJob {
			fmt.Fprintf(&summary, "[artifacts] cap_reached=%d\n", maxArtifactsPerJob)
			break
		}
		full := filepath.Join(execDir, filepath.FromSlash(rel))
		info, err := os.Stat(full)
		if err != nil {
			fmt.Fprintf(&summary, "[artifacts] skip=%s err=%v\n", rel, err)
			continue
		}
		if info.IsDir() {
			continue
		}
		if info.Size() > maxArtifactFileBytes {
			fmt.Fprintf(&summary, "[artifacts] skip=%s reason=size(%d>%d)\n", rel, info.Size(), maxArtifactFileBytes)
			continue
		}

		content, err := os.ReadFile(full)
		if err != nil {
			fmt.Fprintf(&summary, "[artifacts] skip=%s err=%v\n", rel, err)
			continue
		}
		uploads = append(uploads, protocol.UploadArtifact{Path: rel, DataBase64: base64.StdEncoding.EncodeToString(content)})
		fmt.Fprintf(&summary, "[artifacts] include=%s size=%d\n", rel, len(content))
	}

	if len(uploads) == 0 {
		fmt.Fprintf(&summary, "[artifacts] none\n")
	}
	return uploads, summary.String(), nil
}
