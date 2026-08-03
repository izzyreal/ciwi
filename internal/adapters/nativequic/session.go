package nativequic

import (
	"context"

	"github.com/izzyreal/ciwi/pkg/cnp"
	"github.com/quic-go/quic-go"
)

const (
	connectionProtocolError quic.ApplicationErrorCode = 0x100
	streamProtocolError     quic.StreamErrorCode      = 0x101
)

type session struct {
	connection *quic.Conn
}

func (s *session) OpenStream(ctx context.Context) (cnp.Stream, error) {
	stream, err := s.connection.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}
	return quicStream{Stream: stream}, nil
}

func (s *session) AcceptStream(ctx context.Context) (cnp.Stream, error) {
	stream, err := s.connection.AcceptStream(ctx)
	if err != nil {
		return nil, err
	}
	return quicStream{Stream: stream}, nil
}

func (s *session) CloseWithError(err error) error {
	message := "session closed"
	code := quic.ApplicationErrorCode(0)
	if err != nil {
		message = err.Error()
		code = connectionProtocolError
	}
	return s.connection.CloseWithError(code, message)
}

type quicStream struct {
	*quic.Stream
}

func (s quicStream) CancelRead()  { s.Stream.CancelRead(streamProtocolError) }
func (s quicStream) CancelWrite() { s.Stream.CancelWrite(streamProtocolError) }

var _ cnp.Session = (*session)(nil)
var _ cnp.Stream = quicStream{}
