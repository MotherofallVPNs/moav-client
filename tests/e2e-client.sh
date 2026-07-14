#!/usr/bin/env bash
# =============================================================================
# moav-client live e2e — brings the client stack up against a REAL MoaV server
# bundle, then proves the tunnel works end to end:
#   1. the bundle's endpoints get parsed,
#   2. moav-client's own /api/probe validates ≥1 endpoint (tunnel + TLS
#      handshake — the same check the dashboard's Probe button runs), and
#   3. a fetch through the aggregate SOCKS5 :1080 exits from the SERVER's IP,
#      not the runner's (proves traffic is actually tunnelled).
#
# It does NOT build a MoaV server — it consumes a bundle/subscription you
# provide (see docs/devdocs/E2E-TESTING.md). Runs on a self-hosted runner via
# .github/workflows/e2e.yml, or by hand:
#
#   MOAV_TEST_SUB_URL=moav://…  MOAV_TEST_EXIT_IP=203.0.113.9  bash tests/e2e-client.sh
#   # or provide a bundle file instead of a URL:
#   MOAV_TEST_BUNDLE=/path/to/bundle.zip  MOAV_TEST_EXIT_IP=…  bash tests/e2e-client.sh
#
# Env:
#   MOAV_TEST_SUB_URL   a moav:// / subscription URL to import (or…)
#   MOAV_TEST_BUNDLE    …a path to a MoaV bundle .zip to import instead
#   MOAV_TEST_EXIT_IP   the server's expected egress IP (the assertion). If
#                       unset, we only assert the fetch is proxied (IP != runner)
#   API                 API base (default http://127.0.0.1:8088)
#   SOCKS               aggregate SOCKS5 (default 127.0.0.1:1080)
#   COMPOSE_UP          command to bring the stack up (default: docker compose up -d --build)
# =============================================================================
set -euo pipefail

API="${API:-http://127.0.0.1:8088}"
SOCKS="${SOCKS:-127.0.0.1:1080}"
EXIT_IP="${MOAV_TEST_EXIT_IP:-}"
ART="${ART_DIR:-.}"; mkdir -p "$ART"

log()  { printf '\033[0;34m[e2e]\033[0m %s\n' "$*"; }
fail() { printf '\033[0;31m[e2e] FAIL:\033[0m %s\n' "$*" >&2; exit 1; }

# Redact a server IP/domain for logs: keep only first+last char (and the TLD for
# domains), e.g. 203.0.113.9 -> 2…9, sub.example.com -> s…e.com. The raw values
# are still used for the actual assertion — this only affects what's printed, so
# a public CI log doesn't leak the test server's address.
mask() {
  local v="$1"
  case "$v" in ""|"<none>"|unknown|"proxied"*) printf '%s' "$v"; return ;; esac
  if [[ "$v" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    printf '%s…%s' "${v:0:1}" "${v: -1}"
  elif [[ "$v" == *.* ]]; then
    local tld="${v##*.}" name="${v%.*}"
    printf '%s…%s.%s' "${name:0:1}" "${name: -1}" "$tld"
  else
    printf '%s…%s' "${v:0:1}" "${v: -1}"
  fi
}

[[ -n "${MOAV_TEST_SUB_URL:-}" || -n "${MOAV_TEST_BUNDLE:-}" ]] \
  || fail "provide MOAV_TEST_SUB_URL or MOAV_TEST_BUNDLE (a server bundle to connect through)"

# --- 1. configure the source -------------------------------------------------
# Start from the example config, then point the subscription at the provided
# bundle/URL. NB: subscription.file wants a *subscription.txt* (a list of URIs),
# NOT a bundle .zip — so we unzip the bundle and reference the files inside.
cp config.yaml.example config.yaml
if [[ -n "${MOAV_TEST_BUNDLE:-}" ]]; then
  mkdir -p data/e2e-bundle
  if command -v unzip >/dev/null 2>&1; then
    unzip -o -q "$MOAV_TEST_BUNDLE" -d data/e2e-bundle
  elif command -v python3 >/dev/null 2>&1; then
    python3 -m zipfile -e "$MOAV_TEST_BUNDLE" data/e2e-bundle
  else
    fail "need 'unzip' or python3 on the runner to extract the bundle"
  fi
  # Locate subscription.txt wherever it landed — a bundle may be flat or nested
  # under a top-level dir depending on how it was zipped.
  sub=$(find data/e2e-bundle -type f -name subscription.txt | head -1)
  if [[ -z "$sub" ]]; then
    echo "extracted files:"; find data/e2e-bundle -type f | sed 's/^/  /' | head -40
    fail "bundle has no subscription.txt (is it a MoaV user bundle?)"
  fi
  bdir=$(dirname "$sub")
  # WireGuard / AmneziaWG come as their own .conf files (one endpoint each).
  wg=""
  for f in wireguard.conf amneziawg.conf; do
    [[ -f "$bdir/$f" ]] && wg="${wg}\"$bdir/$f\", "
  done
  log "using bundle: $sub + [${wg%, }]"
  sed -i.bak "s|^  file: \"\".*|  file: \"$sub\"|" config.yaml
  sed -i.bak "s|^  wireguard_files: \[\].*|  wireguard_files: [${wg%, }]|" config.yaml
  rm -f config.yaml.bak
else
  log "using subscription URL from MOAV_TEST_SUB_URL"
  sed -i.bak "s|^  url: \"\".*|  url: \"${MOAV_TEST_SUB_URL}\"|" config.yaml
  rm -f config.yaml.bak
fi

# Tunnel-only: kill the involuntary direct fallback so a downed/undialable
# endpoint can't make the exit-IP check pass on the runner's own IP. With this,
# a fetch through :1080 either egresses from the server or fails cleanly.
sed -i.bak 's|^  block_direct: false|  block_direct: true|' config.yaml && rm -f config.yaml.bak

# --- 2. bring the stack up ---------------------------------------------------
# docker-compose.yml declares `env_file: .env` (and bind-mounts ./.env), so
# compose refuses to start without it. Seed from the example (values are
# irrelevant for a loopback e2e run).
[[ -f .env ]] || cp .env.example .env 2>/dev/null || touch .env
log "bringing the client stack up…"
${COMPOSE_UP:-docker compose up -d --build}

cleanup() { log "tearing down…"; docker compose down -v 2>/dev/null || true; }
trap cleanup EXIT

# --- 3. wait for the API + parsed endpoints ---------------------------------
log "waiting for the API…"
for i in $(seq 1 60); do
  curl -fsS "$API/api/healthz" >/dev/null 2>&1 && break
  [[ $i -eq 60 ]] && fail "API never came up on $API"
  sleep 2
done

log "waiting for endpoints to be parsed from the bundle…"
n=0
for i in $(seq 1 30); do
  curl -fsS "$API/api/endpoints" -o "$ART/endpoints.json" 2>/dev/null || true
  n=$(jq '.endpoints | length' "$ART/endpoints.json" 2>/dev/null || echo 0)
  [[ "${n:-0}" -ge 1 ]] && break
  sleep 2
done
[[ "${n:-0}" -ge 1 ]] || fail "no endpoints parsed from the provided bundle"
log "parsed $n endpoint(s):"
jq -r '.endpoints[] | "\(.Protocol)\t\(.ID)"' "$ART/endpoints.json" 2>/dev/null \
  | while IFS=$'\t' read -r proto id; do printf '  %s\t%s\n' "$proto" "$(mask "$id")"; done || true

# --- 4. probe (async) then poll endpoints for status ------------------------
# POST /api/probe returns 202 immediately and probes in a goroutine; results
# land as each endpoint's status. Poll until ≥1 is "ok" (tunnels take a moment;
# DNS-tunnel protocols are the slowest).
log "triggering probe + waiting for an endpoint to validate…"
curl -fsS -X POST "$API/api/probe" >/dev/null 2>&1 || true
oks=0
for i in $(seq 1 40); do
  curl -fsS "$API/api/endpoints" -o "$ART/endpoints.json" 2>/dev/null || true
  oks=$(jq '[.endpoints[] | select(.Status == "ok")] | length' "$ART/endpoints.json" 2>/dev/null || echo 0)
  settled=$(jq '[.endpoints[] | select(.Status != "unknown")] | length' "$ART/endpoints.json" 2>/dev/null || echo 0)
  [[ "${oks:-0}" -ge 1 ]] && break
  # every endpoint reported a (non-ok) verdict and none is ok → stop waiting
  [[ "${settled:-0}" -ge "${n:-1}" && $i -ge 5 ]] && break
  sleep 3
done
log "probe result: $oks/$n endpoint(s) ok"
jq -r '.endpoints[] | "  \(.Protocol)\t\(.Status)\t\(.LatencyMs)ms"' "$ART/endpoints.json" 2>/dev/null || true
[[ "${oks:-0}" -ge 1 ]] || fail "no endpoint validated (0 ok) — server down, bundle stale, or a client-side gap"

# --- 5. exit-IP: a fetch through :1080 must egress from the SERVER -----------
log "checking the exit IP through SOCKS5 $SOCKS…"
runner_ip=$(curl -fsS --max-time 15 https://api.ipify.org 2>/dev/null || echo "unknown")
# Retry: the balancer may need a couple of failover attempts, and probing keeps
# settling. block_direct=true means a fetch can't leak out the runner IP — it
# either egresses from the server or fails. Accept on the expected match (or,
# with no expected IP, on any proxied result).
got=""; matched=""
for attempt in $(seq 1 15); do
  for ep in "https://api.ipify.org" "https://ifconfig.me/ip" "https://icanhazip.com"; do
    got=$(curl -fsS --max-time 20 --socks5-hostname "$SOCKS" "$ep" 2>/dev/null | tr -d '[:space:]' || true)
    [[ -n "$got" ]] && break
  done
  if [[ -n "$EXIT_IP" ]]; then
    [[ "$got" == "$EXIT_IP" ]] && { matched=1; break; }
  else
    [[ -n "$got" && "$got" != "$runner_ip" ]] && { matched=1; break; }
  fi
  log "  attempt $attempt: exit=$(mask "${got:-<none>}") (want ${EXIT_IP:+$(mask "$EXIT_IP")}${EXIT_IP:-proxied≠$(mask "$runner_ip")}) — retrying…"
  sleep 5
done
log "runner IP=$(mask "$runner_ip")   tunnel exit IP=$(mask "${got:-<none>}")"
[[ -n "$matched" ]] || {
  if [[ -n "$EXIT_IP" ]]; then fail "tunnel exit IP $(mask "${got:-<none>}") != expected server IP $(mask "$EXIT_IP")"
  else fail "no proxied exit IP through the tunnel (got $(mask "${got:-<none>}"), runner $(mask "$runner_ip"))"; fi
}
log "exit IP confirms tunnelled egress ✓"

log "e2e PASSED"
