import EndpointTable from "./components/EndpointTable";
import ProbeButton from "./components/ProbeButton";
import ConfigEditor from "./components/ConfigEditor";

const styles: Record<string, React.CSSProperties> = {
  app: {
    fontFamily: "system-ui, sans-serif",
    maxWidth: 960,
    margin: "0 auto",
    padding: "2rem 1rem",
  },
  header: {
    display: "flex",
    alignItems: "center",
    gap: "1rem",
    marginBottom: "2rem",
  },
  grid: {
    display: "grid",
    gap: "1.5rem",
    gridTemplateColumns: "1fr",
  },
  panel: {
    border: "1px solid #e2e8f0",
    borderRadius: 8,
    padding: "1.25rem",
  },
  panelTitle: {
    margin: "0 0 1rem",
    fontSize: "1rem",
    fontWeight: 600,
    color: "#334155",
  },
};

export default function App() {
  return (
    <div style={styles.app}>
      <header style={styles.header}>
        <h1 style={{ margin: 0, fontSize: "1.5rem" }}>MoaV Client</h1>
        <span style={{ color: "#64748b", fontSize: "0.875rem" }}>
          multi-protocol proxy dashboard
        </span>
      </header>

      <div style={styles.grid}>
        <div style={styles.panel}>
          <h2 style={styles.panelTitle}>Endpoints</h2>
          <EndpointTable />
        </div>

        <div style={styles.panel}>
          <h2 style={styles.panelTitle}>Probe</h2>
          <ProbeButton />
        </div>

        <div style={styles.panel}>
          <h2 style={styles.panelTitle}>Config</h2>
          <ConfigEditor />
        </div>
      </div>
    </div>
  );
}
