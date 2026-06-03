package xray

import (
	"encoding/json"
	"testing"

	"github.com/ibeezhan/moav-client/proxy-core/subscription"
)

// xray's vless outbound carries the upstream in settings.vnext[0].
// When spoof_via is set, that's the slot to rewrite.
func TestApplySpoof_RewritesVnext_VLESS_XHTTP(t *testing.T) {
	ep := subscription.Endpoint{
		Protocol: "vless",
		Address:  "1.2.3.4:2096",
		Config: map[string]string{
			"uuid":      "u",
			"net":       "xhttp",
			"security":  "tls", // not reality — applySpoof must still rewrite
			"sni":       "real.example.com",
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
	settings, _ := first["settings"].(map[string]any)
	vnext, _ := settings["vnext"].([]any)
	v0, _ := vnext[0].(map[string]any)
	if v0["address"] != "sni-spoof" {
		t.Errorf("vnext[0].address should be 'sni-spoof', got %v", v0["address"])
	}
	if int(v0["port"].(float64)) != 13000 {
		t.Errorf("vnext[0].port should be 13000, got %v", v0["port"])
	}
}

func TestApplySpoof_SkipsReality(t *testing.T) {
	ep := subscription.Endpoint{
		Protocol: "vless",
		Address:  "1.2.3.4:2096",
		Config: map[string]string{
			"uuid":      "u",
			"net":       "xhttp",
			"security":  "reality",
			"sni":       "miui.example.com",
			"pbk":       "x",
			"sid":       "y",
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
	settings, _ := first["settings"].(map[string]any)
	vnext, _ := settings["vnext"].([]any)
	v0, _ := vnext[0].(map[string]any)
	if v0["address"] == "sni-spoof" {
		t.Error("Reality endpoint must NOT route via the spoofer")
	}
	if v0["address"] != "1.2.3.4" {
		t.Errorf("vnext[0].address should be 1.2.3.4, got %v", v0["address"])
	}
}
