import { useState } from "react";
import EndpointTable from "./components/EndpointTable";
import ProbeButton from "./components/ProbeButton";
import ConfigEditor from "./components/ConfigEditor";

type Tab = "endpoints" | "probe" | "config";

const tabStyle = (active: boolean): React.CSSProperties => ({
  padding: "0.4rem 1rem",
  border: "none",
  borderBottom: active ? "2px solid #3b82f6" : "2px solid transparent",
  background: "none",
  cursor: "pointer",
  fontWeight: active ? 600 : 400,
  color: active ? "#1d4ed8" : "#64748b",
  fontSize: "0.9rem",
});

export default function App() {
  const [tab, setTab] = useState<Tab>("endpoints");
  const [healthy, setHealthy] = useState(0);
  const [total, setTotal] = useState(0);

  return (
    <div style={{ fontFamily: "system-ui, sans-serif", maxWidth: 1000, margin: "0 auto", padding: "2rem 1rem" }}>
      <header style={{ display: "flex", alignItems: "center", gap: "1rem", marginBottom: "1.5rem" }}>
        <h1 style={{ margin: 0, fontSize: "1.4rem", color: "#0f172a" }}>moav-client</h1>
        <span
          style={{
            display: "inline-block",
            padding: "0.2rem 0.7rem",
            borderRadius: 12,
            fontSize: "0.78rem",
            fontWeight: 600,
            background: healthy > 0 ? "#dcfce7" : "#fee2e2",
            color: healthy > 0 ? "#16a34a" : "#dc2626",
          }}
        >
          {total === 0 ? "no endpoints" : `${healthy}/${total} healthy`}
        </span>
      </header>

      <nav style={{ display: "flex", borderBottom: "1px solid #e2e8f0", marginBottom: "1.5rem" }}>
        {(["endpoints", "probe", "config"] as Tab[]).map((t) => (
          <button key={t} style={tabStyle(tab === t)} onClick={() => setTab(t)}>
            {t.charAt(0).toUpperCase() + t.slice(1)}
          </button>
        ))}
      </nav>

      <div style={{ border: "1px solid #e2e8f0", borderRadius: 8, padding: "1.25rem" }}>
        {tab === "endpoints" && (
          <EndpointTable onHealthChange={(h, t) => { setHealthy(h); setTotal(t); }} />
        )}
        {tab === "probe" && <ProbeButton />}
        {tab === "config" && <ConfigEditor />}
      </div>
    </div>
  );
}
