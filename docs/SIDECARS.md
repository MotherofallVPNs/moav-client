# Sidecars

Optional protocol engines, each a container on the `moav-net` Docker network
exposing a SOCKS5 inbound that moav-client treats as one balancer endpoint.
Enable per-install with `--sidecars a,b,c` (or the dashboard); each is gated by
a docker-compose `--profile`.

| Sidecar | Port | Needs |
|---|---|---|
| MasterDNS | 5300 | bundle's `domain` + `key` (DNS tunnel to `m.<bundle>.<tld>`) |
| AmneziaWG | 5500 | a `.conf`; `NET_ADMIN` + `/dev/net/tun` (set in compose) |
| Psiphon | 5400 | nothing — embedded config tunnels out of the box |
| TrustTunnel | 5600 | bundle's `trusttunnel.toml` |
| Tor | 9150 | nothing |

Bundle import (Sources tab) auto-wires the config paths for MasterDNS,
AmneziaWG and TrustTunnel; you just flip the sidecar on.

## TrustTunnel

Runs the upstream prebuilt CLI client
([TrustTunnel/TrustTunnelClient](https://github.com/TrustTunnel/TrustTunnelClient),
Apache-2.0) in SOCKS5 mode. The bundle ships a `[listener.tun]` config; the
sidecar rewrites it to a loopback `[listener.socks]` and `socat`-republishes it
on `0.0.0.0:5600`, so no TUN device or `NET_ADMIN` is needed. Pin the client
version via the `TT_VERSION` build arg in `sidecars/trusttunnel/Dockerfile`.

## Tor

`peterdavehello/tor-socks-proxy`, SOCKS5 on `:9150`, no credentials. The
container's healthcheck is overridden with a port-open check (the image's
default fetches a Facebook `.onion`, which is slow/blocked on many networks).

## Psiphon

Psiphon ConsoleClient, built from
[Psiphon-Labs/psiphon-tunnel-core](https://github.com/Psiphon-Labs/psiphon-tunnel-core).
SOCKS5 on `:5400`.

**It tunnels out of the box** — the sidecar ships an embedded config (valid
all-`F` `PropagationChannelId` / `SponsorId` plus the correct remote-server-list
signing key), so it bootstraps a Psiphon circuit with no user input. Supply
your own config only to point at a private Psiphon network:

```yaml
sidecars:
  psiphon:
    enabled: true
    config:
      # Either: a verbatim Psiphon-issued ConsoleClient config:
      config_json: |
        { "PropagationChannelId": "...", "SponsorId": "...",
          "RemoteServerListUrls": [{"URL": "..."}], ... }
      # Or: individual keys merged with safe defaults:
      propagation_channel_id: "<hex>"
      sponsor_id:             "<hex>"
      remote_server_list_signature_public_key: "<base64 RSA pubkey>"
      remote_server_list_url:               "<base64 url>"
      obfuscated_server_list_root_url:      "<base64 url>"
```

Config sources: [psiphon.ca](https://psiphon.ca/en/license.html), or the
`psiphon_config` resource extracted from an official Psiphon Pro build.
