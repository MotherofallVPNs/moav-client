# Protocol parity: moav-client ↔ MoaV server

Which protocols the **MoaV server** hands out, and whether **moav-client** can
actually connect to each — and how. Audited 2026-07-12 against server v1.9.0 /
client v1.3.1. The sing-box mappings below are pinned by
`proxy-core/singbox/parity_test.go`; treat that test as the executable version
of this table.

## How the client connects

Three code paths turn a MoaV bundle endpoint into a live tunnel, each exposing a
local SOCKS5 the balancer dials:

1. **sing-box core** (`proxy-core/singbox/generator.go`) — `vless, trojan,
   anytls, ss, hysteria2, vmess, tuic` outbounds, plus `wireguard` via the
   modern `endpoints[]` block. Transports: `tcp, ws, grpc, http/h2, httpupgrade`.
2. **xray core** (`proxy-core/xray/generator.go`) — the endpoints sing-box can't
   speak: `vless` over `xhttp`/`splithttp`/`raw` (Reality or TLS), and `mtproxy`.
3. **sidecars** (`proxy-core/sidecars/`) — a per-protocol helper container, each
   with its own SOCKS5: MasterDNS `:5300`, dnstt `:5301`, Slipstream `:5302`,
   Psiphon `:5400`, Tor `:9150`, AmneziaWG `:5500`, TrustTunnel `:5600`.

## Parity table

| Server protocol | Client connects? | Via | Notes / gaps |
|---|:---:|---|---|
| Reality (VLESS) | ✅ | sing-box (vless+reality) or xray (reality+xhttp) | full |
| Trojan | ✅ | sing-box `trojan` | full |
| AnyTLS | ✅ | sing-box `anytls` (utls=random) | full |
| Hysteria2 | ✅ | sing-box `hysteria2` (+obfs) | full |
| **Shadowsocks-2022** | ✅ | sing-box `shadowsocks` | 2022-blake3 method strings pass through verbatim (guard-tested) |
| XHTTP (VLESS+XHTTP+Reality) | ✅ | xray `xhttpSettings` | sing-box deliberately rejects xhttp → routed to xray |
| CDN (VLESS+WS via Cloudflare) | ✅ | sing-box `ws` transport | full |
| WireGuard | ✅ | sing-box `endpoints[]` wireguard | full |
| telemt (Telegram MTProxy) | ✅ | xray `mtproto` outbound | parses tg:// / mtproxy:// / t.me/proxy |
| AmneziaWG | ✅ | sidecar `:5500` | needs the user's `awg0.conf` (from bundle); sing-box can't dial AWG |
| MasterDNS | ✅ | sidecar `:5300` | bundle `masterdns-instructions.txt` → `writeMasterDNS` |
| TrustTunnel | ✅ | sidecar `:5600` | bundle `trusttunnel.toml` → `writeTrustTunnel` |
| Conduit (Psiphon) | 🟡 | sidecar `:5400` | Psiphon connectivity works; ships anonymous channel, not the in-app "Conduit" proxy specifically |
| dnstt | ❌ | — | **GAP (bigger than it looks): the `dns-tunnels` sidecar Dockerfile is a mislabeled *MasterDNS* copy** (runs the `masterdns` binary, header comment and all), not a dnstt client. Needs a real dnstt-client sidecar container + a `writeDNSTT` config generator — feature work, not a configgen one-liner |
| Slipstream | — | — | **Removed** the dead stub in this PR (it had no compose service and dialed a nonexistent `slipstream:5302`). Re-add as a real sidecar if demand warrants |
| wstunnel (WG-over-wss) | ❌ | — | **GAP: no client path anywhere** |
| XDNS (Xray mKCP over DNS) | ❌ | — | **GAP: no client path anywhere** |
| GooseRelay | ❌ | — | **GAP: no client path anywhere** |
| Snowflake | ❌ | — | **GAP: no PT wiring** — a Tor sidecar exists (`:9150`) but no Snowflake pluggable-transport plumbing |

Client-only extras (supported by the client, not in the server roster): **VMess**,
**TUIC**, plain **Tor**.

## Summary

- **13 protocols at full parity**, 1 partial (Conduit/Psiphon).
- **5 hard gaps** with no client path: **wstunnel, XDNS, GooseRelay, Snowflake,
  dnstt** (its sidecar is a mislabeled MasterDNS copy — see the table).
- **Slipstream** dead stub was **removed** in this PR.

## Recommended follow-ups (feature work — separate cards)

1. **Build a real dnstt sidecar** — the `dns-tunnels` container currently runs the
   MasterDNS binary, not dnstt. Needs a proper dnstt-client image + a `writeDNSTT`
   generator (domain + server pubkey + DoH resolver from the bundle). Then it's a
   real 14th protocol.
2. ~~Finish or remove Slipstream~~ — **done** (removed; re-add as a real sidecar if
   demand warrants).
3. **wstunnel client** — WireGuard-over-`wss://` with the per-install upgrade-path
   secret the server now emits (server #139). Highest-value gap for censored nets.
4. **XDNS / GooseRelay / Snowflake** — evaluate demand; each needs a dedicated
   client transport or sidecar.

## Version note

`proxy-core/go.mod` is still `github.com/ibeezhan/moav-client/proxy-core` (the
pre-org owner). Rename to the `MotherofallVPNs` path as part of the v2.0.0 cut
(invasive — its own change, not this PR).
