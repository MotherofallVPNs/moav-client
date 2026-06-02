package sidecars

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// GenerateConfigs writes per-sidecar config files into baseDir using the
// free-form Config map declared on each SidecarEntry in config.yaml.
//
// We only generate for sidecars whose entry.Config carries the relevant keys;
// otherwise the sidecar container's entrypoint loop (which waits for its own
// config file) just keeps polling, which is also fine.
//
// Per-sidecar layout under baseDir:
//   masterdns/client_config.toml
//   psiphon/psiphon.config
//   amneziawg/awg0.conf
//   trusttunnel/client.toml
func (m *SidecarManager) GenerateConfigs(baseDir string) error {
	if m.Config.MasterDNS.Enabled {
		if err := writeMasterDNS(baseDir, m.Config.MasterDNS.Config); err != nil {
			return fmt.Errorf("masterdns: %w", err)
		}
	}
	if m.Config.Psiphon.Enabled {
		if err := writePsiphon(baseDir, m.Config.Psiphon.Config); err != nil {
			return fmt.Errorf("psiphon: %w", err)
		}
	}
	if m.Config.AmneziaWG.Enabled {
		if err := writeAmneziaWG(baseDir, m.Config.AmneziaWG.Config); err != nil {
			return fmt.Errorf("amneziawg: %w", err)
		}
	}
	if m.Config.TrustTunnel.Enabled {
		if err := writeTrustTunnel(baseDir, m.Config.TrustTunnel.Config); err != nil {
			return fmt.Errorf("trusttunnel: %w", err)
		}
	}
	return nil
}

func writeMasterDNS(baseDir string, c map[string]string) error {
	if c == nil || c["domain"] == "" || c["key"] == "" {
		return nil
	}
	domain := c["domain"]
	method := c["method"]
	if method == "" {
		method = "5"
	}
	key := c["key"]
	body := fmt.Sprintf(`DOMAINS = ["%s"]
DATA_ENCRYPTION_METHOD = %s
ENCRYPTION_KEY = "%s"
PROTOCOL_TYPE = "SOCKS5"
LISTEN_IP = "0.0.0.0"
LISTEN_PORT = 5300
CLIENT_RESOLVERS_FILE = "client_resolvers.txt"
`, domain, method, key)
	if err := writeAtomic(filepath.Join(baseDir, "masterdns", "client_config.toml"), []byte(body)); err != nil {
		return err
	}
	// Resolver list — both DoH-cap public resolvers + UDP 53 fallback.
	resolvers := []byte(`# One resolver per line. Used by the masterdns client to send queries.
# Format: <ip>:<port> [proto]
1.1.1.1:53
8.8.8.8:53
9.9.9.9:53
1.0.0.1:53
`)
	return writeAtomic(filepath.Join(baseDir, "masterdns", "client_resolvers.txt"), resolvers)
}

func writePsiphon(baseDir string, c map[string]string) error {
	if c == nil {
		return nil
	}
	// The user is expected to provide a real Psiphon config blob via the
	// 'config_json' key. We just persist it verbatim.
	if raw := c["config_json"]; raw != "" {
		return writeAtomic(filepath.Join(baseDir, "psiphon", "psiphon.config"), []byte(raw))
	}
	// Otherwise write a minimal default that points at the public
	// psiphon-tunnel-core server-list bootstrap (suitable for testing).
	cfg := map[string]any{
		"PropagationChannelId":         "FFFFFFFFFFFFFFFF",
		"SponsorId":                    "FFFFFFFFFFFFFFFF",
		"LocalSocksProxyPort":          5400,
		"LocalHttpProxyPort":           0,
		"EmitDiagnosticNotices":        true,
		"EmitBytesTransferred":         false,
		"DataRootDirectory":            "/var/lib/psiphon",
		"DisableLocalSocksProxy":       false,
		"DisableLocalHTTPProxy":        true,
	}
	enc, _ := json.MarshalIndent(cfg, "", "  ")
	return writeAtomic(filepath.Join(baseDir, "psiphon", "psiphon.config"), enc)
}

func writeAmneziaWG(baseDir string, c map[string]string) error {
	src := ""
	if c != nil {
		src = c["source_path"]
	}
	if src == "" {
		// No-op: container entrypoint waits for awg0.conf and will idle.
		return nil
	}
	raw, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read %s: %w", src, err)
	}
	return writeAtomic(filepath.Join(baseDir, "amneziawg", "awg0.conf"), raw)
}

func writeTrustTunnel(baseDir string, c map[string]string) error {
	src := ""
	if c != nil {
		src = c["source_path"]
	}
	if src == "" {
		return nil
	}
	raw, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read %s: %w", src, err)
	}
	return writeAtomic(filepath.Join(baseDir, "trusttunnel", "client.toml"), raw)
}

func writeAtomic(path string, body []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
