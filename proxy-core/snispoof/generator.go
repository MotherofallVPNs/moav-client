// Package snispoof generates a mappings file for the sni-spoof sidecar
// (aleskxyz/SNI-Spoofing-Go wrapper) and rewrites each endpoint's effective
// dial address so sing-box / xray hit the spoofer first.
//
// One mapping per spoofed endpoint:
//   { "listen": ":<allocated-port>",
//     "connect": "<real-upstream-host:port>",
//     "fake_sni": "<decoy>",
//     "utls": "chrome" }
//
// Endpoint.Config["spoof_via"] is set to "<dial_host>:<allocated-port>" so
// the singbox / xray outbound generators dial the spoofer instead of the
// real upstream. The TLS server_name on the outbound stays the REAL SNI —
// the spoofer just slips a decoy CH onto the wire first.
package snispoof

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"

	"github.com/ibeezhan/moav-client/proxy-core/subscription"
)

// Config controls bind / dial hosts and the allocated port range.
type Config struct {
	ListenHost string // 0.0.0.0 inside the sidecar container
	DialHost   string // "sni-spoof" — docker-compose service name
	BasePort   int    // first listen port; endpoint i gets BasePort+i
}

// Defaults returns sensible Docker Compose values.
func Defaults() Config {
	return Config{
		ListenHost: "0.0.0.0",
		DialHost:   "sni-spoof",
		BasePort:   12800,
	}
}

// HandlesEndpoint reports whether this endpoint has a fake_sni set in its
// Config map — the trigger for routing it through the spoofer. The caller
// sets fake_sni either in the bundle source (subscription.sources[].spoof)
// or via the dashboard Settings → SNI spoof section.
func HandlesEndpoint(ep subscription.Endpoint) bool {
	if ep.Config == nil {
		return false
	}
	return ep.Config["fake_sni"] != ""
}

// Mapping is one row of the mappings file consumed by entrypoint.sh.
type Mapping struct {
	Listen      string `json:"listen"`
	Connect     string `json:"connect"`
	FakeSNI     string `json:"fake_sni"`
	UTLS        string `json:"utls,omitempty"`
	FakeRepeat  int    `json:"fake_repeat,omitempty"`
	FakeDelay   string `json:"fake_delay,omitempty"`
	SNIChunk    int    `json:"sni_chunk,omitempty"`
}

// Generate scans eps for any with fake_sni set, returns the mappings JSON
// blob to drop in data/sni-spoof.json AND a copy of the endpoint slice
// where each handled entry has Config["spoof_via"] set to dial_host:port.
//
// Endpoints with no fake_sni are returned untouched.
func Generate(eps []subscription.Endpoint, cfg Config) ([]byte, []subscription.Endpoint, error) {
	if cfg.BasePort == 0 {
		cfg.BasePort = 12800
	}
	if cfg.ListenHost == "" {
		cfg.ListenHost = "0.0.0.0"
	}
	if cfg.DialHost == "" {
		cfg.DialHost = "127.0.0.1"
	}

	updated := make([]subscription.Endpoint, len(eps))
	copy(updated, eps)

	var mappings []Mapping
	port := cfg.BasePort
	for i := range updated {
		ep := updated[i]
		if !HandlesEndpoint(ep) {
			continue
		}
		fakeSNI := ep.Config["fake_sni"]
		utls := ep.Config["utls"]
		if utls == "" {
			utls = "chrome"
		}
		mappings = append(mappings, Mapping{
			Listen:  ":" + strconv.Itoa(port),
			Connect: ep.Address,
			FakeSNI: fakeSNI,
			UTLS:    utls,
		})
		updated[i].Config = ensureConfigMap(updated[i].Config)
		updated[i].Config["spoof_via"] = net.JoinHostPort(cfg.DialHost, strconv.Itoa(port))
		port++
	}

	if len(mappings) == 0 {
		return nil, updated, nil
	}
	buf, err := json.MarshalIndent(mappings, "", "  ")
	if err != nil {
		return nil, nil, fmt.Errorf("snispoof: marshal: %w", err)
	}
	return buf, updated, nil
}

func ensureConfigMap(m map[string]string) map[string]string {
	if m == nil {
		return make(map[string]string)
	}
	return m
}
