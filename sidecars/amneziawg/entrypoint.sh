#!/bin/bash
set -e

CONF=/etc/amneziawg/awg0.conf
echo "[amneziawg] waiting for $CONF ..."
while [ ! -s "$CONF" ]; do sleep 1; done
echo "[amneziawg] config detected"

# Parse Address from the [Interface] section. Default if missing.
ADDR=$(awk -F'=' '/^[[:space:]]*Address/{print $2}' "$CONF" | tr -d ' ' | head -1)
ADDR=${ADDR:-10.67.67.6/32}

# 1. Start amneziawg-go in userspace mode (creates the awg0 tun device).
echo "[amneziawg] starting amneziawg-go on awg0"
amneziawg-go awg0 &
AWG_PID=$!
sleep 1

# 2. Load the conf via the patched `awg setconf`. setconf only understands
#    the wg-protocol-level keys, so strip the wg-quick-only directives
#    (Address, DNS, MTU) — they're applied with ip(8) below.
STRIPPED=$(mktemp)
awk '
    /^[[:space:]]*\[Interface\]/ {section="i"; print; next}
    /^[[:space:]]*\[Peer\]/      {section="p"; print; next}
    /^[[:space:]]*$/             {print; next}
    /^[[:space:]]*#/             {print; next}
    section=="i" && /^[[:space:]]*(Address|DNS|MTU)[[:space:]]*=/ {next}
    {print}
' "$CONF" > "$STRIPPED"

if ! awg setconf awg0 "$STRIPPED"; then
    echo "[amneziawg] FATAL: awg setconf failed. Container will idle."
    cat "$STRIPPED"
    exec sleep infinity
fi
rm -f "$STRIPPED"

# 3. Capture the eth0 default route + add a /32 host route for the wg peer
#    BEFORE swinging the default to awg0 — otherwise the wg handshake itself
#    can't reach the server. Also enable SNAT so SOCKS5 client packets get
#    a source address that wg can route.
ETH_GW=$(ip -o -4 route show default | awk '{print $3}' | head -1)
PEER_HOST=$(awk -F'=' '/^[[:space:]]*Endpoint/{print $2}' "$CONF" | tr -d ' ' | head -1 | cut -d: -f1)
if [ -n "$PEER_HOST" ] && [ -n "$ETH_GW" ]; then
    ip route add "$PEER_HOST/32" via "$ETH_GW" dev eth0 || true
fi

ip link set awg0 up
ip addr add "$ADDR" dev awg0 || true
ip route replace default dev awg0 || true

echo "[amneziawg] awg0 is up at $ADDR; SOCKS5 listening on :5500"

# 4. Run a minimal SOCKS5 server. Because awg0 is the kernel default route,
#    every outbound TCP from this container egresses via the AmneziaWG tunnel.
exec microsocks -i 0.0.0.0 -p 5500
