package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration structure.
type Config struct {
	Proxy        ProxyConfig        `yaml:"proxy"`
	Subscription SubscriptionConfig `yaml:"subscription"`
	LoadBalancing LoadBalancingConfig `yaml:"load_balancing"`
	Plugins      PluginsConfig      `yaml:"plugins"`
	Sidecars     SidecarsConfig     `yaml:"sidecars"`
}

type ProxyConfig struct {
	SOCKS5Port int `yaml:"socks5_port"`
	HTTPPort   int `yaml:"http_port"`
	APIPort    int `yaml:"api_port"`
}

type SubscriptionConfig struct {
	URL  string `yaml:"url"`
	File string `yaml:"file"`
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
	Enabled  bool `yaml:"enabled"`
	Priority int  `yaml:"priority,omitempty"`
}

type SidecarsConfig struct {
	MasterDNS  SidecarEntry `yaml:"masterdns"`
	DNSTT      SidecarEntry `yaml:"dnstt"`
	Slipstream SidecarEntry `yaml:"slipstream"`
	Psiphon    SidecarEntry `yaml:"psiphon"`
	Tor        SidecarEntry `yaml:"tor"`
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
