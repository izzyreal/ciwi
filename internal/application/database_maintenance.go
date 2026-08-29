package application

import (
	"context"
	"fmt"
	"time"
)

const vacuumDatabaseOperation = "vacuum_database"

type DatabaseVacuumRequest struct {
	IdempotencyKey string
}

type DatabaseVacuumResult struct {
	BeforeBytes    int64  `json:"before_bytes"`
	AfterBytes     int64  `json:"after_bytes"`
	ReclaimedBytes int64  `json:"reclaimed_bytes"`
	ElapsedMS      int64  `json:"elapsed_ms"`
	Message        string `json:"message"`
}

type DatabaseMaintenanceBackend interface {
	VacuumDatabase(context.Context) (DatabaseVacuumResult, error)
}

type DatabaseMaintenanceOperations struct {
	backend  DatabaseMaintenanceBackend
	receipts CommandReceiptRepository
}

func NewDatabaseMaintenanceOperations(backend DatabaseMaintenanceBackend, receipts CommandReceiptRepository) *DatabaseMaintenanceOperations {
	return &DatabaseMaintenanceOperations{backend: backend, receipts: receipts}
}

func (o *DatabaseMaintenanceOperations) Vacuum(ctx context.Context, request DatabaseVacuumRequest) (DatabaseVacuumResult, error) {
	if o == nil || o.backend == nil {
		return DatabaseVacuumResult{}, NewError(ErrorUnavailable, "database maintenance unavailable", nil)
	}
	key, err := validateCommandKey(request.IdempotencyKey)
	if err != nil {
		return DatabaseVacuumResult{}, err
	}
	execute := func() (DatabaseVacuumResult, error) {
		result, err := o.backend.VacuumDatabase(ctx)
		if err != nil {
			return DatabaseVacuumResult{}, err
		}
		result.Message = fmt.Sprintf("Database vacuumed: %s → %s in %s", formatDatabaseBytes(result.BeforeBytes), formatDatabaseBytes(result.AfterBytes), formatVacuumElapsed(result.ElapsedMS))
		return result, nil
	}
	return executeIdempotentCommand(ctx, o.receipts, key, vacuumDatabaseOperation, vacuumDatabaseOperation, execute)
}

func formatDatabaseBytes(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	value := float64(bytes)
	units := []string{"KB", "MB", "GB", "TB"}
	for _, unit := range units {
		value /= 1024
		if value < 1024 || unit == units[len(units)-1] {
			if value >= 10 {
				return fmt.Sprintf("%.0f %s", value, unit)
			}
			return fmt.Sprintf("%.1f %s", value, unit)
		}
	}
	return fmt.Sprintf("%d B", bytes)
}

func formatVacuumElapsed(milliseconds int64) string {
	duration := time.Duration(milliseconds) * time.Millisecond
	if duration < time.Second {
		return duration.Round(time.Millisecond).String()
	}
	return duration.Round(time.Second).String()
}
