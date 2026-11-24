#!/usr/bin/env python3
"""Format a Telegram message for the MoaV notifier from the GitHub event env.

Emits the message body to stdout (Telegram HTML parse_mode), or NOTHING when the
event shouldn't be announced (e.g. an issue labeled with something other than the
configured announce label) — the workflow skips sending on empty output.

Env in: EVENT, REPO, ANNOUNCE_LABEL, LABEL_ADDED, and event-specific vars.
"""
import html
import os
import re

# Release-notes bodies are typically 1–3 KB; Telegram's hard cap is 4096 chars
# (raw text incl. HTML tags). 3500 leaves ample room for the header + footer
# links while fitting a full release section without truncation.
MAX_BODY = 3500
HEADER_EMOJI = "🛡️"  # release header icon — change here (or set to "" for none)


def esc(s: str) -> str:
    return html.escape(s or "", quote=True)


def _inline(s: str) -> str:
    """Escape one line, then map inline markdown to the Telegram HTML subset:
    links, `code`, **bold**, *italic*. Order matters — links first so a URL
    can't be chewed by the emphasis passes."""
    s = esc(s)
    # [text](url): keep a real link only for absolute URLs — a repo-relative
    # href (docs/…, CHANGELOG.md) can't resolve in Telegram, so drop it to text.
    s = re.sub(
        r"\[([^\]]+)\]\(([^)]+)\)",
        lambda m: (f'<a href="{m.group(2)}">{m.group(1)}</a>'
                   if re.match(r"^(https?|tg|mailto):", m.group(2)) else m.group(1)),
        s,
    )
    s = re.sub(r"`([^`]+)`", r"<code>\1</code>", s)                    # `code`
    s = re.sub(r"\*\*([^*]+)\*\*", r"<b>\1</b>", s)                    # **bold**
    s = re.sub(r"(?<![\*\w])\*([^*\n]+)\*(?!\*)", r"<i>\1</i>", s)     # *italic*
    return s


def to_html(md: str) -> str:
    """Convert a GitHub-flavoured release body into Telegram HTML: headings ->
    bold, bullets -> •, fenced code -> <pre>, tables -> compact ' · ' lines,
    blockquotes -> <blockquote>, plus inline formatting. Telegram HTML has no
    tables/lists/headings, so those are flattened."""
    out, code, in_code = [], [], False
    for line in (md or "").split("\n"):
        if re.match(r"^\s*```", line):
            if in_code:
                out.append("<pre>" + esc("\n".join(code)) + "</pre>")
                code, in_code = [], False
            else:
                in_code = True
            continue
        if in_code:
            code.append(line)
            continue
        if re.match(r"^\s*([-*_])(\s*\1){2,}\s*$", line):   # --- horizontal rule
            continue
        m = re.match(r"^\s*#{1,6}\s+(.*)$", line)           # heading -> bold
        if m:
            out += ["", "<b>" + _inline(m.group(1)) + "</b>"]
            continue
        if re.match(r"^\s*\|.*\|\s*$", line):               # table row
            if re.match(r"^\s*\|[\s:|-]+\|\s*$", line):     # |---| separator
                continue
            cells = [c.strip() for c in line.strip().strip("|").split("|")]
            out.append("• " + " · ".join(_inline(c) for c in cells if c))
            continue
        m = re.match(r"^\s*[-*]\s+(.*)$", line)             # bullet -> •
        if m:
            out.append("• " + _inline(m.group(1)))
            continue
        m = re.match(r"^\s*>\s?(.*)$", line)                # blockquote
        if m:
            out.append("<blockquote>" + _inline(m.group(1)) + "</blockquote>")
            continue
        out.append(_inline(line))
    if in_code and code:
        out.append("<pre>" + esc("\n".join(code)) + "</pre>")
    return re.sub(r"\n{3,}", "\n\n", "\n".join(out)).strip()


def trim(s: str, limit: int = MAX_BODY) -> str:
    s = s.strip()
    if len(s) <= limit:
        return s
    cut = s[:limit]
    # Prefer to end on a newline, but only if one is near the end — otherwise a
    # long single-line bullet would collapse the whole body back to its header.
    nl = cut.rfind("\n")
    if nl >= limit - 300:
        cut = cut[:nl]
    return cut.rstrip() + "\n…"


def link(url: str, text: str) -> str:
    return f'<a href="{esc(url)}">{esc(text)}</a>'


def release() -> str:
    name = os.environ.get("REL_NAME") or os.environ.get("REL_TAG") or "New release"
    url = os.environ.get("REL_URL", "")
    head = f"{HEADER_EMOJI} " if HEADER_EMOJI else ""
    # Both repos post to the same channel — prefix the product so a release is
    # unambiguous. PRODUCT env wins; else the repo short-name. Skip if the
    # release name already leads with it (avoids "MoaV MoaV v1.9.1").
    product = os.environ.get("PRODUCT") or os.environ.get("REPO", "").split("/")[-1]
    label = name if (product and name.lower().startswith(product.lower())) else f"{product} {name}".strip()

    header = f"{head}<b>{esc(label)}</b> is out"
    footer = "\n".join([
        "<b>Install</b>",
        "<pre>curl -fsSL moav.sh/client-install.sh | bash</pre>",
        "",
        f"📦 {link(url, 'Release notes')}   ·   🌐 {link('https://moav.sh', 'moav.sh')}"
        f"   ·   📚 {link('https://moav.sh/docs/client/', 'Docs')}",
    ])
    # Budget the notes so header + body + footer fit Telegram's 4096-char cap.
    # Trim the raw markdown (÷1.3 to leave room for tag expansion) THEN convert,
    # so the emitted HTML is always well-formed (Telegram 400s on unbalanced tags).
    room = 4096 - len(header) - len(footer) - 12
    body = to_html(trim(os.environ.get("REL_BODY", ""), int(room / 1.3)))
    parts = [header] + (["", body] if body else []) + ["", footer]
    return "\n".join(parts)


def issue() -> str:
    announce = os.environ.get("ANNOUNCE_LABEL", "announce")
    if os.environ.get("LABEL_ADDED", "") != announce:
        return ""  # not an announce label -> skip
    repo = os.environ.get("REPO", "")
    num = os.environ.get("ISSUE_NUM", "")
    title = os.environ.get("ISSUE_TITLE", "")
    url = os.environ.get("ISSUE_URL", "")
    return (f"📣 <b>{esc(repo)} #{esc(num)}</b>\n{esc(title)}\n\n"
            f"🔗 {link(url, 'View on GitHub')}")


def main() -> None:
    event = os.environ.get("EVENT", "")
    if event == "release":
        msg = release()
    elif event == "issues":
        msg = issue()
    elif event == "workflow_dispatch":
        msg = esc(os.environ.get("DISPATCH_TEXT", "MoaV Telegram notifier test ✅"))
    else:
        msg = ""
    if msg:
        print(msg)


if __name__ == "__main__":
    main()
