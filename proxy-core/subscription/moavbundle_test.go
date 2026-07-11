package subscription

import (
	"strings"
	"testing"
)

func TestParseMoaVBundle_Realistic(t *testing.T) {
	// One server's full surface as documented in docs/MOAV_BUNDLE.md.
	// All credentials are placeholders — no real material.
	uri := "moav://demo-user@1.2.3.4?" +
		"uuid=aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee" +
		"&pw=trojan-and-hy2-pass" +
		"&ss_method=2022-blake3-aes-128-gcm" +
		"&ss_pw=ss-pass-1:ss-pass-2" +
		"&pbk=RealityPubKey" +
		"&sid=deadbeef" +
		"&sni_default=fallback.example.com" +
		"&fp=chrome" +
		"&p=reality,443,sni=update.example.com,flow=xtls-rprx-vision" +
		"&p=vless-ws,443,host=cdn.example.com,path=/vless,sni=cdn.example.com,alpn=http/1.1" +
		"&p=vless-xhttp,2096,sni=miui.example.com" +
		"&p=trojan,8443,sni=tls.example.com" +
		"&p=anytls,8445,sni=tls.example.com" +
		"&p=ss,8388" +
		"&p=hy2,443,sni=tls.example.com,obfs=salamander,obfs_pw=hy2-obfs" +
		"#MoaV-demo"

	eps, err := ParseMoaVBundle(uri)
	if err != nil {
		t.Fatal(err)
	}
	if len(eps) != 7 {
		t.Fatalf("expected 7 endpoints, got %d", len(eps))
	}

	want := []struct{ proto, addr string }{
		{"vless", "1.2.3.4:443"},
		{"vless", "cdn.example.com:443"},
		{"vless", "1.2.3.4:2096"},
		{"trojan", "1.2.3.4:8443"},
		{"anytls", "1.2.3.4:8445"},
		{"ss", "1.2.3.4:8388"},
		{"hysteria2", "1.2.3.4:443"},
	}
	for i, w := range want {
		if eps[i].Protocol != w.proto {
			t.Errorf("ep[%d] protocol: want %s got %s", i, w.proto, eps[i].Protocol)
		}
		if eps[i].Address != w.addr {
			t.Errorf("ep[%d] address: want %s got %s", i, w.addr, eps[i].Address)
		}
	}

	// Reality endpoint should carry the merged Reality keys.
	reality := eps[0]
	if reality.Config["pbk"] != "RealityPubKey" {
		t.Errorf("reality pbk not propagated: %q", reality.Config["pbk"])
	}
	if reality.Config["sid"] != "deadbeef" {
		t.Errorf("reality sid not propagated: %q", reality.Config["sid"])
	}
	if reality.Config["sni"] != "update.example.com" {
		t.Errorf("reality sni override didn't win over sni_default: %q", reality.Config["sni"])
	}
	// CDN endpoint should fall back to sni_default if no explicit sni.
	// (Our test specifies sni=cdn.example.com explicitly so it overrides.)
	if eps[1].Config["sni"] != "cdn.example.com" {
		t.Errorf("vless-ws sni override didn't win: %q", eps[1].Config["sni"])
	}
	// Trojan should pick up the shared pw + per-record sni.
	if eps[3].Config["password"] != "trojan-and-hy2-pass" {
		t.Errorf("trojan password not propagated: %q", eps[3].Config["password"])
	}
	if eps[3].Config["sni"] != "tls.example.com" {
		t.Errorf("trojan sni: %q", eps[3].Config["sni"])
	}
	// AnyTLS should pick up the shared pw + per-record sni (mirrors Trojan).
	if eps[4].Config["password"] != "trojan-and-hy2-pass" {
		t.Errorf("anytls password not propagated: %q", eps[4].Config["password"])
	}
	if eps[4].Config["sni"] != "tls.example.com" {
		t.Errorf("anytls sni: %q", eps[4].Config["sni"])
	}
	// Hy2 obfs.
	if eps[6].Config["obfs"] != "salamander" {
		t.Errorf("hy2 obfs: %q", eps[6].Config["obfs"])
	}
	if eps[6].Config["obfs_password"] != "hy2-obfs" {
		t.Errorf("hy2 obfs_password: %q", eps[6].Config["obfs_password"])
	}
}

func TestParseMoaVBundle_RejectsBadFormat(t *testing.T) {
	cases := []string{
		"not-a-moav-uri",
		"moav://?p=reality,443", // no host
		"moav://x@host?",        // no p=
	}
	for _, c := range cases {
		if _, err := ParseMoaVBundle(c); err == nil {
			t.Errorf("expected error for %q", c)
		}
	}
}

func TestParseSubscription_HandlesMoaVAndLegacyMix(t *testing.T) {
	mixed := strings.Join([]string{
		"vless://aaa@1.1.1.1:443?type=tcp#legacy",
		"moav://x@2.2.2.2?uuid=u&p=ss,8388,&ss_method=aes-256-gcm&ss_pw=p#bundle",
	}, "\n")
	eps, err := ParseSubscription(mixed)
	if err != nil {
		t.Fatal(err)
	}
	if len(eps) != 2 {
		t.Errorf("want 2 endpoints from mixed subscription, got %d", len(eps))
	}
}
