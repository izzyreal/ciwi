//go:build darwin || ios || linux || windows

package gio

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"gioui.org/x/explorer"
	cnpv1 "github.com/izzyreal/ciwi/pkg/cnp/v1"
	"github.com/izzyreal/ciwi/pkg/cnpclient"
	"google.golang.org/protobuf/proto"
)

type resumableDownloadClientStub struct {
	mu             sync.Mutex
	content        []byte
	chunkSize      int
	failAfterFirst bool
	calls          int
	requests       []*cnpv1.ArtifactDownloadRequest
}

func (s *resumableDownloadClientStub) DownloadArtifactChunk(_ context.Context, request *cnpv1.ArtifactDownloadRequest) (*cnpv1.ArtifactDownloadChunk, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	s.requests = append(s.requests, proto.Clone(request).(*cnpv1.ArtifactDownloadRequest))
	if request.GetCancel() {
		return &cnpv1.ArtifactDownloadChunk{Token: request.GetToken(), Complete: true}, nil
	}
	if s.failAfterFirst && s.calls > 1 {
		return nil, &cnpclient.Error{Code: cnpv1.StatusCode_STATUS_CODE_UNAVAILABLE, Message: "disconnected"}
	}
	contentID := testDownloadContentID(s.content)
	if request.GetToken() == "expired" {
		return nil, &cnpclient.Error{Code: cnpv1.StatusCode_STATUS_CODE_NOT_FOUND, Message: "expired"}
	}
	if request.GetOffset() > 0 && request.GetExpectedContentId() != contentID {
		return nil, &cnpclient.Error{Code: cnpv1.StatusCode_STATUS_CODE_FAILED_PRECONDITION, Message: "changed"}
	}
	start := request.GetOffset()
	end := start + int64(s.chunkSize)
	if end > int64(len(s.content)) {
		end = int64(len(s.content))
	}
	return &cnpv1.ArtifactDownloadChunk{
		Token: "expired", FileName: "artifact.bin", ContentId: contentID,
		Data: append([]byte(nil), s.content[start:end]...), NextOffset: end, TotalSize: int64(len(s.content)), Complete: end == int64(len(s.content)),
	}, nil
}

type pickerSequence struct {
	mu      sync.Mutex
	writers []io.WriteCloser
	calls   int
}

type failingPicker struct{ err error }

func (p failingPicker) CreateFile(string) (io.WriteCloser, error) { return nil, p.err }

func (p *pickerSequence) CreateFile(string) (io.WriteCloser, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.calls >= len(p.writers) {
		return nil, errors.New("unexpected picker call")
	}
	writer := p.writers[p.calls]
	p.calls++
	return writer, nil
}

type failDownloadWriter struct{ closed bool }

func (w *failDownloadWriter) Write([]byte) (int, error) {
	return 0, errors.New("destination unavailable")
}
func (w *failDownloadWriter) Close() error { w.closed = true; return nil }

func TestArtifactDownloadManagerRecoversAfterClientAndServerRestart(t *testing.T) {
	configDir := t.TempDir()
	preferencesPath := filepath.Join(configDir, "native-ui.json")
	destinationPath := filepath.Join(t.TempDir(), "artifact.bin")
	destination, err := os.Create(destinationPath)
	if err != nil {
		t.Fatal(err)
	}
	content := bytes.Repeat([]byte("resumable-content-"), 80)
	firstClient := &resumableDownloadClientStub{content: content, chunkSize: 301, failAfterFirst: true}
	manager, err := newArtifactDownloadManager(t.Context(), preferencesPath, &pickerSequence{writers: []io.WriteCloser{destination}})
	if err != nil {
		t.Fatal(err)
	}
	manager.AttachClient(firstClient, "server-1")
	if _, _, err := manager.Start("Artifact", map[string]string{"jobExecutionId": "job-1", "kind": "file", "path": "artifact.bin"}); err != nil {
		t.Fatal(err)
	}
	waitForDownloadState(t, manager, downloadPaused)
	manager.Close()

	restartedClient := &resumableDownloadClientStub{content: content, chunkSize: 211}
	restored, err := newArtifactDownloadManager(t.Context(), preferencesPath, &pickerSequence{})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	restored.AttachClient(restartedClient, "server-1")
	waitForDownloadState(t, restored, downloadCompleted)
	payload, err := os.ReadFile(destinationPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(payload, content) {
		t.Fatalf("destination contains %d bytes, want %d exact bytes", len(payload), len(content))
	}
	restartedClient.mu.Lock()
	requests := append([]*cnpv1.ArtifactDownloadRequest(nil), restartedClient.requests...)
	restartedClient.mu.Unlock()
	if len(requests) < 2 || requests[0].GetToken() != "expired" || requests[1].GetToken() != "" || requests[1].GetOffset() == 0 || requests[1].GetExpectedContentId() != testDownloadContentID(content) {
		t.Fatalf("resume requests = %+v", requests)
	}
}

func TestArtifactDownloadManagerRecoveryTruncatesUncheckpointedBytes(t *testing.T) {
	configDir := t.TempDir()
	stageDir := filepath.Join(configDir, "downloads")
	if err := os.MkdirAll(stageDir, 0o700); err != nil {
		t.Fatal(err)
	}
	stagePath := filepath.Join(stageDir, "download.part")
	if err := os.WriteFile(stagePath, []byte("durable-extra"), 0o600); err != nil {
		t.Fatal(err)
	}
	record := nativeDownloadRecord{
		ID: "download", ServerInstallationID: "server-1", JobExecutionID: "job-1", Kind: "file", ContentID: testDownloadContentID([]byte("durable")),
		TotalSize: 20, Offset: 7, FileName: "artifact.bin", Label: "Artifact", StagePath: stagePath, State: downloadDownloading,
	}
	payload, err := json.Marshal(nativeDownloadJournal{Downloads: map[string]nativeDownloadRecord{"download": record}})
	if err != nil {
		t.Fatal(err)
	}
	journalPath := filepath.Join(configDir, "native-downloads.json")
	if err := os.WriteFile(journalPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	manager, err := newArtifactDownloadManager(t.Context(), filepath.Join(configDir, "native-ui.json"), &pickerSequence{})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	info, err := os.Stat(stagePath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 7 || manager.Snapshot()[0].State != string(downloadPaused) {
		t.Fatalf("restored size/state = %d/%s", info.Size(), manager.Snapshot()[0].State)
	}
}

func TestArtifactDownloadManagerStagesThroughDestinationFailureAndSavesLater(t *testing.T) {
	configDir := t.TempDir()
	content := bytes.Repeat([]byte("payload"), 200)
	client := &resumableDownloadClientStub{content: content, chunkSize: 173}
	failed := &failDownloadWriter{}
	savedPath := filepath.Join(t.TempDir(), "saved-artifact.bin")
	saved, err := os.Create(savedPath)
	if err != nil {
		t.Fatal(err)
	}
	picker := &pickerSequence{writers: []io.WriteCloser{failed, saved}}
	manager, err := newArtifactDownloadManager(t.Context(), filepath.Join(configDir, "native-ui.json"), picker)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	manager.AttachClient(client, "server-1")
	id, _, err := manager.Start("Artifact", map[string]string{"jobExecutionId": "job-1", "kind": "file", "path": "artifact.bin"})
	if err != nil {
		t.Fatal(err)
	}
	waitForDownloadState(t, manager, downloadReadyToSave)
	if !failed.closed {
		t.Fatal("failed destination was not closed")
	}
	if err := manager.Save(id); err != nil {
		t.Fatal(err)
	}
	waitForDownloadState(t, manager, downloadCompleted)
	payload, err := os.ReadFile(savedPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(payload, content) {
		t.Fatalf("saved payload = %d bytes, want %d", len(payload), len(content))
	}
	record := manager.recordCopy(id)
	if record == nil || record.DestinationPath != savedPath {
		t.Fatalf("saved destination = %+v, want %q", record, savedPath)
	}
}

func TestArtifactDownloadManagerManualPauseSurvivesRestoreWithoutAutoResume(t *testing.T) {
	configDir := t.TempDir()
	stageDir := filepath.Join(configDir, "downloads")
	if err := os.MkdirAll(stageDir, 0o700); err != nil {
		t.Fatal(err)
	}
	stagePath := filepath.Join(stageDir, "manual.part")
	if err := os.WriteFile(stagePath, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager, err := newArtifactDownloadManager(t.Context(), filepath.Join(configDir, "native-ui.json"), &pickerSequence{})
	if err != nil {
		t.Fatal(err)
	}
	workerCtx, cancel := context.WithCancel(t.Context())
	manager.mu.Lock()
	manager.serverID = "server-1"
	manager.records["manual"] = &nativeDownloadRecord{
		ID: "manual", ServerInstallationID: "server-1", JobExecutionID: "job-1", Kind: "file",
		ContentID: testDownloadContentID([]byte("partial-more")), TotalSize: 12, Offset: 7,
		FileName: "artifact.bin", Label: "Artifact", StagePath: stagePath, State: downloadDownloading,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	manager.active["manual"] = cancel
	manager.mu.Unlock()
	if err := manager.Pause("manual"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-workerCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("manual pause did not cancel the active worker")
	}
	manager.Close()

	restored, err := newArtifactDownloadManager(t.Context(), filepath.Join(configDir, "native-ui.json"), &pickerSequence{})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	client := &resumableDownloadClientStub{content: []byte("partial-more"), chunkSize: 5}
	restored.AttachClient(client, "server-1")
	time.Sleep(50 * time.Millisecond)
	client.mu.Lock()
	calls := client.calls
	client.mu.Unlock()
	snapshot := restored.Snapshot()
	if calls != 0 || len(snapshot) != 1 || snapshot[0].State != string(downloadPaused) || !snapshot[0].PausedByUser {
		t.Fatalf("restored manual pause = calls %d, snapshot %+v", calls, snapshot)
	}
}

func TestArtifactDownloadManagerPersistsCompletedHistoryAndRemoveKeepsFile(t *testing.T) {
	configDir := t.TempDir()
	destinationPath := filepath.Join(t.TempDir(), "artifact.bin")
	if err := os.WriteFile(destinationPath, []byte("complete"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager, err := newArtifactDownloadManager(t.Context(), filepath.Join(configDir, "native-ui.json"), &pickerSequence{})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	manager.mu.Lock()
	manager.records["complete"] = &nativeDownloadRecord{
		ID: "complete", FileName: "artifact.bin", Label: "Artifact", State: downloadCompleted,
		TotalSize: 8, Offset: 8, DestinationPath: destinationPath, CreatedAt: now, UpdatedAt: now,
	}
	if err := manager.saveLocked(); err != nil {
		manager.mu.Unlock()
		t.Fatal(err)
	}
	manager.mu.Unlock()
	manager.Close()

	restored, err := newArtifactDownloadManager(t.Context(), filepath.Join(configDir, "native-ui.json"), &pickerSequence{})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	snapshot := restored.Snapshot()
	if len(snapshot) != 1 || snapshot[0].State != string(downloadCompleted) || !snapshot[0].CanReveal || snapshot[0].DestinationMissing {
		t.Fatalf("restored completed download = %+v", snapshot)
	}
	if err := restored.Remove("complete"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(destinationPath); err != nil {
		t.Fatalf("removing history affected downloaded file: %v", err)
	}
}

func TestArtifactDownloadManagerMarksMissingCompletedDestination(t *testing.T) {
	manager, err := newArtifactDownloadManager(t.Context(), filepath.Join(t.TempDir(), "native-ui.json"), &pickerSequence{})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	manager.mu.Lock()
	manager.records["missing"] = &nativeDownloadRecord{
		ID: "missing", State: downloadCompleted, DestinationPath: filepath.Join(t.TempDir(), "missing.bin"), UpdatedAt: time.Now().UTC(),
	}
	manager.mu.Unlock()
	snapshot := manager.Snapshot()
	if len(snapshot) != 1 || snapshot[0].CanReveal || !snapshot[0].DestinationMissing {
		t.Fatalf("missing destination snapshot = %+v", snapshot)
	}
	if _, err := manager.RevealPath("missing"); err == nil {
		t.Fatal("missing completed destination was revealable")
	}
}

func TestArtifactDownloadManagerPickerCancellationLeavesNoJournal(t *testing.T) {
	configDir := t.TempDir()
	client := &resumableDownloadClientStub{content: []byte("payload"), chunkSize: 7}
	manager, err := newArtifactDownloadManager(t.Context(), filepath.Join(configDir, "native-ui.json"), failingPicker{err: explorer.ErrUserDecline})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	manager.AttachClient(client, "server-1")
	if _, _, err := manager.Start("Artifact", map[string]string{"jobExecutionId": "job-1", "kind": "file", "path": "artifact.bin"}); err != nil {
		t.Fatal(err)
	}
	select {
	case completion := <-manager.Completed():
		if !completion.Cancelled {
			t.Fatalf("completion = %+v", completion)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for picker cancellation")
	}
	if snapshots := manager.Snapshot(); len(snapshots) != 0 {
		t.Fatalf("cancelled download snapshots = %+v", snapshots)
	}
	payload, err := os.ReadFile(filepath.Join(configDir, "native-downloads.json"))
	if err == nil && bytes.Contains(payload, []byte("job-1")) {
		t.Fatalf("cancelled picker left a journal entry: %s", payload)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
}

func TestArtifactDownloadManagerPublishesStartedBeforeCompletion(t *testing.T) {
	content := []byte("small artifact")
	manager, err := newArtifactDownloadManager(t.Context(), filepath.Join(t.TempDir(), "native-ui.json"), &pickerSequence{writers: []io.WriteCloser{&artifactWriter{}}})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	manager.AttachClient(&resumableDownloadClientStub{content: content, chunkSize: len(content)}, "server-1")
	if _, _, err := manager.Start("Artifact", map[string]string{"jobExecutionId": "job-1", "kind": "file", "path": "artifact.bin"}); err != nil {
		t.Fatal(err)
	}
	first := waitForDownloadEvent(t, manager.Events())
	second := waitForDownloadEvent(t, manager.Events())
	if first.Started == nil || first.Completion != nil || second.Started != nil || second.Completion == nil || second.Completion.Err != nil {
		t.Fatalf("download events = first %+v, second %+v", first, second)
	}
}

func TestArtifactDownloadManagerCoalescesProgressInvalidations(t *testing.T) {
	manager, err := newArtifactDownloadManager(t.Context(), filepath.Join(t.TempDir(), "native-ui.json"), &pickerSequence{})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	manager.mu.Lock()
	manager.records["download"] = &nativeDownloadRecord{ID: "download", State: downloadDownloading, TotalSize: 100}
	manager.mu.Unlock()
	for offset := int64(1); offset <= 50; offset++ {
		manager.updateProgress("download", offset, "token")
	}
	select {
	case <-manager.Changed():
	default:
		t.Fatal("progress produced no invalidation")
	}
	select {
	case <-manager.Changed():
		t.Fatal("progress burst was not coalesced")
	default:
	}
}

func TestArtifactDownloadManagerLeavesChangedDesktopDestinationUntouched(t *testing.T) {
	configDir := t.TempDir()
	destinationPath := filepath.Join(t.TempDir(), "artifact.bin")
	destination, err := os.Create(destinationPath)
	if err != nil {
		t.Fatal(err)
	}
	content := bytes.Repeat([]byte("desktop-reconcile"), 80)
	first := &resumableDownloadClientStub{content: content, chunkSize: 257, failAfterFirst: true}
	manager, err := newArtifactDownloadManager(t.Context(), filepath.Join(configDir, "native-ui.json"), &pickerSequence{writers: []io.WriteCloser{destination}})
	if err != nil {
		t.Fatal(err)
	}
	manager.AttachClient(first, "server-1")
	if _, _, err := manager.Start("Artifact", map[string]string{"jobExecutionId": "job-1", "kind": "file", "path": "artifact.bin"}); err != nil {
		t.Fatal(err)
	}
	waitForDownloadState(t, manager, downloadPaused)
	manager.Close()

	replacement := []byte("this file now belongs to the user")
	if err := os.WriteFile(destinationPath, replacement, 0o600); err != nil {
		t.Fatal(err)
	}
	restored, err := newArtifactDownloadManager(t.Context(), filepath.Join(configDir, "native-ui.json"), &pickerSequence{})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	restored.AttachClient(&resumableDownloadClientStub{content: content, chunkSize: 199}, "server-1")
	waitForDownloadState(t, restored, downloadReadyToSave)
	got, err := os.ReadFile(destinationPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, replacement) {
		t.Fatalf("changed destination was overwritten: %q", got)
	}
}

func TestArtifactDownloadManagerMarksReconstructedContentChange(t *testing.T) {
	configDir := t.TempDir()
	content := bytes.Repeat([]byte("original"), 200)
	manager, err := newArtifactDownloadManager(t.Context(), filepath.Join(configDir, "native-ui.json"), &pickerSequence{writers: []io.WriteCloser{&artifactWriter{}}})
	if err != nil {
		t.Fatal(err)
	}
	manager.AttachClient(&resumableDownloadClientStub{content: content, chunkSize: 311, failAfterFirst: true}, "server-1")
	if _, _, err := manager.Start("Artifact", map[string]string{"jobExecutionId": "job-1", "kind": "file", "path": "artifact.bin"}); err != nil {
		t.Fatal(err)
	}
	waitForDownloadState(t, manager, downloadPaused)
	manager.Close()

	changed := append([]byte(nil), content...)
	changed[len(changed)-1] ^= 0xff
	restored, err := newArtifactDownloadManager(t.Context(), filepath.Join(configDir, "native-ui.json"), &pickerSequence{})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	restored.AttachClient(&resumableDownloadClientStub{content: changed, chunkSize: 173}, "server-1")
	snapshot := waitForDownloadState(t, restored, downloadSourceChanged)
	if snapshot.Error == "" {
		t.Fatalf("source-changed snapshot = %+v", snapshot)
	}
}

func TestArtifactDownloadManagerDoesNotResumeAgainstDifferentServer(t *testing.T) {
	configDir := t.TempDir()
	content := bytes.Repeat([]byte("server-bound"), 100)
	manager, err := newArtifactDownloadManager(t.Context(), filepath.Join(configDir, "native-ui.json"), &pickerSequence{writers: []io.WriteCloser{&artifactWriter{}}})
	if err != nil {
		t.Fatal(err)
	}
	manager.AttachClient(&resumableDownloadClientStub{content: content, chunkSize: 233, failAfterFirst: true}, "server-1")
	id, _, err := manager.Start("Artifact", map[string]string{"jobExecutionId": "job-1", "kind": "file", "path": "artifact.bin"})
	if err != nil {
		t.Fatal(err)
	}
	waitForDownloadState(t, manager, downloadPaused)
	manager.Close()

	restored, err := newArtifactDownloadManager(t.Context(), filepath.Join(configDir, "native-ui.json"), &pickerSequence{})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	other := &resumableDownloadClientStub{content: content, chunkSize: 100}
	restored.AttachClient(other, "server-2")
	if err := restored.Resume(id); err == nil {
		t.Fatal("resume unexpectedly accepted a different server")
	}
	other.mu.Lock()
	calls := other.calls
	other.mu.Unlock()
	if calls != 0 || restored.Snapshot()[0].State != string(downloadPaused) {
		t.Fatalf("different-server resume = calls %d, snapshots %+v", calls, restored.Snapshot())
	}
}

func TestArtifactDownloadManagerFocusesDuplicateAndRunsDistinctDownloadsConcurrently(t *testing.T) {
	content := bytes.Repeat([]byte("parallel"), 100)
	manager, err := newArtifactDownloadManager(t.Context(), filepath.Join(t.TempDir(), "native-ui.json"), &pickerSequence{writers: []io.WriteCloser{&artifactWriter{}, &artifactWriter{}}})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	manager.AttachClient(&resumableDownloadClientStub{content: content, chunkSize: 157}, "server-1")
	firstID, duplicate, err := manager.Start("Artifact", map[string]string{"jobExecutionId": "job-1", "kind": "file", "path": "one.bin"})
	if err != nil || duplicate {
		t.Fatalf("first start = %q, duplicate %t, err %v", firstID, duplicate, err)
	}
	duplicateID, duplicate, err := manager.Start("Artifact", map[string]string{"jobExecutionId": "job-1", "kind": "file", "path": "one.bin"})
	if err != nil || !duplicate || duplicateID != firstID {
		t.Fatalf("duplicate start = %q, duplicate %t, err %v", duplicateID, duplicate, err)
	}
	secondID, duplicate, err := manager.Start("Artifact", map[string]string{"jobExecutionId": "job-1", "kind": "file", "path": "two.bin"})
	if err != nil || duplicate || secondID == firstID {
		t.Fatalf("second start = %q, duplicate %t, err %v", secondID, duplicate, err)
	}
	waitForDownloadCountInState(t, manager, 2, downloadCompleted)
}

func TestArtifactDownloadManagerCancelDeletesStageAndJournalEntry(t *testing.T) {
	configDir := t.TempDir()
	manager, err := newArtifactDownloadManager(t.Context(), filepath.Join(configDir, "native-ui.json"), &pickerSequence{writers: []io.WriteCloser{&failDownloadWriter{}}})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	content := bytes.Repeat([]byte("cancel"), 100)
	manager.AttachClient(&resumableDownloadClientStub{content: content, chunkSize: 127}, "server-1")
	id, _, err := manager.Start("Artifact", map[string]string{"jobExecutionId": "job-1", "kind": "file", "path": "cancel.bin"})
	if err != nil {
		t.Fatal(err)
	}
	waitForDownloadState(t, manager, downloadReadyToSave)
	record := manager.recordCopy(id)
	if record == nil || record.StagePath == "" {
		t.Fatalf("staged record = %+v", record)
	}
	if err := manager.Cancel(id); err != nil {
		t.Fatal(err)
	}
	if len(manager.Snapshot()) != 0 {
		t.Fatalf("cancelled snapshots = %+v", manager.Snapshot())
	}
	if _, err := os.Stat(record.StagePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cancelled stage still exists: %v", err)
	}
	payload, err := os.ReadFile(filepath.Join(configDir, "native-downloads.json"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(payload, []byte(id)) {
		t.Fatalf("cancelled journal still contains %q: %s", id, payload)
	}
}

func waitForDownloadState(t *testing.T, manager *artifactDownloadManager, state nativeDownloadState) nativeDownloadSnapshot {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		for _, snapshot := range manager.Snapshot() {
			if snapshot.State == string(state) {
				return snapshot
			}
		}
		select {
		case <-manager.Changed():
		case <-deadline.C:
			t.Fatalf("timed out waiting for download state %s; snapshots = %+v", state, manager.Snapshot())
		}
	}
}

func waitForDownloadCountInState(t *testing.T, manager *artifactDownloadManager, count int, state nativeDownloadState) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		matched := 0
		for _, snapshot := range manager.Snapshot() {
			if snapshot.State == string(state) {
				matched++
			}
		}
		if matched == count {
			return
		}
		select {
		case <-manager.Changed():
		case <-deadline.C:
			t.Fatalf("timed out waiting for %d downloads in state %s; snapshots = %+v", count, state, manager.Snapshot())
		}
	}
}

func waitForDownloadEvent(t *testing.T, events <-chan nativeDownloadEvent) nativeDownloadEvent {
	t.Helper()
	select {
	case event := <-events:
		return event
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for download event")
		return nativeDownloadEvent{}
	}
}

func testDownloadContentID(content []byte) string {
	hash := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(hash[:])
}
