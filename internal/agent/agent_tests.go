package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func uploadTestReport(ctx context.Context, client *http.Client, serverURL, agentID, jobID string, report protocol.JobExecutionTestReport) error {
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

func testReportSummary(report protocol.JobExecutionTestReport) string {
	if report.Total == 0 {
		return "[tests] none"
	}
	return "[tests] total=" + strconv.Itoa(report.Total) +
		" passed=" + strconv.Itoa(report.Passed) +
		" failed=" + strconv.Itoa(report.Failed) +
		" skipped=" + strconv.Itoa(report.Skipped)
}
