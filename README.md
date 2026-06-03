# moav-client

![Go](https://img.shields.io/badge/Go-1.25-blue?logo=go) ![License: MIT](https://img.shields.io/badge/License-MIT-green)

A client for **[MoaV — Mother of all VPNs](https://github.com/shayanb/MoaV)** servers. It ingests a multi-protocol subscription bundle, delegates real protocol cryptography to sing-box plus a stack of optional sidecars (MasterDNS, AmneziaWG, Psiphon, TrustTunnel, Tor), latency-probes every endpoint end-to-end through its tunnel, load-balances across the healthy set, and exposes a single local SOCKS5 / HTTP CONNECT proxy. A React dashboard styled to match the MoaV admin panel gives live visibility into endpoint health, per-protocol throughput, plugin rule editing, and a streaming debug log.

---

## Quick start

```bash
git clone https://github.com/ibeezhan/moav-client
cd moav-client

# Drop your MoaV bundle into data/, then point config.yaml at it.
cp config.yaml.example config.yaml
$EDITOR config.yaml

docker compose up -d
# Add sidecars on demand:
docker compose --profile masterdns up -d
docker compose --profile amneziawg up -d   # needs NET_ADMIN + /dev/net/tun
docker compose --profile psiphon   up -d   # needs a Psiphon-issued config
docker compose --profile trusttunnel up -d

# Dashboard:    http://localhost:3001
# SOCKS5 proxy: localhost:1080
# HTTP proxy:   localhost:8081
# REST API:     localhost:8088
```

Point your browser or system proxy at `socks5h://localhost:1080`. Every connection is routed through the healthiest moav server endpoint.

---

## Supported protocols

The bundle parser accepts the standard MoaV subscription format (base64-encoded V2Ray-style URIs) plus optional WireGuard `.conf` files alongside.

| Protocol | Dial path | Notes |
|---|---|---|
| VLESS / Reality | sing-box outbound | utls fingerprint, Reality pbk + sid |
| VLESS + WS + TLS (CDN) | sing-box outbound | utls + ALPN + path / host |
| Trojan + TLS | sing-box outbound | uTLS fingerprint, SNI |
| Shadowsocks-2022 | sing-box outbound | 2022-blake3-aes-128-gcm |
| Hysteria 2 (+obfs) | sing-box outbound | salamander obfs |
| VLESS + XHTTP + Reality | _skipped_ | xhttp is Xray-only; sing-box can't speak it |
| WireGuard | sing-box `endpoints[]` | parsed from `wireguard.conf` |
| AmneziaWG | `amneziawg` sidecar | userspace `amneziawg-go` + `awg setconf` + microsocks on awg0 default route |
| TrustTunnel | `trusttunnel` sidecar | placeholder — mount the upstream client binary to activate |
| MasterDNS | `masterdns` sidecar | upstream binary from `masterking32/MasterDnsVPN` releases |
| Psiphon | `psiphon` sidecar | builds `Psiphon-Labs/psiphon-tunnel-core` from source; needs Psiphon-issued credentials |
| Tor / Arti | `tor` sidecar | `torproject/arti:latest` |

Every sidecar exposes its own SOCKS5 inbound on the `moav-net` Docker network; moav-client treats each as one entry in the balancer pool.

---

## Web dashboard

| Tab | What you can do |
|---|---|
| **Endpoints** | Live status & latency. Toggle each on/off (sidecar toggles also stop/start the docker container). Edit priority inline. |
| **Analytics** | Per-protocol upload/download cards with rolling 2-min sparklines, a stacked-area throughput chart of all protocols, per-endpoint table with dial / error / failover counts and last-error reason. |
| **Plugins** | List, reorder, edit, enable/disable, delete routing rules. Add from a curated template catalog (LAN-direct, IR geoip proxy, BitTorrent trackers, ad domains, telemetry, port-80 block, Anthropic direct) — all disabled by default. Changes hot-apply, no restart. |
| **Settings** | Switch load-balancing strategy live (latency / priority / weighted random). "Probe all endpoints now" button. |
| **Debug** | Streaming log tail (server-side ring buffer of last 500 events). Level chips (info / warn / error) with counts, substring filter, pause / autoscroll / copy / clear. |
| **Config** | Live-loads `config.yaml` from disk. Edit + save (writes back atomically). "Restart proxy-core to apply" notice for structural changes. |

A `↻ Refresh` button in the topbar reloads every tab in place; the health pill next to it shows `healthy/total`.

---

## Config reference

`config.yaml` controls every aspect of the client. Defaults are set by `config.Defaults()` in `proxy-core/config/config.go`; the file below shows every supported key.

```yaml
proxy:
  socks5_port: 1080
  http_port: 8080
  api_port: 8088
  auth:
    username: ""        # optional SOCKS5 auth
    password: ""

subscription:
  url: ""                              # V2Ray-style subscription URL (base64 or plain)
  file: "./data/<bundle>/subscription.txt"
  wireguard_files:                     # WG / AmneziaWG .conf paths become endpoints
    - "./data/<bundle>/wireguard.conf"
    - "./data/<bundle>/amneziawg.conf"

load_balancing:
  strategy: latency                    # latency | priority | weighted
  probe_on_start: true

plugins:
  torrent_block: true
  routing_rules:                       # see "Plugins" below
    - {match: {type: geoip, value: ir},       action: proxy}
    - {match: {type: ip_cidr, value: 10.0.0.0/8}, action: direct}

# sing-box does the real protocol cryptography. moav-client generates
# data/singbox.json from the subscription and rewrites Config["socks5_addr"]
# so the balancer dials through sing-box's local port.
singbox:
  enabled: true
  listen_host: "0.0.0.0"
  dial_host: "singbox"                 # docker-compose service name
  base_port: 10800
  output_path: "data/singbox.json"

sidecars:
  masterdns:                           # m.t7d.my MoaV DNS tunnel
    enabled: true
    priority: 2
    config:
      domain: "m.<your-bundle>.<tld>"
      method: "5"                      # 5 = AES-256-GCM
      key: "<hex encryption key>"
  amneziawg:
    enabled: true
    priority: 5
    config:
      source_path: "./data/<bundle>/amneziawg.conf"
  trusttunnel:
    enabled: false
    priority: 5
    config:
      source_path: "./data/<bundle>/trusttunnel.toml"
  psiphon:
    enabled: false
    priority: 8
    config:
      # Either: paste a verbatim Psiphon-issued ConsoleClient config:
      #   config_json: |
      #     { "PropagationChannelId": "...", "SponsorId": "...", ... }
      # Or: drop in the individual keys (see "Psiphon" below).
      client_platform: "Linux_moav-client"
  tor:
    enabled: false
  dnstt:
    enabled: false                     # legacy, superseded by masterdns
  slipstream:
    enabled: false
```

---

## Plugins

First-match-wins rule chain. Both `config.yaml` and the dashboard Plugins tab feed the same engine; changes from the dashboard hot-apply.

Match types: `domain`, `domain_suffix`, `domain_keyword`, `ip_cidr`, `geoip`, `port`, `protocol`.
Actions: `proxy` (default — go through the balancer), `direct` (bypass), `block` (drop).

Curated templates ship with the binary and surface in the dashboard's `+ from template…` picker — all rules land disabled so you can review before enabling:

- `lan-direct` — direct dial for RFC1918 / loopback / link-local CIDRs (practically required when SOCKS5 is set system-wide)
- `ir-geo-proxy` — force IR geo CIDRs through the proxy
- `block-known-trackers` — hard-block public BitTorrent trackers
- `block-ad-networks` — conservative ad / tracking domain list
- `block-telemetry` — opt-out telemetry endpoints (MS, Mozilla, JetBrains)
- `force-tls-only` — block plain HTTP (port 80)
- `direct-anthropic` — direct dial Anthropic / Claude APIs

### GeoIP

`matchGeoIP` reads `geoip/<cc>.txt` (one CIDR per line). For production use replace with `github.com/oschwald/maxminddb-golang` against a GeoLite2 db.

---

## Psiphon

Psiphon ConsoleClient builds from source as part of the `psiphon` sidecar (Go 1.26 base image). The container exposes SOCKS5 on `psiphon:5400` inside the Docker network so the prober reaches it.

**Actual tunneling requires Psiphon-issued credentials.** Until then the endpoint stays `status=error` and the balancer rolls over — same failover path as any other unhealthy endpoint.

Two ways to provide credentials:

```yaml
# Best — verbatim config from Psiphon Inc.:
sidecars:
  psiphon:
    enabled: true
    config:
      config_json: |
        { "PropagationChannelId": "...", "SponsorId": "...",
          "RemoteServerListUrls": [{"URL": "..."}], ... }

# Or — individual keys merged with safe defaults:
      propagation_channel_id: "<hex>"
      sponsor_id:             "<hex>"
      remote_server_list_signature_public_key: "<base64 RSA pubkey>"
      remote_server_list_url:               "<base64 url>"
      obfuscated_server_list_root_url:      "<base64 url>"
```

Sources: [psiphon.ca/en/license.html](https://psiphon.ca/en/license.html), or extracted from an official Psiphon Pro Android / iOS build (`psiphon_config` resource).

---

## CLI

`moav-client` ships as a single binary. Subcommands:

```
moav-client [command] [flags]

Commands:
  serve       Start the proxy + API (default if no command given)
  probe       One-shot latency probe of all endpoints
  list        List endpoints without probing
  fetch-sub   Fetch and parse a subscription URL
  version     Print version
  help        Print usage

Global flags:
  --config    Path to config.yaml  (default: config.yaml)
```

---

## REST API

The API server listens on `proxy.api_port` (default 8088). Responses are JSON; all routes accept permissive CORS for the dashboard.

| Method | Path | Description |
|---|---|---|
| GET | `/api/healthz` | liveness — `{"ok":true}` |
| GET | `/api/endpoints` | current pool with status / latency / config |
| PATCH | `/api/endpoints/<id>` | `{enabled, priority}` — patches the endpoint, also stops/starts the docker container for sidecars (if the docker socket is mounted) |
| POST | `/api/probe` | trigger an immediate probe pass |
| GET | `/api/stats` | per-endpoint counters (dials, errors, failovers, bytes_up/down, last_error) + active strategy |
| POST | `/api/strategy` | switch load-balancing strategy at runtime |
| GET | `/api/plugins` | `{rules, templates}` |
| PUT | `/api/plugins` | atomic rule-list replace |
| GET | `/api/logs` | log ring buffer; optional `?level=` filter |
| GET | `/api/config` | `{path, yaml}` — actual on-disk config |
| POST | `/api/config` | atomic write back to disk |
| WS | `/api/ws` | multiplexes `endpoints` and `log` frames |

---

## Internal architecture

For implementation details (sing-box bridge, balancer failover, prober tunnel-aware semantics, sidecar config generation, docker control), see **[docs/INTERNALS.md](docs/INTERNALS.md)**. There's also an LLM agent guide at **[CLAUDE.md](CLAUDE.md)**.

---

## Bundle format

To compress one server's full protocol surface into a single transferable URL instead of N protocol-specific URIs, see the proposal at **[docs/MOAV_BUNDLE.md](docs/MOAV_BUNDLE.md)** and tracking issue [#1](https://github.com/ibeezhan/moav-client/issues/1).

---

## Development

### Run proxy-core locally (no docker)

```bash
cd proxy-core
go run . --config ../config.yaml
```

### Run web-ui locally

```bash
cd web-ui
npm install
npm run dev
# Vite dev server at http://localhost:5173
# Default API target: http://localhost:8088 (override with VITE_API_URL)
```

### Tests

```bash
cd proxy-core && go test ./...
cd web-ui && npm run build  # type-check + bundle
```

---

## License

MIT — see [LICENSE](LICENSE).
