package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/MotherofallVPNs/moav-client/proxy-core/bundles"
	"github.com/MotherofallVPNs/moav-client/proxy-core/config"
	"github.com/MotherofallVPNs/moav-client/proxy-core/plugins"
)

// Handler-level pins for every path that reads or rewrites config.yaml, so the
// file each dashboard action produces stays loadable by config.Load.

const handlerYAML = `# top comment
proxy:
    socks5_port: 1080 # inline
subscription:
    file: ./legacy.txt
    url: https://legacy.example/sub
    wireguard_files: [legacy.conf]
    # sources live here
    sources:
        - name: alpha
          file: ./alpha.txt
        - name: beta
          url: https://beta.example/sub
          origin: custom
plugins:
    block_direct: false
    routing_rules:
        - match: {type: geoip, value: ir}
          action: direct
sidecars:
    masterdns:
        enabled: true
        config:
            source: alpha
            method: 5
    tor:
        enabled: yes
sni_spoof:
    enabled: on
    default_fake_sni: fake.example
`

func mustLoadConfig(t *testing.T, path string) *config.Config {
	t.Helper()
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("config.Load after edit: %v\n%s", err, readString(t, path))
	}
	return cfg
}

func TestPersistBlockDirect_Golden(t *testing.T) {
	p := writeTemp(t, handlerYAML)
	s, _ := newTestMux(t, p, "", nil)
	if err := s.persistBlockDirect(true); err != nil {
		t.Fatal(err)
	}
	// This path round-trips through a generic map: keys come back sorted and
	// comments are dropped. Values and their types must survive exactly.
	want := `plugins:
    block_direct: true
    routing_rules:
        - action: direct
          match:
            type: geoip
            value: ir
proxy:
    socks5_port: 1080
sidecars:
    masterdns:
        config:
            method: 5
            source: alpha
        enabled: true
    tor:
        enabled: "yes"
sni_spoof:
    default_fake_sni: fake.example
    enabled: "on"
subscription:
    file: ./legacy.txt
    sources:
        - file: ./alpha.txt
          name: alpha
        - name: beta
          origin: custom
          url: https://beta.example/sub
    url: https://legacy.example/sub
    wireguard_files:
        - legacy.conf
`
	if got := readString(t, p); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	if cfg := mustLoadConfig(t, p); !cfg.Plugins.BlockDirect {
		t.Error("block_direct not persisted")
	}
}

func TestPersistBlockDirect_CreatesPluginsSection(t *testing.T) {
	p := writeTemp(t, "proxy:\n  socks5_port: 1080\n")
	s, _ := newTestMux(t, p, "", nil)
	if err := s.persistBlockDirect(true); err != nil {
		t.Fatal(err)
	}
	cfg := mustLoadConfig(t, p)
	if !cfg.Plugins.BlockDirect || cfg.Proxy.SOCKS5Port != 1080 {
		t.Errorf("unexpected config after persist: %+v", cfg)
	}

	bad := writeTemp(t, "proxy: [unterminated\n")
	s2, _ := newTestMux(t, bad, "", nil)
	if err := s2.persistBlockDirect(true); err == nil {
		t.Error("expected an error for unparseable config.yaml")
	}
	if got := readString(t, bad); got != "proxy: [unterminated\n" {
		t.Errorf("unparseable file was rewritten: %q", got)
	}
}

func TestPersistRoutingRules_RoundTripsThroughConfigLoad(t *testing.T) {
	p := writeTemp(t, handlerYAML)
	s, _ := newTestMux(t, p, "", nil)
	rules := []plugins.Rule{
		{Match: plugins.MatchExpr{Type: "port", Value: "443"}, ActionName: "block", Enabled: false, Note: "numeric-looking value"},
		{Match: plugins.MatchExpr{Type: "domain_suffix", Value: "ir"}, ActionName: "direct", Enabled: true},
		{Match: plugins.MatchExpr{Type: "domain_keyword", Value: "yes"}, ActionName: "proxy", Enabled: true, Note: "multi\nline: note # with hash"},
		{Match: plugins.MatchExpr{Type: "ip_cidr", Value: "10.0.0.0/8"}, ActionName: "direct", Enabled: true, Note: "فارسی"},
	}
	if err := s.persistRoutingRules(rules); err != nil {
		t.Fatal(err)
	}
	out := readString(t, p)
	for _, want := range []string{"# top comment", "socks5_port: 1080 # inline", "# sources live here"} {
		if !strings.Contains(out, want) {
			t.Errorf("comment %q lost:\n%s", want, out)
		}
	}

	cfg := mustLoadConfig(t, p)
	if len(cfg.Plugins.RoutingRules) != len(rules) {
		t.Fatalf("got %d rules, want %d\n%s", len(cfg.Plugins.RoutingRules), len(rules), out)
	}
	for i, r := range rules {
		got := cfg.Plugins.RoutingRules[i]
		if got.Match.Type != r.Match.Type || got.Match.Value != r.Match.Value || got.Action != r.ActionName || got.Note != r.Note {
			t.Errorf("rule %d = %+v, want %+v", i, got, r)
		}
		if got.Enabled == nil || *got.Enabled != r.Enabled {
			t.Errorf("rule %d enabled = %v, want %v", i, got.Enabled, r.Enabled)
		}
	}
	// Untouched sections decode exactly as before the edit.
	before := mustLoadConfig(t, writeTemp(t, handlerYAML))
	if !reflect.DeepEqual(cfg.Subscription, before.Subscription) || !reflect.DeepEqual(cfg.Sidecars, before.Sidecars) {
		t.Errorf("unrelated sections changed:\n%s", out)
	}
}

func TestHandleSources_ParsesConfig(t *testing.T) {
	p := writeTemp(t, handlerYAML)
	_, h := newTestMux(t, p, "", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/sources", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var resp struct {
		Sources []struct {
			Name    string   `json:"name"`
			File    string   `json:"file"`
			URL     string   `json:"url"`
			WGFiles []string `json:"wireguard_files"`
			Tags    []string `json:"tags"`
			IsMoav  bool     `json:"is_moav"`
		} `json:"sources"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Sources) != 3 {
		t.Fatalf("want 3 sources, got %+v", resp.Sources)
	}
	def, alpha, beta := resp.Sources[0], resp.Sources[1], resp.Sources[2]
	if def.Name != "default" || def.File != "./legacy.txt" || def.URL != "https://legacy.example/sub" ||
		!reflect.DeepEqual(def.WGFiles, []string{"legacy.conf"}) || !reflect.DeepEqual(def.Tags, []string{"subscription", "wireguard"}) {
		t.Errorf("default source = %+v", def)
	}
	// sidecars.masterdns.config.source tags the bundle that configured it.
	if alpha.Name != "alpha" || !alpha.IsMoav || !reflect.DeepEqual(alpha.Tags, []string{"subscription", "masterdns"}) {
		t.Errorf("alpha source = %+v", alpha)
	}
	if beta.Name != "beta" || beta.IsMoav || beta.URL != "https://beta.example/sub" {
		t.Errorf("beta source = %+v", beta)
	}

	bad := writeTemp(t, "subscription: [unterminated\n")
	_, hb := newTestMux(t, bad, "", nil)
	rec = httptest.NewRecorder()
	hb.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/sources", nil))
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "parse config") {
		t.Errorf("unparseable config: status %d body %q", rec.Code, rec.Body)
	}
}

func TestHandleSourceByName_Delete(t *testing.T) {
	p := writeTemp(t, handlerYAML)
	_, h := newTestMux(t, p, "", nil)
	do := func(name string) int {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/sources/"+name, nil))
		return rec.Code
	}

	if code := do("nope"); code != http.StatusNotFound {
		t.Errorf("unknown source: status %d", code)
	}
	if got := readString(t, p); got != handlerYAML {
		t.Errorf("a no-op delete must not rewrite the file:\n%s", got)
	}

	if code := do("alpha"); code != http.StatusOK {
		t.Fatalf("delete alpha: status %d", code)
	}
	cfg := mustLoadConfig(t, p)
	if len(cfg.Subscription.Sources) != 1 || cfg.Subscription.Sources[0].Name != "beta" {
		t.Errorf("sources after delete = %+v", cfg.Subscription.Sources)
	}
	if !strings.Contains(readString(t, p), "# sources live here") {
		t.Error("comment above sources lost on delete")
	}

	if code := do("default"); code != http.StatusOK {
		t.Fatalf("delete default: status %d", code)
	}
	cfg = mustLoadConfig(t, p)
	if cfg.Subscription.File != "" || cfg.Subscription.URL != "" || len(cfg.Subscription.WireGuardFiles) != 0 {
		t.Errorf("legacy fields not cleared: %+v", cfg.Subscription)
	}
	if len(cfg.Subscription.Sources) != 1 {
		t.Errorf("named sources must survive deleting default: %+v", cfg.Subscription.Sources)
	}
}

func TestAddSourceToConfig_WiresBundle(t *testing.T) {
	p := writeTemp(t, handlerYAML)
	s, _ := newTestMux(t, p, "", nil)
	res := &bundles.Result{
		Name:              "gamma",
		SubscriptionPath:  "data/gamma/subscription.txt",
		WireGuardConfPath: "data/gamma/wireguard.conf",
		AmneziaWGConfPath: "data/gamma/moav-x-awg.conf",
		TrustTunnelPath:   "data/gamma/trusttunnel.toml",
		MasterDNSDomain:   "m.gamma.example",
		MasterDNSKey:      "00ff",
		MasterDNSMethod:   "5",
	}
	if err := s.addSourceToConfig(res); err != nil {
		t.Fatal(err)
	}
	cfg := mustLoadConfig(t, p)
	srcs := cfg.Subscription.Sources
	if len(srcs) != 3 {
		t.Fatalf("sources = %+v", srcs)
	}
	if got, want := srcs[2], (config.SourceConfig{Name: "gamma", File: "data/gamma/subscription.txt", WireGuardFiles: []string{"data/gamma/wireguard.conf"}}); !reflect.DeepEqual(got, want) {
		t.Errorf("new source = %+v, want %+v", got, want)
	}
	if got, want := cfg.Sidecars.MasterDNS.Config, map[string]string{"source": "gamma", "method": "5", "domain": "m.gamma.example", "key": "00ff"}; !reflect.DeepEqual(got, want) {
		t.Errorf("masterdns config = %#v, want %#v", got, want)
	}
	if !cfg.Sidecars.MasterDNS.Enabled {
		t.Error("existing masterdns.enabled must be left as it was")
	}
	if got, want := cfg.Sidecars.TrustTunnel.Config, map[string]string{"source": "gamma", "source_path": "data/gamma/trusttunnel.toml"}; !reflect.DeepEqual(got, want) {
		t.Errorf("trusttunnel config = %#v, want %#v", got, want)
	}
	if cfg.Sidecars.TrustTunnel.Enabled {
		t.Error("a bundle import must not enable the sidecar")
	}
	if got := cfg.Sidecars.AmneziaWG.Config["source_path"]; got != "data/gamma/moav-x-awg.conf" {
		t.Errorf("amneziawg source_path = %q", got)
	}
	out := readString(t, p)
	for _, want := range []string{"# top comment", "socks5_port: 1080 # inline", "# sources live here"} {
		if !strings.Contains(out, want) {
			t.Errorf("comment %q lost:\n%s", want, out)
		}
	}
}

func TestHandleSNISpoof_GetAndPut(t *testing.T) {
	p := writeTemp(t, handlerYAML)
	s, _ := newTestMux(t, p, "", nil)

	rec := httptest.NewRecorder()
	s.handleSNISpoof(rec, httptest.NewRequest(http.MethodGet, "/api/snispoof", nil))
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("GET: %v: %s", err, rec.Body)
	}
	// The generic-map read sees the YAML 1.1 word "on" as a string, not a bool,
	// so the dashboard reports the spoofer as disabled for `enabled: on`.
	if got["enabled"] != false || got["default_fake_sni"] != "fake.example" || got["default_utls"] != "chrome" {
		t.Errorf("GET = %v", got)
	}

	rec = httptest.NewRecorder()
	body := strings.NewReader(`{"enabled":true,"default_fake_sni":"www.example.org","default_utls":"firefox"}`)
	s.handleSNISpoof(rec, httptest.NewRequest(http.MethodPut, "/api/snispoof", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status %d: %s", rec.Code, rec.Body)
	}
	cfg := mustLoadConfig(t, p)
	if !cfg.SNISpoof.Enabled || cfg.SNISpoof.DefaultFakeSNI != "www.example.org" || cfg.SNISpoof.DefaultUTLS != "firefox" {
		t.Errorf("sni_spoof after PUT = %+v", cfg.SNISpoof)
	}
	if out := readString(t, p); !strings.Contains(out, "# top comment") || !strings.Contains(out, "# sources live here") {
		t.Errorf("comments lost on PUT:\n%s", out)
	}

	rec = httptest.NewRecorder()
	s.handleSNISpoof(rec, httptest.NewRequest(http.MethodGet, "/api/snispoof", nil))
	got = nil
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["enabled"] != true || got["default_utls"] != "firefox" {
		t.Errorf("GET after PUT = %v", got)
	}
}
