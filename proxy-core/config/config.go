package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration structure.
type Config struct {
	Proxy        ProxyConfig         `yaml:"proxy"`
	Subscription SubscriptionConfig  `yaml:"subscription"`
	LoadBalancing LoadBalancingConfig `yaml:"load_balancing"`
	Plugins      PluginsConfig       `yaml:"plugins"`
	Sidecars     SidecarsConfig      `yaml:"sidecars"`
	Singbox      SingboxConfig       `yaml:"singbox"`
	Xray         XrayConfig          `yaml:"xray"`
	SNISpoof     SNISpoofConfig      `yaml:"sni_spoof"`
}

// SingboxConfig controls the sing-box dialer sidecar integration.
type SingboxConfig struct {
	Enabled    bool   `yaml:"enabled"`
	ListenHost string `yaml:"listen_host"` // "0.0.0.0" inside Docker
	DialHost   string `yaml:"dial_host"`   // "singbox" inside compose, "127.0.0.1" on host
	BasePort   int    `yaml:"base_port"`
	OutputPath string `yaml:"output_path"` // where to write generated sing-box config
}

// XrayConfig controls the Xray-core dialer sidecar — handles transports
// sing-box can't speak (xhttp, splithttp, etc.).
type XrayConfig struct {
	Enabled    bool   `yaml:"enabled"`
	ListenHost string `yaml:"listen_host"`
	DialHost   string `yaml:"dial_host"`
	BasePort   int    `yaml:"base_port"`
	OutputPath string `yaml:"output_path"`
}

// SNISpoofConfig controls the optional SNI-spoofing sidecar
// (aleskxyz/SNI-Spoofing-Go wrapper). When enabled, every endpoint whose
// Config["fake_sni"] is set gets routed via the sidecar — sing-box / xray
// dial sni-spoof:port instead of the real upstream, the sidecar slips a
// decoy ClientHello onto the wire, then forwards the real TLS bytes.
//
// Reality is automatically excluded (its handshake auth doesn't survive a
// faked CH).
type SNISpoofConfig struct {
	Enabled    bool   `yaml:"enabled"`
	ListenHost string `yaml:"listen_host"`
	DialHost   string `yaml:"dial_host"`
	BasePort   int    `yaml:"base_port"`
	OutputPath string `yaml:"output_path"`

	// Defaults applied to any endpoint without an explicit per-endpoint
	// fake_sni / utls. If DefaultFakeSNI is empty AND no endpoint has its
	// own fake_sni set, the sidecar idles (mappings file is never written).
	DefaultFakeSNI string `yaml:"default_fake_sni"`
	DefaultUTLS    string `yaml:"default_utls"`
}

type ProxyAuthConfig struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type ProxyConfig struct {
	SOCKS5Port int             `yaml:"socks5_port"`
	HTTPPort   int             `yaml:"http_port"`
	APIPort    int             `yaml:"api_port"`
	Auth       ProxyAuthConfig `yaml:"auth"`
	// Exposure is the bind policy for the SOCKS5 / HTTP CONNECT listeners
	// AT THE HOST level (not the container — the container always binds
	// 0.0.0.0). One of: loopback (default), lan, public. The dashboard
	// writes this to .env so docker-compose picks it up on next up.
	Exposure string `yaml:"exposure,omitempty"`
}

// SourceConfig is one upstream subscription bundle (one MoaV server typically).
type SourceConfig struct {
	Name           string   `yaml:"name"`
	URL            string   `yaml:"url"`
	File           string   `yaml:"file"`
	WireGuardFiles []string `yaml:"wireguard_files"`
}

type SubscriptionConfig struct {
	// Single-source legacy form. If File or URL is set, an implicit source
	// named "default" is created. New configs should prefer Sources.
	URL            string   `yaml:"url"`
	File           string   `yaml:"file"`
	WireGuardFiles []string `yaml:"wireguard_files"`

	// Sources is the new multi-server list. Each entry becomes one logical
	// upstream group; per-endpoint .Source carries the SourceConfig.Name so
	// the dashboard can show which moav server each endpoint came from.
	Sources []SourceConfig `yaml:"sources,omitempty"`
}

// EffectiveSources collapses the single-source legacy fields + the Sources
// list into the unified view callers should use. Order: legacy first, then
// the explicit list.
func (s SubscriptionConfig) EffectiveSources() []SourceConfig {
	out := make([]SourceConfig, 0, len(s.Sources)+1)
	if s.URL != "" || s.File != "" || len(s.WireGuardFiles) > 0 {
		out = append(out, SourceConfig{
			Name:           "default",
			URL:            s.URL,
			File:           s.File,
			WireGuardFiles: s.WireGuardFiles,
		})
	}
	out = append(out, s.Sources...)
	return out
}

type LoadBalancingConfig struct {
	Strategy    string `yaml:"strategy"`
	ProbeOnStart bool   `yaml:"probe_on_start"`
}

// MatchExprConfig is the YAML representation of a single match expression.
type MatchExprConfig struct {
	Type  string `yaml:"type"`
	Value string `yaml:"value"`
}

// RoutingRuleConfig is the YAML representation of one routing rule.
// Match may be a single expression or a list; the parser handles both.
type RoutingRuleConfig struct {
	Match  MatchExprConfig `yaml:"match"`
	Action string          `yaml:"action"`
}

type PluginsConfig struct {
	TorrentBlock bool                `yaml:"torrent_block"`
	RoutingRules []RoutingRuleConfig `yaml:"routing_rules"`
}

type SidecarEntry struct {
	Enabled  bool              `yaml:"enabled"`
	Priority int               `yaml:"priority,omitempty"`
	Config   map[string]string `yaml:"config,omitempty"` // free-form per-sidecar params
}

type SidecarsConfig struct {
	MasterDNS   SidecarEntry `yaml:"masterdns"`
	DNSTT       SidecarEntry `yaml:"dnstt"`
	Slipstream  SidecarEntry `yaml:"slipstream"`
	Psiphon     SidecarEntry `yaml:"psiphon"`
	Tor         SidecarEntry `yaml:"tor"`
	AmneziaWG   SidecarEntry `yaml:"amneziawg"`
	TrustTunnel SidecarEntry `yaml:"trusttunnel"`
}

// Defaults returns a Config with sensible defaults.
func Defaults() *Config {
	return &Config{
		Proxy: ProxyConfig{
			SOCKS5Port: 1080,
			HTTPPort:   8080,
			APIPort:    8088,
		},
		LoadBalancing: LoadBalancingConfig{
			Strategy:     "latency",
			ProbeOnStart: true,
		},
		// sing-box does all protocol cryptography; without it the client can't
		// dial any endpoint, so it's on by default. dial_host "singbox" is the
		// compose service name — host-mode (non-docker) runs must override it.
		Singbox: SingboxConfig{
			Enabled:    true,
			ListenHost: "0.0.0.0",
			DialHost:   "singbox",
			BasePort:   10800,
			OutputPath: "data/singbox.json",
		},
		// Xray covers transports sing-box can't speak (xhttp/splithttp). Idle
		// when no such endpoints exist, so on by default is harmless.
		Xray: XrayConfig{
			Enabled:    true,
			ListenHost: "0.0.0.0",
			DialHost:   "xray",
			BasePort:   11800,
			OutputPath: "data/xray.json",
		},
		SNISpoof: SNISpoofConfig{
			Enabled:        false,
			ListenHost:     "0.0.0.0",
			DialHost:       "sni-spoof",
			BasePort:       12800,
			OutputPath:     "data/sni-spoof.json",
			DefaultUTLS:    "chrome",
		},
	}
}

// Load reads and parses the YAML config file at path.
func Load(path string) (*Config, error) {
	cfg := Defaults()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}

	return cfg, nil
}
