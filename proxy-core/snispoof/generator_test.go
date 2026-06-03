package snispoof

import (
	"encoding/json"
	"testing"

	"github.com/ibeezhan/moav-client/proxy-core/subscription"
)

func TestHandlesEndpoint(t *testing.T) {
	cases := []struct {
		ep   subscription.Endpoint
		want bool
	}{
		{subscription.Endpoint{Config: map[string]string{"fake_sni": "hcaptcha.com"}}, true},
		{subscription.Endpoint{Config: map[string]string{"fake_sni": ""}}, false},
		{subscription.Endpoint{Config: nil}, false},
		{subscription.Endpoint{Config: map[string]string{"other": "x"}}, false},
	}
	for i, c := range cases {
		if got := HandlesEndpoint(c.ep); got != c.want {
			t.Errorf("[%d] want %v got %v for %+v", i, c.want, got, c.ep.Config)
		}
	}
}

func TestGenerate_AllocatesPortsAndPinsSpoofVia(t *testing.T) {
	eps := []subscription.Endpoint{
		{ // 0: spoofed
			ID:       "trojan:1.2.3.4:8443",
			Protocol: "trojan",
			Address:  "1.2.3.4:8443",
			Config:   map[string]string{"fake_sni": "hcaptcha.com", "password": "p"},
		},
		{ // 1: not spoofed
			ID:       "ss:1.2.3.4:8388",
			Protocol: "ss",
			Address:  "1.2.3.4:8388",
			Config:   map[string]string{"method": "aes-256-gcm"},
		},
		{ // 2: spoofed
			ID:       "vless:cdn.example.com:443",
			Protocol: "vless",
			Address:  "cdn.example.com:443",
			Config:   map[string]string{"fake_sni": "windowsupdate.com", "utls": "firefox"},
		},
	}

	cfg := Defaults()
	cfg.DialHost = "127.0.0.1"
	cfg.BasePort = 13000
	jsonBytes, updated, err := Generate(eps, cfg)
	if err != nil {
		t.Fatal(err)
	}

	// Two endpoints should get spoof_via pinned.
	if updated[0].Config["spoof_via"] != "127.0.0.1:13000" {
		t.Errorf("ep[0] spoof_via=%q want 127.0.0.1:13000", updated[0].Config["spoof_via"])
	}
	if _, set := updated[1].Config["spoof_via"]; set {
		t.Errorf("ep[1] should not have spoof_via, got %q", updated[1].Config["spoof_via"])
	}
	if updated[2].Config["spoof_via"] != "127.0.0.1:13001" {
		t.Errorf("ep[2] spoof_via=%q want 127.0.0.1:13001", updated[2].Config["spoof_via"])
	}

	var maps []Mapping
	if err := json.Unmarshal(jsonBytes, &maps); err != nil {
		t.Fatal(err)
	}
	if len(maps) != 2 {
		t.Fatalf("want 2 mappings, got %d", len(maps))
	}
	if maps[0].Listen != ":13000" || maps[0].Connect != "1.2.3.4:8443" || maps[0].FakeSNI != "hcaptcha.com" || maps[0].UTLS != "chrome" {
		t.Errorf("mappings[0] wrong: %+v", maps[0])
	}
	if maps[1].UTLS != "firefox" {
		t.Errorf("mappings[1] utls=%q want firefox", maps[1].UTLS)
	}
}

func TestGenerate_EmptyWhenNoFakeSNI(t *testing.T) {
	eps := []subscription.Endpoint{
		{ID: "x", Address: "1.2.3.4:443", Config: map[string]string{"password": "p"}},
	}
	jsonBytes, _, err := Generate(eps, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if jsonBytes != nil {
		t.Errorf("expected nil JSON when no endpoints carry fake_sni; got %d bytes", len(jsonBytes))
	}
}
