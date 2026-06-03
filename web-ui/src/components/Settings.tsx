import { useEffect, useState } from "react";
import { theme } from "../theme";

const API_BASE = (import.meta.env.VITE_API_URL as string | undefined) ?? "http://localhost:8088";

type Strategy = "latency" | "priority" | "weighted";

const STRATEGY_OPTIONS: { value: Strategy; label: string; help: string }[] = [
  { value: "latency", label: "Latency", help: "Pick the endpoint with the lowest measured RTT through sing-box." },
  { value: "priority", label: "Priority", help: "Pick the endpoint with the lowest Priority field (config-driven)." },
  { value: "weighted", label: "Weighted random", help: "Random pick with weights = Priority field. Spreads load." },
];

interface Props {
  refreshTick?: number;
}

export default function Settings({ refreshTick }: Props) {
  const [strategy, setStrategy] = useState<Strategy>("latency");
  const [loaded, setLoaded] = useState(false);
  const [probing, setProbing] = useState(false);
  const [toast, setToast] = useState<{ msg: string; ok: boolean } | null>(null);

  useEffect(() => {
    fetch(`${API_BASE}/api/stats`)
      .then((r) => r.json())
      .then((data) => {
        if (data?.strategy && STRATEGY_OPTIONS.find((o) => o.value === data.strategy)) {
          setStrategy(data.strategy as Strategy);
        }
        setLoaded(true);
      })
      .catch(() => setLoaded(true));
  }, [refreshTick]);

  const flash = (msg: string, ok: boolean) => {
    setToast({ msg, ok });
    setTimeout(() => setToast(null), 2500);
  };

  const switchStrategy = async (s: Strategy) => {
    const prev = strategy;
    setStrategy(s);
    try {
      const res = await fetch(`${API_BASE}/api/strategy`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ strategy: s }),
      });
      if (!res.ok) {
        setStrategy(prev);
        flash(`Failed: ${res.status} ${res.statusText}`, false);
      } else {
        flash(`strategy → ${s}`, true);
      }
    } catch {
      setStrategy(prev);
      flash("Could not reach proxy-core API.", false);
    }
  };

  const triggerProbe = async () => {
    setProbing(true);
    try {
      const res = await fetch(`${API_BASE}/api/probe`, { method: "POST" });
      if (res.ok) {
        flash("Probing all endpoints…", true);
      } else {
        flash(`Failed: ${res.status} ${res.statusText}`, false);
      }
    } catch {
      flash("Could not reach proxy-core API.", false);
    } finally {
      setTimeout(() => setProbing(false), 4500);
    }
  };

  return (
    <div>
      <section style={{ marginBottom: "2rem" }}>
        <h3 style={section()}>load-balancing strategy</h3>
        <p style={blurb()}>Applied immediately, no restart required.</p>
        <div style={{ display: "flex", flexDirection: "column", gap: "0.5rem" }}>
          {STRATEGY_OPTIONS.map((opt) => (
            <label
              key={opt.value}
              style={{
                display: "flex",
                gap: "0.75rem",
                padding: "0.75rem",
                border: `1px solid ${strategy === opt.value ? theme.green : theme.border}`,
                borderRadius: 6,
                cursor: loaded ? "pointer" : "not-allowed",
                background: strategy === opt.value ? theme.greenDim : theme.surface2,
              }}
            >
              <input
                type="radio"
                name="strategy"
                value={opt.value}
                checked={strategy === opt.value}
                disabled={!loaded}
                onChange={() => switchStrategy(opt.value)}
                style={{ marginTop: 3 }}
              />
              <div>
                <div style={{ fontFamily: theme.mono, fontWeight: 600, color: theme.text, fontSize: "0.82rem" }}>
                  {opt.label}
                </div>
                <div style={{ color: theme.textDim, fontSize: "0.74rem", marginTop: 2 }}>{opt.help}</div>
              </div>
            </label>
          ))}
        </div>
      </section>

      <section style={{ marginBottom: "1.5rem" }}>
        <h3 style={section()}>actions</h3>
        <button
          onClick={triggerProbe}
          disabled={probing}
          style={{
            display: "inline-flex",
            alignItems: "center",
            gap: "0.5rem",
            padding: "0.45rem 1rem",
            background: probing ? theme.surface2 : "transparent",
            color: theme.blue,
            border: `1px solid ${theme.blue}`,
            borderRadius: 4,
            cursor: probing ? "wait" : "pointer",
            fontFamily: theme.mono,
            fontSize: "0.72rem",
            fontWeight: 600,
            textTransform: "uppercase",
            letterSpacing: "0.04em",
          }}
        >
          {probing && (
            <span
              style={{
                width: 10,
                height: 10,
                border: `2px solid ${theme.blue}`,
                borderTopColor: "transparent",
                borderRadius: "50%",
                animation: "spin 0.7s linear infinite",
              }}
            />
          )}
          probe all endpoints
        </button>
        <style>{`@keyframes spin { to { transform: rotate(360deg); } }`}</style>
        <div style={{ marginTop: "0.4rem", color: theme.textDim, fontSize: "0.72rem" }}>
          Results stream into the <strong>Endpoints</strong> tab via WebSocket — switch tabs to watch them update.
        </div>
      </section>

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
            fontFamily: theme.mono,
            fontWeight: 600,
          }}
        >
          {toast.msg}
        </div>
      )}
    </div>
  );
}

const section = (): React.CSSProperties => ({
  margin: "0 0 0.5rem",
  fontFamily: theme.mono,
  fontSize: "0.78rem",
  color: theme.text,
  textTransform: "uppercase" as const,
  letterSpacing: "0.04em",
});
const blurb = (): React.CSSProperties => ({
  margin: "0 0 0.75rem",
  color: theme.textDim,
  fontSize: "0.78rem",
});
