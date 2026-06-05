# Changelog

All notable changes to moav-client are documented here. Format loosely follows
[Keep a Changelog](https://keepachangelog.com/); versions are [SemVer](https://semver.org/).

## [1.0.0] — 2026-06-05

First public release. A local client for [MoaV](https://github.com/shayanb/MoaV)
servers: ingests a multi-protocol subscription bundle, delegates protocol
cryptography to sing-box + xray and a stack of optional sidecars, probes every
endpoint end-to-end, load-balances across the healthy set, and exposes one
local SOCKS5 / HTTP CONNECT proxy with a React dashboard.

### Protocols
- VLESS/Reality, VLESS+WS+TLS (CDN), Trojan, Shadowsocks-2022, Hysteria2 via
  sing-box; VLESS+XHTTP+Reality via xray; WireGuard via sing-box.
- Sidecars (opt-in): AmneziaWG, MasterDNS, Psiphon (embedded config — tunnels
  out of the box), Tor, and TrustTunnel (upstream prebuilt client, SOCKS5 mode).

### Routing & balancing
- Load-balancing strategies: latency, priority, weighted. Priority and the
  enabled/disabled toggle are honored per-request.
- Plugin rule engine (first-match-wins): domain / domain_suffix /
  domain_keyword / ip_cidr / geoip / port / protocol → proxy / direct / block.
- `geoip:<cc>` rules backed by `geoip/<cc>.txt` CIDR lists (Iran shipped),
  refreshed weekly by CI from RIPE + arastu sources.
- `block_direct` kill-switch: drop anything that would go direct (a `direct`
  rule or the all-endpoints-down fallback) so the host never leaks an
  unproxied connection.

### Dashboard & API
- Tabs: Endpoints, Sources (drop-in `.zip` bundle import), Analytics, Plugins,
  Settings (strategy / network exposure / SNI-spoof / backup-restore),
  Debug (per-level log rings + per-connection flows), Diagnostics, Config.
- REST + WebSocket API covering endpoints, probe, stats, strategy, plugins,
  sources, exposure, diagnostics, backup/restore, version.

### Install & ops
- One-line installer (`install.sh`), headless flags, global `moav-client`
  management command.
- Footprint: ~313 MB core / ~945 MB full on disk; first install downloads
  ~190 MB (core) to ~810 MB (full stack).
