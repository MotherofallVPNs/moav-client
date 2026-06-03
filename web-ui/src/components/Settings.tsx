import { useEffect, useRef, useState } from "react";
import { theme } from "../theme";
import { API_BASE } from "../apiBase";

type Strategy = "latency" | "priority" | "weighted";
type Exposure = "loopback" | "lan" | "public";

const STRATEGY_OPTIONS: { value: Strategy; label: string; help: string }[] = [
  { value: "latency", label: "Latency", help: "Pick the endpoint with the lowest measured RTT through sing-box." },
  { value: "priority", label: "Priority", help: "Pick the endpoint with the lowest Priority field (config-driven)." },
  { value: "weighted", label: "Weighted random", help: "Random pick with weights = Priority field. Spreads load." },
];

interface Props {
  refreshTick?: number;
}

const EXPOSURE_OPTIONS: { value: Exposure; label: string; help: string; warn?: string }[] = [
  {
    value: "loopback",
    label: "Loopback only (default)",
    help: "Bind 127.0.0.1 — only this machine can use the proxy. Safest.",
  },
  {
    value: "lan",
    label: "Local network",
    help: "Bind 0.0.0.0 — every device on your LAN (router, phones, TVs) can point at <this-machine>:1080.",
    warn: "Anyone on your local network can use your moav exit. Set a SOCKS5 username/password below.",
  },
  {
    value: "public",
    label: "Public exposure",
    help: "Same bind as LAN, but you'll port-forward 1080/8081/8088 on your router. Treat as a public proxy.",
    warn: "EVERY internet host that can reach your IP can hit the proxy. AUTHENTICATION IS MANDATORY.",
  },
];

const randomPassword = (n = 16) => {
  const alphabet = "ABCDEFGHJKMNPQRSTUVWXYZabcdefghjkmnpqrstuvwxyz23456789";
  let out = "";
  const arr = new Uint8Array(n);
  (window.crypto || (window as any).msCrypto).getRandomValues(arr);
  for (const x of arr) out += alphabet[x % alphabet.length];
  return out;
};

export default function Settings({ refreshTick }: Props) {
  const [strategy, setStrategy] = useState<Strategy>("latency");
  const [loaded, setLoaded] = useState(false);
  const [probing, setProbing] = useState(false);
  const [toast, setToast] = useState<{ msg: string; ok: boolean } | null>(null);

  // Exposure section state.
  const [exposure, setExposure] = useState<Exposure>("loopback");
  const [authUser, setAuthUser] = useState("");
  const [authPass, setAuthPass] = useState("");
  const [exposureSaving, setExposureSaving] = useState(false);

  useEffect(() => {
    fetch(`${API_BASE}/api/exposure`)
      .then((r) => r.json())
      .then((d) => {
        if (d.exposure) setExposure(d.exposure as Exposure);
        if (d.auth_username) setAuthUser(d.auth_username);
        // We never receive the unmasked password back; show only a hint.
      })
      .catch(() => {});
    fetch(`${API_BASE}/api/stats`)
      .then((r) => r.json())
      .then((data) => {
        if (data?.strategy && STRATEGY_OPTIONS.find((o) => o.value === data.strategy)) {
          setStrategy(data.strategy as Strategy);
        }
        setLoaded(true);
      })
      .catch(() => setLoaded(true));
  }, [refreshTick]);

  const flash = (msg: string, ok: boolean) => {
    setToast({ msg, ok });
    setTimeout(() => setToast(null), 2500);
  };

  const switchStrategy = async (s: Strategy) => {
    const prev = strategy;
    setStrategy(s);
    try {
      const res = await fetch(`${API_BASE}/api/strategy`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ strategy: s }),
      });
      if (!res.ok) {
        setStrategy(prev);
        flash(`Failed: ${res.status} ${res.statusText}`, false);
      } else {
        flash(`strategy → ${s}`, true);
      }
    } catch {
      setStrategy(prev);
      flash("Could not reach proxy-core API.", false);
    }
  };

  const triggerProbe = async () => {
    setProbing(true);
    try {
      const res = await fetch(`${API_BASE}/api/probe`, { method: "POST" });
      if (res.ok) {
        flash("Probing all endpoints…", true);
      } else {
        flash(`Failed: ${res.status} ${res.statusText}`, false);
      }
    } catch {
      flash("Could not reach proxy-core API.", false);
    } finally {
      setTimeout(() => setProbing(false), 4500);
    }
  };

  const saveExposure = async () => {
    setExposureSaving(true);
    try {
      const res = await fetch(`${API_BASE}/api/exposure`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          exposure,
          auth: { username: authUser, password: authPass },
        }),
      });
      const data = await res.json();
      if (!res.ok) {
        flash(`Failed: ${data?.error || res.statusText}`, false);
        return;
      }
      flash(data.note ?? "Exposure saved.", true);
    } catch (e) {
      flash(`Could not save: ${(e as Error).message}`, false);
    } finally {
      setExposureSaving(false);
    }
  };

  return (
    <div>
      <section style={{ marginBottom: "2rem" }}>
        <h3 style={section()}>load-balancing strategy</h3>
        <p style={blurb()}>Applied immediately, no restart required.</p>
        <div style={{ display: "flex", flexDirection: "column", gap: "0.5rem" }}>
          {STRATEGY_OPTIONS.map((opt) => (
            <label
              key={opt.value}
              style={{
                display: "flex",
                gap: "0.75rem",
                padding: "0.75rem",
                border: `1px solid ${strategy === opt.value ? theme.green : theme.border}`,
                borderRadius: 6,
                cursor: loaded ? "pointer" : "not-allowed",
                background: strategy === opt.value ? theme.greenDim : theme.surface2,
              }}
            >
              <input
                type="radio"
                name="strategy"
                value={opt.value}
                checked={strategy === opt.value}
                disabled={!loaded}
                onChange={() => switchStrategy(opt.value)}
                style={{ marginTop: 3 }}
              />
              <div>
                <div style={{ fontFamily: theme.mono, fontWeight: 600, color: theme.text, fontSize: "0.82rem" }}>
                  {opt.label}
                </div>
                <div style={{ color: theme.textDim, fontSize: "0.74rem", marginTop: 2 }}>{opt.help}</div>
              </div>
            </label>
          ))}
        </div>
      </section>

      <section style={{ marginBottom: "1.5rem" }}>
        <h3 style={section()}>actions</h3>
        <button
          onClick={triggerProbe}
          disabled={probing}
          style={{
            display: "inline-flex",
            alignItems: "center",
            gap: "0.5rem",
            padding: "0.45rem 1rem",
            background: probing ? theme.surface2 : "transparent",
            color: theme.blue,
            border: `1px solid ${theme.blue}`,
            borderRadius: 4,
            cursor: probing ? "wait" : "pointer",
            fontFamily: theme.mono,
            fontSize: "0.72rem",
            fontWeight: 600,
            textTransform: "uppercase",
            letterSpacing: "0.04em",
          }}
        >
          {probing && (
            <span
              style={{
                width: 10,
                height: 10,
                border: `2px solid ${theme.blue}`,
                borderTopColor: "transparent",
                borderRadius: "50%",
                animation: "spin 0.7s linear infinite",
              }}
            />
          )}
          probe all endpoints
        </button>
        <style>{`@keyframes spin { to { transform: rotate(360deg); } }`}</style>
        <div style={{ marginTop: "0.4rem", color: theme.textDim, fontSize: "0.72rem" }}>
          Results stream into the <strong>Endpoints</strong> tab via WebSocket — switch tabs to watch them update.
        </div>
      </section>

      <section style={{ marginBottom: "1.5rem" }}>
        <h3 style={section()}>network exposure</h3>
        <p style={blurb()}>
          Controls which interfaces the SOCKS5 / HTTP CONNECT / dashboard ports bind to on the
          <strong> host</strong>. Saved to <code>.env</code>; a compose recreate of <code>proxy-core</code> applies it.
        </p>
        <div style={{ display: "flex", flexDirection: "column", gap: "0.5rem" }}>
          {EXPOSURE_OPTIONS.map((opt) => {
            const active = exposure === opt.value;
            return (
              <label
                key={opt.value}
                style={{
                  display: "flex",
                  gap: "0.75rem",
                  padding: "0.75rem",
                  border: `1px solid ${active ? theme.blue : theme.border}`,
                  borderRadius: 6,
                  cursor: "pointer",
                  background: active ? theme.blueDim : theme.surface2,
                }}
              >
                <input
                  type="radio"
                  name="exposure"
                  value={opt.value}
                  checked={active}
                  onChange={() => setExposure(opt.value)}
                  style={{ marginTop: 3 }}
                />
                <div>
                  <div style={{ fontFamily: theme.mono, fontWeight: 600, color: theme.text, fontSize: "0.82rem" }}>
                    {opt.label}
                  </div>
                  <div style={{ color: theme.textDim, fontSize: "0.74rem", marginTop: 2 }}>{opt.help}</div>
                  {opt.warn && active && (
                    <div style={{ color: theme.red, fontSize: "0.72rem", marginTop: 4, fontFamily: theme.mono }}>
                      ⚠ {opt.warn}
                    </div>
                  )}
                </div>
              </label>
            );
          })}
        </div>

        {exposure !== "loopback" && (
          <div style={{ marginTop: "0.75rem", padding: "0.75rem", border: `1px solid ${theme.border}`, borderRadius: 6, background: theme.surface2 }}>
            <div style={{ fontFamily: theme.mono, fontSize: "0.72rem", color: theme.textDim, marginBottom: 6 }}>
              SOCKS5 authentication (recommended for LAN, mandatory for public)
            </div>
            <div style={{ display: "grid", gridTemplateColumns: "auto 1fr auto", gap: "0.4rem 0.75rem", alignItems: "center" }}>
              <span style={{ fontFamily: theme.mono, fontSize: "0.78rem", color: theme.textDim }}>username</span>
              <input
                type="text"
                value={authUser}
                onChange={(e) => setAuthUser(e.target.value)}
                placeholder="moav"
                style={{ padding: "0.35rem 0.55rem", borderRadius: 4, fontFamily: theme.mono, fontSize: "0.82rem" }}
              />
              <span />
              <span style={{ fontFamily: theme.mono, fontSize: "0.78rem", color: theme.textDim }}>password</span>
              <input
                type="text"
                value={authPass}
                onChange={(e) => setAuthPass(e.target.value)}
                placeholder="•••••••••"
                style={{ padding: "0.35rem 0.55rem", borderRadius: 4, fontFamily: theme.mono, fontSize: "0.82rem" }}
              />
              <button
                onClick={() => setAuthPass(randomPassword())}
                style={{
                  padding: "0.35rem 0.7rem",
                  background: "transparent",
                  border: `1px solid ${theme.border}`,
                  borderRadius: 4,
                  fontFamily: theme.mono,
                  fontSize: "0.7rem",
                  color: theme.textDim,
                  cursor: "pointer",
                }}
              >
                generate
              </button>
            </div>
          </div>
        )}

        <button
          onClick={saveExposure}
          disabled={exposureSaving}
          style={{
            marginTop: "0.75rem",
            padding: "0.45rem 1rem",
            background: theme.blue,
            color: theme.bg,
            border: "none",
            borderRadius: 4,
            cursor: exposureSaving ? "wait" : "pointer",
            fontFamily: theme.mono,
            fontSize: "0.72rem",
            fontWeight: 600,
            textTransform: "uppercase",
            letterSpacing: "0.04em",
          }}
        >
          {exposureSaving ? "saving…" : "save exposure"}
        </button>
        <div style={{ marginTop: "0.4rem", color: theme.textDim, fontSize: "0.72rem" }}>
          After saving, run{" "}
          <code>docker compose up -d --force-recreate proxy-core web-ui</code> (or{" "}
          <code>./moav-client restart</code>) to apply the new bind.
        </div>
      </section>

      <section style={{ marginBottom: "1.5rem" }}>
        <h3 style={section()}>backup &amp; restore</h3>
        <p style={blurb()}>
          Download a <code>.zip</code> of <code>config.yaml</code> + <code>data/</code> +{" "}
          <code>.env</code> — everything you need to migrate to another box. Restore overwrites the
          current install; restart proxy-core afterwards.
        </p>
        <div style={{ display: "flex", gap: "0.5rem", alignItems: "center" }}>
          <a
            href={`${API_BASE}/api/backup`}
            style={{
              padding: "0.45rem 1rem",
              background: theme.green,
              color: theme.bg,
              border: "none",
              borderRadius: 4,
              cursor: "pointer",
              fontFamily: theme.mono,
              fontSize: "0.72rem",
              fontWeight: 600,
              textTransform: "uppercase",
              letterSpacing: "0.04em",
              textDecoration: "none",
            }}
          >
            ↓ download backup
          </a>
          <RestoreButton onResult={(ok, msg) => flash(msg, ok)} />
        </div>
      </section>

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
          }}
        >
          {toast.msg}
        </div>
      )}
    </div>
  );
}

const section = (): React.CSSProperties => ({
  margin: "0 0 0.5rem",
  fontFamily: theme.mono,
  fontSize: "0.78rem",
  color: theme.text,
  textTransform: "uppercase" as const,
  letterSpacing: "0.04em",
});
function RestoreButton({ onResult }: { onResult: (ok: boolean, msg: string) => void }) {
  const inputRef = useRef<HTMLInputElement | null>(null);
  return (
    <>
      <input
        ref={inputRef}
        type="file"
        accept=".zip"
        style={{ display: "none" }}
        onChange={async (e) => {
          const f = e.target.files?.[0];
          if (!f) return;
          if (!window.confirm(`Restore from ${f.name}? This overwrites config.yaml + data/.`)) return;
          try {
            const fd = new FormData();
            fd.append("backup", f);
            const r = await fetch(`${API_BASE}/api/restore`, { method: "POST", body: fd });
            const data = await r.json();
            onResult(r.ok, data.note ?? `${data.files_restored} files restored.`);
          } catch (err) {
            onResult(false, `Restore failed: ${(err as Error).message}`);
          } finally {
            e.target.value = "";
          }
        }}
      />
      <button
        onClick={() => inputRef.current?.click()}
        style={{
          padding: "0.45rem 1rem",
          background: "transparent",
          color: theme.yellow,
          border: `1px solid ${theme.yellow}`,
          borderRadius: 4,
          cursor: "pointer",
          fontFamily: theme.mono,
          fontSize: "0.72rem",
          fontWeight: 600,
          textTransform: "uppercase",
          letterSpacing: "0.04em",
        }}
      >
        ↑ restore from zip
      </button>
    </>
  );
}

const blurb = (): React.CSSProperties => ({
  margin: "0 0 0.75rem",
  color: theme.textDim,
  fontSize: "0.78rem",
});
