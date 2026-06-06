import { useEffect, useState } from "react";
import EndpointTable from "./components/EndpointTable";
import BlockDirectToggle from "./components/BlockDirectToggle";
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
import { useIsMobile } from "./useIsMobile";

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
  padding: "0.5rem 0.85rem",
  border: `1px solid ${active ? theme.green : theme.border}`,
  borderRadius: 6,
  background: active ? theme.greenDim : theme.surface2,
  cursor: "pointer",
  fontWeight: active ? 700 : 500,
  color: active ? theme.green : theme.textDim,
  fontSize: "0.72rem",
  fontFamily: theme.mono,
  letterSpacing: "0.03em",
  textTransform: "uppercase" as const,
  transition: "all 0.15s",
  whiteSpace: "nowrap",
  textAlign: "center",
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

function RestartButton({ fullWidth }: { fullWidth?: boolean }) {
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
        padding: "0.4rem 0.75rem",
        borderRadius: 6,
        border: `1px solid ${theme.yellow}`,
        background: busy ? theme.yellowDim : "transparent",
        color: theme.yellow,
        cursor: busy ? "wait" : "pointer",
        width: fullWidth ? "100%" : undefined,
        flex: fullWidth ? 1 : undefined,
      }}
      title="Restart proxy-core to apply pending config / source changes"
    >
      {busy ? "restarting…" : "↻ Apply / restart"}
    </button>
  );
}

const TAB_STORAGE_KEY = "moav.activeTab";

function initialTab(): Tab {
  if (typeof window === "undefined") return "endpoints";
  const saved = window.localStorage.getItem(TAB_STORAGE_KEY);
  return saved && saved in TAB_LABELS ? (saved as Tab) : "endpoints";
}

export default function App() {
  const [tab, setTabState] = useState<Tab>(initialTab);
  const setTab = (t: Tab) => {
    setTabState(t);
    try {
      window.localStorage.setItem(TAB_STORAGE_KEY, t);
    } catch {
      // private mode / storage disabled — non-fatal, tab just won't persist.
    }
  };
  const [healthy, setHealthy] = useState(0);
  const [total, setTotal] = useState(0);
  const [refreshTick, setRefreshTick] = useState(0);
  const [clock, setClock] = useState(new Date());

  useEffect(() => {
    const id = setInterval(() => setClock(new Date()), 1000);
    return () => clearInterval(id);
  }, []);

  const refresh = () => setRefreshTick((t) => t + 1);
  const isMobile = useIsMobile();

  return (
    <div
      style={{
        maxWidth: 1100,
        margin: "0 auto",
        padding: isMobile ? "1rem 0.6rem" : "1.5rem 2rem",
        fontFamily: theme.sans,
        color: theme.text,
        minHeight: "100vh",
      }}
    >
      {/* Topbar */}
      <header
        style={{
          display: "flex",
          flexDirection: isMobile ? "column" : "row",
          justifyContent: "space-between",
          alignItems: isMobile ? "stretch" : "center",
          gap: isMobile ? "0.85rem" : 0,
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
          <a
            href="https://moav.sh"
            target="_blank"
            rel="noopener noreferrer"
            style={{
              marginLeft: 8,
              color: theme.textDim,
              fontSize: "0.62rem",
              letterSpacing: "0.15em",
              textTransform: "uppercase",
              textDecoration: "none",
              display: isMobile ? "none" : "inline",
            }}
            title="Mother of all VPNs — moav.sh"
          >
            Mother of all VPNs
          </a>
        </h1>
        <div
          style={{
            display: "flex",
            flexDirection: isMobile ? "column" : "row",
            alignItems: isMobile ? "stretch" : "center",
            gap: isMobile ? "0.55rem" : "0.75rem",
          }}
        >
          {/* status: clock + health */}
          <div
            style={{
              display: "flex",
              alignItems: "center",
              gap: "0.6rem",
              justifyContent: isMobile ? "space-between" : "flex-end",
            }}
          >
            <span style={{ color: theme.textDim, fontFamily: theme.mono, fontSize: "0.7rem" }}>
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
                background: total === 0 ? theme.redDim : healthy === total ? theme.greenDim : theme.yellowDim,
                color: total === 0 ? theme.red : healthy === total ? theme.green : theme.yellow,
                border: `1px solid ${total === 0 ? theme.red : healthy === total ? theme.green : theme.yellow}44`,
              }}
            >
              ● {total === 0 ? "no endpoints" : `${healthy}/${total} healthy`}
            </span>
          </div>
          {/* actions: refresh + apply/restart */}
          <div style={{ display: "flex", alignItems: "center", gap: "0.5rem" }}>
            <button
              onClick={refresh}
              style={{
                ...btn,
                borderColor: theme.blue,
                color: theme.blue,
                flex: isMobile ? 1 : undefined,
                justifyContent: "center",
              }}
              title="Refresh all tabs (re-fetch endpoints / stats / logs in place)"
            >
              ↻ Refresh
            </button>
            <RestartButton fullWidth={isMobile} />
          </div>
        </div>
      </header>

      {/* Tab bar — pills; even grid on mobile, wrapping row on desktop. */}
      <nav
        style={{
          display: isMobile ? "grid" : "flex",
          gridTemplateColumns: isMobile ? "repeat(3, 1fr)" : undefined,
          gap: "0.4rem",
          flexWrap: isMobile ? undefined : "wrap",
          marginBottom: "1.25rem",
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
          padding: isMobile ? "0.85rem 0.7rem" : "1.25rem",
        }}
      >
        {tab === "endpoints" && (
          <>
            <BlockDirectToggle refreshTick={refreshTick} />
            <EndpointTable
              refreshTick={refreshTick}
              onHealthChange={(h, t) => {
                setHealthy(h);
                setTotal(t);
              }}
            />
          </>
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
