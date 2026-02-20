package server

import (
	"context"
	"testing"
)

func TestMaybeRunPostUpdateProjectReloadGuards(t *testing.T) {
	// nil state should be a no-op
	var nilState *stateStore
	nilState.maybeRunPostUpdateProjectReload(context.Background())

	// state without db should be a no-op
	s := &stateStore{}
	s.maybeRunPostUpdateProjectReload(context.Background())
}
