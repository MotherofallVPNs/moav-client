import { useEffect, useMemo, useRef, useState } from "react";
import { theme, statusColor } from "../theme";

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

const td: React.CSSProperties = { padding: "0.5rem 0.65rem", fontSize: "0.78rem", verticalAlign: "middle" };
const th: React.CSSProperties = {
  ...td,
  textAlign: "left",
  fontWeight: 500,
  color: theme.textDim,
  background: theme.surface2,
  fontFamily: theme.mono,
  fontSize: "0.7rem",
  letterSpacing: "0.04em",
  textTransform: "uppercase" as const,
  borderBottom: `1px solid ${theme.border}`,
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
  if (diff < 60) return `${diff}s`;
  if (diff < 3600) return `${Math.floor(diff / 60)}m`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}h`;
  return `${Math.floor(diff / 86400)}d`;
}

const HISTORY_LEN = 60; // 2-min window @ 2s sample
type HistoryEntry = { up: number; down: number };
type History = HistoryEntry[];

function colorForProtocol(p: string): string {
  // Stable per-protocol hue using a small static palette.
  const palette = [theme.green, theme.blue, theme.yellow, theme.orange, theme.red, "#a78bfa", "#34d399"];
  let h = 0;
  for (const ch of p) h = (h * 31 + ch.charCodeAt(0)) >>> 0;
  return palette[h % palette.length];
}

interface AreaChartProps {
  samples: number[];
  color: string;
  width?: number;
  height?: number;
}
function AreaChart({ samples, color, width = 220, height = 50 }: AreaChartProps) {
  if (samples.length < 2) {
    return (
      <svg width={width} height={height} style={{ display: "block" }}>
        <text x={width / 2} y={height / 2} textAnchor="middle" fontSize={10} fill={theme.textDim}>
          waiting for samples…
        </text>
      </svg>
    );
  }
  const max = Math.max(...samples, 1);
  const pts: string[] = [];
  samples.forEach((s, i) => {
    const x = (i / (samples.length - 1)) * width;
    const y = height - (s / max) * (height - 4) - 2;
    pts.push(`${x.toFixed(1)},${y.toFixed(1)}`);
  });
  const fillPath = `M 0,${height} L ${pts.join(" L ")} L ${width},${height} Z`;
  const linePath = `M ${pts.join(" L ")}`;
  return (
    <svg width={width} height={height} style={{ display: "block" }}>
      <path d={fillPath} fill={color} fillOpacity={0.18} />
      <path d={linePath} fill="none" stroke={color} strokeWidth={1.5} />
    </svg>
  );
}

interface Props {
  refreshTick?: number;
}

export default function Analytics({ refreshTick }: Props) {
  const [stats, setStats] = useState<StatsResp>({ strategy: "", rows: [] });
  // Per-endpoint history of delta-per-sample (so we can graph throughput).
  const historyRef = useRef<Record<string, History>>({});
  const prevRef = useRef<Record<string, { up: number; down: number }>>({});

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
          const p = prev[row.id] || { up: 0, down: 0 };
          const dU = Math.max(0, row.bytes_up - p.up);
          const dD = Math.max(0, row.bytes_down - p.down);
          next[row.id] = { up: row.bytes_up, down: row.bytes_down };
          const hist = historyRef.current[row.id] || [];
          hist.push({ up: dU, down: dD });
          while (hist.length > HISTORY_LEN) hist.shift();
          historyRef.current[row.id] = hist;
        }
        prevRef.current = next;
        setStats(data);
      } catch {
        // ignore
      }
    };
    fetchOnce();
    const id = setInterval(fetchOnce, 2000);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, [refreshTick]);

  // Aggregate by protocol.
  const byProtocol = useMemo(() => {
    type Bucket = {
      protocol: string;
      dials: number;
      errors: number;
      bytesUp: number;
      bytesDown: number;
      history: HistoryEntry[];
    };
    const map = new Map<string, Bucket>();
    for (const row of stats.rows) {
      let b = map.get(row.protocol);
      if (!b) {
        b = {
          protocol: row.protocol,
          dials: 0,
          errors: 0,
          bytesUp: 0,
          bytesDown: 0,
          history: Array.from({ length: HISTORY_LEN }, () => ({ up: 0, down: 0 })),
        };
        map.set(row.protocol, b);
      }
      b.dials += row.dials;
      b.errors += row.dial_errors;
      b.bytesUp += row.bytes_up;
      b.bytesDown += row.bytes_down;
      const hist = historyRef.current[row.id] || [];
      // Add this endpoint's history into the protocol bucket (align by tail).
      for (let i = 0; i < HISTORY_LEN; i++) {
        const j = hist.length - HISTORY_LEN + i;
        if (j >= 0 && j < hist.length) {
          b.history[i].up += hist[j].up;
          b.history[i].down += hist[j].down;
        }
      }
    }
    return Array.from(map.values()).sort((a, b) => b.bytesUp + b.bytesDown - (a.bytesUp + a.bytesDown));
  }, [stats]);

  const totalDials = stats.rows.reduce((a, r) => a + r.dials, 0);
  const totalErrors = stats.rows.reduce((a, r) => a + r.dial_errors, 0);
  const totalBytesUp = stats.rows.reduce((a, r) => a + r.bytes_up, 0);
  const totalBytesDown = stats.rows.reduce((a, r) => a + r.bytes_down, 0);

  // Stacked area for ALL protocols, last HISTORY_LEN samples.
  const totalSeries = useMemo(() => {
    const out: { up: number; down: number; perProtocol: { color: string; up: number; down: number; label: string }[] }[] = [];
    for (let i = 0; i < HISTORY_LEN; i++) {
      const entry: (typeof out)[number] = { up: 0, down: 0, perProtocol: [] };
      for (const b of byProtocol) {
        const h = b.history[i];
        entry.up += h.up;
        entry.down += h.down;
        entry.perProtocol.push({ color: colorForProtocol(b.protocol), up: h.up, down: h.down, label: b.protocol });
      }
      out.push(entry);
    }
    return out;
  }, [byProtocol]);

  return (
    <div>
      {/* KPI cards */}
      <div
        style={{
          display: "grid",
          gridTemplateColumns: "repeat(auto-fit, minmax(150px, 1fr))",
          gap: "0.75rem",
          marginBottom: "1.25rem",
        }}
      >
        <Card label="Strategy" value={stats.strategy || "—"} color={theme.blue} />
        <Card label="Total dials" value={String(totalDials)} />
        <Card label="Dial errors" value={String(totalErrors)} color={totalErrors > 0 ? theme.red : undefined} />
        <Card label="Upload" value={fmtBytes(totalBytesUp)} color={theme.green} />
        <Card label="Download" value={fmtBytes(totalBytesDown)} color={theme.blue} />
      </div>

      {/* Stacked overall chart */}
      <Section title="Throughput by protocol (rolling 2-min window @ 2s)">
        <StackedAreaChart series={totalSeries} byProtocol={byProtocol} />
      </Section>

      {/* Per-protocol cards */}
      <Section title="Per-protocol traffic">
        <div
          style={{
            display: "grid",
            gridTemplateColumns: "repeat(auto-fit, minmax(260px, 1fr))",
            gap: "0.75rem",
          }}
        >
          {byProtocol.length === 0 ? (
            <div style={{ color: theme.textDim, fontSize: "0.8rem" }}>
              No traffic yet — send a request through the SOCKS5 listener.
            </div>
          ) : (
            byProtocol.map((b) => {
              const upSeries = b.history.map((h) => h.up);
              const downSeries = b.history.map((h) => h.down);
              const color = colorForProtocol(b.protocol);
              return (
                <div
                  key={b.protocol}
                  style={{
                    background: theme.surface2,
                    border: `1px solid ${theme.border}`,
                    borderRadius: 6,
                    padding: "0.75rem",
                  }}
                >
                  <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                    <div
                      style={{
                        fontFamily: theme.mono,
                        fontSize: "0.78rem",
                        color,
                        fontWeight: 600,
                      }}
                    >
                      {b.protocol}
                    </div>
                    <div style={{ fontSize: "0.7rem", color: theme.textDim, fontFamily: theme.mono }}>
                      {b.dials} dials
                      {b.errors > 0 && <span style={{ color: theme.red }}> · {b.errors} err</span>}
                    </div>
                  </div>
                  <div style={{ marginTop: "0.4rem", display: "flex", justifyContent: "space-between" }}>
                    <Stat label="↑" value={fmtBytes(b.bytesUp)} color={theme.green} />
                    <Stat label="↓" value={fmtBytes(b.bytesDown)} color={theme.blue} />
                  </div>
                  <div style={{ marginTop: "0.4rem" }}>
                    <AreaChart samples={upSeries} color={theme.green} width={232} height={40} />
                    <AreaChart samples={downSeries} color={theme.blue} width={232} height={40} />
                  </div>
                </div>
              );
            })
          )}
        </div>
      </Section>

      {/* Per-endpoint table */}
      <Section title="Per-endpoint">
        <table style={{ width: "100%", borderCollapse: "collapse" }}>
          <thead>
            <tr>
              {["Endpoint", "Dials", "Err", "Failovers", "↑", "↓", "Last dial", "Last error"].map((h) => (
                <th key={h} style={th}>
                  {h}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {stats.rows.length === 0 ? (
              <tr>
                <td colSpan={8} style={{ ...td, color: theme.textDim }}>
                  No stats yet.
                </td>
              </tr>
            ) : (
              stats.rows
                .sort((a, b) => (b.bytes_up + b.bytes_down - (a.bytes_up + a.bytes_down)) || b.dials - a.dials)
                .map((row) => (
                  <tr key={row.id} style={{ borderTop: `1px solid ${theme.border}` }}>
                    <td style={td}>
                      <div>{row.name || row.id}</div>
                      <div style={{ color: theme.textDim, fontSize: "0.68rem", fontFamily: theme.mono }}>
                        {row.protocol} · {row.address}{" "}
                        <span style={{ color: statusColor(row.status) }}>● {row.status}</span>
                      </div>
                    </td>
                    <td style={{ ...td, fontFamily: theme.mono }}>{row.dials}</td>
                    <td
                      style={{
                        ...td,
                        color: row.dial_errors > 0 ? theme.red : theme.text,
                        fontFamily: theme.mono,
                      }}
                    >
                      {row.dial_errors}
                    </td>
                    <td style={{ ...td, fontFamily: theme.mono }}>{row.failovers}</td>
                    <td style={{ ...td, fontFamily: theme.mono, color: theme.green }}>{fmtBytes(row.bytes_up)}</td>
                    <td style={{ ...td, fontFamily: theme.mono, color: theme.blue }}>{fmtBytes(row.bytes_down)}</td>
                    <td style={{ ...td, color: theme.textDim, fontFamily: theme.mono }}>{timeAgo(row.last_dial_unix)}</td>
                    <td
                      style={{
                        ...td,
                        color: theme.red,
                        maxWidth: 240,
                        overflow: "hidden",
                        textOverflow: "ellipsis",
                        whiteSpace: "nowrap",
                        fontFamily: theme.mono,
                        fontSize: "0.68rem",
                      }}
                      title={row.last_error}
                    >
                      {row.last_error || "—"}
                    </td>
                  </tr>
                ))
            )}
          </tbody>
        </table>
      </Section>
    </div>
  );
}

function Card({ label, value, color }: { label: string; value: string; color?: string }) {
  return (
    <div
      style={{
        background: theme.surface2,
        border: `1px solid ${theme.border}`,
        borderRadius: 6,
        padding: "0.6rem 0.75rem",
      }}
    >
      <div
        style={{
          color: theme.textDim,
          fontSize: "0.65rem",
          textTransform: "uppercase",
          letterSpacing: 0.5,
          fontFamily: theme.mono,
        }}
      >
        {label}
      </div>
      <div
        style={{
          marginTop: 4,
          fontSize: "1.05rem",
          fontWeight: 600,
          color: color ?? theme.text,
          fontFamily: theme.mono,
        }}
      >
        {value}
      </div>
    </div>
  );
}

function Stat({ label, value, color }: { label: string; value: string; color: string }) {
  return (
    <div style={{ fontFamily: theme.mono, fontSize: "0.78rem" }}>
      <span style={{ color }}>{label}</span> <span style={{ color: theme.text }}>{value}</span>
    </div>
  );
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div style={{ marginBottom: "1.5rem" }}>
      <div
        style={{
          fontFamily: theme.mono,
          fontSize: "0.72rem",
          color: theme.textDim,
          textTransform: "uppercase",
          letterSpacing: "0.05em",
          marginBottom: "0.5rem",
        }}
      >
        {title}
      </div>
      {children}
    </div>
  );
}

interface StackedProps {
  series: { up: number; down: number; perProtocol: { color: string; up: number; down: number; label: string }[] }[];
  byProtocol: { protocol: string }[];
}
function StackedAreaChart({ series, byProtocol }: StackedProps) {
  const width = 900;
  const height = 120;
  const max = Math.max(1, ...series.map((s) => s.up + s.down));

  if (byProtocol.length === 0) {
    return (
      <div
        style={{
          height,
          background: theme.surface2,
          border: `1px solid ${theme.border}`,
          borderRadius: 6,
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          color: theme.textDim,
          fontSize: "0.78rem",
        }}
      >
        no traffic yet
      </div>
    );
  }

  const protocols = byProtocol.map((b) => b.protocol);

  return (
    <div
      style={{
        background: theme.surface2,
        border: `1px solid ${theme.border}`,
        borderRadius: 6,
        padding: "0.75rem",
      }}
    >
      <svg viewBox={`0 0 ${width} ${height}`} preserveAspectRatio="none" style={{ width: "100%", height }}>
        {/* Build stacked layers — one per protocol */}
        {protocols.map((p, pi) => {
          const color = colorForProtocol(p);
          const points: string[] = [];
          const baseTop: number[] = [];

          // bottom line of this layer = sum of all PRIOR layers (up+down)
          for (let i = 0; i < series.length; i++) {
            let base = 0;
            for (let q = 0; q < pi; q++) {
              const prior = series[i].perProtocol[q];
              if (prior) base += prior.up + prior.down;
            }
            const me = series[i].perProtocol[pi];
            const meTotal = (me?.up || 0) + (me?.down || 0);
            const top = base + meTotal;
            const x = (i / (series.length - 1)) * width;
            const yTop = height - (top / max) * height;
            points.push(`${x.toFixed(1)},${yTop.toFixed(1)}`);
            baseTop.push(height - (base / max) * height);
          }
          // close polygon
          const reversed = baseTop
            .map((y, i) => `${((series.length - 1 - i) / (series.length - 1)) * width},${y}`)
            .reverse();
          return (
            <polygon
              key={p}
              points={[...points, ...reversed].join(" ")}
              fill={color}
              fillOpacity={0.35}
              stroke={color}
              strokeWidth={1}
            />
          );
        })}
      </svg>
      <div style={{ display: "flex", flexWrap: "wrap", gap: "0.6rem", marginTop: "0.6rem" }}>
        {protocols.map((p) => (
          <div
            key={p}
            style={{
              display: "inline-flex",
              alignItems: "center",
              gap: 4,
              fontSize: "0.7rem",
              fontFamily: theme.mono,
              color: theme.text,
            }}
          >
            <span
              style={{ width: 10, height: 10, background: colorForProtocol(p), display: "inline-block", borderRadius: 2 }}
            />
            {p}
          </div>
        ))}
      </div>
    </div>
  );
}
