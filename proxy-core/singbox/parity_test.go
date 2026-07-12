package singbox

import (
	"encoding/json"
	"testing"

	"github.com/ibeezhan/moav-client/proxy-core/subscription"
)

// This file is the client-side protocol parity guard: for every protocol the
// MoaV server can hand out and sing-box is meant to speak, it pins the shape of
// the generated outbound. If a mapping regresses (or the sing-box/xray split
// moves), a case here fails. See docs/PROTOCOL-PARITY.md.

func mkEP(proto, addr string, cfg map[string]string) subscription.Endpoint {
	return subscription.Endpoint{ID: "t", Protocol: proto, Address: addr, Enabled: true, Status: "ok", Config: cfg}
}

// str fetches ob[key] as a string (t.Fatal on miss/type).
func str(t *testing.T, ob map[string]any, key string) string {
	t.Helper()
	v, ok := ob[key]
	if !ok {
		t.Fatalf("outbound missing key %q; have %v", key, keys(ob))
	}
	s, ok := v.(string)
	if !ok {
		t.Fatalf("key %q = %v (%T), want string", key, v, v)
	}
	return s
}

func sub(t *testing.T, ob map[string]any, key string) map[string]any {
	t.Helper()
	v, ok := ob[key].(map[string]any)
	if !ok {
		t.Fatalf("key %q not a map: %v", key, ob[key])
	}
	return v
}

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestOutboundPerProtocol(t *testing.T) {
	cases := []struct {
		name     string
		ep       subscription.Endpoint
		wantType string
		check    func(t *testing.T, ob map[string]any)
	}{
		{
			name:     "reality-vless",
			ep:       mkEP("vless", "1.2.3.4:443", map[string]string{"uuid": "U", "security": "reality", "sni": "www.apple.com", "pbk": "PUB", "sid": "ab", "flow": "xtls-rprx-vision"}),
			wantType: "vless",
			check: func(t *testing.T, ob map[string]any) {
				if str(t, ob, "uuid") != "U" || str(t, ob, "flow") != "xtls-rprx-vision" {
					t.Fatalf("vless uuid/flow wrong: %v", ob)
				}
				reality := sub(t, sub(t, ob, "tls"), "reality")
				if reality["enabled"] != true || str(t, reality, "public_key") != "PUB" || str(t, reality, "short_id") != "ab" {
					t.Fatalf("reality block wrong: %v", reality)
				}
			},
		},
		{
			name:     "vless-ws-tls (CDN)",
			ep:       mkEP("vless", "1.2.3.4:443", map[string]string{"uuid": "U", "security": "tls", "sni": "cdn.example.com", "net": "ws", "path": "/ws", "host": "cdn.example.com"}),
			wantType: "vless",
			check: func(t *testing.T, ob map[string]any) {
				tr := sub(t, ob, "transport")
				if str(t, tr, "type") != "ws" || str(t, tr, "path") != "/ws" {
					t.Fatalf("ws transport wrong: %v", tr)
				}
			},
		},
		{
			name:     "trojan",
			ep:       mkEP("trojan", "1.2.3.4:443", map[string]string{"password": "P", "security": "tls", "sni": "s"}),
			wantType: "trojan",
			check: func(t *testing.T, ob map[string]any) {
				if str(t, ob, "password") != "P" {
					t.Fatal("trojan password missing")
				}
				if sub(t, ob, "tls")["enabled"] != true {
					t.Fatal("trojan tls not enabled")
				}
			},
		},
		{
			name:     "anytls",
			ep:       mkEP("anytls", "1.2.3.4:8443", map[string]string{"password": "P", "sni": "s", "insecure": "1"}),
			wantType: "anytls",
			check: func(t *testing.T, ob map[string]any) {
				tls := sub(t, ob, "tls")
				if tls["insecure"] != true {
					t.Fatal("anytls insecure flag not honoured")
				}
				if str(t, sub(t, tls, "utls"), "fingerprint") != "random" {
					t.Fatal("anytls utls fingerprint should be random")
				}
			},
		},
		{
			name:     "shadowsocks-2022",
			ep:       mkEP("ss", "1.2.3.4:8388", map[string]string{"method": "2022-blake3-aes-256-gcm", "password": "b64psk"}),
			wantType: "shadowsocks",
			check: func(t *testing.T, ob map[string]any) {
				// SS-2022 method strings must survive verbatim — the whole point
				// of the client speaking Shadowsocks-2022.
				if str(t, ob, "method") != "2022-blake3-aes-256-gcm" {
					t.Fatalf("SS-2022 method mangled: %q", ob["method"])
				}
				if str(t, ob, "password") != "b64psk" {
					t.Fatal("ss password missing")
				}
			},
		},
		{
			name:     "hysteria2",
			ep:       mkEP("hysteria2", "1.2.3.4:443", map[string]string{"auth": "A", "sni": "s", "obfs": "salamander", "obfs_password": "op"}),
			wantType: "hysteria2",
			check: func(t *testing.T, ob map[string]any) {
				if str(t, ob, "password") != "A" { // hy2 auth maps to password
					t.Fatal("hysteria2 auth->password missing")
				}
				obfs := sub(t, ob, "obfs")
				if str(t, obfs, "type") != "salamander" || str(t, obfs, "password") != "op" {
					t.Fatalf("hysteria2 obfs wrong: %v", obfs)
				}
			},
		},
		{
			name:     "vmess",
			ep:       mkEP("vmess", "1.2.3.4:443", map[string]string{"uuid": "U", "net": "tcp"}),
			wantType: "vmess",
			check: func(t *testing.T, ob map[string]any) {
				if str(t, ob, "security") != "auto" || str(t, ob, "uuid") != "U" {
					t.Fatal("vmess uuid/security wrong")
				}
			},
		},
		{
			name:     "tuic",
			ep:       mkEP("tuic", "1.2.3.4:443", map[string]string{"uuid": "U", "password": "P", "congestion": "bbr", "sni": "s"}),
			wantType: "tuic",
			check: func(t *testing.T, ob map[string]any) {
				if str(t, ob, "congestion_control") != "bbr" {
					t.Fatal("tuic congestion_control missing")
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ob, ok := outboundFromEndpoint(tc.ep)
			if !ok {
				t.Fatalf("outboundFromEndpoint returned ok=false for %s", tc.name)
			}
			if got := str(t, ob, "type"); got != tc.wantType {
				t.Fatalf("type = %q, want %q", got, tc.wantType)
			}
			if str(t, ob, "server") != "1.2.3.4" {
				t.Fatalf("server host not parsed: %v", ob["server"])
			}
			if tc.check != nil {
				tc.check(t, ob)
			}
		})
	}
}

// xhttp / splithttp / raw are Xray-only transports — sing-box must REFUSE them
// so the caller routes them through the xray core instead. This pins the split.
func TestXrayOnlyTransportsRejected(t *testing.T) {
	for _, net := range []string{"xhttp", "splithttp", "raw"} {
		ep := mkEP("vless", "1.2.3.4:443", map[string]string{"uuid": "U", "security": "reality", "sni": "s", "pbk": "K", "net": net})
		if _, ok := outboundFromEndpoint(ep); ok {
			t.Fatalf("sing-box should reject the Xray-only transport %q", net)
		}
	}
}

func TestOutboundRejectsUnknownAndBadAddr(t *testing.T) {
	if _, ok := outboundFromEndpoint(mkEP("nope", "1.2.3.4:443", nil)); ok {
		t.Fatal("unknown protocol should return ok=false")
	}
	if _, ok := outboundFromEndpoint(mkEP("vless", "not-a-host-port", map[string]string{"uuid": "U"})); ok {
		t.Fatal("bad address should return ok=false")
	}
}

func TestTransportFromConfig(t *testing.T) {
	supported := map[string]string{"ws": "ws", "grpc": "grpc", "http": "http", "h2": "http", "httpupgrade": "httpupgrade", "": "", "tcp": ""}
	for net, wantType := range supported {
		tr, ok := transportFromConfig(map[string]string{"net": net, "path": "/p"})
		if !ok {
			t.Fatalf("transport %q should be supported", net)
		}
		if wantType == "" {
			if tr != nil {
				t.Fatalf("transport %q should map to nil (plain tcp), got %v", net, tr)
			}
			continue
		}
		if tr["type"] != wantType {
			t.Fatalf("transport %q -> %v, want type %q", net, tr, wantType)
		}
	}
	for _, net := range []string{"xhttp", "splithttp", "raw", "bogus"} {
		if _, ok := transportFromConfig(map[string]string{"net": net}); ok {
			t.Fatalf("transport %q must be unsupported", net)
		}
	}
}

func TestNormalizeFingerprint(t *testing.T) {
	for in, want := range map[string]string{"": "chrome", "random": "chrome", "firefox": "firefox", "safari": "safari"} {
		if got := normalizeFingerprint(in); got != want {
			t.Fatalf("normalizeFingerprint(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWireGuardNativeEndpoint(t *testing.T) {
	wg := mkEP("wireguard", "1.2.3.4:51820", map[string]string{
		"private_key": "PRIV", "public_key": "PUB", "address": "10.0.0.2/32", "allowed_ips": "0.0.0.0/0", "psk": "PSK",
	})
	jsonBytes, updated, err := Generate([]subscription.Endpoint{wg}, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(jsonBytes, &root); err != nil {
		t.Fatalf("Generate emitted invalid JSON: %v", err)
	}
	// WireGuard goes in the modern endpoints[] block, not outbounds.
	eb, ok := root["endpoints"].([]any)
	if !ok || len(eb) != 1 {
		t.Fatalf("expected one wireguard endpoints[] entry, got %v", root["endpoints"])
	}
	e0 := eb[0].(map[string]any)
	if e0["type"] != "wireguard" || e0["private_key"] != "PRIV" {
		t.Fatalf("wireguard endpoint block wrong: %v", e0)
	}
	// The endpoint gets a local socks5_addr wired back for the balancer to dial.
	if updated[0].Config["socks5_addr"] == "" {
		t.Fatal("wireguard endpoint should get a socks5_addr assigned")
	}

	// A WG endpoint missing keys is skipped (no endpoints block).
	bad := mkEP("wireguard", "1.2.3.4:51820", map[string]string{"address": "10.0.0.2/32"})
	jb, _, err := Generate([]subscription.Endpoint{bad}, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	var root2 map[string]any
	_ = json.Unmarshal(jb, &root2)
	if _, present := root2["endpoints"]; present {
		t.Fatal("keyless wireguard endpoint should be skipped")
	}
}

func TestGenerateShapeAndSidecarSkip(t *testing.T) {
	eps := []subscription.Endpoint{
		mkEP("trojan", "1.2.3.4:443", map[string]string{"password": "P", "security": "tls", "sni": "s"}),
		mkEP("sidecar", "127.0.0.1:5300", map[string]string{}), // must be skipped in sing-box
	}
	jsonBytes, updated, err := Generate(eps, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(jsonBytes, &root); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	outs := root["outbounds"].([]any)
	// exactly one real outbound (trojan) + the always-present direct fallback.
	if len(outs) != 2 {
		t.Fatalf("outbounds = %d, want 2 (trojan + direct)", len(outs))
	}
	last := outs[len(outs)-1].(map[string]any)
	if last["type"] != "direct" || last["tag"] != "out-direct" {
		t.Fatalf("missing direct fallback outbound: %v", last)
	}
	// The trojan endpoint got a socks5_addr; the sidecar did not (skipped).
	if updated[0].Config["socks5_addr"] == "" {
		t.Fatal("trojan endpoint should get socks5_addr")
	}
	if updated[1].Config["socks5_addr"] != "" {
		t.Fatal("sidecar endpoint must be skipped by sing-box (no socks5_addr)")
	}
}
