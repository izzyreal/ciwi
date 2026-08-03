package nativequic

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"time"

	"github.com/izzyreal/ciwi/internal/adapters/nativecnp"
	"github.com/quic-go/quic-go"
)

const ALPN = nativecnp.ALPN

type Services = nativecnp.Services

type Server struct {
	listener *quic.Listener
	handler  *nativecnp.Handler
}

func Listen(address string, services Services) (*Server, error) {
	handler, err := nativecnp.NewHandler(services)
	if err != nil {
		return nil, err
	}
	tlsConfig, err := nativecnp.ServerTLSConfig()
	if err != nil {
		return nil, fmt.Errorf("create native TLS configuration: %w", err)
	}
	return ListenWithHandler(address, handler, tlsConfig)
}

func ListenWithHandler(address string, handler *nativecnp.Handler, tlsConfig *tls.Config) (*Server, error) {
	if handler == nil {
		return nil, fmt.Errorf("native CNP handler is required")
	}
	if tlsConfig == nil {
		return nil, fmt.Errorf("native TLS configuration is required")
	}
	listener, err := quic.ListenAddr(address, tlsConfig.Clone(), &quic.Config{
		Allow0RTT:          false,
		EnableDatagrams:    false,
		MaxIncomingStreams: 128,
		KeepAlivePeriod:    15 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("listen for native QUIC connections: %w", err)
	}
	return &Server{listener: listener, handler: handler}, nil
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
	return s.listener.Close()
}

func (s *Server) Serve(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		_ = s.Close()
	}()
	for {
		connection, err := s.listener.Accept(ctx)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, quic.ErrServerClosed) {
				return nil
			}
			return fmt.Errorf("accept native QUIC connection: %w", err)
		}
		go s.handler.ServeSession(ctx, &session{connection: connection})
	}
}
