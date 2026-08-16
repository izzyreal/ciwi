//go:build darwin || ios || linux || windows

package gio

import (
	"context"
	"time"

	cnpv1 "github.com/izzyreal/ciwi/pkg/cnp/v1"
)

const nativeJobLogSearchDebounce = 250 * time.Millisecond

type nativeJobLogSearchClient interface {
	SearchJobLog(context.Context, *cnpv1.JobLogSearchRequest) (*cnpv1.JobLogSearchResult, error)
	GetJobLogPage(context.Context, *cnpv1.JobLogPageRequest) (*cnpv1.JobLogPage, error)
}

type nativeJobLogSearchRequest struct {
	generation    uint64
	jobID         string
	query         string
	selectedIndex int64
}

type nativeJobLogSearchResult struct {
	request nativeJobLogSearchRequest
	search  jobLogSearchSnapshot
	page    *jobLogStreamSnapshot
	failure string
	err     error
}

func executeNativeJobLogSearch(ctx context.Context, client nativeJobLogSearchClient, request nativeJobLogSearchRequest) nativeJobLogSearchResult {
	outcome := nativeJobLogSearchResult{request: request}
	result, err := client.SearchJobLog(ctx, &cnpv1.JobLogSearchRequest{
		JobExecutionId: request.jobID, Query: request.query, SelectedIndex: request.selectedIndex,
	})
	if err != nil {
		outcome.failure, outcome.err = "Output search failed", err
		return outcome
	}
	outcome.search = jobLogSearchSnapshot{
		JobID: result.GetJobExecutionId(), Query: result.GetQuery(),
		SelectedIndex: int(result.GetSelectedIndex()), TotalMatches: int(result.GetTotalMatches()),
	}
	if outcome.search.Query == "" {
		outcome.search.Query = request.query
	}
	match := result.GetMatch()
	if match == nil {
		return outcome
	}
	outcome.search.ItemID = match.GetItemId()
	outcome.search.ChunkID = match.GetChunkId()
	outcome.search.StartRune = int(match.GetStartRune())
	outcome.search.EndRune = int(match.GetEndRune())
	page, err := client.GetJobLogPage(ctx, &cnpv1.JobLogPageRequest{
		JobExecutionId: outcome.search.JobID, ItemId: outcome.search.ItemID,
		Mode: cnpv1.JobLogPageMode_JOB_LOG_PAGE_MODE_AROUND, Cursor: outcome.search.ChunkID,
	})
	if err != nil {
		outcome.failure, outcome.err = "Search result unavailable", err
		return outcome
	}
	pageSnapshot := jobLogPageFromProto(page)
	pageSnapshot.LoadedMode = "around"
	pageSnapshot.SelectedChunkID = outcome.search.ChunkID
	pageSnapshot.SelectedStartRune = outcome.search.StartRune
	pageSnapshot.SelectedEndRune = outcome.search.EndRune
	outcome.page = &pageSnapshot
	return outcome
}
