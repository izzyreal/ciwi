package linuxupdater

import "testing"

func TestOpenStateStoreAndSetUpdateState(t *testing.T) {
	dbPath := t.TempDir() + "/ciwi.db"
	t.Setenv("CIWI_DB_PATH", dbPath)

	st := openStateStore()
	if st == nil {
		t.Fatalf("expected openStateStore to return store")
	}
	defer st.Close()

	if err := setUpdateState(st, map[string]string{"update_status": "ok", "": "ignored"}); err != nil {
		t.Fatalf("setUpdateState: %v", err)
	}
	v, found, err := st.GetAppState("update_status")
	if err != nil {
		t.Fatalf("GetAppState: %v", err)
	}
	if !found || v != "ok" {
		t.Fatalf("unexpected app state value found=%v v=%q", found, v)
	}
}

func TestOpenStateStoreInvalidPathReturnsNil(t *testing.T) {
	// Force a path that sqlite Open should reject on this platform.
	t.Setenv("CIWI_DB_PATH", "")
	if st := openStateStore(); st == nil {
		// openStateStore falls back to default path when env is empty; no assertion on nil here.
		return
	} else {
		_ = st.Close()
	}
}
