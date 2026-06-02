package subscription

import (
	"bufio"
	"fmt"
	"net"
	"strconv"
	"strings"
)

// ParseWireGuardConf reads a wg-quick / AmneziaWG style INI config and
// returns an Endpoint. AmneziaWG-specific fields (Jc, Jmin, Jmax, S1, S2,
// H1..H4) are preserved in Config under the same names so a downstream
// AmneziaWG sidecar can pick them up — otherwise they're ignored.
//
// rawText is the file contents; nameHint becomes Endpoint.Name when the
// file doesn't carry a label.
func ParseWireGuardConf(rawText, nameHint string) (Endpoint, error) {
	cfg := make(map[string]string)
	var address []string
	var allowedIPs []string
	var endpointHost string

	section := ""
	scanner := bufio.NewScanner(strings.NewReader(rawText))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.Trim(line, "[]"))
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])

		switch section {
		case "interface":
			switch key {
			case "PrivateKey":
				cfg["private_key"] = val
			case "Address":
				for _, a := range splitCSV(val) {
					address = append(address, a)
				}
			case "DNS":
				cfg["dns"] = val
			case "MTU":
				cfg["mtu"] = val
			// AmneziaWG obfuscation params (passed through verbatim).
			case "Jc", "Jmin", "Jmax", "S1", "S2", "H1", "H2", "H3", "H4":
				cfg[strings.ToLower(key)] = val
			}
		case "peer":
			switch key {
			case "PublicKey":
				cfg["public_key"] = val
			case "PresharedKey":
				cfg["psk"] = val
			case "AllowedIPs":
				for _, a := range splitCSV(val) {
					allowedIPs = append(allowedIPs, a)
				}
			case "Endpoint":
				endpointHost = val
			case "PersistentKeepalive":
				cfg["persistent_keepalive"] = val
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return Endpoint{}, fmt.Errorf("wg conf scan: %w", err)
	}
	if endpointHost == "" {
		return Endpoint{}, fmt.Errorf("wg conf: missing Peer.Endpoint")
	}
	if cfg["private_key"] == "" || cfg["public_key"] == "" {
		return Endpoint{}, fmt.Errorf("wg conf: missing PrivateKey or PublicKey")
	}

	if len(address) > 0 {
		cfg["address"] = strings.Join(address, ",")
	}
	if len(allowedIPs) > 0 {
		cfg["allowed_ips"] = strings.Join(allowedIPs, ",")
	}

	// AmneziaWG is signalled by the presence of obfuscation params; the
	// vanilla protocol stays "wireguard" so sing-box can dial it.
	protocol := "wireguard"
	if _, hasJc := cfg["jc"]; hasJc {
		protocol = "amneziawg"
	}

	host, port, err := net.SplitHostPort(endpointHost)
	if err != nil {
		return Endpoint{}, fmt.Errorf("wg conf endpoint %q: %w", endpointHost, err)
	}
	if _, err := strconv.Atoi(port); err != nil {
		return Endpoint{}, fmt.Errorf("wg conf endpoint port %q: %w", port, err)
	}

	return Endpoint{
		ID:        genID(protocol, endpointHost),
		Protocol:  protocol,
		Name:      nameHint,
		Address:   net.JoinHostPort(host, port),
		RawURI:    "wgconf://" + nameHint, // synthetic — needs to be stable for dedup
		Config:    cfg,
		Enabled:   true,
		LatencyMs: -1,
		Status:    "unknown",
	}, nil
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
