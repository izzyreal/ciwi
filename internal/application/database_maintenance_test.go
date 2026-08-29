package application

import (
	"context"
	"testing"
)

type databaseMaintenanceBackendStub struct {
	calls  int
	result DatabaseVacuumResult
	err    error
}

func (s *databaseMaintenanceBackendStub) VacuumDatabase(context.Context) (DatabaseVacuumResult, error) {
	s.calls++
	return s.result, s.err
}

func TestDatabaseVacuumIsIdempotentAndFormatsResult(t *testing.T) {
	backend := &databaseMaintenanceBackendStub{result: DatabaseVacuumResult{
		BeforeBytes: 700 * 1024 * 1024, AfterBytes: 420 * 1024 * 1024,
		ReclaimedBytes: 280 * 1024 * 1024, ElapsedMS: 134000,
	}}
	operations := NewDatabaseMaintenanceOperations(backend, newReceiptRepositoryStub())
	request := DatabaseVacuumRequest{IdempotencyKey: "vacuum-1"}

	first, err := operations.Vacuum(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := operations.Vacuum(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if backend.calls != 1 || second != first {
		t.Fatalf("calls = %d, results = %#v / %#v", backend.calls, first, second)
	}
	if first.Message != "Database vacuumed: 700 MB → 420 MB in 2m14s" {
		t.Fatalf("message = %q", first.Message)
	}
}

func TestDatabaseVacuumPreservesTypedBackendError(t *testing.T) {
	backend := &databaseMaintenanceBackendStub{err: NewError(ErrorFailedPrecondition, "active executions", nil)}
	_, err := NewDatabaseMaintenanceOperations(backend, nil).Vacuum(t.Context(), DatabaseVacuumRequest{})
	if ErrorKindOf(err) != ErrorFailedPrecondition {
		t.Fatalf("error = %v, kind = %q", err, ErrorKindOf(err))
	}
}
