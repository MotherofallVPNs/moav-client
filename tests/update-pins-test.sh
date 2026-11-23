#!/bin/bash
# `moav-client update` / `sync-pins` must carry a release's component version
# bumps into an existing .env (seeded once at install), and touch nothing else.
set -uo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
pass=0; fail=0
ok()  { printf '  ok    %s\n' "$1"; pass=$((pass+1)); }
bad() { printf '  FAIL  %s\n' "$1"; fail=$((fail+1)); }
has() { grep -qxF "$2" "$1"; }   # file contains this exact line

TMP=$(mktemp -d); trap 'rm -rf "$TMP"' EXIT
moavc() { MOAV_DIR="$TMP" bash "$ROOT/moav-client" "$@" </dev/null 2>&1; }

seed() {
  cat > "$TMP/.env.example" <<'X'
SOCKS5_BIND=127.0.0.1
# MOAV_VERSION=9.9.9
SINGBOX_VERSION=2.0.0
XRAY_VERSION=v99.1.1
MASTERDNS_VERSION=v2099.01.01.000000-abcdef0
NEW_THING_VERSION=1.0.0
# IMAGE_SINGBOX=ghcr.io/sagernet/sing-box:v2.0.0
X
  cat > "$TMP/.env" <<'X'
SOCKS5_BIND=0.0.0.0
MOAV_DASHBOARD_PASS=s3cret=with=equals
# SINGBOX_VERSION=0.0.1
SINGBOX_VERSION=1.0.0
XRAY_VERSION="v98.0.0"
MASTERDNS_VERSION=v2099.01.01.000000-abcdef0
MOAV_VERSION=custom-build
X
}

echo "update pin re-sync"

seed; out=$(moavc sync-pins --check)
has "$TMP/.env" "SINGBOX_VERSION=1.0.0" && has "$TMP/.env" 'XRAY_VERSION="v98.0.0"' \
  && ok "--check changes nothing" || bad "--check modified .env"
grep -q "SINGBOX_VERSION" <<<"$out" && grep -q "XRAY_VERSION" <<<"$out" \
  && ok "--check lists the stale pins" || bad "--check did not list stale pins: $out"

seed; moavc sync-pins --yes >/dev/null
has "$TMP/.env" "SINGBOX_VERSION=2.0.0" && ok "stale pin updated"        || bad "SINGBOX_VERSION not updated"
has "$TMP/.env" "XRAY_VERSION=v99.1.1"  && ok "quoted stale pin updated" || bad "XRAY_VERSION not updated"
has "$TMP/.env" "SOCKS5_BIND=0.0.0.0" && has "$TMP/.env" "MOAV_DASHBOARD_PASS=s3cret=with=equals" \
  && ok "non-version settings untouched" || bad "non-version settings changed"
has "$TMP/.env" "# SINGBOX_VERSION=0.0.1" && has "$TMP/.env" "MOAV_VERSION=custom-build" \
  && ok "commented pin and MOAV_VERSION override untouched" || bad "comment/MOAV_VERSION changed"
grep -q '^NEW_THING_VERSION=' "$TMP/.env" \
  && bad "pin absent from .env was added (compose default should apply)" || ok "pin absent from .env stays absent"
[ "$(wc -l < "$TMP/.env")" -eq 7 ] && ok "no lines added or removed" || bad "line count changed"

before=$(cat "$TMP/.env"); out=$(moavc sync-pins --yes)
[ "$before" = "$(cat "$TMP/.env")" ] && grep -q "match this release" <<<"$out" \
  && ok "second run is a no-op" || bad "second run changed .env: $out"

# No TTY: the prompt defaults to yes, so an unattended `update` still syncs.
# (Skipped on a terminal, where the prompt reads /dev/tty and would wait.)
if ! { : </dev/tty; } 2>/dev/null; then
  seed; moavc sync-pins >/dev/null
  has "$TMP/.env" "SINGBOX_VERSION=2.0.0" && ok "non-interactive run applies (default yes)" || bad "non-interactive run did not apply"
fi

echo ""
if [ "$fail" -gt 0 ]; then echo "FAILED ($fail failed, $pass passed)"; exit 1; fi
echo "PASSED ($pass checks)"
