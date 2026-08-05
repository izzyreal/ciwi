package server

import (
	"path/filepath"
	"testing"

	"github.com/izzyreal/ciwi/internal/store"
)

func TestServerInstallationIDIsStableInDatabase(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ciwi.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	first, err := ensureServerInstallationID(db)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ensureServerInstallationID(db)
	if err != nil {
		t.Fatal(err)
	}
	if first == "" || second != first {
		t.Fatalf("installation IDs = %q and %q", first, second)
	}
}
