# Configuration model

moav-client has two config files with distinct jobs. Knowing which owns what
avoids the "I changed it and nothing happened" traps.

| File | Layer | Owns | Read by |
|---|---|---|---|
| `.env` | deploy / infra | component **versions**, host **exposure** (binds), proxy **ports**, dashboard/SOCKS **auth** | docker-compose + proxy-core |
| `config.yaml` | app / VPN | endpoints & subscriptions, load-balancing, routing rules, sidecar enable + per-sidecar config, sing-box/xray port coordination | proxy-core |

Rule of thumb: **infrastructure and versions live in `.env`; VPN behaviour lives
in `config.yaml`.** Copy the templates before first run:
`cp .env.example .env` and `cp config.yaml.example config.yaml`.

## Versions — single source of truth

- **App version (`MOAV_VERSION`)**: the `VERSION` file. The `moav-client` wrapper
  exports it into every `docker compose` call, so the binary, dashboard footer,
  and `moav-client version` all report what's in `VERSION`. The literals in
  `docker-compose.yml`, `proxy-core/Dockerfile`, and `proxy-core/cmd/cli.go` are
  fallbacks for a bare `docker compose build` / `go build`; `tests/version-sync-test.sh`
  fails if any drifts from `VERSION`.
- **Component versions**: uncommented `_VERSION` vars in `.env` (`SINGBOX_VERSION`,
  `XRAY_VERSION`, `AMNEZIAWG_GO_VERSION`, `AWGTOOLS_VERSION`, `MASTERDNS_VERSION`,
  `TRUSTTUNNEL_CLIENT_VERSION`). These are the single source; `docker-compose.yml`
  passes them as build args / image tags with matching `${VAR:-default}` fallbacks
  (kept in sync by the same test), and the values track the MoaV server's
  `.env.example`. Pulled images can be swapped wholesale with `IMAGE_SINGBOX`,
  `IMAGE_TOR`, `IMAGE_CADDY`.

Change a component version and rebuild just that piece:
`moav-client up` (or `docker compose up -d --build`).

## Ports — `.env` is the source, `config.yaml` is the base

`SOCKS5_PORT`, `HTTP_PORT`, and `API_PORT` in `.env` are authoritative:

1. **proxy-core** reads them and binds its listeners, overriding
   `config.yaml → proxy.*_port`.
2. **docker-compose** maps the same container ports.
3. **nginx** (dashboard) points its `/api` upstream at `API_PORT` on start.

`config.yaml`'s port fields are the declarative fallback — if `.env` is absent
(e.g. running the binary directly), those apply. Under compose, change ports in
`.env`, not `config.yaml`, so all three stay in sync.

## Auth — a deliberate override chain, not a duplicate

Auth intentionally lives in two places with a defined precedence:

- **SOCKS5 auth**: `config.yaml → proxy.auth` is the declarative base;
  `.env` `SOCKS5_USERNAME` / `SOCKS5_PASSWORD` **override** it. The dashboard's
  Network tab writes to `.env` so a change survives a plain restart without
  editing `config.yaml`. Precedence: `.env` file > process env > `config.yaml`.
- **Dashboard / API auth**: `.env` `MOAV_DASHBOARD_USER` / `MOAV_DASHBOARD_PASS`
  only — enforced by nginx (basic auth) and the API. Not in `config.yaml`.

Do not "de-duplicate" the SOCKS5 auth by removing a layer: the dashboard relies
on the `.env` override existing.
