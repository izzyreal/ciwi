package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/domain"
)

type jobLogDescriptorResponse struct {
	JobExecutionID string                 `json:"job_execution_id"`
	Version        int                    `json:"version"`
	Available      bool                   `json:"available"`
	Terminal       bool                   `json:"terminal"`
	LatestChunkID  int64                  `json:"latest_chunk_id"`
	Streams        []jobLogStreamResponse `json:"streams"`
}

type jobLogStreamResponse struct {
	ItemID       string `json:"item_id"`
	FirstChunkID int64  `json:"first_chunk_id"`
	LastChunkID  int64  `json:"last_chunk_id"`
	ChunkCount   int64  `json:"chunk_count"`
	ByteCount    int64  `json:"byte_count"`
}

type jobLogPageResponse struct {
	JobExecutionID string                `json:"job_execution_id"`
	ItemID         string                `json:"item_id"`
	Chunks         []jobLogChunkResponse `json:"chunks"`
	FirstCursor    int64                 `json:"first_cursor"`
	LastCursor     int64                 `json:"last_cursor"`
	HasBefore      bool                  `json:"has_before"`
	HasAfter       bool                  `json:"has_after"`
	Terminal       bool                  `json:"terminal"`
}

type jobLogChunkResponse struct {
	ID        int64  `json:"id"`
	ItemID    string `json:"item_id"`
	Text      string `json:"text"`
	ByteCount int    `json:"byte_count"`
	RuneCount int    `json:"rune_count"`
}

type jobLogSearchRequest struct {
	Query         string `json:"query"`
	SelectedIndex int64  `json:"selected_index"`
}

type jobLogSearchResponse struct {
	JobExecutionID string               `json:"job_execution_id"`
	Query          string               `json:"query"`
	SelectedIndex  int64                `json:"selected_index"`
	TotalMatches   int64                `json:"total_matches"`
	Match          *jobLogMatchResponse `json:"match,omitempty"`
}

type jobLogMatchResponse struct {
	ItemID    string `json:"item_id"`
	ChunkID   int64  `json:"chunk_id"`
	StartRune int    `json:"start_rune"`
	EndRune   int    `json:"end_rune"`
}

func (s *stateStore) jobLogViewHandler(w http.ResponseWriter, r *http.Request, jobID, operation string) {
	switch operation {
	case "descriptor":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		descriptor, err := s.app().jobDetails.GetJobLogDescriptor(r.Context(), jobID)
		if err != nil {
			http.Error(w, err.Error(), applicationErrorHTTPStatus(err))
			return
		}
		writeJSON(w, http.StatusOK, jobLogDescriptorToResponse(descriptor))
	case "page":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		mode := domain.JobLogPageMode(strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mode"))))
		if mode == "" {
			mode = domain.JobLogPageHead
		}
		cursor := int64(0)
		if raw := strings.TrimSpace(r.URL.Query().Get("cursor")); raw != "" {
			parsed, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || parsed < 0 {
				http.Error(w, "cursor must be a non-negative integer", http.StatusBadRequest)
				return
			}
			cursor = parsed
		}
		page, err := s.app().jobDetails.GetJobLogPage(r.Context(), jobID, r.URL.Query().Get("item_id"), mode, cursor)
		if err != nil {
			http.Error(w, err.Error(), applicationErrorHTTPStatus(err))
			return
		}
		writeJSON(w, http.StatusOK, jobLogPageToResponse(page))
	case "search":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var request jobLogSearchRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4*1024))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			http.Error(w, "invalid search request", http.StatusBadRequest)
			return
		}
		result, err := s.app().jobDetails.SearchJobLog(r.Context(), jobID, request.Query, request.SelectedIndex)
		if err != nil {
			http.Error(w, err.Error(), applicationErrorHTTPStatus(err))
			return
		}
		writeJSON(w, http.StatusOK, jobLogSearchToResponse(result))
	case "stream":
		s.jobLogChangeStreamHandler(w, r, jobID)
	default:
		http.NotFound(w, r)
	}
}

func jobLogDescriptorToResponse(descriptor domain.JobLogDescriptor) jobLogDescriptorResponse {
	response := jobLogDescriptorResponse{
		JobExecutionID: descriptor.JobExecutionID, Version: descriptor.Version, Available: descriptor.Available,
		Terminal: descriptor.Terminal, LatestChunkID: descriptor.LatestChunkID,
		Streams: make([]jobLogStreamResponse, 0, len(descriptor.Streams)),
	}
	for _, stream := range descriptor.Streams {
		response.Streams = append(response.Streams, jobLogStreamResponse{
			ItemID: stream.ItemID, FirstChunkID: stream.FirstChunkID, LastChunkID: stream.LastChunkID,
			ChunkCount: stream.ChunkCount, ByteCount: stream.ByteCount,
		})
	}
	return response
}

func jobLogPageToResponse(page domain.JobLogPage) jobLogPageResponse {
	response := jobLogPageResponse{
		JobExecutionID: page.JobExecutionID, ItemID: page.ItemID, FirstCursor: page.FirstCursor,
		LastCursor: page.LastCursor, HasBefore: page.HasBefore, HasAfter: page.HasAfter, Terminal: page.Terminal,
		Chunks: make([]jobLogChunkResponse, 0, len(page.Chunks)),
	}
	for _, chunk := range page.Chunks {
		response.Chunks = append(response.Chunks, jobLogChunkResponse{
			ID: chunk.ID, ItemID: chunk.ItemID, Text: chunk.Text, ByteCount: chunk.ByteCount, RuneCount: chunk.RuneCount,
		})
	}
	return response
}

func jobLogSearchToResponse(result domain.JobLogSearchResult) jobLogSearchResponse {
	response := jobLogSearchResponse{
		JobExecutionID: result.JobExecutionID, Query: result.Query,
		SelectedIndex: result.SelectedIndex, TotalMatches: result.TotalMatches,
	}
	if result.Match != nil {
		response.Match = &jobLogMatchResponse{
			ItemID: result.Match.ItemID, ChunkID: result.Match.ChunkID,
			StartRune: result.Match.StartRune, EndRune: result.Match.EndRune,
		}
	}
	return response
}

func (s *stateStore) jobLogChangeStreamHandler(w http.ResponseWriter, r *http.Request, jobID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	after, err := jobLogStreamCursor(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	if _, err := fmt.Fprint(w, "retry: 1000\n\n"); err != nil {
		return
	}
	flusher.Flush()
	changes := s.app().changes.Watch(r.Context())
	heartbeat := time.NewTicker(jobOutputHeartbeatInterval)
	defer heartbeat.Stop()

	emit := func() (bool, error) {
		descriptor, err := s.app().jobDetails.GetJobLogDescriptor(r.Context(), jobID)
		if err != nil {
			return false, err
		}
		if descriptor.LatestChunkID > after || descriptor.Terminal {
			payload, _ := json.Marshal(jobLogDescriptorToResponse(descriptor))
			if err := writeSSE(w, flusher, "change", strconv.FormatInt(descriptor.LatestChunkID, 10), payload); err != nil {
				return false, err
			}
			after = descriptor.LatestChunkID
		}
		return descriptor.Terminal, nil
	}
	terminal, err := emit()
	if err != nil {
		writeJobOutputSSEError(w, flusher, err)
		return
	}
	if terminal {
		_ = writeSSE(w, flusher, "complete", strconv.FormatInt(after, 10), []byte(`{"terminal":true}`))
		return
	}
	for {
		select {
		case change, ok := <-changes:
			if !ok {
				return
			}
			if !jobOutputChangeAffects(change, jobID) {
				continue
			}
			terminal, err := emit()
			if err != nil {
				writeJobOutputSSEError(w, flusher, err)
				return
			}
			if terminal {
				_ = writeSSE(w, flusher, "complete", strconv.FormatInt(after, 10), []byte(`{"terminal":true}`))
				return
			}
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func jobLogStreamCursor(r *http.Request) (int64, error) {
	raw := strings.TrimSpace(r.Header.Get("Last-Event-ID"))
	if raw == "" {
		raw = strings.TrimSpace(r.URL.Query().Get("after_chunk_id"))
	}
	if raw == "" {
		return 0, nil
	}
	cursor, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || cursor < 0 {
		return 0, fmt.Errorf("after_chunk_id must be a non-negative integer")
	}
	return cursor, nil
}
