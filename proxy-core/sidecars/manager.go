// Package sidecars exposes locally-running sidecar processes as Endpoints.
package sidecars

import (
	"fmt"

	"github.com/ibeezhan/moav-client/proxy-core/config"
	"github.com/ibeezhan/moav-client/proxy-core/subscription"
)

// SidecarManager converts the sidecars section of config into Endpoints.
// Each sidecar is assumed to expose a SOCKS5 inbound at a known port inside
// the moav-net Docker network. The endpoint's Config["socks5_addr"] is set
// to "<service>:<port>" so balancer.dialThrough hits the sidecar container
// directly (not 127.0.0.1, which inside proxy-core means its own loopback).
type SidecarManager struct {
	Config config.SidecarsConfig
}

type sidecarMeta struct {
	name       string
	dockerHost string // docker-compose service name
	port       int
	priority   int
	entry      config.SidecarEntry
}

// EnabledEndpoints returns one Endpoint per enabled sidecar.
func (m *SidecarManager) EnabledEndpoints() []subscription.Endpoint {
	entries := []sidecarMeta{
		{"masterdns", "masterdns", 5300, prio(m.Config.MasterDNS, 1), m.Config.MasterDNS},
		{"dnstt", "dns-tunnels", 5301, prio(m.Config.DNSTT, 5), m.Config.DNSTT},
		{"psiphon", "psiphon", 5400, prio(m.Config.Psiphon, 5), m.Config.Psiphon},
		{"tor", "tor", 9150, prio(m.Config.Tor, 5), m.Config.Tor},
		{"amneziawg", "amneziawg", 5500, prio(m.Config.AmneziaWG, 5), m.Config.AmneziaWG},
		{"trusttunnel", "trusttunnel", 5600, prio(m.Config.TrustTunnel, 5), m.Config.TrustTunnel},
	}

	var out []subscription.Endpoint
	for _, e := range entries {
		if !e.entry.Enabled {
			continue
		}
		dockerAddr := fmt.Sprintf("%s:%d", e.dockerHost, e.port)
		// If a bundle importer tagged this sidecar with its source, surface it
		// as the endpoint's Source so the dashboard's Source column shows the
		// originating bundle instead of the generic "sidecars" group.
		ep := subscription.Endpoint{
			ID:        "sidecar:" + e.name,
			Protocol:  "sidecar",
			Name:      "sidecar-" + e.name,
			Source:    e.entry.Config["source"],
			Address:   dockerAddr,
			RawURI:    "sidecar://" + dockerAddr,
			Priority:  e.priority,
			Enabled:   true,
			LatencyMs: -1,
			Status:    "unknown",
			Config:    map[string]string{"socks5_addr": dockerAddr, "sidecar_kind": e.name},
		}
		for k, v := range e.entry.Config {
			ep.Config[k] = v
		}
		out = append(out, ep)
	}
	return out
}

func prio(e config.SidecarEntry, dflt int) int {
	if e.Priority > 0 {
		return e.Priority
	}
	return dflt
}
