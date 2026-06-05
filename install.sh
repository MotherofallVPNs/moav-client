#!/usr/bin/env bash
# =============================================================================
#  moav-client installer
#
#  One-liner install:
#    curl -fsSL https://raw.githubusercontent.com/MotherofallVPNs/moav-client/main/install.sh | bash
#
#  Interactive mode (default when TTY attached):
#    bash install.sh
#
#  Headless mode (env or flags):
#    MOAV_HEADLESS=1 \
#    MOAV_DIR=/opt/moav-client \
#    MOAV_SUBSCRIPTION=/path/to/subscription.txt \
#    MOAV_WG_CONF=/path/to/wireguard.conf \
#    MOAV_SIDECARS=masterdns,amneziawg \
#    bash install.sh
#
#  Or with flags:
#    bash install.sh --headless \
#      --dir /opt/moav-client \
#      --subscription /path/to/sub.txt \
#      --sidecars masterdns,psiphon
#
#  Skip building (faster re-run if you just want to up):
#    MOAV_SKIP_BUILD=1 bash install.sh
# =============================================================================
set -euo pipefail

REPO_URL="${MOAV_REPO_URL:-https://github.com/MotherofallVPNs/moav-client.git}"
REPO_BRANCH="${MOAV_REPO_BRANCH:-main}"
DEFAULT_DIR="${HOME:-/root}/moav-client"

# ---------- colors ----------------------------------------------------------
if [[ -t 1 ]]; then
  C_RESET=$'\033[0m'
  C_DIM=$'\033[2m'
  C_BOLD=$'\033[1m'
  C_RED=$'\033[31m'
  C_GREEN=$'\033[32m'
  C_YELLOW=$'\033[33m'
  C_BLUE=$'\033[34m'
  C_CYAN=$'\033[36m'
else
  C_RESET= C_DIM= C_BOLD= C_RED= C_GREEN= C_YELLOW= C_BLUE= C_CYAN=
fi

say() { printf '%s%s%s\n' "$1" "$2" "$C_RESET"; }
ok()   { say "$C_GREEN"  "  ✓ $1"; }
warn() { say "$C_YELLOW" "  ! $1"; }
err()  { say "$C_RED"    "  ✗ $1"; }
note() { say "$C_DIM"    "    $1"; }
step() { printf '\n%s%s%s\n' "$C_CYAN$C_BOLD" "$1" "$C_RESET"; }
hdr()  {
  echo ""
  say "$C_BOLD" "═════════════════════════════════════════════════════════"
  printf '%s  %s%s\n' "$C_BOLD$C_CYAN" "$1" "$C_RESET"
  say "$C_BOLD" "═════════════════════════════════════════════════════════"
}

# ---------- arg / env parsing ----------------------------------------------
HEADLESS="${MOAV_HEADLESS:-}"
INSTALL_DIR="${MOAV_DIR:-}"
SUBSCRIPTION="${MOAV_SUBSCRIPTION:-}"
WG_CONF="${MOAV_WG_CONF:-}"
SIDECAR_CSV="${MOAV_SIDECARS:-}"
SKIP_BUILD="${MOAV_SKIP_BUILD:-}"

while (( $# > 0 )); do
  case "$1" in
    --headless)        HEADLESS=1 ;;
    --dir)             INSTALL_DIR="$2"; shift ;;
    --subscription)    SUBSCRIPTION="$2"; shift ;;
    --wg-conf)         WG_CONF="$2"; shift ;;
    --sidecars)        SIDECAR_CSV="$2"; shift ;;
    --skip-build)      SKIP_BUILD=1 ;;
    --branch)          REPO_BRANCH="$2"; shift ;;
    --repo)            REPO_URL="$2"; shift ;;
    --help|-h)
      sed -n '2,/^# ===/p' "$0" | sed 's/^#//' | head -n 30
      exit 0
      ;;
    *)  err "unknown flag: $1"; exit 1 ;;
  esac
  shift
done

INSTALL_DIR="${INSTALL_DIR:-$DEFAULT_DIR}"
# Detect interactive vs piped — auto-go-headless when stdin isn't a TTY
# AND the caller didn't explicitly set HEADLESS.
if [[ -z "$HEADLESS" && ! -t 0 ]]; then
  HEADLESS=auto
fi

# ---------- sidecar catalog -------------------------------------------------
# Each entry: kind | image MB | label | one-liner
SIDECAR_KINDS=(masterdns amneziawg psiphon trusttunnel tor)
sidecar_meta() {
  case "$1" in
    masterdns)
      echo "160|MasterDNS|DNS-tunnel client for MoaV-issued DNS tunnels (m.<bundle>.<tld>)."
      ;;
    amneziawg)
      echo "180|AmneziaWG|Userspace amneziawg-go + microsocks on awg0 default route. Needs NET_ADMIN + /dev/net/tun."
      ;;
    psiphon)
      echo "195|Psiphon|Psiphon-Labs ConsoleClient built from source. Needs a Psiphon-issued config to actually tunnel."
      ;;
    trusttunnel)
      echo "85|TrustTunnel|HTTP/2 + HTTP/3 tunnel. Placeholder — mount the upstream client binary to activate."
      ;;
    tor)
      echo "15|Tor|peterdavehello/tor-socks-proxy — Tor SOCKS5 on :9150. No credentials required."
      ;;
  esac
}

# ---------- banner ----------------------------------------------------------
clear || true
cat <<BANNER
${C_CYAN}${C_BOLD}
  ███╗   ███╗ ██████╗  █████╗ ██╗   ██╗
  ████╗ ████║██╔═══██╗██╔══██╗██║   ██║   ${C_RESET}${C_DIM}client${C_RESET}${C_CYAN}${C_BOLD}
  ██╔████╔██║██║   ██║███████║██║   ██║
  ██║╚██╔╝██║██║   ██║██╔══██║╚██╗ ██╔╝
  ██║ ╚═╝ ██║╚██████╔╝██║  ██║ ╚████╔╝
  ╚═╝     ╚═╝ ╚═════╝ ╚═╝  ╚═╝  ╚═══╝${C_RESET}

  ${C_DIM}Mother of all VPNs — local client${C_RESET}

BANNER

# ---------- step 1: prereqs -------------------------------------------------
hdr "[1/5] checking prerequisites"

need_cmd() {
  if command -v "$1" >/dev/null 2>&1; then
    ok "$1 ($(command -v "$1"))"
  else
    err "$1 not found"
    return 1
  fi
}

need_cmd git || { warn "install git first: https://git-scm.com/downloads"; exit 1; }
need_cmd curl || { warn "install curl first"; exit 1; }
# python3 drives the config.yaml sidecar-toggle step; without it the install
# aborts mid-config under set -e.
need_cmd python3 || { warn "install python3 first"; exit 1; }
need_cmd docker || {
  err "docker not found"
  warn "install Docker Desktop or the docker engine: https://docs.docker.com/get-docker/"
  exit 1
}

if docker compose version >/dev/null 2>&1; then
  ok "docker compose ($(docker compose version --short))"
else
  err "docker compose v2 plugin not found"
  warn "install: https://docs.docker.com/compose/install/"
  exit 1
fi

if ! docker info >/dev/null 2>&1; then
  err "docker daemon isn't reachable — start Docker and re-run"
  exit 1
fi
ok "docker daemon reachable"

# Probe available disk so we can warn if it's tight.
DF_AVAIL_MB=$(df -m "$(dirname "$INSTALL_DIR")" 2>/dev/null | awk 'NR==2 {print $4}' || echo "?")
note "free disk at install path: ${DF_AVAIL_MB} MB"

# ---------- step 2: clone / update repo ------------------------------------
hdr "[2/5] fetching moav-client"

if [[ -d "$INSTALL_DIR/.git" ]]; then
  ok "existing repo at $INSTALL_DIR — pulling latest"
  git -C "$INSTALL_DIR" fetch --quiet origin "$REPO_BRANCH" || true
  git -C "$INSTALL_DIR" checkout --quiet "$REPO_BRANCH" 2>/dev/null || true
  git -C "$INSTALL_DIR" pull --quiet --ff-only origin "$REPO_BRANCH" || warn "couldn't fast-forward; leaving working tree as-is"
elif [[ -e "$INSTALL_DIR" ]]; then
  err "$INSTALL_DIR exists but isn't a git repo — refusing to clobber"
  exit 1
else
  ok "cloning $REPO_URL → $INSTALL_DIR"
  git clone --quiet --depth=1 --branch "$REPO_BRANCH" "$REPO_URL" "$INSTALL_DIR"
fi

cd "$INSTALL_DIR"
ok "at $(pwd) ($(git rev-parse --short HEAD))"

# ---------- step 3: choose protocols ---------------------------------------
hdr "[3/5] choose protocols & sidecars"

CORE_MB=$((16 + 76 + 113))   # proxy-core + web-ui + sing-box
echo "  ${C_BOLD}Core stack${C_RESET} (always installed) ─ ${C_GREEN}~${CORE_MB} MB${C_RESET}"
printf '    %-20s  %s\n' "proxy-core"   "~16 MB    Go binary — SOCKS5/HTTP CONNECT + balancer + API"
printf '    %-20s  %s\n' "web-ui"       "~76 MB    React dashboard (nginx-alpine + dist)"
printf '    %-20s  %s\n' "sing-box"     "~113 MB   Real protocol cryptography (VLESS/Reality/Trojan/SS/Hy2/WG)"
echo ""

echo "  ${C_BOLD}Optional sidecars${C_RESET}:"
for kind in "${SIDECAR_KINDS[@]}"; do
  IFS='|' read -r mb label oneliner <<<"$(sidecar_meta "$kind")"
  printf '    %-20s  %s%-9s%s %s\n' "[$kind]" "$C_YELLOW" "~${mb} MB" "$C_RESET" "$label"
  note "$oneliner"
done
echo ""

choose_sidecars() {
  local picked=()
  for kind in "${SIDECAR_KINDS[@]}"; do
    IFS='|' read -r mb label _ <<<"$(sidecar_meta "$kind")"
    read -r -p "    enable $C_BOLD$label$C_RESET? ${C_DIM}(~${mb} MB)${C_RESET} [y/N] " ans </dev/tty || ans=""
    case "${ans,,}" in
      y|yes) picked+=("$kind") ;;
    esac
  done
  echo "${picked[*]}"
}

SIDECARS=()
if [[ -n "$SIDECAR_CSV" ]]; then
  IFS=',' read -r -a SIDECARS <<<"$SIDECAR_CSV"
  ok "sidecars from --sidecars / MOAV_SIDECARS: ${SIDECARS[*]:-none}"
elif [[ "$HEADLESS" == "1" ]]; then
  ok "headless: no extra sidecars (set MOAV_SIDECARS to enable)"
elif [[ "$HEADLESS" == "auto" ]]; then
  warn "non-interactive stdin and no MOAV_SIDECARS — defaulting to core stack only"
else
  read -r -p "  pick interactively (Y) or list comma-separated keys (e.g. masterdns,psiphon)? [Y/n] " mode </dev/tty || mode=""
  case "${mode,,}" in
    n|no|"")
      list=$(choose_sidecars)
      IFS=' ' read -r -a SIDECARS <<<"$list"
      ;;
    *)
      if [[ "${mode,,}" == "y" || "${mode,,}" == "yes" ]]; then
        list=$(choose_sidecars)
        IFS=' ' read -r -a SIDECARS <<<"$list"
      else
        IFS=',' read -r -a SIDECARS <<<"$mode"
      fi
      ;;
  esac
fi

# Validate sidecar keys.
for k in "${SIDECARS[@]:-}"; do
  [[ -z "$k" ]] && continue
  good=
  for valid in "${SIDECAR_KINDS[@]}"; do
    [[ "$k" == "$valid" ]] && good=1 && break
  done
  if [[ -z "$good" ]]; then
    err "unknown sidecar key: $k (valid: ${SIDECAR_KINDS[*]})"
    exit 1
  fi
done

# Tally total estimated disk.
TOTAL_MB=$CORE_MB
for k in "${SIDECARS[@]:-}"; do
  [[ -z "$k" ]] && continue
  IFS='|' read -r mb _ _ <<<"$(sidecar_meta "$k")"
  TOTAL_MB=$((TOTAL_MB + mb))
done
echo ""
ok "estimated docker image footprint: ${TOTAL_MB} MB"
ok "selected sidecars: ${SIDECARS[*]:-none}"

if [[ "$DF_AVAIL_MB" != "?" && "$DF_AVAIL_MB" -lt $((TOTAL_MB + 500)) ]]; then
  warn "free disk ($DF_AVAIL_MB MB) is tight for ~${TOTAL_MB} MB images + build cache."
fi

# ---------- step 4: subscription + config ----------------------------------
hdr "[4/5] subscription & config"

if [[ -z "$SUBSCRIPTION" ]]; then
  # Try to detect an existing data/<bundle>/subscription.txt.
  detected=$(find "$INSTALL_DIR/data" -maxdepth 2 -name 'subscription.txt' 2>/dev/null | head -1 || true)
  if [[ -n "$detected" ]]; then
    SUBSCRIPTION="$detected"
    ok "auto-detected subscription: $SUBSCRIPTION"
  elif [[ "$HEADLESS" == "1" || "$HEADLESS" == "auto" ]]; then
    warn "no subscription file specified — config.yaml will be left with the example bundle path"
  else
    read -r -p "  path to your MoaV subscription.txt (blank to skip): " SUBSCRIPTION </dev/tty || SUBSCRIPTION=""
  fi
fi

CONFIG="$INSTALL_DIR/config.yaml"
if [[ -f "$CONFIG" ]]; then
  note "preserving existing $CONFIG"
elif [[ -f "$INSTALL_DIR/config.yaml.example" ]]; then
  cp "$INSTALL_DIR/config.yaml.example" "$CONFIG"
  ok "seeded $CONFIG from config.yaml.example"
else
  err "no config.yaml.example to seed from"
  exit 1
fi

# .env must exist before docker-compose can mount it (proxy-core writes to it
# from the dashboard's Network exposure setting). Seed from .env.example or
# create an empty file.
ENVF="$INSTALL_DIR/.env"
if [[ ! -f "$ENVF" ]]; then
  if [[ -f "$INSTALL_DIR/.env.example" ]]; then
    cp "$INSTALL_DIR/.env.example" "$ENVF"
    ok "seeded .env from .env.example (loopback exposure)"
  else
    touch "$ENVF"
    ok "created empty .env"
  fi
fi

if [[ -n "$SUBSCRIPTION" && -f "$SUBSCRIPTION" ]]; then
  # Best-effort in-place edit. Escape slashes for sed.
  esc=$(printf '%s' "$SUBSCRIPTION" | sed 's#/#\\/#g')
  if grep -qE '^\s*file:\s*"' "$CONFIG"; then
    sed -i.bak -E "s|^([[:space:]]*file:[[:space:]]*)\"[^\"]*\"|\1\"${SUBSCRIPTION}\"|" "$CONFIG"
    rm -f "${CONFIG}.bak"
    ok "set subscription.file → $SUBSCRIPTION"
  fi
fi

if [[ -n "$WG_CONF" && -f "$WG_CONF" ]]; then
  ok "wireguard conf: $WG_CONF (edit config.yaml -> subscription.wireguard_files to point at it)"
fi

# Toggle sidecar enable flags in config.yaml.
toggle_sidecar() {
  local kind="$1" enable="$2"
  # Match the YAML block "<kind>:" followed by "enabled: <bool>".
  python3 - "$CONFIG" "$kind" "$enable" <<'PY'
import re, sys, pathlib
path, kind, val = sys.argv[1], sys.argv[2], sys.argv[3]
p = pathlib.Path(path)
src = p.read_text()
# Find the sidecars:<kind>: block and rewrite its `enabled:` line.
pattern = re.compile(
    r'(^\s*' + re.escape(kind) + r':\s*\n(?:\s*#.*\n)*?\s*)enabled:\s*\w+',
    re.MULTILINE,
)
new, n = pattern.subn(r'\1enabled: ' + val, src)
if n:
    p.write_text(new)
PY
}

for kind in "${SIDECAR_KINDS[@]}"; do
  if printf '%s\n' "${SIDECARS[@]:-}" | grep -qx "$kind"; then
    toggle_sidecar "$kind" "true" && ok "config.yaml: $kind enabled"
  else
    toggle_sidecar "$kind" "false" && note "config.yaml: $kind disabled"
  fi
done

# ---------- step 5: build & up ---------------------------------------------
hdr "[5/5] build & start"

profiles=()
for k in "${SIDECARS[@]:-}"; do
  [[ -z "$k" ]] && continue
  profiles+=(--profile "$k")
done

if [[ -n "$SKIP_BUILD" ]]; then
  warn "MOAV_SKIP_BUILD set — skipping image build"
else
  ok "building core images (proxy-core + web-ui) — this can take 1–3 min on first run"
  docker compose build proxy-core web-ui
  if (( ${#profiles[@]} > 0 )); then
    ok "building sidecars: ${SIDECARS[*]}"
    docker compose "${profiles[@]}" build "${SIDECARS[@]}"
  fi
fi

ok "starting stack…"
docker compose "${profiles[@]}" up -d
sleep 4

# Quick smoke status.
status="$(docker compose "${profiles[@]}" ps --format 'table {{.Name}}\t{{.Status}}' 2>/dev/null || true)"
echo ""
say "$C_DIM" "  $status"

# ---------- install the `moav-client` command globally ----------------------
# Symlink the management wrapper into PATH so it's usable from anywhere (the
# wrapper resolves the symlink back to this install dir). Best-effort: skip
# quietly if we can't write and have no sudo.
GLOBAL_BIN="/usr/local/bin/moav-client"
WRAPPER="$INSTALL_DIR/moav-client"
HAVE_GLOBAL=
if [[ -x "$WRAPPER" ]]; then
  if [[ -w "$(dirname "$GLOBAL_BIN")" ]]; then
    ln -sf "$WRAPPER" "$GLOBAL_BIN" && HAVE_GLOBAL=1
  elif command -v sudo >/dev/null 2>&1; then
    sudo ln -sf "$WRAPPER" "$GLOBAL_BIN" 2>/dev/null && HAVE_GLOBAL=1
  fi
  if [[ -n "$HAVE_GLOBAL" ]]; then
    ok "installed 'moav-client' command → $GLOBAL_BIN"
  else
    warn "couldn't symlink to $GLOBAL_BIN (no write access / no sudo) — use ./moav-client from $INSTALL_DIR"
  fi
fi
# CLI prefix used in the closing tips: bare command if global, else ./relative.
MC="moav-client"
[[ -z "$HAVE_GLOBAL" ]] && MC="./moav-client"

# ---------- done -----------------------------------------------------------
hdr "✓ moav-client is up"

cat <<DONE
  ${C_BOLD}Dashboard:${C_RESET}    http://localhost:3001
  ${C_BOLD}SOCKS5 proxy:${C_RESET} localhost:1080  (point your browser here — socks5h://localhost:1080)
  ${C_BOLD}HTTP proxy:${C_RESET}   localhost:8081
  ${C_BOLD}REST API:${C_RESET}     localhost:8088

  Next steps:
    • Open the dashboard ${C_DIM}(http://localhost:3001)${C_RESET} and verify endpoints in the ${C_BOLD}Endpoints${C_RESET} tab.
    • Quick test: ${C_DIM}curl --socks5-hostname localhost:1080 https://api.ipify.org${C_RESET}
    • Manage from CLI: ${C_DIM}${MC} status | up | down | logs ...${C_RESET}
    • Run on LAN / public: dashboard → ${C_BOLD}Settings → Network exposure${C_RESET}.
    • Import another moav server's bundle: dashboard → ${C_BOLD}Sources → drop .zip${C_RESET}.
DONE

# ---------- auto-open dashboard ---------------------------------------------
# Best-effort cross-platform open. Skipped in headless mode (no GUI) or
# when MOAV_NO_OPEN=1.
if [[ "$HEADLESS" != "1" && "$HEADLESS" != "auto" && -z "${MOAV_NO_OPEN:-}" ]]; then
  url="http://localhost:3001"
  if command -v open >/dev/null 2>&1; then
    open "$url" >/dev/null 2>&1 || true
  elif command -v xdg-open >/dev/null 2>&1; then
    xdg-open "$url" >/dev/null 2>&1 &
  elif command -v powershell.exe >/dev/null 2>&1; then
    powershell.exe -NoProfile -Command "Start-Process '$url'" >/dev/null 2>&1 || true
  fi
fi

if printf '%s\n' "${SIDECARS[@]:-}" | grep -qx "psiphon"; then
  echo ""
  warn "Psiphon won't actually tunnel until you provide Psiphon-issued credentials."
  note "Paste a verbatim Psiphon config under sidecars.psiphon.config.config_json in config.yaml,"
  note "or fill in the individual keys (propagation_channel_id, sponsor_id, server-list URL, signing pubkey)."
fi

if printf '%s\n' "${SIDECARS[@]:-}" | grep -qx "trusttunnel"; then
  echo ""
  warn "TrustTunnel sidecar is a placeholder — there's no public Linux client binary yet."
  note "Mount the upstream binary at /usr/local/bin/trusttunnel-client + your client.toml to activate."
fi

echo ""
