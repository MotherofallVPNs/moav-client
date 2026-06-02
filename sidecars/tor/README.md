# Tor Sidecar

This sidecar uses the official Tor image via the `tor` Docker profile.

**docker-compose.yml** entry uses `torproject/arti` (Rust Tor implementation).

Phase 5 notes:
- proxy-core will forward traffic to `tor:9050` (SOCKS5) when `sidecars.tor.enabled: true`
- Alternatively use `torproject/tor` for the C implementation with standard `torrc`
- Consider `obfs4proxy` or `snowflake` pluggable transports for censored networks
