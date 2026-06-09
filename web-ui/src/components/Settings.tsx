import { useEffect, useRef, useState } from "react";
import { theme } from "../theme";
import { API_BASE } from "../apiBase";
import { copyText } from "../clipboard";
import { useIsMobile } from "../useIsMobile";
import SNISpoof from "./SNISpoof";
import ConfigEditor from "./ConfigEditor";

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
  const [authUser, setAuthUser] = useState("moav");
  const [authPass, setAuthPass] = useState("");
  const [authSet, setAuthSet] = useState(false); // proxy password already on file
  const [authEnabled, setAuthEnabled] = useState(false); // require proxy auth
  const [showAuthPass, setShowAuthPass] = useState(false);
  // Dashboard/API admin auth — separate from the proxy creds above.
  const [dashUser, setDashUser] = useState("moav");
  const [dashPass, setDashPass] = useState("");
  const [dashSet, setDashSet] = useState(false); // a password is already on file
  const [dashEnabled, setDashEnabled] = useState(false); // require dashboard login
  const [showDashPass, setShowDashPass] = useState(false);
  // True when the API returned the real (not masked) stored passwords — only
  // happens once the dashboard itself is auth-protected.
  const [revealed, setRevealed] = useState(false);
  const [exposureSaving, setExposureSaving] = useState(false);
  const [savedBanner, setSavedBanner] = useState(false);
  const [applying, setApplying] = useState(false);
  // Bumped after a successful save so the ACCESS & URLS panel re-fetches the
  // new exposure mode instead of showing the stale one.
  const [infoTick, setInfoTick] = useState(0);
  // True once /api/exposure has resolved. The exposure section is hidden until
  // then so it doesn't paint the loopback default and reflow when the real
  // (e.g. lan) mode arrives and the auth panels appear.
  const [expoLoaded, setExpoLoaded] = useState(false);

  useEffect(() => {
    fetch(`${API_BASE}/api/exposure`)
      .then((r) => r.json())
      .then((d) => {
        if (d.exposure) setExposure(d.exposure as Exposure);
        if (d.auth_username) setAuthUser(d.auth_username);
        if (d.dashboard_user) setDashUser(d.dashboard_user);
        setAuthSet(!!d.auth_set);
        setDashSet(!!d.dashboard_set);
        setAuthEnabled(!!d.auth_set);
        setDashEnabled(!!d.dashboard_set);
        // Only prefill the fields when the API returned real (revealed)
        // secrets — otherwise the values are masked dots and we must NOT echo
        // them back into the inputs (that would save the dots as the password).
        if (d.secrets_revealed) {
          setRevealed(true);
          if (d.auth_password) setAuthPass(d.auth_password);
          if (d.dashboard_pass) setDashPass(d.dashboard_pass);
        }
      })
      .catch(() => {})
      .finally(() => setExpoLoaded(true));
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
          dashboard: { username: dashUser, password: dashPass },
          auth_enabled: authEnabled,
          dashboard_enabled: dashEnabled,
        }),
      });
      const data = await res.json();
      if (!res.ok) {
        flash(`Failed: ${data?.error || res.statusText}`, false);
        return;
      }
      setAuthSet(authEnabled && (authSet || !!authPass));
      setDashSet(dashEnabled && (dashSet || !!dashPass));
      setSavedBanner(true);
      setInfoTick((t) => t + 1);
      flash("Saved to .env.", true);
    } catch (e) {
      flash(`Could not save: ${(e as Error).message}`, false);
    } finally {
      setExposureSaving(false);
    }
  };

  // Restart proxy-core (and web-ui) via the API so auth / config changes from
  // the dashboard take effect — no terminal needed. (A loopback↔LAN bind
  // change still needs a full recreate; see the banner note.)
  const applyNow = async () => {
    setApplying(true);
    try {
      const res = await fetch(`${API_BASE}/api/exposure/apply`, { method: "POST" });
      const data = await res.json().catch(() => ({}));
      flash(data.note ?? (data.ok === false ? "Couldn't restart automatically — run the command." : "Applying…"), data.ok !== false);
    } catch (e) {
      flash(`Apply failed: ${(e as Error).message}`, false);
    } finally {
      setTimeout(() => setApplying(false), 4000);
    }
  };

  const isMobile = useIsMobile();
  // Two equal columns on desktop, single column on mobile. Panels are carded
  // so the page reads as grouped controls instead of one long scroll.
  const col2: React.CSSProperties = {
    display: "grid",
    gridTemplateColumns: isMobile ? "1fr" : "1fr 1fr",
    gap: "1rem",
    alignItems: "start",
    marginBottom: "1rem",
  };
  const card: React.CSSProperties = {
    border: `1px solid ${theme.border}`,
    borderRadius: 8,
    padding: "1rem 1.1rem",
    marginBottom: 0,
  };

  return (
    <div>
      {/* Row 1: strategy + probe (left)  ·  network exposure (right) */}
      <div style={col2}>
      <div style={card}>
      <section style={{ marginBottom: "1.25rem" }}>
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
      </div>

      <section style={card}>
        <h3 style={section()}>network exposure</h3>
        <p style={blurb()}>
          Controls which interfaces the SOCKS5 / HTTP CONNECT / dashboard ports bind to on the
          <strong> host</strong>. Saved to <code>.env</code>; a compose recreate of <code>proxy-core</code> applies it.
        </p>
        {!expoLoaded && (
          <div style={{ color: theme.textDim, fontSize: "0.8rem", padding: "0.75rem 0", fontFamily: theme.mono }}>
            loading current exposure…
          </div>
        )}
        {expoLoaded && (
        <>
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
          <>
            {/* Proxy access — the SOCKS5/HTTP exit. Optional; open by default. */}
            <div style={{ marginTop: "0.75rem", padding: "0.75rem", border: `1px solid ${theme.border}`, borderRadius: 6, background: theme.surface2 }}>
              <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: "0.5rem", marginBottom: 2 }}>
                <div style={{ fontFamily: theme.mono, fontSize: "0.72rem", color: theme.text, fontWeight: 600 }}>
                  1 · Proxy access <span style={{ color: theme.textDim, fontWeight: 400 }}>(SOCKS5 / HTTP)</span>
                </div>
                <AuthEnableToggle on={authEnabled} onChange={setAuthEnabled} />
              </div>
              <div style={{ fontFamily: theme.mono, fontSize: "0.68rem", color: theme.textDim, marginBottom: 8 }}>
                Who may use your VPN exit. Off = open to anyone who can reach the port. Mandatory for public.
              </div>
              {!authEnabled ? (
                <div style={{ ...statusLine(false), color: theme.textDim }}>
                  Auth off — the proxy is open to anyone who can reach the port.
                  {authSet && " Turning this off and saving clears the stored password."}
                </div>
              ) : (
                <>
                  <div style={statusLine(authSet)}>
                    {authSet
                      ? revealed
                        ? "✓ password set (shown below — click show)"
                        : "✓ password set · enable the dashboard login below to reveal it here"
                      : "✗ no password yet — set one below"}
                  </div>
                  <div style={{ display: "grid", gridTemplateColumns: "auto 1fr auto", gap: "0.4rem 0.75rem", alignItems: "center" }}>
                    <span style={{ fontFamily: theme.mono, fontSize: "0.78rem", color: theme.textDim }}>username</span>
                    <input
                      type="text"
                      value={authUser}
                      onChange={(e) => setAuthUser(e.target.value)}
                      placeholder="moav"
                      style={{ padding: "0.35rem 0.55rem", borderRadius: 4, fontFamily: theme.mono, fontSize: "0.82rem", width: "100%", minWidth: 0, boxSizing: "border-box" }}
                    />
                    <span />
                    <span style={{ fontFamily: theme.mono, fontSize: "0.78rem", color: theme.textDim }}>password</span>
                    <PwField
                      value={authPass}
                      onChange={setAuthPass}
                      show={showAuthPass}
                      onToggle={() => setShowAuthPass((s) => !s)}
                      placeholder={authSet && !revealed ? "•••• unchanged — type to replace" : "set a password"}
                    />
                    <button
                      onClick={() => { setAuthPass(randomPassword()); setShowAuthPass(true); }}
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
                </>
              )}
            </div>

            {/* Dashboard & API login — protects this control panel itself. */}
            <div
              style={{
                marginTop: "0.6rem",
                padding: "0.75rem",
                border: `1px solid ${!dashEnabled ? theme.red : theme.border}`,
                borderRadius: 6,
                background: theme.surface2,
              }}
            >
              <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: "0.5rem", marginBottom: 2 }}>
                <div style={{ fontFamily: theme.mono, fontSize: "0.72rem", color: theme.text, fontWeight: 600 }}>
                  2 · Dashboard &amp; API login <span style={{ color: theme.textDim, fontWeight: 400 }}>(admin)</span>
                </div>
                <AuthEnableToggle on={dashEnabled} onChange={setDashEnabled} />
              </div>
              <div style={{ fontFamily: theme.mono, fontSize: "0.68rem", color: theme.textDim, marginBottom: 8 }}>
                Protects this dashboard + API. Without it, anyone on the network can view endpoints and toggle your proxy.
              </div>
              {!dashEnabled ? (
                <div style={{ ...statusLine(false), color: theme.red }}>
                  ⚠ Login off — anyone on the network can open this panel.
                  {dashSet && " Turning this off and saving clears the stored password."}
                </div>
              ) : (
                <>
                  <div style={statusLine(dashSet)}>
                    {dashSet
                      ? revealed
                        ? "✓ dashboard login set (shown below — click show). Leave blank to keep."
                        : "✓ dashboard login set. Leave blank to keep; type a new one to change."
                      : "✗ no password yet — set one below."}
                  </div>
                  <div style={{ display: "grid", gridTemplateColumns: "auto 1fr auto", gap: "0.4rem 0.75rem", alignItems: "center" }}>
                    <span style={{ fontFamily: theme.mono, fontSize: "0.78rem", color: theme.textDim }}>username</span>
                    <input
                      type="text"
                      value={dashUser}
                      onChange={(e) => setDashUser(e.target.value)}
                      placeholder="moav"
                      style={{ padding: "0.35rem 0.55rem", borderRadius: 4, fontFamily: theme.mono, fontSize: "0.82rem", width: "100%", minWidth: 0, boxSizing: "border-box" }}
                    />
                    <span />
                    <span style={{ fontFamily: theme.mono, fontSize: "0.78rem", color: theme.textDim }}>password</span>
                    <PwField
                      value={dashPass}
                      onChange={setDashPass}
                      show={showDashPass}
                      onToggle={() => setShowDashPass((s) => !s)}
                      placeholder={dashSet && !revealed ? "•••• unchanged — type to replace" : "set a password"}
                    />
                    <button
                      onClick={() => { setDashPass(randomPassword()); setShowDashPass(true); }}
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
                </>
              )}
            </div>
          </>
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

        {savedBanner && (
          <div
            style={{
              marginTop: "0.85rem",
              padding: "0.75rem 0.85rem",
              border: `1px solid ${theme.yellow}66`,
              borderLeft: `3px solid ${theme.yellow}`,
              borderRadius: 6,
              background: theme.surface2,
            }}
          >
            <div style={{ fontSize: "0.78rem", color: theme.text, marginBottom: "0.55rem", lineHeight: 1.5 }}>
              Saved to <code>.env</code>. To apply:
            </div>
            <div style={{ display: "flex", gap: "0.5rem", flexWrap: "wrap", alignItems: "center", marginBottom: "0.6rem" }}>
              <button onClick={applyNow} disabled={applying} style={applyBtn(applying)}>
                {applying ? "restarting…" : "↻ Apply now (restart dashboard + proxy)"}
              </button>
              <span style={{ fontSize: "0.7rem", color: theme.textDim }}>applies auth / password changes</span>
            </div>
            <div style={{ fontSize: "0.72rem", color: theme.textDim, lineHeight: 1.5 }}>
              If you just enabled a dashboard password you may be asked to log in after it restarts. Changing the{" "}
              <strong>loopback ↔ LAN/public</strong> mode rebinds the ports, which needs a full recreate (the
              restart above can't do that):
            </div>
            <div style={{ display: "flex", gap: "0.4rem", alignItems: "center", marginTop: "0.4rem", flexWrap: "wrap" }}>
              <code style={{ fontSize: "0.7rem", color: theme.blue, wordBreak: "break-all", flex: 1 }}>
                docker compose up -d --force-recreate proxy-core web-ui
              </code>
              <button
                onClick={async () => { const ok = await copyText("docker compose up -d --force-recreate proxy-core web-ui"); flash(ok ? "command copied" : "copy failed — long-press to select", ok); }}
                style={miniBtn()}
              >
                copy
              </button>
            </div>
          </div>
        )}
        </>
        )}
      </section>
      </div>

      {/* Row 2: access & urls (left)  ·  SNI spoofing (right) */}
      <div style={col2}>
      <section style={card}>
        <h3 style={section()}>access &amp; urls</h3>
        <ConnectionInfo refreshTick={(refreshTick ?? 0) + infoTick} onFlash={flash} />
      </section>

      <section style={card}>
        <h3 style={section()}>SNI spoofing</h3>
        <SNISpoof />
      </section>
      </div>

      <section style={{ ...card, marginBottom: "1rem" }}>
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

      <section style={{ ...card, marginBottom: "1rem" }}>
        <details>
          <summary
            style={{
              cursor: "pointer",
              fontFamily: theme.mono,
              fontSize: "0.78rem",
              color: theme.text,
              textTransform: "uppercase",
              letterSpacing: "0.04em",
              listStyle: "revert",
            }}
          >
            advanced — edit config.yaml
          </summary>
          <p style={{ ...blurb(), marginTop: "0.6rem" }}>
            Raw <code>config.yaml</code> editor. Most settings have a control above — only edit here if
            you know the schema. Structural changes need an <strong>Apply / restart</strong> to take effect.
          </p>
          <ConfigEditor refreshTick={refreshTick} />
        </details>
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

const statusLine = (set: boolean): React.CSSProperties => ({
  fontFamily: theme.mono,
  fontSize: "0.72rem",
  marginBottom: 8,
  color: set ? theme.green : theme.red,
});

// Sliding on/off switch for enabling/disabling an auth section. Styled as a
// real toggle (track + knob + label) so it clearly reads as actionable.
// Off → the section's credentials are cleared on save.
function AuthEnableToggle({ on, onChange }: { on: boolean; onChange: (v: boolean) => void }) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={on}
      onClick={() => onChange(!on)}
      title={on ? "Enabled — click to turn off" : "Disabled — click to turn on"}
      style={{
        display: "inline-flex",
        alignItems: "center",
        gap: 7,
        padding: "0.22rem 0.55rem 0.22rem 0.3rem",
        borderRadius: 999,
        border: `1px solid ${on ? theme.green : theme.border}`,
        background: on ? theme.greenDim : theme.surface2,
        cursor: "pointer",
        whiteSpace: "nowrap",
      }}
    >
      <span
        aria-hidden
        style={{
          position: "relative",
          width: 30,
          height: 16,
          borderRadius: 999,
          background: on ? theme.green : theme.textDim,
          transition: "background 0.15s",
          flexShrink: 0,
        }}
      >
        <span
          style={{
            position: "absolute",
            top: 2,
            left: on ? 16 : 2,
            width: 12,
            height: 12,
            borderRadius: "50%",
            background: "#fff",
            transition: "left 0.15s",
          }}
        />
      </span>
      <span
        style={{
          fontFamily: theme.mono,
          fontSize: "0.66rem",
          fontWeight: 700,
          textTransform: "uppercase",
          letterSpacing: "0.04em",
          color: on ? theme.green : theme.textDim,
        }}
      >
        {on ? "on" : "off"}
      </span>
    </button>
  );
}

// Password input with a show/hide toggle. Default hidden; click "show" to
// reveal what's typed (and, when prefilled from a revealed secret, the stored
// value too).
function PwField({
  value,
  onChange,
  show,
  onToggle,
  placeholder,
}: {
  value: string;
  onChange: (v: string) => void;
  show: boolean;
  onToggle: () => void;
  placeholder: string;
}) {
  return (
    <div style={{ position: "relative", display: "flex", alignItems: "center" }}>
      <input
        type={show ? "text" : "password"}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        style={{
          padding: "0.35rem 3rem 0.35rem 0.55rem",
          borderRadius: 4,
          fontFamily: theme.mono,
          fontSize: "0.82rem",
          width: "100%",
          minWidth: 0,
          boxSizing: "border-box",
        }}
      />
      <button
        type="button"
        onClick={onToggle}
        title={show ? "hide" : "show"}
        style={{
          position: "absolute",
          right: 4,
          background: "none",
          border: "none",
          cursor: "pointer",
          color: theme.textDim,
          fontFamily: theme.mono,
          fontSize: "0.62rem",
          textTransform: "uppercase",
          letterSpacing: "0.04em",
        }}
      >
        {show ? "hide" : "show"}
      </button>
    </div>
  );
}

interface ExposureInfo {
  exposure: string;
  auth_username?: string;
  auth_password?: string;
  auth_set?: boolean;
  dashboard_set?: boolean;
  lan_ip?: string;
}

const MODE_META: Record<string, { label: string; desc: string; color: string }> = {
  loopback: { label: "Loopback", desc: "This machine only — other devices can't reach it.", color: theme.textDim },
  lan: { label: "LAN", desc: "Reachable from devices on your local network.", color: theme.green },
  public: { label: "Public", desc: "Internet-facing — actual reach depends on your firewall / NAT.", color: theme.red },
};

// ConnectionInfo shows the current network-accessibility mode and the URLs to
// reach each service, derived from how you're currently connected (the page's
// hostname). Clears up the common mix-up: the dashboard is :3001, the SOCKS5
// proxy is :1080 (a proxy, not a web page).
function ConnectionInfo({ refreshTick, onFlash }: { refreshTick?: number; onFlash: (m: string, ok: boolean) => void }) {
  const [info, setInfo] = useState<ExposureInfo | null>(null);
  // The proxy's own egress IP — for a VPS / public host this is the address
  // other machines reach it at, so it's a useful fallback when the dashboard
  // is being viewed locally (e.g. over an SSH tunnel) and can't see it.
  const [egressIp, setEgressIp] = useState("");
  useEffect(() => {
    fetch(`${API_BASE}/api/exposure`).then((r) => r.json()).then(setInfo).catch(() => {});
    fetch(`${API_BASE}/api/version`).then((r) => r.json()).then((d) => setEgressIp(d?.direct_ip || "")).catch(() => {});
  }, [refreshTick]);

  const browserHost = (typeof window !== "undefined" && window.location.hostname) || "localhost";
  const isLoopbackHost = browserHost === "localhost" || browserHost.startsWith("127.");
  const mode = info?.exposure ?? "loopback";
  const meta = MODE_META[mode] ?? MODE_META.loopback;
  const hasAuth = !!(info?.auth_set ?? (info?.auth_username || info?.auth_password));
  const dashAuth = !!info?.dashboard_set;

  // Resolve the address to show per exposure mode, entirely from local signals
  // (the browser's own hostname + a backend interface hint) — no external IP
  // lookup. `placeholder` means we genuinely can't know it from here.
  let displayHost = browserHost;
  let hostNote = "";
  let placeholder = false;
  if (mode === "loopback") {
    displayHost = "127.0.0.1";
  } else if (!isLoopbackHost) {
    // You reached this dashboard over a real (non-loopback) address — the proxy
    // is reachable at that exact host too. Covers LAN IPs, public IPs (VPS),
    // and domains without any guessing.
    displayHost = browserHost;
  } else {
    // Viewing locally (same machine or an SSH tunnel), so the browser can't see
    // the external address. Use backend hints: a real LAN IP for `lan`, else
    // the detected egress IP (right for a VPS / public host).
    const usableLanIp = info?.lan_ip && /^(192\.168\.|10\.)/.test(info.lan_ip) ? info.lan_ip : "";
    if (mode === "lan" && usableLanIp) {
      displayHost = usableLanIp;
    } else if (egressIp) {
      displayHost = egressIp;
      placeholder = true;
      hostNote = `Showing this host's detected public IP (${egressIp}) — correct for a VPS / internet-facing host. On a home LAN, other devices instead need this machine's 192.168.x address. Reachable only if the firewall allows the port.`;
    } else {
      displayHost = mode === "lan" ? "<this-machine-LAN-IP>" : "<your-public-IP-or-domain>";
      placeholder = true;
      hostNote =
        mode === "lan"
          ? "You're viewing locally, so the LAN IP isn't visible here — open this page from another device, or check your machine's network settings."
          : "Public reach needs a port-forward on your router; replace this with your public IP or domain.";
    }
  }

  const services = [
    { name: "Dashboard", url: `http://${displayHost}:3001`, hint: dashAuth ? "login required" : "open — no login" },
    { name: "SOCKS5 proxy", url: `socks5h://${displayHost}:1080`, hint: hasAuth ? "auth required" : "no auth set" },
    { name: "HTTP proxy", url: `http://${displayHost}:8081`, hint: "" },
    { name: "REST / WS API", url: `http://${displayHost}:8088`, hint: "" },
  ];

  const copy = async (s: string) => {
    const ok = await copyText(s);
    onFlash(ok ? "copied" : "copy failed — long-press to select", ok);
  };

  return (
    <div>
      <div style={{ display: "flex", alignItems: "center", gap: "0.6rem", marginBottom: "0.7rem", flexWrap: "wrap" }}>
        <span style={{ padding: "2px 9px", borderRadius: 4, fontSize: "0.7rem", fontWeight: 700, fontFamily: theme.mono, textTransform: "uppercase", color: meta.color, border: `1px solid ${meta.color}66`, background: meta.color + "1a" }}>
          {meta.label}
        </span>
        <span style={{ color: theme.textDim, fontSize: "0.74rem" }}>{meta.desc}</span>
      </div>

      <div style={{ border: `1px solid ${theme.border}`, borderRadius: 6, padding: "0.5rem 0.65rem", background: theme.surface2 }}>
        {services.map((s, i) => (
          <div
            key={s.name}
            style={{
              padding: "6px 0",
              borderTop: i === 0 ? "none" : `1px solid ${theme.border}`,
            }}
          >
            {/* Top line: label + copy, so the URL below gets the full width and
                doesn't break mid-string in a cramped column. */}
            <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: "0.5rem" }}>
              <span style={{ fontFamily: theme.mono, fontSize: "0.72rem", color: theme.text }}>{s.name}</span>
              <button onClick={() => copy(s.url)} style={miniBtn()}>copy</button>
            </div>
            <code style={{ display: "block", fontSize: "0.74rem", color: theme.blue, wordBreak: "break-word", marginTop: 2 }}>
              {s.url}
              {s.hint && <span style={{ color: theme.textDim }}> · {s.hint}</span>}
            </code>
          </div>
        ))}
      </div>

      <div style={{ marginTop: "0.7rem", fontSize: "0.72rem", color: theme.textDim, lineHeight: 1.5 }}>
        {hostNote ? (
          <span style={{ color: placeholder ? theme.yellow : theme.textDim }}>{hostNote}</span>
        ) : (
          <>
            The <strong>dashboard is :3001</strong>; point another device's <strong>proxy setting</strong>{" "}
            (not its address bar) at the SOCKS5 address above — :1080 is a proxy, not a web page.
          </>
        )}
        <div style={{ marginTop: 4 }}>
          <code>socks5h://</code> is intentional — the “h” means DNS is resolved at the proxy (no DNS
          leaks). Apps that only take host/port: use just <code>{displayHost}:1080</code>, type SOCKS5.
          These URLs reflect the <strong>saved</strong> mode; the actual port binding changes only after{" "}
          <strong>Apply / recreate</strong>.
        </div>
        {(mode === "lan" || mode === "public") && !dashAuth && (
          <div style={{ color: theme.red, marginTop: 4 }}>
            ⚠ No dashboard login set while exposed — anyone on the network can open this control panel.
            Set one in “Dashboard &amp; API login” above.
          </div>
        )}
        {(mode === "lan" || mode === "public") && !hasAuth && (
          <div style={{ color: theme.yellow, marginTop: 4 }}>
            ⚠ No SOCKS5 auth set while exposed — anyone on the network can use the proxy. Add a
            username/password above.
          </div>
        )}
      </div>
    </div>
  );
}

const miniBtn = (): React.CSSProperties => ({
  padding: "0.2rem 0.5rem",
  background: "transparent",
  color: theme.textDim,
  border: `1px solid ${theme.border}`,
  borderRadius: 4,
  cursor: "pointer",
  fontFamily: theme.mono,
  fontSize: "0.66rem",
});

const applyBtn = (busy: boolean): React.CSSProperties => ({
  padding: "0.4rem 0.85rem",
  background: theme.yellow,
  color: theme.bg,
  border: "none",
  borderRadius: 4,
  cursor: busy ? "wait" : "pointer",
  fontFamily: theme.mono,
  fontSize: "0.72rem",
  fontWeight: 600,
});
