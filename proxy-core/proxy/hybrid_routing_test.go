package proxy

import (
	"context"
	"io"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/MotherofallVPNs/moav-client/proxy-core/plugins"
)

// Hybrid resolution: name rules on the hostname first; only if none matched
// and IP rules exist, a local lookup classifies the name before any dial.

// fakeResolver answers from a fixed table and records every lookup.
type fakeResolver struct {
	answers map[string][]netip.Addr
	mu      sync.Mutex
	lookups []string
}

func (f *fakeResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	f.mu.Lock()
	f.lookups = append(f.lookups, host)
	f.mu.Unlock()
	if a, ok := f.answers[host]; ok {
		return a, nil
	}
	return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
}

func (f *fakeResolver) seen() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.lookups...)
}

// directRecorder stands in for the direct dialer: it records the target it was
// asked for and connects to a local echo server instead.
type directRecorder struct {
	echo    string
	mu      sync.Mutex
	targets []string
}

func (d *directRecorder) dial(ctx context.Context, network, addr string) (net.Conn, error) {
	d.mu.Lock()
	d.targets = append(d.targets, addr)
	d.mu.Unlock()
	var nd net.Dialer
	return nd.DialContext(ctx, network, d.echo)
}

func (d *directRecorder) seen() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.targets...)
}

// writeGeoIP makes geoip/<cc>.txt, relative to a fresh working directory.
func writeGeoIP(t *testing.T, cc string, cidrs ...string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "geoip"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := strings.Join(cidrs, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(dir, "geoip", cc+".txt"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
}

// The anti-leak case end to end, with the host's real resolver: a socks5h
// name whose address is on the geoip:ir list must go direct, and the tunnel
// must never be asked for it. "localhost" stands in for an in-country name and
// the test list for Iran's, so the direct dial lands on a local echo server.
func TestSOCKS5_GeoIPDirectClassifiesHostnameBeforeDial(t *testing.T) {
	requireLocalhost(t)
	writeGeoIP(t, "ir", "127.0.0.0/8", "::1/128")
	echo := startEcho(t, "127.0.0.1:0")
	up := startRecordingUpstream(t)
	b := balancerVia(up.addr) // a live endpoint the leak would use

	eng := plugins.NewEngine([]plugins.Rule{rule("geoip", "ir", plugins.DecisionDirect)})
	c := dialRaw(t, startSOCKS5(t, NewServer(0, 0, b, eng, nil)))
	greet(t, c, methodNoAuth)
	mustWrite(t, c, requestMsg(cmdConnect, atypDomain, []byte("localhost"), echo.addr.Port))
	if r := readReply(t, c); r.rep != repSuccess {
		t.Fatalf("reply = %#x, want success", r.rep)
	}
	expectEcho(t, c, []byte("in-country, direct"))

	if got := up.seen(); len(got) != 0 {
		t.Fatalf("LEAK: geoip:ir name was sent into the tunnel: %+v", got)
	}
	if peers := echo.peerList(); len(peers) != 1 {
		t.Errorf("destination saw %d direct connections, want 1", len(peers))
	}
	for _, fl := range b.Flows().Snapshot() {
		t.Errorf("balancer opened a flow for a direct destination: %+v", fl)
	}
}

func TestRouting_HybridResolutionOrder(t *testing.T) {
	// The shipped Iran list, so the IR address below is a real one.
	t.Chdir("../..")
	const (
		irIP    = "185.10.72.10"  // in geoip/ir.txt (185.10.72.0/22)
		outIP   = "93.184.215.14" // not in it
		irName  = "bank.example.test"
		outName = "news.example.test"
		mixName = "cdn.example.test"
	)
	answers := map[string][]netip.Addr{
		irName:  {netip.MustParseAddr(irIP)},
		outName: {netip.MustParseAddr(outIP)},
		mixName: {netip.MustParseAddr(outIP), netip.MustParseAddr(irIP)},
	}
	geoDirect := rule("geoip", "ir", plugins.DecisionDirect)

	cases := []struct {
		name        string
		rules       []plugins.Rule
		host        string
		blocked     bool
		wantDirect  []string // direct dial targets; nil means the tunnel was used
		wantLookups int
	}{
		{name: "geoip ir direct: IR name goes direct, tunnel untouched",
			rules: []plugins.Rule{geoDirect}, host: irName,
			wantDirect: []string{irIP + ":443"}, wantLookups: 1},
		{name: "geoip ir direct: non-IR name is proxied by name",
			rules: []plugins.Rule{geoDirect}, host: outName, wantLookups: 1},
		{name: "mixed answer: only the IR address is dialled direct",
			rules: []plugins.Rule{geoDirect}, host: mixName,
			wantDirect: []string{irIP + ":443"}, wantLookups: 1},
		{name: "a name rule decides first, with no lookup",
			rules: []plugins.Rule{rule("domain_suffix", "example.test", plugins.DecisionProxy), geoDirect},
			host:  irName},
		{name: "rule order holds on the resolved address",
			rules: []plugins.Rule{rule("ip_cidr", "185.10.72.0/22", plugins.DecisionBlock), geoDirect},
			host:  irName, blocked: true, wantLookups: 1},
		{name: "geoip block on a resolved name",
			rules: []plugins.Rule{rule("geoip", "ir", plugins.DecisionBlock)},
			host:  irName, blocked: true, wantLookups: 1},
		{name: "no IP rules: no lookup",
			rules: []plugins.Rule{rule("domain_suffix", "other.test", plugins.DecisionBlock)},
			host:  irName},
		{name: "local lookup fails: default proxy route",
			rules: []plugins.Rule{geoDirect}, host: "unknown.example.test", wantLookups: 1},
		{name: "IP literal is matched as is, no lookup",
			rules: []plugins.Rule{geoDirect}, host: irIP, wantDirect: []string{irIP + ":443"}},
	}

	for _, in := range inbounds {
		for _, tc := range cases {
			t.Run(in.name+"/"+tc.name, func(t *testing.T) {
				up := startRecordingUpstream(t)
				res := &fakeResolver{answers: answers}
				direct := &directRecorder{echo: startEcho(t, "127.0.0.1:0").addr.String()}
				rt := newRouter(balancerVia(up.addr), plugins.NewEngine(tc.rules), nil)
				rt.resolver = res
				rt.dialDirect = direct.dial

				stream, ok := in.connect(t, rt, tc.host, 443)
				if got := res.seen(); len(got) != tc.wantLookups {
					t.Errorf("local lookups = %v, want %d", got, tc.wantLookups)
				}
				if tc.blocked {
					if ok {
						t.Error("request was accepted, want it blocked")
					}
					if len(up.seen())+len(direct.seen()) != 0 {
						t.Errorf("blocked request was dialled: tunnel %+v, direct %v", up.seen(), direct.seen())
					}
					return
				}
				if !ok {
					t.Fatal("request was refused")
				}
				expectEchoRW(t, stream, []byte("payload"))

				if tc.wantDirect != nil {
					if got := up.seen(); len(got) != 0 {
						t.Errorf("LEAK: direct destination reached the tunnel: %+v", got)
					}
					if got := direct.seen(); strings.Join(got, ",") != strings.Join(tc.wantDirect, ",") {
						t.Errorf("direct dials = %v, want %v", got, tc.wantDirect)
					}
					return
				}
				if got := direct.seen(); len(got) != 0 {
					t.Errorf("proxied request was also dialled direct: %v", got)
				}
				// A proxied name stays a name: the local answer only classified it.
				want := upstreamTarget{atyp: atypDomain, host: tc.host, port: 443}
				if _, err := netip.ParseAddr(tc.host); err == nil {
					want.atyp = atypIPv4
				}
				if got := up.seen(); len(got) != 1 || got[0] != want {
					t.Errorf("tunnel saw %+v, want [%+v]", got, want)
				}
			})
		}
	}
}

func expectEchoRW(t *testing.T, rw io.ReadWriter, payload []byte) {
	t.Helper()
	if _, err := rw.Write(payload); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(payload))
	if _, err := io.ReadFull(rw, got); err != nil || string(got) != string(payload) {
		t.Fatalf("echo = %q, %v; want %q", got, err, payload)
	}
}
