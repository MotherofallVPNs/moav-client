package subscription

import (
	"fmt"
	"net/url"
	"strings"
)

// ParseMoaVBundle expands one moav:// URL into the N Endpoint records it
// describes. See docs/MOAV_BUNDLE.md for the format spec.
//
// Grammar (simplified):
//
//	moav://<userTag>@<defaultHost>?<shared>&p=<proto-spec>&p=<...>#<label>
//
// where <shared> is a flat query-string carrying any keys that don't vary
// across protocols (uuid, pw, ss_method, ss_pw, pbk, sid, sni_default, fp),
// and each p= record is a comma-list:
//
//	p = <name>,<port>[,k=v[,k=v…]]
//	name = reality | vless-ws | vless-xhttp | trojan | anytls | ss | hy2 | tuic | vmess
//
// Per-protocol k=v overrides anything in <shared>. Each materialised
// Endpoint carries a synthetic RawURI of the equivalent single-protocol
// URI so dedup, persistence, and the existing balancer all keep working.
func ParseMoaVBundle(uri string) ([]Endpoint, error) {
	if !strings.HasPrefix(uri, "moav://") {
		return nil, fmt.Errorf("not a moav:// uri")
	}
	u, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("moav: parse url: %w", err)
	}

	host := u.Hostname() // <defaultHost>
	if host == "" {
		return nil, fmt.Errorf("moav: missing defaultHost")
	}
	userTag := u.User.Username() // optional
	label := u.Fragment           // optional friendly name

	// Snapshot shared params. We then strip p= and use the rest as the
	// merge floor for every protocol record.
	q := u.Query()
	pRecords := q["p"]
	q.Del("p")
	shared := flatten(q)

	if len(pRecords) == 0 {
		return nil, fmt.Errorf("moav: no p= entries")
	}

	var out []Endpoint
	for i, rec := range pRecords {
		ep, err := expandProto(rec, host, userTag, label, shared)
		if err != nil {
			return nil, fmt.Errorf("moav: p[%d]=%q: %w", i, rec, err)
		}
		out = append(out, ep)
	}
	return out, nil
}

func flatten(q url.Values) map[string]string {
	out := make(map[string]string, len(q))
	for k, vs := range q {
		if len(vs) > 0 {
			out[k] = vs[0]
		}
	}
	return out
}

// expandProto turns one "name,port,k=v,..." record into an Endpoint by
// projecting the shared params + the per-record overrides onto the
// protocol-specific URI scheme parser. We reuse the existing parseVLESS /
// parseTrojan / etc. by rebuilding the canonical scheme URI and delegating.
func expandProto(rec, defaultHost, userTag, label string, shared map[string]string) (Endpoint, error) {
	parts := strings.Split(rec, ",")
	if len(parts) < 2 {
		return Endpoint{}, fmt.Errorf("need at least name,port")
	}
	name := strings.TrimSpace(parts[0])
	port := strings.TrimSpace(parts[1])
	overrides := map[string]string{}
	for _, kv := range parts[2:] {
		kv = strings.TrimSpace(kv)
		if eq := strings.IndexByte(kv, '='); eq > 0 {
			overrides[kv[:eq]] = kv[eq+1:]
		}
	}
	// allow per-record host override
	hostOverride := overrides["host"]
	if hostOverride != "" {
		delete(overrides, "host")
	} else {
		hostOverride = defaultHost
	}

	merged := mergeFlat(shared, overrides)

	switch name {
	case "reality":
		// Build a vless:// URI with the merged params and parse via the
		// existing parseVLESS to get the same Config layout downstream.
		uri := buildVLESSURI(userTag, hostOverride, port, label, merged, "tcp", "reality")
		return ParseURI(uri)
	case "vless-ws":
		uri := buildVLESSURI(userTag, hostOverride, port, label, merged, "ws", merged["security"])
		return ParseURI(uri)
	case "vless-xhttp":
		uri := buildVLESSURI(userTag, hostOverride, port, label, merged, "xhttp", "reality")
		return ParseURI(uri)
	case "vmess":
		// vmess uses base64-encoded JSON. Synthesize an equivalent URI via
		// the vless parser is wrong here — but for v1 we only need the
		// protocol field set correctly downstream; the balancer dials via
		// sing-box, and sing-box's vmess outbound is generated from
		// Endpoint.Config. So we fake the RawURI but populate Config
		// directly.
		return Endpoint{
			ID:        genID("vmess", hostOverride+":"+port),
			Protocol:  "vmess",
			Name:      label + "-vmess",
			Address:   hostOverride + ":" + port,
			RawURI:    fmt.Sprintf("moav-vmess://%s:%s#%s", hostOverride, port, label),
			Config:    merged,
			Enabled:   true,
			LatencyMs: -1,
			Status:    "unknown",
		}, nil
	case "trojan":
		pw := defaultStr(merged["pw"], merged["password"])
		uri := fmt.Sprintf("trojan://%s@%s:%s?security=tls&sni=%s&type=tcp#%s-trojan",
			pw, hostOverride, port, query(merged, "sni", "sni_default"), label)
		return ParseURI(uri)
	case "anytls":
		pw := defaultStr(merged["pw"], merged["password"])
		uri := fmt.Sprintf("anytls://%s@%s:%s?sni=%s&insecure=0#%s-anytls",
			pw, hostOverride, port, query(merged, "sni", "sni_default"), label)
		return ParseURI(uri)
	case "ss":
		method := defaultStr(merged["ss_method"], "aes-256-gcm")
		pw := defaultStr(merged["ss_pw"], merged["password"])
		uri := fmt.Sprintf("ss://%s@%s:%s#%s-ss",
			base64URLEncode(method+":"+pw), hostOverride, port, label)
		return ParseURI(uri)
	case "hy2":
		auth := defaultStr(merged["pw"], merged["password"])
		params := []string{
			"sni=" + query(merged, "sni", "sni_default"),
		}
		if merged["obfs"] != "" {
			params = append(params, "obfs="+merged["obfs"])
			if merged["obfs_pw"] != "" {
				params = append(params, "obfs-password="+merged["obfs_pw"])
			}
		}
		uri := fmt.Sprintf("hysteria2://%s@%s:%s?%s#%s-hy2",
			auth, hostOverride, port, strings.Join(params, "&"), label)
		return ParseURI(uri)
	case "tuic":
		uuid := merged["uuid"]
		pw := defaultStr(merged["pw"], merged["password"])
		uri := fmt.Sprintf("tuic://%s:%s@%s:%s?sni=%s#%s-tuic",
			uuid, pw, hostOverride, port, query(merged, "sni", "sni_default"), label)
		return ParseURI(uri)
	default:
		return Endpoint{}, fmt.Errorf("unknown protocol %q (valid: reality, vless-ws, vless-xhttp, trojan, anytls, ss, hy2, tuic, vmess)", name)
	}
}

func buildVLESSURI(user, host, port, label string, m map[string]string, transport, security string) string {
	uuid := m["uuid"]
	if user != "" {
		uuid = user
	}
	params := []string{
		"type=" + transport,
		"encryption=none",
	}
	if security != "" {
		params = append(params, "security="+security)
	}
	for _, k := range []string{"flow", "sni", "fp", "pbk", "sid", "alpn", "path"} {
		if v := m[k]; v != "" {
			params = append(params, k+"="+v)
		}
	}
	// Allow the shared sni_default to back-fill the SNI.
	if !hasParam(params, "sni") && m["sni_default"] != "" {
		params = append(params, "sni="+m["sni_default"])
	}
	if m["host"] != "" {
		params = append(params, "host="+m["host"])
	}
	return fmt.Sprintf("vless://%s@%s:%s?%s#%s-%s", uuid, host, port, strings.Join(params, "&"), label, transport)
}

func hasParam(params []string, k string) bool {
	prefix := k + "="
	for _, p := range params {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}

func mergeFlat(a, b map[string]string) map[string]string {
	out := make(map[string]string, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

func defaultStr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func query(m map[string]string, keys ...string) string {
	for _, k := range keys {
		if v := m[k]; v != "" {
			return v
		}
	}
	return ""
}

// base64URLEncode encodes the SS userinfo block per the SIP002 format.
func base64URLEncode(s string) string {
	const alpha = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	var out strings.Builder
	bs := []byte(s)
	for i := 0; i < len(bs); i += 3 {
		var n uint32
		k := 0
		for ; k < 3 && i+k < len(bs); k++ {
			n |= uint32(bs[i+k]) << (16 - 8*k)
		}
		for j := 0; j < 4; j++ {
			if j > k {
				out.WriteByte('=')
				continue
			}
			out.WriteByte(alpha[(n>>(18-6*j))&0x3F])
		}
	}
	return out.String()
}
