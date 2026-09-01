# Automated PR review with Claude

`.github/workflows/claude-review.yml` runs the official
[`anthropics/claude-code-action`](https://github.com/anthropics/claude-code-action)
with the `code-review` plugin. It reads the PR diff, reviews it against
[`REVIEW.md`](../REVIEW.md), and posts inline comments plus a grouped summary.
It is **review-only** — it never commits, pushes, or merges — and the check is
**advisory** (it does not block merging). It augments human review; it doesn't
replace it.

## One-time setup: the token

Auth is via a **Claude subscription** OAuth token (no API key needed):

1. On a machine with Claude Code, run:
   ```bash
   claude setup-token
   ```
2. Copy the token, then add it in GitHub under
   **Settings → Secrets and variables → Actions → New repository secret**
   (or at the org level to share it across repos):
   - Name: `CLAUDE_CODE_OAUTH_TOKEN`
   - Value: the token
3. Approve the **Claude GitHub App** for the repo if prompted (the workflow
   authenticates as that app to post comments — it needs no personal token).

The token is tied to the subscriber's account; usage counts against that
subscription.

## How to trigger it (admin-only for now)

The reviewer runs only when a maintainer asks — it is intentionally not on every
PR yet:

- **Comment** `@claude review` on the PR. Only org members with write access
  (`OWNER` / `MEMBER` / `COLLABORATOR`) can trigger it; other people's comments
  are ignored.
- **Manual**: Actions tab → *Claude PR Review* → *Run workflow* → enter the PR
  number.

Re-comment `@claude review` after pushing changes to get a fresh pass.

## Issue triage

A companion workflow (`.github/workflows/claude-issue-triage.yml`) triages issues
the same way — admin-triggered and advisory. It reads the issue, applies labels
from the repo's **existing** label set, and posts one concise triage comment
(category, likely area, clarifying questions), following the rubric in
[`TRIAGE.md`](../TRIAGE.md). It never edits code, opens a PR, or closes issues.

- **Trigger:** a maintainer comments `@claude triage` on an issue, or runs
  *Actions → Claude Issue Triage → Run workflow* with an issue number.
- **Security:** for a circumvention client, the rubric tells it never to echo a
  leaked secret / server IP / share-URI back into a comment — it flags it and
  asks the reporter to redact.
- **Enable auto-triage** of every new issue: uncomment the `issues` trigger in
  the workflow (same pattern as enabling auto-review).

It uses the same `CLAUDE_CODE_OAUTH_TOKEN` secret and the Claude GitHub App
(which carries `issues: write`), so no extra setup beyond the token above.

## Where to tweak

### Turn on automatic review (the "hybrid" model)

Once the rubric is tuned and the signal-to-noise is good, edit
`.github/workflows/claude-review.yml`:

1. Uncomment the `pull_request` trigger (`opened`, `ready_for_review`,
   `reopened`).

That's the only change needed — the job's `if:` already carries a **diff-size
guard** (`additions < 600`) on the `pull_request` branch, so very large PRs skip
auto-review (a maintainer can still run them on demand). Adjust the threshold
there if you like.

Keep the comment and manual triggers so a maintainer can still re-run on demand.
That combination — auto on open + on-demand — is the hybrid model.

### Split into per-dimension agents

`REVIEW.md` is written so each dimension (Correctness, Security & opsec, Tests,
UI/UX) is self-contained. To run them as separate, focused reviewers (e.g. a
stricter security pass that fails the check, and a lighter style pass that
doesn't), duplicate the `review` job per dimension and point each one's prompt
at its section — for example scope the prompt to "review only the **Security &
opsec** dimension of REVIEW.md". Give each its own summary heading so findings
stay grouped. Start with one combined pass (current setup); split only where a
dimension earns its own cadence or gating.

## Cost and noise

- A review costs subscription usage per run; admin-gating keeps volume bounded
  while we calibrate.
- Nits are capped (see `REVIEW.md`); tighten severity there if reviews feel
  noisy.
- The check never blocks a merge. If you later want to gate on Important
  findings, do it in a branch-protection rule against the review's output, not
  by making the job fail.

## Rolling out to other repos

Copy the workflow and add a repo-specific `REVIEW.md` (the rubric differs per
repo — e.g. moav-site is docs/translation-focused, the MoaV server is
bash/opsec-focused). The `CLAUDE_CODE_OAUTH_TOKEN` secret can live at the org
level so every repo shares it.
