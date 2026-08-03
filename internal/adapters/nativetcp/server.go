package nativetcp

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/izzyreal/ciwi/internal/adapters/nativecnp"
	"github.com/izzyreal/ciwi/internal/cnptransport/tcpmux"
	"github.com/izzyreal/ciwi/pkg/cnp"
)

const ALPN = nativecnp.ALPN

type Server struct {
	listener  net.Listener
	handler   *nativecnp.Handler
	tlsConfig *tls.Config
	sessions  sync.Map
}

func ListenWithHandler(address string, handler *nativecnp.Handler, tlsConfig *tls.Config) (*Server, error) {
	if handler == nil {
		return nil, fmt.Errorf("native CNP handler is required")
	}
	if tlsConfig == nil {
		return nil, fmt.Errorf("native TLS configuration is required")
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("listen for native TCP connections: %w", err)
	}
	return &Server{listener: listener, handler: handler, tlsConfig: tlsConfig.Clone()}, nil
}

func (s *Server) Addr() string {
	if s == nil || s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

func (s *Server) Close() error {
	if s == nil || s.listener == nil {
		return nil
	}
	err := s.listener.Close()
	s.sessions.Range(func(key, _ any) bool {
		_ = key.(cnp.Session).CloseWithError(nil)
		return true
	})
	return err
}

func (s *Server) Serve(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		_ = s.Close()
	}()
	for {
		connection, err := s.listener.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return fmt.Errorf("accept native TCP connection: %w", err)
		}
		go s.handleConnection(ctx, connection)
	}
}

func (s *Server) handleConnection(ctx context.Context, connection net.Conn) {
	if tcpConnection, ok := connection.(*net.TCPConn); ok {
		_ = tcpConnection.SetKeepAlive(true)
		_ = tcpConnection.SetKeepAlivePeriod(30 * time.Second)
	}
	tlsConnection := tls.Server(connection, s.tlsConfig.Clone())
	handshakeCtx, cancelHandshake := context.WithTimeout(ctx, 8*time.Second)
	err := tlsConnection.HandshakeContext(handshakeCtx)
	cancelHandshake()
	if err != nil {
		_ = connection.Close()
		slog.Debug("native TCP TLS handshake failed", "remote", connection.RemoteAddr(), "error", err)
		return
	}
	if tlsConnection.ConnectionState().NegotiatedProtocol != ALPN {
		_ = tlsConnection.Close()
		slog.Debug("native TCP connection used unexpected ALPN", "remote", connection.RemoteAddr())
		return
	}
	session, err := tcpmux.Server(tlsConnection, yamuxLogger{})
	if err != nil {
		_ = tlsConnection.Close()
		return
	}
	s.sessions.Store(session, struct{}{})
	defer s.sessions.Delete(session)
	defer session.CloseWithError(nil)
	s.handler.ServeSession(ctx, session)
}

type yamuxLogger struct{}

func (yamuxLogger) Print(arguments ...any) {
	slog.Debug("native TCP multiplexer", "message", fmt.Sprint(arguments...))
}

func (yamuxLogger) Printf(format string, arguments ...any) {
	slog.Debug("native TCP multiplexer", "message", fmt.Sprintf(format, arguments...))
}

func (yamuxLogger) Println(arguments ...any) {
	slog.Debug("native TCP multiplexer", "message", fmt.Sprintln(arguments...))
}
