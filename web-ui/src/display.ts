// Display helpers — keep canonical IDs intact in storage / wire format, but
// render clean, consistent labels in the dashboard. The Source column already
// shows which bundle an endpoint came from, so the NAME doesn't repeat it:
//   "MoaV-Hysteria2-beezhantest2"  → "Hysteria2"
//   "beezhantest2/wireguard"       → "WireGuard"
//   "sidecar-masterdns"            → "MasterDNS"

// Pretty-case for known lowercase protocol / sidecar kinds.
const PRETTY: Record<string, string> = {
  wireguard: "WireGuard",
  amneziawg: "AmneziaWG",
  masterdns: "MasterDNS",
  trusttunnel: "TrustTunnel",
  psiphon: "Psiphon",
  tor: "Tor",
  dnstt: "DNSTT",
  slipstream: "Slipstream",
  hysteria2: "Hysteria2",
  vless: "VLESS",
  vmess: "VMess",
  trojan: "Trojan",
  anytls: "AnyTLS",
  tuic: "TUIC",
  ss: "Shadowsocks",
  shadowsocks: "Shadowsocks",
};

function escapeRegex(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

export function displayEndpointName(name: string, id?: string, source?: string): string {
  let s = name || id || "";
  // Strip sidecar prefixes ("sidecar-masterdns" / "sidecar:masterdns").
  s = s.replace(/^sidecar[-_:]/, "");
  // Strip a leading "MoaV-" decoration.
  s = s.replace(/^MoaV[-_]/i, "");
  // Strip the source/bundle decoration: "<label>-<source>", "<label>/<source>",
  // or a leading "<source>/<label>".
  if (source) {
    const src = escapeRegex(source);
    s = s.replace(new RegExp(`[-_/]${src}$`, "i"), "");
    s = s.replace(new RegExp(`^${src}[-_/]`, "i"), "");
  }
  s = s.trim();
  const key = s.toLowerCase();
  if (PRETTY[key]) return PRETTY[key];
  return s || name || id || "";
}

// For per-connection flow rows. They show "vless · vless:1.2.3.4:443" or
// "sidecar · sidecar:psiphon" — the second part is the endpoint ID and the
// "sidecar:" prefix is the type tag. Strip just the leading "sidecar:".
export function displayEndpointId(id: string): string {
  return id.replace(/^sidecar:/, "");
}

// resolveLabel mirrors the Go-side logic for sidecars — kind wins over the
// generic "sidecar" tag wherever we already pass sidecar_kind around.
export function displayProtocol(protocol: string, sidecarKind?: string): string {
  if (protocol === "sidecar" && sidecarKind) return sidecarKind;
  return protocol;
}
