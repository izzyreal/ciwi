//go:build darwin || ios || linux || windows

package gio

import (
	"context"
	"errors"
	"testing"

	cnpv1 "github.com/izzyreal/ciwi/pkg/cnp/v1"
)

type fakeNativeJobLogSearchClient struct {
	searchRequest *cnpv1.JobLogSearchRequest
	pageRequest   *cnpv1.JobLogPageRequest
	searchResult  *cnpv1.JobLogSearchResult
	pageResult    *cnpv1.JobLogPage
	searchErr     error
	pageErr       error
}

func (client *fakeNativeJobLogSearchClient) SearchJobLog(_ context.Context, request *cnpv1.JobLogSearchRequest) (*cnpv1.JobLogSearchResult, error) {
	client.searchRequest = request
	return client.searchResult, client.searchErr
}

func (client *fakeNativeJobLogSearchClient) GetJobLogPage(_ context.Context, request *cnpv1.JobLogPageRequest) (*cnpv1.JobLogPage, error) {
	client.pageRequest = request
	return client.pageResult, client.pageErr
}

func TestExecuteNativeJobLogSearchLoadsMatchPageAndPreservesRuneSpan(t *testing.T) {
	client := &fakeNativeJobLogSearchClient{
		searchResult: &cnpv1.JobLogSearchResult{
			JobExecutionId: "job-1", Query: "needle", SelectedIndex: 2, TotalMatches: 4,
			Match: &cnpv1.JobLogMatch{ItemId: "step:1", ChunkId: 9, StartRune: 11, EndRune: 17},
		},
		pageResult: &cnpv1.JobLogPage{
			JobExecutionId: "job-1", ItemId: "step:1",
			Chunks: []*cnpv1.JobLogChunk{{Id: 9, ItemId: "step:1", Text: "prefix needle suffix"}},
		},
	}
	request := nativeJobLogSearchRequest{generation: 3, jobID: "job-1", query: "needle", selectedIndex: 2}
	outcome := executeNativeJobLogSearch(context.Background(), client, request)
	if outcome.err != nil {
		t.Fatal(outcome.err)
	}
	if client.searchRequest.GetSelectedIndex() != 2 || client.pageRequest.GetMode() != cnpv1.JobLogPageMode_JOB_LOG_PAGE_MODE_AROUND || client.pageRequest.GetCursor() != 9 {
		t.Fatalf("search requests = search %+v page %+v", client.searchRequest, client.pageRequest)
	}
	if outcome.search.StartRune != 11 || outcome.search.EndRune != 17 || outcome.page == nil ||
		outcome.page.SelectedStartRune != 11 || outcome.page.SelectedEndRune != 17 {
		t.Fatalf("search outcome = %+v", outcome)
	}
}

func TestExecuteNativeJobLogSearchDoesNotLoadPageWithoutMatch(t *testing.T) {
	client := &fakeNativeJobLogSearchClient{searchResult: &cnpv1.JobLogSearchResult{
		JobExecutionId: "job-1", Query: "absent",
	}}
	outcome := executeNativeJobLogSearch(context.Background(), client, nativeJobLogSearchRequest{
		jobID: "job-1", query: "absent",
	})
	if outcome.err != nil || outcome.page != nil || client.pageRequest != nil {
		t.Fatalf("no-match outcome = %+v, page request = %+v", outcome, client.pageRequest)
	}
}

func TestExecuteNativeJobLogSearchReportsPageFailureSeparately(t *testing.T) {
	client := &fakeNativeJobLogSearchClient{
		searchResult: &cnpv1.JobLogSearchResult{
			JobExecutionId: "job-1", Query: "needle",
			Match: &cnpv1.JobLogMatch{ItemId: "step:1", ChunkId: 9},
		},
		pageErr: errors.New("page failed"),
	}
	outcome := executeNativeJobLogSearch(context.Background(), client, nativeJobLogSearchRequest{
		jobID: "job-1", query: "needle",
	})
	if !errors.Is(outcome.err, client.pageErr) || outcome.failure != "Search result unavailable" {
		t.Fatalf("page failure outcome = %+v", outcome)
	}
}
