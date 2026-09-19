package agent

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	agentLogMaxBytes = 10 * 1024 * 1024
	agentLogBackups  = 3
)

func newAgentFileLogger(path string, stderr io.Writer, levelName string) (*slog.Logger, io.Closer, error) {
	w, err := openAgentLog(path, agentLogMaxBytes, agentLogBackups)
	if err != nil {
		return nil, nil, err
	}
	level := slog.LevelInfo
	switch strings.ToLower(strings.TrimSpace(levelName)) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	// Windows services may have an invalid stderr handle. Write the file first
	// and treat console output as best effort so it cannot prevent persistence.
	output := io.MultiWriter(w, bestEffortLogWriter{stderr})
	return slog.New(slog.NewTextHandler(output, &slog.HandlerOptions{Level: level})), w, nil
}

type bestEffortLogWriter struct{ io.Writer }

func (w bestEffortLogWriter) Write(p []byte) (int, error) {
	if w.Writer != nil {
		_, _ = w.Writer.Write(p)
	}
	return len(p), nil
}

type agentLogWriter struct {
	mu      sync.Mutex
	path    string
	file    *os.File
	size    int64
	maxSize int64
	backups int
}

func openAgentLog(path string, maxSize int64, backups int) (*agentLogWriter, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	w := &agentLogWriter{path: path, maxSize: maxSize, backups: backups}
	if err := w.open(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *agentLogWriter) open() error {
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	w.file, w.size = f, info.Size()
	return nil
}

func (w *agentLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return 0, os.ErrClosed
	}
	if w.size > 0 && w.size+int64(len(p)) > w.maxSize {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := w.file.Write(p)
	w.size += int64(n)
	return n, err
}

func (w *agentLogWriter) rotate() (err error) {
	// Close before renaming for Windows. Reopen even on a rotation failure,
	// allowing subsequent writes to retry rather than leaving a closed logger.
	err = w.file.Close()
	w.file = nil
	defer func() { err = errors.Join(err, w.open()) }()
	if err != nil {
		return err
	}
	oldest := fmt.Sprintf("%s.%d", w.path, w.backups)
	if err := os.Remove(oldest); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for i := w.backups - 1; i >= 0; i-- {
		source := w.path
		if i > 0 {
			source = fmt.Sprintf("%s.%d", w.path, i)
		}
		if err := os.Rename(source, fmt.Sprintf("%s.%d", w.path, i+1)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func (w *agentLogWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}
