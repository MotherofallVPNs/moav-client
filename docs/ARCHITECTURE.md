# moav-client — Architecture

Technical reference for the proxy-core internals: the sing-box/xray dialer
bridge, balancer + failover, the tunnel-aware prober, sidecar config
generation, and docker control. Useful for integration, debugging, or
extending the client.

---

## Two dialer bridges (sing-box and xray)

moav-client splits real protocol cryptography across two upstream dialers so
each one only has to do what it's good at:

- **sing-box** speaks VLESS / Reality / Trojan / SS-2022 / Hy2 / WireGuard.
  Generator: `proxy-core/singbox/generator.go`. Inbound port range: `10800+`.
- **xray** speaks Xray-only transports — currently `xhttp`, `splithttp`, `raw`.
  Generator: `proxy-core/xray/generator.go`. Inbound port range: `11800+`. The
  image is the official XTLS release binary (`sidecars/xray/Dockerfile`, pinned
  via `XRAY_VERSION`), not a third-party image.

The two generators are mutually exclusive: `xray.HandlesEndpoint(ep)` only
matches endpoints sing-box would have rejected, so each parsed endpoint is
claimed by exactly one of them. main.go runs sing-box's `Generate` first
(handles the common case), then xray's `Generate` over the leftovers; the
balancer just dials whichever `socks5_addr` the generator pinned and doesn't
need to know which dialer is on the other side.

Both sidecars wait for their config file before launching
(`/etc/sing-box/singbox.json` and `/etc/xray/xray.json`), so there's no
circular dependency with proxy-core. If a config file is never written
(e.g. xray.enabled=false, or no xhttp endpoints in the bundle), the sidecar
just idles in the wait loop.

---

## sing-box dialer bridge

File: `proxy-core/singbox/generator.go`

`Generate(eps, Config)` converts each parsed `subscription.Endpoint` into a
sing-box outbound block (`vless` w/ Reality + utls, `trojan`, `shadowsocks`,
`hysteria2` w/ obfs, `vmess`, `tuic`) and pairs it with a SOCKS5 inbound on
`Config.ListenHost:Config.BasePort+i` plus a 1:1 route rule. The returned
endpoint slice has `Config["socks5_addr"]` populated with `DialHost:port` —
which is what `balancer.dialThrough` and `prober.ProbeOne` use to reach the
endpoint via sing-box.

Endpoints whose **transport** is Xray-only (`xhttp`, `splithttp`, `raw`,
unknown) are returned unchanged — no `socks5_addr` — so the balancer's
legacy path tries to SOCKS5 the upstream directly (which usually fails and
trips the failover loop). They still get probed via raw TCP to ep.Address.

`main.go` writes the generated JSON atomically (`os.WriteFile` to `.tmp` +
`os.Rename`) to `cfg.Singbox.OutputPath` (default `data/singbox.json`). The
sing-box sidecar container waits for that file before launching
(`docker-compose.yml` entrypoint loop).

There is no circular dependency: sing-box's entrypoint spins on
`[ -s /etc/sing-box/singbox.json ]` so proxy-core can start first, drop the
file, and sing-box will pick it up within one second.

---

## Balancer failover (multi-attempt)

File: `proxy-core/balancer/balancer.go`

`DialContext` tries up to `maxDialAttempts` (4) different live endpoints before
falling back to direct dial. `pickExcluding(map[id]struct{})` mirrors `Pick()`
but skips IDs that already failed this call — preventing the same broken peer
(e.g. Reality with a server-side handshake bug) from being re-picked just
because its latency is lowest.

On each dial failure: `markError(ep.ID)` flips its status to "error"
immediately (write lock), so concurrent SOCKS5 connections seen during the
retry window also skip it. The next background probe (30 s) restores it if
the underlying issue resolves.

---

## Prober — real tunnel latency

File: `proxy-core/prober/prober.go`

When `Config["socks5_addr"]` is set, the probe is a SOCKS5 CONNECT through
that address to `1.1.1.1:443` (overridable via `Prober.Target`). The measured
latency is wall-clock for the whole chain: client → sing-box inbound → moav
server → 1.1.1.1. This is what surfaces Reality handshake breakage as
`status=error`. The fallback (no `socks5_addr`) is the previous raw TCP
connect against `ep.Address`.

---

## Sidecar config generation

File: `proxy-core/sidecars/configgen.go`

`SidecarManager.GenerateConfigs(baseDir)` writes per-sidecar config files
under `baseDir` (default `data/sidecar-configs/`) on every serve startup.
The free-form `config:` map declared per-sidecar in `config.yaml` carries
the secrets / paths needed:

- **masterdns**: emits `masterdns/client_config.toml` (with `LISTEN_PORT=5300`)
  and a small `masterdns/client_resolvers.txt` of public 53/udp resolvers.
- **amneziawg**: copies the file at `config.source_path` (a wg-quick / Amnezia
  `.conf`) to `amneziawg/awg0.conf`. The sidecar's `entrypoint.sh` reads it,
  strips wg-quick-only directives (Address/DNS/MTU), feeds the rest to
  `awg setconf awg0`, brings up the interface, adds a `/32` host route for
  the wg peer through the original eth0 gateway, swings the default route
  to awg0, and starts `microsocks` on `:5500`. NET_ADMIN + `/dev/net/tun`
  are required (set in docker-compose).
- **trusttunnel**: copies the file at `config.source_path` to
  `trusttunnel/client.toml`. The sidecar is a stub — it waits for a real
  `trusttunnel-client` binary to be mounted at `/usr/local/bin/` because
  the upstream project has no public Linux build yet.
- **psiphon**: writes either the verbatim `config_json` blob or a minimal
  default to `psiphon/psiphon.config`. The sidecar builds the
  `Psiphon-Labs/psiphon-tunnel-core` ConsoleClient from source.

Each sidecar's docker-compose entry mounts the matching subdirectory at the
container's expected path (e.g. `./data/sidecar-configs/masterdns` →
`/etc/masterdns`). Sidecars are gated by docker-compose profiles
(`--profile masterdns`, etc.) so `docker compose up` without flags starts
only the core stack.

---

## WebSocket broadcast

File: `proxy-core/api/api.go`

The API keeps a hub of subscriber channels (`clients map[chan []byte]struct{}`
under `hubMu`). On connect to `/api/ws`, the handler registers a buffered
channel and immediately sends the current endpoint list, then forwards each
broadcast frame to the socket; on disconnect the channel is removed.

`broadcast(eps)` marshals the pool to JSON and fans out with a non-blocking
`select { case ch <- data: default: }`, so a slow client drops a frame rather
than stalling the broadcast. It fires after an API-triggered probe
(`POST /api/probe`). The periodic background probe updates the balancer pool
(`SetEndpoints`); connected clients see those changes on the next probe
broadcast or a dashboard refresh.

---

## Sidecar endpoints

File: `proxy-core/sidecars/manager.go`

`EnabledEndpoints()` turns each enabled sidecar into a synthetic balancer
endpoint with `Config["socks5_addr"] = "<service>:<port>"` (the docker-compose
service name on `moav-net`) and `Config["sidecar_kind"] = <name>`, so
`balancer.dialThrough` and `prober.ProbeOne` reach it like any other endpoint.

| Sidecar | Address (moav-net) | Default priority |
|---|---|---|
| masterdns | `masterdns:5300` | 1 (highest) |
| psiphon | `psiphon:5400` | 5 |
| amneziawg | `amneziawg:5500` | 5 |
| trusttunnel | `trusttunnel:5600` | 5 |
| tor | `tor:9150` | 5 |

`Priority` feeds the `priority` strategy (ascending — masterdns first) and the
`weighted` strategy (weight = priority).

---

## State persistence

File: `proxy-core/state/state.go`

`State` struct:
```go
type State struct {
    LastProbeAt time.Time              `json:"last_probe_at"`
    Endpoints   []subscription.Endpoint `json:"endpoints"`
}
```

**What is saved**: The full `Endpoint` slice including `LatencyMs`, `Status`, and all `Config` map entries. `RawURI` is the restore key in `main.go`.

**Atomic write**: `Save(path)` writes to `path + ".tmp"` first, then `os.Rename(tmp, path)`. On POSIX systems, rename is atomic — a crash between write and rename leaves the old file intact. On Windows, rename may fail if the destination exists; this is not handled.

**Restore logic** (`main.go`): After subscription parsing, a `stateByURI map[string]Endpoint` is built from saved state. For each newly parsed endpoint, if `stateByURI[ep.RawURI]` exists, its `LatencyMs` and `Status` are copied in. This gives the balancer an initial view of endpoint health without waiting for a probe.

**State path**: Hardcoded as `"data/state.json"` in `main.go`. Docker Compose mounts `./data` as a volume so state survives container restarts.

---

## Docker network & ports

File: `docker-compose.yml`

All services join the `moav-net` bridge and address each other by service name
(e.g. `proxy-core:8088`, `singbox:10800`, `xray:11800`, `masterdns:5300`).

Host-exposed ports — the bind address is `.env`-driven (`127.0.0.1` by default,
`0.0.0.0` for LAN/public via the Network exposure setting):

| Service | Host | Container | Purpose |
|---|---|---|---|
| proxy-core | 1080 | 1080 | SOCKS5 |
| proxy-core | 8081 | 8080 | HTTP CONNECT |
| proxy-core | 8088 | 8088 | REST / WebSocket API |
| web-ui | 3001 | 3000 | dashboard (nginx) |

The dashboard talks to the API same-origin — nginx reverse-proxies `/api` to
`proxy-core`, so the browser only needs `:3001`. For local dev
(`npm run dev`), the API target defaults to `http://localhost:8088` (override
with `VITE_API_URL`).

---

## Docker Compose profiles

`proxy-core`, `web-ui`, `singbox`, and `xray` have no profile and always start;
optional services are gated so a bare `docker compose up` runs only the core
stack.

| Profile | Service | Image / build |
|---|---|---|
| `masterdns` | masterdns | `./sidecars/dns-tunnels` |
| `amneziawg` | amneziawg | `./sidecars/amneziawg` |
| `psiphon` | psiphon | `./sidecars/psiphon` |
| `trusttunnel` | trusttunnel | `./sidecars/trusttunnel` |
| `tor` | tor | `peterdavehello/tor-socks-proxy` |
| `sni-spoof` | sni-spoof | `./sidecars/sni-spoof` |
| `https` | caddy | `caddy:2` (HTTPS termination) |
| `all-sidecars` | all of the above | — |

```bash
docker compose --profile tor up -d        # core + Tor
moavc sidecar add tor               # the wrapper does this for you
```

Named volumes (`psiphon-data`, `caddy-data`, `caddy-config`) persist across
restarts; `./data` is bind-mounted for config, state, and generated sidecar
configs.
