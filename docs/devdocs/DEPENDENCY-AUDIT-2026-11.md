# Dependency audit, 2026-11

Audit date: 2026-11-21. Audited tree: `main` at `591daaa` (moav-client 1.4.0), with the `dev` delta (`f0dd51f`) noted where it differs. Previous audit: none; the last dependency pass was PR #22 (2026-08-21, shipped in 1.4.0). Companion to the MoaV server audit of the same date (MotherofallVPNs/MoaV PR #354).

This is research and planning only. No version was changed by this audit. Every "latest" below was read from the upstream release API, tag list or registry on the audit date and is cited by tag.

## Summary

- 33 third-party components carry an explicit version pin. **26 are behind upstream or out of support, 2 sit on archived or abandoned upstreams, 5 are current.** Four more components are not pinned at all (two source builds from branch HEAD, two floating image tags).
- **6 P0 security items**: the proxy-core Go toolchain (Go 1.25 is EOL; govulncheck reports 11 reachable standard-library vulnerabilities), MPTCP listener hardening on the proxy ports, Xray-core (a July pre-release, behind two emergency security releases), sing-box (binary built with Go 1.25.12), the dashboard runtime image (nginx 1.27, EOL since 2025-06 and not rebuilt since 2025-04) and the web-ui dev toolchain (11 npm advisories, 2 critical, none in the shipped bundle).
- Out of support: **Go 1.25** (EOL 2026-08-19; proxy-core, AmneziaWG and SNI-spoof builders, `go.mod`, CI), **Alpine 3.21** (EOL 2026-11-01; Xray runtime stage), **nginx 1.27** (EOL 2025-06-24; dashboard).
- Archived or abandoned upstreams: **go-yaml/yaml** (`gopkg.in/yaml.v3`, archived 2025-04), **armon/go-socks5** (last commit 2016-09). Watch list: `golang.org/x/net/websocket`, the `peterdavehello/tor-socks-proxy` image, SNI-Spoofing-Go.
- Every pin the client shares with the MoaV server is now **behind the server**, although `.env.example:49` says the values track it: sing-box 1.13.19 vs 1.14.4, Xray v26.7.28 vs v26.10.21, amneziawg-go v3.1.20260814 vs v3.1.20260828, MasterDNS v2026.05.10 vs v2026.06.13. Only awg-tools and the TrustTunnel client match.
- Biggest wins: Xray v26.11.9 (security plus the Chrome 155 uTLS fingerprint), sing-box 1.14.4 (already half done on `dev`), amneziawg-go v3.1.20261116 (obfuscation parameter hardening), MasterDNS v2026.11.18 (client FastStart, resolver starvation and race fixes).
- Two tooling gaps found on the way: `moav-client update` never re-syncs the `*_VERSION` pins in an existing `.env`, so a bump does not reach existing installs; and the CI golangci-lint step has been failing silently since the move to Go 1.25.

## Method

1. Fresh clone of `MotherofallVPNs/moav-client`, then a sweep of `.env.example`, `docker-compose.yml`, all 8 Dockerfiles (`proxy-core/`, `web-ui/`, `sidecars/*/`), `proxy-core/go.mod` and `go.sum`, `web-ui/package.json` and `package-lock.json`, `install.sh`, the `moav-client` wrapper, `scripts/`, `tests/` and `.github/workflows/`.
2. Latest stable per upstream from `gh api repos/<owner>/<repo>/releases/latest` (prereleases excluded) or the tag list where a project publishes no releases, the Go module proxy for Go modules, the npm registry for packages, Docker Hub and GHCR tag metadata for images, OSV and the Go vulnerability database for advisories, NVD for the kernel CVE, nginx.org for nginx advisories, and endoflife.date for support windows.
3. Release notes and commit lists between the pinned and latest tags were read for each component that is behind.
4. Two static scanners were run, which goes one step further than the server audit: `govulncheck` (call-graph reachability) on `proxy-core` under the Go 1.25.14 toolchain the image builds with, and `npm audit --package-lock-only` on `web-ui`.

Limits: no image was built and the stack was not run. Risk ratings come from upstream notes, the scanners and reading the config generators, not from a test deployment. Items marked "not verified" need a pass on the e2e runner (`docs/devdocs/E2E-TESTING.md`) before their bump lands.

## Inventory

### Protocol engines and sidecars

| Component | Pinned | Set in | Latest stable | MoaV server pin | Behind |
|---|---|---|---|---|---|
| sing-box | 1.13.19 (`dev`: 1.14.0) | `.env.example:60`, `docker-compose.yml:64` | v1.14.4 (2026-10-24) | 1.14.4 | 1 minor line. No 1.13 release since v1.13.21 (2026-08-30) |
| Xray-core | v26.7.28 (an upstream **pre-release**) | `.env.example:63`, `docker-compose.yml:116`, `sidecars/xray/Dockerfile:15` | v26.11.9 (2026-11-09) | v26.10.21 | 195 commits, 3 stable releases, 2 of them emergency security releases |
| amneziawg-go | v3.1.20260814 | `.env.example:66`, `docker-compose.yml:218`, `sidecars/amneziawg/Dockerfile:24` | v3.1.20261116 (tag, 2026-11-16) | v3.1.20260828 | 2 tags, 13 commits |
| amneziawg-tools | v3.1.20260812 | `.env.example:67`, `docker-compose.yml:219`, `sidecars/amneziawg/Dockerfile:34` | v3.1.20260812 | 3.1.20260812 | current |
| MasterDNS client | v2026.05.10.180256-27c7e11 | `.env.example:70`, `docker-compose.yml:205` and `:260`, `sidecars/dns-tunnels/Dockerfile:12` | v2026.11.18.210322-7198b45 | v2026.06.13.234407-7de2476 | 3 releases |
| TrustTunnel client | v1.0.49 | `.env.example:73`, `docker-compose.yml:236`, `sidecars/trusttunnel/Dockerfile:13` | v1.3.0 (2026-11-19) | 1.0.49 (endpoint 1.1.0) | 2 minor lines (1.1, 1.3) |
| Psiphon tunnel-core | **not pinned**: `git clone --depth=1` of `master` | `sidecars/psiphon/Dockerfile:15` | v2.0.43 (2026-11-17); HEAD `c8f89a991` (2026-11-19) | Conduit 2.1.0 (different binary) | floats, whatever HEAD is at build time |
| SNI-Spoofing-Go | **not pinned**: `git clone --depth=1` of `main` | `sidecars/sni-spoof/Dockerfile:20` | v0.7.2 (2026-06-01); HEAD equals the tag | n/a | floats, currently equal to v0.7.2 |
| Tor | image `peterdavehello/tor-socks-proxy:latest` | `docker-compose.yml:268` | image last published 2026-06-08 | n/a | floats, image is 5 months old |
| Caddy | image `caddy:2` | `docker-compose.yml:174` | v2.11.9 (2026-11-05), image rebuilt 2026-11-16 | n/a | floats within major 2, current |

Trojan, Hysteria2, Shadowsocks, VMess, TUIC, AnyTLS, Reality, WireGuard and the CDN transports are all dialled by sing-box, so they have no separate pin. No dnstt or Slipstream binary is bundled: the `dns-tunnels` compose service is a second build of the MasterDNS Dockerfile, so the Slipstream archival noted in the server audit creates no client pin.

### Go modules and toolchain (proxy-core)

| Component | Pinned | Set in | Latest | Status |
|---|---|---|---|---|
| Go toolchain | 1.25 (image has 1.25.14) | `proxy-core/go.mod:3`, `proxy-core/Dockerfile:1`, `.github/workflows/ci.yml:15` | 1.27.4, 1.26.11 | **EOL 2026-08-19.** Image frozen since that day |
| golang.org/x/net | v0.58.0 (2026-08-12) | `proxy-core/go.mod:7` | v0.63.0 (2026-11-17) | 5 minor. 7 advisories on the module, none reachable (see findings) |
| gopkg.in/yaml.v3 | v3.0.1 (2022-05) | `proxy-core/go.mod:8` | v3.0.1 on this path; successor `go.yaml.in/yaml/v3` v3.0.6 (2026-10-13), `v4` v4.0.0 (2026-11-04) | **upstream archived** |
| github.com/armon/go-socks5 | v0.0.0-20160902184237-e75332964ef5 | `proxy-core/go.mod:6` | same pseudo-version, no tags | **unmaintained**, last commit 2016-09-02 |

proxy-core imports only `golang.org/x/net/proxy` and `golang.org/x/net/websocket` from x/net.

### Base images and builders

| Image | Used by | Status on 2026-11-21 |
|---|---|---|
| `golang:1.25-alpine` | `proxy-core/Dockerfile:1` | **Go 1.25 EOL 2026-08-19.** Image frozen at 1.25.14 since 2026-08-19 |
| `golang:1.25-bookworm` | `sidecars/amneziawg/Dockerfile:17`, `sidecars/sni-spoof/Dockerfile:12` | same, EOL and frozen |
| `golang:1.26-alpine` | `sidecars/xray/Dockerfile:10` (source fallback only) | supported (1.26.11). Cannot build Xray v26.9.8 or later, which needs Go 1.27 |
| `golang:1.26-bookworm` | `sidecars/psiphon/Dockerfile:7` | supported (1.26.11); matches psiphon-tunnel-core's `go 1.26.0` directive |
| `alpine:3.21` | `sidecars/xray/Dockerfile:49` (runtime stage, provides the CA bundle) | **EOL 2026-11-01.** Supported lines: 3.22 (to 2027-05), 3.23 (to 2027-11), 3.24 (to 2028-06) |
| `debian:bookworm-slim`, `debian:bookworm` | amneziawg (`:30`, `:43`), dns-tunnels (`:10`, `:46`), psiphon (`:19`), sni-spoof (`:24`), trusttunnel (`:12`, `:31`) | Debian 12 is in its LTS phase (to 2028-06); Debian 13 "trixie" is current stable. Image still rebuilt (2026-11-05) |
| `node:22-alpine` | `web-ui/Dockerfile:1`; also `node-version: "22"` at `.github/workflows/ci.yml:76` | supported, maintenance LTS until 2027-04-30. Active LTS lines are 24 and 26 |
| `nginx:1.27-alpine` | `web-ui/Dockerfile:10` (the dashboard runtime) | **nginx 1.27 EOL 2025-06-24.** Tag last built 2025-04-16. Supported lines: 1.30 (stable), 1.31 (mainline) |
| `scratch` | `proxy-core/Dockerfile:20` | n/a. Its CA bundle is copied from the frozen builder image (`proxy-core/Dockerfile:22`) |

Debian packages installed at build time (`microsocks`, `socat`, `iptables`, `iproute2`, `python3`) and the Alpine `apache2-utils` package follow their base image, so they have no pin of their own.

### npm packages (web-ui)

Versions are what `web-ui/package-lock.json` resolves, which is what `npm ci` installs in CI.

| Package | Range (`package.json` line) | Locked | Latest | Notes |
|---|---|---|---|---|
| react, react-dom | ^19.2.8 (15, 16) | 19.2.8 | 19.3.1 | runtime dependencies, no advisories |
| @types/react | ^19.2.18 (21) | 19.2.18 | 19.3.0 | |
| @types/react-dom | ^19.2.4 (22) | 19.2.4 | 19.3.0 | |
| @vitejs/plugin-react | ^6.1.0 (23) | 6.1.0 | 6.3.0 | |
| vite | ^8.2.2 (27) | 8.2.2 | 8.3.8 | **6 advisories** (dev server), fixed after 8.2.3 |
| vitest | ^3.2.7 (28) | 3.2.7 | 5.1.1 (V4 line: 4.1.12) | **critical** via tinypool and @vitest/mocker; fixed in 4.1.12 and 5.x |
| @vitest/coverage-v8 | ^3.2.7 (24) | 3.2.7 | 5.1.1 (V4 line: 4.1.12) | moderate via glob, minimatch |
| jsdom | ^30.0.1 (25) | 30.0.1 | 31.0.1 | pulls undici 8.10.0 (19 advisories, fixed in 8.12.2) |
| @testing-library/react | ^16.0.0 (20) | 16.3.2 | 16.3.3 | |
| @testing-library/jest-dom | ^6.4.8 (19) | 6.9.1 | 8.0.0 | 2 majors |
| typescript | ^5.5.3 (26) | 5.9.3 | 7.0.2 | 2 majors |

`npm audit` total: 11 findings (2 critical, 4 high, 5 moderate), every one in `devDependencies` or their transitive tree. The two runtime dependencies have none.

### CI actions

| Action | Pinned | Used in | Latest |
|---|---|---|---|
| actions/checkout | v4 | every workflow | v7.0.1 |
| actions/setup-go | v5 | `ci.yml:21` | v7.1.0 |
| actions/setup-node | v4 | `ci.yml:74` | v7.1.0 |
| actions/upload-artifact | v4 | `ci.yml:60`, `e2e.yml:97` | v7.0.2 |
| golangci/golangci-lint-action | v6, with `version: latest` | `ci.yml:41`, `ci.yml:43` | v9.3.0 (golangci-lint v2.15.0) |
| anthropics/claude-code-action | v1 | `claude-review.yml:74`, `claude-issue-triage.yml:63` (both on `main` only) | v1, current |

All are major-tag pins, none are SHA pins. The repo has no Dependabot or Renovate config.

### Other unpinned inputs

- `install.sh:155` pipes `https://get.docker.com` into a shell when Docker is missing (interactive opt-in).
- `scripts/update-geoip.sh:28` and `:33` pull the Iran CIDR list from `arastu/mikrotik-iran-address-list` (branch `main`, last push 2026-11-15) and RIPE stat. The weekly workflow commits the result to `main` directly.
- `web-ui/Dockerfile:4` runs `npm install`, not `npm ci`, so the image build re-resolves the caret ranges and can ship a different tree than the lockfile CI tested.

## Per-component findings

### proxy-core Go toolchain: 1.25 to 1.27 (P0)

- Go 1.25 stopped receiving fixes on 2026-08-19. `golang:1.25-alpine` is frozen at 1.25.14, so every proxy-core image built today carries a standard library that misses the fixes shipped in Go 1.26.9, 1.26.10 and 1.26.11 (October and November 2026).
- `govulncheck` under go1.25.14 reports **11 standard-library vulnerabilities reachable from proxy-core code**. The ones that map to what proxy-core does:
  - **CVE-2026-94439** (GO-2026-6613), HTTP/1 server connection desynchronization after a 2xx CONNECT response. proxy-core is an HTTP CONNECT proxy (`proxy/handler.go`, served from `proxy/server.go:117`).
  - **CVE-2026-97054** (GO-2026-6859), HTTP/1 server crash from a `MaxBytesReader` header race; **CVE-2026-94440** (GO-2026-6608), memory limit bypass when parsing MIME headers. Both sit on the API server (8088) and the CONNECT listener.
  - **CVE-2026-111943** (GO-2026-6809), `crypto/x509` now distrusts a set of compromised intermediate certificates (fixed in 1.26.10 and 1.27.3). proxy-core verifies TLS for the subscription fetch (`subscription/fetcher.go:14`) and the egress-IP lookups (`api/api.go:313`, `:326`). An on-path attacker holding one of those intermediates could forge the subscription host's certificate and inject endpoints.
  - **CVE-2026-56866** (GO-2026-6605) and **CVE-2026-78660** (GO-2026-6610), HTTP client desynchronization and malformed HTTP/2 framing headers, both on the subscription fetch path.
  - CVE-2026-97031 (ECH parsing in `crypto/tls`) and four HTTP/2 server issues (CVE-2026-78659, CVE-2026-78663, CVE-2026-78669, CVE-2026-97032) are reported through `net/http`; the API server runs plain HTTP/1, so these are unlikely to be reachable in practice.
- The scratch image's CA bundle is copied from the same frozen builder (`proxy-core/Dockerfile:22`), so its root store is also from 2026-08.
- Exposure: the listeners bind to 127.0.0.1 by default. The dashboard's Network tab can switch them to LAN or public, and that is the case that matters.
- Scope of the change: `proxy-core/Dockerfile:1`, `proxy-core/go.mod:3`, `.github/workflows/ci.yml:15`. Go 1.27 is the target, to match the server plan and the longest support window.
- Risk: **low to medium**. Three small dependencies and a clean `go vet`, race tests and a Windows and macOS cross-compile in CI. Not verified: behaviour changes between 1.25 and 1.27 in `net/http` under the e2e run.

### MPTCP on the proxy listeners (P0, config not version)

- Go 1.24+ enables MPTCP on TCP listeners by default, and the public proof of concept for Linux kernel **CVE-2026-98432** (use-after-free in IPv4 TCP SYN handling, CVSS 9.8, NVD status Analyzed) needs an MPTCP listener. Xray's v26.10.31 notes say explicitly that client-side SOCKS and HTTP inbounds are at risk when "allow connections from LAN" is on. That is moav-client's LAN and public exposure mode.
- Go listeners in the stack, by build toolchain:

| Service | Published port | Go version | MPTCP listener today |
|---|---|---|---|
| proxy-core | 1080, 8081, 8088 (`docker-compose.yml:26-28`) | 1.25.14 | **Yes.** No `GODEBUG` or `SetMultipathTCP` anywhere in the tree |
| sing-box | none (moav-net only) | 1.25.12 (v1.13.19) | No. Official builds set `multipathtcp=0` (`release/LDFLAGS` at v1.13.19, v1.14.0 and v1.14.4) |
| xray | none (moav-net only) | upstream build | Yes until v26.10.31+, but only reachable from other containers |
| caddy (profile `https`) | 8443, 8444 | upstream build | Assumed yes. Not verified |
| psiphon, sni-spoof, amneziawg-go, masterdns | none | 1.25 or 1.26 | internal or no TCP listener |

- nginx (the dashboard on 3001), microsocks, socat, tor and the TrustTunnel client are C, C++ or Rust and are not affected by the Go default.
- Recommendation: add `godebug multipathtcp=0` to `proxy-core/go.mod` so the binary is safe however it is run (`CLAUDE.md` section 8 documents running it outside Docker), and set `GODEBUG=multipathtcp=0` in the compose `environment` of xray, caddy, psiphon and sni-spoof. The Go team's position (golang/go#82717, still open) is that a patched kernel is the real fix; disabling MPTCP only removes the known exploit path. On Docker Desktop the kernel in question is the Docker VM's.
- Risk: **low**. No protocol in the stack depends on MPTCP.

### Xray-core: v26.7.28 to v26.11.9 (P0)

- **The current pin is a pre-release.** Upstream marks v26.7.28 `prerelease`; the stable line is v26.3.27, then v26.10.21, v26.10.31, v26.11.9.
- **Security.** v26.10.21 and v26.10.31 are both labelled emergency security updates upstream:
  - v26.10.10 made the TLS client use Xray's built-in root CA set instead of the OS store (#7105), and v26.10.21 removed three roots after the intermediate-certificate compromise and moved the build to Go 1.27.3 (#7212). The pinned build trusts the OS store of the `alpine:3.21` runtime image, which is EOL. This matters for the TLS-verified XHTTP endpoints the sidecar dials; Reality endpoints do not rely on the CA store.
  - v26.10.31 disables MPTCP on listeners by default (#7291). The sidecar's SOCKS inbounds are only on `moav-net`, so exposure here is low.
  - Also in range: QUIC sniffer rejects a ClientHello with gaps in CRYPTO data (#7292), failed inbounds are closed and removed (#7223), XHTTP client race and data-race fixes (#6665, #6694), XHTTP `GetBody` so HTTP/2 can replay after GOAWAY (#6632), mux client race fixes (#6871, #6822).
  - GHSA-5wf9-h793-w73c (`pinnedPeerCertSha256`) was fixed in v26.7.11 and does not affect the pin.
- **Censorship resistance.** uTLS registers `hellochrome_155` and makes it the default `chrome` fingerprint (#7135). Upstream reports it is not yet targeted by TSPU; that claim is theirs and is not verified here. The client generator defaults to `"fingerprint": "chrome"` (`proxy-core/xray/generator.go:254`, `:263`), so the new fingerprint arrives with the bump and no generator change.
- **Behaviour changes to check.** mux.cool defaults rose to `maxConcurrency` 128 and `maxReuseTimes` 1024. `blockDelay` became `delay` in Freedom `finalRules`, `fragment` and `noises` now warn about moving to Finalmask, and XDNS finalmask keys were renamed (#7090). The client generator emits none of `fragment`, `noises`, `mux`, `finalmask` or `finalRules` (grep of `proxy-core/xray/generator.go`). The transport layer was refactored onto Finalmask's dialer and listener (#6754), so XHTTP needs an e2e pass.
- **Build detail.** Xray requires Go 1.27 since v26.9.8. The prebuilt path is unaffected (`Xray-linux-64.zip` and `Xray-linux-arm64-v8a.zip` exist for v26.11.9), but the source fallback at `sidecars/xray/Dockerfile:10` uses `golang:1.26-alpine` and would fail. Move it to `golang:1.27-alpine` in the same PR, and move the runtime stage off `alpine:3.21`.
- Risk: **low to medium**. Config is drop-in; 195 commits is a long jump, so run the XHTTP endpoints on the e2e runner.
- Recommendation: bump now. The server will move from v26.10.21 to v26.11.9 under its own audit; the client can go first.

### sing-box: 1.13.19 to 1.14.4 (P0)

- `main` ships 1.13.19; `dev` has carried 1.14.0 since PR #32 (2026-09-11), unreleased. The server is on 1.14.4.
- **Security.** v1.13.19 was built with Go 1.25.12 and v1.14.0 with Go 1.26.7 (`go-version` in upstream's build and docker workflows at each tag). Both predate the Go 1.26.9 and 1.26.10 fixes, including the `crypto/x509` distrust (CVE-2026-111943). v1.14.4 is built with Go 1.26.10, which has them. sing-box does the TLS verification for every Trojan, VLESS over WS or CDN, AnyTLS and Hysteria2 endpoint, so this is the binary where the certificate fix matters most. The 1.13 line has had no release since v1.13.21 (2026-08-30). sing-box has no published advisory in this range; the notes for 1.14.1 to 1.14.4 say only "Fixes and improvements".
- **Already done on `dev`.** The 1.14.0 PR validated the generator output against the real `sing-box check`: WireGuard uses `endpoints[]`, no legacy DNS, `block`, `dns` or `sniff` constructs.
- **Still to check.** Hysteria2 client connections now parrot Chrome's QUIC handshake by default, and Chrome does not offer Ed25519, so a server with an Ed25519 certificate fails the handshake unless the outbound sets `disable_chrome_parrot`. The generator's Hysteria2 block (`proxy-core/singbox/generator.go:412`) does not emit it. MoaV servers use ACME certificates, so they should be unaffected; third-party Hysteria2 endpoints with self-signed Ed25519 certificates would break. Not verified.
- Fields deprecated in 1.14 are removed in 1.16 (legacy DNS address filter fields, `independent_cache`, `store_rdrc`, inline `tls.acme`, `download_detour`). The generator emits none of them.
- `ghcr.io/sagernet/sing-box:v1.14.4` exists.
- Risk: **low** from `dev` (four patch releases), **medium** from `main` (minor line change, mitigated by the validation already done).
- Recommendation: set 1.14.4 on `dev` and release it.

### web-ui runtime image: nginx 1.27 to 1.30 (P0)

- `nginx:1.27-alpine` resolves to nginx 1.27.5 on a base image last built 2025-04-16. The 1.27 mainline branch ended 2025-06-24.
- nginx.org lists about 30 advisories whose vulnerable range includes 1.27.5. Most are in modules this config does not use (HTTP/2, HTTP/3, mail, stream, mp4, dav, slice, SSI). The one that touches a configured feature is **CVE-2026-123634**, a use-after-free in the resolver: `web-ui/nginx.conf:28` sets `resolver 127.0.0.11` for the variable `proxy_pass`. The resolver only talks to Docker's embedded DNS, so reachability is low.
- The larger problem is the base: 19 months of Alpine package fixes (OpenSSL, busybox, and the `apache2-utils` that generates the dashboard's basic-auth file) are missing from every dashboard image built today.
- The dashboard port (3001) is LAN-exposable through the same Network tab as the proxy ports.
- Recommendation: `nginx:1.30-alpine` (stable branch, rebuilt 2026-11-20). The config is a plain `server` block with `map`, `resolver`, `proxy_pass` and `try_files`, all unchanged in 1.30. The entrypoint hooks in `/docker-entrypoint.d` use the same official image contract.
- Risk: **low**, drop-in.

### web-ui dev toolchain (P0, low reachability)

- `npm audit` on the lockfile: 11 findings. None is in the bundle users receive, because the image ships only the built static files and the two runtime dependencies (react, react-dom) are clean. The exposure is to developer machines and CI.
- **vite 8.2.2** (high): cross-origin script execution through custom middleware (GHSA-9jrq-w75r-8gcw), files outside `server.fs.allow` served (GHSA-rq7h-c2jc-7f22), partial file disclosure through `?vite-wasm-instance` (GHSA-vfpm-58rq-9qcg), and three `launch-editor` issues. All are dev-server issues, hit by anyone running `npm run dev`. Fixed after 8.2.3; 8.3.8 is in range for `^8.2.2`, so this is a lockfile refresh.
- **vitest 3.2.7** (critical): tinypool prototype pollution leading to code execution (GHSA-5gmw-xhrv-c9v3, GHSA-85c8-ppgw-ccpr) and @vitest/mocker path traversal and unauthenticated mock registration (GHSA-82fw-gwwq-j7x9, GHSA-qxg6-89mg-rqv8). The V3 line has no fix; the fixed versions are 4.1.12 and 5.1.1. That is a major bump for `vitest` and `@vitest/coverage-v8` together.
- Transitive, fixed by a lockfile refresh: undici 8.10.0 (19 advisories, fixed in 8.12.2), brace-expansion, source-map-js.
- Risk: **low** for the lockfile refresh. **Medium** for vitest 3 to 4 (two test files, `display.test.ts` and `theme.test.ts`, plus `vitest.config.ts`).

### amneziawg-go: v3.1.20260814 to v3.1.20261116 (P1)

- 13 commits. Hardening of the AWG 3.1 obfuscation parameters: `jmin > jmax` is rejected instead of underflowing the junk packet size, negative length arguments in `<r>/<rc>/<rd>/<dz>` tags are rejected, I1 to I5 are safe to change while handshakes are being sent, S4 changes are checked in the encryption queue, a timer race under `modifyingLock` is fixed, and the UDP window is used for random padding.
- awg-tools has no newer release; v3.1.20260812 stays matched to the 3.1 line.
- The tag's `go.mod` still says `go 1.25.0`, so it builds on any current builder.
- Risk: **low**, drop-in, same wire format. Bump the builder image (`golang:1.25-bookworm`, EOL) in the same PR.

### MasterDNS client: v2026.05.10 to v2026.11.18 (P1)

- Client-side changes are the ones that matter here: FastStart with background resolver discovery and scoring (#205), a fix for resolver probation starvation in the LossThenLatency strategy (#206), data-race fixes in session init and the resolver health loop (#213), and `golang.org/x/crypto` 0.49.0 to 0.52.0.
- The release assets keep their names (`MasterDnsVPN_Client_Linux_AMD64.tar.gz`, `..._ARM64.tar.gz`), so `sidecars/dns-tunnels/Dockerfile` keeps working.
- Not verified: whether the `client_config.toml` that `proxy-core/sidecars/configgen.go` writes is still accepted unchanged by the FastStart client, and whether the new defaults suit a Docker bridge network. The Dockerfile also downloads without a checksum; the server's `Dockerfile.masterdns` verifies `SHA256SUMS` and the client should do the same.
- Risk: **low to medium**. Needs an e2e run with the `masterdns` profile.

### TrustTunnel client: v1.0.49 to v1.3.0 (P3)

- Tied to the server. The server runs endpoint 1.1.0 with client 1.0.49 and its own audit puts the 1.3.x move at P3. The client sidecar should move in the same step, not before.
- What changes: the QUIC stack moved from quiche to ngtcp2 in 1.1.5, the client now gives up after a bounded number of recovery attempts (breaking: it used to retry forever, which is what a long-running sidecar wants), default MTU rose to 1350, and 1.3.0 adds per-endpoint `tls_profile`, key-derived `client_random` authentication and current Chrome and Firefox ClientHello profiles. 1.1.5 also records a certificate-verification bug on HTTP/3 that existed only in its own betas.
- `configgen.go:190` rewrites the bundle's `[listener.tun]` into a SOCKS listener on 127.0.0.1:5601. Not verified that 1.3.0 keeps the same listener keys or the loopback-only rule.
- The 1.3.0 assets keep the names the Dockerfile expects.
- Risk: **high**. Isolated PR, e2e on HTTP/2 and HTTP/3, together with the server.

### golang.org/x/net: v0.58.0 to v0.63.0 (P2, rides with the toolchain PR)

- Seven advisories are filed against the module at v0.58.0: five in `x/net/http2` (fixed in v0.60.0) and two in `x/net/html` (fixed in v0.62.0). proxy-core imports neither package, and govulncheck confirms none is called.
- Worth bumping anyway so scanners stop flagging the module. Risk: **low**.

### Routine web-ui bumps (P2)

- react and react-dom 19.3.1, `@types/*` 19.3.0, @vitejs/plugin-react 6.3.0, @testing-library/react 16.3.3: all inside the existing caret ranges. jsdom 31.0.1 is one major. Risk: **low**.
- Node 22 is supported until 2027-04; moving the build image and CI to 24 is optional this cycle.

### Go builders and base images for sidecars (P2)

- `golang:1.25-bookworm` builds amneziawg-go and SNI-Spoofing-Go. Neither verifies TLS, so the exposure is smaller than for proxy-core, but the binaries still miss three rounds of Go fixes. Move both to 1.27.
- `debian:bookworm` is supported; moving to trixie is optional and touches five Dockerfiles.

### CI (P2)

- **The golangci-lint step is not linting.** `golangci-lint-action@v6` with `version: latest` installs golangci-lint v1.64.8, which is built with Go 1.24 and refuses a Go 1.25 module ("the Go language version (go1.24) used to build golangci-lint is lower than the targeted Go version (1.25.0)", exit 3). `continue-on-error: true` hides it; the step shows green (run 40796474575 on `main`, 2026-10-29). Action v9 with golangci-lint v2 fixes it and needs a v2-format config if one is added.
- checkout, setup-go, setup-node and upload-artifact are each three majors behind. Bump and pin by SHA.

### Components that are current

- **amneziawg-tools v3.1.20260812**: latest.
- **Caddy** (`caddy:2`) and **Psiphon** (HEAD of `master`): current because they float, not because they are pinned.
- **SNI-Spoofing-Go**: HEAD equals v0.7.2.
- **anthropics/claude-code-action v1**, `golang:1.26-bookworm`, `debian:bookworm-slim`, `node:22-alpine`: supported.

## Unmaintained or archived upstreams

| Component | Evidence | Recommendation |
|---|---|---|
| **gopkg.in/yaml.v3** (`go-yaml/yaml`) | Repository **archived** (last push 2025-04). v3.0.1 dates from 2022-05. No advisory applies to v3.0.1. | Move to the maintained fork `go.yaml.in/yaml/v3` (v3.0.6, same API, an import-path change in three files). v4.0.0 came out on 2026-11-04; leave it for later. |
| **github.com/armon/go-socks5** | Last commit 2016-09-02, no tags, 32 open issues. No advisory on record, but nothing would be fixed if one appeared, and it is the code behind the SOCKS5 listener on 1080. | Plan a replacement. `github.com/things-go/go-socks5` (v0.1.4, 2026-10-28) is the maintained descendant; the API differs, so this is design work. |
| **golang.org/x/net/websocket** | Not archived, but its own package documentation says it lacks features and points to a more actively maintained alternative. Already listed in `CLAUDE.md` section 7. | Replace when the API server is next touched. No urgency. |
| **peterdavehello/tor-socks-proxy** | Repository active (last push 2026-11-04), but `:latest` was last published 2026-06-08 and there are no version tags. It installs `tor` from Alpine edge at build time, so the Tor inside is as old as the image. | Pin by digest now. Consider a small in-repo Dockerfile (Alpine plus `tor`) so Tor updates come with a rebuild. |
| **aleskxyz/SNI-Spoofing-Go** | No commits since 2026-06-01. Young project, one maintainer. | Pin to v0.7.2 and re-check at the next audit. |

## Plan

### P0: security fixes, do first

| # | Change | Config or code changes | Batching |
|---|---|---|---|
| P0-1 | proxy-core Go 1.25 to 1.27, x/net v0.63.0 | `proxy-core/Dockerfile:1`, `proxy-core/go.mod:3` and `:7`, `go.sum`, `.github/workflows/ci.yml:15`. Re-run govulncheck and expect zero reachable findings. | PR A |
| P0-2 | MPTCP off on Go listeners | `godebug multipathtcp=0` in `proxy-core/go.mod`; `GODEBUG=multipathtcp=0` in the compose `environment` of xray, caddy, psiphon, sni-spoof. | PR A (same theme, same test run) |
| P0-3 | Xray-core v26.7.28 to v26.11.9 | `.env.example:63`, `docker-compose.yml:116`, `sidecars/xray/Dockerfile:15`; builder to `golang:1.27-alpine` (`:10`), runtime to `alpine:3.24` (`:49`). e2e: XHTTP endpoints. | PR B |
| P0-4 | sing-box to 1.14.4 | `.env.example:60` and `:76`, `docker-compose.yml:64`. On `dev` this is 1.14.0 to 1.14.4. Decide on `disable_chrome_parrot` for Hysteria2. | PR B (both are engine bumps, one e2e pass), or split if either fails |
| P0-5 | Dashboard runtime to `nginx:1.30-alpine` | `web-ui/Dockerfile:10`. Smoke test: dashboard loads, `/api` and `/api/ws` proxy, basic auth on and off, `/healthz`. | PR C |
| P0-6 | web-ui dev toolchain | Lockfile refresh (vite 8.3.8, undici, brace-expansion, source-map-js), then vitest and @vitest/coverage-v8 to 4.1.12. `npm audit` should come back clean. | PR C for the refresh; the vitest major can follow as PR D if it needs test changes |

PR A, B and C can ship together as 1.4.1, or as 1.5.0 if the sing-box minor line is judged a minor release.

### P1: censorship-resistance and protocol improvements

| # | Change | Config or code changes | Batching |
|---|---|---|---|
| P1-1 | Chrome 155 uTLS fingerprint | Arrives with P0-3; the generator already defaults to `chrome`. | with PR B |
| P1-2 | amneziawg-go v3.1.20261116 | `.env.example:66`, `docker-compose.yml:218`, `sidecars/amneziawg/Dockerfile:24`; builder to Go 1.27 (`:17`). e2e with the `amneziawg` profile. | PR E |
| P1-3 | MasterDNS client v2026.11.18 | `.env.example:70`, `docker-compose.yml:205` and `:260`, `sidecars/dns-tunnels/Dockerfile:12`; add the `SHA256SUMS` check. e2e with the `masterdns` profile. | PR E, or split if either fails |
| P1-4 | Re-sync pins on update | Make `moav-client update` (and the installer's update path) refresh the `*_VERSION` lines of an existing `.env` from `.env.example`, or stop seeding them into `.env` so the compose defaults apply. Without this, none of the bumps above reaches an existing install: `install.sh:597` copies the pins into `.env` once and nothing updates them. | PR F, ideally before the release that carries PR A to C |

### P2: routine bumps and hygiene

| # | Change | Notes | Batching |
|---|---|---|---|
| P2-1 | web-ui routine bumps | react 19.3.1, types, plugin-react, testing-library, jsdom 31. | PR G |
| P2-2 | SNI-spoof builder to Go 1.27 | `sidecars/sni-spoof/Dockerfile:12`. | PR E |
| P2-3 | Pin what floats | Psiphon to `v2.0.43` and SNI-Spoofing-Go to `v0.7.2` (`git clone --branch`), Tor image by digest, Caddy to `2.11`. Add `PSIPHON_VERSION` and `SNI_SPOOF_VERSION` to `.env.example` and to `tests/version-sync-test.sh`. Switch `web-ui/Dockerfile:4` to `npm ci`. | PR H |
| P2-4 | CI actions | checkout v7, setup-go v7, setup-node v7, upload-artifact v7, golangci-lint-action v9, all SHA-pinned. Confirm the lint step really runs, then decide whether it should block. | PR I |
| P2-5 | Documentation | The server-parity claim at `.env.example:49` should be kept true by process (bump both repos together) or reworded. | PR I |

### P3: needs design work

| # | Item | What has to be decided |
|---|---|---|
| P3-1 | TrustTunnel client 1.3.0 | Only together with the server's endpoint 1.3.1. Bounded recovery attempts in a sidecar, listener config compatibility, e2e on HTTP/2 and HTTP/3. |
| P3-2 | Replace `armon/go-socks5` | Candidate `things-go/go-socks5`. Auth, rule hooks and the custom dialer in `proxy/server.go` need to map across. |
| P3-3 | `gopkg.in/yaml.v3` to `go.yaml.in/yaml/v3` | Mechanical, but `api/yamledit.go` depends on comment and formatting round-trips, so compare output before and after. |
| P3-4 | vitest 5, TypeScript 7, jest-dom 8 | Three majors in the dev toolchain. TypeScript 7 is two majors on, so check the `tsc` step in `npm run build`. Take them one at a time after P0-6. |
| P3-5 | Own Tor image | Replace the third-party image with a small in-repo Dockerfile, or keep it and pin by digest (P2-3). |
| P3-6 | Automated update tracking | Renovate with a regex manager over `.env.example`, plus the gomod, npm, dockerfile and github-actions managers, would have opened most of these as PRs. Same recommendation as the server audit (its P3-5); one shared config would serve both repos. |
| P3-7 | Debian trixie, Node 24 | Optional base moves. No support deadline before 2027-04. |

### Suggested order

1. PR A (Go toolchain plus MPTCP), PR B (Xray, sing-box), PR C (nginx, lockfile refresh), and PR F (pin re-sync) so the release actually reaches existing installs.
2. PR D (vitest major) and PR E (amneziawg-go, MasterDNS, SNI-spoof builder).
3. PR G, H, I (routine web-ui bumps, pinning, CI), one at a time.
4. P3 items as separate design threads. TrustTunnel follows the server.

## Sources

- Release data: `gh api repos/<owner>/<repo>/releases` and `/tags` for SagerNet/sing-box, XTLS/Xray-core, amnezia-vpn/amneziawg-go (tags only), amnezia-vpn/amneziawg-tools, masterking32/MasterDnsVPN, TrustTunnel/TrustTunnelClient, TrustTunnel/TrustTunnel, Psiphon-Labs/psiphon-tunnel-core, aleskxyz/SNI-Spoofing-Go, PeterDaveHello/tor-socks-proxy, caddyserver/caddy, armon/go-socks5, go-yaml/yaml, yaml/go-yaml, and the five GitHub Actions repositories.
- Xray: commit list `v26.7.28...v26.11.9` (195 commits), release notes for v26.10.21, v26.10.31 and v26.11.9, XTLS/Xray-core#7105, #7135, #7212, #7291, advisory GHSA-5wf9-h793-w73c.
- sing-box: release notes for v1.14.0, `release/LDFLAGS` and `.github/workflows/build.yml` at v1.13.19, v1.14.0 and v1.14.4, GHCR manifests.
- MPTCP: https://github.com/XTLS/Xray-core/releases/tag/v26.10.31, https://github.com/golang/go/issues/82717, https://nvd.nist.gov/vuln/detail/CVE-2026-98432.
- Go: https://proxy.golang.org (module versions), https://pkg.go.dev/vuln (GO-2026-6603 to GO-2026-6617, GO-2026-6809, GO-2026-6859, GO-2026-6860, GO-2026-6866), `govulncheck` v1.10.0 run on `proxy-core` with `GOTOOLCHAIN=go1.25.14`.
- npm: https://registry.npmjs.org (dist-tags), `npm audit --package-lock-only` on `web-ui/package-lock.json`, GitHub Advisory Database for the GHSA ids.
- nginx: https://nginx.org/en/security_advisories.html.
- MoaV server pins: `.env.example` on `MotherofallVPNs/MoaV` `main`, and `docs/maintenance/dependency-audit-2026-11.md` in MoaV PR #354.
- Support windows: https://endoflife.date (Go, Alpine, Debian, Node.js, nginx). Image freshness: Docker Hub tag metadata.
- CI evidence: workflow run 40796474575 (`ci.yml` on `main`, 2026-10-29), proxy-core job log.
