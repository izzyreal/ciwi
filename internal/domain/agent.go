package domain

import "time"

type Agent struct {
	ID               string
	Hostname         string
	OS               string
	Arch             string
	Version          string
	Authorized       bool
	Deactivated      bool
	JobInProgress    bool
	Capabilities     map[string]string
	LastSeenUTC      time.Time
	RecentLog        []string
	NeedsUpdate      bool
	UpdateTarget     string
	UpdateRequested  bool
	UpdateAttempts   int
	UpdateInProgress bool
	UpdateNextRetry  time.Time
	UpdateLastError  string
}
