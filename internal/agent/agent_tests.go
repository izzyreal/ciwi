package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/izzyreal/ciwi/internal/protocol"
)

type goTestEvent struct {
	Action  string  `json:"Action"`
	Package string  `json:"Package"`
	Test    string  `json:"Test"`
	Elapsed float64 `json:"Elapsed"`
	Output  string  `json:"Output"`
}

func uploadTestReport(ctx context.Context, client *http.Client, serverURL, agentID, jobID string, report protocol.JobTestReport) error {
	body, err := json.Marshal(protocol.UploadTestReportRequest{
		AgentID: agentID,
		Report:  report,
	})
	if err != nil {
		return fmt.Errorf("marshal test report: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL+"/api/v1/jobs/"+jobID+"/tests", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create test report request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send test report: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024))
		return fmt.Errorf("test report rejected: status=%d body=%s", resp.StatusCode, bytes.TrimSpace(respBody))
	}
	return nil
}

func parseJobTestReport(output string) protocol.JobTestReport {
	lines := strings.Split(output, "\n")
	suites := make([]protocol.TestSuiteReport, 0)

	var active bool
	var suiteName string
	var suiteFormat string
	var suiteLines []string

	flushSuite := func() {
		if !active {
			return
		}
		var suite protocol.TestSuiteReport
		switch suiteFormat {
		case "", "go-test-json":
			suite = parseGoTestJSONSuite(suiteName, suiteLines)
		default:
			suite = protocol.TestSuiteReport{Name: suiteName, Format: suiteFormat}
		}
		if suite.Format == "" {
			suite.Format = suiteFormat
		}
		if suite.Total > 0 || len(suite.Cases) > 0 {
			suites = append(suites, suite)
		}
		active = false
		suiteName, suiteFormat = "", ""
		suiteLines = nil
	}

	for _, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		if strings.HasPrefix(line, "__CIWI_TEST_BEGIN__") {
			flushSuite()
			meta := strings.TrimSpace(strings.TrimPrefix(line, "__CIWI_TEST_BEGIN__"))
			name, format := parseTestMarkerMeta(meta)
			if format == "" {
				format = "go-test-json"
			}
			active = true
			suiteName = name
			suiteFormat = format
			suiteLines = make([]string, 0, 128)
			continue
		}
		if strings.HasPrefix(line, "__CIWI_TEST_END__") {
			flushSuite()
			continue
		}
		if active {
			suiteLines = append(suiteLines, line)
		}
	}
	flushSuite()

	report := protocol.JobTestReport{Suites: suites}
	for _, s := range suites {
		report.Total += s.Total
		report.Passed += s.Passed
		report.Failed += s.Failed
		report.Skipped += s.Skipped
	}
	return report
}

func parseTestMarkerMeta(meta string) (name string, format string) {
	parts := strings.Fields(meta)
	for _, part := range parts {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "name":
			name = kv[1]
		case "format":
			format = kv[1]
		}
	}
	return name, format
}

func parseGoTestJSONSuite(name string, lines []string) protocol.TestSuiteReport {
	type caseKey struct {
		pkg  string
		test string
	}
	type caseState struct {
		pkg      string
		name     string
		status   string
		elapsed  float64
		outputSB strings.Builder
	}

	order := make([]caseKey, 0)
	cases := make(map[caseKey]*caseState)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		var ev goTestEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		if strings.TrimSpace(ev.Test) == "" {
			continue
		}
		key := caseKey{pkg: ev.Package, test: ev.Test}
		st, ok := cases[key]
		if !ok {
			st = &caseState{pkg: ev.Package, name: ev.Test}
			cases[key] = st
			order = append(order, key)
		}
		if ev.Output != "" {
			st.outputSB.WriteString(ev.Output)
		}
		switch ev.Action {
		case "pass", "fail", "skip":
			st.status = ev.Action
			if ev.Elapsed > 0 {
				st.elapsed = ev.Elapsed
			}
		}
	}

	suite := protocol.TestSuiteReport{
		Name:   name,
		Format: "go-test-json",
		Cases:  make([]protocol.TestCase, 0, len(order)),
	}
	for _, key := range order {
		st := cases[key]
		status := st.status
		if status == "" {
			status = "unknown"
		}
		tc := protocol.TestCase{
			Package:         st.pkg,
			Name:            st.name,
			Status:          status,
			DurationSeconds: st.elapsed,
			Output:          st.outputSB.String(),
		}
		suite.Cases = append(suite.Cases, tc)
		suite.Total++
		switch status {
		case "pass":
			suite.Passed++
		case "fail":
			suite.Failed++
		case "skip":
			suite.Skipped++
		}
	}
	return suite
}

func testReportSummary(report protocol.JobTestReport) string {
	if report.Total == 0 {
		return "[tests] none"
	}
	return "[tests] total=" + strconv.Itoa(report.Total) +
		" passed=" + strconv.Itoa(report.Passed) +
		" failed=" + strconv.Itoa(report.Failed) +
		" skipped=" + strconv.Itoa(report.Skipped)
}
