package proxy

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"log"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/MotherofallVPNs/moav-client/proxy-core/balancer"
	"github.com/MotherofallVPNs/moav-client/proxy-core/plugins"
	"github.com/MotherofallVPNs/moav-client/proxy-core/subscription"
)

// Wire-level tests for the SOCKS5 listener. They speak raw RFC 1928 / 1929
// bytes so the exact replies are pinned independently of any SOCKS5 library.

const (
	methodNoAuth   = 0x00
	methodUserPass = 0x02
	methodNone     = 0xFF

	cmdConnect   = 0x01
	cmdBind      = 0x02
	cmdAssociate = 0x03

	atypIPv4   = 0x01
	atypDomain = 0x03
	atypIPv6   = 0x04

	repSuccess            = 0x00
	repHostUnreachable    = 0x04
	repConnectionRefused  = 0x05
	repCommandUnsupported = 0x07
	repAddrUnsupported    = 0x08

	ioTimeout = 5 * time.Second
)

var loopbackDirect = []plugins.Rule{
	{Match: plugins.MatchExpr{Type: "ip_cidr", Value: "127.0.0.0/8"}, Action: plugins.DecisionDirect, Enabled: true},
	{Match: plugins.MatchExpr{Type: "ip_cidr", Value: "::1/128"}, Action: plugins.DecisionDirect, Enabled: true},
}

// startSOCKS5 serves srv on an ephemeral loopback port and returns its address.
func startSOCKS5(t *testing.T, srv *Server) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = srv.serveSOCKS5(ctx, ln)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(ioTimeout):
			t.Error("serveSOCKS5 did not return after cancel")
		}
	})
	return ln.Addr().String()
}

// directServer returns a Server that dials loopback destinations directly.
func directServer(extra ...plugins.Rule) *Server {
	rules := append(append([]plugins.Rule{}, extra...), loopbackDirect...)
	b := balancer.New(balancer.StrategyLatency)
	b.SetBlockDirect(true) // nothing in these tests may escape via the fallback dial
	return NewServer(0, 0, b, plugins.NewEngine(rules), nil)
}

type echoServer struct {
	addr  *net.TCPAddr
	mu    sync.Mutex
	peers []string
}

// startEcho runs a TCP echo server; it records each client's remote address.
func startEcho(t *testing.T, listenAddr string) *echoServer {
	t.Helper()
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		t.Skipf("cannot listen on %s: %v", listenAddr, err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	e := &echoServer{addr: ln.Addr().(*net.TCPAddr)}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			e.mu.Lock()
			e.peers = append(e.peers, c.RemoteAddr().String())
			e.mu.Unlock()
			go func() {
				defer func() { _ = c.Close() }()
				_, _ = io.Copy(c, c)
			}()
		}
	}()
	return e
}

func (e *echoServer) peerList() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.peers...)
}

func dialRaw(t *testing.T, addr string) *net.TCPConn {
	t.Helper()
	c, err := net.DialTimeout("tcp", addr, ioTimeout)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	_ = c.SetDeadline(time.Now().Add(ioTimeout))
	return c.(*net.TCPConn)
}

func mustWrite(t *testing.T, c net.Conn, b []byte) {
	t.Helper()
	if _, err := c.Write(b); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func mustRead(t *testing.T, c net.Conn, n int) []byte {
	t.Helper()
	buf := make([]byte, n)
	if _, err := io.ReadFull(c, buf); err != nil {
		t.Fatalf("read %d bytes: %v", n, err)
	}
	return buf
}

// expectClosed asserts the server closed the connection without sending more.
func expectClosed(t *testing.T, c net.Conn) {
	t.Helper()
	extra, err := io.ReadAll(c)
	if err != nil {
		t.Fatalf("expected a clean close, got %v", err)
	}
	if len(extra) != 0 {
		t.Fatalf("expected no further bytes before close, got % x", extra)
	}
}

// greet sends the method-selection message and returns the chosen method.
func greet(t *testing.T, c net.Conn, methods ...byte) byte {
	t.Helper()
	mustWrite(t, c, append([]byte{0x05, byte(len(methods))}, methods...))
	rep := mustRead(t, c, 2)
	if rep[0] != 0x05 {
		t.Fatalf("method reply version = %#x", rep[0])
	}
	return rep[1]
}

func userPassMsg(user, pass string) []byte {
	msg := []byte{0x01, byte(len(user))}
	msg = append(msg, user...)
	msg = append(msg, byte(len(pass)))
	return append(msg, pass...)
}

// login runs the RFC 1929 sub-negotiation and returns the status byte.
func login(t *testing.T, c net.Conn, user, pass string) byte {
	t.Helper()
	mustWrite(t, c, userPassMsg(user, pass))
	rep := mustRead(t, c, 2)
	if rep[0] != 0x01 {
		t.Fatalf("auth reply version = %#x", rep[0])
	}
	return rep[1]
}

func requestMsg(cmd, atyp byte, addr []byte, port int) []byte {
	msg := []byte{0x05, cmd, 0x00, atyp}
	if atyp == atypDomain {
		msg = append(msg, byte(len(addr)))
	}
	msg = append(msg, addr...)
	return binary.BigEndian.AppendUint16(msg, uint16(port))
}

func tcpAddrMsg(cmd byte, a *net.TCPAddr) []byte {
	if ip4 := a.IP.To4(); ip4 != nil {
		return requestMsg(cmd, atypIPv4, ip4, a.Port)
	}
	return requestMsg(cmd, atypIPv6, a.IP.To16(), a.Port)
}

type socksReply struct {
	rep  byte
	atyp byte
	addr net.IP
	port int
}

func readReply(t *testing.T, c net.Conn) socksReply {
	t.Helper()
	h := mustRead(t, c, 4)
	if h[0] != 0x05 || h[2] != 0x00 {
		t.Fatalf("malformed reply header % x", h)
	}
	r := socksReply{rep: h[1], atyp: h[3]}
	switch r.atyp {
	case atypIPv4:
		r.addr = net.IP(mustRead(t, c, 4))
	case atypIPv6:
		r.addr = net.IP(mustRead(t, c, 16))
	default:
		t.Fatalf("unexpected reply ATYP %#x", r.atyp)
	}
	r.port = int(binary.BigEndian.Uint16(mustRead(t, c, 2)))
	return r
}

// expectFailure asserts an error reply with the all-zero IPv4 bind address.
func expectFailure(t *testing.T, c net.Conn, want byte) {
	t.Helper()
	r := readReply(t, c)
	if r.rep != want {
		t.Fatalf("reply code = %#x, want %#x", r.rep, want)
	}
	if r.atyp != atypIPv4 || !r.addr.Equal(net.IPv4zero) || r.port != 0 {
		t.Errorf("failure reply should carry 0.0.0.0:0, got atyp=%#x %s:%d", r.atyp, r.addr, r.port)
	}
	expectClosed(t, c)
}

func expectEcho(t *testing.T, c net.Conn, payload []byte) {
	t.Helper()
	mustWrite(t, c, payload)
	if got := mustRead(t, c, len(payload)); !bytes.Equal(got, payload) {
		t.Fatalf("echo mismatch: got % x, want % x", got, payload)
	}
}

func TestSOCKS5_ConnectIPv4NoAuth(t *testing.T) {
	echo := startEcho(t, "127.0.0.1:0")
	c := dialRaw(t, startSOCKS5(t, directServer()))

	if m := greet(t, c, methodNoAuth); m != methodNoAuth {
		t.Fatalf("method = %#x, want no-auth", m)
	}
	mustWrite(t, c, tcpAddrMsg(cmdConnect, echo.addr))
	r := readReply(t, c)
	if r.rep != repSuccess {
		t.Fatalf("reply = %#x, want success", r.rep)
	}
	expectEcho(t, c, []byte("hello through socks5"))

	// BND.ADDR/BND.PORT is the proxy's outbound socket, i.e. the echo peer.
	peers := echo.peerList()
	if len(peers) != 1 {
		t.Fatalf("echo saw %d connections, want 1", len(peers))
	}
	if got := net.JoinHostPort(r.addr.String(), strconv.Itoa(r.port)); r.atyp != atypIPv4 || got != peers[0] {
		t.Errorf("bind address = %s (atyp %#x), want %s", got, r.atyp, peers[0])
	}
}

func TestSOCKS5_ConnectIPv6(t *testing.T) {
	echo := startEcho(t, "[::1]:0")
	c := dialRaw(t, startSOCKS5(t, directServer()))

	greet(t, c, methodNoAuth)
	mustWrite(t, c, tcpAddrMsg(cmdConnect, echo.addr))
	r := readReply(t, c)
	if r.rep != repSuccess || r.atyp != atypIPv6 || !r.addr.Equal(net.IPv6loopback) {
		t.Fatalf("reply = %+v, want success bound to ::1", r)
	}
	expectEcho(t, c, []byte("v6"))
}

func TestSOCKS5_ConnectDomainName(t *testing.T) {
	requireLocalhost(t)
	echo := startEcho(t, "127.0.0.1:0")
	// A domain target reaches the rules as a hostname, so only the domain rule
	// can send it direct; the loopback CIDR rules never see an IP for it.
	eng := plugins.NewEngine([]plugins.Rule{
		{Match: plugins.MatchExpr{Type: "domain", Value: "localhost"}, Action: plugins.DecisionDirect, Enabled: true},
	})
	b := balancer.New(balancer.StrategyLatency)
	b.SetBlockDirect(true)
	c := dialRaw(t, startSOCKS5(t, NewServer(0, 0, b, eng, nil)))

	greet(t, c, methodNoAuth)
	mustWrite(t, c, requestMsg(cmdConnect, atypDomain, []byte("localhost"), echo.addr.Port))
	if r := readReply(t, c); r.rep != repSuccess {
		t.Fatalf("reply = %#x, want success", r.rep)
	}
	expectEcho(t, c, []byte("by name"))
}

func TestSOCKS5_PayloadSentWithRequestIsForwarded(t *testing.T) {
	echo := startEcho(t, "127.0.0.1:0")
	c := dialRaw(t, startSOCKS5(t, directServer()))

	// Greeting, request and first payload bytes in a single write: the server
	// must not lose what it buffered while parsing the handshake.
	payload := []byte("early data")
	msg := append([]byte{0x05, 0x01, methodNoAuth}, tcpAddrMsg(cmdConnect, echo.addr)...)
	mustWrite(t, c, append(msg, payload...))

	if rep := mustRead(t, c, 2); rep[1] != methodNoAuth {
		t.Fatalf("method reply % x", rep)
	}
	if r := readReply(t, c); r.rep != repSuccess {
		t.Fatalf("reply = %#x", r.rep)
	}
	if got := mustRead(t, c, len(payload)); !bytes.Equal(got, payload) {
		t.Fatalf("early payload lost: got %q", got)
	}
}

func TestSOCKS5_HalfCloseIsPropagated(t *testing.T) {
	// Target reads until EOF, then answers and closes.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		in, _ := io.ReadAll(conn)
		_, _ = conn.Write(append([]byte("got:"), in...))
	}()

	c := dialRaw(t, startSOCKS5(t, directServer()))
	greet(t, c, methodNoAuth)
	mustWrite(t, c, tcpAddrMsg(cmdConnect, ln.Addr().(*net.TCPAddr)))
	if r := readReply(t, c); r.rep != repSuccess {
		t.Fatalf("reply = %#x", r.rep)
	}
	mustWrite(t, c, []byte("request"))
	if err := c.CloseWrite(); err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(c)
	if err != nil {
		t.Fatalf("read after half-close: %v", err)
	}
	if string(got) != "got:request" {
		t.Errorf("response after half-close = %q", got)
	}
}

func TestSOCKS5_LargeTransfersAndConcurrency(t *testing.T) {
	echo := startEcho(t, "127.0.0.1:0")
	addr := startSOCKS5(t, directServer())

	const clients = 16
	var wg sync.WaitGroup
	errs := make(chan error, clients)
	for i := 0; i < clients; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- func() error {
				c, err := net.DialTimeout("tcp", addr, ioTimeout)
				if err != nil {
					return err
				}
				defer func() { _ = c.Close() }()
				_ = c.SetDeadline(time.Now().Add(4 * ioTimeout))
				if _, err := c.Write([]byte{0x05, 0x01, methodNoAuth}); err != nil {
					return err
				}
				if _, err := c.Write(tcpAddrMsg(cmdConnect, echo.addr)); err != nil {
					return err
				}
				head := make([]byte, 2+10)
				if _, err := io.ReadFull(c, head); err != nil {
					return err
				}
				if head[1] != methodNoAuth || head[3] != repSuccess {
					return errors.New("handshake failed")
				}
				payload := make([]byte, 512*1024)
				if _, err := rand.Read(payload); err != nil {
					return err
				}
				go func() { _, _ = c.Write(payload) }()
				got := make([]byte, len(payload))
				if _, err := io.ReadFull(c, got); err != nil {
					return err
				}
				if !bytes.Equal(got, payload) {
					return errors.New("payload corrupted in transit")
				}
				return nil
			}()
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
}

func TestSOCKS5_ReplyCodes(t *testing.T) {
	// A loopback port with nothing listening on it.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closed := ln.Addr().(*net.TCPAddr)
	_ = ln.Close()

	echo := startEcho(t, "127.0.0.1:0")
	blockEcho := plugins.Rule{Match: plugins.MatchExpr{Type: "port", Value: strconv.Itoa(echo.addr.Port)}, Action: plugins.DecisionBlock, Enabled: true}

	// No endpoints + block_direct: the proxy decision has nowhere to go.
	noEndpoints := balancer.New(balancer.StrategyLatency)
	noEndpoints.SetBlockDirect(true)

	cases := []struct {
		name string
		srv  *Server
		req  []byte
		want byte
	}{
		{"connection refused", directServer(), tcpAddrMsg(cmdConnect, closed), repConnectionRefused},
		{"blocked by a rule", directServer(blockEcho), tcpAddrMsg(cmdConnect, echo.addr), repHostUnreachable},
		{"blocked by torrent heuristic", NewServer(0, 0, noEndpoints, plugins.NewEngine(loopbackDirect), &plugins.TorrentBlocker{Enabled: true}),
			requestMsg(cmdConnect, atypIPv4, []byte{127, 0, 0, 1}, 6881), repHostUnreachable},
		{"no endpoint and block_direct", NewServer(0, 0, noEndpoints, plugins.NewEngine(nil), nil), tcpAddrMsg(cmdConnect, echo.addr), repHostUnreachable},
		{"BIND is not supported", directServer(), tcpAddrMsg(cmdBind, echo.addr), repCommandUnsupported},
		{"UDP ASSOCIATE is not supported", directServer(), requestMsg(cmdAssociate, atypIPv4, []byte{0, 0, 0, 0}, 0), repCommandUnsupported},
		{"UDP ASSOCIATE with client address", directServer(), requestMsg(cmdAssociate, atypIPv4, []byte{127, 0, 0, 1}, 40000), repCommandUnsupported},
		{"unknown command", directServer(), tcpAddrMsg(0x09, echo.addr), repCommandUnsupported},
		{"unknown address type", directServer(), []byte{0x05, cmdConnect, 0x00, 0x05, 1, 2, 3, 4, 0, 80}, repAddrUnsupported},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := dialRaw(t, startSOCKS5(t, tc.srv))
			greet(t, c, methodNoAuth)
			mustWrite(t, c, tc.req)
			expectFailure(t, c, tc.want)
		})
	}
	if peers := echo.peerList(); len(peers) != 0 {
		t.Errorf("no request above may reach the target, but it saw %v", peers)
	}
}

func TestSOCKS5_NonSOCKS5ClientIsDropped(t *testing.T) {
	addr := startSOCKS5(t, directServer())
	for name, hello := range map[string][]byte{
		"socks4 connect": {0x04, 0x01, 0x00, 0x50, 127, 0, 0, 1, 0x00},
		"http request":   []byte("GET / HTTP/1.1\r\nHost: example.com\r\nUser-Agent: curl/8.0\r\nAccept: */*\r\nConnection: close\r\n\r\n"),
	} {
		t.Run(name, func(t *testing.T) {
			c := dialRaw(t, addr)
			mustWrite(t, c, hello)
			_ = c.CloseWrite()
			expectClosed(t, c)
		})
	}
}

func TestSOCKS5_MethodNegotiation(t *testing.T) {
	open := startSOCKS5(t, directServer())
	authed := startSOCKS5(t, directServer().WithAuth("alice", "s3cret"))
	// A username without a password leaves auth off.
	userOnly := startSOCKS5(t, directServer().WithAuth("alice", ""))

	cases := []struct {
		name    string
		addr    string
		offered []byte
		want    byte
	}{
		{"open: no-auth offered", open, []byte{methodNoAuth}, methodNoAuth},
		{"open: user/pass listed first", open, []byte{methodUserPass, methodNoAuth}, methodNoAuth},
		{"open: only user/pass offered", open, []byte{methodUserPass}, methodNone},
		{"open: only GSSAPI offered", open, []byte{0x01}, methodNone},
		{"open: no methods offered", open, nil, methodNone},
		{"authed: only no-auth offered", authed, []byte{methodNoAuth}, methodNone},
		{"authed: both offered", authed, []byte{methodNoAuth, methodUserPass}, methodUserPass},
		{"authed: user/pass offered", authed, []byte{methodUserPass}, methodUserPass},
		{"username without password: no-auth", userOnly, []byte{methodNoAuth}, methodNoAuth},
		{"username without password: user/pass refused", userOnly, []byte{methodUserPass}, methodNone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := dialRaw(t, tc.addr)
			if got := greet(t, c, tc.offered...); got != tc.want {
				t.Fatalf("selected method = %#x, want %#x", got, tc.want)
			}
			if tc.want == methodNone {
				expectClosed(t, c)
			}
		})
	}
}

func TestSOCKS5_UserPassAuth(t *testing.T) {
	echo := startEcho(t, "127.0.0.1:0")
	long := string(bytes.Repeat([]byte("x"), 255))

	cases := []struct {
		name       string
		cfgUser    string
		cfgPass    string
		user, pass string
		ok         bool
	}{
		{"correct credentials", "alice", "s3cret", "alice", "s3cret", true},
		{"wrong password", "alice", "s3cret", "alice", "nope", false},
		{"wrong username", "alice", "s3cret", "bob", "s3cret", false},
		{"empty password sent", "alice", "s3cret", "alice", "", false},
		{"empty username sent", "alice", "s3cret", "", "s3cret", false},
		{"case-sensitive username", "alice", "s3cret", "Alice", "s3cret", false},
		{"password-only config, empty username", "", "s3cret", "", "s3cret", true},
		{"password-only config, some username", "", "s3cret", "anyone", "s3cret", false},
		{"password-only config, wrong password", "", "s3cret", "", "wrong", false},
		{"255-byte username and password", long, long, long, long, true},
		{"non-ASCII credentials", "کاربر", "گذرواژه", "کاربر", "گذرواژه", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := dialRaw(t, startSOCKS5(t, directServer().WithAuth(tc.cfgUser, tc.cfgPass)))
			if m := greet(t, c, methodUserPass); m != methodUserPass {
				t.Fatalf("method = %#x, want user/pass", m)
			}
			status := login(t, c, tc.user, tc.pass)
			if !tc.ok {
				if status == 0x00 {
					t.Fatal("bad credentials were accepted")
				}
				expectClosed(t, c)
				return
			}
			if status != 0x00 {
				t.Fatalf("auth status = %#x, want success", status)
			}
			mustWrite(t, c, tcpAddrMsg(cmdConnect, echo.addr))
			if r := readReply(t, c); r.rep != repSuccess {
				t.Fatalf("reply = %#x", r.rep)
			}
			expectEcho(t, c, []byte("authenticated"))
		})
	}
}

func TestSOCKS5_AuthCannotBeSkipped(t *testing.T) {
	echo := startEcho(t, "127.0.0.1:0")
	addr := startSOCKS5(t, directServer().WithAuth("alice", "s3cret"))

	t.Run("request sent instead of credentials", func(t *testing.T) {
		c := dialRaw(t, addr)
		greet(t, c, methodUserPass)
		// 0x05 is not a valid sub-negotiation version: drop without a reply.
		mustWrite(t, c, tcpAddrMsg(cmdConnect, echo.addr))
		_ = c.CloseWrite()
		expectClosed(t, c)
	})
	t.Run("request pipelined after failed login", func(t *testing.T) {
		c := dialRaw(t, addr)
		greet(t, c, methodUserPass)
		mustWrite(t, c, append(userPassMsg("alice", "wrong"), tcpAddrMsg(cmdConnect, echo.addr)...))
		if rep := mustRead(t, c, 2); rep[0] != 0x01 || rep[1] == 0x00 {
			t.Fatalf("auth reply % x, want failure", rep)
		}
		_ = c.CloseWrite()
		expectClosed(t, c)
	})
	if peers := echo.peerList(); len(peers) != 0 {
		t.Errorf("unauthenticated client reached the target: %v", peers)
	}
}

func TestSOCKS5_ProxyDecisionDialsThroughBalancerEndpoint(t *testing.T) {
	echo := startEcho(t, "127.0.0.1:0")
	// Upstream stands in for a sing-box / sidecar SOCKS5 port.
	upstream := startSOCKS5(t, directServer())

	b := balancer.New(balancer.StrategyLatency)
	b.SetBlockDirect(true)
	b.SetEndpoints([]subscription.Endpoint{{
		ID: "sidecar:test", Protocol: "sidecar", Name: "test", Address: upstream,
		Config: map[string]string{"socks5_addr": upstream}, Enabled: true, Status: "ok",
	}})
	// No rules: every destination takes the proxy decision.
	front := startSOCKS5(t, NewServer(0, 0, b, plugins.NewEngine(nil), nil).WithAuth("", "pw"))

	c := dialRaw(t, front)
	if m := greet(t, c, methodNoAuth, methodUserPass); m != methodUserPass {
		t.Fatalf("method = %#x", m)
	}
	if st := login(t, c, "", "pw"); st != 0x00 {
		t.Fatalf("auth status %#x", st)
	}
	mustWrite(t, c, tcpAddrMsg(cmdConnect, echo.addr))
	if r := readReply(t, c); r.rep != repSuccess {
		t.Fatalf("reply = %#x", r.rep)
	}
	expectEcho(t, c, []byte("via endpoint"))

	var dials int64
	for _, st := range b.Stats().Snapshot() {
		if st.ID == "sidecar:test" {
			dials = st.Dials
		}
	}
	if dials != 1 {
		t.Errorf("endpoint dial count = %d, want 1 (stats: %+v)", dials, b.Stats().Snapshot())
	}
}

func TestSOCKS5_ServeStopsOnContextCancel(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- directServer().serveSOCKS5(ctx, ln) }()

	c := dialRaw(t, ln.Addr().String())
	greet(t, c, methodNoAuth)

	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Error("serveSOCKS5 should report the closed listener")
		}
	case <-time.After(ioTimeout):
		t.Fatal("serveSOCKS5 did not return after cancel")
	}
	if c2, err := net.DialTimeout("tcp", ln.Addr().String(), time.Second); err == nil {
		_ = c2.Close()
		t.Error("listener still accepting after cancel")
	}
}

func TestSOCKS5_LoggerKeepsErrPrefix(t *testing.T) {
	var buf bytes.Buffer
	socks5Logger{log.New(&buf, "", 0)}.Errorf("server: %v", errors.New("boom"))
	if got, want := buf.String(), "[ERR] socks: server: boom\n"; got != want {
		t.Errorf("log line = %q, want %q", got, want)
	}

	// The library quotes the client's raw destination; it must stay one line.
	buf.Reset()
	socks5Logger{log.New(&buf, "", 0)}.Errorf("connect to %v failed", "a.test\nBLOCK b.test\x00:443")
	if got, want := buf.String(), "[ERR] socks: connect to a.test?BLOCK b.test?:443 failed\n"; got != want {
		t.Errorf("log line = %q, want %q", got, want)
	}
}
