package agent

import (
	"fmt"
	"os"
	"runtime"
	"time"
)

var (
	removeAllWithRetryGOOSFn  = func() string { return runtime.GOOS }
	removeAllWithRetryFn      = os.RemoveAll
	removeAllWithRetrySleepFn = time.Sleep
)

func removeAllWithRetry(path string) error {
	const (
		windowsAttempts = 8
		windowsDelay    = 500 * time.Millisecond
	)

	if removeAllWithRetryGOOSFn() != "windows" {
		return removeAllWithRetryFn(path)
	}

	var lastErr error
	for attempt := 1; attempt <= windowsAttempts; attempt++ {
		if err := removeAllWithRetryFn(path); err != nil {
			lastErr = err
			if attempt == windowsAttempts {
				break
			}
			removeAllWithRetrySleepFn(windowsDelay)
			continue
		}
		return nil
	}
	if lastErr == nil {
		return nil
	}
	return fmt.Errorf("remove %q after retries: %w", path, lastErr)
}
