# moav-client

**MoaV Client** — a Dockerized, multi-protocol circumvention proxy client for [MoaV (Mirror of All VPNs)](https://github.com/moav). It aggregates subscription endpoints, load-balances across them using latency probing, and exposes a unified SOCKS5/HTTP CONNECT proxy plus a web dashboard.

## Quick Start

```bash
cp config.yaml.example config.yaml
# Edit config.yaml — add your subscription URL
docker compose up
```

| Service   | Address               |
|-----------|-----------------------|
| SOCKS5    | `localhost:1080`      |
| HTTP CONNECT | `localhost:8080`   |
| Web UI    | http://localhost:3000 |
| API       | http://localhost:8088 |

### Optional sidecars

```bash
docker compose --profile dns-tunnels up   # MasterDNS / dnstt
docker compose --profile psiphon up       # Psiphon
docker compose --profile tor up           # Tor (arti)
```

## Phase Roadmap

| Phase | Scope |
|-------|-------|
| **1** | Scaffold: proxy-core (Go), web-ui (React/Vite), sidecar placeholders, docker-compose |
| **2** | SOCKS5 + HTTP CONNECT forwarding to live endpoints; V2Ray subscription parser |
| **3** | Latency prober; priority/weighted balancing; health state machine |
| **4** | Web dashboard connects to API; real-time endpoint table; config editor |
| **5** | MasterDNS / dnstt sidecar integration; Psiphon SDK; Tor SOCKS bridge |
| **6** | Routing rules (geoip, domain blocklist); torrent block plugin |
| **7** | Packaging: single-binary mode, auto-update, Telegram bot alerts |

## Architecture

```
Browser / App
    │ SOCKS5 / HTTP CONNECT
    ▼
proxy-core (Go)
    ├── Balancer  ──► Endpoint pool (V2Ray, VLESS, Trojan, …)
    ├── Prober    ──► latency measurements
    └── API       ──► web-ui (WebSocket)

web-ui (React/Vite)
    └── Endpoint table, probe trigger, YAML config editor

Sidecars (opt-in via Docker profiles)
    ├── dns-tunnels  (MasterDNS / dnstt)
    ├── psiphon
    └── tor (arti)
```

## Requirements

- Docker 24+ with Compose v2
- (dev) Go 1.22+, Node 20+
