# Troubleshooting

Most state is visible in the dashboard: **Endpoints** (health/latency),
**Debug** (logs + per-connection flows), **Diagnostics** (test connectivity
through a chosen endpoint). Start there.

### All endpoints show `error`
- The moav server may be down, or the bundle's credentials/keys are stale.
- Check `moav-client logs proxy-core` for `singbox: wrote N-endpoint config`.
  If sing-box isn't generating, ensure `singbox.enabled: true` (it is by
  default) — without it nothing can be dialed.
- A just-imported bundle needs a restart to load: `moav-client restart` (the
  Configs tab does this for you).

### Dashboard / proxy unreachable from another machine
- Ports bind to `127.0.0.1` by default. Use **Settings → Network exposure** →
  `lan` or `public` (re-up to apply), and open the firewall yourself.

### An endpoint probes `ok` but traffic fails (or vice-versa)
- The prober does a real TLS handshake through the tunnel, so a green probe
  means the tunnel carried bytes end-to-end. If a specific destination fails,
  use **Diagnostics** to TCP/DNS-test it *through* that endpoint.

### A `geoip:` rule isn't blocking/matching
- geoip matches **IP-literal** destinations only — hostnames aren't resolved.
- If `geoip/<cc>.txt` is missing, the rule is inert and proxy-core logs a
  one-time `WARN … rules matching geoip:<cc> are INERT`. Generate the list:
  `scripts/update-geoip.sh <cc>`.

### `block_direct` and direct rules
- The kill-switch only drops the *involuntary* fallback (when every endpoint is
  down). Explicit `direct` rules (e.g. `lan-direct`, `geoip:ir → direct`) are
  always honored — LAN/Iran-direct still work with it on. For a strict
  no-direct policy, also disable those rules. The dashboard toggle lists any
  active `direct` rules when the kill-switch is on.

### A sidecar container is up but not tunneling
- Sidecars need their config: MasterDNS (`domain`+`key`), AmneziaWG/TrustTunnel
  (`source_path`). Importing a bundle wires these automatically; check the
  `sidecars.<kind>.config` block in `config.yaml`.
- Psiphon tunnels out of the box; Tor may take a minute to bootstrap a circuit.
- See [SIDECARS.md](SIDECARS.md).

### Tor shows `unhealthy`
- Expected only briefly during bootstrap. The healthcheck is a SOCKS port
  check; the dashboard probe validates real egress.

### Can't switch branches / `git` shows only one commit
- The installer makes a **shallow, single-branch clone** (`--depth=1 --branch
  main`), so `git branch -a` shows only `main` and `git checkout dev` just
  copies it. Use the wrapper, which fetches the branch's tip explicitly:
  ```bash
  moav-client update -b dev      # or any branch
  ```
- To recover by hand:
  ```bash
  cd ~/moav-client
  git fetch --depth=1 origin dev
  git checkout -B dev FETCH_HEAD
  moav-client restart
  ```

### Reset / remove everything
```bash
moav-client uninstall          # stop + remove containers (config + data kept)
moav-client uninstall --wipe   # also delete config.yaml, .env, data/, volumes & images
```
To stop without removing: `moav-client down` (add `--profile all-sidecars -v`
on a raw `docker compose down` to also drop sidecar volumes).
