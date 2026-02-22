package jobexecution

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/izzyreal/ciwi/internal/protocol"
)

const testReportArtifactPath = "test-report.json"
const coverageReportArtifactPath = "coverage-report.json"

func PersistArtifacts(artifactsDir, jobID string, incoming []protocol.UploadArtifact) ([]protocol.JobExecutionArtifact, error) {
	if len(incoming) == 0 {
		return nil, nil
	}
	base := filepath.Join(artifactsDir, jobID)
	if err := os.MkdirAll(base, 0o755); err != nil {
		return nil, fmt.Errorf("create artifact dir: %w", err)
	}

	artifacts := make([]protocol.JobExecutionArtifact, 0, len(incoming))
	for _, in := range incoming {
		rel := filepath.ToSlash(filepath.Clean(in.Path))
		if !isSafeStoredArtifactPath(rel) {
			return nil, fmt.Errorf("invalid artifact path: %q", in.Path)
		}

		decoded, err := base64.StdEncoding.DecodeString(in.DataBase64)
		if err != nil {
			return nil, fmt.Errorf("decode artifact %q: %w", in.Path, err)
		}

		dst := filepath.Join(base, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return nil, fmt.Errorf("mkdir artifact parent: %w", err)
		}
		if err := os.WriteFile(dst, decoded, 0o644); err != nil {
			return nil, fmt.Errorf("write artifact %q: %w", in.Path, err)
		}

		storedRel := filepath.ToSlash(filepath.Join(jobID, filepath.FromSlash(rel)))
		artifacts = append(artifacts, protocol.JobExecutionArtifact{
			JobExecutionID: jobID,
			Path:           rel,
			URL:            storedRel,
			SizeBytes:      int64(len(decoded)),
		})
	}
	return artifacts, nil
}

func PersistArtifactsZIP(artifactsDir, jobID string, payload []byte) ([]protocol.JobExecutionArtifact, error) {
	if len(payload) == 0 {
		return nil, nil
	}
	base := filepath.Join(artifactsDir, jobID)
	if err := os.MkdirAll(base, 0o755); err != nil {
		return nil, fmt.Errorf("create artifact dir: %w", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		return nil, fmt.Errorf("read artifact zip: %w", err)
	}
	artifacts := make([]protocol.JobExecutionArtifact, 0, len(zr.File))
	for _, zf := range zr.File {
		if zf.FileInfo().IsDir() {
			continue
		}
		rel := filepath.ToSlash(filepath.Clean(zf.Name))
		if !isSafeStoredArtifactPath(rel) {
			return nil, fmt.Errorf("invalid artifact path: %q", zf.Name)
		}
		rc, err := zf.Open()
		if err != nil {
			return nil, fmt.Errorf("open artifact %q from zip: %w", zf.Name, err)
		}
		content, readErr := io.ReadAll(rc)
		closeErr := rc.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read artifact %q from zip: %w", zf.Name, readErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close artifact %q from zip: %w", zf.Name, closeErr)
		}
		dst := filepath.Join(base, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return nil, fmt.Errorf("mkdir artifact parent: %w", err)
		}
		if err := os.WriteFile(dst, content, 0o644); err != nil {
			return nil, fmt.Errorf("write artifact %q: %w", zf.Name, err)
		}
		storedRel := filepath.ToSlash(filepath.Join(jobID, filepath.FromSlash(rel)))
		artifacts = append(artifacts, protocol.JobExecutionArtifact{
			JobExecutionID: jobID,
			Path:           rel,
			URL:            storedRel,
			SizeBytes:      int64(len(content)),
		})
	}
	return artifacts, nil
}

func isSafeStoredArtifactPath(rel string) bool {
	return !(rel == "." || rel == "" || strings.HasPrefix(rel, "/") || rel == ".." || strings.HasPrefix(rel, "../") || strings.Contains(rel, "/../"))
}

func PersistTestReportArtifact(artifactsDir, jobID string, report protocol.JobExecutionTestReport) error {
	base := filepath.Join(artifactsDir, jobID)
	if err := os.MkdirAll(base, 0o755); err != nil {
		return fmt.Errorf("create test report artifact dir: %w", err)
	}
	dst := filepath.Join(base, filepath.FromSlash(testReportArtifactPath))
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal test report artifact: %w", err)
	}
	if err := os.WriteFile(dst, payload, 0o644); err != nil {
		return fmt.Errorf("write test report artifact: %w", err)
	}
	return nil
}

func PersistCoverageReportArtifact(artifactsDir, jobID string, report protocol.JobExecutionTestReport) error {
	base := filepath.Join(artifactsDir, jobID)
	if err := os.MkdirAll(base, 0o755); err != nil {
		return fmt.Errorf("create coverage report artifact dir: %w", err)
	}
	dst := filepath.Join(base, filepath.FromSlash(coverageReportArtifactPath))
	if report.Coverage == nil {
		if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove coverage report artifact: %w", err)
		}
		return nil
	}
	payload, err := json.MarshalIndent(report.Coverage, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal coverage report artifact: %w", err)
	}
	if err := os.WriteFile(dst, payload, 0o644); err != nil {
		return fmt.Errorf("write coverage report artifact: %w", err)
	}
	return nil
}

func AppendSyntheticTestReportArtifact(artifactsDir, jobID string, artifacts []protocol.JobExecutionArtifact) []protocol.JobExecutionArtifact {
	testReportFull := filepath.Join(artifactsDir, jobID, filepath.FromSlash(testReportArtifactPath))
	info, err := os.Stat(testReportFull)
	if err != nil || info.IsDir() {
		return artifacts
	}
	for _, a := range artifacts {
		if a.Path == testReportArtifactPath {
			return artifacts
		}
	}
	artifacts = append(artifacts, protocol.JobExecutionArtifact{
		JobExecutionID: jobID,
		Path:           testReportArtifactPath,
		URL:            filepath.ToSlash(filepath.Join(jobID, filepath.FromSlash(testReportArtifactPath))),
		SizeBytes:      info.Size(),
	})
	sort.SliceStable(artifacts, func(i, j int) bool { return artifacts[i].Path < artifacts[j].Path })
	return artifacts
}

func AppendSyntheticCoverageReportArtifact(artifactsDir, jobID string, artifacts []protocol.JobExecutionArtifact) []protocol.JobExecutionArtifact {
	coverageReportFull := filepath.Join(artifactsDir, jobID, filepath.FromSlash(coverageReportArtifactPath))
	info, err := os.Stat(coverageReportFull)
	if err != nil || info.IsDir() {
		return artifacts
	}
	for _, a := range artifacts {
		if a.Path == coverageReportArtifactPath {
			return artifacts
		}
	}
	artifacts = append(artifacts, protocol.JobExecutionArtifact{
		JobExecutionID: jobID,
		Path:           coverageReportArtifactPath,
		URL:            filepath.ToSlash(filepath.Join(jobID, filepath.FromSlash(coverageReportArtifactPath))),
		SizeBytes:      info.Size(),
	})
	sort.SliceStable(artifacts, func(i, j int) bool { return artifacts[i].Path < artifacts[j].Path })
	return artifacts
}
