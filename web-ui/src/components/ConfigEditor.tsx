import { useEffect, useState } from "react";

const API_BASE = (import.meta.env.VITE_API_URL as string | undefined) ?? "http://localhost:8088";

type ToastType = "success" | "error" | null;

export default function ConfigEditor() {
  const [value, setValue] = useState("");
  const [toast, setToast] = useState<{ msg: string; type: ToastType }>({ msg: "", type: null });

  useEffect(() => {
    fetch(`${API_BASE}/api/config`)
      .then((r) => r.json())
      .then((data) => {
        // If the API returns a YAML string under a "yaml" key, use it; otherwise stringify.
        if (typeof data === "string") {
          setValue(data);
        } else if (data.yaml && typeof data.yaml === "string") {
          setValue(data.yaml);
        } else {
          setValue(JSON.stringify(data, null, 2));
        }
      })
      .catch(() => {
        setValue("# Could not load config from API.");
      });
  }, []);

  const showToast = (msg: string, type: ToastType) => {
    setToast({ msg, type });
    setTimeout(() => setToast({ msg: "", type: null }), 3000);
  };

  const handleSave = async () => {
    try {
      const res = await fetch(`${API_BASE}/api/config`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ yaml: value }),
      });
      if (res.ok) {
        showToast("Config saved.", "success");
      } else {
        showToast(`Save failed: ${res.status} ${res.statusText}`, "error");
      }
    } catch {
      showToast("Could not reach proxy-core API.", "error");
    }
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
          minHeight: 220,
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
        {toast.type && (
          <span
            style={{
              fontSize: "0.875rem",
              color: toast.type === "success" ? "#16a34a" : "#dc2626",
            }}
          >
            {toast.msg}
          </span>
        )}
      </div>
    </div>
  );
}
