import { useEffect, useRef, useState } from "react";

const API_BASE = (import.meta.env.VITE_API_URL as string | undefined) ?? "http://localhost:8088";

interface StatRow {
  id: string;
  name: string;
  protocol: string;
  address: string;
  status: string;
  latency_ms: number;
  dials: number;
  dial_errors: number;
  failovers: number;
  bytes_up: number;
  bytes_down: number;
  last_dial_unix: number;
  last_error_unix: number;
  last_error?: string;
}

interface StatsResp {
  strategy: string;
  rows: StatRow[];
}

const td: React.CSSProperties = { padding: "0.5rem 0.75rem", fontSize: "0.85rem" };
const th: React.CSSProperties = {
  ...td,
  textAlign: "left",
  fontWeight: 500,
  color: "#475569",
  background: "#f8fafc",
};

function fmtBytes(n: number): string {
  if (!n) return "—";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let i = 0;
  let v = n;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(v >= 10 || i === 0 ? 0 : 1)} ${units[i]}`;
}

function timeAgo(unix: number): string {
  if (!unix) return "—";
  const diff = Math.max(0, Math.floor(Date.now() / 1000 - unix));
  if (diff < 60) return `${diff}s ago`;
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
  return `${Math.floor(diff / 86400)}d ago`;
}

// In-memory ring buffer of throughput samples for the inline sparkline.
const HISTORY_LEN = 30;
type HistoryMap = Record<string, { up: number; down: number }[]>;

function Sparkline({ samples, color }: { samples: number[]; color: string }) {
  if (samples.length < 2) return <span style={{ color: "#94a3b8" }}>—</span>;
  const max = Math.max(...samples, 1);
  const w = 80;
  const h = 24;
  const pts = samples.map((s, i) => {
    const x = (i / (samples.length - 1)) * w;
    const y = h - (s / max) * h;
    return `${x.toFixed(1)},${y.toFixed(1)}`;
  });
  return (
    <svg width={w} height={h} style={{ verticalAlign: "middle" }}>
      <polyline points={pts.join(" ")} fill="none" stroke={color} strokeWidth={1.5} />
    </svg>
  );
}

export default function Analytics() {
  const [stats, setStats] = useState<StatsResp>({ strategy: "", rows: [] });
  const historyRef = useRef<HistoryMap>({});
  const prevRef = useRef<Record<string, { up: number; down: number }>>({});
  const [tick, setTick] = useState(0);

  useEffect(() => {
    let cancelled = false;
    const fetchOnce = async () => {
      try {
        const r = await fetch(`${API_BASE}/api/stats`);
        const data: StatsResp = await r.json();
        if (cancelled) return;
        const prev = prevRef.current;
        const next: Record<string, { up: number; down: number }> = {};
        for (const row of data.rows) {
          const prevRow = prev[row.id] || { up: 0, down: 0 };
          const deltaUp = Math.max(0, row.bytes_up - prevRow.up);
          const deltaDown = Math.max(0, row.bytes_down - prevRow.down);
          next[row.id] = { up: row.bytes_up, down: row.bytes_down };
          const hist = historyRef.current[row.id] || [];
          hist.push({ up: deltaUp, down: deltaDown });
          while (hist.length > HISTORY_LEN) hist.shift();
          historyRef.current[row.id] = hist;
        }
        prevRef.current = next;
        setStats(data);
        setTick((t) => t + 1);
      } catch {
        // ignore network errors; we just keep the last snapshot.
      }
    };
    fetchOnce();
    const id = setInterval(fetchOnce, 2000);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, []);

  const totalDials = stats.rows.reduce((a, r) => a + r.dials, 0);
  const totalErrors = stats.rows.reduce((a, r) => a + r.dial_errors, 0);
  const totalBytesUp = stats.rows.reduce((a, r) => a + r.bytes_up, 0);
  const totalBytesDown = stats.rows.reduce((a, r) => a + r.bytes_down, 0);

  return (
    <div>
      <div
        style={{
          display: "grid",
          gridTemplateColumns: "repeat(auto-fit, minmax(140px, 1fr))",
          gap: "0.75rem",
          marginBottom: "1.25rem",
        }}
      >
        <Card label="Strategy" value={stats.strategy || "—"} />
        <Card label="Total dials" value={String(totalDials)} />
        <Card label="Dial errors" value={String(totalErrors)} accent={totalErrors > 0 ? "#dc2626" : undefined} />
        <Card label="Upload" value={fmtBytes(totalBytesUp)} />
        <Card label="Download" value={fmtBytes(totalBytesDown)} />
      </div>

      <table style={{ width: "100%", borderCollapse: "collapse" }}>
        <thead>
          <tr>
            {["Endpoint", "Dials", "Errors", "Failovers", "Up", "Down", "Throughput (2s)", "Last dial", "Last error"].map(
              (h) => (
                <th key={h} style={th}>
                  {h}
                </th>
              )
            )}
          </tr>
        </thead>
        <tbody>
          {stats.rows.length === 0 ? (
            <tr>
              <td colSpan={9} style={{ ...td, color: "#94a3b8" }}>
                No stats yet — send some traffic through the SOCKS5 listener.
              </td>
            </tr>
          ) : (
            stats.rows
              .sort((a, b) => (b.bytes_up + b.bytes_down) - (a.bytes_up + a.bytes_down) || b.dials - a.dials)
              .map((row) => {
                const hist = historyRef.current[row.id] || [];
                const upSeries = hist.map((h) => h.up);
                const downSeries = hist.map((h) => h.down);
                return (
                  <tr key={row.id + tick.toString()} style={{ borderTop: "1px solid #e2e8f0" }}>
                    <td style={td}>
                      <div>{row.name || row.id}</div>
                      <div style={{ color: "#94a3b8", fontSize: "0.72rem" }}>
                        {row.protocol} · {row.address}
                      </div>
                    </td>
                    <td style={td}>{row.dials}</td>
                    <td style={{ ...td, color: row.dial_errors > 0 ? "#dc2626" : "#475569" }}>{row.dial_errors}</td>
                    <td style={td}>{row.failovers}</td>
                    <td style={td}>{fmtBytes(row.bytes_up)}</td>
                    <td style={td}>{fmtBytes(row.bytes_down)}</td>
                    <td style={td}>
                      <Sparkline samples={upSeries} color="#3b82f6" />
                      <Sparkline samples={downSeries} color="#22c55e" />
                    </td>
                    <td style={{ ...td, color: "#64748b" }}>{timeAgo(row.last_dial_unix)}</td>
                    <td style={{ ...td, color: "#dc2626", maxWidth: 220, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }} title={row.last_error}>
                      {row.last_error || "—"}
                    </td>
                  </tr>
                );
              })
          )}
        </tbody>
      </table>
    </div>
  );
}

function Card({ label, value, accent }: { label: string; value: string; accent?: string }) {
  return (
    <div
      style={{
        border: "1px solid #e2e8f0",
        borderRadius: 8,
        padding: "0.6rem 0.75rem",
        background: "#fff",
      }}
    >
      <div style={{ color: "#94a3b8", fontSize: "0.72rem", textTransform: "uppercase", letterSpacing: 0.5 }}>
        {label}
      </div>
      <div style={{ marginTop: 4, fontSize: "1.15rem", fontWeight: 600, color: accent || "#0f172a" }}>{value}</div>
    </div>
  );
}
