import { useEffect, useRef, useState } from "react";

export interface Endpoint {
  ID: string;
  Name: string;
  Protocol: string;
  Address: string;
  LatencyMs: number;
  Status: string;
  Priority: number;
  Enabled: boolean;
  Config?: Record<string, string>;
}

const API_BASE = (import.meta.env.VITE_API_URL as string | undefined) ?? "http://localhost:8088";
const WS_BASE = API_BASE.replace(/^http/, "ws");

function statusColor(status: string): string {
  if (status === "ok") return "#16a34a";
  if (status === "timeout" || status === "error") return "#dc2626";
  return "#94a3b8";
}

const td: React.CSSProperties = { padding: "0.5rem 0.6rem", verticalAlign: "middle" };
const th: React.CSSProperties = {
  ...td,
  textAlign: "left",
  fontWeight: 500,
  color: "#475569",
  background: "#f8fafc",
};

interface Props {
  onHealthChange?: (healthy: number, total: number) => void;
}

export default function EndpointTable({ onHealthChange }: Props) {
  const [endpoints, setEndpoints] = useState<Endpoint[]>([]);
  const [pendingId, setPendingId] = useState<string | null>(null);
  const [editingPrio, setEditingPrio] = useState<{ id: string; value: string } | null>(null);
  const [toast, setToast] = useState<{ msg: string; ok: boolean } | null>(null);
  const wsRef = useRef<WebSocket | null>(null);

  const apply = (eps: Endpoint[]) => {
    setEndpoints(eps);
    onHealthChange?.(eps.filter((e) => e.Status === "ok" && e.Enabled).length, eps.length);
  };

  useEffect(() => {
    fetch(`${API_BASE}/api/endpoints`)
      .then((r) => r.json())
      .then((data) => apply((data.endpoints ?? []) as Endpoint[]))
      .catch(() => {});

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
        `Disable ${ep.Name}?\n\nThis also stops its docker container (if the docker socket is mounted) so the protocol is fully off — not just removed from the dial pool.`
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
      <table style={{ width: "100%", borderCollapse: "collapse", fontSize: "0.875rem" }}>
        <thead>
          <tr>
            {["Name", "Protocol", "Address", "Latency", "Status", "Priority", "Enabled"].map((h) => (
              <th key={h} style={th}>
                {h}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {endpoints.length === 0 ? (
            <tr>
              <td colSpan={7} style={{ ...td, color: "#94a3b8" }}>
                No endpoints loaded.
              </td>
            </tr>
          ) : (
            endpoints.map((ep) => (
              <tr
                key={ep.ID}
                style={{
                  borderTop: "1px solid #e2e8f0",
                  opacity: ep.Enabled ? 1 : 0.55,
                }}
              >
                <td style={td}>
                  {ep.Name || ep.ID}
                  {ep.Protocol === "sidecar" && (
                    <span style={{ marginLeft: 6, fontSize: "0.7rem", color: "#94a3b8" }}>
                      ({ep.Config?.sidecar_kind ?? "?"})
                    </span>
                  )}
                </td>
                <td style={{ ...td, fontFamily: "monospace", fontSize: "0.8rem" }}>{ep.Protocol}</td>
                <td style={{ ...td, fontFamily: "monospace", fontSize: "0.8rem" }}>{ep.Address}</td>
                <td style={td}>{ep.LatencyMs >= 0 ? `${ep.LatencyMs} ms` : "—"}</td>
                <td style={td}>
                  <span
                    style={{
                      display: "inline-block",
                      padding: "0.15rem 0.5rem",
                      borderRadius: 12,
                      fontSize: "0.75rem",
                      fontWeight: 600,
                      background: statusColor(ep.Status) + "22",
                      color: statusColor(ep.Status),
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
                        padding: "0.15rem 0.3rem",
                        border: "1px solid #3b82f6",
                        borderRadius: 4,
                        fontSize: "0.85rem",
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
                        padding: "0.15rem 0.45rem",
                        borderRadius: 4,
                        cursor: pendingId === ep.ID ? "wait" : "pointer",
                        color: "#475569",
                        fontFamily: "monospace",
                      }}
                      onMouseEnter={(e) => (e.currentTarget.style.borderColor = "#cbd5e1")}
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

      <div style={{ marginTop: "0.75rem", fontSize: "0.72rem", color: "#94a3b8" }}>
        Priority is used by the <code>priority</code> and <code>weighted</code> strategies (lower = picked first; weighted treats it as a sampling weight). For
        sidecar endpoints, toggling Enabled also stops/starts the docker container if{" "}
        <code>/var/run/docker.sock</code> is mounted.
      </div>

      {toast && (
        <div
          style={{
            position: "fixed",
            bottom: 24,
            right: 24,
            padding: "0.6rem 1rem",
            background: toast.ok ? "#16a34a" : "#dc2626",
            color: "#fff",
            borderRadius: 6,
            fontSize: "0.85rem",
            boxShadow: "0 4px 14px rgba(0,0,0,0.15)",
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
        width: 38,
        height: 20,
        borderRadius: 999,
        border: "none",
        padding: 2,
        background: checked ? "#22c55e" : "#cbd5e1",
        cursor: disabled ? "wait" : "pointer",
        display: "inline-flex",
        alignItems: "center",
        transition: "background 0.15s ease",
        opacity: disabled ? 0.6 : 1,
      }}
    >
      <span
        style={{
          width: 16,
          height: 16,
          borderRadius: "50%",
          background: "#fff",
          transform: checked ? "translateX(18px)" : "translateX(0)",
          transition: "transform 0.15s ease",
          boxShadow: "0 1px 2px rgba(0,0,0,0.2)",
        }}
      />
    </button>
  );
}
