//go:build !darwin || !cgo

package cnpclient

import (
	"context"
	"time"
)

func discoverPlatformService(context.Context, string, time.Duration) ([]*discoveryEntry, bool, error) {
	return nil, false, nil
}
