# Plugins — routing rules & templates

moav-client decides what to do with every outbound connection **before** it
picks an upstream. The decision comes from a first-match-wins rule chain plus a
torrent heuristic and a kill-switch. Both `config.yaml` and the dashboard
**Plugins** tab feed the same engine; dashboard edits hot-apply (no restart) and
persist back to `config.yaml`.

## How a decision is made

For each connection (`host`, `port`, `protocol`):

1. **TorrentBlocker** (if `plugins.torrent_block: true`) runs first — known
   BitTorrent ports / tracker domains / torrent keywords → **block**.
2. **Rule chain** — rules are evaluated top-to-bottom; the **first** enabled
   rule whose match expression matches wins.
3. **No match → `proxy`** (route through the balancer).

Decisions:

| Action | Effect |
|---|---|
| `proxy` | Route through the balancer to the best healthy endpoint (the default). |
| `direct` | Dial the destination directly, bypassing the VPN. |
| `block` | Drop the connection. |

Match types:

| Type | Matches |
|---|---|
| `domain` | exact host (case-insensitive) |
| `domain_suffix` | host == value **or** host ends with `.value` (covers subdomains) |
| `domain_keyword` | value appears anywhere in the host |
| `ip_cidr` | destination is an IP literal inside the CIDR |
| `geoip` | destination IP is in `geoip/<cc>.txt` (IP-literals only — see below) |
| `port` | exact port (`443`) or inclusive range (`1000-2000`) |
| `protocol` | `tcp` / `udp` |

> **Hostname vs IP.** `ip_cidr` and `geoip` only match when the destination is
> an **IP literal**. With SOCKS5 remote DNS (`socks5h`), the proxy usually
> receives a hostname for domain targets, so use `domain_suffix` for those and
> reserve `ip_cidr`/`geoip` for IP-addressed traffic (LAN, geo CIDRs).

## Block-direct (kill-switch)

`plugins.block_direct: true` — also a toggle above the Endpoints table — drops
the balancer's **involuntary** direct fallback (the dial it would otherwise make
when *every* endpoint is down), so a downed pool can't leak your real IP.
Default `false`.

**Explicit `direct` rules always win** and are honored even with the kill-switch
on (e.g. `geoip:ir → direct` keeps Iranian destinations direct; `lan-direct`
keeps LAN access working). When the kill-switch is on and any `direct` rules are
enabled, the dashboard toggle names them, since that traffic still bypasses the
proxy. For a strict no-direct policy, turn the kill-switch on **and** disable
your `direct` rules.

## GeoIP

`geoip:<cc>` matches a destination IP against `geoip/<cc>.txt` (Iran ships
in-repo, refreshed weekly by CI). IP-only, as noted above. See
[../geoip/README.md](../geoip/README.md) for sources and adding countries.

## Templates

Curated rule sets in the dashboard's **`+ from template…`** picker. Every
template lands **disabled** so you can review (and edit) before enabling; each
rule's action is editable (flip `block`↔`direct`, add/remove domains), and the
result persists to `config.yaml`.

### Networking / privacy

| Template | Action | What it does |
|---|---|---|
| `lan-direct` | direct | RFC1918 / loopback / link-local CIDRs direct — practically required when SOCKS5 is system-wide so gateways, NAS, printers stay reachable. IP-literal matches. |
| `block-known-trackers` | block | Hard-blocks well-known public BitTorrent trackers (complements the `torrent_block` heuristic). |
| `block-ad-networks` | block | Conservative ad / tracking domain starter list (DoubleClick, Google Ads, …). |
| `block-telemetry` | block | Opt-out telemetry endpoints (Microsoft, Mozilla, JetBrains). |
| `force-tls-only` | block | Blocks plain HTTP (port 80). Also blocks plain-HTTP healthchecks. |
| `direct-anthropic` | direct | Sample: dial Anthropic / Claude APIs directly. |

### "Selective app" (keep apps off the VPN / block background traffic)

These approximate TripMode / WireSock per-app tunneling **by destination
domain** — see the caveat below.

| Template | Action | Covers |
|---|---|---|
| `block-system-updates` | block | Apple (macOS/iOS) + Windows software-update CDNs. Saves VPN bandwidth. While on, the OS can't fetch updates *through this proxy* at all — flip to `direct` if you want updates to work but bypass the VPN. |
| `direct-zoom` | direct | Zoom meetings + the Zoom updater (`zoom.us`, `zoom.com`, `zoomgov.com`). Better call quality off-VPN. |
| `direct-icloud` | direct | iCloud / CloudKit / iCloud Drive sync. |
| `direct-cloud-sync` | direct | Dropbox, Google Drive, OneDrive. |
| `direct-streaming` | direct | Netflix, YouTube, Spotify. Note: forgoes geo-unblocking for these. |
| `direct-game-downloads` | direct | Steam, Epic, Blizzard content/downloads. |

> **This is destination-based, not true per-app tunneling.** moav-client is a
> SOCKS5/HTTP proxy — it sees *where* a connection is going, never *which app*
> opened it. Tools like [TripMode](https://tripmode.ch/) and
> [WireSock](https://www.wiresock.net/) run at the OS network layer (a TUN
> adapter / packet filter) where they can identify the originating process.
> These templates therefore work well for apps with distinctive backends (the
> ones above) but **cannot isolate an app that shares a CDN/domain** with apps
> you do want tunneled. Domain lists are curated starting points (verified
> against vendor firewall allowlists) — prune/extend them as you watch traffic
> in **Debug → per-connection flows**.

## config.yaml shape

```yaml
plugins:
  torrent_block: false
  block_direct: false
  routing_rules:
    - match: { type: domain_suffix, value: zoom.us }
      action: direct
      enabled: true          # absent = enabled (back-compat)
      note: Zoom off the VPN
    - match: { type: geoip, value: ir }
      action: direct
```

## API

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/plugins` | `{ rules, templates }` for the Plugins tab |
| PUT | `/api/plugins` | atomic rule-list replace (applies live + persists to `config.yaml`) |
| GET/PUT | `/api/block-direct` | read / set the kill-switch (live + persisted) |
