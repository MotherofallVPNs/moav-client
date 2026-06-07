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
#
#  Missing prerequisites (docker, git, curl, python3) are installed
#  automatically when possible — on Linux via the OS package manager and
#  https://get.docker.com, on macOS via Homebrew. In headless / non-TTY runs
#  this happens without prompting; interactively you're asked first (default
#  yes). Force unattended installs anywhere with --yes / MOAV_ASSUME_YES=1.
#  Disable auto-install of Docker with MOAV_NO_DOCKER_INSTALL=1.
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

# ---------- OS detection & package helpers ---------------------------------
# OS:  macos | debian | rhel | alpine | arch | linux | windows | unknown
# PKG: apt | dnf | yum | apk | pacman | brew | "" (no known package manager)
OS="" PKG=""
detect_os() {
  case "$(uname -s 2>/dev/null)" in
    Darwin) OS=macos ;;
    Linux)
      if   [[ -f /etc/debian_version ]]; then OS=debian
      elif [[ -f /etc/redhat-release || -f /etc/fedora-release ]]; then OS=rhel
      elif [[ -f /etc/alpine-release ]]; then OS=alpine
      elif [[ -f /etc/arch-release ]]; then OS=arch
      else OS=linux; fi ;;
    MINGW*|MSYS*|CYGWIN*) OS=windows ;;
    *) OS=unknown ;;
  esac
  case "$OS" in
    macos)  command -v brew >/dev/null 2>&1 && PKG=brew ;;
    debian) PKG=apt ;;
    rhel)   command -v dnf >/dev/null 2>&1 && PKG=dnf || PKG=yum ;;
    alpine) PKG=apk ;;
    arch)   PKG=pacman ;;
  esac
}

# Privilege escalation prefix for system package installs. Empty when we're
# already root or when sudo is unavailable (brew must NOT run under sudo).
SUDO=""
if [[ "$(id -u 2>/dev/null || echo 0)" -ne 0 ]] && command -v sudo >/dev/null 2>&1; then
  SUDO="sudo"
fi

# Auto-confirm an install prompt. Yes without asking in headless / assume-yes
# mode (the whole point of an automated installer); otherwise prompt (default
# yes) on the TTY.
want_install() {
  if [[ -n "$ASSUME_YES" || "$HEADLESS" == "1" || "$HEADLESS" == "auto" ]]; then
    return 0
  fi
  local ans
  read -r -p "    $1 [Y/n] " ans </dev/tty 2>/dev/null || ans=""
  case "${ans,,}" in n|no) return 1 ;; *) return 0 ;; esac
}

_apt_refreshed=""
pkg_install() {
  case "$PKG" in
    apt)
      [[ -z "$_apt_refreshed" ]] && { $SUDO apt-get update -qq && _apt_refreshed=1 || true; }
      DEBIAN_FRONTEND=noninteractive $SUDO apt-get install -y -qq "$@" ;;
    dnf)    $SUDO dnf install -y "$@" ;;
    yum)    $SUDO yum install -y "$@" ;;
    apk)    $SUDO apk add "$@" ;;
    pacman) $SUDO pacman -Sy --noconfirm "$@" ;;
    brew)   brew install "$@" ;;
    *)      return 1 ;;
  esac
}

# Best-effort cross-platform Docker install. Returns non-zero (with guidance)
# when it can't proceed unattended (e.g. macOS without Homebrew, Windows).
install_docker() {
  case "$OS" in
    debian|rhel|arch|linux)
      ok "installing Docker via get.docker.com…"
      curl -fsSL https://get.docker.com | $SUDO sh
      [[ -n "$SUDO" ]] && $SUDO usermod -aG docker "$(id -un)" 2>/dev/null || true
      $SUDO systemctl enable --now docker 2>/dev/null \
        || $SUDO service docker start 2>/dev/null || true ;;
    alpine)
      pkg_install docker docker-cli-compose 2>/dev/null || pkg_install docker docker-compose
      $SUDO rc-update add docker boot 2>/dev/null || true
      $SUDO service docker start 2>/dev/null || true ;;
    macos)
      if [[ "$PKG" == brew ]]; then
        ok "installing Docker Desktop via Homebrew…"
        brew install --cask docker
        note "launching Docker Desktop — grant it permissions if prompted"
        open -a Docker 2>/dev/null || true
      else
        err "can't auto-install Docker without Homebrew"
        note "install Homebrew (https://brew.sh) then re-run, or grab Docker Desktop:"
        note "https://www.docker.com/products/docker-desktop"
        return 1
      fi ;;
    windows)
      err "auto-install isn't supported on Windows"
      note "install Docker Desktop (with WSL2 backend): https://www.docker.com/products/docker-desktop"
      note "then re-run this from WSL2 or Git-Bash with the daemon running"
      return 1 ;;
    *)
      err "can't auto-install Docker on this OS"
      note "see https://docs.docker.com/engine/install/"
      return 1 ;;
  esac
}

# Resolve a working docker invocation into $DOCKER. Handles two fresh-install
# quirks: (1) on Linux the current shell isn't in the `docker` group yet, so
# the socket needs sudo this session; (2) macOS Docker Desktop takes a while to
# boot. Polls up to ~60s on macOS, returns 1 if the daemon never answers.
DOCKER="docker"
ensure_docker_running() {
  local tries=0 max=1
  [[ "$OS" == macos ]] && max=30
  while :; do
    if docker info >/dev/null 2>&1; then DOCKER="docker"; return 0; fi
    if [[ -n "$SUDO" ]] && $SUDO docker info >/dev/null 2>&1; then DOCKER="$SUDO docker"; return 0; fi
    tries=$((tries + 1))
    (( tries >= max )) && break
    [[ "$OS" == macos ]] && { note "waiting for Docker to start ($tries/$max)…"; sleep 2; }
  done
  return 1
}

# Ensure a CLI tool is present, installing it via the OS package manager when
# missing. Aborts the installer if it's required and can't be installed.
ensure_tool() {
  local cmd="$1" pkg="${2:-$1}" hint="${3:-}"
  if command -v "$cmd" >/dev/null 2>&1; then
    ok "$cmd ($(command -v "$cmd"))"
    return 0
  fi
  warn "$cmd not found"
  if [[ -n "$PKG" ]] && want_install "install $cmd now?"; then
    pkg_install "$pkg" >/dev/null 2>&1 || pkg_install "$pkg" || true
    if command -v "$cmd" >/dev/null 2>&1; then ok "$cmd installed ($(command -v "$cmd"))"; return 0; fi
  fi
  err "$cmd is required${hint:+ — $hint}"
  exit 1
}

# ---------- arg / env parsing ----------------------------------------------
HEADLESS="${MOAV_HEADLESS:-}"
INSTALL_DIR="${MOAV_DIR:-}"
SUBSCRIPTION="${MOAV_SUBSCRIPTION:-}"
WG_CONF="${MOAV_WG_CONF:-}"
SIDECAR_CSV="${MOAV_SIDECARS:-}"
SKIP_BUILD="${MOAV_SKIP_BUILD:-}"
ASSUME_YES="${MOAV_ASSUME_YES:-}"
NO_DOCKER_INSTALL="${MOAV_NO_DOCKER_INSTALL:-}"

while (( $# > 0 )); do
  case "$1" in
    --headless)        HEADLESS=1 ;;
    --yes|-y)          ASSUME_YES=1 ;;
    --no-docker-install) NO_DOCKER_INSTALL=1 ;;
    --dir)             INSTALL_DIR="$2"; shift ;;
    --subscription)    SUBSCRIPTION="$2"; shift ;;
    --wg-conf)         WG_CONF="$2"; shift ;;
    --sidecars)        SIDECAR_CSV="$2"; shift ;;
    --skip-build)      SKIP_BUILD=1 ;;
    --branch)          REPO_BRANCH="$2"; shift ;;
    --repo)            REPO_URL="$2"; shift ;;
    --help|-h)
      sed -n '2,/^# ===/p' "$0" | sed 's/^#//' | head -n 45
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

detect_os
note "platform: ${OS}${PKG:+  •  package manager: $PKG}${SUDO:+  •  via sudo}"
if [[ "$OS" == "unknown" || ( -z "$PKG" && "$OS" != "macos" ) ]]; then
  warn "unrecognized platform — auto-install of missing tools may not work; install manually if a step fails"
fi

# git / curl / python3 — auto-installed via the package manager when missing.
# python3 drives the config.yaml sidecar-toggle step; without it the install
# aborts mid-config under set -e.
ensure_tool git    git     "https://git-scm.com/downloads"
ensure_tool curl   curl    "install curl via your package manager"
ensure_tool python3 python3 "install python3 via your package manager"

# Docker — auto-install when missing (unless MOAV_NO_DOCKER_INSTALL is set).
if command -v docker >/dev/null 2>&1; then
  ok "docker ($(command -v docker))"
else
  warn "docker not found"
  if [[ -n "$NO_DOCKER_INSTALL" ]]; then
    err "docker is required (auto-install disabled via MOAV_NO_DOCKER_INSTALL)"
    note "https://docs.docker.com/get-docker/"
    exit 1
  elif want_install "install Docker now?"; then
    install_docker || { err "Docker install didn't complete"; exit 1; }
  else
    err "docker is required"
    note "https://docs.docker.com/get-docker/"
    exit 1
  fi
fi

# Resolve a working docker invocation ($DOCKER) — may be "sudo docker" right
# after a fresh Linux install, and waits for Docker Desktop to boot on macOS.
if ! ensure_docker_running; then
  err "docker daemon isn't reachable"
  case "$OS" in
    macos)   note "start Docker Desktop (open -a Docker), wait for it to finish booting, then re-run" ;;
    windows) note "start Docker Desktop and re-run from WSL2 / Git-Bash" ;;
    *)       note "fresh install? log out/in for docker-group membership (or run: newgrp docker), then re-run" ;;
  esac
  exit 1
fi
ok "docker daemon reachable${SUDO:+ (using: $DOCKER)}"

if $DOCKER compose version >/dev/null 2>&1; then
  ok "docker compose ($($DOCKER compose version --short 2>/dev/null))"
else
  warn "docker compose v2 plugin not found"
  if [[ "$PKG" == "apt" ]] && want_install "install the docker compose plugin?"; then
    pkg_install docker-compose-plugin || true
  fi
  if $DOCKER compose version >/dev/null 2>&1; then
    ok "docker compose ($($DOCKER compose version --short 2>/dev/null))"
  else
    err "docker compose v2 plugin not found"
    note "https://docs.docker.com/compose/install/"
    exit 1
  fi
fi

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
  $DOCKER compose build proxy-core web-ui
  if (( ${#profiles[@]} > 0 )); then
    ok "building sidecars: ${SIDECARS[*]}"
    $DOCKER compose "${profiles[@]}" build "${SIDECARS[@]}"
  fi
fi

ok "starting stack…"
$DOCKER compose "${profiles[@]}" up -d
sleep 4

# Quick smoke status.
status="$($DOCKER compose "${profiles[@]}" ps --format 'table {{.Name}}\t{{.Status}}' 2>/dev/null || true)"
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
