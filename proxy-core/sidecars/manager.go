// Package sidecars exposes locally-running sidecar processes as Endpoints.
package sidecars

import (
	"fmt"

	"github.com/ibeezhan/moav-client/proxy-core/config"
	"github.com/ibeezhan/moav-client/proxy-core/subscription"
)

// SidecarManager converts the sidecars section of config into Endpoints.
type SidecarManager struct {
	Config config.SidecarsConfig
}

type sidecarMeta struct {
	name     string
	port     int
	priority int
	enabled  bool
}

// EnabledEndpoints returns one Endpoint per enabled sidecar.
func (m *SidecarManager) EnabledEndpoints() []subscription.Endpoint {
	entries := []sidecarMeta{
		{"masterdns", 5300, 1, m.Config.MasterDNS.Enabled},
		{"dnstt", 5301, 5, m.Config.DNSTT.Enabled},
		{"slipstream", 5302, 5, m.Config.Slipstream.Enabled},
		{"psiphon", 5400, 5, m.Config.Psiphon.Enabled},
		{"tor", 9050, 5, m.Config.Tor.Enabled},
	}

	var out []subscription.Endpoint
	for _, e := range entries {
		if !e.enabled {
			continue
		}
		addr := fmt.Sprintf("127.0.0.1:%d", e.port)
		out = append(out, subscription.Endpoint{
			ID:        "sidecar:" + e.name,
			Protocol:  "sidecar",
			Name:      e.name,
			Address:   addr,
			RawURI:    "sidecar://" + addr,
			Priority:  e.priority,
			Enabled:   true,
			LatencyMs: -1,
			Status:    "unknown",
		})
	}
	return out
}
