package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseStepCoverageFromFileAndErrors(t *testing.T) {
	tmp := t.TempDir()

	goCovRel := "reports/cover.out"
	goCovPath := filepath.Join(tmp, filepath.FromSlash(goCovRel))
	if err := os.MkdirAll(filepath.Dir(goCovPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(goCovPath, []byte("mode: set\npkg/a.go:1.1,1.2 1 1\n"), 0o644); err != nil {
		t.Fatalf("write go coverage file: %v", err)
	}

	got, err := parseStepCoverageFromFile(tmp, stepMarkerMeta{coverageReport: goCovRel})
	if err != nil {
		t.Fatalf("parseStepCoverageFromFile go: %v", err)
	}
	if got == nil || got.Format != "go-coverprofile" || got.TotalStatements != 1 || got.CoveredStatements != 1 {
		t.Fatalf("unexpected go coverage result: %+v", got)
	}

	lcovRel := "reports/coverage.info"
	lcovPath := filepath.Join(tmp, filepath.FromSlash(lcovRel))
	if err := os.WriteFile(lcovPath, []byte("SF:src/a.go\nDA:1,1\nDA:2,0\nend_of_record\n"), 0o644); err != nil {
		t.Fatalf("write lcov file: %v", err)
	}
	got, err = parseStepCoverageFromFile(tmp, stepMarkerMeta{coverageReport: lcovRel, coverageFormat: "lcov"})
	if err != nil {
		t.Fatalf("parseStepCoverageFromFile lcov: %v", err)
	}
	if got == nil || got.Format != "lcov" || got.TotalLines != 2 || got.CoveredLines != 1 {
		t.Fatalf("unexpected lcov coverage result: %+v", got)
	}

	got, err = parseStepCoverageFromFile(tmp, stepMarkerMeta{})
	if err != nil {
		t.Fatalf("empty coverage marker should not error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil report for empty marker, got %+v", got)
	}

	_, err = parseStepCoverageFromFile(tmp, stepMarkerMeta{coverageReport: "missing.out"})
	if err == nil {
		t.Fatalf("expected missing report file error")
	}

	badFmtRel := "reports/bad.out"
	badFmtPath := filepath.Join(tmp, filepath.FromSlash(badFmtRel))
	if err := os.WriteFile(badFmtPath, []byte("ignored"), 0o644); err != nil {
		t.Fatalf("write bad format file: %v", err)
	}
	_, err = parseStepCoverageFromFile(tmp, stepMarkerMeta{coverageReport: badFmtRel, coverageFormat: "x-unknown"})
	if err == nil {
		t.Fatalf("expected unsupported format error")
	}
}

func TestCoverageParsersInvalidInputs(t *testing.T) {
	if _, err := parseGoCoverprofileCoverage([]string{"mode: set", "bad-line"}); err == nil {
		t.Fatalf("expected invalid go coverprofile line error")
	}
	if _, err := parseGoCoverprofileCoverage([]string{"pkg/a.go:1.1,2.2 X 1"}); err == nil {
		t.Fatalf("expected invalid go statement count error")
	}
	if _, err := parseGoCoverprofileCoverage([]string{"pkg/a.go:1.1,2.2 1 X"}); err == nil {
		t.Fatalf("expected invalid go hit count error")
	}

	if _, err := parseLCOVCoverage([]string{"SF:", "end_of_record"}); err == nil {
		t.Fatalf("expected invalid lcov SF error")
	}
	if _, err := parseLCOVCoverage([]string{"SF:src/a.go", "LF:X", "end_of_record"}); err == nil {
		t.Fatalf("expected invalid lcov LF error")
	}
	if _, err := parseLCOVCoverage([]string{"SF:src/a.go", "LH:X", "end_of_record"}); err == nil {
		t.Fatalf("expected invalid lcov LH error")
	}
	if _, err := parseLCOVCoverage([]string{"SF:src/a.go", "DA:1", "end_of_record"}); err == nil {
		t.Fatalf("expected invalid lcov DA error")
	}
	if _, err := parseLCOVCoverage([]string{"SF:src/a.go", "DA:1,X", "end_of_record"}); err == nil {
		t.Fatalf("expected invalid lcov DA hits error")
	}
}
