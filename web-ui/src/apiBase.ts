// Resolve where the dashboard talks to the API.
//
// By default we use the SAME ORIGIN as the page and let the web server
// (nginx in the default stack, Caddy in the https profile) reverse-proxy
// /api/* and /api/ws to proxy-core. Single-origin means optional dashboard
// basic-auth set on that server covers the UI and the API together, and there
// is no separate :8088 to expose or hit with mixed-content/CORS.
//
// VITE_API_URL still wins if explicitly set (useful for `npm run dev` against a
// proxy-core running somewhere else). `npm run dev` otherwise works via the
// vite dev-server proxy (see vite.config.ts), which forwards /api to :8088.
const explicit = import.meta.env.VITE_API_URL as string | undefined;
const hasExplicit = !!(explicit && explicit.length > 0);

function wsSameOrigin(): string {
  if (typeof window === "undefined") return "ws://localhost:8088";
  const wsProto = window.location.protocol === "https:" ? "wss:" : "ws:";
  // window.location.host already includes the port.
  return `${wsProto}//${window.location.host}`;
}

// Empty base = relative URLs (`/api/...`) resolved against the current origin.
export const API_BASE = hasExplicit ? (explicit as string) : "";
export const WS_BASE = hasExplicit ? (explicit as string).replace(/^http/, "ws") : wsSameOrigin();

// openWS opens a WebSocket to `path`, first fetching a short-lived ticket over
// an authenticated request and appending it as ?ticket=. Browsers (iOS Safari
// especially) don't send cached basic-auth credentials on a WS handshake, so
// the ticket is how the WS authenticates. When no dashboard password is set the
// ticket endpoint still returns one and the server ignores it — so this path is
// uniform whether or not auth is enabled.
export async function openWS(path: string): Promise<WebSocket> {
  let url = `${WS_BASE}${path}`;
  try {
    const r = await fetch(`${API_BASE}/api/ws-ticket`);
    if (r.ok) {
      const { ticket } = await r.json();
      if (ticket) url += `${path.includes("?") ? "&" : "?"}ticket=${encodeURIComponent(ticket)}`;
    }
  } catch {
    // No ticket (e.g. auth off / endpoint missing) — open without it.
  }
  return new WebSocket(url);
}
