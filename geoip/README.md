# geoip CIDR lists

`geoip:<cc>` routing rules match a destination IP against the CIDRs in
`geoip/<cc>.txt` (one CIDR per line; `#` comments and blank lines ignored).
Lowercase country code → filename, e.g. `geoip:ir` reads `geoip/ir.txt`.

This dir is bind-mounted read-only into proxy-core at `/app/geoip`, so you can
edit lists without rebuilding the image.

Notes:
- Matching is **IP-only**. A destination given as a hostname (the common
  `socks5h://` case) is not resolved here, so a `geoip:` rule won't match it —
  it applies to IP-literal destinations.
- If the list file for a referenced `cc` is missing, the rule is **inert** and
  proxy-core logs a one-time WARN. A `block` rule that references a missing
  list will NOT block — populate the file.
- This is a manually-maintained stub. For full coverage, drop in CIDR exports
  from a GeoLite2 / RIR dataset (e.g. one country's aggregated ranges).
