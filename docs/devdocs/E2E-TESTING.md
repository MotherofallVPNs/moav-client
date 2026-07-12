# moav-client end-to-end testing (self-hosted runner)

The per-PR CI (`.github/workflows/ci.yml`) lints, unit-tests, builds, and
validates the compose file — it never brings the stack up. The **live e2e**
(`.github/workflows/e2e.yml` → `tests/e2e-client.sh`) stands the client stack up
against a **real MoaV server** and proves traffic is actually tunnelled:

1. the server bundle's endpoints get parsed,
2. moav-client's `/api/probe` validates ≥1 endpoint (tunnel + real TLS
   handshake — the same check the dashboard's Probe button runs), and
3. a fetch through the aggregate SOCKS5 `:1080` **exits from the server's IP**,
   not the runner's.

It runs on a **self-hosted runner** (label `moav-client-e2e`) because it needs
Docker, the ~11-image client stack, and outbound reach to a MoaV server —
things a stock GitHub-hosted runner can't provide. Triggered manually
(`workflow_dispatch`) and on published releases.

> The client e2e does **not** build a MoaV server. It consumes a bundle /
> subscription you provide (§2). See §4 for the planned cross-repo flow where a
> server on the box mints a fresh bundle each run.

---

## 1. Register the runner

Same box as the server e2e VPS is fine — just add a **second** runner with a
different label so the two repos' jobs don't collide. As a non-root user with
Docker access + passwordless `sudo` (the runner's `config.sh` refuses root):

```bash
su - gh-runner
mkdir -p ~/actions-runner-client && cd ~/actions-runner-client
# download the runner (see github.com/actions/runner/releases/latest), then:
./config.sh --url https://github.com/MotherofallVPNs/moav-client \
  --token <FRESH_REGISTRATION_TOKEN> \
  --labels moav-client-e2e \
  --name moav-client-e2e-vps
sudo ./svc.sh install gh-runner && sudo ./svc.sh start
```

The workflow selects it via `runs-on: [self-hosted, moav-client-e2e]`. As with
the server runner, add the reclaim-workspace pre-job hook (MoaV/client
containers leave root-owned files in bind mounts) — see the server repo's
`docs/devdocs/E2E-TESTING.md` for that hook; it's identical here.

## 2. Provide a test server (the bundle) + secrets

repo → **Settings → Secrets and variables → Actions**:

| Secret | Value |
|---|---|
| `MOAV_TEST_SUB_URL` | *(option A)* a `moav://` / subscription URL for a fully-running MoaV **test** server |
| `MOAV_TEST_BUNDLE_B64` | *(option B)* base64 of a MoaV user bundle `.zip` for that test server, no newlines — Linux: `base64 -w0 bundle.zip`; macOS: `base64 -i bundle.zip \| tr -d '\n'` (add `\| pbcopy` to copy it) |
| `MOAV_TEST_EXIT_IP` | that server's public egress IP — the exact-match assertion. Omit to only assert "proxied ≠ runner IP" |

Provide **one** of the bundle secrets. To make a bundle: on the MoaV server,
`moav user add e2e-client` and zip `outputs/bundles/e2e-client/`, or hand out a
subscription URL. Use a **throwaway** test server, not production — the bundle
holds working credentials and lives in CI secrets.

## 3. Run it

- **Manually:** Actions → **e2e** → Run workflow (tick *verbose* to always dump
  client logs). If "e2e" isn't listed, the workflow isn't on the default branch
  yet (a `workflow_dispatch` rule).
- **On release:** fires when a Release is published.

The **moav-client-e2e** artifact (endpoints/probe JSON + compose logs) is
attached to every run. The job fails if no endpoint parses, if `/api/probe`
validates zero endpoints, or if the exit IP doesn't match / isn't proxied.

### By hand (no CI)

```bash
MOAV_TEST_SUB_URL="moav://…" MOAV_TEST_EXIT_IP="203.0.113.9" bash tests/e2e-client.sh
# or a bundle file:
MOAV_TEST_BUNDLE=/path/to/bundle.zip MOAV_TEST_EXIT_IP="203.0.113.9" bash tests/e2e-client.sh
```

---

## 4. Planned: cross-repo fresh-provisioning flow

The bundle-secret approach (§2) is the **near-term** path — simple, no cross-repo
wiring. Its weakness: a static bundle **goes stale** when the server rotates keys
or certs, and the e2e then fails for a non-code reason. The **ideal** flow mints a
fresh bundle each run. This needs deliberate planning (hence not built yet):

**Design.**
1. Keep a **dedicated long-lived MoaV server** on the box, *separate from the
   server-repo's e2e stack* (that stack tears itself down between runs and would
   race this). Domainless mode is enough for the IP-only protocols and needs no
   cert; add a domain instance if you want the TLS-domain protocols covered too.
2. A pre-step in this workflow provisions against it and reads the bundle:
   ```bash
   sudo /opt/moav/moav.sh user revoke e2e-client 2>/dev/null || true
   sudo /opt/moav/moav.sh user add e2e-client
   zip -jr /tmp/moav-e2e-bundle.zip /opt/moav/outputs/bundles/e2e-client
   ```
   then feed `/tmp/moav-e2e-bundle.zip` to `tests/e2e-client.sh` (which already
   accepts `MOAV_TEST_BUNDLE`), and `MOAV_TEST_EXIT_IP` = that server's IP.
3. Teardown revokes the `e2e-client` user.

**Open questions to settle before building it:**
- **Ownership** — should this live in the client repo (reaching into the server
  install) or be a small shared "provision-a-bundle" script versioned in the
  server repo and called by both? A shared script avoids the client repo knowing
  server internals.
- **Concurrency across repos** — a `moav user add` while the server-repo e2e is
  wiping state on the same box will flake. Either use a distinct always-up
  server for provisioning, or a cross-repo lock.
- **Protocol coverage** — a domainless provisioning server only exercises the
  IP-only protocols client-side; the TLS-domain ones need a domain server (and
  its LE rate limit) kept up.

Until that's built, keep `MOAV_TEST_BUNDLE_B64` fresh (regenerate when the test
server's keys change).
