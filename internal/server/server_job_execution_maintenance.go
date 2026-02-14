package server

import (
	"context"
	"log/slog"
	"time"
)

const (
	jobExecutionMaintenanceInterval   = 10 * time.Second
	jobExecutionLeaseStaleAfter       = 45 * time.Second
	jobExecutionTimeoutReaperGrace    = 15 * time.Second
	jobExecutionTimeoutReaperErrorMsg = "job timed out while running (server maintenance)"
)

func (s *stateStore) runJobExecutionMaintenancePass(now time.Time) error {
	if s == nil || s.db == nil {
		return nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	requeued, err := s.db.RequeueStaleLeasedJobExecutions(now, jobExecutionLeaseStaleAfter)
	if err != nil {
		return err
	}
	failed, err := s.db.FailTimedOutRunningJobExecutions(now, jobExecutionTimeoutReaperGrace, jobExecutionTimeoutReaperErrorMsg)
	if err != nil {
		return err
	}
	if requeued > 0 || failed > 0 {
		slog.Warn("job execution maintenance applied", "requeued_stale_leased", requeued, "failed_timed_out_running", failed)
	}
	return nil
}

func (s *stateStore) runJobExecutionMaintenanceLoop(ctx context.Context) {
	ticker := time.NewTicker(jobExecutionMaintenanceInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.runJobExecutionMaintenancePass(time.Now().UTC()); err != nil {
				slog.Error("job execution maintenance pass failed", "error", err)
			}
		}
	}
}
