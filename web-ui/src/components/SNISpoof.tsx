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
  active_endpoints: ActiveEndpoint[];
}

const UTLS_OPTIONS = ["chrome", "firefox", "safari", "ios", "android", "edge", "none"];

export default function SNISpoof() {
  const [state, setState] = useState<SpoofState | null>(null);
  const [saving, setSaving] = useState(false);
  const [toast, setToast] = useState<{ msg: string; ok: boolean } | null>(null);

  useEffect(() => {
    fetch(`${API_BASE}/api/snispoof`)
      .then((r) => r.json())
      .then((d) => setState(d as SpoofState))
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
      flash(data.note ?? "Saved.", true);
    } catch (e) {
      flash(`Save failed: ${(e as Error).message}`, false);
    } finally {
      setSaving(false);
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
          currently routed through the spoofer: {state.active_endpoints.length}
        </div>
        {state.active_endpoints.length === 0 ? (
          <div style={{ fontSize: "0.78rem", color: theme.textDim }}>
            no endpoints active — enable above, set a fake SNI, then{" "}
            <code>./moav-client restart</code> or POST <code>/api/sources/reload</code>.
          </div>
        ) : (
          <table style={{ width: "100%", borderCollapse: "collapse", fontSize: "0.78rem" }}>
            <tbody>
              {state.active_endpoints.map((ep) => (
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
