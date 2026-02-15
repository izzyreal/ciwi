package agent

import "testing"

func TestParseGoCoverprofileCoverage(t *testing.T) {
	report, err := parseGoCoverprofileCoverage([]string{
		"mode: set",
		"pkg/a.go:1.1,2.2 2 1",
		"pkg/a.go:3.1,4.2 1 0",
		"pkg/b.go:1.1,2.2 3 1",
	})
	if err != nil {
		t.Fatalf("parse go coverprofile: %v", err)
	}
	if report.Format != "go-coverprofile" {
		t.Fatalf("unexpected format: %q", report.Format)
	}
	if report.TotalStatements != 6 || report.CoveredStatements != 5 {
		t.Fatalf("unexpected statement totals: %+v", report)
	}
	if len(report.Files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(report.Files))
	}
}

func TestParseLCOVCoverage(t *testing.T) {
	report, err := parseLCOVCoverage([]string{
		"TN:",
		"SF:src/a.go",
		"DA:1,1",
		"DA:2,0",
		"LF:2",
		"LH:1",
		"end_of_record",
		"SF:src/b.go",
		"DA:1,3",
		"DA:2,2",
		"end_of_record",
	})
	if err != nil {
		t.Fatalf("parse lcov: %v", err)
	}
	if report.Format != "lcov" {
		t.Fatalf("unexpected format: %q", report.Format)
	}
	if report.TotalLines != 4 || report.CoveredLines != 3 {
		t.Fatalf("unexpected line totals: %+v", report)
	}
	if len(report.Files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(report.Files))
	}
}
