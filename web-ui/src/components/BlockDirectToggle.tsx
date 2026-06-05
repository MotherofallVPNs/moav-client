import { useEffect, useState } from "react";
import { theme } from "../theme";
import { API_BASE } from "../apiBase";

interface Props {
  refreshTick?: number;
}

// Kill-switch banner shown above the endpoint table. When on, any connection
// that would go direct (a `direct` rule or the all-endpoints-down fallback) is
// dropped instead. Applies live via PUT /api/block-direct (no restart).
export default function BlockDirectToggle({ refreshTick }: Props) {
  const [enabled, setEnabled] = useState<boolean | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    fetch(`${API_BASE}/api/block-direct`)
      .then((r) => r.json())
      .then((d) => setEnabled(!!d.enabled))
      .catch(() => setEnabled(false));
  }, [refreshTick]);

  const toggle = async () => {
    if (enabled === null || busy) return;
    const next = !enabled;
    setBusy(true);
    setEnabled(next); // optimistic
    try {
      const r = await fetch(`${API_BASE}/api/block-direct`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ enabled: next }),
      });
      const d = await r.json();
      setEnabled(!!d.enabled);
    } catch {
      setEnabled(!next); // revert on failure
    } finally {
      setBusy(false);
    }
  };

  const on = enabled === true;
  const accent = on ? theme.red : theme.textDim;

  return (
    <div
      style={{
        display: "flex",
        alignItems: "center",
        gap: "0.75rem",
        padding: "0.6rem 0.85rem",
        marginBottom: "0.85rem",
        background: theme.surface2,
        border: `1px solid ${on ? theme.red + "66" : theme.border}`,
        borderLeft: `3px solid ${accent}`,
        borderRadius: 6,
      }}
    >
      <button
        role="switch"
        aria-checked={on}
        onClick={toggle}
        disabled={enabled === null || busy}
        style={{
          width: 36,
          height: 20,
          borderRadius: 999,
          border: "none",
          padding: 2,
          background: on ? theme.red : theme.border,
          cursor: enabled === null || busy ? "wait" : "pointer",
          display: "inline-flex",
          alignItems: "center",
          transition: "background 0.15s ease",
          flexShrink: 0,
        }}
      >
        <span
          style={{
            width: 14,
            height: 14,
            borderRadius: "50%",
            background: "#fff",
            transform: on ? "translateX(16px)" : "translateX(0)",
            transition: "transform 0.15s ease",
          }}
        />
      </button>
      <div style={{ display: "flex", flexDirection: "column", gap: 1 }}>
        <span
          style={{
            fontFamily: theme.mono,
            fontSize: "0.82rem",
            fontWeight: 600,
            color: on ? theme.red : theme.text,
          }}
        >
          Block direct traffic {on ? "— ON (kill-switch active)" : ""}
        </span>
        <span style={{ fontSize: "0.7rem", color: theme.textDim }}>
          {on
            ? "Nothing leaves the host unproxied. Disables direct routing rules (e.g. lan-direct) too."
            : "Off: when all endpoints are down, traffic falls back to a direct (unproxied) dial."}
        </span>
      </div>
    </div>
  );
}
