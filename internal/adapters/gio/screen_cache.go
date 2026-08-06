//go:build darwin || ios || linux || windows

package gio

import "strings"

type nativeScreenCacheKey struct {
	screen         string
	projectID      int64
	jobID          string
	agentDetailsID string
}

type nativeScreenCache struct {
	serverInstallationID string
	entries              map[nativeScreenCacheKey]map[string]any
}

func newNativeScreenCache() *nativeScreenCache {
	return &nativeScreenCache{entries: map[nativeScreenCacheKey]map[string]any{}}
}

// SetServerInstallationID partitions cached views by stable server identity.
// It reports whether an established identity changed and stale views were
// discarded.
func (c *nativeScreenCache) SetServerInstallationID(serverID string) bool {
	serverID = strings.TrimSpace(serverID)
	if c == nil || serverID == "" {
		return false
	}
	changed := c.serverInstallationID != "" && c.serverInstallationID != serverID
	if changed {
		c.entries = map[nativeScreenCacheKey]map[string]any{}
	}
	c.serverInstallationID = serverID
	return changed
}

func (c *nativeScreenCache) Get(navigation navigationState) (map[string]any, bool) {
	key, ok := nativeScreenCacheKeyFor(navigation)
	if c == nil || !ok || c.serverInstallationID == "" {
		return nil, false
	}
	data, exists := c.entries[key]
	if !exists {
		return nil, false
	}
	return cloneBindingMap(data), true
}

func (c *nativeScreenCache) Has(navigation navigationState) bool {
	key, ok := nativeScreenCacheKeyFor(navigation)
	if c == nil || !ok || c.serverInstallationID == "" {
		return false
	}
	_, exists := c.entries[key]
	return exists
}

func (c *nativeScreenCache) Put(navigation navigationState, data map[string]any) {
	key, ok := nativeScreenCacheKeyFor(navigation)
	if c == nil || !ok || c.serverInstallationID == "" || data == nil {
		return
	}
	c.entries[key] = cloneBindingMap(data)
}

func (c *nativeScreenCache) SetRootBinding(navigation navigationState, root, field string, value any) {
	key, ok := nativeScreenCacheKeyFor(navigation)
	if c == nil || !ok {
		return
	}
	data := c.entries[key]
	rootData, exists := data[root].(map[string]any)
	if !exists {
		return
	}
	rootData[field] = cloneBindingValue(value)
}

func nativeScreenCacheKeyFor(navigation navigationState) (nativeScreenCacheKey, bool) {
	key := nativeScreenCacheKey{screen: navigation.screen}
	switch navigation.screen {
	case "front-page", "settings", "agents":
		return key, true
	case "project-details":
		key.projectID = navigation.projectID
		return key, navigation.projectID > 0
	case "job-details":
		key.jobID = strings.TrimSpace(navigation.jobID)
		return key, key.jobID != ""
	case "agent-details":
		key.agentDetailsID = strings.TrimSpace(navigation.agentDetailsID)
		return key, key.agentDetailsID != ""
	default:
		return nativeScreenCacheKey{}, false
	}
}

func cloneBindingMap(source map[string]any) map[string]any {
	if source == nil {
		return nil
	}
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = cloneBindingValue(value)
	}
	return result
}

func cloneBindingValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneBindingMap(typed)
	case []any:
		result := make([]any, len(typed))
		for index := range typed {
			result[index] = cloneBindingValue(typed[index])
		}
		return result
	default:
		return value
	}
}
