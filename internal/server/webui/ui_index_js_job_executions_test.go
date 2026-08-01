package webui

import (
	"strings"
	"testing"
)

func TestIndexJobCardHydrationBatchesMatchVisibleWindow(t *testing.T) {
	if !strings.Contains(indexJS, "const HISTORY_CARD_BATCH = HISTORY_CARD_WINDOW;") {
		t.Fatal("job card hydration should fetch each visible window in one batch")
	}

	for _, endpoint := range []string{
		"/api/v1/job-queue/cards?detail=summary",
		"/api/v1/job-queue/cards?detail=full",
		"/api/v1/job-history/cards?detail=summary",
		"/api/v1/job-history/cards?detail=full",
	} {
		if !strings.Contains(indexJS, endpoint) {
			t.Fatalf("job card hydration no longer requests %q", endpoint)
		}
	}
	if got := strings.Count(indexJS, "offset += HISTORY_CARD_BATCH"); got != 4 {
		t.Fatalf("expected all four job card hydration loops to use the window-sized batch, got %d", got)
	}
	if got := strings.Count(indexJS, "limit=' + String(HISTORY_CARD_BATCH)"); got != 4 {
		t.Fatalf("expected all four job card requests to use the window-sized batch limit, got %d", got)
	}
}

func TestSingleJobExecutionCardsAreCollapsibleInQueueAndHistory(t *testing.T) {
	if !strings.Contains(indexJS, `return total > 0;`) {
		t.Fatal("single-job execution cards should be collapsible")
	}
	if got := strings.Count(indexJS, `historyCardIsCollapsible(card)`); got < 7 {
		t.Fatalf("queue and history render paths must share collapsibility logic, got %d uses", got)
	}
}

func TestIndexJobSectionsProvidePersistentExpandAndCollapseAll(t *testing.T) {
	combined := indexHTML + indexJS
	for _, want := range []string{
		`id="queuedCollapseAllBtn"`,
		`id="queuedExpandAllBtn"`,
		`id="historyCollapseAllBtn"`,
		`id="historyExpandAllBtn"`,
		`icon-chevrons-up`,
		`icon-chevrons-down`,
		`function setAllJobExecutionGroupsExpanded(kind, expanded)`,
		`expandedJobGroups.add(groupKey)`,
		`expandedJobGroups.delete(groupKey)`,
		`saveStringSet(JOB_GROUPS_STORAGE_KEY, expandedJobGroups)`,
	} {
		if !strings.Contains(combined, want) {
			t.Errorf("bulk job expansion UI no longer contains %q", want)
		}
	}
}

func TestIndexAndSettingsHeadersShowServerVersionAfterCiwi(t *testing.T) {
	for name, page := range map[string]string{
		"index":    indexHTML,
		"settings": settingsHTML,
	} {
		if !strings.Contains(page, `ciwi <span class="ciwi-header-version" data-ciwi-server-version></span>`) {
			t.Errorf("%s header no longer places the server version after ciwi", name)
		}
	}
	for _, want := range []string{
		`function renderServerVersionLabels(version)`,
		`apiJSON('/api/v1/server-info')`,
		`refreshServerVersionLabels();`,
	} {
		if !strings.Contains(pagesJS+indexJS+settingsJS, want) {
			t.Errorf("server version header wiring no longer contains %q", want)
		}
	}
}
