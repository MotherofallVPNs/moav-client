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

[[ -n "${MOAV_TEST_SUB_URL:-}" || -n "${MOAV_TEST_BUNDLE:-}" ]] \
  || fail "provide MOAV_TEST_SUB_URL or MOAV_TEST_BUNDLE (a server bundle to connect through)"

# --- 1. configure the source -------------------------------------------------
# Start from the example config, then point the subscription at the provided
# bundle/URL. A file bundle is copied into data/ and referenced by path.
[[ -f config.yaml ]] || cp config.yaml.example config.yaml
if [[ -n "${MOAV_TEST_BUNDLE:-}" ]]; then
  mkdir -p data
  cp "$MOAV_TEST_BUNDLE" data/e2e-bundle.zip
  log "using bundle file data/e2e-bundle.zip"
  # subscription.file accepts a bundle zip; the importer expands it on start.
  python3 - <<'PY'
import re, io, sys
p="config.yaml"; s=open(p).read()
s=re.sub(r'(?m)^(subscription:\n(?:.*\n)*?\s*file:).*$', r'\1 "data/e2e-bundle.zip"', s, count=1)
open(p,"w").write(s)
PY
else
  log "using subscription URL from MOAV_TEST_SUB_URL"
  python3 - "$MOAV_TEST_SUB_URL" <<'PY'
import re, sys
url=sys.argv[1]; p="config.yaml"; s=open(p).read()
s=re.sub(r'(?m)^(subscription:\n(?:.*\n)*?\s*url:).*$', r'\1 "%s"' % url, s, count=1)
open(p,"w").write(s)
PY
fi

# --- 2. bring the stack up ---------------------------------------------------
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
  n=$(jq 'length' "$ART/endpoints.json" 2>/dev/null || echo 0)
  [[ "${n:-0}" -ge 1 ]] && break
  sleep 2
done
[[ "${n:-0}" -ge 1 ]] || fail "no endpoints parsed from the provided bundle"
log "parsed $n endpoint(s):"
jq -r '.[] | "  \(.protocol)\t\(.id)"' "$ART/endpoints.json" 2>/dev/null || true

# --- 4. built-in probe: ≥1 endpoint must validate (tunnel + TLS) -------------
log "probing endpoints via /api/probe…"
curl -fsS -X POST "$API/api/probe" -o "$ART/probe.json" 2>/dev/null \
  || curl -fsS "$API/api/probe" -o "$ART/probe.json" 2>/dev/null || true
oks=$(jq '[.. | objects | select(.status? == "ok")] | length' "$ART/probe.json" 2>/dev/null || echo 0)
log "probe reports $oks endpoint(s) ok"
[[ "${oks:-0}" -ge 1 ]] || fail "no endpoint validated (probe found 0 ok) — server down or bundle stale?"

# --- 5. exit-IP: a fetch through :1080 must egress from the SERVER -----------
log "checking the exit IP through SOCKS5 $SOCKS…"
runner_ip=$(curl -fsS --max-time 15 https://api.ipify.org 2>/dev/null || echo "unknown")
got=""
for ep in "https://api.ipify.org" "https://ifconfig.me/ip" "https://icanhazip.com"; do
  got=$(curl -fsS --max-time 25 --socks5-hostname "$SOCKS" "$ep" 2>/dev/null | tr -d '[:space:]' || true)
  [[ -n "$got" ]] && break
done
[[ -n "$got" ]] || fail "could not fetch an exit IP through the tunnel"
log "runner IP=$runner_ip   tunnel exit IP=$got"

if [[ -n "$EXIT_IP" ]]; then
  [[ "$got" == "$EXIT_IP" ]] || fail "tunnel exit IP $got != expected server IP $EXIT_IP"
  log "exit IP matches the server ✓"
else
  [[ "$got" != "$runner_ip" ]] || fail "tunnel exit IP equals the runner IP — traffic is NOT being proxied"
  log "exit IP differs from the runner (proxied) ✓  — set MOAV_TEST_EXIT_IP to assert the exact server IP"
fi

log "e2e PASSED"
