#!/usr/bin/env bash
# =============================================================================
#  moav-client installer
#
#  One-liner install:
#    curl -fsSL moav.sh/client-install.sh | bash
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

# This script uses bash 4+ features (associative-style indexing, ${var,,},
# mapfile). macOS ships bash 3.2, so a stock `curl … | bash` would die with a
# cryptic "bad substitution". Re-exec under a newer bash if one is on PATH
# (e.g. Homebrew's), otherwise fail with a clear, actionable message.
if [[ -z "${BASH_VERSINFO:-}" || "${BASH_VERSINFO[0]}" -lt 4 ]]; then
  # Re-exec under a newer bash, but only when invoked as a real file — under
  # `curl … | bash` the script is on stdin ($0 is "bash") and can't be re-run.
  if [[ -f "$0" ]]; then
    for _b in /opt/homebrew/bin/bash /usr/local/bin/bash; do
      if [[ -x "$_b" ]] && "$_b" -c '[[ "${BASH_VERSINFO[0]}" -ge 4 ]]' 2>/dev/null; then
        exec "$_b" "$0" "$@"
      fi
    done
  fi
  echo "moav-client install requires bash 4 or newer (you have ${BASH_VERSION:-unknown})." >&2
  echo "  macOS:  brew install bash  then re-run with that bash, e.g.:" >&2
  echo "          \$(brew --prefix)/bin/bash -c \"\$(curl -fsSL <install-url>)\"" >&2
  exit 1
fi

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
  # -e (readline) so arrow keys do line-editing instead of injecting ^[[A.
  # Bold cyan »-prefixed prompt so it's unmistakably a question awaiting input.
  read -e -r -p "$(printf '\n  %s» %s%s %s[Y/n]%s ' "$C_BOLD$C_CYAN" "$1" "$C_RESET" "$C_DIM" "$C_RESET")" ans </dev/tty 2>/dev/null || ans=""
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

# Set KEY=VALUE in an .env file — replace in place if present, else append.
# Mirrors the key contract proxy-core's handleExposure writes, so the dashboard
# and installer/CLI stay consistent.
set_env_kv() {
  local key="$1" val="$2" file="${3:-.env}"
  touch "$file"
  if grep -qE "^${key}=" "$file" 2>/dev/null; then
    sed -i.bak -E "s|^${key}=.*|${key}=${val}|" "$file" && rm -f "${file}.bak"
  else
    printf '%s=%s\n' "$key" "$val" >>"$file"
  fi
}

# Generate a random alphanumeric password (default 16 chars).
gen_password() {
  local n="${1:-16}"
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -base64 48 | tr -dc 'A-Za-z0-9' | head -c "$n"
  elif [[ -r /dev/urandom ]]; then
    LC_ALL=C tr -dc 'A-Za-z0-9' </dev/urandom | head -c "$n"
  else
    printf '%s' "${RANDOM}${RANDOM}${RANDOM}${RANDOM}" | head -c "$n"
  fi
}

# Best-effort primary private IPv4 (what other LAN devices would dial).
lan_ip() {
  local ip=""
  if command -v ip >/dev/null 2>&1; then
    ip=$(ip -4 route get 1.1.1.1 2>/dev/null | sed -n 's/.* src \([0-9.]*\).*/\1/p' | head -1)
  fi
  [[ -z "$ip" ]] && command -v hostname >/dev/null 2>&1 && ip=$(hostname -I 2>/dev/null | awk '{print $1}')
  [[ -z "$ip" ]] && command -v ipconfig >/dev/null 2>&1 && ip=$(ipconfig getifaddr en0 2>/dev/null)
  echo "$ip"
}

# True if the IPv4 is in a private / non-routable range (RFC1918, CGNAT,
# link-local, loopback). A *false* result on a host's primary IP means it's a
# public address — i.e. a VPS/cloud box, where "LAN" exposure is really public.
is_private_ipv4() {
  local ip="$1"
  case "$ip" in
    10.*|192.168.*|127.*|169.254.*) return 0 ;;
    172.1[6-9].*|172.2[0-9].*|172.3[01].*) return 0 ;;
    100.6[4-9].*|100.[7-9][0-9].*|100.1[01][0-9].*|100.12[0-7].*) return 0 ;; # CGNAT 100.64/10
    *) return 1 ;;
  esac
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

# A controlling terminal exists if /dev/tty is openable — true even under
# `curl … | bash`, where stdin is the pipe but the terminal is still attached.
# This is what lets the wizard prompt when piped.
tty_available() { { : >/dev/tty; } 2>/dev/null; }

# Detect interactive vs truly non-interactive. Only fall back to headless when
# the caller didn't ask for it AND stdin isn't a TTY AND there's no usable
# /dev/tty to prompt on (genuine cloud-init / CI / cron). Piped-but-attached
# (curl | bash from a real shell) stays interactive.
if [[ -z "$HEADLESS" && ! -t 0 ]] && ! tty_available; then
  HEADLESS=auto
fi

# ---------- component catalog ----------------------------------------------
# Core is always installed; sidecars are opt-in. Sizes are rough estimates
# (download = compressed layers fetched/built; disk = final image on disk).
#   comp_meta <kind> -> "<download MB>|<disk MB>|<Label>|<one-liner>"
CORE_KINDS=(proxy-core web-ui sing-box xray)
SIDECAR_KINDS=(masterdns amneziawg psiphon trusttunnel tor)
comp_meta() {
  case "$1" in
    proxy-core)  echo "8|18|proxy-core|Go binary — SOCKS5 / HTTP CONNECT + balancer + API" ;;
    web-ui)      echo "30|76|web-ui|React dashboard (nginx-alpine + built assets)" ;;
    sing-box)    echo "50|116|sing-box|VLESS / Reality / Trojan / SS / Hysteria2 / WireGuard crypto" ;;
    xray)        echo "25|66|xray|xhttp / splithttp transports (official XTLS binary)" ;;
    masterdns)   echo "55|138|MasterDNS|DNS-tunnel client for MoaV DNS tunnels (m.<bundle>.<tld>)" ;;
    amneziawg)   echo "60|149|AmneziaWG|amneziawg-go + microsocks (needs NET_ADMIN + /dev/net/tun)" ;;
    psiphon)     echo "70|176|Psiphon|Psiphon ConsoleClient — connects via embedded config" ;;
    trusttunnel) echo "60|147|TrustTunnel|HTTP/2 + HTTP/3 tunnel (official client; needs client.toml from your bundle)" ;;
    tor)         echo "30|86|Tor|Tor SOCKS5 on :9150 (peterdavehello/tor-socks-proxy)" ;;
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
  ok "existing repo at $INSTALL_DIR — updating to origin/$REPO_BRANCH"
  # Fetch just the target branch's tip and point the local branch at it. Works
  # on the shallow, single-branch clone this installer creates (where
  # `checkout <other-branch>` / origin/<other> don't exist). config.yaml / .env
  # / data/ are gitignored, so this won't clobber the user's settings.
  if git -C "$INSTALL_DIR" fetch --depth=1 --quiet origin "$REPO_BRANCH" 2>/dev/null; then
    git -C "$INSTALL_DIR" checkout -B "$REPO_BRANCH" FETCH_HEAD --quiet 2>/dev/null \
      || git -C "$INSTALL_DIR" pull --quiet --ff-only origin "$REPO_BRANCH" \
      || warn "couldn't update; leaving working tree as-is"
  else
    warn "couldn't fetch origin/$REPO_BRANCH; leaving working tree as-is"
  fi
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

# Sidecars already enabled in an existing config.yaml (re-run case) — printed
# one per line so the wizard can pre-check them. Empty on a fresh install.
current_enabled_sidecars() {
  local cfg="$INSTALL_DIR/config.yaml"
  [[ -f "$cfg" ]] || return 0
  python3 - "$cfg" <<'PY' 2>/dev/null || true
import re, sys, pathlib
src = pathlib.Path(sys.argv[1]).read_text()
for kind in ("masterdns","amneziawg","psiphon","trusttunnel","tor"):
    m = re.search(r'^\s*'+kind+r':\s*\n(?:\s*#.*\n)*?\s*enabled:\s*(true|false)', src, re.MULTILINE)
    if m and m.group(1) == "true":
        print(kind)
PY
}

# Unified catalog: core shown checked ([x], green, always on) and optional
# sidecars shown as numbered checkboxes ([x] if pre-selected). Aligned columns
# so both sections read consistently. Populates SIDECAR_INDEX (number→kind).
declare -a SIDECAR_INDEX=()
print_catalog() {
  local preselected=("$@") i=1 dl disk label desc mark mcolor
  echo "  ${C_BOLD}Core stack${C_RESET} ${C_DIM}— always installed${C_RESET}"
  for kind in "${CORE_KINDS[@]}"; do
    IFS='|' read -r dl disk label desc <<<"$(comp_meta "$kind")"
    printf '    %s[x]%s     %s%-12s%s %s~%4s MB%s   %s%s%s\n' \
      "$C_GREEN" "$C_RESET" "$C_BOLD" "$label" "$C_RESET" "$C_DIM" "$disk" "$C_RESET" "$C_DIM" "$desc" "$C_RESET"
  done
  echo ""
  echo "  ${C_BOLD}Optional sidecars${C_RESET} ${C_DIM}— select any; only chosen images are built${C_RESET}"
  SIDECAR_INDEX=()
  for kind in "${SIDECAR_KINDS[@]}"; do
    IFS='|' read -r dl disk label desc <<<"$(comp_meta "$kind")"
    mark=" "; mcolor="$C_DIM"
    if printf '%s\n' "${preselected[@]:-}" | grep -qx "$kind"; then mark="x"; mcolor="$C_GREEN"; fi
    SIDECAR_INDEX[$i]="$kind"
    printf '    %s[%s]%s %s%d)%s %s%-12s%s %s~%4s MB%s   %s%s%s\n' \
      "$mcolor" "$mark" "$C_RESET" "$C_BOLD" "$i" "$C_RESET" "$C_BOLD" "$label" "$C_RESET" \
      "$C_DIM" "$disk" "$C_RESET" "$C_DIM" "$desc" "$C_RESET"
    i=$((i + 1))
  done
  echo ""
}

# Parse a selection line ("1 3", "all", or blank) against SIDECAR_INDEX.
# Blank keeps the pre-selected set. Emits chosen keys one per line.
parse_sidecar_choice() {
  local ans="$1"; shift
  local preselected=("$@") picked=() tok
  if [[ -z "${ans// /}" ]]; then
    (( ${#preselected[@]} )) && printf '%s\n' "${preselected[@]}"
    return 0
  fi
  if [[ "${ans,,}" == "all" ]]; then
    printf '%s\n' "${SIDECAR_KINDS[@]}"
    return 0
  fi
  for tok in ${ans//,/ }; do
    [[ "$tok" =~ ^[0-9]+$ && -n "${SIDECAR_INDEX[$tok]:-}" ]] && picked+=("${SIDECAR_INDEX[$tok]}")
  done
  (( ${#picked[@]} )) && printf '%s\n' "${picked[@]}"
}

SIDECARS=()
mapfile -t PRESELECTED < <(current_enabled_sidecars)

if [[ -n "$SIDECAR_CSV" ]]; then
  print_catalog "${PRESELECTED[@]:-}"
  IFS=',' read -r -a SIDECARS <<<"$SIDECAR_CSV"
  ok "sidecars from --sidecars / MOAV_SIDECARS: ${SIDECARS[*]:-none}"
elif [[ "$HEADLESS" == "1" || "$HEADLESS" == "auto" ]]; then
  print_catalog "${PRESELECTED[@]:-}"
  # Headless keeps whatever's already enabled (re-run) or none (fresh install).
  (( ${#PRESELECTED[@]} )) && SIDECARS=("${PRESELECTED[@]}")
  if [[ "$HEADLESS" == "auto" ]]; then
    warn "non-interactive (no TTY) and no MOAV_SIDECARS — keeping ${SIDECARS[*]:-core stack only}"
  else
    ok "headless: sidecars = ${SIDECARS[*]:-none} (set MOAV_SIDECARS to change)"
  fi
else
  print_catalog "${PRESELECTED[@]:-}"
  # -e so arrow keys edit the line instead of pasting escape codes (^[[A).
  read -e -r -p "$(printf '  %s» enable which?%s type numbers e.g. %s1 3%s, %sall%s, or blank to keep current: ' "$C_BOLD$C_CYAN" "$C_RESET" "$C_BOLD" "$C_RESET" "$C_BOLD" "$C_RESET")" choice </dev/tty || choice=""
  mapfile -t SIDECARS < <(parse_sidecar_choice "$choice" "${PRESELECTED[@]:-}")
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

# Tally estimated download + on-disk size for core + selected sidecars
# (used by the step-5 summary table).
DL_TOTAL=0; DISK_TOTAL=0
size_add() {
  local dl disk
  IFS='|' read -r dl disk _ _ <<<"$(comp_meta "$1")"
  DL_TOTAL=$((DL_TOTAL + dl)); DISK_TOTAL=$((DISK_TOTAL + disk))
}
for k in "${CORE_KINDS[@]}"; do size_add "$k"; done
for k in "${SIDECARS[@]:-}"; do [[ -z "$k" ]] && continue; size_add "$k"; done
echo ""
ok "selected sidecars: ${SIDECARS[*]:-none}"

if [[ "$DF_AVAIL_MB" != "?" && "$DF_AVAIL_MB" -lt $((DISK_TOTAL + 800)) ]]; then
  warn "free disk ($DF_AVAIL_MB MB) is tight for ~${DISK_TOTAL} MB of images + build cache."
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
    read -e -r -p "  path to your MoaV subscription.txt (blank to skip): " SUBSCRIPTION </dev/tty || SUBSCRIPTION=""
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

# Confirm before the (potentially multi-minute) build & start. Show what will be
# built and a per-component download/disk estimate so there are no surprises.
build_list=("${CORE_KINDS[@]}")
for k in "${SIDECARS[@]:-}"; do [[ -n "$k" ]] && build_list+=("$k"); done

row() { printf '    %s%-13s%s %s%9s%s   %s%9s%s\n' "$1" "$2" "$C_RESET" "$C_DIM" "$3" "$C_RESET" "$C_DIM" "$4" "$C_RESET"; }
echo "  ${C_BOLD}About to build & start${C_RESET} ${C_DIM}(estimates — first build is slower than re-runs)${C_RESET}"
echo ""
printf '    %s%-13s %9s   %9s%s\n' "$C_BOLD" "component" "download" "disk" "$C_RESET"
say "$C_DIM" "    ───────────────────────────────────────"
for k in "${build_list[@]}"; do
  IFS='|' read -r dl disk label _ <<<"$(comp_meta "$k")"
  in_core=0; for c in "${CORE_KINDS[@]}"; do [[ "$c" == "$k" ]] && in_core=1; done
  row "$([[ $in_core == 1 ]] && printf '%s' "$C_GREEN" || printf '%s' "$C_BOLD")" "$label" "~${dl}M" "~${disk}M"
done
say "$C_DIM" "    ───────────────────────────────────────"
printf '    %s%-13s %9s   %9s%s\n' "$C_BOLD" "total" "~${DL_TOTAL}M" "~${DISK_TOTAL}M" "$C_RESET"
[[ "$DF_AVAIL_MB" != "?" ]] && note "free disk at install path: ${DF_AVAIL_MB} MB"
if ! want_install "build & start now? — press Enter to continue"; then
  echo ""
  warn "skipping build/start at your request."
  note "when ready, run:  ${C_BOLD}moavc up${C_RESET}   (or: docker compose ${profiles[*]:-} up -d --build)"
  exit 0
fi

if [[ -n "$SKIP_BUILD" ]]; then
  warn "MOAV_SKIP_BUILD set — skipping image build"
else
  ok "building core images (proxy-core + web-ui + xray) — this can take 1–3 min on first run"
  $DOCKER compose build proxy-core web-ui xray
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

# ---------- install the `moavc` / `moav-client` commands globally -----------
# Symlink the management wrapper into PATH so it's usable from anywhere (the
# wrapper resolves the symlink back to this install dir). `moavc` is the short
# official alias; `moav-client` is kept too. Best-effort: skip quietly if we
# can't write and have no sudo.
WRAPPER="$INSTALL_DIR/moav-client"
HAVE_GLOBAL=
# Helper: ln -sf with a sudo fallback. Sets HAVE_GLOBAL on first success.
link_global() {
  local dest="$1"
  if [[ -w "$(dirname "$dest")" ]]; then
    ln -sf "$WRAPPER" "$dest" 2>/dev/null && return 0
  elif command -v sudo >/dev/null 2>&1; then
    sudo ln -sf "$WRAPPER" "$dest" 2>/dev/null && return 0
  fi
  return 1
}
if [[ -x "$WRAPPER" ]]; then
  link_global "/usr/local/bin/moavc"       && HAVE_GLOBAL=1
  link_global "/usr/local/bin/moav-client" || true
  if [[ -n "$HAVE_GLOBAL" ]]; then
    ok "installed 'moavc' command (alias of moav-client) → /usr/local/bin/moavc"
  else
    warn "couldn't symlink into /usr/local/bin (no write access / no sudo) — use ./moav-client from $INSTALL_DIR"
  fi
fi
# CLI prefix used in the closing tips: short alias if global, else ./relative.
MC="moavc"
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

# ---------- LAN exposure (last interactive question) -----------------------
# By default every port binds to 127.0.0.1. Offer to open the dashboard + proxy
# to the LAN now so the user doesn't have to hunt for the Settings tab. Skipped
# in headless mode. Mirrors proxy-core's handleExposure .env key contract.
if [[ "$HEADLESS" != "1" && "$HEADLESS" != "auto" ]]; then
  echo ""
  # Explicit opt-in even under --yes — exposing to the network is security
  # sensitive, so it should never be auto-confirmed. Default is No.
  read -e -r -p "$(printf '  %s» make the dashboard + proxy reachable from other devices on your LAN?%s [y/N] ' "$C_BOLD$C_CYAN" "$C_RESET")" lan_ans </dev/tty || lan_ans=""
  if [[ "${lan_ans,,}" == y || "${lan_ans,,}" == yes ]]; then
    for k in SOCKS5_BIND HTTP_BIND API_BIND UI_BIND; do set_env_kv "$k" "0.0.0.0" "$ENVF"; done
    set_env_kv "MOAV_EXPOSURE" "lan" "$ENVF"
    # A LAN-reachable dashboard with no password is a foot-gun — offer one.
    read -e -r -p "$(printf '  %s» set a dashboard username/password? (recommended)%s [Y/n] ' "$C_BOLD$C_CYAN" "$C_RESET")" pw_ans </dev/tty || pw_ans=""
    if [[ "${pw_ans,,}" != n && "${pw_ans,,}" != no ]]; then
      read -e -r -p "    dashboard username [admin]: " du </dev/tty || du=""
      read -r -s -p "    dashboard password (leave empty to auto-generate): " dp </dev/tty || dp=""; echo ""
      [[ -z "$du" ]] && du="admin"
      pw_generated=""
      if [[ -z "$dp" ]]; then dp="$(gen_password 16)"; pw_generated=1; fi
      set_env_kv "MOAV_DASHBOARD_USER" "$du" "$ENVF"
      set_env_kv "MOAV_DASHBOARD_PASS" "$dp" "$ENVF"
      if [[ -n "$pw_generated" ]]; then
        echo ""
        ok "generated a dashboard password — ${C_BOLD}save it somewhere safe now:${C_RESET}"
        echo "        ${C_BOLD}user:${C_RESET} ${du}"
        echo "        ${C_BOLD}pass:${C_RESET} ${C_GREEN}${dp}${C_RESET}"
        note "also stored in ${ENVF} (MOAV_DASHBOARD_PASS); change later via Settings → Network exposure."
      else
        ok "dashboard auth set for user '$du'"
      fi
    fi
    ok "applying LAN exposure — recreating proxy-core + web-ui…"
    $DOCKER compose "${profiles[@]}" up -d --force-recreate proxy-core web-ui >/dev/null 2>&1 || \
      $DOCKER compose "${profiles[@]}" up -d --force-recreate proxy-core web-ui
    LANIP="$(lan_ip)"
    echo ""
    if [[ -n "$LANIP" ]] && ! is_private_ipv4 "$LANIP"; then
      # The host's primary IP is public — this is a VPS/cloud box, so "LAN"
      # exposure is really internet-facing. Make that loud.
      err "⚠ ${LANIP} is a PUBLIC IP — this host looks internet-facing (VPS/cloud)."
      warn "the dashboard + proxy are now reachable from the INTERNET, not just a local network."
      if [[ -z "${dp:-}" ]]; then
        warn "you did NOT set a dashboard password — anyone on the internet can open the control panel."
        note "lock it down now:  ${MC} expose lan --password <pw>   (or Settings → Network exposure)"
      fi
      echo ""
    fi
    ok "now reachable at:"
    note "Dashboard:    http://${LANIP:-<this-host-ip>}:3001"
    note "SOCKS5 proxy: ${LANIP:-<this-host-ip>}:1080"
    note "change later with: ${MC} expose loopback   (or lan | public)"
  else
    note "staying on localhost only. Open it later with: ${MC} expose lan"
  fi
fi

if printf '%s\n' "${SIDECARS[@]:-}" | grep -qx "masterdns"; then
  echo ""
  warn "MasterDNS is started but idle until you give it your MoaV DNS-tunnel config."
  note "Fill sidecars.masterdns.config (domain / method / key) in config.yaml from your bundle, then:"
  note "  ${MC} sidecar add masterdns   (rebuilds + restarts it)"
fi

if printf '%s\n' "${SIDECARS[@]:-}" | grep -qx "trusttunnel"; then
  echo ""
  warn "TrustTunnel is started but idle until you give it your TrustTunnel config."
  note "Importing a MoaV bundle wires sidecars.trusttunnel.config.source_path (client.toml) for you;"
  note "otherwise point it at your client.toml, then:  ${MC} sidecar add trusttunnel"
fi

echo ""
