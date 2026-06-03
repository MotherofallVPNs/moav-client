import { useEffect, useRef, useState } from "react";
import { theme, statusColor, statusBg } from "../theme";
import { API_BASE, WS_BASE } from "../apiBase";

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
  const [editingPrio, setEditingPrio] = useState<{ id: string; value: string } | null>(null);
  const [toast, setToast] = useState<{ msg: string; ok: boolean } | null>(null);
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

  // Persistent WS stream.
  useEffect(() => {
    const ws = new WebSocket(`${WS_BASE}/api/ws`);
    wsRef.current = ws;
    ws.onmessage = (ev) => {
      try {
        const data = JSON.parse(ev.data as string);
        if (data.endpoints) apply(data.endpoints as Endpoint[]);
      } catch {
        // ignore
      }
    };
    return () => ws.close();
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
    setEditingPrio(null);
    const n = parseInt(raw, 10);
    if (Number.isNaN(n) || n < 0 || n > 1000) {
      flash("Priority must be 0–1000.", false);
      return;
    }
    if (n === ep.Priority) return;
    patch(ep.ID, { priority: n });
  };

  return (
    <div>
      <table style={{ width: "100%", borderCollapse: "collapse" }}>
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
                  <div>{ep.Name || ep.ID}</div>
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
                      background: statusBg(ep.Status),
                      color: statusColor(ep.Status),
                      border: `1px solid ${statusColor(ep.Status)}44`,
                    }}
                  >
                    {ep.Status || "unknown"}
                  </span>
                </td>
                <td style={td}>
                  {editingPrio?.id === ep.ID ? (
                    <input
                      type="number"
                      autoFocus
                      min={0}
                      max={1000}
                      value={editingPrio.value}
                      onChange={(e) => setEditingPrio({ id: ep.ID, value: e.target.value })}
                      onBlur={() => commitPriority(ep, editingPrio.value)}
                      onKeyDown={(e) => {
                        if (e.key === "Enter") commitPriority(ep, editingPrio.value);
                        if (e.key === "Escape") setEditingPrio(null);
                      }}
                      style={{
                        width: 60,
                        padding: "0.2rem 0.4rem",
                        borderRadius: 4,
                        fontSize: "0.82rem",
                        fontFamily: theme.mono,
                      }}
                    />
                  ) : (
                    <button
                      onClick={() => setEditingPrio({ id: ep.ID, value: String(ep.Priority) })}
                      disabled={pendingId === ep.ID}
                      title="Click to edit"
                      style={{
                        background: "transparent",
                        border: "1px dashed transparent",
                        padding: "0.2rem 0.5rem",
                        borderRadius: 4,
                        cursor: pendingId === ep.ID ? "wait" : "pointer",
                        color: theme.text,
                        fontFamily: theme.mono,
                      }}
                      onMouseEnter={(e) => (e.currentTarget.style.borderColor = theme.border)}
                      onMouseLeave={(e) => (e.currentTarget.style.borderColor = "transparent")}
                    >
                      {ep.Priority}
                    </button>
                  )}
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
        Priority is used by the <code>priority</code> and <code>weighted</code> strategies. Toggling sidecar endpoints
        also stops/starts the docker container if <code>/var/run/docker.sock</code> is mounted.
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
