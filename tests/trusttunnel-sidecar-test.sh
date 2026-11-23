#!/bin/bash
# TrustTunnel sidecar smoke test: build the image at the pinned client version
# and check the real client accepts the client.toml proxy-core generates (the
# golden file in proxy-core/sidecars/testdata/trusttunnel) and serves SOCKS5.
# No server is involved (the fixture endpoint is unreachable), so this covers
# build + config + listener only, not the tunnel.
set -uo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT" || exit 1
pass=0; fail=0
ok()  { printf '  ok    %s\n' "$1"; pass=$((pass+1)); }
bad() { printf '  FAIL  %s\n' "$1"; fail=$((fail+1)); }

echo "trusttunnel sidecar smoke"

if ! docker info >/dev/null 2>&1; then
  # A silent skip would hide a dead test in CI, so only skip on a dev machine.
  if [ -n "${CI:-}" ]; then echo "FAILED (docker is not available)"; exit 1; fi
  echo "SKIPPED (docker is not available)"; exit 0
fi

PIN=$(grep -E '^TRUSTTUNNEL_CLIENT_VERSION=' .env.example | tail -1 | cut -d= -f2-)
GOLDEN=proxy-core/sidecars/testdata/trusttunnel/client.golden.toml
IMG="moav-client-trusttunnel-smoke:${PIN}"
TMP=$(mktemp -d)
NAMES=()
cleanup() {
  for n in "${NAMES[@]:-}"; do [ -n "$n" ] && docker rm -f "$n" >/dev/null 2>&1; done
  rm -rf "$TMP"
}
trap cleanup EXIT

# start <label> <config-dir>: run the sidecar with config-dir mounted; echoes the name.
start() {
  local n="tt-smoke-$1-$$"
  docker run -d --name "$n" -v "$2:/etc/trusttunnel:ro" "$IMG" >/dev/null || return 1
  echo "$n"
}
# socks_greeting <container> <port>: SOCKS5 "no auth" hello, prints the reply as hex.
socks_greeting() {
  printf '\005\001\000' | docker exec -i "$1" socat -t2 - "TCP:127.0.0.1:$2" 2>/dev/null \
    | od -An -tx1 | tr -d ' \n'
}

# --- build -------------------------------------------------------------------
if docker build -q --build-arg "TRUSTTUNNEL_CLIENT_VERSION=${PIN}" -t "$IMG" sidecars/trusttunnel >/dev/null; then
  ok "image builds at ${PIN}"
else
  bad "image build failed at ${PIN}"
  echo ""; echo "FAILED ($fail failed, $pass passed)"; exit 1
fi

ver=$(docker run --rm --entrypoint /usr/local/bin/trusttunnel_client "$IMG" --version 2>&1 | head -1)
[ "$ver" = "trusttunnel_client ${PIN#v}" ] && ok "client reports the pinned version ($ver)" \
  || bad "client version: got '$ver', want 'trusttunnel_client ${PIN#v}'"

# --- generated config is accepted, SOCKS5 is served ---------------------------
mkdir -p "$TMP/good" "$TMP/bad" "$TMP/empty"
cp "$GOLDEN" "$TMP/good/client.toml"
# An unknown tls_profile is a hard config error from client 1.3.0 on.
awk '{print} /^\[endpoint\]/{print "tls_profile = \"no-such-profile\""}' "$GOLDEN" > "$TMP/bad/client.toml"
chmod -R a+rX "$TMP"

good=$(start good "$TMP/good"); NAMES+=("$good")
reply=""
for _ in $(seq 1 15); do
  reply=$(socks_greeting "$good" 5600)
  [ "$reply" = "0500" ] && break
  sleep 1
done
[ "$reply" = "0500" ] && ok "SOCKS5 answers on :5600 (socat -> client)" \
  || bad "no SOCKS5 reply on :5600 (got '${reply}')"
[ "$(socks_greeting "$good" 5601)" = "0500" ] && ok "client listens on 127.0.0.1:5601" \
  || bad "client is not listening on 127.0.0.1:5601"
logs=$(docker logs "$good" 2>&1)
grep -q 'Failed to parse config' <<<"$logs" \
  && bad "client rejected the generated config: $(grep -m3 ERROR <<<"$logs")" \
  || ok "generated client.toml parses"

# --- a bad config is rejected -------------------------------------------------
badc=$(start bad "$TMP/bad"); NAMES+=("$badc")
code=running
for _ in $(seq 1 20); do
  if [ "$(docker inspect -f '{{.State.Running}}' "$badc" 2>/dev/null)" = "false" ]; then
    code=$(docker inspect -f '{{.State.ExitCode}}' "$badc"); break
  fi
  sleep 1
done
if [ "$code" != "0" ] && [ "$code" != "running" ] && docker logs "$badc" 2>&1 | grep -q 'Failed to parse config'; then
  ok "invalid config is rejected (exit $code)"
else
  bad "invalid config was not rejected (exit '$code')"
fi

# --- no config: idle, do not crash-loop ---------------------------------------
idle=$(start idle "$TMP/empty"); NAMES+=("$idle")
sleep 2
if [ "$(docker inspect -f '{{.State.Running}}' "$idle" 2>/dev/null)" = "true" ] \
   && docker logs "$idle" 2>&1 | grep -q 'no client.toml'; then
  ok "no client.toml: container idles"
else
  bad "no client.toml: container did not idle"
fi

echo ""
if [ "$fail" -gt 0 ]; then
  echo "--- client log ---"; docker logs "$good" 2>&1 | tail -30
  echo "FAILED ($fail failed, $pass passed)"; exit 1
fi
echo "PASSED ($pass checks)"
