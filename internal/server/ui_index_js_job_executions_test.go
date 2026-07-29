package server

import (
	"strings"
	"testing"
)

func TestIndexJobCardHydrationBatchesMatchVisibleWindow(t *testing.T) {
	if !strings.Contains(uiIndexStateJS, "const HISTORY_CARD_BATCH = HISTORY_CARD_WINDOW;") {
		t.Fatal("job card hydration should fetch each visible window in one batch")
	}

	for _, endpoint := range []string{
		"/api/v1/job-queue/cards?detail=summary",
		"/api/v1/job-queue/cards?detail=full",
		"/api/v1/job-history/cards?detail=summary",
		"/api/v1/job-history/cards?detail=full",
	} {
		if !strings.Contains(uiIndexJobExecutionsJS, endpoint) {
			t.Fatalf("job card hydration no longer requests %q", endpoint)
		}
	}
	if got := strings.Count(uiIndexJobExecutionsJS, "offset += HISTORY_CARD_BATCH"); got != 4 {
		t.Fatalf("expected all four job card hydration loops to use the window-sized batch, got %d", got)
	}
	if got := strings.Count(uiIndexJobExecutionsJS, "limit=' + String(HISTORY_CARD_BATCH)"); got != 4 {
		t.Fatalf("expected all four job card requests to use the window-sized batch limit, got %d", got)
	}
}
