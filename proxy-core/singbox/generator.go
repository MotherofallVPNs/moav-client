// Package singbox generates a sing-box config that exposes one local SOCKS5
// inbound per parsed endpoint, with a matching protocol-specific outbound and
// a 1:1 route rule. moav-client then dials each endpoint by SOCKSing to the
// corresponding local sing-box port — keeping the Go balancer in charge of
// strategy selection while delegating real protocol cryptography to sing-box.
package singbox

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/ibeezhan/moav-client/proxy-core/subscription"
)

// Config holds the generator inputs.
type Config struct {
	// ListenHost is what we tell sing-box to bind the SOCKS5 inbounds to.
	// Inside Docker this is "0.0.0.0" (so other containers can reach it).
	ListenHost string

	// DialHost is what moav-client should dial to reach sing-box. In Docker
	// this is the service name ("singbox"); on the host it's "127.0.0.1".
	DialHost string

	// BasePort is the first local SOCKS5 port to allocate. Each endpoint
	// gets BasePort+i. Default 10800.
	BasePort int
}

// Defaults returns a sensible default configuration for Docker Compose usage.
func Defaults() Config {
	return Config{
		ListenHost: "0.0.0.0",
		DialHost:   "singbox",
		BasePort:   10800,
	}
}

// Generate builds a sing-box JSON config and returns the updated endpoint
// slice (with Config["socks5_addr"] set to DialHost:port for each endpoint
// that mapped successfully). Endpoints whose protocol cannot be expressed
// as a sing-box outbound are returned unchanged (so the prober still tries
// them via TCP, and the balancer falls back).
func Generate(eps []subscription.Endpoint, cfg Config) (jsonBytes []byte, updated []subscription.Endpoint, err error) {
	if cfg.BasePort == 0 {
		cfg.BasePort = 10800
	}
	if cfg.ListenHost == "" {
		cfg.ListenHost = "0.0.0.0"
	}
	if cfg.DialHost == "" {
		cfg.DialHost = "127.0.0.1"
	}

	updated = make([]subscription.Endpoint, len(eps))
	copy(updated, eps)

	var inbounds []map[string]any
	var outbounds []map[string]any
	var endpointsBlock []map[string]any
	var routeRules []map[string]any

	port := cfg.BasePort
	for i := range updated {
		ep := updated[i]
		if ep.Protocol == "sidecar" {
			// Sidecars already expose their own SOCKS5; skip them in sing-box.
			continue
		}

		// WireGuard uses sing-box's newer "endpoints" section, not "outbounds".
		// (AmneziaWG variants stay outside sing-box — they need a dedicated sidecar.)
		if ep.Protocol == "wireguard" {
			wgEp, ok := wireguardEndpointBlock(ep)
			if !ok {
				continue
			}
			tag := fmt.Sprintf("ep-%d", i)
			wgEp["tag"] = "wg-" + tag
			endpointsBlock = append(endpointsBlock, wgEp)

			inbounds = append(inbounds, map[string]any{
				"type":        "socks",
				"tag":         "in-" + tag,
				"listen":      cfg.ListenHost,
				"listen_port": port,
			})
			routeRules = append(routeRules, map[string]any{
				"inbound":  []string{"in-" + tag},
				"outbound": "wg-" + tag,
			})

			updated[i].Config = ensureConfigMap(updated[i].Config)
			updated[i].Config["socks5_addr"] = net.JoinHostPort(cfg.DialHost, strconv.Itoa(port))
			port++
			continue
		}

		ob, ok := outboundFromEndpoint(ep)
		if !ok {
			// Unsupported by sing-box (xhttp, amneziawg, etc.) — caller may
			// still expose this endpoint via a sidecar with its own socks5_addr.
			continue
		}
		tag := fmt.Sprintf("ep-%d", i)
		ob["tag"] = "out-" + tag
		outbounds = append(outbounds, ob)

		inbounds = append(inbounds, map[string]any{
			"type":        "socks",
			"tag":         "in-" + tag,
			"listen":      cfg.ListenHost,
			"listen_port": port,
		})
		routeRules = append(routeRules, map[string]any{
			"inbound":  []string{"in-" + tag},
			"outbound": "out-" + tag,
		})

		updated[i].Config = ensureConfigMap(updated[i].Config)
		updated[i].Config["socks5_addr"] = net.JoinHostPort(cfg.DialHost, strconv.Itoa(port))
		port++
	}

	// Always include a "direct" outbound so failed routes fall back somewhere
	// recognisable instead of dropping.
	outbounds = append(outbounds, map[string]any{
		"type": "direct",
		"tag":  "out-direct",
	})

	root := map[string]any{
		"log": map[string]any{
			"level":     "info",
			"timestamp": true,
		},
		"inbounds":  inbounds,
		"outbounds": outbounds,
		"route": map[string]any{
			"rules":                 routeRules,
			"final":                 "out-direct",
			"auto_detect_interface": false,
		},
	}
	if len(endpointsBlock) > 0 {
		root["endpoints"] = endpointsBlock
	}

	jsonBytes, err = json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, nil, fmt.Errorf("singbox: marshal: %w", err)
	}
	return jsonBytes, updated, nil
}

func ensureConfigMap(m map[string]string) map[string]string {
	if m == nil {
		return make(map[string]string)
	}
	return m
}

// wireguardEndpointBlock builds a sing-box 1.12+ "endpoints[]" entry for a
// WireGuard endpoint. We use the modern endpoint shape rather than the
// deprecated wireguard outbound so the config stays valid going forward.
func wireguardEndpointBlock(ep subscription.Endpoint) (map[string]any, bool) {
	host, port, err := splitHostPort(ep.Address)
	if err != nil {
		return nil, false
	}
	c := ep.Config
	if c["private_key"] == "" || c["public_key"] == "" {
		return nil, false
	}

	addresses := splitNonEmpty(c["address"], ",")
	if len(addresses) == 0 {
		// Sensible default that won't clash with anything else inside the
		// sing-box network namespace.
		addresses = []string{"172.16.0.2/32"}
	}
	allowed := splitNonEmpty(c["allowed_ips"], ",")
	if len(allowed) == 0 {
		allowed = []string{"0.0.0.0/0"}
	}
	mtu := 1408
	if v, err := strconv.Atoi(c["mtu"]); err == nil && v > 0 {
		mtu = v
	}
	peer := map[string]any{
		"address":     host,
		"port":        port,
		"public_key":  c["public_key"],
		"allowed_ips": allowed,
	}
	if c["psk"] != "" {
		peer["pre_shared_key"] = c["psk"]
	}
	ep1 := map[string]any{
		"type":        "wireguard",
		"address":     addresses,
		"private_key": c["private_key"],
		"mtu":         mtu,
		"peers":       []map[string]any{peer},
	}
	return ep1, true
}

func splitNonEmpty(s, sep string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, sep)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// outboundFromEndpoint maps a parsed subscription endpoint to a sing-box outbound
// block. Returns ok=false for protocols we can't express.
func outboundFromEndpoint(ep subscription.Endpoint) (map[string]any, bool) {
	host, port, err := splitHostPort(ep.Address)
	if err != nil {
		return nil, false
	}

	switch ep.Protocol {
	case "vless":
		return vlessOutbound(ep, host, port)
	case "trojan":
		return trojanOutbound(ep, host, port)
	case "ss":
		return ssOutbound(ep, host, port)
	case "hysteria2":
		return hysteria2Outbound(ep, host, port)
	case "vmess":
		return vmessOutbound(ep, host, port)
	case "tuic":
		return tuicOutbound(ep, host, port)
	default:
		return nil, false
	}
}

func vlessOutbound(ep subscription.Endpoint, host string, port int) (map[string]any, bool) {
	c := ep.Config
	ob := map[string]any{
		"type":        "vless",
		"server":      host,
		"server_port": port,
		"uuid":        c["uuid"],
	}
	if c["flow"] != "" {
		ob["flow"] = c["flow"]
	}

	// TLS / reality.
	security := c["security"]
	if security == "tls" || security == "reality" {
		tls := map[string]any{
			"enabled":     true,
			"server_name": c["sni"],
		}
		if c["alpn"] != "" {
			tls["alpn"] = strings.Split(c["alpn"], ",")
		}
		if c["fp"] != "" {
			tls["utls"] = map[string]any{
				"enabled":     true,
				"fingerprint": normalizeFingerprint(c["fp"]),
			}
		}
		if security == "reality" {
			tls["reality"] = map[string]any{
				"enabled":    true,
				"public_key": c["pbk"],
				"short_id":   c["sid"],
			}
		}
		ob["tls"] = tls
	}

	// Transport.
	t, ok := transportFromConfig(c)
	if !ok {
		return nil, false
	}
	if t != nil {
		ob["transport"] = t
	}
	return ob, true
}

func trojanOutbound(ep subscription.Endpoint, host string, port int) (map[string]any, bool) {
	c := ep.Config
	ob := map[string]any{
		"type":        "trojan",
		"server":      host,
		"server_port": port,
		"password":    c["password"],
	}
	if c["security"] == "tls" || c["security"] == "" {
		tls := map[string]any{
			"enabled":     true,
			"server_name": c["sni"],
		}
		if c["alpn"] != "" {
			tls["alpn"] = strings.Split(c["alpn"], ",")
		}
		if c["fp"] != "" {
			tls["utls"] = map[string]any{
				"enabled":     true,
				"fingerprint": normalizeFingerprint(c["fp"]),
			}
		}
		ob["tls"] = tls
	}
	t, ok := transportFromConfig(c)
	if !ok {
		return nil, false
	}
	if t != nil {
		ob["transport"] = t
	}
	return ob, true
}

func ssOutbound(ep subscription.Endpoint, host string, port int) (map[string]any, bool) {
	c := ep.Config
	return map[string]any{
		"type":        "shadowsocks",
		"server":      host,
		"server_port": port,
		"method":      c["method"],
		"password":    c["password"],
	}, true
}

func hysteria2Outbound(ep subscription.Endpoint, host string, port int) (map[string]any, bool) {
	c := ep.Config
	ob := map[string]any{
		"type":        "hysteria2",
		"server":      host,
		"server_port": port,
		"password":    c["auth"],
		"tls": map[string]any{
			"enabled":     true,
			"server_name": c["sni"],
			"insecure":    c["insecure"] == "1" || c["insecure"] == "true",
		},
	}
	if c["obfs"] != "" {
		ob["obfs"] = map[string]any{
			"type":     c["obfs"],
			"password": c["obfs_password"],
		}
	}
	return ob, true
}

func vmessOutbound(ep subscription.Endpoint, host string, port int) (map[string]any, bool) {
	c := ep.Config
	ob := map[string]any{
		"type":        "vmess",
		"server":      host,
		"server_port": port,
		"uuid":        c["uuid"],
		"security":    "auto",
	}
	if c["tls"] == "tls" {
		ob["tls"] = map[string]any{
			"enabled":     true,
			"server_name": c["host"],
		}
	}
	t, ok := transportFromConfig(c)
	if !ok {
		return nil, false
	}
	if t != nil {
		ob["transport"] = t
	}
	return ob, true
}

func tuicOutbound(ep subscription.Endpoint, host string, port int) (map[string]any, bool) {
	c := ep.Config
	return map[string]any{
		"type":               "tuic",
		"server":             host,
		"server_port":        port,
		"uuid":               c["uuid"],
		"password":           c["password"],
		"congestion_control": c["congestion"],
		"udp_relay_mode":     c["udp_relay_mode"],
		"tls": map[string]any{
			"enabled":     true,
			"server_name": c["sni"],
		},
	}, true
}

// transportFromConfig maps the "net"/"type" transport hint to a sing-box transport block.
// Returns (transport, supported). supported=false means we should skip this endpoint
// entirely — sing-box can't speak its transport (e.g. xhttp is Xray-only).
func transportFromConfig(c map[string]string) (map[string]any, bool) {
	net := c["net"]
	if net == "" {
		net = c["type"]
	}
	switch net {
	case "", "tcp":
		return nil, true
	case "ws":
		t := map[string]any{
			"type": "ws",
		}
		if c["path"] != "" {
			t["path"] = c["path"]
		}
		if c["host"] != "" {
			t["headers"] = map[string]any{"Host": c["host"]}
		}
		return t, true
	case "grpc":
		return map[string]any{
			"type":         "grpc",
			"service_name": c["path"],
		}, true
	case "http", "h2":
		t := map[string]any{
			"type": "http",
		}
		if c["host"] != "" {
			t["host"] = strings.Split(c["host"], ",")
		}
		if c["path"] != "" {
			t["path"] = c["path"]
		}
		return t, true
	case "httpupgrade":
		t := map[string]any{
			"type": "httpupgrade",
		}
		if c["path"] != "" {
			t["path"] = c["path"]
		}
		if c["host"] != "" {
			t["host"] = c["host"]
		}
		return t, true
	default:
		// xhttp, splithttp, raw, etc. — Xray-only or not yet supported.
		return nil, false
	}
}

func normalizeFingerprint(fp string) string {
	if fp == "" || fp == "random" {
		return "chrome"
	}
	return fp
}

func splitHostPort(addr string) (string, int, error) {
	host, p, err := net.SplitHostPort(addr)
	if err != nil {
		return "", 0, err
	}
	port, err := strconv.Atoi(p)
	if err != nil {
		return "", 0, fmt.Errorf("singbox: bad port %q: %w", p, err)
	}
	return host, port, nil
}
