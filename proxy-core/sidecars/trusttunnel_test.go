package sidecars

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// genTrustTunnel runs writeTrustTunnel over src and returns the generated client.toml.
func genTrustTunnel(t *testing.T, src string) string {
	t.Helper()
	dir := t.TempDir()
	in := filepath.Join(dir, "trusttunnel.toml")
	if err := os.WriteFile(in, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeTrustTunnel(dir, map[string]string{"source_path": in}); err != nil {
		t.Fatalf("writeTrustTunnel: %v", err)
	}
	return readFile(t, filepath.Join(dir, "trusttunnel", "client.toml"))
}

// tomlSections returns the section headers of a TOML document, in order.
func tomlSections(doc string) []string {
	var out []string
	for _, line := range strings.Split(doc, "\n") {
		if s := strings.TrimSpace(line); strings.HasPrefix(s, "[") {
			out = append(out, s)
		}
	}
	return out
}

// assertSocksOnlyListener checks the one invariant the sidecar depends on: the
// only listener left is the loopback SOCKS5 one, and it closes the document.
func assertSocksOnlyListener(t *testing.T, got string) {
	t.Helper()
	const stanza = "[listener.socks]\naddress = \"127.0.0.1:5601\"\n"
	if !strings.HasSuffix(got, stanza) {
		t.Errorf("config must end with the loopback socks stanza:\n%s", got)
	}
	n := 0
	for _, h := range tomlSections(got) {
		if strings.HasPrefix(h, "[listener") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("want exactly 1 listener section, got %d (%v)", n, tomlSections(got))
	}
}

// bundle.toml mirrors the trusttunnel.toml a MoaV server puts in a bundle; the
// golden output is what tests/trusttunnel-sidecar-test.sh feeds the real client.
func TestWriteTrustTunnel_MoaVBundleGolden(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join("testdata", "trusttunnel", "bundle.toml")
	if err := writeTrustTunnel(base, map[string]string{"source_path": src}); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(base, "trusttunnel", "client.toml"))
	want := readFile(t, filepath.Join("testdata", "trusttunnel", "client.golden.toml"))
	if got != want {
		t.Errorf("generated client.toml differs from golden\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
	assertSocksOnlyListener(t, got)
	if strings.Contains(got, "mtu_size") || strings.Contains(got, "included_routes") {
		t.Errorf("TUN-only keys leaked into the socks config:\n%s", got)
	}
}

func TestWriteTrustTunnel_Variants(t *testing.T) {
	const endpoint = "[endpoint]\n" +
		"hostname = \"tt.example.invalid\"\n" +
		"addresses = [\"192.0.2.1:4443\"]\n" +
		"username = \"u\"\n" +
		"password = \"p\"\n" +
		"upstream_protocol = \"http2\"\n"

	tests := []struct {
		name string
		src  string
		keep []string // must survive verbatim
		drop []string // must be gone
	}{
		{
			// Per-endpoint keys added in client 1.3.0 must reach the client as written.
			name: "client_1_3_0_endpoint_keys",
			src: "loglevel = \"info\"\nexclusions_scannable_ports = \"443,80\"\n\n" + endpoint +
				"tls_profile = \"firefox\"\n" +
				"client_random_auth_key = \"00112233445566778899aabbccddeeff\"\n" +
				"\n[listener.tun]\nmtu_size = 1280\n",
			keep: []string{
				"exclusions_scannable_ports = \"443,80\"\n",
				endpoint,
				"tls_profile = \"firefox\"\n",
				"client_random_auth_key = \"00112233445566778899aabbccddeeff\"\n",
			},
			drop: []string{"[listener.tun]", "mtu_size"},
		},
		{
			name: "multiple_listener_sections",
			src: endpoint +
				"[listener]\nfoo = 1\n" +
				"[listener.tun]\nmtu_size = 1350\nincluded_routes = [\"0.0.0.0/0\"]\n" +
				"[listener.socks]\naddress = \"0.0.0.0:1080\"\nusername = \"x\"\npassword = \"y\"\n",
			keep: []string{endpoint},
			drop: []string{"[listener]", "[listener.tun]", "0.0.0.0:1080", "foo = 1", "included_routes"},
		},
		{
			name: "section_after_listener_is_kept",
			src:  "[listener.tun]\nmtu_size = 1280\n\n" + endpoint + "tls_profile = \"chrome\"\n",
			keep: []string{endpoint, "tls_profile = \"chrome\"\n"},
			drop: []string{"[listener.tun]", "mtu_size"},
		},
		{
			name: "crlf_line_endings",
			src: strings.ReplaceAll("loglevel = \"info\"\n"+endpoint+
				"[listener.tun]\nmtu_size = 1280\nbound_if = \"\"\n", "\n", "\r\n"),
			keep: []string{"hostname = \"tt.example.invalid\"\r\n", "upstream_protocol = \"http2\"\r\n"},
			drop: []string{"[listener.tun]", "mtu_size", "bound_if"},
		},
		{
			name: "no_trailing_newline",
			src:  endpoint + "[listener.tun]\nmtu_size = 1280",
			keep: []string{endpoint},
			drop: []string{"[listener.tun]", "mtu_size"},
		},
		{
			name: "multiline_certificate",
			src: "[endpoint]\nhostname = \"tt.example.invalid\"\n" +
				"certificate = \"\"\"\n-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n\"\"\"\n" +
				"[listener.tun]\nmtu_size = 1280\n",
			keep: []string{"certificate = \"\"\"\n-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n\"\"\"\n"},
			drop: []string{"[listener.tun]", "mtu_size"},
		},
		{
			name: "no_listener_in_source",
			src:  endpoint,
			keep: []string{endpoint},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := genTrustTunnel(t, tt.src)
			assertSocksOnlyListener(t, got)
			for _, k := range tt.keep {
				if !strings.Contains(got, k) {
					t.Errorf("lost %q:\n%q", k, got)
				}
			}
			for _, d := range tt.drop {
				if strings.Contains(got, d) {
					t.Errorf("should have dropped %q:\n%q", d, got)
				}
			}
		})
	}
}
