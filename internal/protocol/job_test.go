package protocol

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestJobMarshalJSONOmitsUnsetStartedAndFinishedUTC(t *testing.T) {
	job := Job{
		ID:                   "job-1",
		Script:               "echo queued",
		RequiredCapabilities: map[string]string{},
		TimeoutSeconds:       30,
		Status:               "queued",
		CreatedUTC:           time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC),
	}

	raw, err := json.Marshal(job)
	if err != nil {
		t.Fatalf("marshal job: %v", err)
	}
	body := string(raw)
	if strings.Contains(body, `"started_utc"`) {
		t.Fatalf("expected started_utc omitted for zero time, body=%s", body)
	}
	if strings.Contains(body, `"finished_utc"`) {
		t.Fatalf("expected finished_utc omitted for zero time, body=%s", body)
	}
	if strings.Contains(body, `"leased_utc"`) {
		t.Fatalf("expected leased_utc omitted for zero time, body=%s", body)
	}
}

func TestJobMarshalJSONIncludesStartedAndFinishedUTCWhenSet(t *testing.T) {
	started := time.Date(2026, time.January, 2, 4, 0, 0, 0, time.UTC)
	finished := time.Date(2026, time.January, 2, 4, 2, 30, 0, time.UTC)
	job := Job{
		ID:                   "job-2",
		Script:               "echo done",
		RequiredCapabilities: map[string]string{},
		TimeoutSeconds:       30,
		Status:               "succeeded",
		CreatedUTC:           time.Date(2026, time.January, 2, 3, 59, 0, 0, time.UTC),
		StartedUTC:           started,
		FinishedUTC:          finished,
	}

	raw, err := json.Marshal(job)
	if err != nil {
		t.Fatalf("marshal job: %v", err)
	}
	body := string(raw)
	if !strings.Contains(body, `"started_utc":"2026-01-02T04:00:00Z"`) {
		t.Fatalf("expected started_utc in payload, body=%s", body)
	}
	if !strings.Contains(body, `"finished_utc":"2026-01-02T04:02:30Z"`) {
		t.Fatalf("expected finished_utc in payload, body=%s", body)
	}
}
