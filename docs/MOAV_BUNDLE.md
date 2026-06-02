# MoaV Bundle — single-blob multi-protocol config (proposal)

## Why

Today a MoaV subscription is N independent URIs — one per protocol — each
repeating the same server host, the same UUID for VLESS variants, the same
TLS cert chain, etc. For the `beezhan-t7d` server:

```
vless://ad7fb761-…@178.105.237.120:443?security=reality&sni=update.samsung.com&pbk=Lor2…&sid=1a2d…    (Reality)
vless://ad7fb761-…@cdn.t7d.my:443?security=tls&type=ws&path=/dl/v2/…&sni=t7d.my&host=cdn.t7d.my       (CDN/WS-TLS)
vless://ad7fb761-…@178.105.237.120:2096?type=xhttp&security=reality&sni=update.miui.com&pbk=Lor2…     (XHTTP)
trojan://XMJY9…@178.105.237.120:8443?security=tls&sni=t7d.my                                          (Trojan)
ss://2022-blake3-aes-128-gcm:Q7Dr…:Uz0J…@178.105.237.120:8388                                         (SS2022)
hysteria2://XMJY9…@178.105.237.120:443?sni=t7d.my&obfs=salamander&obfs-password=VOK…                  (Hy2)
```

That's ~1.5 KB base64 for one server. Bandwidth is fine, but it's also
**brittle**: rotating the VLESS UUID, the Trojan/Hy2 password, or the Reality
public key means regenerating six URIs that must all stay consistent. And
users importing the subscription into stripped-down clients (no V2RayNG)
don't get any signal about which entries are variants of the **same** server.

The proposal: a single canonical bundle format that variabilises one MoaV
server's full protocol surface into one transferable line.

## Format

```
moav://<userTag>@<defaultHost>?<shared>&p=<proto-spec>&p=<proto-spec>&…#<label>
```

Where:

- **`<userTag>`** — short user identifier (the part before `@`, mirrors VLESS).
  Optional; if absent, anonymous.
- **`<defaultHost>`** — the server's primary IP or domain. Each `p=` entry can
  override it; this gives the common case (most protocols live on the same
  host:port-family) a single source of truth.
- **`<shared>`** — query params shared across every protocol:
  - `uuid=…` (used by VLESS / VMess / TUIC)
  - `pw=…` (Trojan / Hy2 password)
  - `ss_method=2022-blake3-aes-128-gcm`
  - `ss_pw=…:…`
  - `pbk=…`, `sid=…` (Reality public key + short id, used by every reality variant)
  - `sni_default=…`, `fp=chrome`
- **`p=<proto-spec>`** — one or more protocol activations, each a tiny
  comma-separated record `name,port,sni?,transport?,…`. See examples.
- **`#<label>`** — human label (`MoaV-beezhan-t7d`).

### Compact protocol grammar

```
p = <name>,<port>[,k=v[,k=v…]]
name = reality | vless-ws | vless-xhttp | trojan | ss | hy2 | tuic | vmess
```

Per-name allowed `k=v` overrides (only specify what's not in `<shared>`):

| name        | extra keys                                         |
|-------------|----------------------------------------------------|
| reality     | `sni`, `flow=xtls-rprx-vision`                     |
| vless-ws    | `host`, `path`, `sni`, `alpn`                      |
| vless-xhttp | `sni`, `mode=auto`                                 |
| trojan      | `sni`                                              |
| ss          | (none — uses ss_method + ss_pw from shared)        |
| hy2         | `sni`, `obfs=salamander`, `obfs_pw`                |
| tuic        | `sni`, `cc=bbr`                                    |
| vmess       | `aid`, `host`, `path`, `net=ws`                    |

### Concrete example (this server, one line)

```
moav://beezhan-t7d@178.105.237.120?\
uuid=ad7fb761-656d-4c04-aae2-5f012e88d5fa&\
pw=XMJY9dIOSQw0cKdsxMhb5RyJ&\
ss_method=2022-blake3-aes-128-gcm&\
ss_pw=Q7Dr12JpM8qfWhIe3DPzqA%3D%3D%3AUz0JRokfhTwusBQAxpavag%3D%3D&\
pbk=Lor2HboQ7pzB2f_QsF7i8Q950RCHzG-95i3ED9w8bEc&\
sid=1a2d863d&\
sni_default=t7d.my&\
fp=chrome&\
p=reality,443,sni=update.samsung.com,flow=xtls-rprx-vision&\
p=vless-ws,443,host=cdn.t7d.my,path=/dl/v2/packages/retrieve/data-snapshot-79.pkg,sni=t7d.my,alpn=http/1.1&\
p=vless-xhttp,2096,sni=update.miui.com&\
p=trojan,8443,sni=t7d.my&\
p=ss,8388&\
p=hy2,443,sni=t7d.my,obfs=salamander,obfs_pw=VOKbsqrmI8UR9DMdS9AXLJux\
#MoaV-beezhan-t7d
```

Whitespace + backslashes are for readability — the wire format is one URL.
Base64'd that's ~640 bytes (the current six-URI subscription is ~2.0 KB
base64). ~70% smaller, and edits to a shared field (UUID rotation) touch
exactly one place.

## How moav-client would consume it

1. **Parser dispatch**: `subscription.ParseURI` gains a `moav://` branch that
   produces N `Endpoint` structs — one per `p=` entry — by combining the
   shared params with the per-protocol overrides. Each materialised endpoint
   keeps a stable `RawURI` (the rebuilt single-protocol URI) so dedup,
   persistence (`state.json`), and the existing balancer all keep working
   unchanged.
2. **Mixed sources**: a subscription file may freely mix legacy
   `vless://…`/`trojan://…` lines with `moav://…` bundles. Each line is
   parsed by its scheme.
3. **Round-trip**: a `moav-client bundle <subscription.txt>` subcommand
   collapses a subscription back into one or more bundles (one per unique
   server host group). Helpful for sharing.

## Backwards compatibility

- Existing subscription URIs continue to work unchanged.
- A bundle expands to the same endpoint objects a multi-line subscription
  would produce; sing-box config generation and the load balancer see no
  difference.
- The MoaV server admin tooling that emits subscriptions can additionally
  emit a `moav://` bundle alongside the legacy URIs (separate line in the
  base64 blob) so old clients keep working and new clients prefer the bundle.

## Open questions

- **Versioning**: prefix with `moav://v1/…`? Or rely on adding a `v=1` query
  param? Probably the latter — cleaner URL.
- **Signing**: optional `sig=<ed25519(canonical)>` field so the client can
  verify a bundle came from a trusted source. Useful for distribution via
  untrusted channels (Telegram, GitHub gists). Out of scope for v1.
- **Sidecar opt-in**: should the bundle declare which sidecars (DNS-over-HTTPS,
  Psiphon, Tor) to enable? Currently sidecars live in client `config.yaml`,
  which is right; bundles should be server-only.

## Implementation surface

In moav-client this would touch:

| File                                   | Change                                                            |
|----------------------------------------|-------------------------------------------------------------------|
| `subscription/parser.go`               | new `parseMoaVBundle(uri string) ([]Endpoint, error)` + dispatch  |
| `subscription/parser_test.go`          | round-trip parse/serialise tests                                  |
| `cmd/cli.go`                           | `bundle <file>` subcommand that emits a bundle URI                |
| (optional) `singbox/generator.go`      | nothing — endpoints are already protocol-typed                    |
