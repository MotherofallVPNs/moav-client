# Changelog

All notable changes to moav-client are documented here. Format loosely follows
[Keep a Changelog](https://keepachangelog.com/); versions are [SemVer](https://semver.org/).

## [Unreleased]

## [1.3.1] — 2026-06-09

### Added
- **`moavc`** — short official alias for `moav-client`, symlinked into `PATH` by
  the installer alongside the full name. `moav-client` keeps working.
- **Per-container resource limits** (`mem_limit` / `cpus`) on every service in
  `docker-compose.yml` — matching the MoaV server. Core idles at ~35 MB RAM,
  the full stack ~130 MB; caps range 128m–256m so a runaway can't eat the host.

### Fixed
- `moavc status` (and `up` / `down` / `logs` / `update`) no longer pass an empty
  argument to `docker compose` when no sidecars are enabled — the
  `"${args[@]:-}"` expansion injected an empty string, so `docker compose "" ps`
  ran and errored. Switched to the `${args[@]+"${args[@]}"}` idiom.
- **Image builds no longer fail on a `403` from the Go module proxy.** All
  Go-building images (proxy-core, sni-spoof, psiphon, amneziawg — xray already
  did) now use the resilient `GOPROXY=…|goproxy.cn|direct` + `GOSUMDB=off`, so a
  rate-limited Google CDN falls through instead of aborting the build.
- Dropped the stale "TrustTunnel is a placeholder / no public binary" notice —
  the sidecar builds the official upstream client; it just needs the bundle's
  `client.toml`.
- `moavc uninstall` / the update-rebuild prompt now actually show their
  question — a stray `2>/dev/null` was hiding `read -p`'s prompt (it writes to
  stderr), so they looked like they did nothing before "cancelled".

### Changed
- Network-exposure **on/off toggles** restyled as a sliding switch (track + knob
  + label) so they clearly read as actionable.
- Installer's build summary: `download` column header (was `down`) and the
  confirm prompt now says "press Enter to continue".
- `moavc update` now asks before the rebuild (`rebuild & restart now? [Y/n]`,
  default yes); answering no leaves the code pulled with a note that it applies
  after a rebuild.

## [1.3.0] — 2026-06-08

An installer + CLI usability pass, an official Xray image, and version pinning.

### Added
- **Cross-platform prerequisite auto-install** in `install.sh` — installs missing
  docker / git / curl / python3 (Linux via the OS package manager +
  <https://get.docker.com>, macOS via Homebrew). Headless/CI installs without
  prompting; interactive asks first (default yes). New `--yes`/`MOAV_ASSUME_YES`
  and `--no-docker-install`/`MOAV_NO_DOCKER_INSTALL`.
- **Numbered multi-select sidecar wizard** — pick `1 3`, `all`, or none; only the
  chosen images are built. Re-running pre-checks already-enabled sidecars.
- **End-of-install LAN exposure prompt** (with optional dashboard password) and a
  loud warning when the host IP is public (VPS/cloud), so "LAN" isn't mistaken for
  private.
- **Official Xray image** — `sidecars/xray/Dockerfile` builds the official XTLS
  release binary (pinned `XRAY_VERSION`, source-compile fallback), replacing the
  third-party `teddysun/xray`.
- **Version pinning** — a `VERSION` file as the single source of truth (stamped into
  the proxy-core binary and the dashboard footer); pulled images pinned via `.env`
  (`IMAGE_SINGBOX`, `IMAGE_TOR`, `IMAGE_CADDY`) and `XRAY_VERSION`, documented in
  `.env.example`.
- **Redesigned `moav-client` CLI** — `help`/`version` show the MoaV logo, version, and
  repo/site links; `status` is a formatted service table with endpoint health and
  access URLs; new `info` (URLs only), `install` (re-run the wizard), `expose`
  (loopback/lan/public), and `update -b <branch>` (test branches on a running box).
- Dashboard surfaces an actionable message when a protocol is enabled whose sidecar
  image was never built (`moav-client sidecar add <kind>`).

### Changed
- Installer is **interactive even under `curl … | bash`** (prompts via `/dev/tty`);
  only truly terminal-less runs go headless. A build & start confirmation was added,
  and a bash 4+ guard (re-exec under a newer bash, else a clear error).
- **Access & URLs** in the dashboard now uses the host you actually reached the page
  on (fixes the `<this-machine-LAN-IP>` placeholder on VPS/remote views), with an
  egress-IP fallback.
- Renamed the **Sources** tab to **Configs**; folded the standalone **Config** (YAML)
  editor into the bottom of **Settings** as a collapsible "advanced" section.

### Fixed
- Wizard/confirm prompts use readline (`read -e`), so arrow keys edit the line instead
  of injecting `^[[A`.
- Removed the outdated Psiphon "needs credentials" warning (it connects out-of-the-box
  via embedded config); added a MasterDNS "idle until configured" note instead.

## [1.2.0] — 2026-06-06

A dashboard-focused release: a real mobile UI, a separate dashboard login, live
network-exposure controls, and the ability to add sources without a bundle.

### Added
- **Dashboard login, separate from the proxy.** Optional admin auth
  (`MOAV_DASHBOARD_USER`/`PASS`) protecting the dashboard + API via nginx basic
  auth on a single origin (nginx reverse-proxies `/api` to proxy-core). The
  WebSocket uses a short-lived **ticket** fetched over an authenticated request,
  so iOS Safari no longer re-prompts on the Endpoints/Debug tabs. Stored
  passwords can be revealed in the form only once the panel itself is
  authenticated. Each auth section has an on/off toggle that clears its creds.
- **Network exposure controls in the dashboard** — switch loopback / LAN /
  public binding (writes `.env`), an **Apply now** button that restarts the
  dashboard + proxy (no terminal), and an **ACCESS & URLS** panel that shows the
  mode-relative address (127.0.0.1 / LAN IP / public host) using only local
  signals — no external IP lookup.
- **Block-direct kill-switch toggle** above the Endpoints table
  (`GET/PUT /api/block-direct`) — live to the engine + balancer, persisted to
  `config.yaml`; names any enabled `direct` rules that still bypass the proxy.
- **Add a source by pasting** a subscription URL or V2Ray URIs
  (`POST /api/sources`) — no bundle zip needed. Accepts any standard V2Ray
  config, not just MoaV.
- **Source component tags** (subscription / wireguard / the sidecar kinds a
  bundle configured) on the Sources tab, and **sidecar→bundle attribution** so
  imported sidecars show their originating bundle instead of "sidecars". MoaV
  bundle imports are marked with a **`moav`** badge / `moav/<bundle>` source
  label to distinguish them from pasted custom sources.
- **"Selective app" routing templates** — curated rule sets to keep specific
  apps off the VPN or block their background traffic, by destination domain
  (system updates → block; Zoom, iCloud, cloud sync, streaming, game downloads
  → direct). NOTE: this is destination-based, not true per-app tunneling (a
  SOCKS5/HTTP proxy can't see the originating process like TripMode/WireSock).
- **Routing rules persist to `config.yaml`** — dashboard rule edits
  (add/enable/disable/reorder) now survive a restart.
- **Comment-preserving config writes** — dashboard edits keep the comments,
  ordering and spacing in `config.yaml` (yaml.Node-based editing).
- **SNI-spoof Apply-now button**; **healthchecks for sing-box + xray**;
  **probe retry** before marking an endpoint unhealthy; **active tab persists**
  across refresh.

### Changed
- **Responsive dashboard.** Card layouts on phones for Endpoints, Sources,
  Analytics (per-endpoint), and the per-connection flows; pill tabs in a grid;
  reorganized header (title / status / actions); centered footer; Diagnostics
  inputs no longer overflow.
- **`block_direct` now honors explicit `direct` rules** — it drops only the
  balancer's *involuntary* fallback (all endpoints down); a deliberate `direct`
  rule takes priority.
- **Default routing** trimmed to a single `geoip:ir → direct`; `torrent_block`
  off; the redundant "Iran → proxy" template removed.
- **Bare endpoint names** — the Source column owns the bundle, so names render
  as just the label ("Hysteria2", "WireGuard", "MasterDNS").
- **SOCKS5 auth**: a password alone enables auth, username defaults to `moav`;
  proxy + dashboard auth re-read `.env` on restart so changes apply without a
  full recreate. Logs lead with the endpoint's friendly name. Sources show the
  remote URL + tags instead of the local file path.

### Fixed
- iOS Safari WebSocket re-auth prompt (ticket auth).
- Clipboard copy did nothing over plain HTTP on a LAN IP (insecure-context
  fallback added).
- Probe errors now classify as ERROR/WARN with their reason, and the TLS
  validation no longer inflates measured latency.
- API binds the same interface as the UI on lan/public, so the dashboard can
  reach it remotely; ACCESS & URLS refreshes after saving exposure.
- Blank Sources page when no sources were configured.
- `index.html` served no-cache so a fresh build loads (notably on mobile); the
  header health count is populated on every tab, not just Endpoints.

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
