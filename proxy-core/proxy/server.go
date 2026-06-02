// Package proxy provides SOCKS5 and HTTP CONNECT listeners.
package proxy

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"

	gosocks5 "github.com/armon/go-socks5"

	"github.com/ibeezhan/moav-client/proxy-core/balancer"
)

// Server holds both SOCKS5 and HTTP CONNECT listeners.
type Server struct {
	socks5Port int
	httpPort   int
	balancer   *balancer.Balancer
}

// NewServer creates a Server.
func NewServer(socks5Port, httpPort int, b *balancer.Balancer) *Server {
	return &Server{
		socks5Port: socks5Port,
		httpPort:   httpPort,
		balancer:   b,
	}
}

// ListenAndServeSOCKS5 starts the SOCKS5 proxy listener.
func (s *Server) ListenAndServeSOCKS5(ctx context.Context) error {
	conf := &gosocks5.Config{
		Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return s.balancer.DialContext(network, addr)
		},
	}
	srv, err := gosocks5.New(conf)
	if err != nil {
		return fmt.Errorf("socks5 init: %w", err)
	}

	addr := fmt.Sprintf("0.0.0.0:%d", s.socks5Port)
	log.Printf("SOCKS5 listening on %s", addr)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("socks5 listen: %w", err)
	}

	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	return srv.Serve(ln)
}

// ListenAndServeHTTP starts the HTTP CONNECT proxy listener.
func (s *Server) ListenAndServeHTTP(ctx context.Context) error {
	addr := fmt.Sprintf("0.0.0.0:%d", s.httpPort)
	log.Printf("HTTP CONNECT listening on %s", addr)

	h := &httpHandler{balancer: s.balancer}
	srv := &http.Server{Addr: addr, Handler: h}

	go func() {
		<-ctx.Done()
		srv.Shutdown(context.Background()) //nolint:errcheck
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("http listen: %w", err)
	}
	return nil
}
