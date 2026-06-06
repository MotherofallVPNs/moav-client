import { useEffect, useState } from "react";
import { theme } from "../theme";
import { API_BASE } from "../apiBase";

type Kind = "tcp" | "dns" | "trace";

interface Endpoint {
  ID: string;
  Name: string;
  Protocol: string;
  Config?: Record<string, string>;
}

interface Result {
  kind: Kind;
  raw: any;
}

interface Props {
  refreshTick?: number;
}

export default function Diagnostics({ refreshTick }: Props) {
  const [kind, setKind] = useState<Kind>("tcp");
  const [target, setTarget] = useState("1.1.1.1:443");
  const [via, setVia] = useState("");
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<Result | null>(null);
  const [endpoints, setEndpoints] = useState<Endpoint[]>([]);

  useEffect(() => {
    fetch(`${API_BASE}/api/endpoints`)
      .then((r) => r.json())
      .then((d) => setEndpoints((d.endpoints ?? []) as Endpoint[]))
      .catch(() => {});
  }, [refreshTick]);

  const run = async () => {
    setBusy(true);
    setResult(null);
    try {
      const qs = new URLSearchParams({ type: kind, target });
      if (via && kind === "tcp") qs.set("via", via);
      const r = await fetch(`${API_BASE}/api/diag?${qs.toString()}`);
      const raw = await r.json();
      setResult({ kind, raw });
    } catch (e) {
      setResult({ kind, raw: { ok: false, error: (e as Error).message } });
    } finally {
      setBusy(false);
    }
  };

  const dialableEndpoints = endpoints.filter((ep) => ep.Config?.socks5_addr);

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "1rem" }}>
      <p style={{ margin: 0, color: theme.textDim, fontSize: "0.8rem" }}>
        Run a connectivity check from proxy-core itself — useful for
        debugging "does my home router actually reach this host" or "is
        this moav endpoint actually able to reach api.example.com".
        <br />
        <span style={{ color: theme.yellow }}>Heads-up:</span> most moav
        tunnels (WireGuard, AmneziaWG, sidecars) are IPv4-only. Dialing an
        IPv6-only host (e.g. <code>wimi-api-v6.whatismyip.com</code>) via
        them surfaces "general SOCKS server failure" / "network
        unreachable" — that's the tunnel rejecting v6, not a bug.
      </p>

      <div
        style={{
          display: "grid",
          gridTemplateColumns: "auto 1fr",
          gap: "0.5rem 0.75rem",
          alignItems: "center",
          background: theme.surface2,
          padding: "0.75rem",
          borderRadius: 6,
          border: `1px solid ${theme.border}`,
        }}
      >
        <label style={lbl}>check type</label>
        <div style={{ display: "flex", gap: "0.5rem" }}>
          {(["tcp", "dns", "trace"] as Kind[]).map((k) => (
            <button
              key={k}
              onClick={() => setKind(k)}
              style={chip(kind === k ? theme.blue : theme.textDim, kind === k)}
            >
              {k}
            </button>
          ))}
        </div>

        <label style={lbl}>target</label>
        <input
          type="text"
          value={target}
          onChange={(e) => setTarget(e.target.value)}
          placeholder={kind === "dns" ? "example.com" : "host:port"}
          style={input}
        />

        {kind === "tcp" && dialableEndpoints.length > 0 && (
          <>
            <label style={lbl}>via endpoint</label>
            <select value={via} onChange={(e) => setVia(e.target.value)} style={input}>
              <option value="">(direct from proxy-core)</option>
              {dialableEndpoints.map((ep) => (
                <option key={ep.ID} value={ep.ID}>
                  {ep.Name || ep.ID} · {ep.Protocol}
                </option>
              ))}
            </select>
          </>
        )}

        <span />
        <button
          onClick={run}
          disabled={busy}
          style={{
            padding: "0.4rem 0.9rem",
            background: theme.blue,
            color: theme.bg,
            border: "none",
            borderRadius: 4,
            cursor: busy ? "wait" : "pointer",
            fontFamily: theme.mono,
            fontSize: "0.72rem",
            fontWeight: 600,
            textTransform: "uppercase",
            letterSpacing: "0.04em",
            justifySelf: "start",
          }}
        >
          {busy ? "running…" : "run check"}
        </button>
      </div>

      {result && (
        <pre
          style={{
            background: theme.bg,
            border: `1px solid ${theme.border}`,
            borderRadius: 6,
            padding: "0.85rem",
            color: result.raw.ok === false ? theme.red : theme.text,
            fontFamily: theme.mono,
            fontSize: "0.78rem",
            overflowX: "auto",
            margin: 0,
            whiteSpace: "pre-wrap",
          }}
        >
          {prettyResult(result)}
        </pre>
      )}
    </div>
  );
}

function prettyResult(r: Result): string {
  const x = r.raw;
  if (r.kind === "trace" && Array.isArray(x.hops)) {
    const head = `traceroute ${x.target} via TCP-TTL fallback`;
    const rows = x.hops
      .map((h: any) =>
        `  ttl=${String(h.ttl).padStart(2, " ")} rtt=${String(h.rtt_ms).padStart(5, " ")}ms ` +
        (h.peer ? h.peer : `error: ${h.error ?? "?"}`)
      )
      .join("\n");
    return head + "\n" + rows;
  }
  if (r.kind === "trace" && typeof x.output === "string") {
    return `traceroute ${x.target} (${x.binary})\n${x.output}`;
  }
  return JSON.stringify(x, null, 2);
}

const lbl: React.CSSProperties = { fontSize: "0.72rem", color: theme.textDim, fontFamily: theme.mono };
const input: React.CSSProperties = {
  padding: "0.4rem 0.55rem",
  borderRadius: 4,
  fontSize: "0.85rem",
  fontFamily: theme.mono,
  // Without these a text input keeps its intrinsic width and overflows the
  // grid's 1fr column on narrow (mobile) screens.
  width: "100%",
  minWidth: 0,
  boxSizing: "border-box",
};
const chip = (color: string, active: boolean): React.CSSProperties => ({
  padding: "0.35rem 0.75rem",
  background: active ? color + "22" : "transparent",
  border: `1px solid ${color}55`,
  borderRadius: 4,
  cursor: "pointer",
  color,
  fontFamily: theme.mono,
  fontSize: "0.72rem",
  fontWeight: 600,
  textTransform: "uppercase",
});
