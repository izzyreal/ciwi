//go:build darwin || linux || windows

package gio

import (
	"context"
	"errors"
	"time"
)

const (
	passiveRefreshDebounce = 250 * time.Millisecond
	passiveRetryInitial    = time.Second
	passiveRetryMaximum    = 8 * time.Second
)

type screenRefreshOrigin uint8

const (
	screenRefreshPassive screenRefreshOrigin = iota
	screenRefreshRetry
	screenRefreshOperation
	screenRefreshExplicit
	screenRefreshNavigation
)

func (o screenRefreshOrigin) passive() bool {
	return o == screenRefreshPassive || o == screenRefreshRetry
}

type screenRefreshRequest struct {
	origin              screenRefreshOrigin
	recoverMissingRoute bool
}

func mergeScreenRefreshRequest(left, right screenRefreshRequest) screenRefreshRequest {
	result := left
	if right.origin > result.origin {
		result.origin = right.origin
	}
	result.recoverMissingRoute = left.recoverMissingRoute || right.recoverMissingRoute
	return result
}

// screenRefreshCoordinator keeps background invalidations from repeatedly
// cancelling a screen request that is already in flight. Navigation remains a
// superseding operation and resets any queued refresh for the old screen.
type screenRefreshCoordinator struct {
	active         bool
	activeRequest  screenRefreshRequest
	pending        bool
	pendingRequest screenRefreshRequest
	retryDelay     time.Duration
}

func (c *screenRefreshCoordinator) supersede(request screenRefreshRequest) {
	c.active = true
	c.activeRequest = request
	c.pending = false
	c.pendingRequest = screenRefreshRequest{}
	c.retryDelay = 0
}

func (c *screenRefreshCoordinator) request(request screenRefreshRequest) bool {
	if c.active {
		if c.pending {
			c.pendingRequest = mergeScreenRefreshRequest(c.pendingRequest, request)
		} else {
			c.pending = true
			c.pendingRequest = request
		}
		return false
	}
	c.active = true
	c.activeRequest = request
	return true
}

func (c *screenRefreshCoordinator) complete() (screenRefreshRequest, bool) {
	if !c.active {
		return screenRefreshRequest{}, false
	}
	c.active = false
	c.activeRequest = screenRefreshRequest{}
	if !c.pending {
		return screenRefreshRequest{}, false
	}
	request := c.pendingRequest
	c.pending = false
	c.pendingRequest = screenRefreshRequest{}
	return request, true
}

func (c *screenRefreshCoordinator) nextRetry() time.Duration {
	if c.retryDelay <= 0 {
		c.retryDelay = passiveRetryInitial
		return c.retryDelay
	}
	c.retryDelay *= 2
	if c.retryDelay > passiveRetryMaximum {
		c.retryDelay = passiveRetryMaximum
	}
	return c.retryDelay
}

func (c *screenRefreshCoordinator) resetRetry() { c.retryDelay = 0 }

func (c *screenRefreshCoordinator) cancel() {
	c.active = false
	c.activeRequest = screenRefreshRequest{}
	c.pending = false
	c.pendingRequest = screenRefreshRequest{}
	c.retryDelay = 0
}

func quietPassiveRefreshFailure(request screenRefreshRequest, err error, cached bool) bool {
	return request.origin.passive() && cached && errors.Is(err, context.DeadlineExceeded)
}
