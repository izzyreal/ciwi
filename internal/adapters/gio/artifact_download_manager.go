//go:build darwin || ios || linux || windows

package gio

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"gioui.org/x/explorer"
	"github.com/google/uuid"
	cnpv1 "github.com/izzyreal/ciwi/pkg/cnp/v1"
	"github.com/izzyreal/ciwi/pkg/cnpclient"
)

const (
	downloadCheckpointBytes    = int64(4 * 1024 * 1024)
	downloadCheckpointInterval = time.Second
	downloadUIInterval         = 100 * time.Millisecond
)

type nativeDownloadState string

const (
	downloadPreparing     nativeDownloadState = "preparing"
	downloadDownloading   nativeDownloadState = "downloading"
	downloadPaused        nativeDownloadState = "paused"
	downloadReadyToSave   nativeDownloadState = "ready-to-save"
	downloadSaving        nativeDownloadState = "saving"
	downloadCompleted     nativeDownloadState = "completed"
	downloadFailed        nativeDownloadState = "failed"
	downloadSourceChanged nativeDownloadState = "source-changed"
)

type nativeDownloadSnapshot struct {
	ID, Label, FileName, State, Error string
	Downloaded, Total                 int64
	UpdatedAt                         time.Time
	PausedByUser                      bool
	CanReveal, DestinationMissing     bool
}

type nativeDownloadStarted struct {
	ID, Label, FileName string
}

type nativeDownloadEvent struct {
	Started    *nativeDownloadStarted
	Completion *nativeDownloadCompletion
}

type nativeDownloadCompletion struct {
	ID, Label, FileName string
	Cancelled           bool
	Err                 error
}

type nativeDownloadRecord struct {
	ID                   string              `json:"id"`
	ServerInstallationID string              `json:"serverInstallationId"`
	JobExecutionID       string              `json:"jobExecutionId"`
	Kind                 string              `json:"kind"`
	Path                 string              `json:"path,omitempty"`
	Token                string              `json:"token,omitempty"`
	ContentID            string              `json:"contentId"`
	TotalSize            int64               `json:"totalSize"`
	Offset               int64               `json:"offset"`
	FileName             string              `json:"fileName"`
	Label                string              `json:"label"`
	StagePath            string              `json:"stagePath"`
	DestinationPath      string              `json:"destinationPath,omitempty"`
	State                nativeDownloadState `json:"state"`
	Error                string              `json:"error,omitempty"`
	CreatedAt            time.Time           `json:"createdAt"`
	UpdatedAt            time.Time           `json:"updatedAt"`
	PausedByUser         bool                `json:"pausedByUser,omitempty"`

	progressOffset int64
}

type nativeDownloadJournal struct {
	Downloads map[string]nativeDownloadRecord `json:"downloads"`
}

type artifactDownloadManager struct {
	ctx      context.Context
	cancel   context.CancelFunc
	picker   artifactDestinationPicker
	path     string
	stageDir string

	mu        sync.Mutex
	records   map[string]*nativeDownloadRecord
	active    map[string]context.CancelFunc
	client    artifactChunkClient
	serverID  string
	changed   chan struct{}
	events    chan nativeDownloadEvent
	completed chan nativeDownloadCompletion
	lastUI    time.Time
	uiTimer   *time.Timer
	closed    bool
}

func newArtifactDownloadManager(ctx context.Context, preferencesPath string, picker artifactDestinationPicker) (*artifactDownloadManager, error) {
	managerCtx, cancel := context.WithCancel(ctx)
	base := filepath.Dir(preferencesPath)
	m := &artifactDownloadManager{
		ctx: managerCtx, cancel: cancel, picker: picker,
		path: filepath.Join(base, "native-downloads.json"), stageDir: filepath.Join(base, "downloads"),
		records: map[string]*nativeDownloadRecord{}, active: map[string]context.CancelFunc{},
		changed: make(chan struct{}, 1), events: make(chan nativeDownloadEvent, 16), completed: make(chan nativeDownloadCompletion, 8),
	}
	if err := m.restore(); err != nil {
		cancel()
		return nil, err
	}
	return m, nil
}

func (m *artifactDownloadManager) Close() {
	if m == nil {
		return
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	if m.uiTimer != nil {
		m.uiTimer.Stop()
	}
	for _, cancel := range m.active {
		cancel()
	}
	m.mu.Unlock()
	m.cancel()
}

func (m *artifactDownloadManager) Changed() <-chan struct{}                   { return m.changed }
func (m *artifactDownloadManager) Events() <-chan nativeDownloadEvent         { return m.events }
func (m *artifactDownloadManager) Completed() <-chan nativeDownloadCompletion { return m.completed }

func (m *artifactDownloadManager) Snapshot() []nativeDownloadSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	snapshot := make([]nativeDownloadSnapshot, 0, len(m.records))
	for _, record := range m.records {
		offset := record.progressOffset
		if offset < record.Offset {
			offset = record.Offset
		}
		canReveal, destinationMissing := false, false
		if record.State == downloadCompleted && runtime.GOOS != "ios" && strings.TrimSpace(record.DestinationPath) != "" {
			info, err := os.Stat(record.DestinationPath)
			destinationMissing = err != nil || info.IsDir()
			canReveal = !destinationMissing
		}
		snapshot = append(snapshot, nativeDownloadSnapshot{
			ID: record.ID, Label: record.Label, FileName: record.FileName, State: string(record.State), Error: record.Error,
			Downloaded: offset, Total: record.TotalSize, UpdatedAt: record.UpdatedAt, PausedByUser: record.PausedByUser,
			CanReveal: canReveal, DestinationMissing: destinationMissing,
		})
	}
	sort.Slice(snapshot, func(i, j int) bool {
		iCompleted, jCompleted := snapshot[i].State == string(downloadCompleted), snapshot[j].State == string(downloadCompleted)
		if iCompleted != jCompleted {
			return !iCompleted
		}
		if !snapshot[i].UpdatedAt.Equal(snapshot[j].UpdatedAt) {
			return snapshot[i].UpdatedAt.After(snapshot[j].UpdatedAt)
		}
		return snapshot[i].ID < snapshot[j].ID
	})
	return snapshot
}

func (m *artifactDownloadManager) AttachClient(client artifactChunkClient, serverID string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.client, m.serverID = client, strings.TrimSpace(serverID)
	resume := make([]string, 0)
	if client != nil {
		for id, record := range m.records {
			if record.ServerInstallationID == m.serverID && record.State == downloadPaused && !record.PausedByUser {
				resume = append(resume, id)
			}
		}
	}
	m.mu.Unlock()
	for _, id := range resume {
		_ = m.Resume(id)
	}
}

func (m *artifactDownloadManager) DetachClient() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.client = nil
	for _, cancel := range m.active {
		cancel()
	}
	m.mu.Unlock()
}

func (m *artifactDownloadManager) Start(label string, arguments map[string]string) (string, bool, error) {
	if m == nil {
		return "", false, fmt.Errorf("download manager is unavailable")
	}
	jobID := strings.TrimSpace(arguments["jobExecutionId"])
	kind := strings.TrimSpace(arguments["kind"])
	path := strings.TrimSpace(arguments["path"])
	if jobID == "" || kind == "" {
		return "", false, fmt.Errorf("download source is incomplete")
	}
	if m.picker == nil {
		return "", false, fmt.Errorf("native save dialog is unavailable")
	}
	m.mu.Lock()
	if m.client == nil || m.serverID == "" {
		m.mu.Unlock()
		return "", false, fmt.Errorf("the server connection is not ready")
	}
	for _, record := range m.records {
		if record.ServerInstallationID == m.serverID && record.JobExecutionID == jobID && record.Kind == kind && record.Path == path && record.State != downloadCompleted {
			id := record.ID
			m.markChangedLocked(true)
			m.mu.Unlock()
			return id, true, nil
		}
	}
	id := uuid.NewString()
	now := time.Now().UTC()
	record := &nativeDownloadRecord{
		ID: id, ServerInstallationID: m.serverID, JobExecutionID: jobID, Kind: kind, Path: path,
		Label: defaultString(strings.TrimSpace(label), "Artifact"), State: downloadPreparing, CreatedAt: now, UpdatedAt: now,
	}
	m.records[id] = record
	client := m.client
	workerCtx, cancel := context.WithCancel(m.ctx)
	m.active[id] = cancel
	m.markChangedLocked(true)
	m.mu.Unlock()
	go m.prepare(workerCtx, client, id)
	return id, false, nil
}

func (m *artifactDownloadManager) prepare(ctx context.Context, client artifactChunkClient, id string) {
	record := m.recordCopy(id)
	if record == nil {
		return
	}
	request := &cnpv1.ArtifactDownloadRequest{JobExecutionId: record.JobExecutionID, Kind: record.Kind, Path: record.Path}
	chunk, err := client.DownloadArtifactChunk(ctx, request)
	if err != nil {
		m.finishFailure(id, fmt.Errorf("start download: %w", err), false)
		return
	}
	if strings.TrimSpace(chunk.GetContentId()) == "" {
		cancelArtifactDownload(ctx, client, chunk.GetToken())
		m.finishFailure(id, fmt.Errorf("server omitted the download content identity"), false)
		return
	}
	fileName := safeDownloadFileName(chunk.GetFileName())
	writer, err := m.picker.CreateFile(fileName)
	if err != nil {
		if !chunk.GetComplete() {
			cancelArtifactDownload(ctx, client, chunk.GetToken())
		}
		if errors.Is(err, explorer.ErrUserDecline) {
			m.removePreparing(id)
			m.sendCompletion(nativeDownloadCompletion{ID: id, Label: record.Label, FileName: fileName, Cancelled: true})
			return
		}
		m.finishFailure(id, fmt.Errorf("choose download destination: %w", err), false)
		return
	}
	if ctx.Err() != nil || m.recordCopy(id) == nil {
		_ = writer.Close()
		cancelArtifactDownload(m.ctx, client, chunk.GetToken())
		return
	}
	if err := os.MkdirAll(m.stageDir, 0o700); err != nil {
		_ = writer.Close()
		cancelArtifactDownload(ctx, client, chunk.GetToken())
		m.finishFailure(id, fmt.Errorf("create download staging directory: %w", err), false)
		return
	}
	stagePath := filepath.Join(m.stageDir, id+".part")
	destinationPath := downloadWriterPath(writer)
	m.mu.Lock()
	var saveErr error
	if current := m.records[id]; current != nil {
		current.FileName, current.StagePath, current.DestinationPath = fileName, stagePath, destinationPath
		current.Token, current.ContentID, current.TotalSize = chunk.GetToken(), chunk.GetContentId(), chunk.GetTotalSize()
		current.State, current.Error, current.UpdatedAt = downloadDownloading, "", time.Now().UTC()
		saveErr = m.saveLocked()
		if saveErr != nil {
			current.State = downloadFailed
			current.Error = "create durable download checkpoint: " + saveErr.Error()
		}
		m.markChangedLocked(true)
	}
	m.mu.Unlock()
	if saveErr != nil {
		_ = writer.Close()
		cancelArtifactDownload(m.ctx, client, chunk.GetToken())
		m.sendCompletion(nativeDownloadCompletion{ID: id, Label: record.Label, FileName: fileName, Err: saveErr})
		m.clearActive(id)
		return
	}
	m.sendStarted(nativeDownloadStarted{ID: id, Label: record.Label, FileName: fileName})
	m.run(ctx, client, id, writer, chunk)
}

func (m *artifactDownloadManager) Resume(id string) error {
	m.mu.Lock()
	record := m.records[id]
	if record == nil {
		m.mu.Unlock()
		return fmt.Errorf("download not found")
	}
	if _, active := m.active[id]; active {
		m.mu.Unlock()
		return nil
	}
	if record.StagePath == "" || record.ContentID == "" {
		copy := *record
		m.mu.Unlock()
		if err := m.Remove(id); err != nil {
			return err
		}
		_, _, err := m.Start(copy.Label, map[string]string{"jobExecutionId": copy.JobExecutionID, "kind": copy.Kind, "path": copy.Path})
		return err
	}
	if m.client == nil || record.ServerInstallationID != m.serverID {
		m.mu.Unlock()
		return fmt.Errorf("the original server is not connected")
	}
	if record.State == downloadReadyToSave || record.State == downloadCompleted || record.State == downloadSourceChanged {
		m.mu.Unlock()
		return fmt.Errorf("download cannot be resumed in state %s", record.State)
	}
	record.State, record.Error, record.UpdatedAt, record.PausedByUser = downloadDownloading, "", time.Now().UTC(), false
	client := m.client
	workerCtx, cancel := context.WithCancel(m.ctx)
	m.active[id] = cancel
	if err := m.saveLocked(); err != nil {
		delete(m.active, id)
		cancel()
		record.State = downloadPaused
		m.mu.Unlock()
		return fmt.Errorf("persist resumed download: %w", err)
	}
	m.markChangedLocked(true)
	m.mu.Unlock()
	go m.run(workerCtx, client, id, nil, nil)
	return nil
}

func (m *artifactDownloadManager) Pause(id string) error {
	m.mu.Lock()
	record := m.records[id]
	if record == nil {
		m.mu.Unlock()
		return fmt.Errorf("download not found")
	}
	if record.State == downloadPaused {
		record.PausedByUser = true
		record.UpdatedAt = time.Now().UTC()
		err := m.saveLocked()
		m.markChangedLocked(true)
		m.mu.Unlock()
		if err != nil {
			return fmt.Errorf("persist paused download: %w", err)
		}
		return nil
	}
	if record.State != downloadDownloading {
		m.mu.Unlock()
		return fmt.Errorf("download cannot be paused in state %s", record.State)
	}
	cancel := m.active[id]
	if cancel == nil {
		m.mu.Unlock()
		return fmt.Errorf("download is not active")
	}
	record.PausedByUser = true
	record.UpdatedAt = time.Now().UTC()
	if err := m.saveLocked(); err != nil {
		record.PausedByUser = false
		m.mu.Unlock()
		return fmt.Errorf("persist paused download: %w", err)
	}
	m.markChangedLocked(true)
	cancel()
	m.mu.Unlock()
	return nil
}

func (m *artifactDownloadManager) Cancel(id string) error {
	m.mu.Lock()
	record := m.records[id]
	if record == nil {
		m.mu.Unlock()
		return nil
	}
	if cancel := m.active[id]; cancel != nil {
		cancel()
	}
	client, token := m.client, record.Token
	label, fileName, stagePath := record.Label, record.FileName, record.StagePath
	delete(m.active, id)
	delete(m.records, id)
	if err := m.saveLocked(); err != nil {
		m.records[id] = record
		m.mu.Unlock()
		return fmt.Errorf("remove download checkpoint: %w", err)
	}
	m.markChangedLocked(true)
	m.mu.Unlock()
	if client != nil && token != "" {
		cancelArtifactDownload(m.ctx, client, token)
	}
	if stagePath != "" {
		_ = os.Remove(stagePath)
	}
	m.sendCompletion(nativeDownloadCompletion{ID: id, Label: label, FileName: fileName, Cancelled: true})
	return nil
}

func (m *artifactDownloadManager) Remove(id string) error {
	m.mu.Lock()
	record := m.records[id]
	if record == nil {
		m.mu.Unlock()
		return nil
	}
	if cancel := m.active[id]; cancel != nil {
		cancel()
	}
	stagePath := record.StagePath
	delete(m.active, id)
	delete(m.records, id)
	if err := m.saveLocked(); err != nil {
		m.records[id] = record
		m.mu.Unlock()
		return fmt.Errorf("remove download checkpoint: %w", err)
	}
	m.markChangedLocked(true)
	m.mu.Unlock()
	if stagePath != "" {
		_ = os.Remove(stagePath)
	}
	return nil
}

func (m *artifactDownloadManager) RevealPath(id string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	record := m.records[id]
	if record == nil {
		return "", fmt.Errorf("download not found")
	}
	if record.State != downloadCompleted {
		return "", fmt.Errorf("download is not complete")
	}
	path := strings.TrimSpace(record.DestinationPath)
	if runtime.GOOS == "ios" || path == "" {
		return "", fmt.Errorf("the saved file location is unavailable")
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return "", fmt.Errorf("the downloaded file is no longer available")
	}
	return path, nil
}

func (m *artifactDownloadManager) Restart(id string) error {
	record := m.recordCopy(id)
	if record == nil {
		return fmt.Errorf("download not found")
	}
	if err := m.Remove(id); err != nil {
		return err
	}
	_, _, err := m.Start(record.Label, map[string]string{"jobExecutionId": record.JobExecutionID, "kind": record.Kind, "path": record.Path})
	return err
}

func (m *artifactDownloadManager) Save(id string) error {
	m.mu.Lock()
	record := m.records[id]
	if record == nil || record.State != downloadReadyToSave {
		m.mu.Unlock()
		return fmt.Errorf("download is not ready to save")
	}
	if _, active := m.active[id]; active {
		m.mu.Unlock()
		return nil
	}
	record.State, record.Error = downloadSaving, ""
	workerCtx, cancel := context.WithCancel(m.ctx)
	m.active[id] = cancel
	m.markChangedLocked(true)
	copy := *record
	m.mu.Unlock()
	go m.export(workerCtx, copy)
	return nil
}

func (m *artifactDownloadManager) run(ctx context.Context, client artifactChunkClient, id string, destination io.WriteCloser, initial *cnpv1.ArtifactDownloadChunk) {
	record := m.recordCopy(id)
	if record == nil {
		if destination != nil {
			_ = destination.Close()
		}
		m.clearActive(id)
		return
	}
	stage, err := os.OpenFile(record.StagePath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		if destination != nil {
			_ = destination.Close()
		}
		m.finishFailure(id, fmt.Errorf("open staged download: %w", err), false)
		return
	}
	defer stage.Close()
	if err := stage.Truncate(record.Offset); err != nil {
		m.finishFailure(id, fmt.Errorf("recover staged download: %w", err), false)
		return
	}
	if _, err := stage.Seek(record.Offset, io.SeekStart); err != nil {
		m.finishFailure(id, fmt.Errorf("seek staged download: %w", err), false)
		return
	}
	if destination == nil {
		destination = reconcileDownloadDestination(record, stage)
		_, _ = stage.Seek(record.Offset, io.SeekStart)
	}
	progress := record.Offset
	checkpointOffset := record.Offset
	checkpointAt := time.Now()
	chunk := initial
	for {
		if err := ctx.Err(); err != nil {
			if destination != nil {
				_ = destination.Close()
			}
			m.pause(id, stage, progress, true)
			return
		}
		if chunk == nil {
			request := &cnpv1.ArtifactDownloadRequest{
				JobExecutionId: record.JobExecutionID, Kind: record.Kind, Path: record.Path,
				Token: record.Token, Offset: progress, ExpectedContentId: record.ContentID,
			}
			chunk, err = client.DownloadArtifactChunk(ctx, request)
			if err != nil && request.Token != "" && isDownloadTokenExpired(err) {
				request.Token = ""
				chunk, err = client.DownloadArtifactChunk(ctx, request)
			}
			if err != nil {
				if destination != nil {
					_ = destination.Close()
				}
				if ctx.Err() != nil || isDownloadUnavailable(err) {
					m.pause(id, stage, progress, ctx.Err() != nil)
				} else {
					m.finishFailure(id, fmt.Errorf("continue download: %w", err), isDownloadSourceChanged(err))
				}
				return
			}
		}
		if err := validateDownloadChunk(record, progress, chunk); err != nil {
			if destination != nil {
				_ = destination.Close()
			}
			m.finishFailure(id, err, true)
			return
		}
		if len(chunk.GetData()) > 0 {
			if err := writeDownloadChunk(stage, chunk.GetData()); err != nil {
				if destination != nil {
					_ = destination.Close()
				}
				m.finishFailure(id, fmt.Errorf("write staged download: %w", err), false)
				return
			}
			if destination != nil {
				if err := writeDownloadChunk(destination, chunk.GetData()); err != nil {
					_ = destination.Close()
					destination = nil
					m.detachDestination(id)
				}
			}
		}
		progress = chunk.GetNextOffset()
		record.Token = chunk.GetToken()
		m.updateProgress(id, progress, record.Token)
		if progress-checkpointOffset >= downloadCheckpointBytes || time.Since(checkpointAt) >= downloadCheckpointInterval || chunk.GetComplete() {
			if err := m.checkpoint(id, stage, progress, record.Token); err != nil {
				if destination != nil {
					_ = destination.Close()
				}
				m.finishFailure(id, err, false)
				return
			}
			checkpointOffset, checkpointAt = progress, time.Now()
		}
		if chunk.GetComplete() {
			break
		}
		chunk = nil
	}
	if err := verifyDownloadContent(stage, record.ContentID, record.TotalSize); err != nil {
		if destination != nil {
			_ = destination.Close()
		}
		m.finishFailure(id, err, true)
		return
	}
	if destination != nil {
		if err := destination.Close(); err != nil {
			destination = nil
			m.detachDestination(id)
		}
	}
	if destination == nil {
		m.clearActive(id)
		m.readyToSave(id)
		return
	}
	m.clearActive(id)
	m.complete(id)
}

func (m *artifactDownloadManager) export(ctx context.Context, record nativeDownloadRecord) {
	writer, err := m.picker.CreateFile(record.FileName)
	if err != nil {
		if errors.Is(err, explorer.ErrUserDecline) {
			m.clearActive(record.ID)
			m.readyToSave(record.ID)
			return
		}
		m.finishFailure(record.ID, fmt.Errorf("choose download destination: %w", err), false)
		return
	}
	if err := m.setDestinationPath(record.ID, downloadWriterPath(writer)); err != nil {
		_ = writer.Close()
		m.finishFailure(record.ID, fmt.Errorf("record download destination: %w", err), false)
		return
	}
	file, err := os.Open(record.StagePath)
	if err == nil {
		err = verifyDownloadContent(file, record.ContentID, record.TotalSize)
	}
	if err == nil {
		_, _ = file.Seek(0, io.SeekStart)
		_, err = copyWithContext(ctx, writer, file)
	}
	if file != nil {
		_ = file.Close()
	}
	closeErr := writer.Close()
	if err != nil {
		m.finishFailure(record.ID, fmt.Errorf("save download: %w", err), false)
		return
	}
	if closeErr != nil {
		m.finishFailure(record.ID, fmt.Errorf("close download destination: %w", closeErr), false)
		return
	}
	m.clearActive(record.ID)
	m.complete(record.ID)
}

func (m *artifactDownloadManager) checkpoint(id string, stage *os.File, offset int64, token string) error {
	if err := stage.Sync(); err != nil {
		return fmt.Errorf("sync staged download: %w", err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	record := m.records[id]
	if record == nil {
		return context.Canceled
	}
	record.Offset, record.progressOffset, record.Token, record.UpdatedAt = offset, offset, token, time.Now().UTC()
	if err := m.saveLocked(); err != nil {
		return fmt.Errorf("checkpoint download: %w", err)
	}
	return nil
}

func (m *artifactDownloadManager) pause(id string, stage *os.File, offset int64, resumeIfReattached bool) {
	record := m.recordCopy(id)
	if record == nil {
		return
	}
	if err := m.checkpoint(id, stage, offset, record.Token); err != nil {
		m.finishFailure(id, err, false)
		return
	}
	m.mu.Lock()
	resume := false
	if current := m.records[id]; current != nil {
		current.State, current.Error, current.UpdatedAt = downloadPaused, "", time.Now().UTC()
		_ = m.saveLocked()
		delete(m.active, id)
		resume = resumeIfReattached && !current.PausedByUser && m.client != nil && current.ServerInstallationID == m.serverID
		m.markChangedLocked(true)
	}
	m.mu.Unlock()
	if resume {
		_ = m.Resume(id)
	}
}

func (m *artifactDownloadManager) readyToSave(id string) {
	m.mu.Lock()
	if record := m.records[id]; record != nil {
		record.State, record.Error, record.UpdatedAt = downloadReadyToSave, "", time.Now().UTC()
		_ = m.saveLocked()
		m.markChangedLocked(true)
	}
	m.mu.Unlock()
}

func (m *artifactDownloadManager) complete(id string) {
	m.mu.Lock()
	record := m.records[id]
	if record == nil {
		m.mu.Unlock()
		return
	}
	completion := nativeDownloadCompletion{ID: id, Label: record.Label, FileName: record.FileName}
	stagePath := record.StagePath
	record.State, record.StagePath, record.Token, record.Error, record.PausedByUser = downloadCompleted, "", "", "", false
	record.progressOffset, record.Offset, record.UpdatedAt = record.TotalSize, record.TotalSize, time.Now().UTC()
	if err := m.saveLocked(); err != nil {
		record.State, record.StagePath = downloadFailed, stagePath
		record.Error = "finish durable download checkpoint: " + err.Error()
		m.markChangedLocked(true)
		m.sendCompletionLocked(nativeDownloadCompletion{ID: id, Label: record.Label, FileName: record.FileName, Err: err})
		m.mu.Unlock()
		return
	}
	m.markChangedLocked(true)
	m.mu.Unlock()
	_ = os.Remove(stagePath)
	m.sendCompletion(completion)
}

func (m *artifactDownloadManager) finishFailure(id string, err error, sourceChanged bool) {
	m.mu.Lock()
	if record := m.records[id]; record != nil {
		record.State = downloadFailed
		if sourceChanged {
			record.State = downloadSourceChanged
		}
		record.Error, record.UpdatedAt = err.Error(), time.Now().UTC()
		_ = m.saveLocked()
		m.markChangedLocked(true)
		m.sendCompletionLocked(nativeDownloadCompletion{ID: id, Label: record.Label, FileName: record.FileName, Err: err})
	}
	delete(m.active, id)
	m.mu.Unlock()
}

func (m *artifactDownloadManager) removePreparing(id string) {
	m.mu.Lock()
	delete(m.records, id)
	delete(m.active, id)
	m.markChangedLocked(true)
	m.mu.Unlock()
}

func (m *artifactDownloadManager) clearActive(id string) {
	m.mu.Lock()
	delete(m.active, id)
	m.mu.Unlock()
}

func (m *artifactDownloadManager) updateProgress(id string, offset int64, token string) {
	m.mu.Lock()
	if record := m.records[id]; record != nil {
		record.progressOffset, record.Token = offset, token
		record.State = downloadDownloading
		m.markChangedLocked(false)
	}
	m.mu.Unlock()
}

func (m *artifactDownloadManager) detachDestination(id string) {
	m.mu.Lock()
	if record := m.records[id]; record != nil {
		record.DestinationPath = ""
	}
	m.mu.Unlock()
}

func (m *artifactDownloadManager) setDestinationPath(id, destinationPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if record := m.records[id]; record != nil {
		record.DestinationPath = destinationPath
		record.UpdatedAt = time.Now().UTC()
		if err := m.saveLocked(); err != nil {
			return err
		}
		m.markChangedLocked(true)
		return nil
	}
	return fmt.Errorf("download not found")
}

func (m *artifactDownloadManager) recordCopy(id string) *nativeDownloadRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	if record := m.records[id]; record != nil {
		copy := *record
		return &copy
	}
	return nil
}

func (m *artifactDownloadManager) restore() error {
	payload, err := os.ReadFile(m.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read native download journal: %w", err)
	}
	var document nativeDownloadJournal
	if err := json.Unmarshal(payload, &document); err != nil {
		return fmt.Errorf("decode native download journal: %w", err)
	}
	for id, value := range document.Downloads {
		record := value
		if record.ID == "" {
			record.ID = id
		}
		if record.State == downloadCompleted {
			record.StagePath = ""
			record.Offset, record.progressOffset = record.TotalSize, record.TotalSize
			m.records[record.ID] = &record
			continue
		}
		if record.StagePath == "" {
			continue
		}
		info, statErr := os.Stat(record.StagePath)
		if statErr != nil || info.IsDir() {
			record.State = downloadFailed
			record.Error = "staged download is unavailable"
			record.Offset, record.progressOffset = 0, 0
		} else {
			offset := min(record.Offset, info.Size())
			file, openErr := os.OpenFile(record.StagePath, os.O_RDWR, 0o600)
			if openErr != nil || file.Truncate(offset) != nil {
				record.State = downloadFailed
				record.Error = "staged download could not be recovered"
			} else {
				record.Offset, record.progressOffset, record.State = offset, offset, downloadPaused
			}
			if file != nil {
				_ = file.Close()
			}
		}
		m.records[record.ID] = &record
	}
	entries, _ := os.ReadDir(m.stageDir)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".part" {
			continue
		}
		path := filepath.Join(m.stageDir, entry.Name())
		referenced := false
		for _, record := range m.records {
			if record.StagePath == path {
				referenced = true
				break
			}
		}
		if !referenced {
			_ = os.Remove(path)
		}
	}
	return m.saveLocked()
}

func (m *artifactDownloadManager) saveLocked() error {
	document := nativeDownloadJournal{Downloads: map[string]nativeDownloadRecord{}}
	for id, record := range m.records {
		if record.State == downloadPreparing {
			continue
		}
		copy := *record
		copy.progressOffset = 0
		document.Downloads[id] = copy
	}
	payload, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(m.path), "native-downloads-*.json")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(payload); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, m.path)
}

func (m *artifactDownloadManager) markChangedLocked(immediate bool) {
	if m.closed {
		return
	}
	now := time.Now()
	if immediate || m.lastUI.IsZero() || now.Sub(m.lastUI) >= downloadUIInterval {
		m.lastUI = now
		select {
		case m.changed <- struct{}{}:
		default:
		}
		return
	}
	if m.uiTimer != nil {
		return
	}
	delay := downloadUIInterval - now.Sub(m.lastUI)
	m.uiTimer = time.AfterFunc(delay, func() {
		m.mu.Lock()
		m.uiTimer = nil
		m.markChangedLocked(true)
		m.mu.Unlock()
	})
}

func (m *artifactDownloadManager) sendCompletion(completion nativeDownloadCompletion) {
	m.mu.Lock()
	m.sendCompletionLocked(completion)
	m.mu.Unlock()
}

func (m *artifactDownloadManager) sendStarted(started nativeDownloadStarted) {
	select {
	case m.events <- nativeDownloadEvent{Started: &started}:
	default:
	}
}

func (m *artifactDownloadManager) sendCompletionLocked(completion nativeDownloadCompletion) {
	select {
	case m.completed <- completion:
	default:
	}
	select {
	case m.events <- nativeDownloadEvent{Completion: &completion}:
	default:
	}
}

func validateDownloadChunk(record *nativeDownloadRecord, offset int64, chunk *cnpv1.ArtifactDownloadChunk) error {
	if chunk == nil || chunk.GetContentId() == "" || chunk.GetContentId() != record.ContentID {
		return fmt.Errorf("download source changed while transferring")
	}
	if chunk.GetTotalSize() != record.TotalSize || chunk.GetNextOffset() != offset+int64(len(chunk.GetData())) || chunk.GetNextOffset() < offset || chunk.GetNextOffset() > record.TotalSize {
		return fmt.Errorf("server returned invalid download progress")
	}
	if !chunk.GetComplete() && chunk.GetNextOffset() == offset {
		return fmt.Errorf("server returned stalled download progress")
	}
	return nil
}

func writeDownloadChunk(writer io.Writer, data []byte) error {
	written, err := writer.Write(data)
	if err != nil {
		return err
	}
	if written != len(data) {
		return io.ErrShortWrite
	}
	return nil
}

func verifyDownloadContent(file *os.File, contentID string, total int64) error {
	if file == nil {
		return fmt.Errorf("staged download is unavailable")
	}
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Size() != total {
		return fmt.Errorf("download size is %d, expected %d", info.Size(), total)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	want := strings.TrimPrefix(contentID, "sha256:")
	got := hex.EncodeToString(hash.Sum(nil))
	if want == contentID || !strings.EqualFold(got, want) {
		return fmt.Errorf("download content hash does not match the server")
	}
	return nil
}

func reconcileDownloadDestination(record *nativeDownloadRecord, stage *os.File) io.WriteCloser {
	if runtime.GOOS == "ios" || strings.TrimSpace(record.DestinationPath) == "" || record.Offset == 0 {
		return nil
	}
	destination, err := os.OpenFile(record.DestinationPath, os.O_RDWR, 0)
	if err != nil {
		return nil
	}
	if !sameFilePrefix(stage, destination, record.Offset) {
		_ = destination.Close()
		return nil
	}
	if err := destination.Truncate(record.Offset); err != nil {
		_ = destination.Close()
		return nil
	}
	if _, err := destination.Seek(record.Offset, io.SeekStart); err != nil {
		_ = destination.Close()
		return nil
	}
	return destination
}

func sameFilePrefix(first, second *os.File, size int64) bool {
	if first == nil || second == nil || size < 0 {
		return false
	}
	firstInfo, err1 := first.Stat()
	secondInfo, err2 := second.Stat()
	if err1 != nil || err2 != nil || firstInfo.Size() < size || secondInfo.Size() < size {
		return false
	}
	const block = 128 * 1024
	left, right := make([]byte, block), make([]byte, block)
	for offset := int64(0); offset < size; {
		length := int64(block)
		if size-offset < length {
			length = size - offset
		}
		if _, err := first.ReadAt(left[:length], offset); err != nil || !readAtEqual(second, right[:length], left[:length], offset) {
			return false
		}
		offset += length
	}
	return true
}

func readAtEqual(file *os.File, scratch, want []byte, offset int64) bool {
	if _, err := file.ReadAt(scratch, offset); err != nil {
		return false
	}
	return bytes.Equal(scratch, want)
}

func downloadWriterPath(writer io.WriteCloser) string {
	if runtime.GOOS == "ios" {
		return ""
	}
	if named, ok := writer.(interface{ Name() string }); ok {
		path := strings.TrimSpace(named.Name())
		if filepath.IsAbs(path) {
			return path
		}
	}
	return ""
}

func isDownloadTokenExpired(err error) bool {
	var remote *cnpclient.Error
	return errors.As(err, &remote) && remote.Code == cnpv1.StatusCode_STATUS_CODE_NOT_FOUND
}

func isDownloadSourceChanged(err error) bool {
	var remote *cnpclient.Error
	return errors.As(err, &remote) && remote.Code == cnpv1.StatusCode_STATUS_CODE_FAILED_PRECONDITION
}

func isDownloadUnavailable(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var remote *cnpclient.Error
	return errors.As(err, &remote) && remote.Code == cnpv1.StatusCode_STATUS_CODE_UNAVAILABLE
}

func copyWithContext(ctx context.Context, destination io.Writer, source io.Reader) (int64, error) {
	buffer := make([]byte, 256*1024)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		read, readErr := source.Read(buffer)
		if read > 0 {
			written, writeErr := destination.Write(buffer[:read])
			total += int64(written)
			if writeErr != nil {
				return total, writeErr
			}
			if written != read {
				return total, io.ErrShortWrite
			}
		}
		if readErr == io.EOF {
			return total, nil
		}
		if readErr != nil {
			return total, readErr
		}
	}
}
