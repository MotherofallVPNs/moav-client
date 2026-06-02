import { useEffect, useState } from "react";

const API_BASE = (import.meta.env.VITE_API_URL as string | undefined) ?? "http://localhost:8088";

type Strategy = "latency" | "priority" | "weighted";

const STRATEGY_OPTIONS: { value: Strategy; label: string; help: string }[] = [
  { value: "latency", label: "Latency", help: "Pick the endpoint with the lowest measured RTT through sing-box." },
  { value: "priority", label: "Priority", help: "Pick the endpoint with the lowest Priority field (config-driven)." },
  { value: "weighted", label: "Weighted random", help: "Random pick with weights = Priority field. Spreads load." },
];

export default function Settings() {
  const [strategy, setStrategy] = useState<Strategy>("latency");
  const [loaded, setLoaded] = useState(false);
  const [toast, setToast] = useState<{ msg: string; ok: boolean } | null>(null);

  // Load current strategy from /api/stats (it carries .strategy).
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
  }, []);

  const showToast = (msg: string, ok: boolean) => {
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
        showToast(`Failed: ${res.status} ${res.statusText}`, false);
      } else {
        showToast(`Strategy set to ${s}.`, true);
      }
    } catch (e) {
      setStrategy(prev);
      showToast("Could not reach proxy-core API.", false);
    }
  };

  const triggerProbe = async () => {
    try {
      const res = await fetch(`${API_BASE}/api/probe`, { method: "POST" });
      if (res.ok) {
        showToast("Probing all endpoints…", true);
      } else {
        showToast(`Failed: ${res.status} ${res.statusText}`, false);
      }
    } catch {
      showToast("Could not reach proxy-core API.", false);
    }
  };

  return (
    <div>
      <section style={{ marginBottom: "2rem" }}>
        <h3 style={{ margin: "0 0 0.5rem", fontSize: "0.95rem", color: "#0f172a" }}>Load-balancing strategy</h3>
        <p style={{ margin: "0 0 0.75rem", color: "#64748b", fontSize: "0.85rem" }}>
          Applied immediately; no restart required.
        </p>
        <div style={{ display: "flex", flexDirection: "column", gap: "0.5rem" }}>
          {STRATEGY_OPTIONS.map((opt) => (
            <label
              key={opt.value}
              style={{
                display: "flex",
                gap: "0.75rem",
                padding: "0.75rem",
                border: "1px solid",
                borderColor: strategy === opt.value ? "#3b82f6" : "#e2e8f0",
                borderRadius: 8,
                cursor: loaded ? "pointer" : "not-allowed",
                background: strategy === opt.value ? "#eff6ff" : "#fff",
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
                <div style={{ fontWeight: 600, color: "#0f172a", fontSize: "0.9rem" }}>{opt.label}</div>
                <div style={{ color: "#64748b", fontSize: "0.8rem" }}>{opt.help}</div>
              </div>
            </label>
          ))}
        </div>
      </section>

      <section style={{ marginBottom: "1.5rem" }}>
        <h3 style={{ margin: "0 0 0.5rem", fontSize: "0.95rem", color: "#0f172a" }}>Actions</h3>
        <button
          onClick={triggerProbe}
          style={{
            padding: "0.5rem 1rem",
            background: "#3b82f6",
            color: "#fff",
            border: "none",
            borderRadius: 6,
            cursor: "pointer",
            fontWeight: 500,
          }}
        >
          Probe all endpoints now
        </button>
      </section>

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
          }}
        >
          {toast.msg}
        </div>
      )}
    </div>
  );
}
