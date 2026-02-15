package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func jsonHTTPResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestSendHeartbeatSuccessAndPayload(t *testing.T) {
	t.Parallel()

	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method: %s", r.Method)
			}
			if got := r.URL.Path; got != "/api/v1/heartbeat" {
				t.Fatalf("unexpected path: %s", got)
			}
			if got := r.Header.Get("Content-Type"); got != "application/json" {
				t.Fatalf("unexpected content type: %q", got)
			}
			var req protocol.HeartbeatRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode heartbeat request: %v", err)
			}
			if req.AgentID != "agent-1" || req.Hostname != "host-1" {
				t.Fatalf("unexpected identity fields: %+v", req)
			}
			if req.UpdateFailure != "failed once" {
				t.Fatalf("expected trimmed update failure, got %q", req.UpdateFailure)
			}
			if req.RestartStatus != "restart requested" {
				t.Fatalf("expected trimmed restart status, got %q", req.RestartStatus)
			}
			if req.OS != runtime.GOOS || req.Arch != runtime.GOARCH {
				t.Fatalf("unexpected platform in request: %s/%s", req.OS, req.Arch)
			}
			if req.TimestampUTC.IsZero() {
				t.Fatalf("expected timestamp to be populated")
			}
			return jsonHTTPResponse(http.StatusOK, `{"accepted":true,"update_requested":true,"update_target":"v2.0.0"}`), nil
		}),
	}

	resp, err := sendHeartbeat(context.Background(), client, "http://ciwi.local", "agent-1", "host-1", map[string]string{"executor": "script"}, " failed once ", " restart requested ")
	if err != nil {
		t.Fatalf("sendHeartbeat returned error: %v", err)
	}
	if !resp.Accepted || !resp.UpdateRequested || resp.UpdateTarget != "v2.0.0" {
		t.Fatalf("unexpected heartbeat response: %+v", resp)
	}
}

func TestSendHeartbeatErrors(t *testing.T) {
	t.Parallel()

	t.Run("request creation error", func(t *testing.T) {
		t.Parallel()
		_, err := sendHeartbeat(context.Background(), &http.Client{}, "://bad-url", "a", "h", nil, "", "")
		if err == nil || !strings.Contains(err.Error(), "create heartbeat request") {
			t.Fatalf("expected create request error, got %v", err)
		}
	})

	t.Run("transport error", func(t *testing.T) {
		t.Parallel()
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return nil, errors.New("boom")
		})}
		_, err := sendHeartbeat(context.Background(), client, "http://ciwi.local", "a", "h", nil, "", "")
		if err == nil || !strings.Contains(err.Error(), "send heartbeat") {
			t.Fatalf("expected transport error, got %v", err)
		}
	})

	t.Run("non-200 response", func(t *testing.T) {
		t.Parallel()
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return jsonHTTPResponse(http.StatusForbidden, `forbidden`), nil
		})}
		_, err := sendHeartbeat(context.Background(), client, "http://ciwi.local", "a", "h", nil, "", "")
		if err == nil || !strings.Contains(err.Error(), "heartbeat rejected") {
			t.Fatalf("expected heartbeat rejected error, got %v", err)
		}
	})

	t.Run("decode error", func(t *testing.T) {
		t.Parallel()
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return jsonHTTPResponse(http.StatusOK, `{not-json`), nil
		})}
		_, err := sendHeartbeat(context.Background(), client, "http://ciwi.local", "a", "h", nil, "", "")
		if err == nil || !strings.Contains(err.Error(), "decode heartbeat response") {
			t.Fatalf("expected decode error, got %v", err)
		}
	})
}

func TestLeaseJobDefaultsAndResponses(t *testing.T) {
	t.Parallel()

	t.Run("applies capability defaults and returns assigned job", func(t *testing.T) {
		t.Parallel()
		client := &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.Method != http.MethodPost {
					t.Fatalf("unexpected method: %s", r.Method)
				}
				if got := r.URL.Path; got != "/api/v1/agent/lease" {
					t.Fatalf("unexpected path: %s", got)
				}
				var req protocol.LeaseJobExecutionRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Fatalf("decode lease request: %v", err)
				}
				if req.AgentID != "agent-1" {
					t.Fatalf("unexpected agent id: %q", req.AgentID)
				}
				if req.Capabilities["executor"] != executorScript {
					t.Fatalf("expected executor default %q, got %q", executorScript, req.Capabilities["executor"])
				}
				if req.Capabilities["os"] != runtime.GOOS || req.Capabilities["arch"] != runtime.GOARCH {
					t.Fatalf("unexpected runtime defaults: %+v", req.Capabilities)
				}
				if strings.TrimSpace(req.Capabilities["shells"]) == "" {
					t.Fatalf("expected shells default to be populated")
				}
				return jsonHTTPResponse(http.StatusOK, `{"assigned":true,"job_execution":{"id":"job-1","script":"echo hi","required_capabilities":{},"timeout_seconds":30,"status":"leased","created_utc":"2026-02-15T00:00:00Z"}}`), nil
			}),
		}

		job, err := leaseJob(context.Background(), client, "http://ciwi.local", "agent-1", nil)
		if err != nil {
			t.Fatalf("leaseJob returned error: %v", err)
		}
		if job == nil || job.ID != "job-1" {
			t.Fatalf("unexpected leased job: %+v", job)
		}
	})

	t.Run("not assigned returns nil,nil", func(t *testing.T) {
		t.Parallel()
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return jsonHTTPResponse(http.StatusOK, `{"assigned":false}`), nil
		})}
		job, err := leaseJob(context.Background(), client, "http://ciwi.local", "agent-1", map[string]string{"executor": "script"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if job != nil {
			t.Fatalf("expected no job when not assigned, got %+v", job)
		}
	})
}

func TestLeaseJobErrors(t *testing.T) {
	t.Parallel()

	clientTransportErr := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return nil, errors.New("boom")
	})}
	if _, err := leaseJob(context.Background(), clientTransportErr, "http://ciwi.local", "agent-1", nil); err == nil || !strings.Contains(err.Error(), "send lease request") {
		t.Fatalf("expected transport error, got %v", err)
	}

	clientReject := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonHTTPResponse(http.StatusConflict, `nope`), nil
	})}
	if _, err := leaseJob(context.Background(), clientReject, "http://ciwi.local", "agent-1", nil); err == nil || !strings.Contains(err.Error(), "lease rejected") {
		t.Fatalf("expected lease rejected error, got %v", err)
	}

	clientDecode := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonHTTPResponse(http.StatusOK, `{not-json`), nil
	})}
	if _, err := leaseJob(context.Background(), clientDecode, "http://ciwi.local", "agent-1", nil); err == nil || !strings.Contains(err.Error(), "decode lease response") {
		t.Fatalf("expected decode error, got %v", err)
	}

	if _, err := leaseJob(context.Background(), &http.Client{}, "://bad-url", "agent-1", nil); err == nil || !strings.Contains(err.Error(), "create lease request") {
		t.Fatalf("expected create request error, got %v", err)
	}
}

func TestReportFailureAndReportJobStatus(t *testing.T) {
	t.Parallel()

	var got protocol.JobExecutionStatusUpdateRequest
	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodPost || r.URL.Path != "/api/v1/jobs/job-1/status" {
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
			}
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatalf("decode status update: %v", err)
			}
			return jsonHTTPResponse(http.StatusOK, `{}`), nil
		}),
	}

	exitCode := 12
	err := reportFailure(context.Background(), client, "http://ciwi.local", "agent-1", protocol.JobExecution{ID: "job-1"}, &exitCode, "boom", "logs")
	if err != nil {
		t.Fatalf("reportFailure returned error: %v", err)
	}
	if got.AgentID != "agent-1" || got.Status != protocol.JobExecutionStatusFailed {
		t.Fatalf("unexpected status payload: %+v", got)
	}
	if got.ExitCode == nil || *got.ExitCode != 12 {
		t.Fatalf("expected exit code 12 in payload: %+v", got)
	}
	if got.Error != "boom" || got.Output != "logs" {
		t.Fatalf("unexpected failure payload: %+v", got)
	}
	if got.TimestampUTC.IsZero() {
		t.Fatalf("expected timestamp in failure payload")
	}
}

func TestReportTerminalJobStatusWithRetryBackoff(t *testing.T) {
	t.Parallel()

	attempt := 0
	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			attempt++
			if attempt == 1 {
				return jsonHTTPResponse(http.StatusServiceUnavailable, `try again`), nil
			}
			return jsonHTTPResponse(http.StatusOK, `{}`), nil
		}),
	}

	start := time.Now()
	err := reportTerminalJobStatusWithRetry(client, "http://ciwi.local", "job-1", protocol.JobExecutionStatusUpdateRequest{
		AgentID: "agent-1",
		Status:  protocol.JobExecutionStatusSucceeded,
	})
	if err != nil {
		t.Fatalf("expected retry flow to eventually succeed, got %v", err)
	}
	if attempt != 2 {
		t.Fatalf("expected exactly 2 attempts, got %d", attempt)
	}
	if elapsed := time.Since(start); elapsed < 900*time.Millisecond {
		t.Fatalf("expected at least one retry backoff sleep, elapsed=%s", elapsed)
	}
}

func TestReportJobStatusErrors(t *testing.T) {
	t.Parallel()

	reqBody := protocol.JobExecutionStatusUpdateRequest{
		AgentID: "agent-1",
		Status:  protocol.JobExecutionStatusRunning,
	}

	if err := reportJobStatus(context.Background(), &http.Client{}, "://bad-url", "job-1", reqBody); err == nil || !strings.Contains(err.Error(), "create job status request") {
		t.Fatalf("expected create request error, got %v", err)
	}

	clientTransportErr := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return nil, errors.New("boom")
	})}
	if err := reportJobStatus(context.Background(), clientTransportErr, "http://ciwi.local", "job-1", reqBody); err == nil || !strings.Contains(err.Error(), "send job status request") {
		t.Fatalf("expected transport error, got %v", err)
	}

	clientReject := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonHTTPResponse(http.StatusBadRequest, `nope`), nil
	})}
	if err := reportJobStatus(context.Background(), clientReject, "http://ciwi.local", "job-1", reqBody); err == nil || !strings.Contains(err.Error(), "status rejected") {
		t.Fatalf("expected status rejected error, got %v", err)
	}
}

func TestCloneMapAndGetJobExecutionState(t *testing.T) {
	t.Parallel()

	if got := cloneMap(nil); got != nil {
		t.Fatalf("expected nil for nil map, got %#v", got)
	}
	in := map[string]string{"a": "1"}
	out := cloneMap(in)
	if out["a"] != "1" {
		t.Fatalf("unexpected clone result: %#v", out)
	}
	out["a"] = "2"
	if in["a"] != "1" {
		t.Fatalf("cloneMap should deep-copy: in=%#v out=%#v", in, out)
	}

	t.Run("state success trims fields", func(t *testing.T) {
		t.Parallel()
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodGet || r.URL.Path != "/api/v1/jobs/job-1" {
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
			}
			return jsonHTTPResponse(http.StatusOK, `{"job_execution":{"status":" succeeded ","error":" forced failure "}}`), nil
		})}
		state, err := getJobExecutionState(context.Background(), client, "http://ciwi.local", "job-1")
		if err != nil {
			t.Fatalf("getJobExecutionState returned error: %v", err)
		}
		if state.Status != "succeeded" || state.Error != "forced failure" {
			t.Fatalf("unexpected state: %+v", state)
		}
	})

	t.Run("state request creation error", func(t *testing.T) {
		t.Parallel()
		_, err := getJobExecutionState(context.Background(), &http.Client{}, "://bad-url", "job-1")
		if err == nil || !strings.Contains(err.Error(), "create job state request") {
			t.Fatalf("expected create request error, got %v", err)
		}
	})

	t.Run("state transport error", func(t *testing.T) {
		t.Parallel()
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return nil, errors.New("boom")
		})}
		_, err := getJobExecutionState(context.Background(), client, "http://ciwi.local", "job-1")
		if err == nil || !strings.Contains(err.Error(), "send job state request") {
			t.Fatalf("expected transport error, got %v", err)
		}
	})

	t.Run("state non-200 error", func(t *testing.T) {
		t.Parallel()
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return jsonHTTPResponse(http.StatusNotFound, `missing`), nil
		})}
		_, err := getJobExecutionState(context.Background(), client, "http://ciwi.local", "job-1")
		if err == nil || !strings.Contains(err.Error(), "job state rejected") {
			t.Fatalf("expected rejected error, got %v", err)
		}
	})

	t.Run("state decode error", func(t *testing.T) {
		t.Parallel()
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return jsonHTTPResponse(http.StatusOK, `{not-json`), nil
		})}
		_, err := getJobExecutionState(context.Background(), client, "http://ciwi.local", "job-1")
		if err == nil || !strings.Contains(err.Error(), "decode job state response") {
			t.Fatalf("expected decode error, got %v", err)
		}
	})
}
