// Package subscription parses V2Ray-style subscription feeds into Endpoints.
package subscription

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// Endpoint is a unified representation of one upstream proxy entry.
type Endpoint struct {
	ID        string
	Protocol  string // vless, vmess, trojan, ss, hysteria2, wireguard, tuic, sidecar
	Name      string // fragment from URI
	Address   string // host:port
	RawURI    string // original URI before any parsing
	Source    string // friendly name of the subscription bundle / moav server this came from
	Config    map[string]string
	Priority  int
	Enabled   bool
	LatencyMs int64  // -1 = untested
	Status    string // unknown, ok, timeout, error
}

// ParseSubscription decodes a base64-encoded subscription blob and returns
// all parseable endpoints. Lines that fail to parse are silently skipped.
func ParseSubscription(raw string) ([]Endpoint, error) {
	// Try to base64-decode first; fall back to treating as raw text.
	decoded, err := base64Decode(strings.TrimSpace(raw))
	if err != nil {
		decoded = raw
	}

	var endpoints []Endpoint
	for _, line := range strings.Split(decoded, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// moav:// is a multi-endpoint bundle URL — expand into N entries.
		if strings.HasPrefix(line, "moav://") {
			eps, err := ParseMoaVBundle(line)
			if err != nil {
				continue
			}
			endpoints = append(endpoints, eps...)
			continue
		}
		ep, err := ParseURI(line)
		if err != nil {
			continue // skip unrecognised lines
		}
		endpoints = append(endpoints, ep)
	}
	return endpoints, nil
}

// ParseURI dispatches a single proxy URI to the correct scheme parser.
//
// Note: moav:// is multi-endpoint by design, so it isn't dispatched here.
// ParseSubscription handles it as a special case before falling through
// to ParseURI for the per-line entries.
func ParseURI(uri string) (Endpoint, error) {
	switch {
	case strings.HasPrefix(uri, "vless://"):
		return parseVLESS(uri)
	case strings.HasPrefix(uri, "vmess://"):
		return parseVMess(uri)
	case strings.HasPrefix(uri, "trojan://"):
		return parseTrojan(uri)
	case strings.HasPrefix(uri, "ss://"):
		return parseSS(uri)
	case strings.HasPrefix(uri, "hysteria2://"):
		return parseHysteria2(uri)
	case strings.HasPrefix(uri, "wireguard://"), strings.HasPrefix(uri, "wg://"):
		return parseWireGuard(uri)
	case strings.HasPrefix(uri, "tuic://"):
		return parseTUIC(uri)
	case strings.HasPrefix(uri, "tg://"), strings.HasPrefix(uri, "mtproxy://"), strings.HasPrefix(uri, "https://t.me/proxy"):
		return parseMTProxy(uri)
	default:
		return Endpoint{}, fmt.Errorf("unsupported scheme: %s", uri)
	}
}

// ---------------------------------------------------------------------------
// VLESS — vless://uuid@host:port?type=...&security=...&pbk=...#name
// ---------------------------------------------------------------------------

func parseVLESS(uri string) (Endpoint, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return Endpoint{}, fmt.Errorf("vless: %w", err)
	}
	q := u.Query()
	cfg := map[string]string{
		"uuid":       u.User.Username(),
		"net":        q.Get("type"),
		"security":   q.Get("security"),
		"flow":       q.Get("flow"),
		"pbk":        q.Get("pbk"),
		"sid":        q.Get("sid"),
		"sni":        q.Get("sni"),
		"fp":         q.Get("fp"),
		"path":       q.Get("path"),
		"host":       q.Get("host"),
		"alpn":       q.Get("alpn"),
		"encryption": q.Get("encryption"),
	}
	return Endpoint{
		ID:        genID("vless", u.Host),
		Protocol:  "vless",
		Name:      u.Fragment,
		Address:   u.Host,
		RawURI:    uri,
		Config:    cfg,
		Enabled:   true,
		LatencyMs: -1,
		Status:    "unknown",
	}, nil
}

// ---------------------------------------------------------------------------
// VMess — vmess://<base64(JSON)>
// ---------------------------------------------------------------------------

func parseVMess(uri string) (Endpoint, error) {
	b64 := strings.TrimPrefix(uri, "vmess://")
	// Fragment might be appended; strip it.
	if idx := strings.IndexByte(b64, '#'); idx != -1 {
		b64 = b64[:idx]
	}
	data, err := base64Decode(b64)
	if err != nil {
		return Endpoint{}, fmt.Errorf("vmess base64: %w", err)
	}

	var v struct {
		Add  string      `json:"add"`
		Port interface{} `json:"port"`
		ID   string      `json:"id"`
		Aid  interface{} `json:"aid"`
		Net  string      `json:"net"`
		Type string      `json:"type"`
		Host string      `json:"host"`
		Path string      `json:"path"`
		TLS  string      `json:"tls"`
		PS   string      `json:"ps"`
	}
	if err := json.Unmarshal([]byte(data), &v); err != nil {
		return Endpoint{}, fmt.Errorf("vmess json: %w", err)
	}

	port := fmt.Sprintf("%v", v.Port)
	addr := joinHostPort(v.Add, port)

	cfg := map[string]string{
		"uuid": v.ID,
		"aid":  fmt.Sprintf("%v", v.Aid),
		"net":  v.Net,
		"type": v.Type,
		"host": v.Host,
		"path": v.Path,
		"tls":  v.TLS,
	}
	return Endpoint{
		ID:        genID("vmess", addr),
		Protocol:  "vmess",
		Name:      v.PS,
		Address:   addr,
		RawURI:    uri,
		Config:    cfg,
		Enabled:   true,
		LatencyMs: -1,
		Status:    "unknown",
	}, nil
}

// ---------------------------------------------------------------------------
// Trojan — trojan://password@host:port?sni=...#name
// ---------------------------------------------------------------------------

func parseTrojan(uri string) (Endpoint, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return Endpoint{}, fmt.Errorf("trojan: %w", err)
	}
	q := u.Query()
	cfg := map[string]string{
		"password": u.User.Username(),
		"sni":      q.Get("sni"),
		"security": q.Get("security"),
		"type":     q.Get("type"),
		"path":     q.Get("path"),
		"host":     q.Get("host"),
		"alpn":     q.Get("alpn"),
		"fp":       q.Get("fp"),
	}
	return Endpoint{
		ID:        genID("trojan", u.Host),
		Protocol:  "trojan",
		Name:      u.Fragment,
		Address:   u.Host,
		RawURI:    uri,
		Config:    cfg,
		Enabled:   true,
		LatencyMs: -1,
		Status:    "unknown",
	}, nil
}

// ---------------------------------------------------------------------------
// Shadowsocks — two formats:
//   Legacy:  ss://base64(method:password)@host:port#name
//   SIP002:  ss://base64(method:password)@host:port?plugin=...#name
//            or ss://userinfo@host:port where userinfo = base64(method:pass)
// ---------------------------------------------------------------------------

func parseSS(uri string) (Endpoint, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return Endpoint{}, fmt.Errorf("ss: %w", err)
	}

	var method, password string

	userinfo := u.User.String()
	if decoded, err2 := base64Decode(userinfo); err2 == nil && strings.Contains(decoded, ":") {
		// Legacy / SIP002 base64 userinfo
		parts := strings.SplitN(decoded, ":", 2)
		method, password = parts[0], parts[1]
	} else if pw, hasPw := u.User.Password(); hasPw {
		// SIP002 plain: method:password
		method = u.User.Username()
		password = pw
	} else {
		// Try to decode the whole authority section before "@"
		raw := strings.TrimPrefix(uri, "ss://")
		if at := strings.LastIndex(raw, "@"); at != -1 {
			decoded, err2 := base64Decode(raw[:at])
			if err2 == nil && strings.Contains(decoded, ":") {
				parts := strings.SplitN(decoded, ":", 2)
				method, password = parts[0], parts[1]
				// re-parse with proper host
				rest := raw[at:]
				u2, _ := url.Parse("ss://x" + rest)
				if u2 != nil {
					u.Host = u2.Host
					u.Fragment = u2.Fragment
				}
			}
		}
	}

	cfg := map[string]string{
		"method":   method,
		"password": password,
		"plugin":   u.Query().Get("plugin"),
	}
	return Endpoint{
		ID:        genID("ss", u.Host),
		Protocol:  "ss",
		Name:      u.Fragment,
		Address:   u.Host,
		RawURI:    uri,
		Config:    cfg,
		Enabled:   true,
		LatencyMs: -1,
		Status:    "unknown",
	}, nil
}

// ---------------------------------------------------------------------------
// Hysteria2 — hysteria2://auth@host:port?sni=...&insecure=...#name
// ---------------------------------------------------------------------------

func parseHysteria2(uri string) (Endpoint, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return Endpoint{}, fmt.Errorf("hysteria2: %w", err)
	}
	q := u.Query()
	cfg := map[string]string{
		"auth":          u.User.Username(),
		"sni":           q.Get("sni"),
		"insecure":      q.Get("insecure"),
		"obfs":          q.Get("obfs"),
		"obfs_password": q.Get("obfs-password"),
	}
	return Endpoint{
		ID:        genID("hysteria2", u.Host),
		Protocol:  "hysteria2",
		Name:      u.Fragment,
		Address:   u.Host,
		RawURI:    uri,
		Config:    cfg,
		Enabled:   true,
		LatencyMs: -1,
		Status:    "unknown",
	}, nil
}

// ---------------------------------------------------------------------------
// WireGuard — best-effort, store raw for wg-quick handoff
// wireguard://privkey@endpoint:port?pub=...&psk=...&dns=...#name
// ---------------------------------------------------------------------------

func parseWireGuard(uri string) (Endpoint, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return Endpoint{}, fmt.Errorf("wireguard: %w", err)
	}
	q := u.Query()
	cfg := map[string]string{
		"private_key": u.User.Username(),
		"public_key":  q.Get("pub"),
		"psk":         q.Get("psk"),
		"dns":         q.Get("dns"),
		"allowed_ips": q.Get("allowed_ips"),
		"mtu":         q.Get("mtu"),
	}
	return Endpoint{
		ID:        genID("wireguard", u.Host),
		Protocol:  "wireguard",
		Name:      u.Fragment,
		Address:   u.Host,
		RawURI:    uri,
		Config:    cfg,
		Enabled:   true,
		LatencyMs: -1,
		Status:    "unknown",
	}, nil
}

// ---------------------------------------------------------------------------
// TUIC — tuic://uuid:password@host:port?...#name
// ---------------------------------------------------------------------------

func parseTUIC(uri string) (Endpoint, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return Endpoint{}, fmt.Errorf("tuic: %w", err)
	}
	password, _ := u.User.Password()
	q := u.Query()
	cfg := map[string]string{
		"uuid":             u.User.Username(),
		"password":         password,
		"sni":              q.Get("sni"),
		"congestion":       q.Get("congestion_control"),
		"alpn":             q.Get("alpn"),
		"udp_relay_mode":   q.Get("udp_relay_mode"),
		"reduce_rtt":       q.Get("reduce_rtt"),
	}
	return Endpoint{
		ID:        genID("tuic", u.Host),
		Protocol:  "tuic",
		Name:      u.Fragment,
		Address:   u.Host,
		RawURI:    uri,
		Config:    cfg,
		Enabled:   true,
		LatencyMs: -1,
		Status:    "unknown",
	}, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// base64Decode tries standard and URL-safe base64, with and without padding.
func base64Decode(s string) (string, error) {
	// Normalise: replace URL-safe chars and strip whitespace.
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "-", "+")
	s = strings.ReplaceAll(s, "_", "/")
	// Add padding if needed.
	switch len(s) % 4 {
	case 2:
		s += "=="
	case 3:
		s += "="
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// genID produces a simple deterministic ID for an endpoint.
func genID(protocol, addr string) string {
	return protocol + ":" + addr
}

func joinHostPort(host, port string) string {
	return net.JoinHostPort(host, port)
}
