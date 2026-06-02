// TODO Phase 4: load config.yaml from API and POST edits back.

import { useState } from "react";

const placeholder = `proxy:
  socks5_port: 1080
  http_port: 8080
  api_port: 8088

load_balancing:
  strategy: latency
  probe_on_start: true
`;

export default function ConfigEditor() {
  const [value, setValue] = useState(placeholder);
  const [saved, setSaved] = useState(false);

  const handleSave = () => {
    // TODO: PUT /api/config
    setSaved(true);
    setTimeout(() => setSaved(false), 2000);
  };

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "0.75rem" }}>
      <textarea
        value={value}
        onChange={(e) => setValue(e.target.value)}
        spellCheck={false}
        style={{
          fontFamily: "monospace",
          fontSize: "0.8125rem",
          padding: "0.75rem",
          border: "1px solid #e2e8f0",
          borderRadius: 6,
          minHeight: 200,
          resize: "vertical",
        }}
      />
      <div style={{ display: "flex", alignItems: "center", gap: "1rem" }}>
        <button
          onClick={handleSave}
          style={{
            padding: "0.4rem 1rem",
            background: "#22c55e",
            color: "#fff",
            border: "none",
            borderRadius: 6,
            cursor: "pointer",
            fontWeight: 500,
          }}
        >
          Save
        </button>
        {saved && <span style={{ color: "#16a34a", fontSize: "0.875rem" }}>Saved!</span>}
      </div>
    </div>
  );
}
