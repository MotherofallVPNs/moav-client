import { useEffect, useState } from "react";
import { theme } from "../theme";
import { API_BASE } from "../apiBase";

interface ActiveEndpoint {
  id: string;
  name: string;
  fake_sni: string;
  utls: string;
  spoof_via: string;
}

interface SpoofState {
  enabled: boolean;
  default_fake_sni: string;
  default_utls: string;
  // Server may serialise an empty slice as JSON null — coerce on receive.
  active_endpoints: ActiveEndpoint[] | null;
}

function normalise(d: any): SpoofState {
  return {
    enabled: !!d?.enabled,
    default_fake_sni: d?.default_fake_sni ?? "",
    default_utls: d?.default_utls ?? "chrome",
    active_endpoints: Array.isArray(d?.active_endpoints) ? d.active_endpoints : [],
  };
}

const UTLS_OPTIONS = ["chrome", "firefox", "safari", "ios", "android", "edge", "none"];

export default function SNISpoof() {
  const [state, setState] = useState<SpoofState | null>(null);
  const [saving, setSaving] = useState(false);
  const [changed, setChanged] = useState(false);
  const [applying, setApplying] = useState(false);
  const [toast, setToast] = useState<{ msg: string; ok: boolean } | null>(null);

  useEffect(() => {
    fetch(`${API_BASE}/api/snispoof`)
      .then((r) => r.json())
      .then((d) => setState(normalise(d)))
      .catch(() => {});
  }, []);

  const flash = (msg: string, ok: boolean) => {
    setToast({ msg, ok });
    setTimeout(() => setToast(null), 3000);
  };

  const update = async (patch: Partial<SpoofState>) => {
    if (!state) return;
    const next = { ...state, ...patch };
    setState(next);
    setSaving(true);
    try {
      const r = await fetch(`${API_BASE}/api/snispoof`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(patch),
      });
      const data = await r.json();
      if (!r.ok) {
        flash(`Save failed: ${data?.error ?? r.statusText}`, false);
        return;
      }
      setChanged(true);
      flash(data.note ?? "Saved.", true);
    } catch (e) {
      flash(`Save failed: ${(e as Error).message}`, false);
    } finally {
      setSaving(false);
    }
  };

  // Restart proxy-core so it regenerates sing-box + sni-spoof config with the
  // new settings — no terminal needed. (The sni-spoof *container* still needs
  // `docker compose --profile sni-spoof up -d` once; a restart can't create it.)
  const applyNow = async () => {
    setApplying(true);
    try {
      const r = await fetch(`${API_BASE}/api/sources/reload`, { method: "POST" });
      const data = await r.json().catch(() => ({}));
      flash(data.note ?? (data.ok === false ? "Couldn't restart automatically — run the command." : "Restarting proxy-core…"), data.ok !== false);
    } catch (e) {
      flash(`Apply failed: ${(e as Error).message}`, false);
    } finally {
      setTimeout(() => setApplying(false), 4000);
    }
  };

  if (!state) {
    return <div style={{ color: theme.textDim, fontFamily: theme.mono, fontSize: "0.78rem" }}>loading…</div>;
  }

  return (
    <div>
      <p style={{ margin: "0 0 0.75rem", color: theme.textDim, fontSize: "0.8rem", lineHeight: 1.5 }}>
        Inserts a decoy <code>ClientHello</code> on the wire before the real TLS handshake, so DPI
        sees the fake SNI. Useful for Trojan-TLS, VLESS+TLS, and VLESS+WS+TLS endpoints. Reality
        is auto-excluded (its handshake auth breaks when the first CH is faked). Requires the{" "}
        <code>--profile sni-spoof</code> docker-compose service.
      </p>

      <label
        style={{
          display: "flex",
          alignItems: "center",
          gap: "0.65rem",
          padding: "0.55rem 0.75rem",
          border: `1px solid ${state.enabled ? theme.green : theme.border}`,
          background: state.enabled ? theme.greenDim : theme.surface2,
          borderRadius: 6,
          cursor: saving ? "wait" : "pointer",
          marginBottom: "0.65rem",
        }}
      >
        <input
          type="checkbox"
          checked={state.enabled}
          disabled={saving}
          onChange={(e) => update({ enabled: e.target.checked })}
        />
        <span style={{ fontFamily: theme.mono, fontSize: "0.85rem", color: theme.text, fontWeight: 600 }}>
          enable SNI spoofing
        </span>
      </label>

      {changed && (
        <div
          style={{
            marginBottom: "0.65rem",
            padding: "0.7rem 0.8rem",
            border: `1px solid ${theme.yellow}66`,
            borderLeft: `3px solid ${theme.yellow}`,
            borderRadius: 6,
            background: theme.surface2,
          }}
        >
          <div style={{ fontSize: "0.76rem", color: theme.text, marginBottom: "0.5rem", lineHeight: 1.5 }}>
            Saved. Restart proxy-core to regenerate the tunnel config:
          </div>
          <div style={{ display: "flex", gap: "0.5rem", flexWrap: "wrap", alignItems: "center", marginBottom: "0.55rem" }}>
            <button
              onClick={applyNow}
              disabled={applying}
              style={{
                padding: "0.4rem 0.85rem",
                background: theme.yellow,
                color: theme.bg,
                border: "none",
                borderRadius: 4,
                cursor: applying ? "wait" : "pointer",
                fontFamily: theme.mono,
                fontSize: "0.72rem",
                fontWeight: 600,
              }}
            >
              {applying ? "restarting…" : "↻ Apply now (restart proxy-core)"}
            </button>
          </div>
          <div style={{ fontSize: "0.7rem", color: theme.textDim, lineHeight: 1.5 }}>
            First-time only: the spoofer runs as a profiled sidecar, so it also needs{" "}
            <code style={{ color: theme.blue }}>docker compose --profile sni-spoof up -d sni-spoof</code>{" "}
            to exist. Enabling spoofing without that container running will break the affected endpoints.
          </div>
        </div>
      )}

      <div
        style={{
          display: "grid",
          gridTemplateColumns: "auto 1fr",
          gap: "0.4rem 0.75rem",
          alignItems: "center",
          marginBottom: "0.75rem",
        }}
      >
        <label style={lbl}>default fake SNI</label>
        <input
          type="text"
          value={state.default_fake_sni}
          onChange={(e) => setState({ ...state, default_fake_sni: e.target.value })}
          onBlur={() => update({ default_fake_sni: state.default_fake_sni })}
          placeholder="hcaptcha.com  (applied to endpoints without their own fake_sni)"
          style={input}
        />

        <label style={lbl}>default uTLS fingerprint</label>
        <select
          value={state.default_utls}
          onChange={(e) => update({ default_utls: e.target.value })}
          style={input}
        >
          {UTLS_OPTIONS.map((u) => (
            <option key={u} value={u}>
              {u}
            </option>
          ))}
        </select>
      </div>

      <div style={{ fontSize: "0.72rem", color: theme.textDim, fontFamily: theme.mono, marginBottom: "0.75rem" }}>
        Per-endpoint overrides go in <code>config.yaml</code> under the endpoint's <code>Config.fake_sni</code>{" "}
        / <code>Config.utls</code>. Run <code>POST /api/sources/reload</code> after changes.
      </div>

      <div
        style={{
          border: `1px solid ${theme.border}`,
          borderRadius: 6,
          background: theme.surface2,
          padding: "0.6rem 0.75rem",
        }}
      >
        <div style={{ fontSize: "0.72rem", color: theme.textDim, fontFamily: theme.mono, marginBottom: 6 }}>
          currently routed through the spoofer: {(state.active_endpoints ?? []).length}
        </div>
        {(state.active_endpoints ?? []).length === 0 ? (
          <div style={{ fontSize: "0.78rem", color: theme.textDim }}>
            no endpoints active — enable above, set a fake SNI, then{" "}
            <code>./moav-client restart</code> or POST <code>/api/sources/reload</code>.
          </div>
        ) : (
          <table style={{ width: "100%", borderCollapse: "collapse", fontSize: "0.78rem" }}>
            <tbody>
              {(state.active_endpoints ?? []).map((ep) => (
                <tr key={ep.id} style={{ borderTop: `1px solid ${theme.border}` }}>
                  <td style={{ padding: "0.3rem 0", fontFamily: theme.mono, color: theme.text }}>{ep.name || ep.id}</td>
                  <td style={{ padding: "0.3rem 0", fontFamily: theme.mono, color: theme.blue }}>{ep.fake_sni}</td>
                  <td style={{ padding: "0.3rem 0", fontFamily: theme.mono, color: theme.textDim }}>{ep.utls}</td>
                  <td style={{ padding: "0.3rem 0", fontFamily: theme.mono, color: theme.green, textAlign: "right" }}>
                    → {ep.spoof_via}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
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
            maxWidth: 440,
          }}
        >
          {toast.msg}
        </div>
      )}
    </div>
  );
}

const lbl: React.CSSProperties = { fontSize: "0.72rem", color: theme.textDim, fontFamily: theme.mono };
const input: React.CSSProperties = {
  padding: "0.4rem 0.55rem",
  borderRadius: 4,
  fontSize: "0.82rem",
  fontFamily: theme.mono,
};
