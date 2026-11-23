package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// These tests pin how Load decodes config.yaml so a YAML library swap or
// upgrade that changes decoding semantics fails here first.

func loadString(t *testing.T, body string) (*Config, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return Load(path)
}

func mustLoad(t *testing.T, body string) *Config {
	t.Helper()
	cfg, err := loadString(t, body)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return cfg
}

func boolPtr(b bool) *bool { return &b }

func TestLoad_ExampleConfigMatchesDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join("..", "..", "config.yaml.example"))
	if err != nil {
		t.Fatalf("config.yaml.example must parse: %v", err)
	}

	want := Defaults()
	want.Subscription.WireGuardFiles = []string{}
	want.Plugins.RoutingRules = []RoutingRuleConfig{
		{Match: MatchExprConfig{Type: "geoip", Value: "ir"}, Action: "direct"},
	}
	want.Sidecars = SidecarsConfig{
		MasterDNS:   SidecarEntry{Priority: 1, Config: map[string]string{"domain": "", "method": "5", "key": ""}},
		AmneziaWG:   SidecarEntry{Priority: 5, Config: map[string]string{"source_path": ""}},
		Psiphon:     SidecarEntry{Priority: 5, Config: map[string]string{"config_json": ""}},
		TrustTunnel: SidecarEntry{Priority: 5, Config: map[string]string{"source_path": ""}},
		Tor:         SidecarEntry{Priority: 5},
	}
	if !reflect.DeepEqual(cfg, want) {
		t.Errorf("example config decoded unexpectedly\n got: %+v\nwant: %+v", cfg, want)
	}
}

func TestLoad_FullDocument(t *testing.T) {
	cfg := mustLoad(t, `
proxy:
  socks5_port: 2080
  http_port: 2081
  api_port: 2082
  exposure: lan
  auth:
    username: alice
    password: "p@ss: word #1"
subscription:
  url: "https://example.com/sub?token=abc&x=1"
  file: ./sub.txt
  wireguard_files:
    - a.conf
    - "b c.conf"
  sources:
    - name: alpha
      url: https://alpha.example/sub
    - name: "بتا"
      file: ./beta.txt
      wireguard_files: [beta-wg.conf]
load_balancing:
  strategy: weighted
  probe_on_start: false
plugins:
  torrent_block: true
  block_direct: true
  routing_rules:
    - match: {type: geoip, value: ir}
      action: direct
    - match:
        type: port
        value: "1000-2000"
      action: block
      enabled: false
      note: |
        two
        lines
    - match:
        type: port
        value: 443
      action: proxy
      enabled: true
sidecars:
  masterdns:
    enabled: true
    priority: 2
    config:
      domain: m.example.com
      method: 5
      key: 00ff
      verbose: true
  psiphon:
    enabled: true
    config:
      config_json: |
        {
          "PropagationChannelId": "ABC"
        }
singbox:
  enabled: false
  listen_host: 127.0.0.1
  dial_host: 127.0.0.1
  base_port: 20800
  output_path: /tmp/sb.json
xray:
  enabled: false
  base_port: 21800
sni_spoof:
  enabled: true
  default_fake_sni: fake.example
  default_utls: firefox
`)

	if got, want := cfg.Proxy, (ProxyConfig{
		SOCKS5Port: 2080, HTTPPort: 2081, APIPort: 2082, Exposure: "lan",
		Auth: ProxyAuthConfig{Username: "alice", Password: "p@ss: word #1"},
	}); got != want {
		t.Errorf("proxy = %+v, want %+v", got, want)
	}

	wantSub := SubscriptionConfig{
		URL:            "https://example.com/sub?token=abc&x=1",
		File:           "./sub.txt",
		WireGuardFiles: []string{"a.conf", "b c.conf"},
		Sources: []SourceConfig{
			{Name: "alpha", URL: "https://alpha.example/sub"},
			{Name: "بتا", File: "./beta.txt", WireGuardFiles: []string{"beta-wg.conf"}},
		},
	}
	if !reflect.DeepEqual(cfg.Subscription, wantSub) {
		t.Errorf("subscription = %+v, want %+v", cfg.Subscription, wantSub)
	}

	if cfg.LoadBalancing != (LoadBalancingConfig{Strategy: "weighted", ProbeOnStart: false}) {
		t.Errorf("load_balancing = %+v", cfg.LoadBalancing)
	}

	wantRules := []RoutingRuleConfig{
		{Match: MatchExprConfig{Type: "geoip", Value: "ir"}, Action: "direct"},
		{Match: MatchExprConfig{Type: "port", Value: "1000-2000"}, Action: "block", Enabled: boolPtr(false), Note: "two\nlines\n"},
		// An unquoted number decodes into the string field as its literal text.
		{Match: MatchExprConfig{Type: "port", Value: "443"}, Action: "proxy", Enabled: boolPtr(true)},
	}
	if !cfg.Plugins.TorrentBlock || !cfg.Plugins.BlockDirect {
		t.Errorf("plugins flags not loaded: %+v", cfg.Plugins)
	}
	if !reflect.DeepEqual(cfg.Plugins.RoutingRules, wantRules) {
		t.Errorf("routing_rules = %+v, want %+v", cfg.Plugins.RoutingRules, wantRules)
	}

	// Unquoted ints and bools in the free-form map decode to their literal text.
	wantMD := SidecarEntry{Enabled: true, Priority: 2, Config: map[string]string{
		"domain": "m.example.com", "method": "5", "key": "00ff", "verbose": "true",
	}}
	if !reflect.DeepEqual(cfg.Sidecars.MasterDNS, wantMD) {
		t.Errorf("masterdns = %+v, want %+v", cfg.Sidecars.MasterDNS, wantMD)
	}
	if got, want := cfg.Sidecars.Psiphon.Config["config_json"], "{\n  \"PropagationChannelId\": \"ABC\"\n}\n"; got != want {
		t.Errorf("psiphon config_json block scalar = %q, want %q", got, want)
	}

	if got, want := cfg.Singbox, (SingboxConfig{ListenHost: "127.0.0.1", DialHost: "127.0.0.1", BasePort: 20800, OutputPath: "/tmp/sb.json"}); got != want {
		t.Errorf("singbox = %+v, want %+v", got, want)
	}
	// Keys absent from a present section keep their defaults.
	if got, want := cfg.Xray, (XrayConfig{Enabled: false, ListenHost: "0.0.0.0", DialHost: "xray", BasePort: 21800, OutputPath: "data/xray.json"}); got != want {
		t.Errorf("xray = %+v, want %+v", got, want)
	}
	if got, want := cfg.SNISpoof, (SNISpoofConfig{
		Enabled: true, ListenHost: "0.0.0.0", DialHost: "sni-spoof", BasePort: 12800,
		OutputPath: "data/sni-spoof.json", DefaultFakeSNI: "fake.example", DefaultUTLS: "firefox",
	}); got != want {
		t.Errorf("sni_spoof = %+v, want %+v", got, want)
	}
}

func TestLoad_EmptyAndCommentOnlyFilesYieldDefaults(t *testing.T) {
	for name, body := range map[string]string{
		"empty":        "",
		"whitespace":   "\n\n  \n",
		"comment-only": "# nothing configured yet\n",
		"doc-marker":   "---\n",
		"null-doc":     "~\n",
	} {
		t.Run(name, func(t *testing.T) {
			cfg := mustLoad(t, body)
			if !reflect.DeepEqual(cfg, Defaults()) {
				t.Errorf("got %+v, want defaults", cfg)
			}
		})
	}
}

func TestLoad_ExplicitZeroAndEmptyOverrideDefaults(t *testing.T) {
	cfg := mustLoad(t, `
proxy:
  socks5_port: 0
load_balancing:
  strategy: ""
  probe_on_start: false
singbox:
  enabled: false
`)
	if cfg.Proxy.SOCKS5Port != 0 {
		t.Errorf("explicit socks5_port: 0 should win over the default, got %d", cfg.Proxy.SOCKS5Port)
	}
	if cfg.Proxy.HTTPPort != 8080 || cfg.Proxy.APIPort != 8088 {
		t.Errorf("sibling defaults lost: %+v", cfg.Proxy)
	}
	if cfg.LoadBalancing.Strategy != "" || cfg.LoadBalancing.ProbeOnStart {
		t.Errorf("explicit empty/false should win: %+v", cfg.LoadBalancing)
	}
	if cfg.Singbox.Enabled || cfg.Singbox.BasePort != 10800 {
		t.Errorf("singbox = %+v", cfg.Singbox)
	}
}

func TestLoad_NumericCoercion(t *testing.T) {
	cfg := mustLoad(t, "proxy:\n  socks5_port: 2080.9\n  http_port: 0x1F90\n  api_port: 8_088\nsingbox:\n  base_port: 0o17\nxray:\n  base_port: 017\n")
	// A float is truncated into an int field rather than rejected.
	if cfg.Proxy.SOCKS5Port != 2080 {
		t.Errorf("float into int: got %d, want 2080", cfg.Proxy.SOCKS5Port)
	}
	if cfg.Proxy.HTTPPort != 0x1F90 || cfg.Proxy.APIPort != 8088 {
		t.Errorf("hex / underscore ints: %+v", cfg.Proxy)
	}
	// Both YAML 1.2 (0o17) and YAML 1.1 (017) octal spellings decode as octal.
	if cfg.Singbox.BasePort != 15 || cfg.Xray.BasePort != 15 {
		t.Errorf("octal ints: singbox=%d xray=%d, want 15", cfg.Singbox.BasePort, cfg.Xray.BasePort)
	}
}

func TestLoad_NullSectionKeepsDefaults(t *testing.T) {
	cfg := mustLoad(t, "proxy:\nsingbox: ~\nload_balancing: null\n")
	if !reflect.DeepEqual(cfg, Defaults()) {
		t.Errorf("null sections should leave defaults untouched, got %+v", cfg)
	}
}

func TestLoad_YAML11BooleansIntoTypedBool(t *testing.T) {
	cfg := mustLoad(t, `
plugins:
  torrent_block: yes
  block_direct: on
  routing_rules:
    - match: {type: domain, value: a.example}
      action: block
      enabled: no
load_balancing:
  probe_on_start: off
sidecars:
  tor:
    enabled: Yes
  psiphon:
    enabled: TRUE
`)
	if !cfg.Plugins.TorrentBlock || !cfg.Plugins.BlockDirect {
		t.Errorf("yes/on should decode to true: %+v", cfg.Plugins)
	}
	if cfg.LoadBalancing.ProbeOnStart {
		t.Error("off should decode to false")
	}
	if e := cfg.Plugins.RoutingRules[0].Enabled; e == nil || *e {
		t.Errorf("enabled: no should decode to a false pointer, got %v", e)
	}
	if !cfg.Sidecars.Tor.Enabled || !cfg.Sidecars.Psiphon.Enabled {
		t.Errorf("Yes/TRUE should decode to true: %+v", cfg.Sidecars)
	}
}

func TestLoad_YAML11BoolWordsStayStringsInStringFields(t *testing.T) {
	cfg := mustLoad(t, `
proxy:
  auth:
    username: yes
    password: off
sidecars:
  masterdns:
    config:
      key: no
      domain: null
      method: 0x1F
`)
	if cfg.Proxy.Auth.Username != "yes" || cfg.Proxy.Auth.Password != "off" {
		t.Errorf("bool-looking words must stay strings: %+v", cfg.Proxy.Auth)
	}
	want := map[string]string{"key": "no", "domain": "", "method": "0x1F"}
	if got := cfg.Sidecars.MasterDNS.Config; !reflect.DeepEqual(got, want) {
		t.Errorf("config map = %#v, want %#v", got, want)
	}
}

func TestLoad_UnknownKeysAreIgnored(t *testing.T) {
	cfg := mustLoad(t, `
future_section:
  anything: [1, 2, 3]
proxy:
  socks5_port: 3080
  not_a_field: true
sidecars:
  slipstream:
    enabled: true
`)
	if cfg.Proxy.SOCKS5Port != 3080 {
		t.Errorf("known key next to unknown one not loaded: %+v", cfg.Proxy)
	}
}

func TestLoad_AnchorsAliasesAndMergeKeys(t *testing.T) {
	cfg := mustLoad(t, `
x-base: &base
  enabled: true
  priority: 7
  config:
    source: shared
sidecars:
  tor:
    <<: *base
  psiphon:
    <<: *base
    priority: 3
  masterdns:
    <<: [*base]
    enabled: false
subscription:
  wireguard_files: &wg [a.conf, b.conf]
  sources:
    - name: s1
      wireguard_files: *wg
`)
	shared := map[string]string{"source": "shared"}
	if got, want := cfg.Sidecars.Tor, (SidecarEntry{Enabled: true, Priority: 7, Config: shared}); !reflect.DeepEqual(got, want) {
		t.Errorf("tor = %+v, want %+v", got, want)
	}
	if got, want := cfg.Sidecars.Psiphon, (SidecarEntry{Enabled: true, Priority: 3, Config: shared}); !reflect.DeepEqual(got, want) {
		t.Errorf("psiphon (explicit key beats merge) = %+v, want %+v", got, want)
	}
	if got, want := cfg.Sidecars.MasterDNS, (SidecarEntry{Enabled: false, Priority: 7, Config: shared}); !reflect.DeepEqual(got, want) {
		t.Errorf("masterdns (merge list) = %+v, want %+v", got, want)
	}
	if got, want := cfg.Subscription.Sources[0].WireGuardFiles, []string{"a.conf", "b.conf"}; !reflect.DeepEqual(got, want) {
		t.Errorf("alias sequence = %v, want %v", got, want)
	}
}

func TestLoad_QuotingAndScalarStyles(t *testing.T) {
	cfg := mustLoad(t, "subscription:\n"+
		"  url: 'it''s'\n"+
		"  file: \"tab\\there \\u00e9\"\n"+
		"  wireguard_files: [ plain, \"double\", 'single', 0777, 1e3, 2026-11-23 ]\n"+
		"sidecars:\n"+
		"  psiphon:\n"+
		"    config:\n"+
		"      folded: >-\n"+
		"        joined\n"+
		"        line\n"+
		"      keep: |+\n"+
		"        kept\n"+
		"\n"+
		"      strip: |-\n"+
		"        stripped\n"+
		"      crlf: \"a\\r\\nb\"\n")
	if cfg.Subscription.URL != "it's" || cfg.Subscription.File != "tab\there é" {
		t.Errorf("quoted scalars: url=%q file=%q", cfg.Subscription.URL, cfg.Subscription.File)
	}
	if got, want := cfg.Subscription.WireGuardFiles, []string{"plain", "double", "single", "0777", "1e3", "2026-11-23"}; !reflect.DeepEqual(got, want) {
		t.Errorf("flow sequence into []string = %q, want %q", got, want)
	}
	want := map[string]string{"folded": "joined line", "keep": "kept\n\n", "strip": "stripped", "crlf": "a\r\nb"}
	if got := cfg.Sidecars.Psiphon.Config; !reflect.DeepEqual(got, want) {
		t.Errorf("block scalars = %#v, want %#v", got, want)
	}
}

func TestLoad_CRLFAndBOM(t *testing.T) {
	cfg := mustLoad(t, "\ufeffproxy:\r\n  socks5_port: 4080\r\n  auth:\r\n    password: secret\r\n")
	if cfg.Proxy.SOCKS5Port != 4080 || cfg.Proxy.Auth.Password != "secret" {
		t.Errorf("CRLF + BOM file not decoded: %+v", cfg.Proxy)
	}
}

func TestLoad_Errors(t *testing.T) {
	cases := []struct {
		name string
		body string
		want []string // substrings that must all appear in the error
	}{
		{"unterminated flow sequence", "proxy:\n  socks5_port: [1, 2\n", []string{"config: parse", "yaml:", "line "}},
		{"tab indentation", "proxy:\n\tsocks5_port: 1080\n", []string{"config: parse", "yaml:", "line 2"}},
		{"string into int", "proxy:\n  socks5_port: abc\n", []string{"config: parse", "line 2", "cannot unmarshal !!str `abc` into int"}},
		{"string into bool", "singbox:\n  enabled: maybe\n", []string{"line 2", "cannot unmarshal !!str `maybe` into bool"}},
		{"mapping into string", "proxy:\n  auth:\n    username: {a: b}\n", []string{"line 3", "cannot unmarshal !!map into string"}},
		{"scalar into sequence", "subscription:\n  wireguard_files: one.conf\n", []string{"line 2", "cannot unmarshal !!str `one.conf` into []string"}},
		{"scalar root", "just a string\n", []string{"config: parse", "cannot unmarshal !!str `just a ...` into config.Config"}},
		{"sequence root", "- a\n- b\n", []string{"cannot unmarshal !!seq into config.Config"}},
		{"duplicate key", "proxy:\n  socks5_port: 1\n  socks5_port: 2\n", []string{"line 3", "mapping key \"socks5_port\" already defined at line 2"}},
		{"unknown anchor", "proxy: *nope\n", []string{"unknown anchor 'nope' referenced"}},
		{"merge of a scalar", "sidecars:\n  tor:\n    <<: 5\n", []string{"map merge requires map or sequence of maps as the value"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := loadString(t, tc.body)
			if err == nil {
				t.Fatalf("expected an error, got config %+v", cfg)
			}
			if cfg != nil {
				t.Errorf("config must be nil on error, got %+v", cfg)
			}
			for _, sub := range tc.want {
				if !strings.Contains(err.Error(), sub) {
					t.Errorf("error %q does not contain %q", err, sub)
				}
			}
		})
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	if err == nil || !strings.Contains(err.Error(), "config: read") || !os.IsNotExist(unwrapAll(err)) {
		t.Errorf("want a wrapped not-exist read error, got %v", err)
	}
}

func unwrapAll(err error) error {
	for {
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return err
		}
		err = u.Unwrap()
	}
}

func TestLoad_OnlyFirstDocumentIsRead(t *testing.T) {
	cfg := mustLoad(t, "proxy:\n  socks5_port: 5080\n---\nproxy:\n  socks5_port: 6080\n")
	if cfg.Proxy.SOCKS5Port != 5080 {
		t.Errorf("only the first YAML document should be decoded, got port %d", cfg.Proxy.SOCKS5Port)
	}
}
