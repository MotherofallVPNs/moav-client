package singbox

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ibeezhan/moav-client/proxy-core/subscription"
)

// When an endpoint has spoof_via set (non-Reality), the sing-box outbound's
// server / server_port should point at the spoofer, not the real upstream.
func TestApplySpoof_RewritesServer_TrojanTLS(t *testing.T) {
	ep := subscription.Endpoint{
		Protocol: "trojan",
		Address:  "1.2.3.4:8443",
		Config: map[string]string{
			"password":  "p",
			"sni":       "real.example.com",
			"security":  "tls",
			"spoof_via": "sni-spoof:13000",
		},
	}
	cfg := Defaults()
	cfg.DialHost = "127.0.0.1"
	jsonBytes, _, err := Generate([]subscription.Endpoint{ep}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	json.Unmarshal(jsonBytes, &root)
	outbounds, _ := root["outbounds"].([]any)
	first, _ := outbounds[0].(map[string]any)
	if first["server"] != "sni-spoof" {
		t.Errorf("server should be 'sni-spoof', got %v", first["server"])
	}
	if int(first["server_port"].(float64)) != 13000 {
		t.Errorf("server_port should be 13000, got %v", first["server_port"])
	}
	// Sanity: the TLS server_name (the REAL SNI) is preserved.
	body := string(jsonBytes)
	if !strings.Contains(body, `"server_name": "real.example.com"`) {
		t.Errorf("real SNI not preserved on the TLS block: %s", body)
	}
}

// Reality must NOT be spoofed — the spoofer's fake CH breaks Reality auth.
func TestApplySpoof_SkipsReality(t *testing.T) {
	ep := subscription.Endpoint{
		Protocol: "vless",
		Address:  "1.2.3.4:443",
		Config: map[string]string{
			"uuid":      "x",
			"flow":      "xtls-rprx-vision",
			"security":  "reality",
			"pbk":       "pbk",
			"sid":       "sid",
			"sni":       "update.example.com",
			"spoof_via": "sni-spoof:13000",
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
	if first["server"] == "sni-spoof" {
		t.Error("Reality endpoint must NOT route via the spoofer (would break auth)")
	}
	if first["server"] != "1.2.3.4" {
		t.Errorf("server should be 1.2.3.4, got %v", first["server"])
	}
}
