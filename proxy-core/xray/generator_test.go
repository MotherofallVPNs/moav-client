package xray

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ibeezhan/moav-client/proxy-core/subscription"
)

func TestHandlesEndpoint(t *testing.T) {
	cases := []struct {
		ep   subscription.Endpoint
		want bool
		name string
	}{
		{
			subscription.Endpoint{Protocol: "vless", Config: map[string]string{"net": "xhttp"}},
			true,
			"vless xhttp",
		},
		{
			subscription.Endpoint{Protocol: "vless", Config: map[string]string{"net": "splithttp"}},
			true,
			"vless splithttp",
		},
		{
			subscription.Endpoint{Protocol: "vless", Config: map[string]string{"net": "tcp"}},
			false,
			"vless tcp (sing-box handles)",
		},
		{
			subscription.Endpoint{Protocol: "trojan", Config: map[string]string{"type": "tcp"}},
			false,
			"trojan (sing-box handles)",
		},
		{
			subscription.Endpoint{Protocol: "sidecar"},
			false,
			"sidecar",
		},
	}
	for _, c := range cases {
		got := HandlesEndpoint(c.ep)
		if got != c.want {
			t.Errorf("%s: want %v, got %v", c.name, c.want, got)
		}
	}
}

func TestGenerate_VLESS_XHTTP_Reality(t *testing.T) {
	eps := []subscription.Endpoint{
		{
			Protocol: "vless",
			Address:  "1.2.3.4:2096",
			Config: map[string]string{
				"uuid":     "deadbeef-0000-0000-0000-000000000000",
				"net":      "xhttp",
				"security": "reality",
				"sni":      "example.com",
				"pbk":      "RealityPubKeyTest",
				"sid":      "deadbeef",
				"fp":       "chrome",
			},
		},
		// Should NOT be handled (tcp — sing-box's job).
		{
			Protocol: "vless",
			Address:  "5.6.7.8:443",
			Config: map[string]string{
				"uuid": "f00", "net": "tcp", "security": "reality", "sni": "example.com",
			},
		},
	}
	cfg := Defaults()
	cfg.DialHost = "127.0.0.1"
	jsonBytes, updated, err := Generate(eps, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if jsonBytes == nil {
		t.Fatal("expected JSON for the xhttp endpoint")
	}
	if updated[0].Config["socks5_addr"] != "127.0.0.1:11800" {
		t.Errorf("first endpoint should be pinned to 127.0.0.1:11800, got %q", updated[0].Config["socks5_addr"])
	}
	if updated[1].Config["socks5_addr"] == "127.0.0.1:11800" {
		t.Errorf("tcp endpoint should NOT be pinned to xray")
	}

	// Verify structure: one inbound, one outbound (+ direct fallback), one route rule.
	var root map[string]any
	if err := json.Unmarshal(jsonBytes, &root); err != nil {
		t.Fatal(err)
	}
	inb, _ := root["inbounds"].([]any)
	if len(inb) != 1 {
		t.Errorf("want 1 inbound, got %d", len(inb))
	}
	out, _ := root["outbounds"].([]any)
	if len(out) != 2 { // ep + direct fallback
		t.Errorf("want 2 outbounds, got %d", len(out))
	}
	routing, _ := root["routing"].(map[string]any)
	rules, _ := routing["rules"].([]any)
	if len(rules) != 1 {
		t.Errorf("want 1 route rule, got %d", len(rules))
	}

	// Reality settings should be in the outbound.
	body := string(jsonBytes)
	for _, expect := range []string{
		`"network": "xhttp"`,
		`"security": "reality"`,
		`"publicKey": "RealityPubKeyTest"`,
		`"shortId": "deadbeef"`,
	} {
		if !strings.Contains(body, expect) {
			t.Errorf("expected %q in generated config; not found", expect)
		}
	}
}

func TestGenerate_NoXrayOnlyEndpoints_ReturnsNilJSON(t *testing.T) {
	eps := []subscription.Endpoint{
		{Protocol: "vless", Address: "1.2.3.4:443", Config: map[string]string{"uuid": "x", "net": "tcp"}},
		{Protocol: "trojan", Address: "1.2.3.4:443", Config: map[string]string{"password": "p", "type": "tcp"}},
	}
	jsonBytes, _, err := Generate(eps, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if jsonBytes != nil {
		t.Errorf("expected nil JSON when no xray-handled endpoints; got %d bytes", len(jsonBytes))
	}
}
