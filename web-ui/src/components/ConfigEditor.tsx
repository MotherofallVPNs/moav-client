import { useEffect, useState } from "react";
import { theme } from "../theme";
import { API_BASE, WS_BASE } from "../apiBase";


interface ConfigResp {
  path: string;
  yaml: string;
}

interface Props {
  refreshTick?: number;
}

export default function ConfigEditor({ refreshTick }: Props) {
  const [value, setValue] = useState("");
  const [originalValue, setOriginalValue] = useState("");
  const [path, setPath] = useState("");
  const [toast, setToast] = useState<{ msg: string; ok: boolean } | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    setLoading(true);
    fetch(`${API_BASE}/api/config`)
      .then((r) => {
        if (!r.ok) throw new Error(`${r.status} ${r.statusText}`);
        return r.json();
      })
      .then((data: ConfigResp) => {
        setValue(data.yaml ?? "");
        setOriginalValue(data.yaml ?? "");
        setPath(data.path ?? "config.yaml");
      })
      .catch((e) => {
        setValue(`# Could not load config: ${(e as Error).message}`);
      })
      .finally(() => setLoading(false));
  }, [refreshTick]);

  const flash = (msg: string, ok: boolean) => {
    setToast({ msg, ok });
    setTimeout(() => setToast(null), 3000);
  };

  const dirty = value !== originalValue;

  const handleSave = async () => {
    try {
      const res = await fetch(`${API_BASE}/api/config`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ yaml: value }),
      });
      if (!res.ok) {
        flash(`Save failed: ${res.status} ${res.statusText}`, false);
        return;
      }
      const data = await res.json();
      setOriginalValue(value);
      flash(data.note ?? "Saved.", true);
    } catch {
      flash("Could not reach proxy-core API.", false);
    }
  };

  const handleRevert = () => setValue(originalValue);

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "0.75rem" }}>
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
        <div style={{ fontFamily: theme.mono, fontSize: "0.78rem", color: theme.textDim }}>
          editing <span style={{ color: theme.blue }}>{path || "config.yaml"}</span>
          {dirty && <span style={{ color: theme.yellow, marginLeft: 6 }}>● unsaved</span>}
        </div>
        <div style={{ fontFamily: theme.mono, fontSize: "0.7rem", color: theme.textDim }}>
          {loading ? "loading…" : `${value.split("\n").length} lines · ${value.length} chars`}
        </div>
      </div>

      <textarea
        value={value}
        onChange={(e) => setValue(e.target.value)}
        spellCheck={false}
        style={{
          fontFamily: theme.mono,
          fontSize: "0.8rem",
          padding: "0.75rem",
          borderRadius: 6,
          minHeight: 380,
          resize: "vertical",
          lineHeight: 1.5,
          background: theme.bg,
        }}
      />

      <div style={{ display: "flex", gap: "0.5rem", alignItems: "center" }}>
        <button
          onClick={handleSave}
          disabled={!dirty}
          style={{
            padding: "0.4rem 0.9rem",
            background: dirty ? theme.green : "transparent",
            color: dirty ? theme.bg : theme.textDim,
            border: `1px solid ${dirty ? theme.green : theme.border}`,
            borderRadius: 4,
            cursor: dirty ? "pointer" : "not-allowed",
            fontFamily: theme.mono,
            fontSize: "0.72rem",
            fontWeight: 600,
            textTransform: "uppercase",
            letterSpacing: "0.04em",
          }}
        >
          save
        </button>
        <button
          onClick={handleRevert}
          disabled={!dirty}
          style={{
            padding: "0.4rem 0.9rem",
            background: "transparent",
            color: dirty ? theme.text : theme.textDim,
            border: `1px solid ${theme.border}`,
            borderRadius: 4,
            cursor: dirty ? "pointer" : "not-allowed",
            fontFamily: theme.mono,
            fontSize: "0.72rem",
            fontWeight: 600,
            textTransform: "uppercase",
            letterSpacing: "0.04em",
          }}
        >
          revert
        </button>
        <div style={{ marginLeft: "auto", color: theme.textDim, fontSize: "0.72rem" }}>
          Save writes to disk. Structural changes need <code>docker compose restart proxy-core</code> to take effect.
        </div>
      </div>

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
            maxWidth: 420,
          }}
        >
          {toast.msg}
        </div>
      )}
    </div>
  );
}
