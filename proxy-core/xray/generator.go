// Package xray generates an Xray-core JSON config that exposes one SOCKS5
// inbound per parsed endpoint that sing-box CAN'T speak (xhttp, splithttp,
// other Xray-specific transports). We pair an inbound + outbound + a route
// rule per endpoint exactly like the singbox package, just for the leftover
// protocols. Endpoints already handled by sing-box are skipped here.
//
// Xray and sing-box both run on the moav-net Docker network with their own
// SOCKS5 port ranges — sing-box at base 10800, xray at base 11800 — so the
// balancer just dials whichever socks5_addr the generator pinned on the
// endpoint and doesn't need to know which dialer is on the other side.
package xray

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/ibeezhan/moav-client/proxy-core/subscription"
)

// Config controls bind / dial hosts and the inbound port range.
type Config struct {
	ListenHost string // 0.0.0.0 inside the xray container
	DialHost   string // "xray" — the docker-compose service name
	BasePort   int    // first inbound port; endpoint i listens on BasePort+i
}

// Defaults returns a sensible Docker-Compose default.
func Defaults() Config {
	return Config{
		ListenHost: "0.0.0.0",
		DialHost:   "xray",
		BasePort:   11800,
	}
}

// HandlesEndpoint reports whether this endpoint should be routed via Xray
// (because sing-box can't speak its transport). The singbox generator already
// rejects xhttp / splithttp / unknown net values; we mirror that decision
// rule here so the two generators never claim the same endpoint.
func HandlesEndpoint(ep subscription.Endpoint) bool {
	if ep.Protocol == "sidecar" {
		return false
	}
	// Telegram MTProxy lives only on Xray (sing-box has no mtproto outbound).
	if ep.Protocol == "mtproxy" {
		return true
	}
	c := ep.Config
	net := c["net"]
	if net == "" {
		net = c["type"]
	}
	switch net {
	case "xhttp", "splithttp", "raw":
		return true
	}
	return false
}

// Generate builds an Xray config covering only the endpoints HandlesEndpoint
// returns true for. The returned endpoint slice has Config["socks5_addr"]
// pointing at the xray container for those entries; everything else is left
// untouched.
func Generate(eps []subscription.Endpoint, cfg Config) (jsonBytes []byte, updated []subscription.Endpoint, err error) {
	if cfg.BasePort == 0 {
		cfg.BasePort = 11800
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
	var routeRules []map[string]any

	port := cfg.BasePort
	for i := range updated {
		ep := updated[i]
		if !HandlesEndpoint(ep) {
			continue
		}
		ob, ok := outboundFromEndpoint(ep)
		if !ok {
			continue
		}
		tag := fmt.Sprintf("ep-%d", i)
		ob["tag"] = "out-" + tag

		inbound := map[string]any{
			"tag":      "in-" + tag,
			"listen":   cfg.ListenHost,
			"port":     port,
			"protocol": "socks",
			"settings": map[string]any{
				"auth": "noauth",
				"udp":  true,
			},
		}
		inbounds = append(inbounds, inbound)
		outbounds = append(outbounds, ob)
		routeRules = append(routeRules, map[string]any{
			"type":        "field",
			"inboundTag":  []string{"in-" + tag},
			"outboundTag": "out-" + tag,
		})

		updated[i].Config = ensureConfigMap(updated[i].Config)
		updated[i].Config["socks5_addr"] = net.JoinHostPort(cfg.DialHost, strconv.Itoa(port))
		port++
	}

	if len(outbounds) == 0 {
		return nil, updated, nil
	}

	// Add a freedom fallback so unmatched routes go direct rather than dropping.
	outbounds = append(outbounds, map[string]any{
		"tag":      "out-direct",
		"protocol": "freedom",
	})

	root := map[string]any{
		"log":       map[string]any{"loglevel": "warning"},
		"inbounds":  inbounds,
		"outbounds": outbounds,
		"routing": map[string]any{
			"domainStrategy": "AsIs",
			"rules":          routeRules,
		},
	}

	jsonBytes, err = json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, nil, fmt.Errorf("xray: marshal: %w", err)
	}
	return jsonBytes, updated, nil
}

func ensureConfigMap(m map[string]string) map[string]string {
	if m == nil {
		return make(map[string]string)
	}
	return m
}

// outboundFromEndpoint maps an endpoint to an Xray outbound. Only VLESS with
// Xray-specific transports is implemented at the moment — that's the case
// the user actually hits in MoaV bundles.
func outboundFromEndpoint(ep subscription.Endpoint) (map[string]any, bool) {
	host, port, err := splitHostPort(ep.Address)
	if err != nil {
		return nil, false
	}
	var ob map[string]any
	switch ep.Protocol {
	case "vless":
		ob = vlessOutbound(ep, host, port)
	case "mtproxy":
		ob = mtproxyOutbound(ep, host, port)
	default:
		return nil, false
	}
	applySpoof(ob, ep)
	return ob, true
}

// applySpoof rewrites a vless outbound's vnext[0].address/port to the
// sni-spoof sidecar when Endpoint.Config["spoof_via"] is set. Same
// caveats as singbox.applySpoof — Reality is skipped because the
// fake CH breaks Reality auth.
func applySpoof(ob map[string]any, ep subscription.Endpoint) {
	via := ep.Config["spoof_via"]
	if via == "" {
		return
	}
	if ep.Config["security"] == "reality" {
		return
	}
	host, port, err := splitHostPort(via)
	if err != nil {
		return
	}
	settings, _ := ob["settings"].(map[string]any)
	if settings == nil {
		return
	}
	// vless outbound shape: settings.vnext[0].address/port
	if vnext, _ := settings["vnext"].([]any); len(vnext) > 0 {
		if first, _ := vnext[0].(map[string]any); first != nil {
			first["address"] = host
			first["port"] = port
		}
	}
	// mtproto outbound shape: settings.servers[0].address/port
	if servers, _ := settings["servers"].([]any); len(servers) > 0 {
		if first, _ := servers[0].(map[string]any); first != nil {
			first["address"] = host
			first["port"] = port
		}
	}
}

// mtproxyOutbound builds the Xray mtproto outbound entry.
func mtproxyOutbound(ep subscription.Endpoint, host string, port int) map[string]any {
	secret := ep.Config["secret"]
	return map[string]any{
		"protocol": "mtproto",
		"settings": map[string]any{
			"servers": []any{
				map[string]any{
					"address": host,
					"port":    port,
					"users": []any{
						map[string]any{"secret": secret},
					},
				},
			},
		},
	}
}

func vlessOutbound(ep subscription.Endpoint, host string, port int) map[string]any {
	c := ep.Config
	user := map[string]any{
		"id":         c["uuid"],
		"encryption": defaultStr(c["encryption"], "none"),
	}
	if c["flow"] != "" {
		user["flow"] = c["flow"]
	}

	streamSettings := map[string]any{
		"network": defaultStr(c["net"], "tcp"),
	}
	if streamSettings["network"] == "" {
		streamSettings["network"] = c["type"]
	}

	// Reality / TLS.
	security := c["security"]
	if security == "reality" {
		streamSettings["security"] = "reality"
		streamSettings["realitySettings"] = map[string]any{
			"serverName":  c["sni"],
			"fingerprint": defaultStr(normFP(c["fp"]), "chrome"),
			"publicKey":   c["pbk"],
			"shortId":     c["sid"],
			"spiderX":     "/",
		}
	} else if security == "tls" {
		streamSettings["security"] = "tls"
		tlsCfg := map[string]any{
			"serverName":  c["sni"],
			"fingerprint": defaultStr(normFP(c["fp"]), "chrome"),
		}
		if c["alpn"] != "" {
			tlsCfg["alpn"] = strings.Split(c["alpn"], ",")
		}
		streamSettings["tlsSettings"] = tlsCfg
	}

	// Transport-specific settings.
	netKind := streamSettings["network"]
	switch netKind {
	case "xhttp", "splithttp":
		settings := map[string]any{
			"path": defaultStr(c["path"], "/"),
			"host": c["host"],
			"mode": defaultStr(c["mode"], "auto"),
		}
		streamSettings["xhttpSettings"] = settings
	case "ws":
		streamSettings["wsSettings"] = map[string]any{
			"path": defaultStr(c["path"], "/"),
			"headers": map[string]any{
				"Host": c["host"],
			},
		}
	}

	return map[string]any{
		"protocol": "vless",
		"settings": map[string]any{
			"vnext": []any{
				map[string]any{
					"address": host,
					"port":    port,
					"users":   []any{user},
				},
			},
		},
		"streamSettings": streamSettings,
	}
}

func splitHostPort(addr string) (string, int, error) {
	host, p, err := net.SplitHostPort(addr)
	if err != nil {
		return "", 0, err
	}
	port, err := strconv.Atoi(p)
	if err != nil {
		return "", 0, fmt.Errorf("xray: bad port %q: %w", p, err)
	}
	return host, port, nil
}

func defaultStr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func normFP(s string) string {
	if s == "" || s == "random" {
		return "chrome"
	}
	return s
}
