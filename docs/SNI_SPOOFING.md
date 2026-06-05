# SNI spoofing

> Status: **implemented, needs further real-world testing.** The `sni-spoof`
> sidecar builds and the dial-path wiring is in place (Settings → SNI-spoof,
> or `sni_spoof` in `config.yaml`), but it has not been validated against a
> live DPI'd network at scale — treat it as experimental until it has.

## Upstream

| Project | Role |
|---|---|
| [aleskxyz/SNI-Spoofing-Go](https://github.com/aleskxyz/SNI-Spoofing-Go) | **Source of truth** — the Go implementation the `sni-spoof` sidecar builds from. Local TCP proxy that injects a fake-SNI (uTLS-fingerprinted) ClientHello, then forwards the real handshake. |
| [patterniha/SNI-Spoofing](https://github.com/patterniha/SNI-Spoofing) | Original IP/TCP-header manipulation approach; SNI-Spoofing-Go is the modernised port. Reference only. |

## What it is

DPI systems on hostile networks routinely block TLS handshakes by SNI: a
ClientHello with `server_name=blocked.example.com` is RST'd at the TCP
layer before any meaningful response. **SNI spoofing** sidesteps that by
sending a *first* ClientHello whose SNI is a known-good decoy (`hcaptcha.com`,
`update.windows.com`, etc.), letting the DPI's stateful inspector accept
the connection, then issuing the *real* TLS handshake against the actual
destination.

## Where it fits in moav-client

The technique is L4-adjacent and operates **before** any of moav's protocol
crypto. It's most useful for:

1. **VLESS+TLS, Trojan+TLS, VLESS+WS+TLS** — TLS-fronted endpoints where the
   real SNI is blocked but a decoy isn't.
2. **Reality** does *not* need this — Reality already disguises its
   ClientHello to match an arbitrary `dest` SNI. Adding a spoof on top is
   redundant.
3. **Hysteria2** uses QUIC. Spoofing-Go is TCP-only, so no help there.

## Two integration paths

### Path A — per-endpoint config knob + local spoofer sidecar (recommended)

A new docker-compose profile `sni-spoof` runs SNI-Spoofing-Go bound to a
range of localhost ports. Each spoofed endpoint pins
`Endpoint.Config["spoof_via"] = "sni-spoof:<port>"` (parallel to how we
already pin `socks5_addr`). The balancer's dial path layers it ahead of the
existing sing-box/xray hop:

```
moav-client SOCKS5 ─→ sni-spoof:<port> ─→ singbox:<port> ─→ moav server
                       (decoy CH)         (real VLESS+TLS)
```

Per-endpoint user config:

```yaml
subscription:
  sources:
    - name: server-A
      file: ./data/server-A/subscription.txt
      spoof:
        # Endpoints whose protocol matches one of these get a spoofed
        # decoy. Empty == no spoofing.
        trojan: { fake_sni: hcaptcha.com, utls: chrome }
        vless:  { fake_sni: update.windows.com, utls: firefox }
```

Implementation cost: small. moav-client generates the SNI-Spoofing-Go
config from the spoof section, mutates `Endpoint.Config["socks5_addr"]` to
the spoof port, and the rest of the dial chain is unchanged.

### Path B — sing-box `fake_sni` outbound option

sing-box has a `tls.fake_sni` (still gated behind a `with_quic` build flag
at the moment) for QUIC-based protocols. Adding it for the TCP outbounds
would be cleaner — no separate sidecar — but the upstream feature is
incomplete. Tracking issue in sing-box: [#TODO].

## Existing hooks already in place

- `singbox/generator.go` and `xray/generator.go` both honour
  `Endpoint.Config["fake_sni"]` if present in the parsed config — they just
  don't currently use it because no parser populates it. Implementing Path A
  is a 1-file generator addition + the sidecar.
- `Endpoint.Config["spoof_via"]` is reserved and not used by anything else,
  so adding it later doesn't conflict.

## Operational caveats

- Spoofing-Go on Linux / macOS / Windows needs root (it sets BPF / raw
  sockets). The sidecar container would need `cap_add: [NET_RAW]`.
- Decoy SNIs need to be plausibly *allowed* on the target network. There
  isn't a universal list — `hcaptcha.com` is a common pick.
- A spoofer is only helpful when DPI bans by SNI. Networks that ban by IP
  / port / encrypted fingerprint will see through it.
- Reality already does this style of disguise at the protocol level.
  Don't stack the two.

## Current state

- The `sni-spoof` sidecar (built from aleskxyz/SNI-Spoofing-Go) ships behind
  the `sni-spoof` compose profile.
- `snispoof/generator.go` writes its config from the `sni_spoof` block in
  `config.yaml` (or per-endpoint `fake_sni` / `utls`), and rewrites eligible
  endpoints' `Config["socks5_addr"]` to route through the spoofer ahead of the
  sing-box/xray hop. Reality / Shadowsocks / UDP-QUIC / sidecar endpoints are
  auto-excluded.
- Toggle it from **Settings → SNI-spoof** or `sni_spoof.enabled` in config.

## Needs further testing

This works in the lab but hasn't been proven on a live DPI'd network at scale.
Before relying on it: validate decoy-SNI acceptance on the target network,
confirm the uTLS fingerprint isn't itself flagged, and measure throughput
overhead of the extra local hop.
