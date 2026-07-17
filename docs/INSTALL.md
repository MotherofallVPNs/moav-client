# Install

The one-liner covers most cases:

```bash
curl -fsSL moav.sh/client-install.sh | bash
```

It checks prerequisites (git, curl, docker, docker compose v2, python3) and
**installs the missing ones automatically** — on Linux via the OS package
manager and <https://get.docker.com>, on macOS via Homebrew (Docker Desktop).
Then it clones the repo, lets you pick sidecars, seeds `config.yaml`, builds
images, brings the stack up, and symlinks a global **`moavc`** command into
`/usr/local/bin` (the full name `moav-client` is installed too — use whichever
you prefer).

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
  bash -c "$(curl -fsSL moav.sh/client-install.sh)"
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

The installer prompts interactively even under `curl … | bash` (it uses
`/dev/tty`). It only auto-selects headless (core stack, no prompts) when there
is genuinely no terminal — true cloud-init / CI / cron, or `--headless`.

## Choosing sidecars

The wizard shows a numbered catalog; enter the numbers you want
(`1 3`, `all`, or blank for none). Only the images you pick are built. Re-run
the wizard any time to add or remove sidecars — already-enabled ones are
pre-checked:

```bash
moavc install
```

You can also add/remove a single sidecar without the wizard:

```bash
moavc sidecar add psiphon      # enable + build + start
moavc sidecar remove psiphon   # stop + disable
moavc sidecar list
```

If you enable a protocol in the dashboard whose sidecar image was never built,
the dashboard tells you to run `moavc sidecar add <name>` to provision it.

## Network exposure

By default the SOCKS5 / HTTP / dashboard / API ports bind to `127.0.0.1`
(loopback). The installer asks once, at the end, whether to open it to your LAN
(and offers to set a dashboard password). Change it any time from the CLI:

```bash
moavc expose lan                        # reachable from other LAN devices
moavc expose lan --user me --password s3cret   # + dashboard auth
moavc expose loopback                   # back to localhost only
```

or from the dashboard **Settings → Network exposure** — both write the same
`.env` keys (`MOAV_EXPOSURE`, `*_BIND`, `MOAV_DASHBOARD_*`) and recreate the
containers. `lan` and `public` both bind `0.0.0.0`; your firewall / router is
what actually makes `public` reachable. Always set a dashboard password before
exposing.

## Versions & pinning

`VERSION` holds the client version (shown by `moavc version` and the
dashboard footer). Component versions are pinned in `.env` — copy the commented
block from `.env.example` to override:

- `XRAY_VERSION` — official XTLS release tag built by `sidecars/xray/Dockerfile`
  (pre-releases like `v26.5.9` are fine).
- `IMAGE_SINGBOX` / `IMAGE_TOR` / `IMAGE_CADDY` — full `repo:tag` of the pulled
  images.

After changing a version: `moavc up` (or `docker compose up -d --build`).

## Updating

```bash
moavc update              # pull current branch + rebuild + restart
moavc update -b dev       # switch to (and track) another branch
```

The installer makes a shallow, single-branch clone, so `update` fetches the
target branch's tip explicitly — `-b <branch>` works even though
`git branch -a` only shows `main`.

## Uninstalling

```bash
moavc uninstall           # stop + remove containers (config + data kept)
moavc uninstall --wipe    # also delete config.yaml, .env, data/, volumes & images
```

Both also remove the global `moavc` / `moav-client` commands. The repo clone is
left in place — `rm -rf ~/moav-client` to remove it too.
