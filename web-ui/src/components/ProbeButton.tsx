// TODO Phase 4: POST /api/probe and show live result.

import { useState } from "react";

export default function ProbeButton() {
  const [status, setStatus] = useState<string | null>(null);

  const handleProbe = async () => {
    setStatus("probing…");
    try {
      const apiUrl = import.meta.env.VITE_API_URL ?? "http://localhost:8088";
      const res = await fetch(`${apiUrl}/api/probe`, { method: "POST" });
      setStatus(res.ok ? "Probe triggered." : `Error: ${res.status}`);
    } catch {
      setStatus("Could not reach proxy-core API.");
    }
  };

  return (
    <div style={{ display: "flex", alignItems: "center", gap: "1rem" }}>
      <button
        onClick={handleProbe}
        style={{
          padding: "0.5rem 1.25rem",
          background: "#3b82f6",
          color: "#fff",
          border: "none",
          borderRadius: 6,
          cursor: "pointer",
          fontWeight: 500,
        }}
      >
        Run probe now
      </button>
      {status && <span style={{ color: "#475569", fontSize: "0.875rem" }}>{status}</span>}
    </div>
  );
}
