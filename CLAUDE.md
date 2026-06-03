# moav-client — LLM Agent Guide

## 1. Project overview

moav-client is the local client for [**MoaV — Mother of all VPNs**](https://github.com/shayanb/MoaV), written in Go with a React/TypeScript dark-themed web dashboard styled after the MoaV admin panel. It ingests a multi-protocol MoaV subscription bundle (base64-encoded V2Ray-style URIs plus optional WireGuard/AmneziaWG `.conf` files), parses each line into a typed `Endpoint`, delegates the protocol cryptography to a sing-box sidecar (one SOCKS5 inbound per endpoint) plus an optional stack of additional sidecars (MasterDNS, AmneziaWG, Psiphon, TrustTunnel, Tor), probes every endpoint end-to-end through its tunnel to measure real latency, then exposes a SOCKS5 listener and an HTTP CONNECT listener that route outbound connections through the best live endpoint as selected by a pluggable load-balancing strategy. A hot-swappable routing-rule engine and a torrent-traffic blocker allow per-host or per-port decisions (proxy / direct / block) to be applied before the balancer picks an upstream. A REST + WebSocket API drives the dashboard, which shows live endpoint health and per-protocol throughput, lets the user toggle endpoints / adjust priorities (and stop/start the corresponding sidecar container via the docker socket), edit routing rules in place, view a streaming log tail, and edit `config.yaml` from the browser.

---

## 2. Architecture map

| Path | Purpose |
|---|---|
| `proxy-core/` | Go binary — the main service (SOCKS5, HTTP CONNECT, API, probing, balancing) |
| `proxy-core/api/` | REST + WebSocket API server consumed by the web-ui |
| `proxy-core/balancer/` | Load balancer: strategies latency / priority / weighted; dials upstreams via SOCKS5 |
| `proxy-core/cmd/` | CLI subcommands: serve, probe, list, fetch-sub, version, help |
| `proxy-core/config/` | YAML config loader using `gopkg.in/yaml.v3`; defines all config structs |
| `proxy-core/plugins/` | Plugin engine (first-match-wins rule list) + TorrentBlocker heuristic |
| `proxy-core/prober/` | Concurrent TCP latency prober with background loop; 10 parallel goroutines max |
| `proxy-core/proxy/` | SOCKS5 listener (`armon/go-socks5`) and HTTP CONNECT handler; both call `pluginDecide` then `balancer.DialContext` |
| `proxy-core/sidecars/` | Maps enabled sidecar entries (masterdns / amneziawg / trusttunnel / psiphon / tor / dnstt / slipstream) to synthetic `Endpoint` structs with `socks5_addr` set to `<docker-service>:<port>` + writes per-sidecar config files (`configgen.go`) from `config.yaml` parameters |
| `proxy-core/singbox/` | Generates a sing-box config (1 SOCKS5 inbound + 1 protocol outbound per endpoint, plus 1 `endpoints[]` block per WireGuard endpoint) and rewrites `Endpoint.Config["socks5_addr"]` to point at the local sing-box port |
| `proxy-core/state/` | Atomic JSON persistence of probe results to `data/state.json` |
| `proxy-core/subscription/` | URI parsers for vless/vmess/trojan/ss/hysteria2/wireguard/tuic; base64-subscription decoder; HTTP fetcher |
| `proxy-core/dockerctl/` | Engine-API client over `/var/run/docker.sock` — used by `handleEndpointPatch` to stop/start sidecar containers when the user toggles them in the dashboard |
| `proxy-core/logbus/` | Pub/sub log bus + level classifier (info / warn / error) + ring buffer; mirrors every `log.Printf` for the Debug tab |
| `web-ui/src/` | React 18 dark dashboard (Vite + TypeScript) styled after the MoaV admin panel: `App.tsx` (tabs + topbar refresh), `theme.ts` (palette), `components/EndpointTable.tsx` (toggle/priority controls), `Analytics.tsx` (per-protocol stacked area + sparklines), `Plugins.tsx` (hot-swap rule editor + template catalog), `Settings.tsx`, `Debug.tsx` (filtered log tail), `ConfigEditor.tsx` (loads on-disk YAML) |

---

## 3. Key data flows

### Startup
1. `main.go` parses `--config` flag, dispatches via `cmd.ParseAndRun()`.
2. `config.Load()` reads `config.yaml` with yaml.v3; defaults applied first.
3. `state.Load("data/state.json")` — restores `LatencyMs` / `Status` from previous run; non-fatal on missing file.
4. Subscription endpoints loaded: file first (`os.ReadFile` → `subscription.ParseSubscription`), then URL (`subscription.FetchSubscription`). Duplicates are deduped by `RawURI`.
5. Saved state merged into newly parsed endpoints (restores latency without re-probing).
6. `sidecars.SidecarManager.EnabledEndpoints()` generates synthetic `Endpoint` structs for each enabled sidecar.
7. If `singbox.enabled: true`, `singbox.Generate(endpoints, cfg)` is called: writes `data/singbox.json` atomically and replaces the endpoint slice with one whose `Config["socks5_addr"]` points at the sing-box service (e.g. `singbox:10800`). Endpoints whose transport sing-box cannot speak (xhttp etc.) are returned unchanged.
8. All endpoints passed to `balancer.SetEndpoints()`.
9. If `load_balancing.probe_on_start: true`, `prober.ProbeAll()` runs concurrently in a goroutine; on completion `balancer.SetEndpoints(updated)` and `state.Save()` are called. The background `prober.Run(ctx, eps)` loop then starts, probing every 30 s.
10. Plugin engine and TorrentBlocker constructed from config.
11. `proxy.NewServer`, `api.New` created; all three servers (`ListenAndServeSOCKS5`, `ListenAndServeHTTP`, `ListenAndServe`) started in goroutines.

### Connection (SOCKS5 or HTTP CONNECT)
1. Client connects on `:1080` (SOCKS5) or `:8080` (HTTP CONNECT).
2. `server.pluginDecide(host, port, network)` / `handler.decide(host, port, proto)`:
   a. `TorrentBlocker.Match()` checked first — if true, `DecisionBlock`.
   b. `Engine.Evaluate()` iterates rules in order (first-match-wins) — returns `DecisionBlock`, `DecisionDirect`, or `DecisionProxy`.
3. On `DecisionBlock`: connection closed immediately.
4. On `DecisionDirect`: `net.Dial` directly to destination.
5. On `DecisionProxy`: `balancer.DialContext(network, addr)` called.
   - Tries up to `maxDialAttempts` (4) different endpoints. `pickExcluding(triedIDs)` selects the best remaining one by strategy.
   - `dialThrough(ep, network, addr)`: if `Config["socks5_addr"]` is set (sing-box or sidecar), dials via SOCKS5 to that local port. Otherwise: `sidecar` → `127.0.0.1:1080`; vless/vmess/trojan/ss/tuic → legacy SOCKS5 to `ep.Address`; hysteria2/wireguard → error.
   - On dial failure, `markError(ep.ID)` flips status to "error" and the loop picks the next-best endpoint. If all attempts fail, falls back to direct dial.
6. Bidirectional `io.Copy` tunnel established.

### Probe (triggered via API or background loop)
1. `POST /api/probe` → goroutine calls `prober.ProbeAll(eps)`.
2. `ProbeAll` fans out up to 10 concurrent goroutines; each calls `ProbeOne(ep)`.
3. `ProbeOne`: if `Config["socks5_addr"]` is set, sends a SOCKS5 CONNECT through it to `Prober.Target` (default `1.1.1.1:443`) — measures end-to-end tunnel latency. Otherwise raw `tcpConnect(ep.Address, timeout)` (or `127.0.0.1:1080` for sidecars). Sets `LatencyMs` and `Status` ("ok" / "timeout" / "error").
4. `balancer.SetEndpoints(updated)` atomically replaces the pool.
5. `api.Server.broadcast(updated)` marshals endpoints to JSON and fans out to all connected WebSocket clients via buffered channels (slow clients are skipped).
6. `state.Save()` writes `data/state.json` atomically (write to `.tmp` → `os.Rename`).

### Config update
1. `POST /api/config` with JSON body.
2. `handleConfig` merges keys into `s.config` (a `map[string]interface{}`).
3. The change is in-memory only — listeners read config at startup and are not restarted. A process restart is required to apply structural changes (ports, subscription URL, plugin rules).

---

## 4. Endpoint struct

```go
// proxy-core/subscription/parser.go
type Endpoint struct {
    ID        string            // deterministic: "<protocol>:<host:port>"
    Protocol  string            // vless | vmess | trojan | ss | hysteria2 | wireguard | tuic | sidecar
    Name      string            // human-readable label from URI fragment (#name)
    Address   string            // "host:port" of the remote server
    RawURI    string            // original URI; used as dedup key
    Config    map[string]string // protocol-specific fields (uuid, password, sni, etc.)
    Priority  int               // lower = higher priority (used by priority and weighted strategies)
    Enabled   bool              // false disables selection by Pick(); set true on parse
    LatencyMs int64             // TCP connect time in ms; -1 means not yet probed
    Status    string            // "unknown" | "ok" | "timeout" | "error"
}
```

Field notes:
- `Config` keys vary by protocol. Common keys: `uuid`, `password`, `sni`, `net` (transport), `security`, `flow`, `pbk` (public key for reality), `method` (SS cipher), `auth` (hysteria2 token), `socks5_addr` (sidecar override).
- `Status` drives `Pick()`: only `"ok"` endpoints are selected.
- `Priority` is used by the `priority` strategy (ascending sort) and as weight in the `weighted` strategy.
- Sidecar endpoints set `Config["socks5_addr"]` = `"127.0.0.1:<port>"` for `dialThrough`.

---

## 4a. REST + WebSocket API

| Endpoint | Method | Purpose |
|---|---|---|
| `/api/healthz`   | GET    | liveness probe (`{"ok":true}`) |
| `/api/endpoints` | GET    | current endpoint pool with status/latency/config |
| `/api/probe`     | POST   | trigger immediate probe pass; updates broadcast over WS |
| `/api/stats`     | GET    | per-endpoint cumulative counters (dials, errors, failovers, bytes_up/down, last_error) + active strategy |
| `/api/strategy`  | POST   | switch load-balancing strategy at runtime — body `{"strategy":"latency"\|"priority"\|"weighted"}` |
| `/api/config`    | GET/POST | in-memory free-form config blob (not parsed yet — see §7) |
| `/api/ws`        | WS     | streaming endpoint updates (every probe pass) |

All responses include permissive CORS so the dashboard on a different port can fetch directly.

---

## 5. Config reference

All fields live in `config.yaml`. Defaults are set by `config.Defaults()`.

```
proxy:
  socks5_port: 1080        # int  — SOCKS5 listener port (default 1080)
  http_port: 8080          # int  — HTTP CONNECT listener port (default 8080)
  api_port: 8088           # int  — REST/WebSocket API port (default 8088)
  auth:
    username: ""           # string — SOCKS5 auth username; empty = no auth
    password: ""           # string — SOCKS5 auth password

subscription:
  url: ""                  # string — V2Ray subscription URL (base64 or plain)
  file: ""                 # string — local subscription file path

load_balancing:
  strategy: latency        # string — "latency" | "priority" | "weighted" (default "latency")
  probe_on_start: true     # bool   — run ProbeAll at startup (default true)

plugins:
  torrent_block: false     # bool   — enable TorrentBlocker heuristic
  routing_rules:           # ordered list; first match wins
    - match:
        type: domain       # match type; see §6
        value: example.com
      action: direct       # "direct" | "block" | "proxy" (default for no match)

subscription:
  wireguard_files: []      # list of wg-quick / AmneziaWG .conf paths; each
                           # becomes one endpoint. Files with Jc/Jmin/Jmax/S1
                           # etc. are tagged protocol=amneziawg and routed via
                           # the amneziawg sidecar rather than sing-box.

sidecars:
  masterdns:
    enabled: false         # SOCKS5 on masterdns:5300 (docker service name)
    priority: 1
    config:
      domain: ""           # tunnel domain (e.g. "m.t7d.my")
      method: "5"          # encryption method id (5 = AES-256-GCM)
      key: ""              # encryption key (hex)
  amneziawg:
    enabled: false         # SOCKS5 on amneziawg:5500 (sidecar with awg0 default route)
    priority: 5
    config:
      source_path: ""      # path to .conf file (consumed by configgen.go on startup)
  trusttunnel:
    enabled: false         # SOCKS5 on trusttunnel:5600 (needs upstream binary mounted)
    priority: 5
    config:
      source_path: ""      # path to client.toml (consumed by configgen.go)
  psiphon:
    enabled: false         # SOCKS5 on psiphon:5400 (built from psiphon-tunnel-core source)
    priority: 5
    config:
      config_json: ""      # verbatim Psiphon config blob (PropagationChannelId, SponsorId, …)
  dnstt:
    enabled: false         # legacy entry (use masterdns for the active MoaV tunnel)
  slipstream:
    enabled: false
  tor:
    enabled: false

singbox:
  enabled: true            # bool   — generate data/singbox.json and route endpoints through the sing-box sidecar
  listen_host: "0.0.0.0"   # string — what sing-box binds its SOCKS5 inbounds to
  dial_host: "singbox"     # string — what proxy-core dials (docker service name; "127.0.0.1" for host-mode)
  base_port: 10800         # int    — first port; endpoint i listens on base_port+i
  output_path: "data/singbox.json"  # string — atomic-written config consumed by the sidecar
```

---

## 6. Plugin engine

### Rule evaluation (`plugins/engine.go`, `plugins/routing.go`)

`Engine.Evaluate(host, port, protocolHint)` iterates `Engine.Rules` in order. First rule where `matchExpr()` returns true determines the `Decision`. If no rule matches, `DecisionProxy` is returned.

**Match types** (`matchExpr` switch):
| type | semantics |
|---|---|
| `domain` | case-insensitive exact match against host |
| `domain_suffix` | matches host == value OR host ends with "." + value |
| `domain_keyword` | case-insensitive substring in host |
| `ip_cidr` | host must be a parseable IP that falls within the CIDR |
| `geoip` | reads `geoip/<cc>.txt` (one CIDR per line); matches if IP is in any listed CIDR |
| `port` | exact port number ("443") or inclusive range ("1000-2000") |
| `protocol` | case-insensitive match of protocolHint ("tcp", "udp") |

**Decision constants**: `DecisionProxy = 0`, `DecisionDirect = 1`, `DecisionBlock = 2`.

### TorrentBlocker integration (`plugins/torrent.go`)

`TorrentBlocker.Match(host, port, proto)` is called by `server.pluginDecide` and `handler.decide` **before** the Engine, so it overrides all routing rules. It fires when:
1. Port is a known BitTorrent port (6881–6889, 51413), OR
2. Host matches a known tracker domain (exact or subdomain), OR
3. Port is 80/443 or protocolHint is "udp", AND host contains "torrent" or "tracker".

### Router (multi-match AND logic, `plugins/routing.go`)

`Router` is a separate type that allows a rule to specify multiple `MatchExpr` values, all of which must match (AND semantics). The current `main.go` wiring uses the simpler `Engine` (single-expr rules), not `Router`.

---

## 7. Known limitations / TODOs

1. **Native protocol dialing**: Real VLESS/VMess/Trojan/Shadowsocks/Hysteria2/TUIC client cryptography is delegated to **sing-box** (sidecar). `singbox.Generate` produces one outbound per endpoint and rewrites `Endpoint.Config["socks5_addr"]` so the Go balancer dials through the local sing-box port instead. WireGuard is still unimplemented; if `singbox.enabled: false`, the balancer falls back to dialing `ep.Address` as a SOCKS5 server (legacy mode).

2. **xhttp / splithttp transports**: sing-box does not speak Xray's `xhttp` transport, so VLESS+xhttp endpoints get no `socks5_addr` and the balancer's legacy SOCKS5-to-upstream path fails for them. The failover loop absorbs this — they just never get used.

3. **GeoIP**: `matchGeoIP` is a file-based stub that reads CIDRs from `geoip/<cc>.txt`. No MaxMind mmdb integration. For production, replace with `github.com/oschwald/maxminddb-golang`.

4. **WebSocket library**: Uses `golang.org/x/net/websocket` (older stdlib-adjacent package). Consider migrating to `gorilla/websocket` for production use (ping/pong, more control).

5. **Config hot-reload**: `POST /api/config` updates an in-memory `map[string]interface{}` only; it does not parse or apply values. Listeners, plugin rules, and subscription settings use the config loaded at startup. A process restart is required for structural changes.

6. **No TLS**: The API server, SOCKS5 listener, and HTTP CONNECT listener have no TLS. The web-ui dev server (`npm run dev`) also runs plain HTTP.

7. **Plain HTTP forwarding**: `handler.go` only handles `CONNECT`. Non-CONNECT HTTP requests return 405.

8. **Weighted strategy uses Priority as weight**: `weightedRandom` treats `ep.Priority` as the weight integer. Zero-priority endpoints get weight 1.

---

## 8. Build and run locally (without Docker)

```bash
# Backend
cd proxy-core
go build -o moav-client .
./moav-client serve --config ../config.yaml

# Frontend (separate terminal)
cd web-ui
npm install
npm run dev
# Opens http://localhost:5173; proxies API calls to http://localhost:8088 (see vite.config.ts)
```

Binary subcommands:
```bash
./moav-client list --config config.yaml            # list parsed endpoints
./moav-client probe --config config.yaml           # probe all, print table
./moav-client probe --config config.yaml --json    # probe all, output JSON
./moav-client fetch-sub <url>                      # fetch + parse subscription URL
./moav-client version
```

---

## 9. Run tests

```bash
# All tests
cd proxy-core && go test ./...

# Plugin engine tests (verbose)
cd proxy-core && go test ./plugins/... -v

# Subscription parser tests (verbose)
cd proxy-core && go test ./subscription/... -v

# Web-UI TypeScript type-check + build
cd web-ui && npm run build
```

---

## 10. Common agent tasks

### Add a new CLI subcommand
Add a case to the `switch os.Args[1]` block in `cmd/cli.go` `ParseAndRun()`. Implement a `runXxx(globalConfig *string)` function in the same file following the pattern of `runProbe` / `runList`.

### Add a new match type to the plugin engine
Add a case to the `switch m.Type` block in `plugins/routing.go` `matchExpr()`. Add a corresponding test case in `plugins/engine_test.go`.

### Add a new protocol to the subscription parser
Add a `case strings.HasPrefix(uri, "newproto://"):` branch in `subscription/parser.go` `ParseURI()`. Implement a `parseNewProto(uri string) (Endpoint, error)` function in the same file. Add test coverage in `subscription/parser_test.go`.

### Add a new API endpoint
1. Add a handler method to `api/api.go` (e.g. `func (s *Server) handleFoo(w http.ResponseWriter, r *http.Request)`).
2. Register it in `ListenAndServe`: `mux.HandleFunc("/api/foo", s.handleFoo)`.
3. If the endpoint needs balancer access, it is already available via `s.balancer`.
