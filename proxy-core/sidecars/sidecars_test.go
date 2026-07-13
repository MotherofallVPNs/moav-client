package sidecars

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ibeezhan/moav-client/proxy-core/config"
)

func TestEnabledEndpoints_None(t *testing.T) {
	m := &SidecarManager{}
	if eps := m.EnabledEndpoints(); len(eps) != 0 {
		t.Errorf("no sidecars enabled should yield 0 endpoints, got %d", len(eps))
	}
}

func TestEnabledEndpoints_PortsAndDefaults(t *testing.T) {
	m := &SidecarManager{Config: config.SidecarsConfig{
		MasterDNS:   config.SidecarEntry{Enabled: true},
		Tor:         config.SidecarEntry{Enabled: true},
		TrustTunnel: config.SidecarEntry{Enabled: true},
	}}
	eps := m.EnabledEndpoints()
	if len(eps) != 3 {
		t.Fatalf("want 3 endpoints, got %d", len(eps))
	}

	byName := map[string]struct {
		addr     string
		priority int
	}{}
	for _, ep := range eps {
		byName[ep.Name] = struct {
			addr     string
			priority int
		}{ep.Address, ep.Priority}
		// Common invariants for all sidecar endpoints.
		if ep.Protocol != "sidecar" {
			t.Errorf("%s: protocol = %q, want sidecar", ep.Name, ep.Protocol)
		}
		if ep.LatencyMs != -1 || ep.Status != "unknown" || !ep.Enabled {
			t.Errorf("%s: unexpected defaults %+v", ep.Name, ep)
		}
		if ep.Config["socks5_addr"] != ep.Address {
			t.Errorf("%s: socks5_addr %q != address %q", ep.Name, ep.Config["socks5_addr"], ep.Address)
		}
	}

	// masterdns has a special default priority of 1; others default to 5.
	if got := byName["sidecar-masterdns"]; got.addr != "masterdns:5300" || got.priority != 1 {
		t.Errorf("masterdns: got %+v, want addr masterdns:5300 priority 1", got)
	}
	if got := byName["sidecar-tor"]; got.addr != "tor:9150" || got.priority != 5 {
		t.Errorf("tor: got %+v, want addr tor:9150 priority 5", got)
	}
	if got := byName["sidecar-trusttunnel"]; got.addr != "trusttunnel:5600" || got.priority != 5 {
		t.Errorf("trusttunnel: got %+v, want addr trusttunnel:5600 priority 5", got)
	}
}

func TestEnabledEndpoints_PriorityOverride(t *testing.T) {
	m := &SidecarManager{Config: config.SidecarsConfig{
		Psiphon: config.SidecarEntry{Enabled: true, Priority: 42},
	}}
	eps := m.EnabledEndpoints()
	if len(eps) != 1 {
		t.Fatalf("want 1 endpoint, got %d", len(eps))
	}
	if eps[0].Priority != 42 {
		t.Errorf("explicit priority should win: got %d, want 42", eps[0].Priority)
	}
	if eps[0].ID != "sidecar:psiphon" || eps[0].RawURI != "sidecar://psiphon:5400" {
		t.Errorf("unexpected id/rawuri: %+v", eps[0])
	}
}

func TestEnabledEndpoints_ConfigMergeAndSource(t *testing.T) {
	m := &SidecarManager{Config: config.SidecarsConfig{
		Tor: config.SidecarEntry{Enabled: true, Config: map[string]string{
			"source":    "my-bundle",
			"custom_key": "custom_val",
		}},
	}}
	eps := m.EnabledEndpoints()
	if len(eps) != 1 {
		t.Fatalf("want 1 endpoint, got %d", len(eps))
	}
	ep := eps[0]
	if ep.Source != "my-bundle" {
		t.Errorf("source = %q, want my-bundle", ep.Source)
	}
	// Free-form config keys should be merged onto the endpoint Config map.
	if ep.Config["custom_key"] != "custom_val" {
		t.Errorf("custom config key not merged: %+v", ep.Config)
	}
	// Base keys still present.
	if ep.Config["sidecar_kind"] != "tor" || ep.Config["socks5_addr"] != "tor:9150" {
		t.Errorf("base config keys missing: %+v", ep.Config)
	}
}

func TestStripListenerSections(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{
			name: "drops_listener_tun",
			in:   "[endpoint]\nhost = \"x\"\n[listener.tun]\naddress = \"10.0.0.1\"\n",
			want: "[endpoint]\nhost = \"x\"\n",
		},
		{
			// Trailing "\n" splits into a final empty element that is re-emitted,
			// so a kept-final section gains one extra trailing newline. Harmless:
			// callers append their own stanza after this output.
			name: "drops_bare_listener",
			in:   "[listener]\naddress = \"a\"\n[other]\nk = 1\n",
			want: "[other]\nk = 1\n\n",
		},
		{
			name: "multiple_listener_sections",
			in:   "[a]\nx=1\n[listener]\ny=2\n[listener.socks]\nz=3\n[b]\nw=4\n",
			want: "[a]\nx=1\n[b]\nw=4\n\n",
		},
		{
			name: "no_listener_unchanged",
			in:   "[a]\nx=1\n[b]\ny=2\n",
			want: "[a]\nx=1\n[b]\ny=2\n\n",
		},
		{
			name: "listener_at_eof",
			in:   "[keep]\nk=1\n[listener.tun]\nz=9\n",
			want: "[keep]\nk=1\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripListenerSections(tt.in); got != tt.want {
				t.Errorf("stripListenerSections(%q)\n got=%q\nwant=%q", tt.in, got, tt.want)
			}
		})
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func TestWriteMasterDNS(t *testing.T) {
	base := t.TempDir()

	// Missing required keys → no-op (no file written).
	if err := writeMasterDNS(base, map[string]string{"domain": "x"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(base, "masterdns", "client_config.toml")); !os.IsNotExist(err) {
		t.Errorf("expected no file written when key missing, stat err=%v", err)
	}
	// nil map → no-op.
	if err := writeMasterDNS(base, nil); err != nil {
		t.Fatal(err)
	}

	// Full config → writes both toml + resolvers, method defaults to 5.
	if err := writeMasterDNS(base, map[string]string{
		"domain": "m.example.com",
		"key":    "deadbeef",
	}); err != nil {
		t.Fatal(err)
	}
	toml := readFile(t, filepath.Join(base, "masterdns", "client_config.toml"))
	if !strings.Contains(toml, `DOMAINS = ["m.example.com"]`) {
		t.Errorf("domain missing from toml:\n%s", toml)
	}
	if !strings.Contains(toml, `ENCRYPTION_KEY = "deadbeef"`) {
		t.Errorf("key missing from toml:\n%s", toml)
	}
	if !strings.Contains(toml, "DATA_ENCRYPTION_METHOD = 5") {
		t.Errorf("default method 5 missing:\n%s", toml)
	}
	resolvers := readFile(t, filepath.Join(base, "masterdns", "client_resolvers.txt"))
	if !strings.Contains(resolvers, "1.1.1.1:53") {
		t.Errorf("resolvers list missing entries:\n%s", resolvers)
	}

	// Explicit method overrides the default.
	if err := writeMasterDNS(base, map[string]string{
		"domain": "m.example.com", "key": "k", "method": "3",
	}); err != nil {
		t.Fatal(err)
	}
	toml = readFile(t, filepath.Join(base, "masterdns", "client_config.toml"))
	if !strings.Contains(toml, "DATA_ENCRYPTION_METHOD = 3") {
		t.Errorf("explicit method not applied:\n%s", toml)
	}
}

func TestWritePsiphon_Verbatim(t *testing.T) {
	base := t.TempDir()
	raw := `{"custom":"blob"}`
	if err := writePsiphon(base, map[string]string{"config_json": raw}); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(base, "psiphon", "psiphon.config"))
	if got != raw {
		t.Errorf("verbatim config_json should be written unchanged: got %q", got)
	}
}

func TestWritePsiphon_Synthesized(t *testing.T) {
	base := t.TempDir()
	// nil config → synthesises defaults.
	if err := writePsiphon(base, nil); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(base, "psiphon", "psiphon.config"))
	for _, want := range []string{
		`"PropagationChannelId": "FFFFFFFFFFFFFFFF"`,
		`"SponsorId": "FFFFFFFFFFFFFFFF"`,
		`"LocalSocksProxyPort": 5400`,
		`"ClientPlatform": "Linux_moav-client"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("synthesized config missing %q:\n%s", want, got)
		}
	}
	// Optional keys absent by default.
	if strings.Contains(got, "EgressRegion") || strings.Contains(got, "ObfuscatedServerListRootURLs") {
		t.Errorf("optional keys should be absent by default:\n%s", got)
	}
}

func TestWritePsiphon_OptionalKeys(t *testing.T) {
	base := t.TempDir()
	if err := writePsiphon(base, map[string]string{
		"egress_region":                   "US",
		"obfuscated_server_list_root_url": "https://osl.example/root",
		"propagation_channel_id":          "ABC123",
	}); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(base, "psiphon", "psiphon.config"))
	if !strings.Contains(got, `"EgressRegion": "US"`) {
		t.Errorf("egress region not set:\n%s", got)
	}
	if !strings.Contains(got, "ObfuscatedServerListRootURLs") || !strings.Contains(got, "https://osl.example/root") {
		t.Errorf("obfuscated server list not set:\n%s", got)
	}
	if !strings.Contains(got, `"PropagationChannelId": "ABC123"`) {
		t.Errorf("propagation channel override not applied:\n%s", got)
	}
}

func TestWriteAmneziaWG(t *testing.T) {
	base := t.TempDir()

	// No source_path → no-op.
	if err := writeAmneziaWG(base, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(base, "amneziawg", "awg0.conf")); !os.IsNotExist(err) {
		t.Errorf("expected no file without source_path, stat err=%v", err)
	}

	// Valid source → copies verbatim.
	src := filepath.Join(t.TempDir(), "src.conf")
	body := "[Interface]\nPrivateKey = xxx\nJc = 4\n"
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeAmneziaWG(base, map[string]string{"source_path": src}); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(base, "amneziawg", "awg0.conf")); got != body {
		t.Errorf("awg config mismatch: got %q, want %q", got, body)
	}

	// Missing source file → error.
	if err := writeAmneziaWG(base, map[string]string{"source_path": "/no/such/file"}); err == nil {
		t.Error("expected error for missing source file")
	}
}

func TestWriteTrustTunnel(t *testing.T) {
	base := t.TempDir()

	// No source_path → no-op.
	if err := writeTrustTunnel(base, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(base, "trusttunnel", "client.toml")); !os.IsNotExist(err) {
		t.Errorf("expected no file without source_path, stat err=%v", err)
	}

	// Source with a [listener.tun] stanza → stripped, socks listener appended.
	src := filepath.Join(t.TempDir(), "tt.toml")
	body := "[endpoint]\nhostname = \"t.example.com\"\n[listener.tun]\naddress = \"10.0.0.1\"\n"
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeTrustTunnel(base, map[string]string{"source_path": src}); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(base, "trusttunnel", "client.toml"))
	if strings.Contains(got, "[listener.tun]") {
		t.Errorf("listener.tun should be stripped:\n%s", got)
	}
	if !strings.Contains(got, "[endpoint]") || !strings.Contains(got, "t.example.com") {
		t.Errorf("endpoint section should be preserved:\n%s", got)
	}
	if !strings.Contains(got, "[listener.socks]") || !strings.Contains(got, `address = "127.0.0.1:5601"`) {
		t.Errorf("socks listener should be appended:\n%s", got)
	}

	// Missing source file → error.
	if err := writeTrustTunnel(base, map[string]string{"source_path": "/no/such/file"}); err == nil {
		t.Error("expected error for missing source file")
	}
}

func TestGenerateConfigs(t *testing.T) {
	base := t.TempDir()
	m := &SidecarManager{Config: config.SidecarsConfig{
		MasterDNS: config.SidecarEntry{Enabled: true, Config: map[string]string{
			"domain": "m.example.com", "key": "k",
		}},
		Psiphon: config.SidecarEntry{Enabled: true, Config: map[string]string{
			"config_json": `{"x":1}`,
		}},
		// Disabled entries must not produce files.
		AmneziaWG: config.SidecarEntry{Enabled: false},
	}}
	if err := m.GenerateConfigs(base); err != nil {
		t.Fatalf("GenerateConfigs: %v", err)
	}
	if _, err := os.Stat(filepath.Join(base, "masterdns", "client_config.toml")); err != nil {
		t.Errorf("masterdns config not generated: %v", err)
	}
	if _, err := os.Stat(filepath.Join(base, "psiphon", "psiphon.config")); err != nil {
		t.Errorf("psiphon config not generated: %v", err)
	}
	if _, err := os.Stat(filepath.Join(base, "amneziawg")); !os.IsNotExist(err) {
		t.Errorf("disabled amneziawg should not produce a dir, stat err=%v", err)
	}
}

func TestGenerateConfigs_PropagatesError(t *testing.T) {
	base := t.TempDir()
	m := &SidecarManager{Config: config.SidecarsConfig{
		TrustTunnel: config.SidecarEntry{Enabled: true, Config: map[string]string{
			"source_path": "/no/such/file",
		}},
	}}
	if err := m.GenerateConfigs(base); err == nil {
		t.Error("expected error to propagate from writeTrustTunnel")
	}
}
