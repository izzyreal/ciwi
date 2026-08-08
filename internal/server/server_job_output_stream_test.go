package server

import (
	"net/http/httptest"
	"testing"

	"github.com/izzyreal/ciwi/internal/application"
)

func TestJobOutputStreamCursorPrefersReconnectHeader(t *testing.T) {
	request := httptest.NewRequest("GET", "/output/stream?after_event_id=4", nil)
	request.Header.Set("Last-Event-ID", "9")
	cursor, err := jobOutputStreamCursor(request)
	if err != nil || cursor != 9 {
		t.Fatalf("cursor = %d, err = %v", cursor, err)
	}
}

func TestJobOutputChangeIsExecutionScoped(t *testing.T) {
	other := application.Change{Topics: []application.ChangeTopic{application.ChangeJobOutput}, JobExecutionIDs: []string{"job-2"}}
	if jobOutputChangeAffects(other, "job-1") {
		t.Fatal("another execution woke output stream")
	}
	current := application.Change{Topics: []application.ChangeTopic{application.ChangeJobOutput}, JobExecutionIDs: []string{"job-1"}}
	if !jobOutputChangeAffects(current, "job-1") {
		t.Fatal("current execution did not wake output stream")
	}
}
