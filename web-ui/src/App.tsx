import { useEffect, useState } from "react";
import EndpointTable from "./components/EndpointTable";
import ConfigEditor from "./components/ConfigEditor";
import Analytics from "./components/Analytics";
import Settings from "./components/Settings";
import Debug from "./components/Debug";
import Plugins from "./components/Plugins";
import Sources from "./components/Sources";
import Footer from "./components/Footer";
import Diagnostics from "./components/Diagnostics";
import { theme } from "./theme";
import { API_BASE } from "./apiBase";

type Tab = "endpoints" | "sources" | "analytics" | "plugins" | "settings" | "debug" | "diag" | "config";

const TAB_LABELS: Record<Tab, string> = {
  endpoints: "Endpoints",
  sources: "Sources",
  analytics: "Analytics",
  plugins: "Plugins",
  settings: "Settings",
  debug: "Debug",
  diag: "Diagnostics",
  config: "Config",
};

const tabStyle = (active: boolean): React.CSSProperties => ({
  padding: "0.55rem 1.1rem",
  border: "1px solid transparent",
  borderBottom: active ? `2px solid ${theme.green}` : "2px solid transparent",
  background: "none",
  cursor: "pointer",
  fontWeight: active ? 600 : 400,
  color: active ? theme.text : theme.textDim,
  fontSize: "0.78rem",
  fontFamily: theme.mono,
  letterSpacing: "0.02em",
  textTransform: "uppercase" as const,
  transition: "color 0.15s, border-color 0.15s",
});

const btn: React.CSSProperties = {
  fontFamily: theme.mono,
  fontSize: "0.7rem",
  padding: "0.35rem 0.75rem",
  borderRadius: 4,
  border: `1px solid ${theme.border}`,
  background: theme.surface,
  color: theme.text,
  cursor: "pointer",
  display: "inline-flex",
  alignItems: "center",
  gap: "0.3rem",
  transition: "border-color 0.15s",
};

function RestartButton() {
  const [busy, setBusy] = useState(false);
  const click = async () => {
    if (!window.confirm("Restart proxy-core?\n\nApplies any pending config / source changes. The dashboard will reconnect in a few seconds.")) {
      return;
    }
    setBusy(true);
    try {
      await fetch(`${API_BASE}/api/sources/reload`, { method: "POST" });
    } catch {
      // The restart kills the connection mid-flight — that's expected.
    }
    setTimeout(() => setBusy(false), 6000);
  };
  return (
    <button
      onClick={click}
      disabled={busy}
      style={{
        fontFamily: theme.mono,
        fontSize: "0.7rem",
        padding: "0.35rem 0.75rem",
        borderRadius: 4,
        border: `1px solid ${theme.yellow}`,
        background: busy ? theme.yellowDim : "transparent",
        color: theme.yellow,
        cursor: busy ? "wait" : "pointer",
      }}
      title="Restart proxy-core to apply pending config / source changes"
    >
      {busy ? "restarting…" : "↻ Apply / restart"}
    </button>
  );
}

export default function App() {
  const [tab, setTab] = useState<Tab>("endpoints");
  const [healthy, setHealthy] = useState(0);
  const [total, setTotal] = useState(0);
  const [refreshTick, setRefreshTick] = useState(0);
  const [clock, setClock] = useState(new Date());

  useEffect(() => {
    const id = setInterval(() => setClock(new Date()), 1000);
    return () => clearInterval(id);
  }, []);

  const refresh = () => setRefreshTick((t) => t + 1);

  return (
    <div
      style={{
        maxWidth: 1100,
        margin: "0 auto",
        padding: "1.5rem 2rem",
        fontFamily: theme.sans,
        color: theme.text,
        minHeight: "100vh",
      }}
    >
      {/* Topbar */}
      <header
        style={{
          display: "flex",
          justifyContent: "space-between",
          alignItems: "center",
          paddingBottom: "1rem",
          marginBottom: "1.25rem",
          borderBottom: `1px solid ${theme.border}`,
        }}
      >
        <h1
          style={{
            margin: 0,
            fontFamily: theme.mono,
            fontSize: "1.1rem",
            fontWeight: 600,
            letterSpacing: "-0.02em",
            color: theme.text,
            display: "flex",
            alignItems: "center",
            gap: "0.55rem",
          }}
        >
          <img
            src="/logo.png"
            alt="MoaV"
            style={{ height: 24, width: "auto", display: "block" }}
          />
          MoaV<span style={{ color: theme.green }}>-client</span>
          <span
            style={{
              marginLeft: 8,
              color: theme.textDim,
              fontSize: "0.62rem",
              letterSpacing: "0.15em",
              textTransform: "uppercase",
            }}
          >
            mother of all VPNs
          </span>
        </h1>
        <div style={{ display: "flex", alignItems: "center", gap: "0.75rem" }}>
          <span
            style={{
              color: theme.textDim,
              fontFamily: theme.mono,
              fontSize: "0.7rem",
            }}
          >
            {clock.toISOString().slice(11, 19)} UTC
          </span>
          <span
            style={{
              display: "inline-flex",
              alignItems: "center",
              padding: "0.25rem 0.7rem",
              borderRadius: 12,
              fontSize: "0.7rem",
              fontWeight: 600,
              fontFamily: theme.mono,
              background:
                total === 0
                  ? theme.redDim
                  : healthy === total
                  ? theme.greenDim
                  : theme.yellowDim,
              color:
                total === 0
                  ? theme.red
                  : healthy === total
                  ? theme.green
                  : theme.yellow,
              border: `1px solid ${
                total === 0
                  ? theme.red
                  : healthy === total
                  ? theme.green
                  : theme.yellow
              }44`,
            }}
          >
            ● {total === 0 ? "no endpoints" : `${healthy}/${total} healthy`}
          </span>
          <button
            onClick={refresh}
            style={{
              ...btn,
              borderColor: theme.blue,
              color: theme.blue,
            }}
            title="Refresh all tabs (re-fetch endpoints / stats / logs in place)"
          >
            ↻ Refresh
          </button>
          <RestartButton />
        </div>
      </header>

      {/* Tab bar */}
      <nav
        style={{
          display: "flex",
          borderBottom: `1px solid ${theme.border}`,
          marginBottom: "1.25rem",
          flexWrap: "wrap",
        }}
      >
        {(Object.keys(TAB_LABELS) as Tab[]).map((t) => (
          <button key={t} style={tabStyle(tab === t)} onClick={() => setTab(t)}>
            {TAB_LABELS[t]}
          </button>
        ))}
      </nav>

      {/* Content card */}
      <div
        style={{
          background: theme.surface,
          border: `1px solid ${theme.border}`,
          borderRadius: 8,
          padding: "1.25rem",
        }}
      >
        {tab === "endpoints" && (
          <EndpointTable
            refreshTick={refreshTick}
            onHealthChange={(h, t) => {
              setHealthy(h);
              setTotal(t);
            }}
          />
        )}
        {tab === "sources" && <Sources refreshTick={refreshTick} />}
        {tab === "analytics" && <Analytics refreshTick={refreshTick} />}
        {tab === "plugins" && <Plugins refreshTick={refreshTick} />}
        {tab === "settings" && <Settings refreshTick={refreshTick} />}
        {tab === "debug" && <Debug refreshTick={refreshTick} />}
        {tab === "diag" && <Diagnostics refreshTick={refreshTick} />}
        {tab === "config" && <ConfigEditor refreshTick={refreshTick} />}
      </div>

      <Footer />
    </div>
  );
}
