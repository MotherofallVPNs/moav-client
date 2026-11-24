package proxy

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/MotherofallVPNs/moav-client/proxy-core/balancer"
	"github.com/MotherofallVPNs/moav-client/proxy-core/plugins"
	"github.com/MotherofallVPNs/moav-client/proxy-core/subscription"
)

// Routing-order tests: both inbounds must hand the rule engine the destination
// exactly as the client sent it, and resolve a hostname only after the decision.

type upstreamTarget struct {
	atyp byte
	host string
	port int
}

// recordingUpstream stands in for a sing-box / sidecar SOCKS5 port. It records
// every CONNECT target as it arrived on the wire, then echoes.
type recordingUpstream struct {
	addr    string
	mu      sync.Mutex
	targets []upstreamTarget
}

func startRecordingUpstream(t *testing.T) *recordingUpstream {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	u := &recordingUpstream{addr: ln.Addr().String()}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go u.serve(c)
		}
	}()
	return u
}

func (u *recordingUpstream) serve(c net.Conn) {
	defer func() { _ = c.Close() }()
	r := bufio.NewReader(c)
	read := func(n int) []byte {
		b := make([]byte, n)
		if _, err := io.ReadFull(r, b); err != nil {
			return nil
		}
		return b
	}

	hello := read(2)
	if hello == nil || hello[0] != 0x05 || read(int(hello[1])) == nil {
		return
	}
	if _, err := c.Write([]byte{0x05, methodNoAuth}); err != nil {
		return
	}
	head := read(4)
	if head == nil || head[1] != cmdConnect {
		return
	}
	tgt := upstreamTarget{atyp: head[3]}
	switch tgt.atyp {
	case atypIPv4, atypIPv6:
		n := net.IPv4len
		if tgt.atyp == atypIPv6 {
			n = net.IPv6len
		}
		ip := read(n)
		if ip == nil {
			return
		}
		tgt.host = net.IP(ip).String()
	case atypDomain:
		l := read(1)
		if l == nil {
			return
		}
		name := read(int(l[0]))
		if name == nil {
			return
		}
		tgt.host = string(name)
	default:
		return
	}
	port := read(2)
	if port == nil {
		return
	}
	tgt.port = int(binary.BigEndian.Uint16(port))

	u.mu.Lock()
	u.targets = append(u.targets, tgt)
	u.mu.Unlock()

	if _, err := c.Write([]byte{0x05, repSuccess, 0x00, atypIPv4, 0, 0, 0, 0, 0, 0}); err != nil {
		return
	}
	_, _ = io.Copy(c, r)
}

func (u *recordingUpstream) seen() []upstreamTarget {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]upstreamTarget(nil), u.targets...)
}

// balancerVia returns a balancer whose only endpoint is the given SOCKS5 port.
// block_direct is on, so nothing can leave through the fallback dial.
func balancerVia(upstreamAddr string) *balancer.Balancer {
	b := balancer.New(balancer.StrategyLatency)
	b.SetBlockDirect(true)
	b.SetEndpoints([]subscription.Endpoint{{
		ID: "sidecar:test", Protocol: "sidecar", Name: "test", Address: upstreamAddr,
		Config: map[string]string{"socks5_addr": upstreamAddr}, Enabled: true, Status: "ok",
	}})
	return b
}

func rule(matchType, value string, action plugins.Decision) plugins.Rule {
	return plugins.Rule{Match: plugins.MatchExpr{Type: matchType, Value: value}, Action: action, Enabled: true}
}

// inbound opens one tunnel to host:port through a listener. It reports whether
// the listener accepted the request; on success the returned stream is the
// tunnel.
type inbound struct {
	name    string
	connect func(t *testing.T, rt *router, host string, port int) (io.ReadWriter, bool)
}

var inbounds = []inbound{
	{"socks5", func(t *testing.T, rt *router, host string, port int) (io.ReadWriter, bool) {
		srv := NewServer(0, 0, nil, nil, nil)
		srv.router = rt
		c := dialRaw(t, startSOCKS5(t, srv))
		greet(t, c, methodNoAuth)
		mustWrite(t, c, hostRequestMsg(host, port))
		if r := readReply(t, c); r.rep != repSuccess {
			if r.rep != repHostUnreachable {
				t.Errorf("refusal reply = %#x, want %#x", r.rep, repHostUnreachable)
			}
			expectClosed(t, c)
			return nil, false
		}
		return c, true
	}},
	{"http-connect", func(t *testing.T, rt *router, host string, port int) (io.ReadWriter, bool) {
		srv := httptest.NewServer(&httpHandler{router: rt})
		t.Cleanup(srv.Close)
		c := dialRaw(t, srv.Listener.Addr().String())
		target := net.JoinHostPort(host, strconv.Itoa(port))
		mustWrite(t, c, []byte(fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target)))
		br := bufio.NewReader(c)
		resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodConnect})
		if err != nil {
			return nil, false // a blocked CONNECT is dropped without a response
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("CONNECT status = %d", resp.StatusCode)
			return nil, false
		}
		return struct {
			io.Reader
			io.Writer
		}{br, c}, true
	}},
}

// hostRequestMsg builds a CONNECT request the way a socks5h client does: an IP
// literal goes out as ATYP IPv4/IPv6, anything else as ATYP domain.
func hostRequestMsg(host string, port int) []byte {
	if ip := net.ParseIP(host); ip != nil {
		return tcpAddrMsg(cmdConnect, &net.TCPAddr{IP: ip, Port: port})
	}
	return requestMsg(cmdConnect, atypDomain, []byte(host), port)
}

// The .test names below never resolve (RFC 6761), and the router gets a
// resolver that knows none of them, so no case depends on the host's DNS.
func TestRouting_DestinationReachesRulesAndUpstreamUnresolved(t *testing.T) {
	onlyExampleProxied := []plugins.Rule{
		rule("domain_suffix", "example.test", plugins.DecisionProxy),
		rule("protocol", "tcp", plugins.DecisionBlock),
	}
	blockSuffix := []plugins.Rule{rule("domain_suffix", "example.test", plugins.DecisionBlock)}
	blockExact := []plugins.Rule{rule("domain", "example.test", plugins.DecisionBlock)}
	blockCIDR := []plugins.Rule{rule("ip_cidr", "203.0.113.0/24", plugins.DecisionBlock)}

	cases := []struct {
		name     string
		rules    []plugins.Rule
		torrent  bool
		host     string
		port     int
		blocked  bool
		wantAtyp byte
	}{
		{name: "no rules: hostname takes the default proxy route", host: "www.example.test", port: 443, wantAtyp: atypDomain},
		{name: "domain_suffix rule matches a subdomain", rules: onlyExampleProxied, host: "api.example.test", port: 443, wantAtyp: atypDomain},
		{name: "domain_suffix rule matches the apex", rules: onlyExampleProxied, host: "example.test", port: 8443, wantAtyp: atypDomain},
		{name: "hostname outside the suffix falls to the next rule", rules: onlyExampleProxied, host: "api.other.test", port: 443, blocked: true},
		{name: "domain_suffix block is case-insensitive", rules: blockSuffix, host: "API.Example.Test", port: 443, blocked: true},
		{name: "domain_suffix does not match a lookalike", rules: blockSuffix, host: "notexample.test", port: 443, wantAtyp: atypDomain},
		{name: "domain rule matches the exact host", rules: blockExact, host: "example.test", port: 443, blocked: true},
		{name: "domain rule does not match a subdomain", rules: blockExact, host: "www.example.test", port: 443, wantAtyp: atypDomain},
		{name: "ip_cidr rule matches an IPv4 literal", rules: blockCIDR, host: "203.0.113.7", port: 443, blocked: true},
		{name: "IPv4 literal outside the CIDR is proxied", rules: blockCIDR, host: "198.51.100.7", port: 443, wantAtyp: atypIPv4},
		{name: "IPv6 literal with no matching rule is proxied", rules: blockCIDR, host: "2001:db8::7", port: 443, wantAtyp: atypIPv6},
		{name: "ip_cidr rule: a name that does not resolve locally takes the default route", rules: blockCIDR, host: "www.example.test", port: 443, wantAtyp: atypDomain},
		{name: "domain_suffix block matches a name with a trailing dot", rules: blockSuffix, host: "www.example.test.", port: 443, blocked: true},
		{name: "domain block matches the apex with a trailing dot", rules: blockExact, host: "example.test.", port: 443, blocked: true},
		{name: "torrent heuristic sees the tracker hostname", torrent: true, host: "tracker.opentrackr.org", port: 1337, blocked: true},
	}

	for _, in := range inbounds {
		for _, tc := range cases {
			t.Run(in.name+"/"+tc.name, func(t *testing.T) {
				up := startRecordingUpstream(t)
				b := balancerVia(up.addr)
				var tb *plugins.TorrentBlocker
				if tc.torrent {
					tb = &plugins.TorrentBlocker{Enabled: true}
				}

				rt := newRouter(b, plugins.NewEngine(tc.rules), tb)
				rt.resolver = &fakeResolver{}
				stream, ok := in.connect(t, rt, tc.host, tc.port)
				if tc.blocked {
					if ok {
						t.Error("request was accepted, want it blocked")
					}
					if got := up.seen(); len(got) != 0 {
						t.Errorf("blocked request reached the upstream: %+v", got)
					}
					return
				}
				if !ok {
					t.Fatal("request was refused, want it proxied")
				}
				payload := []byte("via endpoint")
				if _, err := stream.Write(payload); err != nil {
					t.Fatal(err)
				}
				echoed := make([]byte, len(payload))
				if _, err := io.ReadFull(stream, echoed); err != nil || string(echoed) != string(payload) {
					t.Fatalf("echo through the upstream = %q, %v", echoed, err)
				}

				// The upstream must get the client's own target, so a hostname is
				// resolved at the far end of the tunnel.
				want := upstreamTarget{atyp: tc.wantAtyp, host: tc.host, port: tc.port}
				got := up.seen()
				if len(got) != 1 || got[0].atyp != want.atyp || !strings.EqualFold(got[0].host, want.host) || got[0].port != want.port {
					t.Errorf("upstream saw %+v, want [%+v]", got, want)
				}
			})
		}
	}
}

// requireLocalhost skips unless "localhost" resolves to 127.0.0.1 here.
func requireLocalhost(t *testing.T) {
	t.Helper()
	addrs, err := net.LookupHost("localhost")
	for _, a := range addrs {
		if a == "127.0.0.1" {
			return
		}
	}
	t.Skipf("localhost does not resolve to 127.0.0.1 here (%v, %v)", addrs, err)
}

// A name that does resolve locally: the old listener turned it into 127.0.0.1
// before the rules ran, so these fail on ordering alone, not on DNS.
func TestSOCKS5_DomainRulesSeeHostnameNotResolvedIP(t *testing.T) {
	requireLocalhost(t)
	noUpstream := func() *balancer.Balancer {
		b := balancer.New(balancer.StrategyLatency)
		b.SetBlockDirect(true)
		return b
	}
	blockLoopback := []plugins.Rule{
		rule("ip_cidr", "127.0.0.0/8", plugins.DecisionBlock),
		rule("ip_cidr", "::1/128", plugins.DecisionBlock),
	}

	t.Run("domain_suffix direct rule wins over a later block", func(t *testing.T) {
		echo := startEcho(t, "127.0.0.1:0")
		eng := plugins.NewEngine([]plugins.Rule{
			rule("domain_suffix", "localhost", plugins.DecisionDirect),
			rule("protocol", "tcp", plugins.DecisionBlock),
		})
		c := dialRaw(t, startSOCKS5(t, NewServer(0, 0, noUpstream(), eng, nil)))
		greet(t, c, methodNoAuth)
		mustWrite(t, c, requestMsg(cmdConnect, atypDomain, []byte("localhost"), echo.addr.Port))
		if r := readReply(t, c); r.rep != repSuccess {
			t.Fatalf("reply = %#x, want success: the domain_suffix rule did not match the hostname", r.rep)
		}
		expectEcho(t, c, []byte("direct by name"))
	})

	t.Run("domain block rule is enforced", func(t *testing.T) {
		echo := startEcho(t, "127.0.0.1:0")
		c := dialRaw(t, startSOCKS5(t, directServer(rule("domain", "localhost", plugins.DecisionBlock))))
		greet(t, c, methodNoAuth)
		mustWrite(t, c, requestMsg(cmdConnect, atypDomain, []byte("localhost"), echo.addr.Port))
		expectFailure(t, c, repHostUnreachable)
		if peers := echo.peerList(); len(peers) != 0 {
			t.Errorf("blocked hostname reached the target: %v", peers)
		}
	})

	t.Run("a name rule wins over an IP rule on its address", func(t *testing.T) {
		echo := startEcho(t, "127.0.0.1:0")
		rules := append(append([]plugins.Rule{}, blockLoopback...), rule("domain", "localhost", plugins.DecisionDirect))
		addr := startSOCKS5(t, NewServer(0, 0, noUpstream(), plugins.NewEngine(rules), nil))

		// Name rules run first, so the domain rule decides before the CIDR
		// block could see 127.0.0.1.
		c := dialRaw(t, addr)
		greet(t, c, methodNoAuth)
		mustWrite(t, c, requestMsg(cmdConnect, atypDomain, []byte("localhost"), echo.addr.Port))
		if r := readReply(t, c); r.rep != repSuccess {
			t.Fatalf("reply = %#x, want success", r.rep)
		}
		expectEcho(t, c, []byte("by name"))

		// The same destination as an IP literal still hits the CIDR block.
		c = dialRaw(t, addr)
		greet(t, c, methodNoAuth)
		mustWrite(t, c, tcpAddrMsg(cmdConnect, echo.addr))
		expectFailure(t, c, repHostUnreachable)
		if peers := echo.peerList(); len(peers) != 1 {
			t.Errorf("echo saw %d connections, want only the by-name one", len(peers))
		}
	})
}

// Some clients put an IP literal in the domain field; IP rules must still match.
func TestSOCKS5_IPLiteralInDomainFieldMatchesIPRules(t *testing.T) {
	echo := startEcho(t, "127.0.0.1:0")
	// No endpoints and block_direct: only the loopback ip_cidr rule can connect.
	c := dialRaw(t, startSOCKS5(t, directServer()))
	greet(t, c, methodNoAuth)
	mustWrite(t, c, requestMsg(cmdConnect, atypDomain, []byte("127.0.0.1"), echo.addr.Port))
	if r := readReply(t, c); r.rep != repSuccess {
		t.Fatalf("reply = %#x, want success", r.rep)
	}
	expectEcho(t, c, []byte("literal as a name"))
}

// A SOCKS5 domain is arbitrary client bytes; junk must not reach rules or logs.
func TestSOCKS5_MalformedHostnameIsRefused(t *testing.T) {
	for name, host := range map[string]string{
		"empty":              "",
		"space":              "bad host.test",
		"newline":            "a.test\nBLOCK SOCKS5 b.test",
		"NUL":                "a.test\x00b.test",
		"only a trailing CR": "a.test\r",
	} {
		t.Run(name, func(t *testing.T) {
			up := startRecordingUpstream(t)
			c := dialRaw(t, startSOCKS5(t, NewServer(0, 0, balancerVia(up.addr), plugins.NewEngine(nil), nil)))
			greet(t, c, methodNoAuth)
			mustWrite(t, c, requestMsg(cmdConnect, atypDomain, []byte(host), 443))
			expectFailure(t, c, repHostUnreachable)
			if got := up.seen(); len(got) != 0 {
				t.Errorf("malformed host reached the upstream: %+v", got)
			}
		})
	}
}
