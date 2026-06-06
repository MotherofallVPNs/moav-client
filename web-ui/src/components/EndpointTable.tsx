import { useEffect, useRef, useState } from "react";
import { theme, statusColor, statusBg } from "../theme";
import { displayEndpointName } from "../display";
import { API_BASE, openWS } from "../apiBase";
import { useIsMobile } from "../useIsMobile";

export interface Endpoint {
  ID: string;
  Name: string;
  Protocol: string;
  Address: string;
  Source?: string;
  LatencyMs: number;
  Status: string;
  Priority: number;
  Enabled: boolean;
  Config?: Record<string, string>;
}


const td: React.CSSProperties = { padding: "0.5rem 0.65rem", verticalAlign: "middle", fontSize: "0.82rem" };
const th: React.CSSProperties = {
  ...td,
  textAlign: "left",
  fontWeight: 500,
  color: theme.textDim,
  background: theme.surface2,
  fontFamily: theme.mono,
  fontSize: "0.72rem",
  letterSpacing: "0.04em",
  textTransform: "uppercase" as const,
  borderBottom: `1px solid ${theme.border}`,
};

interface Props {
  onHealthChange?: (healthy: number, total: number) => void;
  refreshTick?: number;
}

export default function EndpointTable({ onHealthChange, refreshTick }: Props) {
  const [endpoints, setEndpoints] = useState<Endpoint[]>([]);
  const [pendingId, setPendingId] = useState<string | null>(null);
  const [toast, setToast] = useState<{ msg: string; ok: boolean } | null>(null);
  const isMobile = useIsMobile();
  const wsRef = useRef<WebSocket | null>(null);

  const apply = (eps: Endpoint[]) => {
    setEndpoints(eps);
    onHealthChange?.(eps.filter((e) => e.Status === "ok" && e.Enabled).length, eps.length);
  };

  // Initial + on refresh tick.
  useEffect(() => {
    fetch(`${API_BASE}/api/endpoints`)
      .then((r) => r.json())
      .then((data) => apply((data.endpoints ?? []) as Endpoint[]))
      .catch(() => {});
  }, [refreshTick]);

  // Persistent WS stream (ticket-authenticated — see openWS).
  useEffect(() => {
    let closed = false;
    let ws: WebSocket | null = null;
    openWS("/api/ws").then((sock) => {
      if (closed) {
        sock.close();
        return;
      }
      ws = sock;
      wsRef.current = sock;
      sock.onmessage = (ev) => {
        try {
          const data = JSON.parse(ev.data as string);
          if (data.endpoints) apply(data.endpoints as Endpoint[]);
        } catch {
          // ignore
        }
      };
    });
    return () => {
      closed = true;
      ws?.close();
    };
  }, []);

  const flash = (msg: string, ok: boolean) => {
    setToast({ msg, ok });
    setTimeout(() => setToast(null), 2500);
  };

  const patch = async (id: string, body: { enabled?: boolean; priority?: number }) => {
    setPendingId(id);
    try {
      const r = await fetch(`${API_BASE}/api/endpoints/${encodeURIComponent(id)}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });
      if (!r.ok) throw new Error(`${r.status} ${r.statusText}`);
      const data = await r.json();
      setEndpoints((prev) => prev.map((ep) => (ep.ID === id ? { ...ep, ...data.endpoint } : ep)));
      flash("Updated.", true);
    } catch (e) {
      flash(`Update failed: ${(e as Error).message}`, false);
    } finally {
      setPendingId(null);
    }
  };

  const toggleEnabled = (ep: Endpoint) => {
    const next = !ep.Enabled;
    if (ep.Protocol === "sidecar" && !next) {
      const ok = window.confirm(
        `Disable ${ep.Name}?\n\nThis also stops its docker container (if /var/run/docker.sock is mounted) so the protocol is fully off — not just removed from the dial pool.`
      );
      if (!ok) return;
    }
    patch(ep.ID, { enabled: next });
  };

  const commitPriority = (ep: Endpoint, raw: string) => {
    const n = parseInt(raw, 10);
    if (Number.isNaN(n) || n < 0 || n > 10) {
      flash("Priority must be 0–10.", false);
      return;
    }
    if (n === ep.Priority) return;
    patch(ep.ID, { priority: n });
  };

  const statusPill = (ep: Endpoint) => (
    <span
      style={{
        display: "inline-block",
        padding: "0.15rem 0.55rem",
        borderRadius: 12,
        fontSize: "0.65rem",
        fontWeight: 600,
        fontFamily: theme.mono,
        letterSpacing: "0.05em",
        textTransform: "uppercase",
        whiteSpace: "nowrap",
        background: ep.Enabled ? statusBg(ep.Status) : "rgba(110, 118, 129, 0.15)",
        color: ep.Enabled ? statusColor(ep.Status) : theme.textDim,
        border: `1px solid ${ep.Enabled ? statusColor(ep.Status) : theme.textDim}44`,
      }}
    >
      {ep.Enabled ? (ep.Status || "unknown") : "disabled"}
    </span>
  );

  if (isMobile) {
    return (
      <div>
        {endpoints.length === 0 ? (
          <div style={{ color: theme.textDim, fontSize: "0.82rem" }}>No endpoints loaded.</div>
        ) : (
          <div style={{ display: "flex", flexDirection: "column", gap: "0.55rem" }}>
            {endpoints.map((ep) => (
              <div
                key={ep.ID}
                style={{
                  border: `1px solid ${theme.border}`,
                  borderRadius: 8,
                  padding: "0.65rem 0.75rem",
                  background: theme.surface2,
                  opacity: ep.Enabled ? 1 : 0.55,
                }}
              >
                <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", gap: "0.5rem" }}>
                  <div style={{ fontWeight: 600, fontSize: "0.88rem", wordBreak: "break-word" }}>
                    {displayEndpointName(ep.Name, ep.ID, ep.Source)}
                  </div>
                  {statusPill(ep)}
                </div>
                <div style={{ marginTop: 3, fontFamily: theme.mono, fontSize: "0.7rem", color: theme.textDim, wordBreak: "break-all" }}>
                  <span style={{ color: theme.green }}>{ep.Source || "—"}</span>{" · "}
                  <span style={{ color: theme.blue }}>
                    {ep.Protocol === "sidecar" ? (ep.Config?.sidecar_kind ?? "sidecar") : ep.Protocol}
                  </span>{" · "}
                  {ep.Address}
                </div>
                <div style={{ marginTop: 9, display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                  <span style={{ fontFamily: theme.mono, fontSize: "0.78rem", color: theme.text }}>
                    {ep.LatencyMs >= 0 ? `${ep.LatencyMs} ms` : "— ms"}
                  </span>
                  <div style={{ display: "flex", alignItems: "center", gap: "0.85rem" }}>
                    <label style={{ display: "flex", alignItems: "center", gap: 5, fontSize: "0.7rem", color: theme.textDim, fontFamily: theme.mono }}>
                      prio
                      <select
                        value={ep.Priority}
                        disabled={pendingId === ep.ID}
                        onChange={(e) => commitPriority(ep, e.target.value)}
                        style={{ padding: "0.25rem 0.35rem", borderRadius: 4, fontSize: "0.82rem", fontFamily: theme.mono }}
                      >
                        {Array.from({ length: 11 }, (_, i) => (
                          <option key={i} value={i}>{i}</option>
                        ))}
                      </select>
                    </label>
                    <Switch checked={ep.Enabled} onChange={() => toggleEnabled(ep)} disabled={pendingId === ep.ID} />
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}
        {toast && (
          <div style={{ position: "fixed", bottom: 16, left: 16, right: 16, padding: "0.6rem 1rem", background: toast.ok ? theme.green : theme.red, color: theme.bg, borderRadius: 6, fontSize: "0.78rem", fontFamily: theme.mono, fontWeight: 600, textAlign: "center" }}>
            {toast.msg}
          </div>
        )}
      </div>
    );
  }

  return (
    <div>
      <table style={{ width: "100%", minWidth: 620, borderCollapse: "collapse" }}>
        <thead>
          <tr>
            {["Name", "Source", "Protocol", "Address", "Latency", "Status", "Priority", "Enabled"].map((h) => (
              <th key={h} style={th}>
                {h}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {endpoints.length === 0 ? (
            <tr>
              <td colSpan={8} style={{ ...td, color: theme.textDim }}>
                No endpoints loaded.
              </td>
            </tr>
          ) : (
            endpoints.map((ep) => (
              <tr
                key={ep.ID}
                style={{
                  borderTop: `1px solid ${theme.border}`,
                  opacity: ep.Enabled ? 1 : 0.5,
                }}
              >
                <td style={td}>
                  <div>{displayEndpointName(ep.Name, ep.ID, ep.Source)}</div>
                  {ep.Protocol === "sidecar" && (
                    <div style={{ fontSize: "0.65rem", color: theme.textDim, marginTop: 2, fontFamily: theme.mono }}>
                      {ep.Config?.sidecar_kind ?? "?"}
                    </div>
                  )}
                </td>
                <td style={{ ...td, fontFamily: theme.mono, fontSize: "0.72rem", color: theme.green }}>
                  {ep.Source || "—"}
                </td>
                <td style={{ ...td, fontFamily: theme.mono, color: theme.blue }}>{ep.Protocol}</td>
                <td style={{ ...td, fontFamily: theme.mono, color: theme.textDim }}>{ep.Address}</td>
                <td style={{ ...td, fontFamily: theme.mono }}>{ep.LatencyMs >= 0 ? `${ep.LatencyMs}ms` : "—"}</td>
                <td style={td}>
                  <span
                    style={{
                      display: "inline-block",
                      padding: "0.15rem 0.55rem",
                      borderRadius: 12,
                      fontSize: "0.65rem",
                      fontWeight: 600,
                      fontFamily: theme.mono,
                      letterSpacing: "0.05em",
                      textTransform: "uppercase",
                      background: ep.Enabled ? statusBg(ep.Status) : "rgba(110, 118, 129, 0.15)",
                      color: ep.Enabled ? statusColor(ep.Status) : theme.textDim,
                      border: `1px solid ${ep.Enabled ? statusColor(ep.Status) : theme.textDim}44`,
                    }}
                    title={ep.Enabled ? "" : "endpoint is disabled — not probed"}
                  >
                    {ep.Enabled ? (ep.Status || "unknown") : "disabled"}
                  </span>
                </td>
                <td style={td}>
                  <select
                    value={ep.Priority}
                    disabled={pendingId === ep.ID}
                    onChange={(e) => commitPriority(ep, e.target.value)}
                    title="Under priority strategy: LOWER picked first. Under weighted: HIGHER = more traffic. Ignored under latency."
                    style={{
                      padding: "0.2rem 0.4rem",
                      borderRadius: 4,
                      fontSize: "0.82rem",
                      fontFamily: theme.mono,
                      cursor: pendingId === ep.ID ? "wait" : "pointer",
                    }}
                  >
                    {Array.from({ length: 11 }, (_, i) => (
                      <option key={i} value={i}>
                        {i}
                      </option>
                    ))}
                  </select>
                </td>
                <td style={td}>
                  <Switch
                    checked={ep.Enabled}
                    onChange={() => toggleEnabled(ep)}
                    disabled={pendingId === ep.ID}
                  />
                </td>
              </tr>
            ))
          )}
        </tbody>
      </table>

      <div style={{ marginTop: "0.75rem", fontSize: "0.7rem", color: theme.textDim, fontFamily: theme.mono }}>
        Priority: <strong>lower</strong> is picked first under the <code>priority</code> strategy;
        <strong> higher</strong> = more traffic under the <code>weighted</code> strategy; ignored under <code>latency</code>.
        Toggling sidecar endpoints also stops/starts the docker container if <code>/var/run/docker.sock</code> is mounted.
      </div>

      {toast && (
        <div
          style={{
            position: "fixed",
            bottom: 24,
            right: 24,
            padding: "0.6rem 1rem",
            background: toast.ok ? theme.green : theme.red,
            color: theme.bg,
            borderRadius: 4,
            fontSize: "0.78rem",
            fontWeight: 600,
            fontFamily: theme.mono,
            maxWidth: 360,
          }}
        >
          {toast.msg}
        </div>
      )}
    </div>
  );
}

function Switch({
  checked,
  onChange,
  disabled,
}: {
  checked: boolean;
  onChange: () => void;
  disabled?: boolean;
}) {
  return (
    <button
      role="switch"
      aria-checked={checked}
      onClick={onChange}
      disabled={disabled}
      style={{
        width: 36,
        height: 20,
        borderRadius: 999,
        border: "none",
        padding: 2,
        background: checked ? theme.green : theme.border,
        cursor: disabled ? "wait" : "pointer",
        display: "inline-flex",
        alignItems: "center",
        transition: "background 0.15s ease",
        opacity: disabled ? 0.6 : 1,
      }}
    >
      <span
        style={{
          width: 14,
          height: 14,
          borderRadius: "50%",
          background: "#fff",
          transform: checked ? "translateX(16px)" : "translateX(0)",
          transition: "transform 0.15s ease",
        }}
      />
    </button>
  );
}
