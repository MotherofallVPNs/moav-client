package bundles

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// helper: builds an in-memory zip with the given files (path → contents).
func makeZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, body := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		f.Write([]byte(body))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestExtract_DetectsKnownFiles(t *testing.T) {
	base := t.TempDir()

	zipBytes := makeZip(t, map[string]string{
		"my-server/subscription.txt": "vless://abc@example.com:443?type=tcp#sample",
		"my-server/wireguard.conf":   "[Interface]\nPrivateKey = aaa\n",
		"my-server/amneziawg.conf":   "[Interface]\nPrivateKey = bbb\nJc = 4\n",
		"my-server/trusttunnel.toml": "[endpoint]\nhostname = \"t.example.com\"\n",
		"my-server/masterdns-instructions.txt": `# Tunnel Domain:
m.example.com
# encryption method:
5
# Encryption key:
abcdef0123456789
`,
	})

	res, err := Extract(zipBytes, base, "")
	if err != nil {
		t.Fatal(err)
	}
	// Name should be derived from the top-level dir inside the archive.
	if res.Name != "my-server" {
		t.Errorf("want name 'my-server', got %q", res.Name)
	}
	// Subscription / wg / amnezia / trusttunnel paths should all be detected.
	checks := []struct {
		got, wantSuffix, label string
	}{
		{res.SubscriptionPath, "subscription.txt", "subscription"},
		{res.WireGuardConfPath, "wireguard.conf", "wireguard"},
		{res.AmneziaWGConfPath, "amneziawg.conf", "amneziawg"},
		{res.TrustTunnelPath, "trusttunnel.toml", "trusttunnel"},
	}
	for _, c := range checks {
		if !strings.HasSuffix(c.got, c.wantSuffix) {
			t.Errorf("%s: want suffix %q, got %q", c.label, c.wantSuffix, c.got)
		}
	}
	// MasterDNS instructions should produce the parsed values.
	if res.MasterDNSDomain != "m.example.com" {
		t.Errorf("masterdns domain: want m.example.com, got %q", res.MasterDNSDomain)
	}
	if res.MasterDNSKey != "abcdef0123456789" {
		t.Errorf("masterdns key: want abcdef0123456789, got %q", res.MasterDNSKey)
	}
	if res.MasterDNSMethod != "5" {
		t.Errorf("masterdns method: want 5, got %q", res.MasterDNSMethod)
	}
	// Files should physically exist on disk inside base/my-server/.
	for _, name := range []string{"subscription.txt", "wireguard.conf", "amneziawg.conf", "trusttunnel.toml"} {
		if _, err := os.Stat(filepath.Join(base, "my-server", name)); err != nil {
			t.Errorf("expected %s on disk: %v", name, err)
		}
	}
}

// The v2 server renamed the per-protocol files: WG/AWG carry a moav-<tag>-
// prefix and short suffixes, and MasterDNS ships a TOML instead of the old
// free-form instructions. This mirrors the exact names a real `moav user
// package` zip contains, so the importer must still wire them up.
func TestExtract_DetectsV2BundleNames(t *testing.T) {
	base := t.TempDir()

	zipBytes := makeZip(t, map[string]string{
		"user-moav-configs/subscription.txt":     "dmxlc3M6Ly9hYmNAZXhhbXBsZS5jb206NDQz",
		"user-moav-configs/moav-yorks-wg.conf":   "[Interface]\nPrivateKey = aaa\n",
		"user-moav-configs/moav-yorks-wgws.conf": "[Interface]\nPrivateKey = ccc\n",
		"user-moav-configs/moav-yorks-awg.conf":  "[Interface]\nPrivateKey = bbb\nJc = 4\n",
		"user-moav-configs/trusttunnel.toml":     "[endpoint]\nhostname = \"t.example.com\"\n",
		"user-moav-configs/masterdns-client_config.toml": `# MasterDNS client config
DOMAINS = m.example.com
DATA_ENCRYPTION_METHOD = 5
ENCRYPTION_KEY = abcdef0123456789
PROTOCOL_TYPE = udp
`,
	})

	res, err := Extract(zipBytes, base, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(res.WireGuardConfPath, "moav-yorks-wg.conf") {
		t.Errorf("wireguard: v2 name not detected, got %q", res.WireGuardConfPath)
	}
	if !strings.HasSuffix(res.AmneziaWGConfPath, "moav-yorks-awg.conf") {
		t.Errorf("amneziawg: v2 name not detected, got %q", res.AmneziaWGConfPath)
	}
	// -wgws.conf (WG-over-wstunnel) must NOT be misfiled as the plain WG conf.
	if strings.HasSuffix(res.WireGuardConfPath, "wgws.conf") {
		t.Errorf("wgws.conf wrongly taken as the primary wireguard conf: %q", res.WireGuardConfPath)
	}
	if res.MasterDNSDomain != "m.example.com" || res.MasterDNSKey != "abcdef0123456789" || res.MasterDNSMethod != "5" {
		t.Errorf("masterdns toml parse: got domain=%q key=%q method=%q",
			res.MasterDNSDomain, res.MasterDNSKey, res.MasterDNSMethod)
	}
}

// Real MoaV bundles emit the TOML array form for DOMAINS and an inline
// "# 5 = AES-256-GCM" comment after DATA_ENCRYPTION_METHOD; both must be
// stripped or the masterdns sidecar gets a garbage domain/method.
func TestParseMasterDNSToml_ArrayAndInlineComment(t *testing.T) {
	body := `# MasterDNS client config
DOMAINS = ["m.example.com"]
DATA_ENCRYPTION_METHOD = 5   # 5 = AES-256-GCM
ENCRYPTION_KEY = "deadbeefdeadbeefdeadbeefdeadbeef"
PROTOCOL_TYPE = "SOCKS5"
`
	domain, key, method := parseMasterDNSToml(body)
	if domain != "m.example.com" {
		t.Errorf("domain: got %q, want %q", domain, "m.example.com")
	}
	if method != "5" {
		t.Errorf("method: got %q, want %q (inline comment leaked)", method, "5")
	}
	if key != "deadbeefdeadbeefdeadbeefdeadbeef" {
		t.Errorf("key: got %q", key)
	}
}

func TestExtract_RejectsZipSlip(t *testing.T) {
	base := t.TempDir()
	zipBytes := makeZip(t, map[string]string{
		"good/innocuous.txt": "hi",
		// Path traversal attempt.
		"../escape.txt":   "boom",
		"good/../../also": "x",
	})
	res, err := Extract(zipBytes, base, "test")
	if err != nil {
		t.Fatal(err)
	}
	// Walk base; nothing should exist outside <base>/test/ .
	filepath.Walk(filepath.Dir(base), func(p string, _ os.FileInfo, _ error) error {
		if strings.Contains(p, "escape.txt") || strings.HasSuffix(p, "also") {
			t.Errorf("zip-slip leaked file outside dest: %s", p)
		}
		return nil
	})
	_ = res
}

func TestExtract_SanitizesName(t *testing.T) {
	base := t.TempDir()
	zipBytes := makeZip(t, map[string]string{"sub.txt": "x"})
	res, err := Extract(zipBytes, base, "weird/../$$$%^$^name/with/slashes")
	if err != nil {
		t.Fatal(err)
	}
	// Sanitized name should not contain any path separators.
	if strings.ContainsAny(res.Name, "/\\") {
		t.Errorf("name not sanitized: %q", res.Name)
	}
	if res.Name == "" {
		t.Error("name empty after sanitize")
	}
}
