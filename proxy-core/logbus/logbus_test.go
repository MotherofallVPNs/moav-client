package logbus

import "testing"

func TestClassifyLevel(t *testing.T) {
	cases := []struct {
		line string
		want string
	}{
		// Probe results — always info, regardless of per-endpoint status.
		{"probe vless:178.105.237.120:443 via singbox:10800: status=error latency=598ms", "info"},
		{"probe wireguard:178.105.237.120:51820 via singbox:10805: status=ok latency=247ms", "info"},
		{"probe ss:178.105.237.120:8388 via singbox:10803: status=timeout latency=10000ms", "info"},
		{"initial probe complete: 11 endpoints updated", "info"},

		// Warns — recoverable, operator-relevant.
		{"balancer: dial through vless:178.105.237.120:443 failed (EOF); trying next endpoint", "warn"},
		{"balancer: dial 1.1.1.1:443 via hysteria2:178.105.237.120:443 succeeded after 1 failover(s)", "warn"},
		{"balancer: no healthy endpoint, dialing api.example.com directly (falling back)", "warn"},

		// System errors.
		{"fatal: load config: open /missing.yaml: no such file or directory", "error"},
		{"http listen: bind tcp 0.0.0.0:1080: address already in use", "error"},

		// Degraded but still serving — warn, not error.
		{"balancer: all candidates failed, dialing api.example.com directly", "warn"},

		// Plain info.
		{"moav-client starting — SOCKS5 :1080  HTTP :8080  API :8088", "info"},
		{"subscription: loaded 6 endpoints from ./data/beezhan-t7d/subscription.txt", "info"},
		{"plugins: replaced rule list (3 rules) via API", "info"},
	}

	for _, c := range cases {
		got := classifyLevel(c.line)
		if got != c.want {
			t.Errorf("classifyLevel(%q):\n  want %s\n   got %s", c.line, c.want, got)
		}
	}
}

func TestStripDatePrefix(t *testing.T) {
	in := "2026/06/03 00:51:07 balancer: dial through X failed"
	want := "balancer: dial through X failed"
	if got := stripDatePrefix(in); got != want {
		t.Errorf("stripDatePrefix: got %q want %q", got, want)
	}
}
