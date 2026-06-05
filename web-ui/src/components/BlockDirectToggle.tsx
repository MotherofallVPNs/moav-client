import { useEffect, useState } from "react";
import { theme } from "../theme";
import { API_BASE } from "../apiBase";

interface Props {
  refreshTick?: number;
}

interface Rule {
  match: { type: string; value: string };
  action: string;
  enabled: boolean;
}

// Kill-switch banner above the endpoint table. When on, the balancer's
// involuntary direct fallback (all endpoints down) is dropped instead of
// dialing direct. Explicit `direct` routing rules are still honored — so when
// any are enabled we name them, since that traffic still bypasses the proxy.
export default function BlockDirectToggle({ refreshTick }: Props) {
  const [enabled, setEnabled] = useState<boolean | null>(null);
  const [busy, setBusy] = useState(false);
  const [directRules, setDirectRules] = useState<string[]>([]);

  useEffect(() => {
    fetch(`${API_BASE}/api/block-direct`)
      .then((r) => r.json())
      .then((d) => setEnabled(!!d.enabled))
      .catch(() => setEnabled(false));
    fetch(`${API_BASE}/api/plugins`)
      .then((r) => r.json())
      .then((d) => {
        const rules: Rule[] = d.rules ?? [];
        setDirectRules(
          rules
            .filter((r) => r.enabled && r.action === "direct")
            .map((r) => `${r.match.type}:${r.match.value}`)
        );
      })
      .catch(() => setDirectRules([]));
  }, [refreshTick]);

  const toggle = async () => {
    if (enabled === null || busy) return;
    const next = !enabled;
    setBusy(true);
    setEnabled(next);
    try {
      const r = await fetch(`${API_BASE}/api/block-direct`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ enabled: next }),
      });
      const d = await r.json();
      setEnabled(!!d.enabled);
    } catch {
      setEnabled(!next);
    } finally {
      setBusy(false);
    }
  };

  const on = enabled === true;
  const accent = on ? theme.red : theme.textDim;
  const leak = on && directRules.length > 0;

  return (
    <div
      style={{
        padding: "0.6rem 0.85rem",
        marginBottom: "0.85rem",
        background: theme.surface2,
        border: `1px solid ${on ? theme.red + "66" : theme.border}`,
        borderLeft: `3px solid ${accent}`,
        borderRadius: 6,
      }}
    >
      <div style={{ display: "flex", alignItems: "center", gap: "0.75rem" }}>
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
              ? "If every endpoint is down, traffic is dropped instead of dialing direct. Explicit direct rules below are still honored."
              : "Off: if every endpoint is down, traffic falls back to a direct (unproxied) dial."}
          </span>
        </div>
      </div>

      {leak && (
        <div
          style={{
            marginTop: "0.5rem",
            padding: "0.4rem 0.55rem",
            background: theme.yellowDim,
            border: `1px solid ${theme.yellow}55`,
            borderRadius: 4,
            fontSize: "0.72rem",
            color: theme.text,
            fontFamily: theme.mono,
          }}
        >
          <span style={{ color: theme.yellow, fontWeight: 600 }}>⚠ heads-up:</span>{" "}
          {directRules.length} enabled <code>direct</code> rule
          {directRules.length > 1 ? "s" : ""} still send matching traffic
          unproxied — {directRules.join(", ")}. Disable them in the Plugins tab
          for a strict no-direct policy.
        </div>
      )}
    </div>
  );
}
