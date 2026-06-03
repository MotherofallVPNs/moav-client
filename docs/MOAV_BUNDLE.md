# `moav://` bundle format — specification

**Version**: 1
**Status**: implemented in moav-client (`proxy-core/subscription/moavbundle.go`)
**Tracking**: [ibeezhan/moav-client#1](https://github.com/ibeezhan/moav-client/issues/1)

## Purpose

A MoaV subscription today is N independent URIs — one per protocol — each
repeating the same server host, the same UUID for VLESS variants, the same
passwords for Trojan/Hy2, the same Reality keypair. Rotating any shared
credential means regenerating *every* URI; users importing the subscription
into stripped-down clients can't tell which entries belong to the same
server.

The `moav://` bundle format compresses one server's full protocol surface
into a single transferable URL whose shared params are factored out and
per-protocol records override only what differs. ~70% smaller than the
multi-URI form for a typical 6-protocol bundle, and rotating shared
secrets becomes a one-line edit.

## Grammar

```
moav-uri  = "moav://" [user "@"] host ["?" shared-query ("&p=" proto-spec)+ ] ["#" label]

user      = <userTag, optional — mirrors VLESS userinfo>
host      = <defaultHost — IPv4 / IPv6 in [brackets] / hostname>
label     = <human-readable fragment for the whole bundle>

shared-query = <flat query-string carrying shared keys; see "Shared keys">

proto-spec   = name "," port [ "," kv ("," kv)* ]
name         = "reality" | "vless-ws" | "vless-xhttp"
             | "trojan" | "ss" | "hy2" | "tuic" | "vmess"
port         = 1..65535
kv           = key "=" value         ; per-record override
```

### Shared keys

Any `<shared-query>` key applies to *every* `p=` record unless the record
overrides it. Order doesn't matter.

| Key            | Used by               | Notes |
|----------------|-----------------------|-------|
| `uuid`         | vless / vmess / tuic  | required for those protocols |
| `pw`           | trojan / hy2 / tuic   | also `password` as an alias |
| `ss_method`    | ss                    | e.g. `2022-blake3-aes-128-gcm` |
| `ss_pw`        | ss                    | for `2022-blake3-*`, includes the `:` separator |
| `pbk`          | reality / vless-xhttp | Reality public key |
| `sid`          | reality / vless-xhttp | Reality short ID |
| `sni_default`  | every TLS protocol    | back-fills `sni` when a record doesn't override |
| `fp`           | every TLS protocol    | uTLS fingerprint, e.g. `chrome`, `random` |
| `v`            | reserved              | bundle format version — current spec is `v=1`. Omittable for now. |
| `sig`          | reserved              | future ed25519 signature; ignored by v1 parsers |

### Per-record overrides

Inside one `p=` record any `k=v` after the port replaces the shared key of
the same name. Common per-record keys:

| Key      | Where it matters                    |
|----------|-------------------------------------|
| `sni`    | this protocol uses a different SNI than `sni_default` |
| `flow`   | usually `xtls-rprx-vision` on `reality` |
| `path`   | VLESS+WS / VLESS+XHTTP transport path |
| `host`   | the actual server host for *this* protocol if different from `<defaultHost>`. Also used as the WS `Host:` header. |
| `alpn`   | comma-separated ALPN list for TLS |
| `obfs`   | `hy2` obfuscation type, e.g. `salamander` |
| `obfs_pw`| `hy2` obfuscation password |

## Concrete example

The `beezhan-t7d` MoaV bundle (~2.0 KB in legacy 6-URI form) becomes:

```
moav://beezhan-t7d@<MOAV-IP>?\
uuid=<UUID>&\
pw=<TROJAN-AND-HY2-PASSWORD>&\
ss_method=2022-blake3-aes-128-gcm&\
ss_pw=<SS-PSK-1>%3A<SS-PSK-2>&\
pbk=<REALITY-PUBKEY>&\
sid=<REALITY-SID>&\
sni_default=<DEFAULT-SNI>&\
fp=chrome&\
p=reality,443,sni=<REALITY-SNI>,flow=xtls-rprx-vision&\
p=vless-ws,443,host=<CDN-HOST>,path=<CDN-PATH>,sni=<CDN-SNI>,alpn=http/1.1&\
p=vless-xhttp,2096,sni=<XHTTP-SNI>&\
p=trojan,8443,sni=<TROJAN-SNI>&\
p=ss,8388&\
p=hy2,443,sni=<HY2-SNI>,obfs=salamander,obfs_pw=<HY2-OBFS-PW>\
#MoaV-beezhan-t7d
```

Whitespace and backslashes are only there for the doc; the wire format is
one line. Base64'd that's ~640 bytes vs the 2.0 KB legacy.

## Reference parser semantics

The reference parser is `subscription.ParseMoaVBundle` in moav-client. It
expands a bundle by:

1. Parsing the URL with stdlib `net/url`. Reject if scheme isn't `moav`.
2. Reading `<defaultHost>`, `<userTag>`, `<label>`.
3. Snapshotting the query into a flat `map[string]string` — *shared*.
4. For each `p=` value, splitting on `,` to get `name,port` + N `k=v` overrides.
5. Building a canonical single-protocol URI (`vless://…`, `trojan://…`, etc.)
   from `merge(shared, overrides)` and delegating to the existing per-scheme
   parser. The materialised Endpoint inherits the bundle's label name with
   the protocol suffixed (`MoaV-beezhan-t7d-reality`, etc.).
6. Each Endpoint's `RawURI` is the rebuilt single-protocol URI so dedup
   (by `RawURI`) keeps working when a user has both legacy URIs and a
   bundle pointing at the same server.

### Mixing with legacy URIs

A subscription file MAY contain a mix of `moav://` bundles and legacy
single-protocol URIs (`vless://`, `trojan://`, …). Each line is parsed
independently; downstream dedup is by `RawURI`.

## Versioning

The `v=` query parameter is reserved for future extensions. Current parsers
treat its absence as `v=1`. A future `v=2` parser MUST either accept `v=1`
inputs unchanged or reject them explicitly.

## Signing (reserved, not implemented in v1)

A future `sig=` parameter will carry an ed25519 signature over the
canonical (sorted-key) serialisation of the bundle minus the `sig` field
itself. Clients that don't recognise `sig=` (i.e. all v1 parsers) MUST
ignore it. This is so untrusted distribution channels — Telegram channels,
GitHub gists — can ship bundles whose authenticity the receiver can
verify against a known operator pubkey.

## Open work

- [ ] Server-side adoption in shayanb/MoaV (issuing `moav://` URLs
      alongside the legacy URIs).
- [ ] `moav-client bundle <subscription.txt>` CLI subcommand that
      collapses an N-URI subscription back into one or more bundles
      (one per unique server host group).
- [ ] Signing.

## Notes for implementers

- Don't URL-decode keys that the destination parser will re-decode itself.
  `path=/foo/bar`, not `path=%2Ffoo%2Fbar`.
- For `ss_pw`, the `:` separator inside 2022-blake3 PSKs must be URL-encoded
  as `%3A` because `,` is the per-record delimiter.
- IPv6 hosts MUST be wrapped in `[]` to disambiguate the colon from the
  port separator.
- A bundle without a `defaultHost` is invalid even if every `p=` record
  carries a per-record `host=` override — the host slot is structurally
  required.
