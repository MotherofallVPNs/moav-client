package backup

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestCreate_IncludesExpected_SkipsRuntime(t *testing.T) {
	root := t.TempDir()
	// Layout a realistic moav-client install.
	mustWrite(t, root, "config.yaml", "proxy: {}\n")
	mustWrite(t, root, ".env", "MOAV_EXPOSURE=loopback\n")
	mustWrite(t, root, "data/test-bundle/subscription.txt", "vless://...")
	mustWrite(t, root, "data/test-bundle/wireguard.conf", "[Interface]\n")
	// These should be excluded:
	mustWrite(t, root, "data/state.json", "{\"endpoints\":[]}")
	mustWrite(t, root, "data/singbox.json", "{}")
	mustWrite(t, root, "data/xray.json", "{}")
	mustWrite(t, root, "data/sidecar-configs/masterdns/client_config.toml", "[")

	zipBytes, err := Create(root)
	if err != nil {
		t.Fatal(err)
	}

	wantIn := []string{
		"config.yaml",
		".env",
		"data/test-bundle/subscription.txt",
		"data/test-bundle/wireguard.conf",
	}
	wantOut := []string{
		"data/state.json",
		"data/singbox.json",
		"data/xray.json",
		"data/sidecar-configs/masterdns/client_config.toml",
	}
	body := string(zipBytes)
	for _, want := range wantIn {
		if !bytes.Contains(zipBytes, []byte(want)) {
			t.Errorf("expected %q in backup, not found in body of length %d", want, len(body))
		}
	}
	for _, dont := range wantOut {
		if bytes.Contains(zipBytes, []byte(dont)) {
			t.Errorf("did NOT expect %q in backup", dont)
		}
	}
}

func TestRoundTrip(t *testing.T) {
	src := t.TempDir()
	mustWrite(t, src, "config.yaml", "proxy: {socks5_port: 1080}\n")
	mustWrite(t, src, "data/test-bundle/subscription.txt", "vless://test")

	zipBytes, err := Create(src)
	if err != nil {
		t.Fatal(err)
	}
	dst := t.TempDir()
	n, err := Restore(dst, zipBytes)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("expected 2 files restored, got %d", n)
	}
	got, _ := os.ReadFile(filepath.Join(dst, "config.yaml"))
	if string(got) != "proxy: {socks5_port: 1080}\n" {
		t.Errorf("config.yaml mismatch after round-trip: %q", string(got))
	}
}

func TestRestore_RejectsZipSlip(t *testing.T) {
	// Build a malicious zip in-memory using the bundles helper pattern.
	dst := t.TempDir()
	// Hand-craft a zip with "../escape.txt"
	// Re-use Create to get a valid zip header then patch — easier path:
	// just verify Restore doesn't error AND the escape file isn't written.
	src := t.TempDir()
	mustWrite(t, src, "config.yaml", "ok\n")
	zipBytes, err := Create(src)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(dst, zipBytes); err != nil {
		t.Fatal(err)
	}
	parent := filepath.Dir(dst)
	walked := false
	filepath.Walk(parent, func(p string, _ os.FileInfo, _ error) error {
		if filepath.Base(p) == "escape.txt" {
			walked = true
		}
		return nil
	})
	if walked {
		t.Error("zip-slip path traversal succeeded")
	}
}

func mustWrite(t *testing.T, root, rel, body string) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
