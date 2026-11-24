// Package proxy provides SOCKS5 and HTTP CONNECT listeners.
package proxy

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"

	gosocks5 "github.com/things-go/go-socks5"
	"github.com/things-go/go-socks5/statute"

	"github.com/MotherofallVPNs/moav-client/proxy-core/balancer"
	"github.com/MotherofallVPNs/moav-client/proxy-core/plugins"
)

// Server holds both SOCKS5 and HTTP CONNECT listeners.
type Server struct {
	socks5Port   int
	httpPort     int
	router       *router
	authUsername string
	authPassword string
}

// NewServer creates a Server.
func NewServer(socks5Port, httpPort int, b *balancer.Balancer, eng *plugins.Engine, tb *plugins.TorrentBlocker) *Server {
	return &Server{
		socks5Port: socks5Port,
		httpPort:   httpPort,
		router:     newRouter(b, eng, tb),
	}
}

// WithAuth configures optional username/password authentication on the SOCKS5 server.
func (s *Server) WithAuth(username, password string) *Server {
	s.authUsername = username
	s.authPassword = password
	return s
}

// ListenAndServeSOCKS5 starts the SOCKS5 proxy listener.
func (s *Server) ListenAndServeSOCKS5(ctx context.Context) error {
	addr := fmt.Sprintf("0.0.0.0:%d", s.socks5Port)
	log.Printf("SOCKS5 listening on %s", addr)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("socks5 listen: %w", err)
	}
	return s.serveSOCKS5(ctx, ln)
}

// serveSOCKS5 serves SOCKS5 connections on ln until ctx is cancelled.
func (s *Server) serveSOCKS5(ctx context.Context, ln net.Listener) error {
	srv := s.newSOCKS5Server()
	go func() {
		<-ctx.Done()
		ln.Close()
	}()
	return srv.Serve(ln)
}

// newSOCKS5Server builds the SOCKS5 protocol server: optional user/pass auth
// plus the dial hook that routes every CONNECT through the plugin engine.
func (s *Server) newSOCKS5Server() *gosocks5.Server {
	opts := []gosocks5.Option{
		gosocks5.WithDial(s.socks5Dial),
		// Name rules must see the hostname, so the library may not resolve it
		// first; the router resolves it itself only for IP rules.
		gosocks5.WithResolver(unresolved{}),
		gosocks5.WithLogger(socks5Logger{log.New(os.Stdout, "", log.LstdFlags)}),
		// CONNECT only. The library's own UDP ASSOCIATE relay would dial
		// datagrams outside the tunnel, so both commands stay refused.
		gosocks5.WithBindHandle(refuseSOCKS5Command),
		gosocks5.WithAssociateHandle(refuseSOCKS5Command),
	}

	// A password alone turns auth on; the username may be empty (clients then
	// send an empty username). Requiring both would leave the proxy open when
	// someone sets only a password.
	if s.authPassword != "" {
		creds := gosocks5.StaticCredentials{s.authUsername: s.authPassword}
		opts = append(opts, gosocks5.WithAuthMethods([]gosocks5.Authenticator{
			gosocks5.UserPassAuthenticator{Credentials: creds},
		}))
		log.Printf("SOCKS5 auth enabled (user %q)", s.authUsername)
	}
	return gosocks5.NewServer(opts...)
}

// refuseSOCKS5Command answers a request with "command not supported".
func refuseSOCKS5Command(_ context.Context, w io.Writer, _ *gosocks5.Request) error {
	return gosocks5.SendReply(w, statute.RepCommandNotSupported, nil)
}

// unresolved hands a domain target to the dial hook as the client sent it.
// The router decides when, and by whom, it is resolved.
type unresolved struct{}

func (unresolved) Resolve(ctx context.Context, _ string) (context.Context, net.IP, error) {
	return ctx, nil, nil
}

// socks5Logger writes the library's per-connection errors to stdout, one line
// each: they quote the client's raw destination, which may hold control bytes.
type socks5Logger struct{ *log.Logger }

func (l socks5Logger) Errorf(format string, args ...interface{}) {
	msg := strings.Map(func(r rune) rune {
		if r < ' ' || r == 0x7f {
			return '?'
		}
		return r
	}, fmt.Sprintf(format, args...))
	l.Printf("[ERR] socks: %s", msg)
}

// socks5Dial is the SOCKS5 dial hook: it routes the destination, a hostname or
// an IP literal, through the plugin decision (block / direct / proxy).
func (s *Server) socks5Dial(ctx context.Context, network, addr string) (net.Conn, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
		portStr = "0"
	}
	port := 0
	if p, err2 := net.LookupPort("tcp", portStr); err2 == nil {
		port = p
	}
	if !validHost(host) {
		return nil, fmt.Errorf("invalid destination host %q", host)
	}

	r := s.router.decide(ctx, host, port, network)
	switch r.decision {
	case plugins.DecisionBlock:
		log.Printf("BLOCK SOCKS5 %s:%d", host, port)
	case plugins.DecisionDirect:
		log.Printf("DIRECT SOCKS5 %s:%d%s", host, port, viaAddrs(r))
	}
	return s.router.dial(ctx, r, network, host, port)
}

// viaAddrs notes, for the log, the resolved addresses an IP rule matched.
func viaAddrs(r route) string {
	if len(r.addrs) == 0 {
		return ""
	}
	return fmt.Sprintf(" (IP rule on %v)", r.addrs)
}

// validHost rejects an empty name and one with spaces or control characters:
// a SOCKS5 domain is raw client bytes that end up in rules and log lines.
func validHost(host string) bool {
	if host == "" {
		return false
	}
	for i := 0; i < len(host); i++ {
		if c := host[i]; c <= ' ' || c == 0x7f {
			return false
		}
	}
	return true
}

// ListenAndServeHTTP starts the HTTP CONNECT proxy listener.
func (s *Server) ListenAndServeHTTP(ctx context.Context) error {
	addr := fmt.Sprintf("0.0.0.0:%d", s.httpPort)
	log.Printf("HTTP CONNECT listening on %s", addr)

	h := &httpHandler{router: s.router}
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
