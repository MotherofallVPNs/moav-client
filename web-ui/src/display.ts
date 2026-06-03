// Display helpers — keep canonical IDs intact in storage / wire format,
// but strip the redundant "sidecar" prefix in the dashboard render layer.
//
// Backend names look like:
//   ID:         sidecar:psiphon
//   Name:       sidecar-psiphon
//   Protocol:   sidecar
//   Config.sidecar_kind: psiphon
// Users see the kind already in the chip/colored label; the prefix is noise.

export function displayEndpointName(name: string, id?: string): string {
  const src = name || id || "";
  // "sidecar-psiphon" → "psiphon"; leaves regular names alone.
  return src.replace(/^sidecar[-_]/, "").replace(/^sidecar:/, "");
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
