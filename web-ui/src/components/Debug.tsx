import { useEffect, useMemo, useRef, useState } from "react";
import { theme } from "../theme";
import { API_BASE, WS_BASE } from "../apiBase";
import { displayEndpointId } from "../display";
import { copyText } from "../clipboard";


type Level = "info" | "warn" | "error";

interface LogEvent {
  ts: number;
  level: Level;
  source: string;
  message: string;
}

const LEVEL_COLOR: Record<Level, { fg: string; bg: string }> = {
  info: { fg: theme.blue, bg: theme.blueDim },
  warn: { fg: theme.yellow, bg: theme.yellowDim },
  error: { fg: theme.red, bg: theme.redDim },
};

const fmtTime = (ms: number) => {
  const d = new Date(ms);
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}.${pad(d.getMilliseconds() % 1000).slice(0, 3)}`;
};

const RING_LIMIT = 1000;

interface Props {
  refreshTick?: number;
}

export default function Debug({ refreshTick }: Props) {
  const [events, setEvents] = useState<LogEvent[]>([]);
  const [levels, setLevels] = useState<Record<Level, boolean>>({ info: true, warn: true, error: true });
  const [query, setQuery] = useState("");
  const [paused, setPaused] = useState(false);
  const [autoscroll, setAutoscroll] = useState(true);
  const wsRef = useRef<WebSocket | null>(null);
  const pausedRef = useRef(paused);
  const tailRef = useRef<HTMLDivElement | null>(null);

  pausedRef.current = paused;

  useEffect(() => {
    let cancelled = false;
    fetch(`${API_BASE}/api/logs`)
      .then((r) => r.json())
      .then((data) => {
        if (cancelled) return;
        setEvents((data.events ?? []) as LogEvent[]);
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [refreshTick]);

  useEffect(() => {
    const ws = new WebSocket(`${WS_BASE}/api/ws`);
    wsRef.current = ws;
    ws.onmessage = (ev) => {
      try {
        const frame = JSON.parse(ev.data as string);
        if (frame.log && !pausedRef.current) {
          setEvents((prev) => {
            const next = [...prev, frame.log as LogEvent];
            if (next.length > RING_LIMIT) next.splice(0, next.length - RING_LIMIT);
            return next;
          });
        }
      } catch {
        // ignore
      }
    };
    return () => ws.close();
  }, []);

  useEffect(() => {
    if (!autoscroll || !tailRef.current) return;
    tailRef.current.scrollTop = tailRef.current.scrollHeight;
  }, [events, autoscroll]);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    return events.filter((e) => {
      if (!levels[e.level]) return false;
      if (!q) return true;
      return e.message.toLowerCase().includes(q) || e.source.toLowerCase().includes(q);
    });
  }, [events, levels, query]);

  const counts = useMemo(() => {
    const c = { info: 0, warn: 0, error: 0 };
    for (const e of events) c[e.level]++;
    return c;
  }, [events]);

  const toggle = (lvl: Level) => setLevels((s) => ({ ...s, [lvl]: !s[lvl] }));
  const copyAll = () => {
    const txt = filtered.map((e) => `${fmtTime(e.ts)} [${e.level.toUpperCase()}] ${e.source}: ${e.message}`).join("\n");
    copyText(txt);
  };

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "0.75rem" }}>
      <div style={{ display: "flex", flexWrap: "wrap", gap: "0.5rem", alignItems: "center" }}>
        {(["info", "warn", "error"] as Level[]).map((lvl) => (
          <button
            key={lvl}
            onClick={() => toggle(lvl)}
            style={{
              padding: "0.25rem 0.7rem",
              border: `1px solid ${levels[lvl] ? LEVEL_COLOR[lvl].fg : theme.border}`,
              borderRadius: 12,
              background: levels[lvl] ? LEVEL_COLOR[lvl].bg : "transparent",
              color: levels[lvl] ? LEVEL_COLOR[lvl].fg : theme.textDim,
              fontSize: "0.7rem",
              fontWeight: 600,
              fontFamily: theme.mono,
              cursor: "pointer",
              textTransform: "uppercase",
              letterSpacing: 0.3,
            }}
          >
            {lvl} ({counts[lvl]})
          </button>
        ))}
        <input
          type="text"
          placeholder="Filter…"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          style={{
            flex: 1,
            minWidth: 180,
            padding: "0.35rem 0.6rem",
            borderRadius: 4,
            fontSize: "0.8rem",
            fontFamily: theme.mono,
          }}
        />
        <button onClick={() => setPaused((p) => !p)} style={chip(paused ? theme.yellow : theme.textDim)}>
          {paused ? "▶ resume" : "❚❚ pause"}
        </button>
        <label style={{ fontSize: "0.72rem", color: theme.textDim, display: "flex", alignItems: "center", gap: 4, fontFamily: theme.mono }}>
          <input type="checkbox" checked={autoscroll} onChange={(e) => setAutoscroll(e.target.checked)} />
          autoscroll
        </label>
        <button onClick={copyAll} style={chip(theme.textDim)}>copy</button>
        <button onClick={() => setEvents([])} style={chip(theme.textDim)}>clear</button>
      </div>

      <div
        ref={tailRef}
        style={{
          fontFamily: theme.mono,
          fontSize: "0.74rem",
          background: theme.bg,
          color: theme.text,
          borderRadius: 6,
          border: `1px solid ${theme.border}`,
          padding: "0.75rem",
          height: 460,
          overflowY: "auto",
          lineHeight: 1.55,
        }}
      >
        {filtered.length === 0 ? (
          <div style={{ color: theme.textDim }}>No log events match the current filter.</div>
        ) : (
          filtered.map((e, i) => (
            <div
              key={i + ":" + e.ts}
              style={{
                color: e.level === "error" ? theme.red : e.level === "warn" ? theme.yellow : theme.text,
                whiteSpace: "pre-wrap",
                wordBreak: "break-word",
                padding: "1px 4px",
              }}
            >
              <span style={{ color: theme.textDim }}>{fmtTime(e.ts)}</span>{" "}
              <span
                style={{
                  display: "inline-block",
                  minWidth: 44,
                  padding: "0 4px",
                  background: LEVEL_COLOR[e.level].bg,
                  color: LEVEL_COLOR[e.level].fg,
                  borderRadius: 3,
                  fontWeight: 700,
                  fontSize: "0.65rem",
                  textAlign: "center",
                  marginRight: 6,
                  border: `1px solid ${LEVEL_COLOR[e.level].fg}44`,
                }}
              >
                {e.level.toUpperCase()}
              </span>
              <span style={{ color: theme.green }}>{e.source}</span> {e.message}
            </div>
          ))
        )}
      </div>

      <div style={{ fontSize: "0.7rem", color: theme.textDim, fontFamily: theme.mono }}>
        showing {filtered.length} of {events.length} events · server ring buffer holds the last {RING_LIMIT}
      </div>

      <FlowsPane refreshTick={refreshTick} />
    </div>
  );
}

interface Flow {
  id: number;
  opened_unix: number;
  closed_unix?: number;
  client: string;
  dest: string;
  endpoint_id: string;
  protocol: string;
  bytes_up: number;
  bytes_down: number;
  result: string;
}

function fmtBytesShort(n: number): string {
  if (!n) return "—";
  const units = ["B", "KB", "MB", "GB"];
  let i = 0;
  let v = n;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(v >= 10 || i === 0 ? 0 : 1)}${units[i]}`;
}

function FlowsPane({ refreshTick }: { refreshTick?: number }) {
  const [flows, setFlows] = useState<Flow[]>([]);

  useEffect(() => {
    let cancelled = false;
    const fetchOnce = async () => {
      try {
        const r = await fetch(`${API_BASE}/api/flows`);
        const d = await r.json();
        if (!cancelled) setFlows((d.flows ?? []) as Flow[]);
      } catch {
        // ignore
      }
    };
    fetchOnce();
    const id = setInterval(fetchOnce, 3000);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, [refreshTick]);

  return (
    <div
      style={{
        marginTop: "1rem",
        border: `1px solid ${theme.border}`,
        borderRadius: 6,
        overflow: "hidden",
      }}
    >
      <div
        style={{
          padding: "0.5rem 0.75rem",
          background: theme.surface2,
          color: theme.textDim,
          fontFamily: theme.mono,
          fontSize: "0.7rem",
          textTransform: "uppercase",
          letterSpacing: "0.04em",
        }}
      >
        per-connection flows · last {flows.length}
      </div>
      {flows.length === 0 ? (
        <div style={{ padding: "0.75rem", fontFamily: theme.mono, fontSize: "0.78rem", color: theme.textDim }}>
          no connections recorded yet — send some traffic through the SOCKS5 proxy.
        </div>
      ) : (
        <div
          style={{
            maxHeight: 200,
            overflowY: "auto",
            fontFamily: theme.mono,
            fontSize: "0.72rem",
          }}
        >
          {flows
            .slice()
            .reverse()
            .map((fl) => (
              <div
                key={fl.id}
                style={{
                  display: "grid",
                  gridTemplateColumns: "60px 1fr 1fr 70px 70px 80px",
                  gap: "0.5rem",
                  padding: "4px 0.75rem",
                  borderTop: `1px solid ${theme.border}`,
                  color: fl.result.startsWith("error") ? theme.red : theme.text,
                }}
              >
                <span style={{ color: theme.textDim }}>#{fl.id}</span>
                <span style={{ color: theme.blue, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }} title={fl.dest}>
                  {fl.dest}
                </span>
                <span style={{ color: theme.green, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }} title={fl.endpoint_id}>
                  {fl.protocol === "sidecar"
                    ? displayEndpointId(fl.endpoint_id)
                    : `${fl.protocol}${fl.endpoint_id ? ` · ${displayEndpointId(fl.endpoint_id)}` : ""}`}
                </span>
                <span style={{ color: theme.text }}>↑{fmtBytesShort(fl.bytes_up)}</span>
                <span style={{ color: theme.text }}>↓{fmtBytesShort(fl.bytes_down)}</span>
                <span style={{ color: theme.textDim, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }} title={fl.result}>
                  {fl.result}
                </span>
              </div>
            ))}
        </div>
      )}
    </div>
  );
}

const chip = (color: string): React.CSSProperties => ({
  padding: "0.35rem 0.7rem",
  background: "transparent",
  border: `1px solid ${theme.border}`,
  borderRadius: 4,
  cursor: "pointer",
  fontSize: "0.72rem",
  color,
  fontFamily: theme.mono,
});
