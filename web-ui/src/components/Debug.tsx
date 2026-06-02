import { useEffect, useMemo, useRef, useState } from "react";

const API_BASE = (import.meta.env.VITE_API_URL as string | undefined) ?? "http://localhost:8088";
const WS_BASE = API_BASE.replace(/^http/, "ws");

type Level = "info" | "warn" | "error";

interface LogEvent {
  ts: number;
  level: Level;
  source: string;
  message: string;
}

const LEVEL_COLOR: Record<Level, { fg: string; bg: string }> = {
  info: { fg: "#1d4ed8", bg: "#dbeafe" },
  warn: { fg: "#a16207", bg: "#fef3c7" },
  error: { fg: "#b91c1c", bg: "#fee2e2" },
};

const ROW_BG: Record<Level, string> = {
  info: "transparent",
  warn: "rgba(254, 243, 199, 0.4)",
  error: "rgba(254, 226, 226, 0.5)",
};

const fmtTime = (ms: number) => {
  const d = new Date(ms);
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}.${pad(d.getMilliseconds() % 1000).slice(0, 3)}`;
};

const RING_LIMIT = 1000;

export default function Debug() {
  const [events, setEvents] = useState<LogEvent[]>([]);
  const [levels, setLevels] = useState<Record<Level, boolean>>({ info: true, warn: true, error: true });
  const [query, setQuery] = useState("");
  const [paused, setPaused] = useState(false);
  const [autoscroll, setAutoscroll] = useState(true);
  const wsRef = useRef<WebSocket | null>(null);
  const pausedRef = useRef(paused);
  const tailRef = useRef<HTMLDivElement | null>(null);

  pausedRef.current = paused;

  // Initial snapshot + WS tail.
  useEffect(() => {
    let cancelled = false;
    fetch(`${API_BASE}/api/logs`)
      .then((r) => r.json())
      .then((data) => {
        if (cancelled) return;
        setEvents((data.events ?? []) as LogEvent[]);
      })
      .catch(() => {});

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
    return () => {
      cancelled = true;
      ws.close();
    };
  }, []);

  // Autoscroll on new events.
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
    navigator.clipboard?.writeText(txt);
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
              border: "1px solid",
              borderColor: levels[lvl] ? LEVEL_COLOR[lvl].fg : "#cbd5e1",
              borderRadius: 14,
              background: levels[lvl] ? LEVEL_COLOR[lvl].bg : "#fff",
              color: levels[lvl] ? LEVEL_COLOR[lvl].fg : "#64748b",
              fontSize: "0.78rem",
              fontWeight: 600,
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
          placeholder="Filter by text…"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          style={{
            flex: 1,
            minWidth: 180,
            padding: "0.35rem 0.6rem",
            border: "1px solid #e2e8f0",
            borderRadius: 6,
            fontSize: "0.85rem",
          }}
        />
        <button
          onClick={() => setPaused((p) => !p)}
          style={{
            padding: "0.35rem 0.8rem",
            background: paused ? "#fef3c7" : "#fff",
            border: "1px solid #e2e8f0",
            borderRadius: 6,
            cursor: "pointer",
            fontSize: "0.82rem",
            fontWeight: 500,
            color: paused ? "#a16207" : "#0f172a",
          }}
        >
          {paused ? "▶ Resume" : "❚❚ Pause"}
        </button>
        <label style={{ fontSize: "0.78rem", color: "#475569", display: "flex", alignItems: "center", gap: 4 }}>
          <input type="checkbox" checked={autoscroll} onChange={(e) => setAutoscroll(e.target.checked)} />
          autoscroll
        </label>
        <button
          onClick={copyAll}
          style={{
            padding: "0.35rem 0.8rem",
            background: "#fff",
            border: "1px solid #e2e8f0",
            borderRadius: 6,
            cursor: "pointer",
            fontSize: "0.82rem",
          }}
        >
          Copy
        </button>
        <button
          onClick={() => setEvents([])}
          style={{
            padding: "0.35rem 0.8rem",
            background: "#fff",
            border: "1px solid #e2e8f0",
            borderRadius: 6,
            cursor: "pointer",
            fontSize: "0.82rem",
            color: "#64748b",
          }}
        >
          Clear view
        </button>
      </div>

      <div
        ref={tailRef}
        style={{
          fontFamily: "ui-monospace, SFMono-Regular, monospace",
          fontSize: "0.78rem",
          background: "#0f172a",
          color: "#e2e8f0",
          borderRadius: 8,
          padding: "0.75rem",
          height: 460,
          overflowY: "auto",
          lineHeight: 1.55,
        }}
      >
        {filtered.length === 0 ? (
          <div style={{ color: "#64748b" }}>No log events match the current filter.</div>
        ) : (
          filtered.map((e, i) => (
            <div
              key={i + ":" + e.ts}
              style={{
                background: ROW_BG[e.level],
                color: e.level === "error" ? "#fecaca" : e.level === "warn" ? "#fde68a" : "#e2e8f0",
                padding: "1px 4px",
                borderRadius: 3,
                whiteSpace: "pre-wrap",
                wordBreak: "break-word",
              }}
            >
              <span style={{ color: "#64748b" }}>{fmtTime(e.ts)}</span>{" "}
              <span
                style={{
                  display: "inline-block",
                  minWidth: 42,
                  padding: "0 4px",
                  background: LEVEL_COLOR[e.level].bg,
                  color: LEVEL_COLOR[e.level].fg,
                  borderRadius: 3,
                  fontWeight: 700,
                  fontSize: "0.7rem",
                  textAlign: "center",
                  marginRight: 6,
                }}
              >
                {e.level.toUpperCase()}
              </span>
              <span style={{ color: "#94a3b8" }}>{e.source}</span>{" "}
              {e.message}
            </div>
          ))
        )}
      </div>

      <div style={{ fontSize: "0.72rem", color: "#94a3b8" }}>
        Showing {filtered.length} of {events.length} events (server ring buffer holds the last {RING_LIMIT}).
      </div>
    </div>
  );
}
