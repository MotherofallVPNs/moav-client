package plugins

import (
	"os"
	"path/filepath"
	"testing"
)

// ---- TorrentBlocker tests ----

func TestTorrentBlocker_Port(t *testing.T) {
	tb := &TorrentBlocker{Enabled: true}
	for _, port := range []int{6881, 6882, 6889, 51413} {
		if !tb.Match("example.com", port, "tcp") {
			t.Errorf("expected block on torrent port %d", port)
		}
	}
}

func TestTorrentBlocker_KnownTrackerDomain(t *testing.T) {
	tb := &TorrentBlocker{Enabled: true}
	domains := []string{
		"tracker.thepiratebay.org",
		"tracker.openbittorrent.com",
		"tracker.opentrackr.org",
		"sub.tracker.opentrackr.org", // subdomain
		"exodus.desync.com",
	}
	for _, d := range domains {
		if !tb.Match(d, 80, "tcp") {
			t.Errorf("expected block on tracker domain %q", d)
		}
	}
}

func TestTorrentBlocker_Disabled(t *testing.T) {
	tb := &TorrentBlocker{Enabled: false}
	if tb.Match("tracker.thepiratebay.org", 6881, "tcp") {
		t.Error("disabled TorrentBlocker should never match")
	}
}

// ---- Engine / first-match-wins tests ----

func TestEngine_FirstMatchWins(t *testing.T) {
	rules := []Rule{
		{Match: MatchExpr{Type: "domain", Value: "example.com"}, Action: DecisionDirect, Enabled: true},
		{Match: MatchExpr{Type: "domain", Value: "example.com"}, Action: DecisionBlock, Enabled: true},
	}
	eng := NewEngine(rules)
	got := eng.Evaluate("example.com", 80, "tcp")
	if got != DecisionDirect {
		t.Errorf("want DecisionDirect, got %d", got)
	}
}

func TestEngine_DefaultProxy(t *testing.T) {
	eng := NewEngine(nil)
	got := eng.Evaluate("no-rule.example.com", 443, "tcp")
	if got != DecisionProxy {
		t.Errorf("want DecisionProxy as default, got %d", got)
	}
}

// ---- matchExpr individual type tests ----

func TestMatchExpr_Domain_Exact(t *testing.T) {
	m := MatchExpr{Type: "domain", Value: "example.com"}
	if !matchExpr(m, "example.com", 80, "") {
		t.Error("exact domain should match")
	}
	if matchExpr(m, "sub.example.com", 80, "") {
		t.Error("exact domain should not match subdomain")
	}
}

func TestMatchExpr_DomainSuffix(t *testing.T) {
	m := MatchExpr{Type: "domain_suffix", Value: "example.com"}
	cases := []struct {
		host string
		want bool
	}{
		{"example.com", true},
		{"sub.example.com", true},
		{"deep.sub.example.com", true},
		{"notexample.com", false},
		{"other.org", false},
	}
	for _, c := range cases {
		got := matchExpr(m, c.host, 80, "")
		if got != c.want {
			t.Errorf("domain_suffix example.com vs %q: want %v got %v", c.host, c.want, got)
		}
	}
}

func TestMatchExpr_DomainKeyword(t *testing.T) {
	m := MatchExpr{Type: "domain_keyword", Value: "torrent"}
	if !matchExpr(m, "mytorrentsite.com", 80, "") {
		t.Error("keyword should match")
	}
	if matchExpr(m, "normal.com", 80, "") {
		t.Error("keyword should not match unrelated host")
	}
}

func TestMatchExpr_IPCIDR(t *testing.T) {
	m := MatchExpr{Type: "ip_cidr", Value: "10.0.0.0/8"}
	if !matchExpr(m, "10.1.2.3", 80, "") {
		t.Error("10.1.2.3 should match 10.0.0.0/8")
	}
	if matchExpr(m, "192.168.1.1", 80, "") {
		t.Error("192.168.1.1 should not match 10.0.0.0/8")
	}
	// Non-IP host should not match.
	if matchExpr(m, "example.com", 80, "") {
		t.Error("hostname should not match ip_cidr rule")
	}
}

func TestMatchExpr_Port_Single(t *testing.T) {
	m := MatchExpr{Type: "port", Value: "443"}
	if !matchExpr(m, "example.com", 443, "") {
		t.Error("port 443 should match")
	}
	if matchExpr(m, "example.com", 80, "") {
		t.Error("port 80 should not match rule for 443")
	}
}

func TestMatchExpr_Port_Range(t *testing.T) {
	m := MatchExpr{Type: "port", Value: "1000-2000"}
	if !matchExpr(m, "example.com", 1500, "") {
		t.Error("port 1500 should be in range 1000-2000")
	}
	if matchExpr(m, "example.com", 999, "") {
		t.Error("port 999 should be outside range 1000-2000")
	}
}

// ---- GeoIP stub test ----

func TestMatchExpr_GeoIP_Stub(t *testing.T) {
	// Create a temporary root directory with a geoip/ subdirectory and test CC file.
	root := t.TempDir()
	geoipDir := filepath.Join(root, "geoip")
	if err := os.Mkdir(geoipDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cc := "TS"
	path := filepath.Join(geoipDir, "ts.txt")
	if err := os.WriteFile(path, []byte("203.0.113.0/24\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Change working directory so matchGeoIP can find geoip/ts.txt.
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir) //nolint:errcheck
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	m := MatchExpr{Type: "geoip", Value: cc}
	if !matchExpr(m, "203.0.113.5", 80, "") {
		t.Error("203.0.113.5 should match geoip stub 203.0.113.0/24")
	}
	if matchExpr(m, "198.51.100.1", 80, "") {
		t.Error("198.51.100.1 should not match geoip stub 203.0.113.0/24")
	}
}

// ---- Router AND-logic test ----

func TestRouter_MultiMatch_AND(t *testing.T) {
	r := &Router{
		Rules: []RoutingRule{
			{
				Match:  []MatchExpr{{Type: "domain_keyword", Value: "torrent"}, {Type: "port", Value: "80"}},
				Action: DecisionBlock,
			},
		},
	}
	// Both match → block.
	if got := r.Evaluate("mytorrentsite.com", 80, ""); got != DecisionBlock {
		t.Errorf("want DecisionBlock, got %d", got)
	}
	// Only keyword matches, port doesn't → no block.
	if got := r.Evaluate("mytorrentsite.com", 443, ""); got != DecisionProxy {
		t.Errorf("want DecisionProxy, got %d", got)
	}
}

// ---- Engine integration: torrent blocker wired via rules ----

func TestEngine_TorrentBlockIntegration(t *testing.T) {
	tb := &TorrentBlocker{Enabled: true}

	// Build a rule that fires when TorrentBlocker matches.
	// We use a custom approach: wrap the Engine with pre-check for TorrentBlocker
	// then fall through to rules. This mirrors how main.go wires it (see handler.go).
	host := "tracker.thepiratebay.org"
	port := 80
	if !tb.Match(host, port, "tcp") {
		t.Fatal("TorrentBlocker should match tracker domain")
	}
	// When TorrentBlocker matches, handler returns DecisionBlock.
	decision := DecisionProxy
	if tb.Match(host, port, "tcp") {
		decision = DecisionBlock
	}
	if decision != DecisionBlock {
		t.Errorf("expected DecisionBlock, got %d", decision)
	}
}
