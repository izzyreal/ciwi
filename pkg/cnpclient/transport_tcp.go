package cnpclient

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/izzyreal/ciwi/internal/cnptransport/tcpmux"
	"github.com/izzyreal/ciwi/pkg/cnp"
)

func dialTCP(ctx context.Context, address string) (cnp.Session, error) {
	connection, err := (&net.Dialer{KeepAlive: 30 * time.Second}).DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}
	tlsConnection := tls.Client(connection, &tls.Config{ // #nosec G402 -- documented CNP v1 trust model.
		MinVersion: tls.VersionTLS13, NextProtos: []string{ALPN}, InsecureSkipVerify: true,
	})
	if err := tlsConnection.HandshakeContext(ctx); err != nil {
		_ = connection.Close()
		return nil, err
	}
	if tlsConnection.ConnectionState().NegotiatedProtocol != ALPN {
		_ = tlsConnection.Close()
		return nil, fmt.Errorf("native TCP endpoint did not negotiate %s", ALPN)
	}
	session, err := tcpmux.Client(tlsConnection, clientYamuxLogger{})
	if err != nil {
		_ = tlsConnection.Close()
		return nil, err
	}
	return session, nil
}

type clientYamuxLogger struct{}

func (clientYamuxLogger) Print(arguments ...any) {
	slog.Debug("native TCP client multiplexer", "message", fmt.Sprint(arguments...))
}

func (clientYamuxLogger) Printf(format string, arguments ...any) {
	slog.Debug("native TCP client multiplexer", "message", fmt.Sprintf(format, arguments...))
}

func (clientYamuxLogger) Println(arguments ...any) {
	slog.Debug("native TCP client multiplexer", "message", fmt.Sprintln(arguments...))
}
