package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func collectJobCacheStats(caches []resolvedJobCache) []protocol.JobCacheStats {
	if len(caches) == 0 {
		return nil
	}
	out := make([]protocol.JobCacheStats, 0, len(caches))
	for _, cache := range caches {
		stat := protocol.JobCacheStats{
			ID:     strings.TrimSpace(cache.ID),
			Env:    strings.TrimSpace(cache.Env),
			Path:   filepath.ToSlash(strings.TrimSpace(cache.Path)),
			Source: strings.TrimSpace(cache.Source),
			Type:   inferCacheType(cache),
		}
		files, dirs, size, err := summarizeDir(cache.Path)
		if err != nil {
			stat.Error = err.Error()
		} else {
			stat.Files = files
			stat.Directories = dirs
			stat.SizeBytes = size
		}
		if stat.Type == "ccache" {
			stat.ToolMetrics = readCCacheMetrics(cache.Path)
		}
		out = append(out, stat)
	}
	return out
}

func inferCacheType(cache resolvedJobCache) string {
	id := strings.ToLower(strings.TrimSpace(cache.ID))
	env := strings.ToLower(strings.TrimSpace(cache.Env))
	switch {
	case strings.Contains(id, "ccache") || strings.Contains(env, "ccache"):
		return "ccache"
	case strings.Contains(id, "fetchcontent") || strings.Contains(env, "fetchcontent"):
		return "fetchcontent"
	default:
		return "generic"
	}
}

func summarizeDir(root string) (files int64, dirs int64, size int64, err error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return 0, 0, 0, os.ErrNotExist
	}
	if _, statErr := os.Stat(root); statErr != nil {
		return 0, 0, 0, statErr
	}
	walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		if d.IsDir() {
			dirs++
			return nil
		}
		files++
		info, infoErr := d.Info()
		if infoErr == nil {
			size += info.Size()
		}
		return nil
	})
	if walkErr != nil {
		return files, dirs, size, walkErr
	}
	return files, dirs, size, nil
}

func readCCacheMetrics(cacheDir string) map[string]string {
	if strings.TrimSpace(cacheDir) == "" {
		return nil
	}
	if _, err := exec.LookPath("ccache"); err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	out, err := runCommandCapture(ctx, "", "ccache", "--dir", cacheDir, "--show-stats", "--verbose")
	if err != nil {
		return nil
	}
	metrics := map[string]string{}
	for _, raw := range strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		if key == "" || val == "" {
			continue
		}
		metrics[key] = val
	}
	if len(metrics) == 0 {
		return nil
	}
	return metrics
}
