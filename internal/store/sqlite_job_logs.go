package store

import (
	"database/sql"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/izzyreal/ciwi/internal/domain"
	"github.com/izzyreal/ciwi/internal/logtext"
	"github.com/izzyreal/ciwi/internal/protocol"
)

const maxLogPageRows = 128

func appendIndexedLogEvent(tx *sql.Tx, jobID string, eventID int64, event protocol.JobExecutionEvent) error {
	itemID, text := logEventContent(event)
	text = logtext.Clean(text)
	if text == "" {
		return nil
	}
	chunks := logtext.Split(text, logtext.ChunkBytes)
	var tail string
	_ = tx.QueryRow(`
		SELECT tail_text FROM job_execution_log_streams
		WHERE job_execution_id = ? AND item_id = ?
	`, jobID, itemID).Scan(&tail)

	var firstID, lastID int64
	var bytesAdded int64
	for index, chunk := range chunks {
		overlap := logtext.TailRunes(tail, logtext.SearchOverlap)
		result, err := tx.Exec(`
			INSERT INTO job_execution_log_chunks
			(job_execution_id, event_id, item_id, event_chunk_index, text, indexed_text, overlap_runes, byte_count, rune_count)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, jobID, eventID, itemID, index, chunk, strings.ToLower(overlap+chunk),
			utf8.RuneCountInString(overlap), len(chunk), utf8.RuneCountInString(chunk))
		if err != nil {
			return fmt.Errorf("insert interactive log chunk: %w", err)
		}
		chunkID, err := result.LastInsertId()
		if err != nil {
			return fmt.Errorf("interactive log chunk id: %w", err)
		}
		if firstID == 0 {
			firstID = chunkID
		}
		lastID = chunkID
		bytesAdded += int64(len(chunk))
		tail = logtext.TailRunes(tail+chunk, logtext.SearchOverlap)
	}
	_, err := tx.Exec(`
		INSERT INTO job_execution_log_streams
		(job_execution_id, item_id, first_chunk_id, last_chunk_id, chunk_count, byte_count, tail_text)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(job_execution_id, item_id) DO UPDATE SET
			last_chunk_id = excluded.last_chunk_id,
			chunk_count = job_execution_log_streams.chunk_count + excluded.chunk_count,
			byte_count = job_execution_log_streams.byte_count + excluded.byte_count,
			tail_text = excluded.tail_text
	`, jobID, itemID, firstID, lastID, len(chunks), bytesAdded, tail)
	if err != nil {
		return fmt.Errorf("update interactive log stream: %w", err)
	}
	return nil
}

func logEventContent(event protocol.JobExecutionEvent) (string, string) {
	switch event.Type {
	case protocol.JobExecutionEventTypeSystemMessage:
		return "", event.Message
	case protocol.JobExecutionEventTypeStepOutput:
		if event.Step == nil || event.Step.Index <= 0 {
			return "", ""
		}
		return fmt.Sprintf("step:%d", event.Step.Index), event.Output
	case protocol.JobExecutionEventTypePhaseOutput:
		if event.Phase == nil {
			return "", ""
		}
		return strings.TrimSpace(event.Phase.ID), event.Output
	default:
		return "", ""
	}
}

func (s *Store) GetJobLogDescriptor(jobID string) (domain.JobLogDescriptor, error) {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return domain.JobLogDescriptor{}, fmt.Errorf("job id is required")
	}
	var version int
	var status string
	if err := s.db.QueryRow(`SELECT interactive_log_version, status FROM job_executions WHERE id = ?`, jobID).Scan(&version, &status); err != nil {
		if err == sql.ErrNoRows {
			return domain.JobLogDescriptor{}, fmt.Errorf("job not found")
		}
		return domain.JobLogDescriptor{}, fmt.Errorf("read job log descriptor: %w", err)
	}
	descriptor := domain.JobLogDescriptor{
		JobExecutionID: jobID, Version: version,
		Available: version == domain.InteractiveJobLogVersion,
		Terminal:  protocol.IsTerminalJobExecutionStatus(protocol.NormalizeJobExecutionStatus(status)),
	}
	if !descriptor.Available {
		return descriptor, nil
	}
	rows, err := s.db.Query(`
		SELECT item_id, first_chunk_id, last_chunk_id, chunk_count, byte_count
		FROM job_execution_log_streams WHERE job_execution_id = ? ORDER BY first_chunk_id ASC
	`, jobID)
	if err != nil {
		return domain.JobLogDescriptor{}, fmt.Errorf("list job log streams: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var stream domain.JobLogStream
		if err := rows.Scan(&stream.ItemID, &stream.FirstChunkID, &stream.LastChunkID, &stream.ChunkCount, &stream.ByteCount); err != nil {
			return domain.JobLogDescriptor{}, fmt.Errorf("scan job log stream: %w", err)
		}
		descriptor.Streams = append(descriptor.Streams, stream)
		if stream.LastChunkID > descriptor.LatestChunkID {
			descriptor.LatestChunkID = stream.LastChunkID
		}
	}
	if err := rows.Err(); err != nil {
		return domain.JobLogDescriptor{}, fmt.Errorf("iterate job log streams: %w", err)
	}
	return descriptor, nil
}

func (s *Store) GetJobLogPage(jobID, itemID string, mode domain.JobLogPageMode, cursor int64) (domain.JobLogPage, error) {
	descriptor, err := s.GetJobLogDescriptor(jobID)
	if err != nil {
		return domain.JobLogPage{}, err
	}
	if !descriptor.Available {
		return domain.JobLogPage{}, fmt.Errorf("interactive log unavailable for legacy job")
	}
	if cursor < 0 {
		return domain.JobLogPage{}, fmt.Errorf("log cursor must be non-negative")
	}
	page := domain.JobLogPage{JobExecutionID: jobID, ItemID: itemID, Terminal: descriptor.Terminal}
	switch mode {
	case domain.JobLogPageHead:
		page.Chunks, err = s.queryJobLogChunks(jobID, itemID, 0, true, false, logtext.PageBytes)
	case domain.JobLogPageTail:
		page.Chunks, err = s.queryJobLogChunks(jobID, itemID, 0, false, false, logtext.PageBytes)
	case domain.JobLogPageAfter:
		if err = s.validateJobLogCursor(jobID, itemID, cursor); err == nil {
			page.Chunks, err = s.queryJobLogChunks(jobID, itemID, cursor, true, true, logtext.PageBytes)
		}
	case domain.JobLogPageBefore:
		if err = s.validateJobLogCursor(jobID, itemID, cursor); err == nil {
			page.Chunks, err = s.queryJobLogChunks(jobID, itemID, cursor, false, true, logtext.PageBytes)
		}
	case domain.JobLogPageAround:
		if err = s.validateJobLogCursor(jobID, itemID, cursor); err == nil {
			var before, after []domain.JobLogChunk
			before, err = s.queryJobLogChunks(jobID, itemID, cursor+1, false, true, logtext.PageBytes/2)
			if err == nil {
				after, err = s.queryJobLogChunks(jobID, itemID, cursor, true, true, logtext.PageBytes/2)
				page.Chunks = append(before, after...)
			}
		}
	default:
		err = fmt.Errorf("invalid job log page mode %q", mode)
	}
	if err != nil {
		return domain.JobLogPage{}, err
	}
	if len(page.Chunks) == 0 {
		return page, nil
	}
	page.FirstCursor = page.Chunks[0].ID
	page.LastCursor = page.Chunks[len(page.Chunks)-1].ID
	page.HasBefore, err = s.jobLogChunkExists(jobID, itemID, page.FirstCursor, false)
	if err == nil {
		page.HasAfter, err = s.jobLogChunkExists(jobID, itemID, page.LastCursor, true)
	}
	return page, err
}

func (s *Store) validateJobLogCursor(jobID, itemID string, cursor int64) error {
	if cursor <= 0 {
		return fmt.Errorf("log cursor is required")
	}
	var found int
	err := s.db.QueryRow(`
		SELECT 1 FROM job_execution_log_chunks
		WHERE id = ? AND job_execution_id = ? AND item_id = ?
	`, cursor, jobID, itemID).Scan(&found)
	if err == sql.ErrNoRows {
		return fmt.Errorf("log cursor does not belong to the requested stream")
	}
	return err
}

func (s *Store) queryJobLogChunks(jobID, itemID string, cursor int64, ascending, exclusive bool, byteLimit int) ([]domain.JobLogChunk, error) {
	operator, order := ">", "ASC"
	if !ascending {
		operator, order = "<", "DESC"
	}
	if !exclusive {
		if ascending {
			operator = ">="
		} else {
			operator = "<="
		}
	}
	queryCursor := cursor
	if cursor == 0 && !ascending {
		queryCursor = int64(^uint64(0) >> 1)
	}
	rows, err := s.db.Query(`
		SELECT id, item_id, text, byte_count, rune_count
		FROM job_execution_log_chunks
		WHERE job_execution_id = ? AND item_id = ? AND id `+operator+` ?
		ORDER BY id `+order+` LIMIT ?
	`, jobID, itemID, queryCursor, maxLogPageRows)
	if err != nil {
		return nil, fmt.Errorf("query job log chunks: %w", err)
	}
	defer rows.Close()
	chunks := make([]domain.JobLogChunk, 0)
	bytesRead := 0
	for rows.Next() {
		var chunk domain.JobLogChunk
		if err := rows.Scan(&chunk.ID, &chunk.ItemID, &chunk.Text, &chunk.ByteCount, &chunk.RuneCount); err != nil {
			return nil, fmt.Errorf("scan job log chunk: %w", err)
		}
		if len(chunks) > 0 && bytesRead+chunk.ByteCount > byteLimit {
			break
		}
		chunks = append(chunks, chunk)
		bytesRead += chunk.ByteCount
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate job log chunks: %w", err)
	}
	if !ascending {
		slices.Reverse(chunks)
	}
	return chunks, nil
}

func (s *Store) jobLogChunkExists(jobID, itemID string, cursor int64, after bool) (bool, error) {
	operator := "<"
	if after {
		operator = ">"
	}
	var found int
	err := s.db.QueryRow(`
		SELECT 1 FROM job_execution_log_chunks
		WHERE job_execution_id = ? AND item_id = ? AND id `+operator+` ? LIMIT 1
	`, jobID, itemID, cursor).Scan(&found)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) SearchJobLog(jobID, query string, selectedIndex int64) (domain.JobLogSearchResult, error) {
	query = logtext.Clean(query)
	queryRunes := utf8.RuneCountInString(query)
	if queryRunes < logtext.SearchMinRunes || queryRunes > logtext.SearchMaxRunes {
		return domain.JobLogSearchResult{}, fmt.Errorf("search query must contain 3 to 256 characters")
	}
	if selectedIndex < 0 {
		return domain.JobLogSearchResult{}, fmt.Errorf("selected search index must be non-negative")
	}
	descriptor, err := s.GetJobLogDescriptor(jobID)
	if err != nil {
		return domain.JobLogSearchResult{}, err
	}
	if !descriptor.Available {
		return domain.JobLogSearchResult{}, fmt.Errorf("interactive log search unavailable for legacy job")
	}
	job, err := s.GetJobExecution(jobID)
	if err != nil {
		return domain.JobLogSearchResult{}, err
	}
	order := []string{""}
	seen := map[string]bool{"": true}
	for _, item := range protocol.BuildJobExecutionTimeline(job) {
		order = append(order, item.ID)
		seen[item.ID] = true
	}
	for _, stream := range descriptor.Streams {
		if !seen[stream.ItemID] {
			order = append(order, stream.ItemID)
			seen[stream.ItemID] = true
		}
	}

	result := domain.JobLogSearchResult{JobExecutionID: jobID, Query: query, SelectedIndex: selectedIndex}
	lowerQuery := []rune(strings.ToLower(query))
	ftsQuery := `job_execution_id : "` + escapeFTSPhrase(jobID) + `" AND indexed_text : "` + escapeFTSPhrase(strings.ToLower(query)) + `"`
	candidateQuery := jobLogSearchCandidateQuery(order)
	queryArguments := make([]any, 0, len(order)+2)
	queryArguments = append(queryArguments, ftsQuery, jobID)
	for _, itemID := range order {
		queryArguments = append(queryArguments, itemID)
	}
	rows, err := s.db.Query(candidateQuery, queryArguments...)
	if err != nil {
		return domain.JobLogSearchResult{}, fmt.Errorf("search job log: %w", err)
	}
	defer rows.Close()
	var first *domain.JobLogMatch
	for rows.Next() {
		var chunkID int64
		var foundItem, indexed string
		var overlap int
		if scanErr := rows.Scan(&chunkID, &foundItem, &indexed, &overlap); scanErr != nil {
			return domain.JobLogSearchResult{}, fmt.Errorf("scan job log search candidate: %w", scanErr)
		}
		for _, span := range runeSubstringMatches([]rune(indexed), lowerQuery) {
			if span[1] <= overlap {
				continue
			}
			match := domain.JobLogMatch{ItemID: foundItem, ChunkID: chunkID, StartRune: span[0] - overlap, EndRune: span[1] - overlap}
			if first == nil {
				copy := match
				first = &copy
			}
			if result.TotalMatches == selectedIndex {
				copy := match
				result.Match = &copy
			}
			result.TotalMatches++
		}
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return domain.JobLogSearchResult{}, fmt.Errorf("iterate job log search candidates: %w", rowsErr)
	}
	if result.TotalMatches > 0 && result.Match == nil {
		result.SelectedIndex = 0
		result.Match = first
	}
	return result, nil
}

// jobLogSearchCandidateQuery forces FTS to produce the selective candidate
// rowids before the ordinary table is consulted. CROSS JOIN is intentional:
// without it SQLite prefers idx_job_log_chunks_stream and probes FTS once for
// every chunk, which defeats the trigram index. The CASE expression preserves
// the user-visible system/timeline stream order without issuing one FTS query
// per stream.
func jobLogSearchCandidateQuery(itemOrder []string) string {
	var query strings.Builder
	query.WriteString(`
		SELECT c.id, c.item_id, c.indexed_text, c.overlap_runes
		FROM job_execution_log_chunks_fts AS f
		CROSS JOIN job_execution_log_chunks AS c ON c.id = f.rowid
		WHERE job_execution_log_chunks_fts MATCH ?
		  AND c.job_execution_id = ?
		ORDER BY CASE c.item_id`)
	for index := range itemOrder {
		query.WriteString(" WHEN ? THEN ")
		query.WriteString(strconv.Itoa(index))
	}
	query.WriteString(" ELSE ")
	query.WriteString(strconv.Itoa(len(itemOrder)))
	query.WriteString(" END, c.id ASC")
	return query.String()
}

func escapeFTSPhrase(value string) string {
	return strings.ReplaceAll(value, `"`, `""`)
}

func runeSubstringMatches(haystack, needle []rune) [][2]int {
	if len(needle) == 0 || len(haystack) < len(needle) {
		return nil
	}
	var matches [][2]int
	for offset := 0; offset <= len(haystack)-len(needle); {
		found := -1
		for index := offset; index <= len(haystack)-len(needle); index++ {
			if slices.Equal(haystack[index:index+len(needle)], needle) {
				found = index
				break
			}
		}
		if found < 0 {
			break
		}
		matches = append(matches, [2]int{found, found + len(needle)})
		offset = found + len(needle)
	}
	return matches
}
