# Plugins — routing rules & templates

moav-client decides what to do with every outbound connection **before** it
picks an upstream. The decision comes from a first-match-wins rule chain plus a
torrent heuristic and a kill-switch. Both `config.yaml` and the dashboard
**Plugins** tab feed the same engine; dashboard edits hot-apply (no restart) and
persist back to `config.yaml`.

## How a decision is made

For each connection (`host`, `port`, `protocol`), where `host` is the
destination exactly as the client sent it (see
[What the rules see](#what-the-rules-see)):

1. **TorrentBlocker** (if `plugins.torrent_block: true`) runs first — known
   BitTorrent ports / tracker domains / torrent keywords → **block**.
2. **Rule chain** — rules are evaluated top-to-bottom; the **first** enabled
   rule whose match expression matches wins.
3. **Hostname, no rule matched, IP rules present** — the name is resolved with
   proxy-core's local resolver and the rule chain runs again on its addresses,
   so `ip_cidr` / `geoip` / `ip_list` can still decide. See
   [Resolution order](#resolution-order).
4. **No match → `proxy`** (route through the balancer).

Decisions:

| Action | Effect |
|---|---|
| `proxy` | Route through the balancer to the best healthy endpoint (the default). |
| `direct` | Dial the destination directly, bypassing the VPN. |
| `block` | Drop the connection. |

Match types:

| Type | Matches |
|---|---|
| `domain` | exact host (case-insensitive; one trailing dot ignored) |
| `domain_suffix` | host == value **or** host ends with `.value` (covers subdomains; one trailing dot ignored) |
| `domain_keyword` | value appears anywhere in the host |
| `ip_cidr` | destination IP inside the CIDR (an IP literal, or a hostname's local answer — see below) |
| `geoip` | destination IP is in `geoip/<cc>.txt` (same as `ip_cidr`) |
| `port` | exact port (`443`) or inclusive range (`1000-2000`) |
| `protocol` | `tcp` / `udp` |

## What the rules see

Both listeners (SOCKS5 and HTTP CONNECT) pass the destination to the rules
exactly as the client sent it, and both use the same resolution order:

| Client | proxy-core receives | Rules that can match |
|---|---|---|
| SOCKS5 with remote DNS (`socks5h://`, the recommended setting) | the hostname | `domain*`, `port`, `protocol`; then `ip_cidr` / `geoip` on the local answer if none of those matched |
| SOCKS5 with client-side DNS (`socks5://`) | the IP the client resolved | `ip_cidr`, `geoip`, `port`, `protocol` |
| HTTP CONNECT | the host in the request, normally a hostname | same as the row that fits what was sent |
| Any client, destination typed as an IP | the IP literal | `ip_cidr`, `geoip`, `port`, `protocol` |

A hostname with a trailing dot (`example.com.`, the fully qualified form)
matches `domain`, `domain_suffix` and `domain_list` rules written without it.

### Resolution order

For a hostname, proxy-core decides in two passes, like Xray's
`IPIfNonMatch`:

1. **Name pass.** The rule chain runs on the hostname itself. `domain*`,
   `port` and `protocol` rules can match here; IP rules cannot. If a rule
   matches, that is the decision and **no lookup is made**.
2. **Address pass.** Only if no rule matched, *and* at least one enabled
   `ip_cidr`, `geoip` or `ip_list` rule exists, proxy-core resolves the name
   with its **local** resolver (the host's or container's DNS; never through a
   tunnel, 5 s limit) and runs the rule chain again on the addresses. The
   first rule, in list order, that matches any of them decides.
3. **Default.** No match in either pass (or the lookup failed) → `proxy`.

This happens before any connection is opened. It is what keeps in-country
traffic off the tunnel: with `geoip: ir → direct`, a `socks5h://` request for
a name that resolves to an Iranian address goes direct, and no MoaV endpoint
is ever asked to connect to it. Without the address pass, that name would go
through the tunnel, and the exit server would connect to a server inside Iran
from its own IP, revealing it.

Rule order still matters across the passes: a name rule anywhere in the list
beats an IP rule, because the name pass runs first. Put a `domain_suffix` rule
in only when you want it to win regardless of where the name resolves.

The local answer is used to classify and, for `direct`, to connect:

| Decision | What gets dialled |
|---|---|
| `proxy` | The endpoint gets the **hostname**, never the local answer, and resolves it itself (sing-box and Xray send it on to the server; a SOCKS5 sidecar resolves it its own way, e.g. AmneziaWG inside its container). A censored local resolver therefore cannot redirect proxied traffic. |
| `direct` from the name pass | proxy-core resolves the name and dials it. |
| `direct` from the address pass | proxy-core dials the addresses the deciding rule matched, reusing the lookup (no second query). If a name resolves to an Iranian and a foreign address, only the Iranian one is dialled direct. |
| `block` | Nothing. |

If no IP rules are enabled, no lookup is made for classification at all. If the
local lookup fails, the connection takes the default `proxy` route and the log
says so (`route: local lookup of … failed`).

If every endpoint is down and `block_direct` is off, the balancer's fallback
dial resolves the name locally, like `direct`.

The TorrentBlocker gets the same unresolved `host`, so its tracker-domain and
keyword checks apply on both listeners.

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
in-repo, refreshed weekly by CI). For a hostname it matches the local answer,
in the address pass of the [resolution order](#resolution-order). See
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
