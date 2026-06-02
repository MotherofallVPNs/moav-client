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
	"github.com/ibeezhan/moav-client/proxy-core/plugins"
)

// Server holds both SOCKS5 and HTTP CONNECT listeners.
type Server struct {
	socks5Port int
	httpPort   int
	balancer   *balancer.Balancer
	engine     *plugins.Engine
	torrent    *plugins.TorrentBlocker
}

// NewServer creates a Server.
func NewServer(socks5Port, httpPort int, b *balancer.Balancer, eng *plugins.Engine, tb *plugins.TorrentBlocker) *Server {
	return &Server{
		socks5Port: socks5Port,
		httpPort:   httpPort,
		balancer:   b,
		engine:     eng,
		torrent:    tb,
	}
}

// ListenAndServeSOCKS5 starts the SOCKS5 proxy listener.
func (s *Server) ListenAndServeSOCKS5(ctx context.Context) error {
	conf := &gosocks5.Config{
		Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, portStr, err := net.SplitHostPort(addr)
			if err != nil {
				host = addr
				portStr = "0"
			}
			port := 0
			if p, err2 := net.LookupPort("tcp", portStr); err2 == nil {
				port = p
			}

			// Apply plugin engine decision.
			dec := s.pluginDecide(host, port, network)
			switch dec {
			case plugins.DecisionBlock:
				log.Printf("BLOCK SOCKS5 %s:%d", host, port)
				return nil, fmt.Errorf("blocked by plugin engine")
			case plugins.DecisionDirect:
				log.Printf("DIRECT SOCKS5 %s:%d", host, port)
				return net.Dial(network, addr)
			default:
				return s.balancer.DialContext(network, addr)
			}
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

	h := &httpHandler{balancer: s.balancer, engine: s.engine, torrent: s.torrent}
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

// pluginDecide applies TorrentBlocker then Engine to get a Decision.
func (s *Server) pluginDecide(host string, port int, proto string) plugins.Decision {
	if s.torrent != nil && s.torrent.Match(host, port, proto) {
		return plugins.DecisionBlock
	}
	if s.engine != nil {
		return s.engine.Evaluate(host, port, proto)
	}
	return plugins.DecisionProxy
}
