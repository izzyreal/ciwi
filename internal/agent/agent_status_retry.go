package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

type statusRetryPolicy struct {
	budget, attemptTimeout, initialBackoff, maxBackoff time.Duration
}

func defaultStatusRetryPolicy() statusRetryPolicy {
	return statusRetryPolicy{5 * time.Minute, 15 * time.Second, time.Second, 15 * time.Second}
}

type retryableStatusError struct{ error }

func (e *retryableStatusError) Unwrap() error { return e.error }

type terminalStatusReportError struct{ error }

func (e *terminalStatusReportError) Unwrap() error { return e.error }

type serverTerminalStatusError struct{ status string }

func (e *serverTerminalStatusError) Error() string { return "job already " + e.status + " on server" }

type statusHTTPError struct {
	code             int
	body, retryAfter string
}

func (e *statusHTTPError) Error() string {
	return fmt.Sprintf("status rejected: status=%d body=%s", e.code, e.body)
}

func retryableStatusFailure(err error) bool {
	var transport *retryableStatusError
	if errors.As(err, &transport) {
		return true
	}
	var response *statusHTTPError
	return errors.As(err, &response) && (response.code == 408 || response.code == 429 || response.code >= 500 && response.code <= 599)
}

func statusRetryAfter(err error) time.Duration {
	var response *statusHTTPError
	if !errors.As(err, &response) {
		return 0
	}
	if seconds, parseErr := strconv.ParseInt(response.retryAfter, 10, 32); parseErr == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if date, parseErr := http.ParseTime(response.retryAfter); parseErr == nil {
		return max(0, time.Until(date))
	}
	return 0
}

func reportJobStatusWithRetry(ctx context.Context, client *http.Client, serverURL, jobID string, req protocol.JobExecutionStatusUpdateRequest, sensitive []string, policy statusRetryPolicy) (resultErr error) {
	defer func() {
		if resultErr != nil {
			resultErr = &redactedStatusError{cause: resultErr, message: redactSensitive(resultErr.Error(), sensitive)}
		}
	}()
	// Freeze output and timestamps once: the server deduplicates exact retries.
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal job status: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, policy.budget)
	defer cancel()
	started := time.Now()
	backoff := policy.initialBackoff
	var lastErr error
	attempts := 0
	for {
		if ctx.Err() != nil {
			break
		}
		attempts++
		attemptCtx, stop := context.WithTimeout(ctx, policy.attemptTimeout)
		lastErr = sendJobStatus(attemptCtx, client, serverURL, jobID, body, req.Status)
		stop()
		if lastErr == nil && ctx.Err() != nil {
			break
		}
		if lastErr == nil {
			if attempts > 1 {
				slog.Info("job status reporting recovered", "job_execution_id", jobID, "current_step", redactSensitive(req.CurrentStep, sensitive), "attempts", attempts, "elapsed", time.Since(started))
			}
			return nil
		}
		if !retryableStatusFailure(lastErr) {
			slog.Error("job status reporting stopped", "job_execution_id", jobID, "current_step", redactSensitive(req.CurrentStep, sensitive), "attempts", attempts, "elapsed", time.Since(started), "error", redactSensitive(lastErr.Error(), sensitive))
			return lastErr
		}
		if ctx.Err() != nil {
			break
		}
		wait := min(policy.maxBackoff, time.Duration(float64(backoff)*(0.8+rand.Float64()*0.4)))
		wait = max(wait, statusRetryAfter(lastErr))
		deadline, _ := ctx.Deadline()
		remaining := time.Until(deadline)
		wait = min(wait, remaining)
		slog.Warn("job status report failed; retrying", "job_execution_id", jobID, "current_step", redactSensitive(req.CurrentStep, sensitive), "attempt", attempts, "elapsed", time.Since(started), "next_wait", wait, "error", redactSensitive(lastErr.Error(), sensitive))
		if wait >= remaining {
			// Do not race an independent timer against the context deadline:
			// Retry-After may prohibit another attempt for the entire budget.
			<-ctx.Done()
			break
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
		backoff = min(backoff*2, policy.maxBackoff)
	}
	err = fmt.Errorf("job status reporting stopped after %d attempts (%s): %w", attempts, time.Since(started).Round(time.Millisecond), errors.Join(ctx.Err(), lastErr))
	slog.Error("job status reporting exhausted", "job_execution_id", jobID, "current_step", redactSensitive(req.CurrentStep, sensitive), "error", redactSensitive(err.Error(), sensitive))
	return err
}

// Preserve classification and cancellation checks while keeping errors safe for
// callers that log or publish them without access to the job's secret values.
type redactedStatusError struct {
	cause   error
	message string
}

func (e *redactedStatusError) Error() string { return e.message }
func (e *redactedStatusError) Unwrap() error { return e.cause }
