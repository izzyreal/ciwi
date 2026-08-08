package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/izzyreal/ciwi/internal/application"
	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/server/jobexecution"
)

const (
	artifactDownloadChunkSize = 512 * 1024
	artifactDownloadTTL       = 15 * time.Minute
)

type artifactDownloadStore interface {
	ListJobExecutionArtifacts(string) ([]protocol.JobExecutionArtifact, error)
	GetJobExecution(string) (protocol.JobExecution, error)
	ListJobExecutionEvents(string) ([]protocol.JobExecutionEvent, error)
}

type artifactDownloadSession struct {
	path        string
	fileName    string
	contentType string
	temporary   bool
	totalSize   int64
	lastUsed    time.Time
}

type artifactDownloadService struct {
	store        artifactDownloadStore
	artifactsDir string
	mu           sync.Mutex
	sessions     map[string]artifactDownloadSession
	now          func() time.Time
}

func newArtifactDownloadService(store artifactDownloadStore, artifactsDir string) *artifactDownloadService {
	return &artifactDownloadService{
		store: store, artifactsDir: artifactsDir, sessions: map[string]artifactDownloadSession{}, now: time.Now,
	}
}

func (s *artifactDownloadService) DownloadArtifact(_ context.Context, request application.ArtifactDownloadRequest) (application.ArtifactDownloadChunk, error) {
	if s == nil || s.store == nil {
		return application.ArtifactDownloadChunk{}, application.NewError(application.ErrorUnavailable, "artifact downloads unavailable", nil)
	}
	if request.Offset < 0 {
		return application.ArtifactDownloadChunk{}, application.NewError(application.ErrorInvalidArgument, "artifact offset must be non-negative", nil)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked()

	token := strings.TrimSpace(request.Token)
	session, ok := s.sessions[token]
	if request.Cancel {
		if token == "" {
			return application.ArtifactDownloadChunk{}, application.NewError(application.ErrorInvalidArgument, "artifact download token is required for cancellation", nil)
		}
		if !ok {
			return application.ArtifactDownloadChunk{}, application.NewError(application.ErrorNotFound, "artifact download expired", nil)
		}
		s.finishLocked(token, session)
		return application.ArtifactDownloadChunk{Token: token, Complete: true}, nil
	}
	if token == "" {
		if request.Offset != 0 {
			return application.ArtifactDownloadChunk{}, application.NewError(application.ErrorInvalidArgument, "new artifact downloads must start at offset zero", nil)
		}
		var err error
		token, session, err = s.startLocked(request)
		if err != nil {
			return application.ArtifactDownloadChunk{}, err
		}
	} else if !ok {
		return application.ArtifactDownloadChunk{}, application.NewError(application.ErrorNotFound, "artifact download expired", nil)
	}
	if request.Offset > session.totalSize {
		return application.ArtifactDownloadChunk{}, application.NewError(application.ErrorInvalidArgument, "artifact offset exceeds file size", nil)
	}

	file, err := os.Open(session.path)
	if err != nil {
		s.finishLocked(token, session)
		return application.ArtifactDownloadChunk{}, application.WrapInternal("open artifact download", err)
	}
	defer file.Close()
	if _, err := file.Seek(request.Offset, io.SeekStart); err != nil {
		return application.ArtifactDownloadChunk{}, application.WrapInternal("seek artifact download", err)
	}
	remaining := session.totalSize - request.Offset
	readSize := int64(artifactDownloadChunkSize)
	if remaining < readSize {
		readSize = remaining
	}
	data := make([]byte, int(readSize))
	if _, err := io.ReadFull(file, data); err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return application.ArtifactDownloadChunk{}, application.WrapInternal("read artifact download", err)
	}
	nextOffset := request.Offset + int64(len(data))
	complete := nextOffset >= session.totalSize
	if complete {
		s.finishLocked(token, session)
	} else {
		session.lastUsed = s.now()
		s.sessions[token] = session
	}
	return application.ArtifactDownloadChunk{
		Token: token, FileName: session.fileName, ContentType: session.contentType,
		Data: data, NextOffset: nextOffset, TotalSize: session.totalSize, Complete: complete,
	}, nil
}

func (s *artifactDownloadService) startLocked(request application.ArtifactDownloadRequest) (string, artifactDownloadSession, error) {
	jobID := strings.TrimSpace(request.JobExecutionID)
	if jobID == "" {
		return "", artifactDownloadSession{}, application.NewError(application.ErrorInvalidArgument, "job execution id is required", nil)
	}
	kind := strings.ToLower(strings.TrimSpace(request.Kind))
	if kind == "log-clean" || kind == "log-raw" {
		return s.startJobLogLocked(jobID, strings.TrimPrefix(kind, "log-"))
	}
	artifacts, err := s.store.ListJobExecutionArtifacts(jobID)
	if err != nil {
		return "", artifactDownloadSession{}, application.WrapInternal("list job artifacts", err)
	}
	artifacts = jobexecution.AppendSyntheticTestReportArtifact(s.artifactsDir, jobID, artifacts)
	artifacts = jobexecution.AppendSyntheticCoverageReportArtifact(s.artifactsDir, jobID, artifacts)

	var session artifactDownloadSession
	switch kind {
	case "file":
		rel, valid := jobexecution.NormalizeRelativeArtifactPath(request.Path)
		if !valid {
			return "", artifactDownloadSession{}, application.NewError(application.ErrorInvalidArgument, "invalid artifact path", nil)
		}
		found := false
		for _, artifact := range artifacts {
			candidate, valid := jobexecution.NormalizeRelativeArtifactPath(artifact.Path)
			if valid && candidate == rel {
				found = true
				break
			}
		}
		if !found {
			return "", artifactDownloadSession{}, application.NewError(application.ErrorNotFound, "artifact not found", nil)
		}
		session.path = filepath.Join(s.artifactsDir, jobID, filepath.FromSlash(rel))
		session.fileName = filepath.Base(rel)
		session.contentType = mime.TypeByExtension(filepath.Ext(rel))
		if session.contentType == "" {
			session.contentType = "application/octet-stream"
		}
	case "prefix", "all":
		prefix := ""
		filtered := artifacts
		if strings.EqualFold(strings.TrimSpace(request.Kind), "prefix") {
			var valid bool
			prefix, valid = jobexecution.NormalizeRelativeArtifactPath(request.Path)
			if !valid {
				return "", artifactDownloadSession{}, application.NewError(application.ErrorInvalidArgument, "invalid artifact prefix", nil)
			}
			filtered = make([]protocol.JobExecutionArtifact, 0, len(artifacts))
			for _, artifact := range artifacts {
				rel, valid := jobexecution.NormalizeRelativeArtifactPath(artifact.Path)
				if valid && (rel == prefix || strings.HasPrefix(rel, prefix+"/")) {
					filtered = append(filtered, artifact)
				}
			}
			if len(filtered) == 0 {
				return "", artifactDownloadSession{}, application.NewError(application.ErrorNotFound, "artifact directory not found", nil)
			}
		}
		fileName := jobexecution.BuildArtifactsZIPFileName(jobID, prefix)
		session.path, session.fileName, err = jobexecution.BuildArtifactsZIP(s.artifactsDir, jobID, filtered, fileName)
		if err != nil {
			return "", artifactDownloadSession{}, application.WrapInternal("build artifact archive", err)
		}
		session.contentType = "application/zip"
		session.temporary = true
	default:
		return "", artifactDownloadSession{}, application.NewError(application.ErrorInvalidArgument, "download kind must be file, prefix, all, log-clean, or log-raw", nil)
	}
	info, err := os.Stat(session.path)
	if err != nil || info.IsDir() {
		if session.temporary {
			_ = os.Remove(session.path)
		}
		if err == nil {
			err = fmt.Errorf("artifact path is a directory")
		}
		return "", artifactDownloadSession{}, application.NewError(application.ErrorNotFound, "artifact file unavailable", err)
	}
	session.totalSize = info.Size()
	session.lastUsed = s.now()
	token, err := randomDownloadToken()
	if err != nil {
		if session.temporary {
			_ = os.Remove(session.path)
		}
		return "", artifactDownloadSession{}, application.WrapInternal("create artifact download token", err)
	}
	s.sessions[token] = session
	s.scheduleCleanup(token, artifactDownloadTTL)
	return token, session, nil
}

func (s *artifactDownloadService) startJobLogLocked(jobID, format string) (string, artifactDownloadSession, error) {
	job, err := s.store.GetJobExecution(jobID)
	if err != nil {
		return "", artifactDownloadSession{}, application.NewError(application.ErrorNotFound, "job not found", err)
	}
	events, err := s.store.ListJobExecutionEvents(jobID)
	if err != nil {
		return "", artifactDownloadSession{}, application.WrapInternal("list job log events", err)
	}
	body, fileName, err := jobexecution.RenderJobLog(job, events, format)
	if err != nil {
		return "", artifactDownloadSession{}, application.NewError(application.ErrorInvalidArgument, err.Error(), err)
	}
	file, err := os.CreateTemp("", "ciwi-job-log-*.log")
	if err != nil {
		return "", artifactDownloadSession{}, application.WrapInternal("create job log download", err)
	}
	path := file.Name()
	if _, err = file.WriteString(body); err == nil {
		err = file.Close()
	} else {
		_ = file.Close()
	}
	if err != nil {
		_ = os.Remove(path)
		return "", artifactDownloadSession{}, application.WrapInternal("write job log download", err)
	}
	session := artifactDownloadSession{
		path: path, fileName: fileName, contentType: "text/plain; charset=utf-8", temporary: true,
		totalSize: int64(len(body)), lastUsed: s.now(),
	}
	token, err := randomDownloadToken()
	if err != nil {
		_ = os.Remove(path)
		return "", artifactDownloadSession{}, application.WrapInternal("create job log download token", err)
	}
	s.sessions[token] = session
	s.scheduleCleanup(token, artifactDownloadTTL)
	return token, session, nil
}

func (s *artifactDownloadService) scheduleCleanup(token string, after time.Duration) {
	time.AfterFunc(after, func() {
		s.mu.Lock()
		session, ok := s.sessions[token]
		if !ok {
			s.mu.Unlock()
			return
		}
		remaining := artifactDownloadTTL - s.now().Sub(session.lastUsed)
		if remaining <= 0 {
			s.finishLocked(token, session)
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()
		s.scheduleCleanup(token, remaining)
	})
}

func (s *artifactDownloadService) finishLocked(token string, session artifactDownloadSession) {
	delete(s.sessions, token)
	if session.temporary {
		_ = os.Remove(session.path)
	}
}

func (s *artifactDownloadService) pruneLocked() {
	cutoff := s.now().Add(-artifactDownloadTTL)
	for token, session := range s.sessions {
		if session.lastUsed.Before(cutoff) {
			s.finishLocked(token, session)
		}
	}
}

func randomDownloadToken() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes[:]), nil
}
