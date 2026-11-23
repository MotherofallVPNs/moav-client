# Changelog

All notable changes to moav-client are documented here. Format loosely follows
[Keep a Changelog](https://keepachangelog.com/); versions are [SemVer](https://semver.org/).

## [Unreleased]

### Added
- **Renovate for dependency updates** (`.github/renovate.json`), matching the
  MoaV server's setup: grouped PRs against `dev` for the component pins in
  `.env.example` (moved together with their compose and Dockerfile `ARG`
  fallbacks so the version-sync gate stays green), Dockerfile base images,
  Caddy and the digest-pinned Tor image, web-ui npm dependencies and the
  SHA-pinned GitHub Actions. Weekly on Monday, actions and digest refreshes
  monthly, majors always in their own PR, no automerge, no dependency
  dashboard issue. See [docs/CONFIGURATION.md](docs/CONFIGURATION.md#automated-update-prs-renovate).

### Changed
- **TrustTunnel sidecar client updated to 1.3.0** (was 1.0.49), pairing with
  TrustTunnel endpoint 1.3.1 on the MoaV server. No config-generation changes
  were needed: the client's TOML format is backward compatible, and the generated
  `client.toml` was validated against the real 1.3.0 binary (it parses and serves
  SOCKS5). Everything outside the bundle's `[listener.*]` sections is passed
  through verbatim, so the new per-endpoint `tls_profile` and
  `client_random_auth_key` keys work as soon as a server bundle carries them.
  Upstream changes that reach this sidecar: the QUIC/HTTP3 stack moved from
  quiche to ngtcp2/nghttp3 (1.1.5), several HTTP/3 crash fixes, a Chrome
  ClientHello refreshed to November 2026, and idle SOCKS5 connections now see
  the destination's close immediately instead of being reused as zombies. An
  unknown `tls_profile` value is a hard config error in 1.3.0 (older clients
  ignored the key).
- **sing-box sidecar updated to 1.14.0** (was 1.13.19), matching the MoaV server.
  No config-generation changes were needed: the generator already emits the
  modern WireGuard `endpoints[]` form (not the outbound removed in 1.14), no
  legacy DNS/`block`/`dns`/`sniff` constructs, and its output was validated
  against the real 1.14 `sing-box check`. Hysteria2 `obfs` is passed through
  verbatim, so a server offering the new `gecko` obfuscation works with no client
  change. (Snell is intentionally not supported: sing-box's Snell *outbound* has
  no v5, so the sidecar cannot dial a Snell v5 server.)
- **AmneziaWG daemon updated to `amneziawg-go v3.1.20261116`** (was
  v3.1.20260814; the tools stay at v3.1.20260812, still the latest). Upstream
  hardened the AWG 3.1 obfuscation parameters: `jmin > jmax` is rejected instead
  of underflowing the junk packet size, negative lengths in the `<r>/<rc>/<rd>/<dz>`
  tags are rejected, I1 to I5 can change safely while handshakes are in flight,
  and a timer race is fixed. Same wire format.
- **MasterDNS client updated to v2026.11.18** (was v2026.05.10): FastStart with
  background resolver discovery and scoring, a fix for resolver probation
  starvation, and data-race fixes in session init and the resolver health loop.
  The build now verifies the download against the release's `SHA256SUMS.txt`,
  as the MoaV server does, and fails on a mismatch.
- **AmneziaWG and SNI-spoof builders move to `golang:1.27-bookworm`** (was
  1.25, end of life), so both binaries pick up the Go security fixes released
  since August.
- **web-ui routine dependency bumps.** react and react-dom 19.3.1, `@types/react`
  and `@types/react-dom` 19.3.0, `@vitejs/plugin-react` 6.3.0,
  `@testing-library/react` 16.3.3 and jsdom 31.0.1, with the `package.json`
  floors raised to match (vite's floor is now 8.3.8, the first release with the
  dev-server fixes). The dashboard build image and the CI job move from Node 22
  to Node 24 (active LTS).
- **Nothing in the stack floats any more.** Psiphon tunnel-core and
  SNI-Spoofing-Go were built from whatever their default branch held at build
  time; they are now built from release tags (`PSIPHON_VERSION=v2.0.43`,
  `SNI_SPOOF_VERSION=v0.7.2`). The Caddy image moves from `caddy:2` to
  `caddy:${CADDY_VERSION}` (2.11), and the Tor image, which has no version tags
  upstream, is pinned by digest instead of `:latest`. The three new variables
  live in `.env.example` with the other component versions, and the drift gate
  (`tests/version-sync-test.sh`) now also fails on an unpinned `git clone` or a
  floating image default. `IMAGE_TOR` / `IMAGE_CADDY` still override the whole
  image ref.
- **The dashboard image is built with `npm ci`** (was `npm install`), so it
  installs exactly the committed lockfile that CI tested instead of re-resolving
  the version ranges at build time.

### Fixed
- **Domain rules now match on the SOCKS5 port.** The SOCKS5 listener resolved a
  hostname on the proxy-core host before the rules ran, so the rule engine and
  the torrent blocker only ever saw an IP: `domain`, `domain_suffix` and
  `domain_keyword` rules never matched SOCKS5 traffic, although they did on the
  HTTP CONNECT port. The hostname now reaches the rules as the client sent it
  (`socks5h://`). A SOCKS5 destination name that is empty or contains spaces or
  control characters is refused.
- **`geoip` / `ip_cidr` rules now classify hostnames on both ports, before any
  dial.** Routing is two-pass, like Xray's `IPIfNonMatch`: rules run on the
  hostname first; if none matched and IP rules are enabled, the name is
  resolved with the local resolver (never through a tunnel) and the rules run
  again on its addresses. With `geoip: ir → direct`, a `socks5h://` or HTTP
  CONNECT request for a name that resolves to an Iranian address now goes
  direct and never reaches a MoaV endpoint, so the exit server's IP is not
  exposed to servers inside Iran. Before, this only worked on the SOCKS5 port
  as a side effect of resolving every name first, and never on HTTP CONNECT.
  Proxied names are still forwarded to the endpoint unresolved, and a direct
  dial reuses the classification lookup. No lookup is made when a name rule
  matches or no IP rule is enabled. See
  [docs/PLUGINS.md](docs/PLUGINS.md#resolution-order).
- **`.ir` names stay direct even when the local lookup fails.** The shipped
  `config.yaml.example` now enables `domain_suffix: ir → direct` next to
  `geoip: ir → direct`. The `geoip` rule needs a local DNS answer to classify a
  name; without one the name took the proxy route, and the exit server would
  connect to an in-country server from its own IP. The suffix rule matches on
  the name, with no lookup, so `.ir` names never reach a tunnel. Existing
  installs keep their own `config.yaml`; add the rule by hand or from the
  dashboard Plugins tab. See
  [docs/PLUGINS.md](docs/PLUGINS.md#why-ir-names-also-have-a-name-rule).
- **A trailing dot no longer defeats domain rules.** `example.com.` now matches
  `domain`, `domain_suffix`, `domain_list` and the torrent tracker list, on
  both ports.
- **HTTP CONNECT no longer drops the first byte a client sends.** The handler
  wrote the `200` and then hijacked the connection, and `net/http` flushes that
  `200` before it stops its one-byte background read. A client that answered
  quickly (a TLS ClientHello right after the `200`) could have its first byte
  parked in the server's read buffer, which the handler discarded, breaking
  the handshake intermittently. Bytes sent along with the CONNECT request were
  always lost. The handler now hijacks first, writes the `200` itself and
  forwards anything already buffered before the copy starts.
- **Data race in `/api/flows`.** Reading the flow list while a connection
  closed raced on the flow's result and byte counters. They are now read and
  written under the ring's lock (and atomically for the counters).
- **Telegram release notification pins its link preview to the release page.**
  The preview showed whatever link appeared first in the notes body (e.g. a
  client's App Store page) instead of the release. It now sets
  `link_preview_options.url` (Bot API 7.0) to the release/issue URL. Matches the
  same fix on the MoaV server.
- **`moav-client update` now carries component version bumps into an existing
  install.** `.env` is seeded from `.env.example` once, at install, and nothing
  refreshed its `*_VERSION` pins afterwards, so an updated install kept building
  the sing-box, Xray and sidecar versions of the day it was installed, whatever
  the release pinned. `update` now re-syncs those lines with the pulled
  release (it lists each change and asks first, default yes, and proceeds on
  its own without a terminal) before the rebuild, and leaves every other `.env`
  line alone. `--keep-pins` skips it for a deliberately held version, the new
  `moav-client sync-pins` runs only the re-sync, and re-running the installer
  over an existing install does the same. **Upgrade note:** the update that
  brings this change in is still run by the previous wrapper, so on an existing
  install run `moav-client update` once more afterwards (or `moav-client
  sync-pins`, then `moav-client up --build`).
- **Saving settings from the dashboard no longer rewrites YAML merge keys.** A
  `config.yaml` that uses anchors with `<<: *name` had each merge key written
  back as `!!merge <<: *name` on every save. It is now left as written. A value
  that starts with a newline is also written back intact (the newline was
  dropped before), and a malformed merge key in `config.yaml` is reported as an
  error instead of a panic in the request handler. All three come from the YAML
  library change listed under Internal.

### Internal
- **TrustTunnel sidecar has a smoke test.** `tests/trusttunnel-sidecar-test.sh`
  (run in CI) builds the image at the pinned version and checks that the real
  client accepts the `client.toml` proxy-core generates and answers SOCKS5 on
  `:5600`. The image build now picks the client binary by exact name (the
  release tarball also ships `trusttunnel_client.sig`, which the old glob could
  match first) and runs `--version` so a wrong or unrunnable asset fails the
  build instead of the container.
- **CI: golangci-lint runs again.** `golangci-lint-action@v6` with
  `version: latest` installed a golangci-lint built with Go 1.24, which refuses
  a Go 1.25 module and exits 3; `continue-on-error` kept the step green, so
  nothing had been linted since the move to Go 1.25. The step now uses action
  v9 with golangci-lint pinned to v2.15.0 (built with Go 1.27). Findings stay
  non-blocking for now (`--issues-exit-code=0`), but a linter that cannot run
  fails the job.
- **CI actions updated and pinned by commit SHA:** checkout v7.0.1, setup-go
  v7.1.0, setup-node v7.1.0, upload-artifact v7.0.2, golangci-lint-action
  v9.3.0 (each was three majors behind on a floating major tag). They run on
  Node 24, so a self-hosted runner must be v2.327.1 or newer.
- **Live e2e no longer gives up before the engines are up.** The single probe
  could land before sing-box and xray were listening, fail every endpoint in
  milliseconds, and end the run as "0 ok". The probe is now re-triggered while
  waiting.
- `.env.example` and `docs/CONFIGURATION.md` no longer claim the component pins
  track the MoaV server's; shared components stay wire-compatible but are
  bumped on the client's own schedule.
- **Two unmaintained Go dependencies replaced.** `gopkg.in/yaml.v3` (archived
  upstream in April 2025) becomes `go.yaml.in/yaml/v3` v3.0.6, the same code
  base now maintained by the YAML organization, with an identical API.
  `armon/go-socks5` (last commit 2016) becomes `things-go/go-socks5` v0.1.4, its
  maintained descendant. The SOCKS5 listener behaves as before: CONNECT only,
  optional username/password auth, the same reply codes, and BIND and UDP
  ASSOCIATE still refused (the new library can relay UDP itself, which would
  bypass the tunnel, so both commands are refused explicitly). New wire-level
  SOCKS5 tests and config.yaml decoding and round-trip tests were written
  against the old libraries first and pass unchanged on the new ones.
- **web-ui type-checks with TypeScript 7.0.2** (was 5.9.3), the native
  compiler. `tsc` is only the type-check half of `npm run build`; vite produces
  the bundle, and the built dashboard is byte-identical. The source needed no
  change (it was already clean under the TypeScript 6 defaults that 7 enforces:
  `types: []`, checked side-effect imports, stable type ordering).
  `tsconfig.json` drops `DOM.Iterable`, an empty lib since TypeScript 6, and now
  also type-checks `vite.config.ts` and `vitest.config.ts`, so a config option
  removed by a tool major fails the build instead of being ignored.
- **web-ui tests run on vitest 5.1.1 with `@testing-library/jest-dom` 8.0.0**
  (were 4.1.12 and 6.9.1); jest-dom 8 only supports vitest 5, so they move
  together. The test setup now imports `@testing-library/jest-dom/vitest`:
  vitest 5 no longer reads `jest.Matchers`, so with the bare import the DOM
  matchers ran but no longer type-checked. `@testing-library/dom`, a required
  peer of jest-dom since 7, is declared explicitly, and `package.json` gains an
  `engines.node` range matching vitest 5 (`^22.12 || ^24 || >=26`; the build
  image and CI are on Node 24). Adds the first component test
  (`BlockDirectToggle`, the kill-switch banner), so jsdom, Testing Library and
  the jest-dom matchers are exercised by `npm test` instead of only being
  installed: 13 tests, up from 8.

### Security
- **proxy-core builds with Go 1.27** (was 1.25, end of life since 2026-08-19).
  Under the Go 1.25 toolchain, govulncheck reported 11 standard-library
  vulnerabilities reachable from proxy-core code, among them the HTTP CONNECT
  desync (CVE-2026-94439), the `MaxBytesReader` header race (CVE-2026-97054)
  and the `crypto/x509` distrust of compromised intermediates (CVE-2026-111943)
  on the subscription fetch. `go.mod` now requires Go 1.27 with a `go1.27.4`
  toolchain floor, the build image is `golang:1.27-alpine` (which also refreshes
  the CA bundle copied into the scratch image), and `golang.org/x/net` moves to
  v0.63.0. govulncheck finds no vulnerabilities in the new build.
- **MPTCP is off on the Go listeners.** Go 1.24+ opens MPTCP listeners by
  default, and the public exploit for Linux kernel CVE-2026-98432 needs one. It
  matters most when the proxy ports are exposed to the LAN or publicly.
  proxy-core pins `godebug multipathtcp=0` in `go.mod`, so it also holds when
  the binary runs outside Docker (a test guards the directive), and compose sets
  `GODEBUG=multipathtcp=0` for the xray, caddy, psiphon and sni-spoof
  containers. A patched kernel is still the real fix; this removes the known
  exploit path.
- **Xray-core sidecar updated to v26.11.9** (was v26.7.28, which upstream marks
  as a pre-release). The range includes two emergency security releases: Xray
  now verifies TLS against its own root CA set with the compromised roots
  removed (v26.10.10, v26.10.21) and disables MPTCP on its listeners
  (v26.10.31), plus XHTTP and mux race fixes. The default `chrome` uTLS
  fingerprint becomes Chrome 155, which reaches every xhttp endpoint with no
  config change because the generator already emits `fingerprint: chrome`. The
  source-compile fallback moves to `golang:1.27-alpine` (Xray needs Go 1.27
  since v26.9.8) and the runtime stage to `alpine:3.24` (3.21 went end of life
  on 2026-11-01). The generated `xray.json` passes `xray run -test` on the new
  binary unchanged.
- **sing-box sidecar updated to 1.14.4** (was 1.13.19), matching the MoaV
  server. 1.14.4 is built with Go 1.26.10, which carries the `crypto/x509`
  distrust of compromised intermediates (CVE-2026-111943); 1.13.19 was built
  with Go 1.25.12 and the 1.13 line is no longer released. No generator change
  was needed: its output for every supported protocol passes the real 1.14.4
  `sing-box check`. Note that since 1.14 Hysteria2 parrots Chrome's QUIC
  handshake, and Chrome does not offer Ed25519, so a third-party Hysteria2
  server with an Ed25519 certificate will fail to connect (MoaV servers use
  ACME certificates and are not affected).
- **Dashboard runtime image moves to `nginx:1.30-alpine`** (was
  `nginx:1.27-alpine`). nginx 1.27 has been end of life since 2025-06 and that
  tag was last built in 2025-04, so every dashboard image missed 19 months of
  nginx and Alpine fixes (OpenSSL, busybox, and the `apache2-utils` that writes
  the basic-auth file), including the resolver use-after-free CVE-2026-123634
  on the `resolver` + variable `proxy_pass` path this config uses. The server
  block and the entrypoint hooks are unchanged.
- **web-ui dev toolchain: lockfile refreshed for the fixable advisories.** vite
  8.2.2 to 8.3.8 (dev-server file disclosure and cross-origin script
  execution), undici 8.10.0 to 8.13.1, brace-expansion and source-map-js, all
  inside the existing `package.json` ranges. `npm audit` drops from 11 findings
  to 7; the rest need the vitest major and are handled separately. None of
  these packages ship in the dashboard bundle.
- **web-ui test runner moves to vitest 4.1.12** (was 3.2.7), with
  `@vitest/coverage-v8` to match. The vitest 3 line has no fix for the two
  critical advisories in its tree (tinypool prototype pollution leading to code
  execution, `@vitest/mocker` path traversal) or for the glob and minimatch
  findings under the coverage provider. No test or config change was needed.
  Together with the lockfile refresh above, `npm audit` is clean (0 findings).

## [1.4.0] — 2026-08-23

### Added
- **`moav://` compact bundle support.** moav-client now imports the compact
  `moav://` subscription the MoaV server emits (one URL encoding a server's whole
  enabled proxy surface) and expands it into per-protocol endpoints — reality,
  vless-ws, **vless-httpupgrade**, vless-xhttp, trojan, anytls, ss, hy2. CDN
  endpoints get the correct WS/httpupgrade transport and Host header. When a
  subscription carries both the `moav://` line and the equivalent legacy
  `vless://…` URIs, they are **deduplicated by protocol + address**, so each
  server appears once instead of twice. See `docs/MOAV_BUNDLE.md`.
- **AmneziaWG v3 client support.** The bundled AmneziaWG sidecar is pinned to the
  v3.1 daemon + tools (`amneziawg-go v3.1.20260814`, `amneziawg-tools
  v3.1.20260812`), matching the MoaV server's move to v3 obfuscation, so imported
  AmneziaWG configs handshake and connect.

### Changed
- **Config: a single source of truth for versions and ports.** The `VERSION`
  file is authoritative for the app version, with a drift-gate test keeping the
  build literals in sync (they had drifted to 1.3.1/1.3.2). Component versions
  moved to uncommented `_VERSION` vars in `.env.example` matching the MoaV server
  (sing-box 1.13.19, xray v26.7.28, and the rest), and
  `SOCKS5_PORT`/`HTTP_PORT`/`API_PORT` in `.env` are now the single source,
  overriding `config.yaml`. New `docs/CONFIGURATION.md` documents the `.env` vs
  `config.yaml` boundary.
- **Dependencies modernized.** web-ui to **React 19 / Vite 8 / Vitest 3** (plus
  matching `@types`, plugin-react 6, jsdom 30); proxy-core `golang.org/x/*`
  security bumps. CI and the web-ui build image move to Node 22, which the new
  toolchain requires.

### Fixed
- **Reality public keys now survive a `moav://` import.** Expanding a bundle
  rebuilt per-protocol URIs from decoded values without re-encoding, so a base64
  `pbk` (which contains `+`) corrupted, with `+` becoming a space. Values are now
  percent-escaped on the rebuild.
- **Bundle import: MasterDNS + AmneziaWG MTU.** Importing a server `.zip`
  mis-parsed `masterdns-client_config.toml` — an inline `# comment` leaked into
  the encryption method and `DOMAINS = ["host"]` kept its array brackets — and
  the AmneziaWG sidecar ignored the config's `MTU` (staying at the kernel
  default). Both are fixed.

### Internal
- CI runs on the `dev` branch (was `main`-only), so the dev→main flow is gated.
- The `install.sh` release asset is now published under the `client-install.sh`
  download name the install one-liner fetches.

## [1.3.3] — 2026-08-18

### Added
- **Community links.** The `moavc` banner (`version` / `help` / `status`) lists
  the Telegram and X links next to the repo and site, and the README (EN + FA)
  gains a Community section.

### Changed
- **README refreshed to the MoaV server's style:** centered header, table of
  contents, and a new **Import your config** section covering the three ways to
  load a bundle (the `moav://` compact URL, a base64 or plain subscription, and
  dropping a server `.zip`). The Farsi README is brought to full parity,
  including the previously-missing REST API and Development sections.
- **`install.sh` is now published as a release asset**, and the install
  one-liner fetches `moav.sh/client-install.sh` instead of a raw GitHub URL.
- **Go module path** renamed from `github.com/ibeezhan/moav-client` to
  `github.com/MotherofallVPNs/moav-client` after the org move. Internal only,
  no behaviour change.

### Internal
- **Telegram release notifier** (`.github/workflows/telegram-notify.yml`) — auto-posts to the MoaV Telegram channel when a moav-client Release is published (labeled `moav-client` so it's distinct from server posts in the shared channel), and when an issue is labeled `announce`. Manual `workflow_dispatch` sends a test message or — with a `release_tag` — a real notification for a past release. Small `curl` to the Telegram Bot API; message built + HTML-escaped by `.github/scripts/telegram_format.py`. Mirrors the server notifier. Needs repo secrets `TELEGRAM_BOT_TOKEN` + `TELEGRAM_CHAT_ID`; skips cleanly when unset.

### Fixed
- **Bundle import now recognizes v2 server bundle file names.** A real `moav
  user package` zip renamed the per-protocol files, so importing a v2 bundle
  silently dropped WireGuard, AmneziaWG, and MasterDNS. WireGuard/AmneziaWG are
  now `moav-<server-tag>-wg.conf` / `-awg.conf` (not `wireguard.conf` /
  `amneziawg.conf`), and MasterDNS ships `masterdns-client_config.toml` (a flat
  `KEY = value` file) in place of the free-form `masterdns-instructions.txt`.
  The importer matches WireGuard/AmneziaWG by suffix and parses the new TOML,
  keeping the legacy names working.
- **Build from source now works on Windows** ([#7](https://github.com/MotherofallVPNs/moav-client/issues/7)). `makeTTLControl` (the by-hand traceroute in `/api/diag`) called `syscall.SetsockoptInt(int(fd), …)`, but a socket fd is an `int` on Unix and a `syscall.Handle` on Windows — so `go build` failed on Windows with *"cannot use int(fd) … as syscall.Handle value"*. Split the setter into `api/ttl_{unix,windows}.go` behind build tags. CI now cross-compiles for `windows`/`darwin` so OS-specific breakage is caught.

## [1.3.2] — 2026-07-13

### Testing
- **Unit tests for the previously-untested Go packages** — `balancer` (0→57%:
  strategy selection, priority ordering, weighted distribution, stats/flows),
  `state` (0→75%: save/load round-trip, missing/corrupt files), `cmd` (0→37%:
  the hand-rolled subscription/YAML field extractor + list output), `sidecars`
  (0→93%: enabled-set→endpoint mapping + every config writer), and a big lift to
  the sing-box generator (46→88%). The sing-box tests double as a **protocol
  parity guard** (`singbox/parity_test.go`): each protocol's outbound shape is
  pinned, and the Xray-only transports (xhttp/splithttp/raw) are asserted to be
  rejected so they route to the xray core.
- **web-ui unit tests** — adopt **Vitest** + Testing Library (there were none);
  cover the display-label and status-color helpers. `npm test` / `test:coverage`.
- **`docs/PROTOCOL-PARITY.md`** — audit of which MoaV-server protocols the client
  can connect with (13 full, 2 partial) and the hard gaps (wstunnel, XDNS,
  GooseRelay, Snowflake; Slipstream stub; dnstt config not generated).

### CI
- Wire the new tests into `.github/workflows/ci.yml`: `vitest` on web-ui, Go
  test with `-coverprofile` + a printed coverage summary (uploaded as an
  artifact), `golangci-lint` (warn-only for now), and a `docker compose config`
  validation job.
- **Live e2e** (`.github/workflows/e2e.yml` + `tests/e2e-client.sh`) — on a
  self-hosted runner, brings the client stack up against a **provided MoaV
  server bundle** (secret) and asserts the endpoints parse, `/api/probe`
  validates ≥1, and a fetch through SOCKS5 `:1080` egresses from the server's IP
  (not the runner's). Runner setup + the planned cross-repo "server on the box
  mints a fresh bundle per run" flow: `docs/devdocs/E2E-TESTING.md`.

### Removed
- **Dead Slipstream sidecar stub** — it had no compose service and, when
  enabled, dialed a nonexistent `slipstream:5302` host. Removed the config
  toggle, the manager entry, and the UI label/color leftovers. (The parity
  audit also found the `dns-tunnels` "dnstt" sidecar is a mislabeled MasterDNS
  copy — a real dnstt client is a separate follow-up. See `docs/PROTOCOL-PARITY.md`.)

### Fixed
- `moavc stats` / `probe` / `status` no longer fail with `401` once a dashboard
  password is set (e.g. after exposing on the LAN) — the wrapper now sends the
  `MOAV_DASHBOARD_USER`/`PASS` basic-auth from `.env` to the API.
- `moavc update` no longer aborts on a deleted `data/.gitkeep` (a tracked
  dir-placeholder that runtime/wipe can remove) — it's restored before the
  local-changes check.

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

First public release. A local client for [MoaV](https://github.com/MotherofallVPNs/moav)
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
