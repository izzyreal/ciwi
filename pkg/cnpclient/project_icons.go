package cnpclient

import (
	"strings"
	"sync"
)

// ProjectIconCache is intentionally independent of a transport session. A
// native app can share one cache across reconnects without allowing project IDs
// from different server installations to collide.
type ProjectIconCache struct {
	mu    sync.Mutex
	icons map[projectIconCacheKey]projectIcon
}

type projectIconCacheKey struct {
	serverInstallationID string
	projectID            int64
}

type projectIcon struct {
	contentType  string
	data         []byte
	loadedCommit string
}

func NewProjectIconCache() *ProjectIconCache {
	return &ProjectIconCache{icons: make(map[projectIconCacheKey]projectIcon)}
}

func (c *ProjectIconCache) get(serverInstallationID string, projectID int64) (projectIcon, bool) {
	if c == nil || projectID <= 0 {
		return projectIcon{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	icon, ok := c.icons[projectIconCacheKey{serverInstallationID: strings.TrimSpace(serverInstallationID), projectID: projectID}]
	if !ok {
		return projectIcon{}, false
	}
	icon.data = append([]byte(nil), icon.data...)
	return icon, true
}

func (c *ProjectIconCache) put(serverInstallationID string, projectID int64, icon projectIcon) {
	if c == nil || projectID <= 0 {
		return
	}
	icon.data = append([]byte(nil), icon.data...)
	c.mu.Lock()
	c.icons[projectIconCacheKey{serverInstallationID: strings.TrimSpace(serverInstallationID), projectID: projectID}] = icon
	c.mu.Unlock()
}
