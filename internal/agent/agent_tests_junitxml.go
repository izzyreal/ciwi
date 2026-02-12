package agent

import (
	"encoding/xml"
	"strconv"
	"strings"

	"github.com/izzyreal/ciwi/internal/protocol"
)

type junitXMLRootName struct {
	XMLName xml.Name
}

type junitTestSuites struct {
	Suites []junitTestSuite `xml:"testsuite"`
}

type junitTestSuite struct {
	Name      string           `xml:"name,attr"`
	Package   string           `xml:"package,attr"`
	Tests     int              `xml:"tests,attr"`
	Failures  int              `xml:"failures,attr"`
	Errors    int              `xml:"errors,attr"`
	Skipped   string           `xml:"skipped,attr"`
	TestCases []junitTestCase  `xml:"testcase"`
	Suites    []junitTestSuite `xml:"testsuite"`
}

type junitTestCase struct {
	Name      string         `xml:"name,attr"`
	ClassName string         `xml:"classname,attr"`
	Time      string         `xml:"time,attr"`
	Failures  []junitMessage `xml:"failure"`
	Errors    []junitMessage `xml:"error"`
	Skipped   []junitMessage `xml:"skipped"`
	SystemOut string         `xml:"system-out"`
	SystemErr string         `xml:"system-err"`
}

type junitMessage struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Body    string `xml:",chardata"`
}

func parseJUnitXMLSuite(name string, lines []string) protocol.TestSuiteReport {
	suite := protocol.TestSuiteReport{
		Name:   name,
		Format: "junit-xml",
	}

	raw := strings.TrimSpace(strings.Join(lines, "\n"))
	if raw == "" {
		return suite
	}

	var root junitXMLRootName
	if err := xml.Unmarshal([]byte(raw), &root); err != nil {
		return suite
	}

	var junitSuites []junitTestSuite
	switch root.XMLName.Local {
	case "testsuite":
		var ts junitTestSuite
		if err := xml.Unmarshal([]byte(raw), &ts); err != nil {
			return suite
		}
		junitSuites = append(junitSuites, flattenJUnitSuites(ts)...)
	case "testsuites":
		var tss junitTestSuites
		if err := xml.Unmarshal([]byte(raw), &tss); err != nil {
			return suite
		}
		for _, child := range tss.Suites {
			junitSuites = append(junitSuites, flattenJUnitSuites(child)...)
		}
	default:
		return suite
	}

	if suite.Name == "" {
		for _, ts := range junitSuites {
			if strings.TrimSpace(ts.Name) != "" {
				suite.Name = strings.TrimSpace(ts.Name)
				break
			}
		}
	}

	cases := make([]protocol.TestCase, 0)
	declaredTotal := 0
	declaredFailed := 0
	declaredSkipped := 0
	for _, ts := range junitSuites {
		declaredTotal += maxInt(0, ts.Tests)
		declaredFailed += maxInt(0, ts.Failures) + maxInt(0, ts.Errors)
		declaredSkipped += parseIntDefault(ts.Skipped, 0)

		defaultPackage := strings.TrimSpace(ts.Package)
		if defaultPackage == "" {
			defaultPackage = strings.TrimSpace(ts.Name)
		}
		for _, tc := range ts.TestCases {
			status := "pass"
			switch {
			case len(tc.Failures) > 0 || len(tc.Errors) > 0:
				status = "fail"
			case len(tc.Skipped) > 0:
				status = "skip"
			}

			pkg := strings.TrimSpace(tc.ClassName)
			if pkg == "" {
				pkg = defaultPackage
			}

			out := collectJUnitMessages(tc.Failures, tc.Errors, tc.Skipped, tc.SystemOut, tc.SystemErr)
			testCase := protocol.TestCase{
				Package:         pkg,
				Name:            strings.TrimSpace(tc.Name),
				Status:          status,
				DurationSeconds: parseFloatDefault(tc.Time, 0),
				Output:          out,
			}
			cases = append(cases, testCase)
		}
	}

	suite.Cases = cases
	suite.Total = len(cases)
	for _, tc := range cases {
		switch tc.Status {
		case "pass":
			suite.Passed++
		case "fail":
			suite.Failed++
		case "skip":
			suite.Skipped++
		}
	}

	// Some reporters emit only suite totals without testcase elements.
	if suite.Total == 0 && declaredTotal > 0 {
		suite.Total = declaredTotal
		suite.Failed = declaredFailed
		suite.Skipped = declaredSkipped
		passed := suite.Total - suite.Failed - suite.Skipped
		if passed < 0 {
			passed = 0
		}
		suite.Passed = passed
	}

	return suite
}

func flattenJUnitSuites(ts junitTestSuite) []junitTestSuite {
	out := []junitTestSuite{ts}
	for _, child := range ts.Suites {
		out = append(out, flattenJUnitSuites(child)...)
	}
	return out
}

func collectJUnitMessages(failures, errors, skipped []junitMessage, systemOut, systemErr string) string {
	lines := make([]string, 0)
	appendMessages := func(kind string, msgs []junitMessage) {
		for _, m := range msgs {
			headParts := []string{kind}
			if t := strings.TrimSpace(m.Type); t != "" {
				headParts = append(headParts, "type="+t)
			}
			if msg := strings.TrimSpace(m.Message); msg != "" {
				headParts = append(headParts, "message="+msg)
			}
			lines = append(lines, strings.Join(headParts, " "))
			if body := strings.TrimSpace(m.Body); body != "" {
				lines = append(lines, body)
			}
		}
	}
	appendMessages("failure", failures)
	appendMessages("error", errors)
	appendMessages("skipped", skipped)
	if v := strings.TrimSpace(systemOut); v != "" {
		lines = append(lines, "system-out")
		lines = append(lines, v)
	}
	if v := strings.TrimSpace(systemErr); v != "" {
		lines = append(lines, "system-err")
		lines = append(lines, v)
	}
	return strings.Join(lines, "\n")
}

func parseIntDefault(raw string, fallback int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return v
}

func parseFloatDefault(raw string, fallback float64) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return fallback
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
