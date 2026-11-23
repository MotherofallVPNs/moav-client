# Review rubric

Instructions for the automated PR reviewer (Claude Code `code-review` plugin).
Read this together with `CLAUDE.md`. moav-client is a Go proxy-core plus a
React/TypeScript dashboard (`web-ui`), so review spans backend logic, a UI, and
the security posture of a censorship-circumvention client.

## Output

- Post findings as **inline comments** on the exact `file:line`, and one
  **grouped summary** comment organized by the dimensions below.
- Lead the summary with a one-line verdict and the counts per dimension.
- Cap **Nits at 5**; if there are more, say "plus N similar" in the summary.

## Severity

- **Important** — breaks behavior, leaks data, or is unsafe to ship: incorrect
  routing/failover, a data race, a panic path, unvalidated input on a listener,
  or any secret / server IP / share-URI reaching logs, the UI, or a committed
  file.
- **Nit** — style, naming, small refactors, non-load-bearing comments.
- **Pre-existing** — a real problem the PR did not introduce; report separately,
  don't block on it.

## Verification bar

A finding must cite the `file:line` it's grounded in, not an inference from a
name. If you can't point at the code, drop it or mark it a question. Prefer
false negatives over confident false positives.

---

# Dimensions

Each dimension is self-contained on purpose: the plan is to later split them into
separate review agents/jobs, one per section. Keep findings tagged by dimension.

## 1. Correctness & value

Does the change do what the PR says, and hold up under load and failure?

- The diff matches the PR's stated intent; no scope creep that isn't described.
- **Concurrency**: goroutines, channels, and shared state (balancer pool, prober
  results, stats counters) are race-free and lock-ordered; no leaked goroutines
  or unbounded fan-out. The prober caps concurrency — respect it.
- **Failover & selection**: balancer strategy changes still pick only healthy
  endpoints, exclude tried ones, and fall back to direct on total failure.
- **Parsing**: subscription/URI and config parsers handle malformed input,
  missing fields, and unknown protocols without panicking.
- Error paths return/propagate; no silent `err` drops on a path that matters.
- Edge cases: empty pools, all-endpoints-down, IPv6, ports, timeouts.

## 2. Security & opsec

This is a client people run in hostile networks. Treat leaks as Important.

- **No secrets in output**: UUIDs, passwords, Reality keys, `auth` tokens,
  server IPs, and full share-URIs must not reach logs, the log bus / Debug tab,
  the API responses, or any committed file. Redact before logging.
- Input reaching the SOCKS5 / HTTP CONNECT / API listeners is validated; no
  request smuggling, no unbounded reads.
- No new fingerprintable surface (predictable ports, banners, timing) added
  without reason.
- New dependencies are justified and pinned; no untrusted network calls added.
- Nothing weakens the routing/blocking rules (torrent blocker, plugin engine)
  or the direct-vs-proxy decision in a way that could deanonymize a user.

## 3. Tests

- **Every bug fix ships a regression test in the same PR** (project policy).
- New parser branches, plugin match types, balancer strategies, or API handlers
  come with Go tests; assert the failure mode, not just the happy path.
- `web-ui` changes keep `npm run build` (type-check) green.
- Don't weaken or skip an existing test to make a change pass — fix the cause.

## 4. UI / UX (web-ui, React + TypeScript)

- Matches the dark MoaV admin-panel aesthetic; colours/spacing come from the
  theme tokens (`theme.ts`), not hardcoded hex values.
- Reuses existing components over near-duplicates; state and effects are sound
  (no unkeyed lists, no effect-loop, cleanup on unmount).
- Accessible: keyboard focus is visible, interactive elements are labelled,
  contrast is adequate in the dark theme.
- Loading / empty / error states are handled, not left blank.
- No secrets or raw endpoint credentials rendered into the DOM.

---

## Skip

- Generated / vendored code, `data/`, `geoip/`, build output, `*.min.*`.
- Anything CI already enforces (`go vet`, `gofmt`, the TypeScript build,
  existing test suites) — don't re-litigate formatting.
- Pure dependency-lockfile churn, unless a dependency itself is the concern.
