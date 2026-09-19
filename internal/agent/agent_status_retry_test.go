package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/store"
)

func fastStatusRetryPolicy() statusRetryPolicy {
	return statusRetryPolicy{time.Second, 50 * time.Millisecond, time.Millisecond, 5 * time.Millisecond}
}

func TestStatusRetryLostResponseDeduplicatesPersistedEvents(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ciwi.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	job, err := db.CreateJobExecution(protocol.CreateJobExecutionRequest{Script: "echo hello", TimeoutSeconds: 60})
	if err != nil {
		t.Fatal(err)
	}
	req := protocol.JobExecutionStatusUpdateRequest{AgentID: "agent", Status: "running", TimestampUTC: time.Now().UTC(), Events: []protocol.JobExecutionEvent{
		{Type: protocol.JobExecutionEventTypeStepStarted, TimestampUTC: time.Now().UTC(), Step: &protocol.JobStepPlanItem{Index: 1, Total: 1, Name: "build"}},
		{Type: protocol.JobExecutionEventTypeStepOutput, TimestampUTC: time.Now().UTC(), Output: "hello\n"},
	}}
	var first []byte
	attempts := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		attempts++
		body, _ := io.ReadAll(r.Body)
		if attempts == 1 {
			first = body
		} else if !bytes.Equal(first, body) {
			t.Error("retry payload changed")
		}
		var got protocol.JobExecutionStatusUpdateRequest
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatal(err)
		}
		if _, err := db.UpdateJobExecutionStatus(job.ID, got); err != nil {
			t.Fatal(err)
		}
		if err := db.AppendJobExecutionEvents(job.ID, got.Events); err != nil {
			t.Fatal(err)
		}
		if attempts == 1 {
			return nil, io.ErrUnexpectedEOF
		}
		return jsonHTTPResponse(200, `{"job_execution":{"status":"running"}}`), nil
	})}
	if err := reportJobStatusWithRetry(t.Context(), client, "http://ciwi", job.ID, req, nil, fastStatusRetryPolicy()); err != nil {
		t.Fatal(err)
	}
	events, err := db.ListJobExecutionEvents(job.ID)
	if err != nil || len(events) != 2 || attempts != 2 {
		t.Fatalf("events=%v attempts=%d err=%v", events, attempts, err)
	}
}

func TestStatusRetryClassification(t *testing.T) {
	for _, code := range []int{400, 401, 403, 404, 409, 408, 429, 500, 502, 503, 504} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			count := 0
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				count++
				if count == 1 {
					return jsonHTTPResponse(code, "failure"), nil
				}
				return jsonHTTPResponse(200, `{}`), nil
			})}
			err := reportJobStatusWithRetry(t.Context(), client, "http://ciwi", "job", protocol.JobExecutionStatusUpdateRequest{Status: "running"}, nil, fastStatusRetryPolicy())
			retry := code == 408 || code == 429 || code >= 500
			if retry && (err != nil || count != 2) || !retry && (err == nil || count != 1) {
				t.Fatalf("count=%d err=%v", count, err)
			}
		})
	}
}

func TestStatusRetryTimeoutRecovery(t *testing.T) {
	count := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		count++
		if count == 1 {
			<-r.Context().Done()
			return nil, r.Context().Err()
		}
		return jsonHTTPResponse(200, `{}`), nil
	})}
	if err := reportJobStatusWithRetry(t.Context(), client, "http://ciwi", "job", protocol.JobExecutionStatusUpdateRequest{Status: "running"}, nil, fastStatusRetryPolicy()); err != nil || count != 2 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}

func TestStatusRetryBudgetAndParentDeadline(t *testing.T) {
	for _, parentDeadline := range []bool{false, true} {
		t.Run(map[bool]string{false: "recovery budget", true: "job deadline"}[parentDeadline], func(t *testing.T) {
			policy := fastStatusRetryPolicy()
			policy.budget = 40 * time.Millisecond
			ctx := t.Context()
			if parentDeadline {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 20*time.Millisecond)
				defer cancel()
				policy.budget = time.Second
			}
			count := 0
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				count++
				return jsonHTTPResponse(503, "unavailable"), nil
			})}
			start := time.Now()
			err := reportJobStatusWithRetry(ctx, client, "http://ciwi", "job", protocol.JobExecutionStatusUpdateRequest{Status: "running"}, nil, policy)
			if !errors.Is(err, context.DeadlineExceeded) || count < 2 || time.Since(start) > time.Second {
				t.Fatalf("count=%d elapsed=%v err=%v", count, time.Since(start), err)
			}
		})
	}
}

func TestStatusRetryCancellationAndRetryAfter(t *testing.T) {
	for _, inRequest := range []bool{false, true} {
		t.Run(map[bool]string{false: "backoff", true: "request"}[inRequest], func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			entered := make(chan struct{})
			count := 0
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				count++
				close(entered)
				if inRequest {
					<-r.Context().Done()
					return nil, r.Context().Err()
				}
				resp := jsonHTTPResponse(429, "wait")
				resp.Header.Set("Retry-After", "120")
				return resp, nil
			})}
			done := make(chan error, 1)
			go func() {
				done <- reportJobStatusWithRetry(ctx, client, "http://ciwi", "job", protocol.JobExecutionStatusUpdateRequest{Status: "running"}, nil, fastStatusRetryPolicy())
			}()
			<-entered
			// With Retry-After respected there cannot be a second call during this wait.
			if !inRequest {
				time.Sleep(20 * time.Millisecond)
			}
			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) || count != 1 {
					t.Fatalf("count=%d err=%v", count, err)
				}
			case <-time.After(time.Second):
				t.Fatal("cancellation blocked")
			}
		})
	}
}

func TestStatusRetryServerTerminal(t *testing.T) {
	for _, status := range []string{"cancelled", "failed", "succeeded"} {
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return jsonHTTPResponse(200, `{"job_execution":{"status":"`+status+`"}}`), nil
		})}
		err := reportJobStatusWithRetry(t.Context(), client, "http://ciwi", "job", protocol.JobExecutionStatusUpdateRequest{Status: "running"}, nil, fastStatusRetryPolicy())
		var terminal *serverTerminalStatusError
		if !errors.As(err, &terminal) || terminal.status != status {
			t.Fatalf("status=%s err=%v", status, err)
		}
	}
}

type countingStatusScriptRunner struct{ calls []string }

func (s *countingStatusScriptRunner) Run(ctx context.Context, req scriptRunRequest) error {
	s.calls = append(s.calls, req.Script)
	return nil
}

func TestExecutionRetriesTransitionsWithoutRepeatingCommands(t *testing.T) {
	for _, scenario := range []string{"start recovers", "finish recovers", "start exhausted", "finish exhausted", "permanent rejection", "terminal response", "phase recovers", "phase exhausted"} {
		t.Run(scenario, func(t *testing.T) {
			runner := &countingStatusScriptRunner{}
			attempts := 0
			var final protocol.JobExecutionStatusUpdateRequest
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.Method == http.MethodGet {
					return jsonHTTPResponse(200, `{"job_execution":{"status":"running"}}`), nil
				}
				var req protocol.JobExecutionStatusUpdateRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Fatal(err)
				}
				if req.Status != "running" {
					final = req
					return jsonHTTPResponse(200, `{}`), nil
				}
				for _, event := range req.Events {
					target := protocol.JobExecutionEventTypeStepStarted
					if strings.HasPrefix(scenario, "finish") {
						target = protocol.JobExecutionEventTypeStepFinished
					}
					isTarget := event.Step != nil && event.Step.Index == 1 && event.Type == target
					if strings.HasPrefix(scenario, "phase") {
						isTarget = event.Phase != nil && event.Phase.ID == protocol.JobExecutionPhaseEnvironment && event.Type == protocol.JobExecutionEventTypePhaseFinished
					}
					if isTarget {
						attempts++
						if scenario == "terminal response" {
							return jsonHTTPResponse(200, `{"job_execution":{"status":"cancelled"}}`), nil
						}
						if scenario == "permanent rejection" {
							return jsonHTTPResponse(403, "forbidden"), nil
						}
						if strings.HasSuffix(scenario, "exhausted") || attempts == 1 {
							return jsonHTTPResponse(503, "unavailable"), nil
						}
					}
				}
				return jsonHTTPResponse(200, `{}`), nil
			})}
			policy := fastStatusRetryPolicy()
			policy.budget = 30 * time.Millisecond
			job := protocol.JobExecution{ID: "job", TimeoutSeconds: 60, StepPlan: []protocol.JobStepPlanItem{
				{Index: 1, Total: 2, Name: "configure", Script: "configure"}, {Index: 2, Total: 2, Name: "build", Script: "build"},
			}}
			err := executeLeasedJobWithDependencies(t.Context(), client, "http://ciwi", "agent", t.TempDir(), nil, job, executionDependencies{scripts: runner, statusRetry: policy})
			if scenario == "terminal response" {
				var terminal *serverTerminalStatusError
				if !errors.As(err, &terminal) || len(runner.calls) != 0 || final.Status != "" {
					t.Fatalf("calls=%v final=%s err=%v", runner.calls, final.Status, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			wantCalls, wantStatus := 0, "failed"
			if strings.HasSuffix(scenario, "recovers") {
				wantCalls, wantStatus = 2, "succeeded"
			} else if scenario == "finish exhausted" {
				wantCalls = 1
			}
			if len(runner.calls) != wantCalls || final.Status != wantStatus {
				t.Fatalf("calls=%v final=%+v", runner.calls, final)
			}
			if strings.HasSuffix(scenario, "recovers") && attempts != 2 {
				t.Fatalf("attempts=%d", attempts)
			}
			if wantStatus == "failed" && !strings.Contains(final.Error, "report") {
				t.Fatalf("lost communication failure: %q", final.Error)
			}
		})
	}
}

func TestTerminalMonitorStopCancelsPendingRequest(t *testing.T) {
	entered := make(chan struct{})
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		close(entered)
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}
	stop := monitorServerTerminalJobState(t.Context(), client, "http://ciwi", "agent", "job", nil, func() { t.Error("unexpected cancellation") })
	<-entered
	done := make(chan struct{})
	go func() { stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("monitor shutdown blocked on HTTP request")
	}
}

func TestStatusRetryRedactsReturnedError(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonHTTPResponse(403, "secret-value rejected"), nil
	})}
	err := reportJobStatusWithRetry(t.Context(), client, "http://ciwi", "job", protocol.JobExecutionStatusUpdateRequest{Status: "running"}, []string{"secret-value"}, fastStatusRetryPolicy())
	var response *statusHTTPError
	if err == nil || strings.Contains(err.Error(), "secret-value") || !errors.As(err, &response) || response.code != 403 {
		t.Fatalf("error was not safely redacted with classification preserved: %v", err)
	}
}

func TestStatusRetryIncompleteResponse(t *testing.T) {
	for _, body := range []string{"", `{"job_execution":`} {
		count := 0
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			count++
			if count == 1 {
				return jsonHTTPResponse(200, body), nil
			}
			return jsonHTTPResponse(200, `{"job_execution":{"status":"running"}}`), nil
		})}
		if err := reportJobStatusWithRetry(t.Context(), client, "http://ciwi", "job", protocol.JobExecutionStatusUpdateRequest{Status: "running"}, nil, fastStatusRetryPolicy()); err != nil || count != 2 {
			t.Fatalf("count=%d err=%v", count, err)
		}
	}
}

func TestStatusRetryAfterHonorsBudget(t *testing.T) {
	policy := fastStatusRetryPolicy()
	policy.budget = 30 * time.Millisecond
	attempts := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		attempts++
		resp := jsonHTTPResponse(503, "unavailable")
		resp.Header.Set("Retry-After", "120")
		return resp, nil
	})}
	start := time.Now()
	err := reportJobStatusWithRetry(t.Context(), client, "http://ciwi", "job", protocol.JobExecutionStatusUpdateRequest{Status: "running"}, nil, policy)
	if !errors.Is(err, context.DeadlineExceeded) || attempts != 1 || time.Since(start) > time.Second {
		t.Fatalf("attempts=%d elapsed=%v err=%v", attempts, time.Since(start), err)
	}
	date := time.Now().Add(time.Minute).UTC().Format(http.TimeFormat)
	delay := statusRetryAfter(&statusHTTPError{retryAfter: date})
	if delay < 58*time.Second || delay > time.Minute {
		t.Fatalf("unexpected HTTP-date delay: %v", delay)
	}
}
