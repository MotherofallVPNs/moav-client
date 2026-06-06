// Copy text to the clipboard with a fallback for insecure contexts.
//
// navigator.clipboard is only available in "secure contexts" — HTTPS or
// localhost. The dashboard is typically served over plain HTTP on a LAN IP
// (e.g. http://192.168.1.239:3001), where navigator.clipboard is undefined and
// a `navigator.clipboard?.writeText(...)` call silently does nothing. Fall back
// to a hidden <textarea> + execCommand("copy"), which works over HTTP.
export async function copyText(text: string): Promise<boolean> {
  try {
    if (typeof navigator !== "undefined" && navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch {
    // fall through to the legacy path
  }
  try {
    const ta = document.createElement("textarea");
    ta.value = text;
    // Keep it out of view and unscrollable, but selectable.
    ta.style.position = "fixed";
    ta.style.top = "-1000px";
    ta.style.opacity = "0";
    ta.setAttribute("readonly", "");
    document.body.appendChild(ta);
    ta.select();
    ta.setSelectionRange(0, ta.value.length);
    const ok = document.execCommand("copy");
    document.body.removeChild(ta);
    return ok;
  } catch {
    return false;
  }
}
