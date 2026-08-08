//go:build darwin || linux || windows

package gio

// screenRefreshCoordinator keeps background invalidations from repeatedly
// cancelling a screen request that is already in flight. Navigation remains a
// superseding operation and resets any queued refresh for the old screen.
type screenRefreshCoordinator struct {
	active         bool
	pending        bool
	pendingRecover bool
}

func (c *screenRefreshCoordinator) supersede() {
	c.active = true
	c.pending = false
	c.pendingRecover = false
}

func (c *screenRefreshCoordinator) request(recoverMissingRoute bool) bool {
	if c.active {
		c.pending = true
		c.pendingRecover = c.pendingRecover || recoverMissingRoute
		return false
	}
	c.active = true
	return true
}

func (c *screenRefreshCoordinator) complete() (startTrailing, recoverMissingRoute bool) {
	if !c.active {
		return false, false
	}
	if c.pending {
		recoverMissingRoute = c.pendingRecover
		c.pending = false
		c.pendingRecover = false
		// The trailing request takes ownership of the active slot.
		return true, recoverMissingRoute
	}
	c.active = false
	return false, false
}

func (c *screenRefreshCoordinator) cancel() {
	c.active = false
	c.pending = false
	c.pendingRecover = false
}
