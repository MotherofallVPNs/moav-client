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
//   masterdns/client_resolvers.txt
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

// writePsiphon assembles a Psiphon ConsoleClient config from individual keys
// in sidecars.psiphon.config (PropagationChannelId, SponsorId, the bootstrap
// server-list URLs, the signing pubkey) — all of which come from a Psiphon
// Inc. licensing agreement or from extracting them out of an official
// Psiphon Pro release. Without them the ConsoleClient still starts and opens
// its SOCKS5 listener so the moav-client probe TCP-connects cleanly, but
// no tunnel will establish — visible in the Debug tab as repeated
// "failed to fetch obfuscated remote server list" warnings.
//
// The simplest way to wire this up is to paste a full Psiphon-issued config
// blob under sidecars.psiphon.config.config_json — moav-client writes it
// verbatim to /etc/psiphon/psiphon.config and we don't synthesise anything.
func writePsiphon(baseDir string, c map[string]string) error {
	if c == nil {
		c = map[string]string{}
	}
	// Verbatim user config wins.
	if raw := c["config_json"]; raw != "" {
		return writeAtomic(filepath.Join(baseDir, "psiphon", "psiphon.config"), []byte(raw))
	}

	// The "FFFFFFFFFFFFFFFF" channel + sponsor IDs are the anonymous
	// Psiphon channel — they're real, Psiphon Inc. honors them, and they
	// ship in upstream OSS clients (verified against
	// Alessandros-Hube/PsiphonLinuxGUI/src/configs/psiphon.config which
	// runs against the same psiphon-tunnel-core-binaries we build). The
	// SignaturePublicKey + RemoteServerListUrl pair is the matching public
	// bootstrap list — Psiphon's signed S3 blob.
	propID := defaultStr(c["propagation_channel_id"], "FFFFFFFFFFFFFFFF")
	sponsorID := defaultStr(c["sponsor_id"], "FFFFFFFFFFFFFFFF")
	clientPlatform := defaultStr(c["client_platform"], "Linux_moav-client")
	pubKey := defaultStr(c["remote_server_list_signature_public_key"], psiphonDefaultPubKey)
	listURL := defaultStr(c["remote_server_list_url"], psiphonDefaultListURL)

	cfg := map[string]any{
		"PropagationChannelId":               propID,
		"SponsorId":                          sponsorID,
		"ClientPlatform":                     clientPlatform,
		"LocalSocksProxyPort":                5400,
		"LocalHttpProxyPort":                 0,
		"DisableLocalHTTPProxy":              true,
		"DataRootDirectory":                  "/var/lib/psiphon",
		"EmitDiagnosticNotices":              true,
		"EmitBytesTransferred":               true,
		"UseIndistinguishableTLS":            true,
		"RemoteServerListSignaturePublicKey": pubKey,
		"RemoteServerListUrl":                listURL,
		"RemoteServerListDownloadFilename":   "remote_server_list",
	}

	if url := c["obfuscated_server_list_root_url"]; url != "" {
		cfg["ObfuscatedServerListRootURLs"] = []map[string]string{{"URL": url}}
	}
	if region := c["egress_region"]; region != "" {
		cfg["EgressRegion"] = region
	}

	enc, _ := json.MarshalIndent(cfg, "", "  ")
	return writeAtomic(filepath.Join(baseDir, "psiphon", "psiphon.config"), enc)
}

// psiphonDefaultPubKey / psiphonDefaultListURL are the bootstrap material
// from Psiphon's public OSS distribution (see Alessandros-Hube/PsiphonLinuxGUI
// and Psiphon-Labs/psiphon-tunnel-core-binaries). They're public by design
// — the pubkey only validates the signed list, it doesn't authorise anything.
const (
	psiphonDefaultPubKey  = "MIICIDANBgkqhkiG9w0BAQEFAAOCAg0AMIICCAKCAgEAt7Ls+/39r+T6zNW7GiVpJfzq/xvL9SBH5rIFnk0RXYEYavax3WS6HOD35eTAqn8AniOwiH+DOkvgSKF2caqk/y1dfq47Pdymtwzp9ikpB1C5OfAysXzBiwVJlCdajBKvBZDerV1cMvRzCKvKwRmvDmHgphQQ7WfXIGbRbmmk6opMBh3roE42KcotLFtqp0RRwLtcBRNtCdsrVsjiI1Lqz/lH+T61sGjSjQ3CHMuZYSQJZo/KrvzgQXpkaCTdbObxHqb6/+i1qaVOfEsvjoiyzTxJADvSytVtcTjijhPEV6XskJVHE1Zgl+7rATr/pDQkw6DPCNBS1+Y6fy7GstZALQXwEDN/qhQI9kWkHijT8ns+i1vGg00Mk/6J75arLhqcodWsdeG/M/moWgqQAnlZAGVtJI1OgeF5fsPpXu4kctOfuZlGjVZXQNW34aOzm8r8S0eVZitPlbhcPiR4gT/aSMz/wd8lZlzZYsje/Jr8u/YtlwjjreZrGRmG8KMOzukV3lLmMppXFMvl4bxv6YFEmIuTsOhbLTwFgh7KYNjodLj/LsqRVfwz31PgWQFTEPICV7GCvgVlPRxnofqKSjgTWI4mxDhBpVcATvaoBl1L/6WLbFvBsoAUBItWwctO2xalKxF5szhGm8lccoc5MZr8kfE0uxMgsxz4er68iCID+rsCAQM="
	psiphonDefaultListURL = "https://s3.amazonaws.com//psiphon/web/mjr4-p23r-puwl/server_list_compressed"
)

func defaultStr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
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
