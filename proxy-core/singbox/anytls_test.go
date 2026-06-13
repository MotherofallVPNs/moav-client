package singbox

import (
	"encoding/json"
	"testing"

	"github.com/ibeezhan/moav-client/proxy-core/subscription"
)

// An AnyTLS endpoint should generate a sing-box `anytls` outbound with a TLS
// block (server_name from SNI, insecure honoured) and utls fingerprint
// randomisation. socks5_addr should be rewritten to the local sing-box port.
func TestAnyTLSOutbound(t *testing.T) {
	ep := subscription.Endpoint{
		Protocol: "anytls",
		Address:  "anytls.example.com:8445",
		Config: map[string]string{
			"password": "p4ss",
			"sni":      "anytls.example.com",
			"insecure": "0",
		},
	}
	cfg := Defaults()
	cfg.DialHost = "127.0.0.1"
	jsonBytes, updated, err := Generate([]subscription.Endpoint{ep}, cfg)
	if err != nil {
		t.Fatal(err)
	}

	var root map[string]any
	if err := json.Unmarshal(jsonBytes, &root); err != nil {
		t.Fatal(err)
	}
	outbounds, _ := root["outbounds"].([]any)
	first, _ := outbounds[0].(map[string]any)

	if first["type"] != "anytls" {
		t.Errorf("type: got %v want anytls", first["type"])
	}
	if first["server"] != "anytls.example.com" {
		t.Errorf("server: got %v", first["server"])
	}
	if int(first["server_port"].(float64)) != 8445 {
		t.Errorf("server_port: got %v want 8445", first["server_port"])
	}
	if first["password"] != "p4ss" {
		t.Errorf("password: got %v", first["password"])
	}
	tls, _ := first["tls"].(map[string]any)
	if tls == nil {
		t.Fatalf("tls block missing: %v", first)
	}
	if tls["enabled"] != true {
		t.Errorf("tls.enabled: got %v want true", tls["enabled"])
	}
	if tls["server_name"] != "anytls.example.com" {
		t.Errorf("tls.server_name: got %v", tls["server_name"])
	}
	if tls["insecure"] != false {
		t.Errorf("tls.insecure: got %v want false", tls["insecure"])
	}
	utls, _ := tls["utls"].(map[string]any)
	if utls == nil || utls["enabled"] != true || utls["fingerprint"] != "random" {
		t.Errorf("utls block wrong: %v", tls["utls"])
	}

	// socks5_addr rewritten to the local sing-box port.
	if got := updated[0].Config["socks5_addr"]; got != "127.0.0.1:10800" {
		t.Errorf("socks5_addr: got %q want 127.0.0.1:10800", got)
	}
}

// insecure=1 must produce tls.insecure == true.
func TestAnyTLSOutbound_Insecure(t *testing.T) {
	ep := subscription.Endpoint{
		Protocol: "anytls",
		Address:  "1.2.3.4:8445",
		Config: map[string]string{
			"password": "p",
			"sni":      "cdn.example.com",
			"insecure": "1",
		},
	}
	jsonBytes, _, err := Generate([]subscription.Endpoint{ep}, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	json.Unmarshal(jsonBytes, &root)
	outbounds, _ := root["outbounds"].([]any)
	first, _ := outbounds[0].(map[string]any)
	tls, _ := first["tls"].(map[string]any)
	if tls["insecure"] != true {
		t.Errorf("tls.insecure: got %v want true", tls["insecure"])
	}
}
