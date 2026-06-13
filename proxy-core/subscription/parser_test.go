package subscription

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Per-protocol ParseURI tests
// ---------------------------------------------------------------------------

func TestParseVLESS(t *testing.T) {
	uri := "vless://550e8400-e29b-41d4-a716-446655440000@example.com:443?type=tcp&security=reality&pbk=abc123&sni=example.com&fp=chrome&flow=xtls-rprx-vision#MyVLESS"
	ep, err := ParseURI(uri)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Protocol != "vless" {
		t.Errorf("protocol: got %q want vless", ep.Protocol)
	}
	if ep.Name != "MyVLESS" {
		t.Errorf("name: got %q want MyVLESS", ep.Name)
	}
	if ep.Address != "example.com:443" {
		t.Errorf("address: got %q want example.com:443", ep.Address)
	}
	if ep.Config["uuid"] != "550e8400-e29b-41d4-a716-446655440000" {
		t.Errorf("uuid: got %q", ep.Config["uuid"])
	}
	if ep.Config["pbk"] != "abc123" {
		t.Errorf("pbk: got %q", ep.Config["pbk"])
	}
	if ep.LatencyMs != -1 {
		t.Errorf("LatencyMs should be -1 on new endpoint")
	}
	if ep.Status != "unknown" {
		t.Errorf("Status should be unknown, got %q", ep.Status)
	}
}

func TestParseVMess(t *testing.T) {
	payload := map[string]interface{}{
		"add":  "vmess.example.com",
		"port": 8443,
		"id":   "deadbeef-dead-beef-dead-beefdeadbeef",
		"aid":  0,
		"net":  "ws",
		"type": "none",
		"host": "vmess.example.com",
		"path": "/ws",
		"tls":  "tls",
		"ps":   "MyVMess",
	}
	b, _ := json.Marshal(payload)
	encoded := base64.StdEncoding.EncodeToString(b)
	uri := "vmess://" + encoded

	ep, err := ParseURI(uri)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Protocol != "vmess" {
		t.Errorf("protocol: got %q want vmess", ep.Protocol)
	}
	if ep.Name != "MyVMess" {
		t.Errorf("name: got %q want MyVMess", ep.Name)
	}
	if ep.Config["uuid"] != "deadbeef-dead-beef-dead-beefdeadbeef" {
		t.Errorf("uuid: got %q", ep.Config["uuid"])
	}
	if ep.Config["net"] != "ws" {
		t.Errorf("net: got %q want ws", ep.Config["net"])
	}
}

func TestParseTrojan(t *testing.T) {
	uri := "trojan://s3cr3tpassword@trojan.example.com:443?sni=trojan.example.com&security=tls#MyTrojan"
	ep, err := ParseURI(uri)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Protocol != "trojan" {
		t.Errorf("protocol: got %q want trojan", ep.Protocol)
	}
	if ep.Config["password"] != "s3cr3tpassword" {
		t.Errorf("password: got %q", ep.Config["password"])
	}
	if ep.Config["sni"] != "trojan.example.com" {
		t.Errorf("sni: got %q", ep.Config["sni"])
	}
	if ep.Name != "MyTrojan" {
		t.Errorf("name: got %q want MyTrojan", ep.Name)
	}
}

func TestParseAnyTLS(t *testing.T) {
	uri := "anytls://s3cr3tpassword@anytls.example.com:8445?sni=anytls.example.com&insecure=0#MyAnyTLS"
	ep, err := ParseURI(uri)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Protocol != "anytls" {
		t.Errorf("protocol: got %q want anytls", ep.Protocol)
	}
	if ep.Config["password"] != "s3cr3tpassword" {
		t.Errorf("password: got %q", ep.Config["password"])
	}
	if ep.Config["sni"] != "anytls.example.com" {
		t.Errorf("sni: got %q", ep.Config["sni"])
	}
	if ep.Config["insecure"] != "0" {
		t.Errorf("insecure: got %q want 0", ep.Config["insecure"])
	}
	if ep.Address != "anytls.example.com:8445" {
		t.Errorf("address: got %q want anytls.example.com:8445", ep.Address)
	}
	if ep.Name != "MyAnyTLS" {
		t.Errorf("name: got %q want MyAnyTLS", ep.Name)
	}
	if ep.LatencyMs != -1 {
		t.Errorf("LatencyMs should be -1 on new endpoint")
	}
	if ep.Status != "unknown" {
		t.Errorf("Status should be unknown, got %q", ep.Status)
	}
}

func TestParseAnyTLS_IPv6_AllowInsecure(t *testing.T) {
	// IPv6 host is bracketed; insecure expressed via the allowInsecure alias.
	uri := "anytls://pass@[2001:db8::1]:8445?sni=cdn.example.com&allowInsecure=1#v6"
	ep, err := ParseURI(uri)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Protocol != "anytls" {
		t.Errorf("protocol: got %q want anytls", ep.Protocol)
	}
	if ep.Address != "[2001:db8::1]:8445" {
		t.Errorf("address: got %q want [2001:db8::1]:8445", ep.Address)
	}
	if ep.Config["password"] != "pass" {
		t.Errorf("password: got %q", ep.Config["password"])
	}
	if ep.Config["sni"] != "cdn.example.com" {
		t.Errorf("sni: got %q", ep.Config["sni"])
	}
	if ep.Config["insecure"] != "1" {
		t.Errorf("insecure (via allowInsecure): got %q want 1", ep.Config["insecure"])
	}
	if ep.Name != "v6" {
		t.Errorf("name: got %q want v6", ep.Name)
	}
}

func TestParseSS_SIP002(t *testing.T) {
	// SIP002 format: ss://base64(method:password)@host:port#name
	userinfo := base64.StdEncoding.EncodeToString([]byte("chacha20-ietf-poly1305:mys3cr3t"))
	uri := "ss://" + userinfo + "@ss.example.com:8388#MySS"

	ep, err := ParseURI(uri)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Protocol != "ss" {
		t.Errorf("protocol: got %q want ss", ep.Protocol)
	}
	if ep.Config["method"] != "chacha20-ietf-poly1305" {
		t.Errorf("method: got %q", ep.Config["method"])
	}
	if ep.Config["password"] != "mys3cr3t" {
		t.Errorf("password: got %q", ep.Config["password"])
	}
	if ep.Name != "MySS" {
		t.Errorf("name: got %q want MySS", ep.Name)
	}
}

func TestParseHysteria2(t *testing.T) {
	uri := "hysteria2://myauthtoken@hy2.example.com:443?sni=hy2.example.com&insecure=0#MyHy2"
	ep, err := ParseURI(uri)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Protocol != "hysteria2" {
		t.Errorf("protocol: got %q want hysteria2", ep.Protocol)
	}
	if ep.Config["auth"] != "myauthtoken" {
		t.Errorf("auth: got %q", ep.Config["auth"])
	}
	if ep.Config["sni"] != "hy2.example.com" {
		t.Errorf("sni: got %q", ep.Config["sni"])
	}
}

func TestParseWireGuard(t *testing.T) {
	uri := "wireguard://YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXoA@wg.example.com:51820?pub=pubkeybase64&dns=1.1.1.1#MyWG"
	ep, err := ParseURI(uri)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Protocol != "wireguard" {
		t.Errorf("protocol: got %q want wireguard", ep.Protocol)
	}
	if ep.Config["public_key"] != "pubkeybase64" {
		t.Errorf("public_key: got %q", ep.Config["public_key"])
	}
	if ep.Config["dns"] != "1.1.1.1" {
		t.Errorf("dns: got %q", ep.Config["dns"])
	}
}

func TestParseTUIC(t *testing.T) {
	uri := "tuic://550e8400-e29b-41d4-a716-446655440000:s3cr3t@tuic.example.com:443?sni=tuic.example.com&congestion_control=bbr&alpn=h3#MyTUIC"
	ep, err := ParseURI(uri)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Protocol != "tuic" {
		t.Errorf("protocol: got %q want tuic", ep.Protocol)
	}
	if ep.Config["uuid"] != "550e8400-e29b-41d4-a716-446655440000" {
		t.Errorf("uuid: got %q", ep.Config["uuid"])
	}
	if ep.Config["password"] != "s3cr3t" {
		t.Errorf("password: got %q", ep.Config["password"])
	}
	if ep.Config["congestion"] != "bbr" {
		t.Errorf("congestion: got %q", ep.Config["congestion"])
	}
}

func TestParseURI_UnknownScheme(t *testing.T) {
	_, err := ParseURI("http://example.com")
	if err == nil {
		t.Fatal("expected error for unknown scheme")
	}
}

// ---------------------------------------------------------------------------
// ParseSubscription tests
// ---------------------------------------------------------------------------

func TestParseSubscription_Base64(t *testing.T) {
	lines := []string{
		"vless://550e8400-e29b-41d4-a716-446655440000@v.example.com:443?type=tcp&security=tls#VL1",
		"trojan://pass@t.example.com:443?sni=t.example.com#TR1",
	}
	raw := base64.StdEncoding.EncodeToString([]byte(strings.Join(lines, "\n")))

	eps, err := ParseSubscription(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(eps) != 2 {
		t.Fatalf("expected 2 endpoints, got %d", len(eps))
	}
	if eps[0].Protocol != "vless" {
		t.Errorf("ep[0] protocol: got %q", eps[0].Protocol)
	}
	if eps[1].Protocol != "trojan" {
		t.Errorf("ep[1] protocol: got %q", eps[1].Protocol)
	}
}

func TestParseSubscription_PlainText(t *testing.T) {
	// Non-base64 input — should be treated as raw lines.
	lines := "vless://abc@plain.example.com:443?type=tcp#P1\n# comment\n\n"
	eps, err := ParseSubscription(lines)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(eps) != 1 {
		t.Fatalf("expected 1 endpoint, got %d", len(eps))
	}
}
