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
}

const API_BASE = (import.meta.env.VITE_API_URL as string | undefined) ?? "http://localhost:8088";
const WS_BASE = API_BASE.replace(/^http/, "ws");

function statusColor(status: string): string {
  if (status === "ok") return "#16a34a";
  if (status === "timeout" || status === "error") return "#dc2626";
  return "#94a3b8";
}

const td: React.CSSProperties = { padding: "0.5rem 0.75rem" };
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
  const wsRef = useRef<WebSocket | null>(null);

  const apply = (eps: Endpoint[]) => {
    setEndpoints(eps);
    onHealthChange?.(eps.filter((e) => e.Status === "ok").length, eps.length);
  };

  useEffect(() => {
    // Initial load via REST.
    fetch(`${API_BASE}/api/endpoints`)
      .then((r) => r.json())
      .then((data) => apply((data.endpoints ?? []) as Endpoint[]))
      .catch(() => {});

    // Live updates via WebSocket.
    const ws = new WebSocket(`${WS_BASE}/api/ws`);
    wsRef.current = ws;
    ws.onmessage = (ev) => {
      try {
        const data = JSON.parse(ev.data as string);
        if (data.endpoints) apply(data.endpoints as Endpoint[]);
      } catch {
        // ignore malformed frames
      }
    };
    return () => ws.close();
  }, []);

  return (
    <table style={{ width: "100%", borderCollapse: "collapse", fontSize: "0.875rem" }}>
      <thead>
        <tr>
          {["Name", "Protocol", "Address", "Latency", "Status", "Priority", "Enabled"].map((h) => (
            <th key={h} style={th}>{h}</th>
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
            <tr key={ep.ID} style={{ borderTop: "1px solid #e2e8f0" }}>
              <td style={td}>{ep.Name || ep.ID}</td>
              <td style={{ ...td, fontFamily: "monospace" }}>{ep.Protocol}</td>
              <td style={{ ...td, fontFamily: "monospace" }}>{ep.Address}</td>
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
              <td style={td}>{ep.Priority}</td>
              <td style={td}>{ep.Enabled ? "✓" : "—"}</td>
            </tr>
          ))
        )}
      </tbody>
    </table>
  );
}
