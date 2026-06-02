# moav-client — Internal Architecture Notes

Deep technical reference for agents doing integration, debugging, or extension work.

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

## WebSocket broadcast (channel-based fan-out)

File: `proxy-core/api/api.go`

The API server maintains a hub: `clients map[chan []byte]struct{}` protected by `hubMu sync.RWMutex`.

**Registration**: When a client connects to `/api/ws`, `handleWebSocket` creates a buffered channel (`make(chan []byte, 8)`), inserts it into `clients` under a write lock, then immediately sends the current endpoint list so the client does not need to wait for the next probe cycle. It then loops on `for msg := range ch`, forwarding each message to the WebSocket connection. On disconnect (ws send error or function return), the channel is deleted from the map under a write lock.

**Broadcast path**: `broadcast(eps []subscription.Endpoint)` marshals the endpoint list to JSON, acquires a read lock on `hubMu`, then ranges over all channels. It uses a non-blocking `select { case ch <- data: default: }` so a slow or disconnected client drops the message rather than blocking the broadcast goroutine.

**Trigger points**: `broadcast` is called from the goroutine spawned by `POST /api/probe` after `prober.ProbeAll` completes, and from the background probe loop in `main.go` indirectly through `balancer.SetEndpoints` → note that the background loop in `main.go` does NOT call `broadcast`; it only calls `balancer.SetEndpoints`. Clients subscribed to `/api/ws` only receive broadcast updates from the API-triggered probe, not from the background prober. This is a known gap — the background loop should call `apiServer.Broadcast(eps)` as well.

---

## Balancer failover

File: `proxy-core/balancer/balancer.go`

`Pick()` filters the endpoint pool to `live := []Endpoint{ep | ep.Enabled && ep.Status == "ok"}`. If `live` is empty, `ErrNoEndpoints` is returned.

`DialContext` wraps `Pick()` + `dialThrough()`. On any dial error:
1. `markError(ep.ID)` sets `endpoints[i].Status = "error"` (write lock). This immediately removes the endpoint from future `Pick()` calls.
2. `net.Dial(network, addr)` is called directly as a fallback — there is no retry-through-next-endpoint loop. If the direct dial also fails, the error is returned to the caller.

The endpoint status remains "error" until the next probe cycle resets it to "ok" or "timeout". There is no exponential backoff or circuit-breaker. A background probe every 30 s will restore "error" endpoints if they become reachable again.

---

## Sidecar port assignments

Defined in `proxy-core/sidecars/manager.go`:

| Sidecar | Local address | Priority |
|---|---|---|
| masterdns | 127.0.0.1:5300 | 1 (highest) |
| dnstt | 127.0.0.1:5301 | 5 |
| slipstream | 127.0.0.1:5302 | 5 |
| psiphon | 127.0.0.1:5400 | 5 |
| tor | 127.0.0.1:9050 | 5 |

These ports are hardcoded in `manager.go`. Sidecars must listen on exactly these SOCKS5 ports inside the Docker network. The `Priority` field is used by both the `priority` strategy (ascending sort → masterdns is tried first) and the `weighted` strategy (weight = Priority, so masterdns gets more traffic).

When `protocol == "sidecar"`, both `prober.ProbeOne` and `balancer.dialThrough` read `ep.Config["socks5_addr"]`; if absent they fall back to `"127.0.0.1:1080"`. The `SidecarManager` does not set `Config["socks5_addr"]` — so the fallback is used unless a caller explicitly sets it. Bug: `manager.go` should set `Config: map[string]string{"socks5_addr": addr}`.

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

## Docker network topology

File: `docker-compose.yml`

All services join the `moav-net` bridge network (driver: bridge, internal Docker network). Services communicate by service name as DNS (e.g. `proxy-core:8088`).

Host-exposed ports:
| Service | Host port | Container port | Protocol |
|---|---|---|---|
| proxy-core | 1080 | 1080 | SOCKS5 |
| proxy-core | 8080 | 8080 | HTTP CONNECT |
| proxy-core | 8088 | 8088 | REST/WebSocket API |
| web-ui | 3000 | 3000 | HTTP (served by nginx in prod image) |

The web-ui receives `VITE_API_URL=http://proxy-core:8088` at build time, so the built JS bakes in the Docker-internal hostname. For local dev (`npm run dev`), `VITE_API_URL` defaults to `http://localhost:8088` (see `EndpointTable.tsx` and `ProbeButton.tsx`).

Sidecar services (dns-tunnels, psiphon, tor) are on the same `moav-net` network but do **not** expose ports to the host. `proxy-core` reaches them at `127.0.0.1:<port>` — this only works when `proxy-core` and the sidecar are on the same host (or the sidecar is in the same Docker namespace). In Docker Compose, `127.0.0.1` inside `proxy-core` refers to the container's loopback, not the sidecar container. The sidecar addresses in `sidecars/manager.go` would need to be Docker service names (e.g. `dns-tunnels:5300`) for inter-container routing. This is a known integration gap.

---

## Docker Compose profiles

Profiles let operators enable optional sidecar services without running them by default:

```bash
# Run with Tor enabled
docker compose --profile tor up

# Run with dns-tunnels + psiphon
docker compose --profile dns-tunnels --profile psiphon up

# Run only core services (no sidecars)
docker compose up
```

Profile → service mapping:
| Profile | Service | Docker image |
|---|---|---|
| `dns-tunnels` | dns-tunnels | `./sidecars/dns-tunnels/Dockerfile` |
| `psiphon` | psiphon | `./sidecars/psiphon/Dockerfile` |
| `tor` | tor | `torproject/arti:latest` |

`proxy-core` and `web-ui` have no profile and always start.

To persist sidecar data across restarts, add named volumes to the relevant service in `docker-compose.yml`. None are currently configured.
