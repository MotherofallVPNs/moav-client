import { useState } from "react";

const API_BASE = (import.meta.env.VITE_API_URL as string | undefined) ?? "http://localhost:8088";

export default function ProbeButton() {
  const [probing, setProbing] = useState(false);
  const [status, setStatus] = useState<string | null>(null);

  const handleProbe = async () => {
    setProbing(true);
    setStatus("Probing… results will appear in the table above via WebSocket.");
    try {
      const res = await fetch(`${API_BASE}/api/probe`, { method: "POST" });
      if (!res.ok) {
        setStatus(`Error: ${res.status} ${res.statusText}`);
        setProbing(false);
        return;
      }
      // The table updates automatically via WebSocket; show a brief note then clear.
      setTimeout(() => {
        setProbing(false);
        setStatus(null);
      }, 5000);
    } catch {
      setStatus("Could not reach proxy-core API.");
      setProbing(false);
    }
  };

  return (
    <div style={{ display: "flex", alignItems: "center", gap: "1rem" }}>
      <button
        onClick={handleProbe}
        disabled={probing}
        style={{
          display: "flex",
          alignItems: "center",
          gap: "0.5rem",
          padding: "0.5rem 1.25rem",
          background: probing ? "#93c5fd" : "#3b82f6",
          color: "#fff",
          border: "none",
          borderRadius: 6,
          cursor: probing ? "not-allowed" : "pointer",
          fontWeight: 500,
          transition: "background 0.2s",
        }}
      >
        {probing && (
          <span
            style={{
              display: "inline-block",
              width: 14,
              height: 14,
              border: "2px solid #fff",
              borderTopColor: "transparent",
              borderRadius: "50%",
              animation: "spin 0.7s linear infinite",
            }}
          />
        )}
        Run probe now
      </button>
      {status && (
        <span style={{ color: "#475569", fontSize: "0.875rem" }}>{status}</span>
      )}
      <style>{`@keyframes spin { to { transform: rotate(360deg); } }`}</style>
    </div>
  );
}
