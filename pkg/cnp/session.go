package cnp

import (
	"context"
	"io"
)

const ALPN = "ciwi-native/1"

// Stream is a bidirectional CNP stream. Close half-closes the local write side
// so the peer can observe completion while responses remain readable.
type Stream interface {
	io.Reader
	io.Writer
	io.Closer
	CancelRead()
	CancelWrite()
}

// Session provides independent logical streams over a native transport.
// QUIC supplies these streams directly; ordered transports may multiplex them.
type Session interface {
	OpenStream(context.Context) (Stream, error)
	AcceptStream(context.Context) (Stream, error)
	CloseWithError(error) error
}
