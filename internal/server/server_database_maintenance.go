package server

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/application"
)

const databaseVacuumDeadline = 5 * time.Minute

func (s *stateStore) vacuumDatabaseHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), databaseVacuumDeadline)
	defer cancel()
	result, err := s.app().databaseMaintenance.Vacuum(ctx, application.DatabaseVacuumRequest{
		IdempotencyKey: strings.TrimSpace(r.Header.Get("Idempotency-Key")),
	})
	if err != nil {
		writeApplicationHTTPError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
