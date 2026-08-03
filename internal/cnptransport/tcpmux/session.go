package tcpmux

import (
	"context"
	"net"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/izzyreal/ciwi/pkg/cnp"
)

func Client(connection net.Conn, logger yamux.Logger) (cnp.Session, error) {
	multiplexer, err := yamux.Client(connection, config(logger))
	if err != nil {
		return nil, err
	}
	return &session{multiplexer: multiplexer}, nil
}

func Server(connection net.Conn, logger yamux.Logger) (cnp.Session, error) {
	multiplexer, err := yamux.Server(connection, config(logger))
	if err != nil {
		return nil, err
	}
	return &session{multiplexer: multiplexer}, nil
}

func config(logger yamux.Logger) *yamux.Config {
	config := yamux.DefaultConfig()
	config.AcceptBacklog = 128
	config.EnableKeepAlive = true
	config.KeepAliveInterval = 15 * time.Second
	config.ConnectionWriteTimeout = 10 * time.Second
	config.StreamOpenTimeout = 10 * time.Second
	config.StreamCloseTimeout = 30 * time.Second
	config.LogOutput = nil
	config.Logger = logger
	return config
}

type session struct {
	multiplexer *yamux.Session
}

func (s *session) OpenStream(ctx context.Context) (cnp.Stream, error) {
	type result struct {
		stream *yamux.Stream
		err    error
	}
	opened := make(chan result, 1)
	go func() {
		stream, err := s.multiplexer.OpenStream()
		opened <- result{stream: stream, err: err}
	}()
	select {
	case <-ctx.Done():
		go func() {
			result := <-opened
			if result.stream != nil {
				_ = result.stream.Close()
			}
		}()
		return nil, ctx.Err()
	case result := <-opened:
		if result.err != nil {
			return nil, result.err
		}
		return stream{Stream: result.stream}, nil
	}
}

func (s *session) AcceptStream(ctx context.Context) (cnp.Stream, error) {
	accepted, err := s.multiplexer.AcceptStreamWithContext(ctx)
	if err != nil {
		return nil, err
	}
	return stream{Stream: accepted}, nil
}

func (s *session) CloseWithError(error) error { return s.multiplexer.Close() }

type stream struct {
	*yamux.Stream
}

func (s stream) CancelRead()  { _ = s.Stream.SetReadDeadline(time.Now()) }
func (s stream) CancelWrite() { _ = s.Stream.Close() }

var _ cnp.Session = (*session)(nil)
var _ cnp.Stream = stream{}
