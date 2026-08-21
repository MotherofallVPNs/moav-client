#!/bin/bash
# Drift gate: every place a version is pinned must agree with its single source.
#   - MOAV_VERSION: the VERSION file (cli.go / Dockerfile / compose are fallbacks)
#   - component versions: .env.example (compose + Dockerfile ARGs are fallbacks)
# Keeps "single source of truth" honest — see docs/CONFIGURATION.md.
set -uo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
pass=0; fail=0
ok()  { printf '  ok    %s\n' "$1"; pass=$((pass+1)); }
bad() { printf '  FAIL  %s\n' "$1"; fail=$((fail+1)); }

# extract helpers -------------------------------------------------------------
env_val()      { grep -E "^$1=" .env.example | tail -1 | cut -d= -f2-; }                       # .env.example VAR=
compose_def()  { grep -oE "$1:-[^}\"]+" docker-compose.yml | head -1 | sed 's/.*:-//'; }        # ${VAR:-default}
dockerfile_arg(){ grep -oE "$1=[^ ]+" "$2" | head -1 | cut -d= -f2; }                           # ARG VAR=default

# assert all args equal ($1 = label, rest = values) ---------------------------
same() {
  local label="$1"; shift
  local first="$1"; shift
  for v in "$@"; do
    if [ "$v" != "$first" ]; then
      bad "$label out of sync: '$first' vs '$v'"; return
    fi
  done
  ok "$label agrees ($first)"
}

echo "version drift gate"

# --- MOAV_VERSION: VERSION file is the source --------------------------------
VER=$(cat VERSION)
CLI=$(grep -oE 'Version = "[0-9][0-9.]*"' proxy-core/cmd/cli.go | grep -oE '[0-9][0-9.]*')
DKR=$(dockerfile_arg MOAV_VERSION proxy-core/Dockerfile)
CMP=$(compose_def MOAV_VERSION)
same "MOAV_VERSION (VERSION/cli.go/Dockerfile/compose)" "$VER" "$CLI" "$DKR" "$CMP"

# --- component versions: .env.example is the source --------------------------
same "SINGBOX_VERSION (.env/compose)"      "$(env_val SINGBOX_VERSION)"      "$(compose_def SINGBOX_VERSION)"
same "XRAY_VERSION (.env/compose/Dockerfile)" "$(env_val XRAY_VERSION)"     "$(compose_def XRAY_VERSION)"     "$(dockerfile_arg XRAY_VERSION sidecars/xray/Dockerfile)"
same "AMNEZIAWG_GO_VERSION (.env/compose/Dockerfile)" "$(env_val AMNEZIAWG_GO_VERSION)" "$(compose_def AMNEZIAWG_GO_VERSION)" "$(dockerfile_arg AMNEZIAWG_GO_VERSION sidecars/amneziawg/Dockerfile)"
same "AWGTOOLS_VERSION (.env/compose/Dockerfile)" "$(env_val AWGTOOLS_VERSION)" "$(compose_def AWGTOOLS_VERSION)" "$(dockerfile_arg AWGTOOLS_VERSION sidecars/amneziawg/Dockerfile)"
same "MASTERDNS_VERSION (.env/compose/Dockerfile)" "$(env_val MASTERDNS_VERSION)" "$(compose_def MASTERDNS_VERSION)" "$(dockerfile_arg MASTERDNS_VERSION sidecars/dns-tunnels/Dockerfile)"
same "TRUSTTUNNEL_CLIENT_VERSION (.env/compose/Dockerfile)" "$(env_val TRUSTTUNNEL_CLIENT_VERSION)" "$(compose_def TRUSTTUNNEL_CLIENT_VERSION)" "$(dockerfile_arg TRUSTTUNNEL_CLIENT_VERSION sidecars/trusttunnel/Dockerfile)"

# --- ports: .env.example defaults match config.yaml.example base -------------
yaml_port() { grep -oE "$1: [0-9]+" config.yaml.example | head -1 | sed 's/.*: //'; }
same "SOCKS5_PORT (.env/config.yaml.example)" "$(env_val SOCKS5_PORT)" "$(yaml_port socks5_port)"
same "API_PORT (.env/config.yaml.example)"    "$(env_val API_PORT)"    "$(yaml_port api_port)"

echo ""
if [ "$fail" -gt 0 ]; then echo "FAILED ($fail failed, $pass passed)"; exit 1; fi
echo "PASSED ($pass checks)"
