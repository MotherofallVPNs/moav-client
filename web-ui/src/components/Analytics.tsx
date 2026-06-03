import { useEffect, useMemo, useState } from "react";
import { theme, statusColor } from "../theme";
import { API_BASE } from "../apiBase";
import { displayEndpointName } from "../display";


interface StatRow {
  id: string;
  name: string;
  protocol: string;
  sidecar_kind?: string;
  source?: string;
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

// A sidecar endpoint's protocol field is just "sidecar"; the meaningful
// label is the sidecar_kind ("psiphon", "masterdns", …). resolveLabel
// makes the chart legend + per-protocol cards show the actual transport
// instead of the generic "sidecar" bucket.
function resolveLabel(r: StatRow): string {
  if (r.protocol === "sidecar" && r.sidecar_kind) return r.sidecar_kind;
  return r.protocol;
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

// History lives at module scope so it survives Analytics tab unmounts /
// remounts (otherwise navigating away and back wipes the chart). prev[]
// tracks the absolute byte counters per-endpoint so each tick can compute
// a delta.
const globalHistory: Record<string, History> = {};
const globalPrev: Record<string, { up: number; down: number }> = {};
const seenEndpoint: Record<string, boolean> = {};

// Hand-picked palette: each known protocol / sidecar-kind gets a fixed,
// visually-distinct color. We do NOT hash — hashes routinely collided
// (e.g. "sidecar" and "vless" both landed on red in the old build),
// rendering the stacked chart unreadable.
const PROTOCOL_COLORS: Record<string, string> = {
  // Main protocols (sing-box / xray outbounds).
  vless:        "#58a6ff", // blue
  trojan:       "#d29922", // amber
  ss:           "#db6d28", // orange
  shadowsocks:  "#db6d28",
  hysteria2:    "#3fb950", // green
  vmess:        "#a78bfa", // purple
  tuic:         "#34d399", // teal
  wireguard:    "#f97171", // red-pink
  amneziawg:    "#fda4af", // soft pink
  mtproxy:      "#fb7185", // rose

  // Resolved sidecar kinds (after resolveLabel maps "sidecar" → kind).
  psiphon:      "#a3e635", // lime
  masterdns:    "#2dd4bf", // cyan-teal
  tor:          "#818cf8", // indigo
  trusttunnel:  "#c084fc", // violet
  dnstt:        "#facc15", // yellow
  slipstream:   "#fb923c", // bright orange

  // Catch-all for legacy / unknown.
  sidecar:      "#94a3b8", // slate
  direct:       theme.textDim,
};

const FALLBACK_COLORS = ["#22d3ee", "#fbbf24", "#a78bfa", "#84cc16", "#fb7185", "#38bdf8"];

function colorForProtocol(p: string): string {
  if (PROTOCOL_COLORS[p]) return PROTOCOL_COLORS[p];
  let h = 0;
  for (const ch of p) h = (h * 31 + ch.charCodeAt(0)) >>> 0;
  return FALLBACK_COLORS[h % FALLBACK_COLORS.length];
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
      {/* Baseline grid line at zero */}
      <line x1={0} y1={height - 1} x2={width} y2={height - 1} stroke="#1e2d3d" strokeWidth={1} />
      <path d={fillPath} fill={color} fillOpacity={0.18} />
      <path d={linePath} fill="none" stroke={color} strokeWidth={1.5} />
      {/* Peak label */}
      <text x={width - 4} y={12} textAnchor="end" fontSize={9} fontFamily="monospace" fill="#6e7681">
        peak {fmtBytes(max)}/s
      </text>
    </svg>
  );
}

interface Props {
  refreshTick?: number;
}

export default function Analytics({ refreshTick }: Props) {
  const [stats, setStats] = useState<StatsResp>({ strategy: "", rows: [] });

  useEffect(() => {
    let cancelled = false;
    const fetchOnce = async () => {
      try {
        const r = await fetch(`${API_BASE}/api/stats`);
        const data: StatsResp = await r.json();
        if (cancelled) return;
        const next: Record<string, { up: number; down: number }> = {};
        for (const row of data.rows) {
          const p = globalPrev[row.id];
          next[row.id] = { up: row.bytes_up, down: row.bytes_down };

          if (!seenEndpoint[row.id]) {
            // First time we see this endpoint — just record the baseline.
            // DON'T push a delta this tick; otherwise the chart spikes
            // because (current - 0) is the full cumulative count.
            seenEndpoint[row.id] = true;
            globalHistory[row.id] = globalHistory[row.id] ?? [];
            continue;
          }

          const dU = p ? Math.max(0, row.bytes_up - p.up) : 0;
          const dD = p ? Math.max(0, row.bytes_down - p.down) : 0;
          const hist = globalHistory[row.id] ?? [];
          hist.push({ up: dU, down: dD });
          while (hist.length > HISTORY_LEN) hist.shift();
          globalHistory[row.id] = hist;
        }
        Object.assign(globalPrev, next);
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

  // Aggregate by RESOLVED label — sidecars are split into their kind
  // ("psiphon" / "masterdns") instead of all collapsing into "sidecar".
  // We also tag each bucket with a representative endpoint name so the
  // per-protocol card can show "sidecar-psiphon" instead of just "psiphon"
  // when the user wants to see which configured endpoint is doing the work.
  const byProtocol = useMemo(() => {
    type Bucket = {
      label: string;        // chart-legend label ("psiphon", "vless", ...)
      endpointName: string; // representative ep.Name for the card header
      dials: number;
      errors: number;
      bytesUp: number;
      bytesDown: number;
      history: HistoryEntry[];
    };
    const map = new Map<string, Bucket>();
    for (const row of stats.rows) {
      const label = resolveLabel(row);
      let b = map.get(label);
      if (!b) {
        b = {
          label,
          endpointName: displayEndpointName(row.name, row.id),
          dials: 0,
          errors: 0,
          bytesUp: 0,
          bytesDown: 0,
          history: Array.from({ length: HISTORY_LEN }, () => ({ up: 0, down: 0 })),
        };
        map.set(label, b);
      }
      b.dials += row.dials;
      b.errors += row.dial_errors;
      b.bytesUp += row.bytes_up;
      b.bytesDown += row.bytes_down;
      const hist = globalHistory[row.id] || [];
      // Add this endpoint's history into the bucket (align by tail).
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

  // Per-protocol series for the overlay chart — total bytes (up+down)
  // sampled at HISTORY_LEN ticks. Sorted so the largest protocol renders
  // FIRST (i.e. underneath), letting smaller series sit visibly on top.
  const overlaySeries = useMemo(() => {
    return byProtocol
      .map((b) => ({
        label: b.label,
        color: colorForProtocol(b.label),
        samples: b.history.map((h) => h.up + h.down),
        peak: Math.max(0, ...b.history.map((h) => h.up + h.down)),
      }))
      .sort((a, b) => b.peak - a.peak);
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

      {/* Overlay chart — each protocol filled from 0 in its own color. */}
      <Section title="Throughput by protocol (rolling 2-min window @ 2s)">
        <OverlayAreaChart series={overlaySeries} />
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
              const color = colorForProtocol(b.label);
              return (
                <div
                  key={b.label}
                  style={{
                    background: theme.surface2,
                    // Subtle colored left border so the user can scan-link
                    // the card to its line in the overlay chart above.
                    borderLeft: `3px solid ${color}`,
                    border: `1px solid ${theme.border}`,
                    borderLeftWidth: 3,
                    borderLeftColor: color,
                    borderRadius: 6,
                    padding: "0.75rem",
                  }}
                >
                  <div style={{ display: "flex", justifyContent: "space-between", alignItems: "baseline" }}>
                    <div>
                      <div
                        style={{
                          fontFamily: theme.mono,
                          fontSize: "0.78rem",
                          color,
                          fontWeight: 600,
                        }}
                      >
                        {b.endpointName}
                      </div>
                      <div
                        style={{
                          marginTop: 2,
                          fontFamily: theme.mono,
                          fontSize: "0.65rem",
                          color: theme.textDim,
                          textTransform: "uppercase",
                          letterSpacing: "0.04em",
                        }}
                      >
                        <span
                          style={{
                            padding: "1px 5px",
                            border: `1px solid ${color}55`,
                            borderRadius: 3,
                            color,
                          }}
                        >
                          {b.label}
                        </span>
                      </div>
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
                      <div>{displayEndpointName(row.name, row.id)}</div>
                      <div style={{ color: theme.textDim, fontSize: "0.68rem", fontFamily: theme.mono }}>
                        {resolveLabel(row)} · {row.address}{" "}
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

// OverlayAreaChart draws each protocol as its OWN translucent area from
// zero — overlaid, not stacked. Cleaner than stacked when one protocol
// (e.g. psiphon) dwarfs another (vless) — the smaller series doesn't
// vanish into a thin sliver at the top of the stack.
//
// Series are pre-sorted so the LARGEST renders first (underneath); smaller
// series draw on top with the same translucent fill + 1px stroke, so they
// remain visible against the bigger one.
interface OverlayProps {
  series: { label: string; color: string; samples: number[]; peak: number }[];
}
function OverlayAreaChart({ series }: OverlayProps) {
  const width = 900;
  const height = 140;
  const padLeft = 52; // y-axis labels
  const padBottom = 18; // x-axis labels
  const innerW = width - padLeft;
  const innerH = height - padBottom;

  const max = Math.max(1, ...series.map((s) => s.peak));
  const hasTraffic = series.some((s) => s.peak > 0);
  if (series.length === 0 || !hasTraffic) {
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
        no traffic yet — make a request through the SOCKS5 proxy to see the chart fill in
      </div>
    );
  }

  const ticks = [0, max / 2, max];
  const N = series[0]?.samples.length ?? 0;

  // Build SVG paths for one series.
  const pathFor = (samples: number[]) => {
    const pts: string[] = [];
    for (let i = 0; i < samples.length; i++) {
      const x = padLeft + (i / (samples.length - 1)) * innerW;
      const y = innerH - (samples[i] / max) * innerH;
      pts.push(`${x.toFixed(1)},${y.toFixed(1)}`);
    }
    const fill = `M ${padLeft},${innerH} L ${pts.join(" L ")} L ${(padLeft + innerW).toFixed(1)},${innerH} Z`;
    const line = `M ${pts.join(" L ")}`;
    return { fill, line };
  };

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
        {/* Y-axis grid + labels */}
        {ticks.map((t, i) => {
          const y = innerH - (t / max) * innerH;
          return (
            <g key={i}>
              <line x1={padLeft} y1={y} x2={width} y2={y} stroke="#1e2d3d" strokeDasharray="3 3" strokeWidth={0.5} />
              <text x={padLeft - 6} y={y + 3} textAnchor="end" fontSize={9} fontFamily="monospace" fill="#6e7681">
                {fmtBytes(t)}/s
              </text>
            </g>
          );
        })}

        {/* Overlay each protocol's own area from 0 — LARGEST first (under). */}
        {series.map((s) => {
          if (s.peak === 0) return null;
          const { fill, line } = pathFor(s.samples);
          return (
            <g key={s.label}>
              <path d={fill} fill={s.color} fillOpacity={0.18} />
              <path d={line} fill="none" stroke={s.color} strokeWidth={1.5} strokeLinejoin="round" />
            </g>
          );
        })}

        {/* X-axis labels */}
        <text x={padLeft} y={innerH + 12} textAnchor="start" fontSize={9} fontFamily="monospace" fill="#6e7681">
          -2m
        </text>
        <text x={padLeft + innerW / 2} y={innerH + 12} textAnchor="middle" fontSize={9} fontFamily="monospace" fill="#6e7681">
          -1m
        </text>
        <text x={width - 2} y={innerH + 12} textAnchor="end" fontSize={9} fontFamily="monospace" fill="#6e7681">
          now
        </text>
      </svg>

      {/* Legend — protocol name + peak throughput, one per series. */}
      <div style={{ display: "flex", flexWrap: "wrap", gap: "0.75rem", marginTop: "0.6rem" }}>
        {series.map((s) => (
          <div
            key={s.label}
            style={{
              display: "inline-flex",
              alignItems: "center",
              gap: 6,
              fontSize: "0.72rem",
              fontFamily: theme.mono,
              color: theme.text,
            }}
            title={`peak ${fmtBytes(s.peak)}/s`}
          >
            <span
              style={{
                width: 12,
                height: 4,
                background: s.color,
                display: "inline-block",
                borderRadius: 2,
              }}
            />
            <span>{s.label}</span>
            <span style={{ color: theme.textDim }}>
              peak {fmtBytes(s.peak)}/s
            </span>
          </div>
        ))}
      </div>
      {/* Sample-count hint when we don't have a full window yet. */}
      {N > 0 && N < HISTORY_LEN && (
        <div style={{ marginTop: 4, fontSize: "0.66rem", color: theme.textDim, fontFamily: theme.mono }}>
          collecting samples · {N}/{HISTORY_LEN}
        </div>
      )}
    </div>
  );
}
