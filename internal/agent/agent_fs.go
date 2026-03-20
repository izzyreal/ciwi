package agent

import (
	"fmt"
	"os"
	"runtime"
	"time"
)

func removeAllWithRetry(path string) error {
	const (
		windowsAttempts = 8
		windowsDelay    = 500 * time.Millisecond
	)

	if runtime.GOOS != "windows" {
		return os.RemoveAll(path)
	}

	var lastErr error
	for attempt := 1; attempt <= windowsAttempts; attempt++ {
		if err := os.RemoveAll(path); err != nil {
			lastErr = err
			if attempt == windowsAttempts {
				break
			}
			time.Sleep(windowsDelay)
			continue
		}
		return nil
	}
	if lastErr == nil {
		return nil
	}
	return fmt.Errorf("remove %q after retries: %w", path, lastErr)
}
