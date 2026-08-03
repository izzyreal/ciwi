package cnpclient

import (
	"context"
	"crypto/tls"
	"time"

	"github.com/izzyreal/ciwi/pkg/cnp"
	"github.com/quic-go/quic-go"
)

const (
	connectionProtocolError quic.ApplicationErrorCode = 0x100
	streamProtocolError     quic.StreamErrorCode      = 0x101
)

func dialQUIC(ctx context.Context, address string) (cnp.Session, error) {
	// CNP v1 intentionally preserves ciwi's trusted-network model. TLS encrypts
	// the connection, but v1 does not authenticate endpoint identity.
	tlsConfig := &tls.Config{ // #nosec G402 -- documented CNP v1 trust model.
		MinVersion:         tls.VersionTLS13,
		NextProtos:         []string{ALPN},
		InsecureSkipVerify: true,
	}
	connection, err := quic.DialAddr(ctx, address, tlsConfig, &quic.Config{
		Allow0RTT:       false,
		EnableDatagrams: false,
		KeepAlivePeriod: 3 * time.Second,
		MaxIdleTimeout:  10 * time.Second,
	})
	if err != nil {
		return nil, err
	}
	return &quicSession{connection: connection}, nil
}

type quicSession struct {
	connection *quic.Conn
}

func (s *quicSession) OpenStream(ctx context.Context) (cnp.Stream, error) {
	stream, err := s.connection.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}
	return clientQUICStream{Stream: stream}, nil
}

func (s *quicSession) AcceptStream(ctx context.Context) (cnp.Stream, error) {
	stream, err := s.connection.AcceptStream(ctx)
	if err != nil {
		return nil, err
	}
	return clientQUICStream{Stream: stream}, nil
}

func (s *quicSession) CloseWithError(err error) error {
	message := "client closed"
	code := quic.ApplicationErrorCode(0)
	if err != nil {
		message = err.Error()
		code = connectionProtocolError
	}
	return s.connection.CloseWithError(code, message)
}

type clientQUICStream struct {
	*quic.Stream
}

func (s clientQUICStream) CancelRead()  { s.Stream.CancelRead(streamProtocolError) }
func (s clientQUICStream) CancelWrite() { s.Stream.CancelWrite(streamProtocolError) }

var _ cnp.Session = (*quicSession)(nil)
var _ cnp.Stream = clientQUICStream{}
