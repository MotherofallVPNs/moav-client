# Install

The one-liner covers most cases:

```bash
curl -fsSL https://raw.githubusercontent.com/MotherofallVPNs/moav-client/main/install.sh | bash
```

It checks prerequisites (git, curl, docker, docker compose v2, python3) and
**installs the missing ones automatically** — on Linux via the OS package
manager and <https://get.docker.com>, on macOS via Homebrew (Docker Desktop).
Then it clones the repo, lets you pick sidecars, seeds `config.yaml`, builds
images, brings the stack up, and symlinks a global `moav-client` command into
`/usr/local/bin`.

In a headless / piped run (no TTY) the prerequisite installs happen without
prompting. Interactively you're asked first (default yes). Force unattended
installs anywhere with `--yes` / `MOAV_ASSUME_YES=1`.

### Platform support

| Platform | Docker auto-install |
|---|---|
| Debian/Ubuntu, RHEL/Fedora, Arch | `get.docker.com` (+ `usermod -aG docker`, `systemctl enable --now`) |
| Alpine | `apk add docker` (+ `rc-update`, `service docker start`) |
| macOS | `brew install --cask docker` (needs Homebrew); waits for Docker Desktop to boot |
| Windows | manual — install Docker Desktop (WSL2), then run from WSL2 / Git-Bash |

On a fresh Linux install the current shell isn't in the `docker` group yet, so
the installer transparently falls back to `sudo docker` for this run. Log out
and back in (or `newgrp docker`) to use `docker` without `sudo` afterwards.

## Headless / non-interactive

Drive everything from env vars (cloud-init, CI, Ansible):

```bash
MOAV_HEADLESS=1 \
MOAV_DIR=/opt/moav-client \
MOAV_SUBSCRIPTION=/etc/moav/subscription.txt \
MOAV_SIDECARS=masterdns,psiphon \
  bash -c "$(curl -fsSL https://raw.githubusercontent.com/MotherofallVPNs/moav-client/main/install.sh)"
```

Or with flags after a clone:

```bash
git clone https://github.com/MotherofallVPNs/moav-client && cd moav-client
./install.sh --headless --dir /opt/moav-client --sidecars masterdns,psiphon
```

| Env | Flag | Meaning |
|---|---|---|
| `MOAV_HEADLESS=1` | `--headless` | no prompts; core stack + listed sidecars |
| `MOAV_ASSUME_YES=1` | `--yes` / `-y` | auto-confirm prerequisite installs (no prompt) |
| `MOAV_NO_DOCKER_INSTALL=1` | `--no-docker-install` | never auto-install Docker; fail if missing |
| `MOAV_DIR` | `--dir` | install directory (default `~/moav-client`) |
| `MOAV_SUBSCRIPTION` | `--subscription` | path to a `subscription.txt` to wire into `config.yaml` |
| `MOAV_WG_CONF` | `--wg-conf` | WireGuard `.conf` to register |
| `MOAV_SIDECARS` | `--sidecars` | comma list: `masterdns,amneziawg,psiphon,trusttunnel,tor` |
| `MOAV_SKIP_BUILD=1` | `--skip-build` | skip image build (fast re-up) |
| `MOAV_REPO_URL` / `MOAV_REPO_BRANCH` | `--repo` / `--branch` | override source |
| `MOAV_NO_OPEN=1` | — | don't auto-open the dashboard |

Run without a TTY (piped / no `/dev/tty`) and the installer auto-selects
headless with the core stack only.

## Network exposure

By default the SOCKS5 / HTTP / dashboard / API ports bind to `127.0.0.1`
(loopback). To expose on the LAN or publicly, use the dashboard
**Settings → Network exposure** (writes `.env`, re-up to apply) — `loopback`,
`lan`, or `public`, with optional SOCKS5 username/password. Your firewall is
what actually makes `public` reachable.

## Updating

```bash
moav-client update    # git pull + rebuild + restart
```
