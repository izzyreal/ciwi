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
	return tcpSession(ctx, connection)
}

func tcpSession(ctx context.Context, connection net.Conn) (cnp.Session, error) {
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

// DialTCPConn starts a CNP client over an already-connected stream. It is used
// by connection adapters such as SSH direct-tcpip without opening a local TCP
// listener or changing the CNP application protocol.
func DialTCPConn(ctx context.Context, connection net.Conn, clientName, clientVersion string) (*Client, error) {
	return DialTCPConnWithProjectIconCache(ctx, connection, clientName, clientVersion, nil)
}

func DialTCPConnWithProjectIconCache(ctx context.Context, connection net.Conn, clientName, clientVersion string, icons *ProjectIconCache) (*Client, error) {
	session, err := tcpSession(ctx, connection)
	if err != nil {
		return nil, fmt.Errorf("dial ciwi native endpoint: %w", err)
	}
	return newClient(ctx, session, clientName, clientVersion, icons)
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
