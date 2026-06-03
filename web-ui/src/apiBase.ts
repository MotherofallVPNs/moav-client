// Resolve where the dashboard should fetch / WebSocket the API.
//
// At build time we can't know if the user will visit http://localhost:3001
// (loopback) or http://192.168.1.10:3001 (LAN exposure). So we derive the
// API URL from the page's current location and just swap the port to 8088.
// VITE_API_URL still wins if explicitly set (useful for `npm run dev`).
const explicit = import.meta.env.VITE_API_URL as string | undefined;

function fromLocation(): string {
  if (typeof window === "undefined") return "http://localhost:8088";
  const proto = window.location.protocol === "https:" ? "https:" : "http:";
  const host = window.location.hostname || "localhost";
  return `${proto}//${host}:8088`;
}

export const API_BASE = explicit && explicit.length > 0 ? explicit : fromLocation();
export const WS_BASE = API_BASE.replace(/^http/, "ws");
