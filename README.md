# moav-client

![Go](https://img.shields.io/badge/Go-1.25-blue?logo=go) ![License: MIT](https://img.shields.io/badge/License-MIT-green)

English | **[فارسی](README-fa.md)**

A client for **[MoaV — Mother of all VPNs](https://github.com/shayanb/MoaV)** servers. It ingests a multi-protocol subscription bundle, delegates real protocol cryptography to sing-box plus a stack of optional sidecars (MasterDNS, AmneziaWG, Psiphon, TrustTunnel, Tor), latency-probes every endpoint end-to-end through its tunnel, load-balances across the healthy set, and exposes a single local SOCKS5 / HTTP CONNECT proxy. A React dashboard styled to match the MoaV admin panel gives live visibility into endpoint health, per-protocol throughput, plugin rule editing, and a streaming debug log.

---

## Quick start

**One-liner install** (recommended):

```bash
curl -fsSL https://raw.githubusercontent.com/MotherofallVPNs/moav-client/main/install.sh | bash
```

The installer checks prerequisites, clones the repo, walks you through enabling sidecars (with disk-size estimates per choice), seeds `config.yaml`, builds the docker images, and brings the stack up. Works both interactively (TTY) and fully headless (env vars / flags).

**Headless examples:**

```bash
# Drive everything from env (no prompts).
MOAV_HEADLESS=1 \
MOAV_DIR=/opt/moav-client \
MOAV_SUBSCRIPTION=/etc/moav/subscription.txt \
MOAV_SIDECARS=masterdns,psiphon \
  bash -c "$(curl -fsSL https://raw.githubusercontent.com/MotherofallVPNs/moav-client/main/install.sh)"

# Or via flags after a local clone.
git clone https://github.com/MotherofallVPNs/moav-client && cd moav-client
./install.sh --headless --dir /opt/moav-client --sidecars masterdns,psiphon
```

After install, `./moav-client` is a thin docker-compose wrapper:

```bash
./moav-client status                # docker compose ps
./moav-client logs proxy-core       # tail logs
./moav-client probe                 # trigger probe via API
./moav-client stats                 # /api/stats JSON
./moav-client sidecar add tor       # enable + build + start the tor sidecar
./moav-client sidecar remove psiphon
./moav-client update                # git pull + rebuild + restart
```

Endpoints exposed:

| What | Address |
|---|---|
| Dashboard | http://localhost:3001 |
| SOCKS5 proxy | `socks5h://localhost:1080` |
| HTTP CONNECT | http://localhost:8081 |
| REST + WS API | http://localhost:8088 |

Point your browser or system proxy at `socks5h://localhost:1080`. Every connection routes through the healthiest moav server endpoint.

### Resource footprint per container

On-disk image sizes (measured on Ubuntu 24.04 / amd64). "Network" is what the
first install fetches: pulled images download compressed layers; built images
compile locally but pull a base image (golang / debian / node) — those base
layers are shared across the built containers, so the real total is far less
than the sum.

| Service | On-disk image | Network (first run) | Comes up |
|---|---|---|---|
| **proxy-core** | ~18 MB | builds locally (Go on scratch; golang-alpine base) | always |
| **web-ui** | ~75 MB | builds locally (Vite build → nginx:alpine, ~94 MB base) | always |
| **sing-box** | ~116 MB | ~50 MB pull (ghcr.io/sagernet/sing-box) | always |
| **xray** | ~104 MB | ~45 MB pull (teddysun/xray — xhttp/splithttp + MTProxy) | always |
| MasterDNS | ~138 MB | builds locally (golang + debian) | `--profile masterdns` |
| AmneziaWG | ~149 MB | builds locally (golang + debian) | `--profile amneziawg` |
| Psiphon | ~176 MB | builds locally (clones psiphon-tunnel-core) | `--profile psiphon` |
| TrustTunnel | ~85 MB | builds locally (placeholder) | `--profile trusttunnel` |
| Tor | ~85 MB | ~30 MB pull (peterdavehello/tor-socks-proxy) | `--profile tor` |

Core stack (always on): **~313 MB** on disk. Full stack with every sidecar:
**~945 MB**, plus ~500 MB of build cache on first run. A fresh install pulls
roughly 600–800 MB of base + runtime layers, most of it shared across the
built sidecars (the golang/debian build stages are downloaded once).

RAM: the core stack idles around ~150 MB; each sidecar adds 20–80 MB. 1 GB is
comfortable for core-only; 2 GB if you enable several sidecars.

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
| VLESS + XHTTP + Reality | xray outbound | xhttp is Xray-only; the xray sidecar handles it on 11800+ |
| WireGuard | sing-box `endpoints[]` | parsed from `wireguard.conf` |
| AmneziaWG | `amneziawg` sidecar | userspace `amneziawg-go` + `awg setconf` + microsocks on awg0 default route |
| TrustTunnel | `trusttunnel` sidecar | placeholder — mount the upstream client binary to activate |
| MasterDNS | `masterdns` sidecar | upstream binary from `masterking32/MasterDnsVPN` releases |
| Psiphon | `psiphon` sidecar | builds `Psiphon-Labs/psiphon-tunnel-core` from source; tunnels out of the box with its embedded config |
| Tor | `tor` sidecar | `peterdavehello/tor-socks-proxy` — SOCKS5 on :9150, no credentials |

Every sidecar exposes its own SOCKS5 inbound on the `moav-net` Docker network; moav-client treats each as one entry in the balancer pool.

---

## Web dashboard

| Tab | What you can do |
|---|---|
| **Endpoints** | Live status & latency. Toggle each on/off (sidecar toggles also stop/start the docker container). Edit priority inline. Disabled rows show a `DISABLED` pill instead of a stale status. |
| **Sources** | Import another MoaV server's bundle by dropping its `.zip` — extracts under `data/<name>/` and appends a `subscription.sources` entry. List / remove configured sources; trigger a reload. |
| **Analytics** | Per-protocol upload/download cards with rolling 2-min sparklines, an overlay-area throughput chart of all protocols, per-endpoint table with dial / error / failover counts and last-error reason. |
| **Plugins** | List, reorder, edit, enable/disable, delete routing rules. Add from a curated template catalog (LAN-direct, IR geoip proxy, BitTorrent trackers, ad domains, telemetry, port-80 block, Anthropic direct) — all disabled by default. Changes hot-apply, no restart. |
| **Settings** | Switch load-balancing strategy live (latency / priority / weighted random), "Probe all endpoints now", **Network exposure** (loopback / LAN / public with optional SOCKS5 auth, written to `.env`), SNI-spoof toggle, and config backup / restore. |
| **Debug** | Streaming log tail (server-side per-level ring buffers, ~800 events each for info / warn / error so warnings aren't crowded out by info spam). Level chips with counts, substring filter, pause / autoscroll / copy / clear. Plus a per-connection flow table. |
| **Diagnostics** | Run a connectivity check from proxy-core itself: TCP connect, DNS lookup, or TCP-TTL traceroute — optionally *through* a chosen endpoint's tunnel, to tell "my router can't reach this host" from "this endpoint's tunnel is down". |
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
  masterdns:                           # m.<your-bundle>.<tld> MoaV DNS tunnel
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

## Known limitations

Things that are *not* bugs in moav-client but will show up as red rows in the dashboard until you act on them:

- **TrustTunnel has no public Linux client.** The sidecar Dockerfile accepts a binary mounted at `/usr/local/bin/trusttunnel-client` plus a `client.toml`. Until upstream ships one, this entry stays errored.
- **Tor container may report `unhealthy` while working.** `peterdavehello/tor-socks-proxy` ships its own healthcheck that fetches a Facebook `.onion` over the Tor circuit — strict, and slow/blocked on some networks. The SOCKS5 proxy on `:9150` works regardless; the probe in the dashboard is the authoritative signal.
- **Reality keypair validity is server-side.** If `pbk` / `sid` in the bundle no longer match the moav server's private key, you'll see `connection: EOF` (sing-box) or `received real certificate (potential MITM or redirection)` (xray) — the server is fall-through-proxying to the configured `dest` instead of completing the Reality handshake. moav-client's failover loop routes around it; if every Reality endpoint is broken simultaneously, ask the operator to rotate. See [shayanb/MoaV#115](https://github.com/shayanb/MoaV/issues/115).
- **Reality + XHTTP requires xray.** sing-box doesn't support Xray's `xhttp` / `splithttp` transports. The bundled xray sidecar handles them; if you want to slim the install, disable `xray.enabled` in `config.yaml` and those endpoints will go silent.
- **AmneziaWG needs container privileges.** The userspace `amneziawg-go` + `awg setconf` + microsocks chain requires `cap_add: NET_ADMIN` and `/dev/net/tun` (already set in `docker-compose.yml`). On strictly hardened hosts this may need a relaxed seccomp profile.

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

Psiphon ConsoleClient builds from source as part of the `psiphon` sidecar. The container exposes SOCKS5 on `psiphon:5400` inside the Docker network so the prober reaches it.

**It tunnels out of the box** — the sidecar ships an embedded config (valid all-`F` `PropagationChannelId` / `SponsorId` plus the correct remote-server-list signing key), so it bootstraps a Psiphon circuit with no user input. You only need to supply your own config to point at a private Psiphon network:

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
  healthcheck Probe the local API and exit 0/1 (container healthcheck; works on the scratch image)
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
| GET | `/api/version` | build version + commit, uptime, install/proxy egress IP + country (footer) |
| GET | `/api/endpoints` | current pool with status / latency / config |
| PATCH | `/api/endpoints/<id>` | `{enabled, priority}` — patches the endpoint, also stops/starts the docker container for sidecars (if the docker socket is mounted) |
| POST | `/api/probe` | trigger an immediate probe pass |
| GET | `/api/stats` | per-endpoint counters (dials, errors, failovers, bytes_up/down, last_error) + active strategy |
| POST | `/api/strategy` | switch load-balancing strategy at runtime |
| GET | `/api/flows` | recent per-connection flow records (dest, endpoint, bytes, result) |
| GET/PUT | `/api/plugins` | get `{rules, templates}` / atomic rule-list replace |
| GET | `/api/logs` | log ring buffer; optional `?level=` filter |
| GET/POST | `/api/config` | get / atomic write-back of on-disk `config.yaml` |
| POST | `/api/bundles` | multipart `.zip` upload → extract under `data/<name>/` + register a source |
| GET | `/api/sources` | list configured subscription sources |
| DELETE | `/api/sources/<name>` | remove a source from `config.yaml` |
| POST | `/api/sources/reload` | self-restart proxy-core to reload subscription state |
| GET/PUT | `/api/exposure` | bind policy (loopback / lan / public) + SOCKS5 auth → `.env` |
| GET/PUT | `/api/snispoof` | SNI-spoof enable + default fake SNI / uTLS |
| GET | `/api/diag` | `?type=tcp\|dns\|trace&target=…&via=<endpoint>` connectivity check |
| GET | `/api/backup` | download a tar.gz of config + sources |
| POST | `/api/restore` | restore from an uploaded backup tar.gz |
| WS | `/api/ws` | multiplexes `endpoints` and `log` frames |

---

## Internal architecture

For implementation details (sing-box bridge, balancer failover, prober tunnel-aware semantics, sidecar config generation, docker control), see **[docs/INTERNALS.md](docs/INTERNALS.md)**. There's also an LLM agent guide at **[CLAUDE.md](CLAUDE.md)**.

---

## Bundle format

To compress one server's full protocol surface into a single transferable URL instead of N protocol-specific URIs, see the proposal at **[docs/MOAV_BUNDLE.md](docs/MOAV_BUNDLE.md)** and tracking issue [#1](https://github.com/MotherofallVPNs/moav-client/issues/1).

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
