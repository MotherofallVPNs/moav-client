# moav-client

![Go](https://img.shields.io/badge/Go-1.22-blue?logo=go) ![License: MIT](https://img.shields.io/badge/License-MIT-green)

A client for [MoaV (Mirror of All VPNs)](https://github.com/moav) servers that aggregates multiple circumvention protocol endpoints from a subscription feed, load-balances across them using real-time latency probing, and exposes a single local SOCKS5/HTTP proxy to your applications. A built-in React web dashboard gives live visibility into endpoint health and lets you switch balancing strategies on the fly.

---

## Quick Start

```bash
# 1. Clone
git clone https://github.com/ibeezhan/moav-client
cd moav-client

# 2. Copy and edit config
cp config.yaml.example config.yaml
# Paste your MoaV subscription URL or bundle file path into config.yaml

# 3. Start
docker compose up -d

# Dashboard:    http://localhost:3000
# SOCKS5 proxy: localhost:1080
# HTTP proxy:   localhost:8080
```

Configure your browser or system proxy to `socks5://localhost:1080` (or `http://localhost:8080`).

---

## Config Reference

`config.yaml` controls every aspect of the client.

| Field | Type | Default | Description |
|---|---|---|---|
| `proxy.socks5_port` | int | `1080` | Local SOCKS5 listen port |
| `proxy.http_port` | int | `8080` | Local HTTP CONNECT listen port |
| `proxy.api_port` | int | `8088` | REST + WebSocket API port |
| `proxy.auth.username` | string | _(none)_ | Optional proxy auth username |
| `proxy.auth.password` | string | _(none)_ | Optional proxy auth password |
| `subscription.url` | string | _(none)_ | V2Ray-style subscription URL (base64 or plain) |
| `subscription.file` | string | _(none)_ | Local subscription bundle file path |
| `load_balancing.strategy` | string | `latency` | `latency` \| `priority` \| `weighted` |
| `load_balancing.probe_on_start` | bool | `true` | Run latency probes at startup |
| `plugins.torrent_block` | bool | `false` | Block BitTorrent tracker connections |
| `plugins.routing_rules` | list | _(none)_ | Per-domain / per-IP routing rules (see Plugins) |
| `sidecars.masterdns.enabled` | bool | `false` | Enable MasterDNS sidecar (port 5300) |
| `sidecars.dnstt.enabled` | bool | `false` | Enable dnstt DNS tunnel sidecar (port 5301) |
| `sidecars.slipstream.enabled` | bool | `false` | Enable Slipstream sidecar (port 5302) |
| `sidecars.psiphon.enabled` | bool | `false` | Enable Psiphon sidecar (port 5400) |
| `sidecars.tor.enabled` | bool | `false` | Enable Tor / Arti sidecar (port 9050) |

---

## Protocols Supported

| Protocol | Handling |
|---|---|
| VLESS | Native TCP dial via parsed URI |
| VMess | Native TCP dial via parsed URI |
| Trojan | Native TCP dial via parsed URI |
| Shadowsocks (`ss://`) | Native TCP dial via parsed URI |
| Hysteria 2 | Native TCP dial via parsed URI |
| WireGuard | Native UDP dial via parsed URI |
| TUIC | Native UDP dial via parsed URI |
| Sidecar | Forwarded to local sidecar port (see Sidecars) |

All endpoints are parsed from the standard V2Ray base64 subscription format. Lines that cannot be parsed are silently skipped.

---

## Plugins

Plugins run as a chain of middleware on every new connection, before the upstream is selected.

### Torrent blocking

```yaml
plugins:
  torrent_block: true
```

Blocks connections to well-known BitTorrent tracker hostnames and IP ranges. Useful when your upstream provider prohibits P2P traffic.

### Routing rules

Rules are evaluated top-to-bottom; the first match wins. Supported match types: `domain`, `ip_cidr`, `geoip`.

```yaml
plugins:
  routing_rules:
    # Send Iranian IPs through the proxy
    - match: {type: geoip, value: ir}
      action: proxy

    # Block a known tracker
    - match: {type: domain, value: tracker.thepiratebay.org}
      action: block

    # Send RFC-1918 traffic direct (no proxy)
    - match: {type: ip_cidr, value: 10.0.0.0/8}
      action: direct
```

Actions: `proxy` (use balancer), `direct` (bypass proxy), `block` (refuse connection).

### GeoIP

GeoIP data lives in the `geoip/` directory. The bundled stub maps a small set of country codes. Replace `geoip/geoip.mmdb` with a full MaxMind GeoLite2-Country database for production use.

---

## Sidecars

Sidecars are external processes that provide additional circumvention transports. Enable them in `config.yaml`; moav-client adds them to the endpoint pool automatically.

| Sidecar | Docker profile | Local port | Notes |
|---|---|---|---|
| MasterDNS | _(built-in)_ | 5300 | DNS-over-HTTPS / DNS-over-TLS resolver |
| dnstt | `dns-tunnels` | 5301 | DNS tunnel transport |
| Slipstream | _(built-in)_ | 5302 | TLS-based slipstream transport |
| Psiphon | `psiphon` | 5400 | Psiphon3 tunnelling |
| Tor / Arti | `tor` | 9050 | Standard Tor SOCKS5 |

Enable a sidecar Docker profile:

```bash
docker compose --profile psiphon up -d
docker compose --profile tor up -d
docker compose --profile dns-tunnels up -d
```

---

## CLI Reference

The `moav-client` binary accepts subcommands. When no subcommand is given it starts the full server (`serve`).

```
moav-client [command] [flags]

Commands:
  serve       Start the proxy + web UI (default)
  probe       One-shot latency probe of all endpoints
  list        List endpoints without probing
  fetch-sub   Fetch and parse a subscription URL
  version     Print version
  help        Print usage

Global flags:
  --config    Path to config.yaml  (default: config.yaml)
```

### Examples

```bash
# Start the server explicitly
moav-client serve --config /etc/moav/config.yaml

# Probe all endpoints, print as JSON
moav-client probe --json --timeout 15

# List endpoints from config
moav-client list

# Fetch and preview a remote subscription
moav-client fetch-sub https://example.com/sub?token=xxx

# Print version
moav-client version
```

---

## REST API

The API server listens on `proxy.api_port` (default 8088).

| Method | Path | Description |
|---|---|---|
| GET | `/api/healthz` | Health check — returns `{"status":"ok"}` |
| GET | `/api/endpoints` | List all endpoints with status/latency |
| POST | `/api/endpoints/{id}/enable` | Enable an endpoint |
| POST | `/api/endpoints/{id}/disable` | Disable an endpoint |
| GET | `/api/balancer` | Current strategy |
| PUT | `/api/balancer` | Change strategy |
| WS | `/ws` | Real-time endpoint updates |

---

## Development

### Run proxy-core locally

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
# Set VITE_API_URL=http://localhost:8088 in .env.local
```

### Run tests

```bash
cd proxy-core && go test ./...
cd web-ui && npm test
```

### Build all

```bash
cd proxy-core && go build ./...
cd web-ui && npm run build
```

---

## License

MIT — see [LICENSE](LICENSE).
