package agent

import (
	"strings"
	"testing"
)

func TestParseJUnitXMLSuiteWithCasesAndMessages(t *testing.T) {
	lines := []string{
		`<testsuite name="root-suite" package="pkg.root" tests="3" failures="1" errors="0" skipped="1">`,
		`  <testcase name="ok" classname="pkg.case" time="0.10"></testcase>`,
		`  <testcase name="fail" time="0.20">`,
		`    <failure message="boom" type="assert">trace line</failure>`,
		`    <system-out>stdout line</system-out>`,
		`    <system-err>stderr line</system-err>`,
		`  </testcase>`,
		`  <testcase name="skip" time="0.30"><skipped message="not now"/></testcase>`,
		`</testsuite>`,
	}
	suite := parseJUnitXMLSuite("", lines)
	if suite.Format != "junit-xml" {
		t.Fatalf("unexpected format: %q", suite.Format)
	}
	if suite.Name != "root-suite" {
		t.Fatalf("unexpected suite name: %q", suite.Name)
	}
	if suite.Total != 3 || suite.Passed != 1 || suite.Failed != 1 || suite.Skipped != 1 {
		t.Fatalf("unexpected counts: %+v", suite)
	}
	if len(suite.Cases) != 3 {
		t.Fatalf("expected 3 cases, got %d", len(suite.Cases))
	}
	if suite.Cases[0].Package != "pkg.case" || suite.Cases[0].Status != "pass" {
		t.Fatalf("unexpected first case: %+v", suite.Cases[0])
	}
	if suite.Cases[1].Package != "pkg.root" || suite.Cases[1].Status != "fail" {
		t.Fatalf("unexpected second case package/status: %+v", suite.Cases[1])
	}
	if !strings.Contains(suite.Cases[1].Output, "failure type=assert message=boom") {
		t.Fatalf("expected failure header in output, got: %q", suite.Cases[1].Output)
	}
	if !strings.Contains(suite.Cases[1].Output, "system-out") || !strings.Contains(suite.Cases[1].Output, "system-err") {
		t.Fatalf("expected system output markers, got: %q", suite.Cases[1].Output)
	}
	if suite.Cases[2].Status != "skip" {
		t.Fatalf("unexpected third case status: %+v", suite.Cases[2])
	}
}

func TestParseJUnitXMLSuiteDeclaredTotalsFallback(t *testing.T) {
	lines := []string{
		`<testsuites>`,
		`  <testsuite name="nested-a" tests="4" failures="1" errors="1" skipped="1"></testsuite>`,
		`</testsuites>`,
	}
	suite := parseJUnitXMLSuite("", lines)
	if suite.Name != "nested-a" {
		t.Fatalf("unexpected fallback suite name: %q", suite.Name)
	}
	if suite.Total != 4 || suite.Failed != 2 || suite.Skipped != 1 || suite.Passed != 1 {
		t.Fatalf("unexpected declared-total fallback counts: %+v", suite)
	}
	if len(suite.Cases) != 0 {
		t.Fatalf("expected no cases for declared-total fallback, got %d", len(suite.Cases))
	}
}

func TestParseJUnitXMLSuiteInvalidAndUnknownRoots(t *testing.T) {
	got := parseJUnitXMLSuite("explicit-name", []string{"<notjunit></notjunit>"})
	if got.Name != "explicit-name" || got.Format != "junit-xml" || got.Total != 0 {
		t.Fatalf("unexpected unknown-root parse result: %+v", got)
	}

	got = parseJUnitXMLSuite("explicit-name", []string{"<testsuite"})
	if got.Name != "explicit-name" || got.Total != 0 {
		t.Fatalf("unexpected invalid-xml parse result: %+v", got)
	}

	got = parseJUnitXMLSuite("", nil)
	if got.Name != "" || got.Total != 0 {
		t.Fatalf("unexpected empty-input parse result: %+v", got)
	}
}

func TestJUnitHelperFunctions(t *testing.T) {
	flat := flattenJUnitSuites(junitTestSuite{
		Name: "root",
		Suites: []junitTestSuite{{
			Name: "child",
			Suites: []junitTestSuite{{
				Name: "grandchild",
			}},
		}},
	})
	if len(flat) != 3 {
		t.Fatalf("expected flattened length 3, got %d", len(flat))
	}
	if flat[0].Name != "root" || flat[1].Name != "child" || flat[2].Name != "grandchild" {
		t.Fatalf("unexpected flatten order: %+v", flat)
	}

	msg := collectJUnitMessages(
		[]junitMessage{{Type: "assert", Message: "boom", Body: "stack"}},
		[]junitMessage{{Message: "err"}},
		[]junitMessage{{}},
		"stdout",
		"stderr",
	)
	if !strings.Contains(msg, "failure type=assert message=boom") ||
		!strings.Contains(msg, "error message=err") ||
		!strings.Contains(msg, "skipped") ||
		!strings.Contains(msg, "system-out") ||
		!strings.Contains(msg, "system-err") {
		t.Fatalf("unexpected collected messages: %q", msg)
	}

	if got := parseIntDefault(" 42 ", 1); got != 42 {
		t.Fatalf("parseIntDefault expected 42, got %d", got)
	}
	if got := parseIntDefault("bad", 7); got != 7 {
		t.Fatalf("parseIntDefault fallback expected 7, got %d", got)
	}
	if got := parseFloatDefault(" 1.5 ", 2.0); got != 1.5 {
		t.Fatalf("parseFloatDefault expected 1.5, got %f", got)
	}
	if got := parseFloatDefault("bad", 2.0); got != 2.0 {
		t.Fatalf("parseFloatDefault fallback expected 2.0, got %f", got)
	}
	if got := maxInt(3, 2); got != 3 {
		t.Fatalf("maxInt expected 3, got %d", got)
	}
	if got := maxInt(-1, 0); got != 0 {
		t.Fatalf("maxInt expected 0, got %d", got)
	}
}
