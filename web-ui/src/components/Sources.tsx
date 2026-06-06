import { useEffect, useRef, useState } from "react";
import { theme } from "../theme";
import { API_BASE } from "../apiBase";
import { useIsMobile } from "../useIsMobile";

interface Source {
  name: string;
  file?: string;
  url?: string;
  wireguard_files?: string[];
  tags?: string[];
  is_moav?: boolean;
  endpoints: number;
  healthy: number;
}

function MoavBadge() {
  return (
    <span
      style={{
        padding: "1px 7px",
        borderRadius: 4,
        border: `1px solid ${theme.blue}66`,
        background: theme.blue + "1a",
        color: theme.blue,
        fontFamily: theme.mono,
        fontSize: "0.6rem",
        fontWeight: 700,
        textTransform: "uppercase",
        letterSpacing: "0.04em",
      }}
    >
      moav
    </span>
  );
}

// Component tags shown per source (mirrors the MoaV admin bundle tags). Color
// keyed by component; unknown kinds fall back to a neutral chip.
const TAG_COLOR: Record<string, string> = {
  subscription: theme.blue,
  wireguard: "#a78bfa",
  amneziawg: "#fda4af",
  masterdns: "#2dd4bf",
  trusttunnel: "#c084fc",
  psiphon: "#a3e635",
  tor: "#818cf8",
  dnstt: "#facc15",
  slipstream: "#fb923c",
};

function Tags({ tags }: { tags?: string[] }) {
  if (!tags || tags.length === 0) return null;
  return (
    <div style={{ display: "flex", flexWrap: "wrap", gap: 4, marginTop: 5 }}>
      {tags.map((t) => {
        const c = TAG_COLOR[t] ?? theme.textDim;
        return (
          <span
            key={t}
            style={{
              padding: "1px 7px",
              borderRadius: 4,
              border: `1px solid ${c}55`,
              background: c + "1a",
              color: c,
              fontFamily: theme.mono,
              fontSize: "0.64rem",
              letterSpacing: "0.02em",
            }}
          >
            {t}
          </span>
        );
      })}
    </div>
  );
}

interface SourcesResp {
  sources: Source[];
  note?: string;
}

interface UploadResult {
  ok: boolean;
  result?: {
    name: string;
    files: string[];
    subscription_path?: string;
    wireguard_conf?: string;
    amneziawg_conf?: string;
    masterdns_domain?: string;
    masterdns_key?: string;
    trusttunnel_path?: string;
  };
  note?: string;
  warning?: string;
}

interface Props {
  refreshTick?: number;
}

const td: React.CSSProperties = { padding: "0.45rem 0.6rem", fontSize: "0.8rem", verticalAlign: "middle" };
const th: React.CSSProperties = {
  ...td,
  textAlign: "left",
  fontWeight: 500,
  color: theme.textDim,
  background: theme.surface2,
  fontFamily: theme.mono,
  fontSize: "0.68rem",
  letterSpacing: "0.04em",
  textTransform: "uppercase" as const,
  borderBottom: `1px solid ${theme.border}`,
};

export default function Sources({ refreshTick }: Props) {
  const isMobile = useIsMobile();
  const [data, setData] = useState<SourcesResp>({ sources: [] });
  const [uploading, setUploading] = useState(false);
  const [dragOver, setDragOver] = useState(false);
  const [helpOpen, setHelpOpen] = useState(false);
  const [lastUpload, setLastUpload] = useState<UploadResult | null>(null);
  const [toast, setToast] = useState<{ msg: string; ok: boolean } | null>(null);
  const [reloading, setReloading] = useState(false);
  const [addOpen, setAddOpen] = useState(false);
  const [addName, setAddName] = useState("");
  const [addText, setAddText] = useState("");
  const [adding, setAdding] = useState(false);
  const fileRef = useRef<HTMLInputElement | null>(null);

  const addTextSource = async () => {
    if (!addName.trim() || !addText.trim()) {
      flash("A name and a URL / pasted URIs are required.", false);
      return;
    }
    setAdding(true);
    try {
      const r = await fetch(`${API_BASE}/api/sources`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name: addName.trim(), text: addText.trim() }),
      });
      if (!r.ok) {
        const msg = await r.text();
        flash(`Add failed: ${msg.slice(0, 160)}`, false);
        return;
      }
      const data = await r.json();
      flash(data.note ?? "Source added. Hit Reload.", true);
      setAddName("");
      setAddText("");
      setAddOpen(false);
      await refresh();
    } catch (e) {
      flash(`Add failed: ${(e as Error).message}`, false);
    } finally {
      setAdding(false);
    }
  };

  const refresh = async () => {
    try {
      const r = await fetch(`${API_BASE}/api/sources`);
      const j = (await r.json()) as SourcesResp;
      // Backend can send sources: null (no sources) — normalise so the render
      // never reads .length on null.
      setData({ ...j, sources: j.sources ?? [] });
    } catch {
      // ignore
    }
  };

  useEffect(() => {
    refresh();
  }, [refreshTick]);

  const flash = (msg: string, ok: boolean) => {
    setToast({ msg, ok });
    setTimeout(() => setToast(null), 3500);
  };

  const upload = async (file: File, name?: string) => {
    setUploading(true);
    try {
      const fd = new FormData();
      fd.append("bundle", file);
      if (name) fd.append("name", name);
      const r = await fetch(`${API_BASE}/api/bundles`, { method: "POST", body: fd });
      if (!r.ok) {
        const err = await r.text();
        flash(`Upload failed: ${err.slice(0, 120)}`, false);
        return;
      }
      const j = (await r.json()) as UploadResult;
      setLastUpload(j);
      flash(j.warning ?? "Bundle imported. Hit Reload to activate.", !j.warning);
      await refresh();
    } catch (e) {
      flash(`Upload failed: ${(e as Error).message}`, false);
    } finally {
      setUploading(false);
      if (fileRef.current) fileRef.current.value = "";
    }
  };

  const onDrop = (e: React.DragEvent) => {
    e.preventDefault();
    setDragOver(false);
    const f = e.dataTransfer.files[0];
    if (!f) return;
    if (!f.name.toLowerCase().endsWith(".zip")) {
      flash("Expected a .zip bundle.", false);
      return;
    }
    upload(f);
  };

  const removeSource = async (name: string) => {
    if (!window.confirm(`Remove source "${name}"? Its endpoints stay in the pool until you reload.`)) return;
    try {
      const r = await fetch(`${API_BASE}/api/sources/${encodeURIComponent(name)}`, { method: "DELETE" });
      const data = await r.json();
      if (!r.ok) {
        flash(`Remove failed: ${data?.error ?? r.statusText}`, false);
        return;
      }
      flash(data.note ?? "Removed. Reload to apply.", true);
      await refresh();
    } catch (e) {
      flash(`Remove failed: ${(e as Error).message}`, false);
    }
  };

  const reload = async () => {
    setReloading(true);
    try {
      const r = await fetch(`${API_BASE}/api/sources/reload`, { method: "POST" });
      const data = await r.json();
      flash(data.note ?? "Reloading…", data.ok !== false);
      // Give proxy-core a moment to come back.
      setTimeout(() => refresh(), 5000);
    } catch (e) {
      flash(`Reload failed: ${(e as Error).message}`, false);
    } finally {
      setTimeout(() => setReloading(false), 4000);
    }
  };

  return (
    <div>
      {/* Upload row — dropzone, then the ?/reload buttons. On mobile the
          buttons drop to their own full-width row so nothing gets squished. */}
      <div
        style={{
          display: "flex",
          flexDirection: isMobile ? "column" : "row",
          gap: "0.6rem",
          alignItems: "stretch",
          marginBottom: "1rem",
        }}
      >
        <div
          onDragOver={(e) => {
            e.preventDefault();
            setDragOver(true);
          }}
          onDragLeave={() => setDragOver(false)}
          onDrop={onDrop}
          onClick={() => fileRef.current?.click()}
          style={{
            flex: 1,
            display: "flex",
            flexWrap: "wrap",
            alignItems: "center",
            justifyContent: "center",
            gap: "0.4rem",
            border: `1.5px dashed ${dragOver ? theme.green : theme.border}`,
            background: dragOver ? theme.greenDim : theme.surface2,
            borderRadius: 6,
            padding: "0.7rem 1rem",
            cursor: uploading ? "wait" : "pointer",
            color: theme.text,
            fontFamily: theme.mono,
            fontSize: "0.82rem",
            minHeight: 44,
            transition: "all 0.15s ease",
          }}
        >
          {uploading ? (
            <span style={{ color: theme.blue }}>uploading…</span>
          ) : (
            <>
              <span style={{ color: theme.blue }}>+ drop bundle .zip</span>
              <span style={{ color: theme.textDim, fontSize: "0.72rem" }}>or click to browse</span>
            </>
          )}
          <input
            ref={fileRef}
            type="file"
            accept=".zip"
            style={{ display: "none" }}
            onChange={(e) => {
              const f = e.target.files?.[0];
              if (f) upload(f);
            }}
          />
        </div>
        <div style={{ display: "flex", gap: "0.5rem" }}>
          <button
            onClick={() => setHelpOpen((o) => !o)}
            style={{ ...chipBtn(theme.textDim), flex: isMobile ? "0 0 auto" : undefined }}
            title="What is a bundle?"
          >
            ?
          </button>
          <button
            onClick={reload}
            disabled={reloading}
            style={{ ...chipBtn(theme.blue), flex: isMobile ? 1 : undefined }}
            title="Restart proxy-core to load any new/removed sources"
          >
            {reloading ? "reloading…" : "↻ reload"}
          </button>
        </div>
      </div>

      {/* Add a source by pasting a subscription URL or URIs (any V2Ray config). */}
      <div style={{ marginBottom: addOpen ? "0.6rem" : "1rem" }}>
        <button
          onClick={() => setAddOpen((o) => !o)}
          style={{
            background: "none",
            border: "none",
            color: theme.blue,
            cursor: "pointer",
            fontFamily: theme.mono,
            fontSize: "0.74rem",
            padding: 0,
          }}
        >
          {addOpen ? "− cancel" : "+ or paste a subscription URL / URIs"}
        </button>
      </div>

      {addOpen && (
        <div
          style={{
            marginBottom: "1rem",
            padding: "0.75rem",
            background: theme.surface2,
            border: `1px solid ${theme.border}`,
            borderRadius: 6,
            display: "flex",
            flexDirection: "column",
            gap: "0.5rem",
          }}
        >
          <input
            type="text"
            value={addName}
            onChange={(e) => setAddName(e.target.value)}
            placeholder="source name (e.g. my-provider)"
            style={{ padding: "0.4rem 0.55rem", borderRadius: 4, fontFamily: theme.mono, fontSize: "0.82rem", width: "100%", minWidth: 0, boxSizing: "border-box" }}
          />
          <textarea
            value={addText}
            onChange={(e) => setAddText(e.target.value)}
            placeholder={"https://… subscription URL\n— or paste one or more —\nvless://…\nvmess://…\ntrojan://…"}
            rows={5}
            style={{ padding: "0.4rem 0.55rem", borderRadius: 4, fontFamily: theme.mono, fontSize: "0.78rem", width: "100%", minWidth: 0, boxSizing: "border-box", resize: "vertical" }}
          />
          <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: "0.5rem", flexWrap: "wrap" }}>
            <span style={{ fontSize: "0.68rem", color: theme.textDim, fontFamily: theme.mono }}>
              Accepts any standard V2Ray config — not just MoaV. Trust the source.
            </span>
            <button onClick={addTextSource} disabled={adding} style={chipBtn(theme.green)}>
              {adding ? "adding…" : "add source"}
            </button>
          </div>
        </div>
      )}

      {helpOpen && (
        <div
          style={{
            marginBottom: "1rem",
            padding: "0.65rem 0.85rem",
            background: theme.surface2,
            border: `1px solid ${theme.border}`,
            borderRadius: 6,
            fontSize: "0.78rem",
            color: theme.textDim,
            lineHeight: 1.5,
          }}
        >
          A bundle is a <code>.zip</code> exported from your MoaV server's per-user
          directory. moav-client scans it for <code>subscription.txt</code>,{" "}
          <code>wireguard.conf</code>, <code>amneziawg.conf</code>,{" "}
          <code>trusttunnel.toml</code>, <code>masterdns-instructions.txt</code> and
          registers a new source. 64 MB max. Endpoints from each source share a
          pool but you can disable individual ones in the <strong>Endpoints</strong> tab.
        </div>
      )}

      {/* Upload result mini-summary */}
      {lastUpload?.result && (
        <div
          style={{
            marginBottom: "0.75rem",
            padding: "0.55rem 0.75rem",
            border: `1px solid ${theme.green}`,
            borderRadius: 6,
            background: theme.greenDim,
            fontFamily: theme.mono,
            fontSize: "0.72rem",
            color: theme.text,
          }}
        >
          ✓ <strong style={{ color: theme.green }}>{lastUpload.result.name}</strong> imported
          {lastUpload.result.subscription_path && " · subscription"}
          {lastUpload.result.wireguard_conf && " · wireguard"}
          {lastUpload.result.amneziawg_conf && " · amneziawg"}
          {lastUpload.result.trusttunnel_path && " · trusttunnel"}
          {lastUpload.result.masterdns_domain && (
            <>
              {" · masterdns="}
              <span style={{ color: theme.blue }}>{lastUpload.result.masterdns_domain}</span>
            </>
          )}{" "}
          —{" "}
          <span style={{ color: theme.yellow }}>
            run reload to load its endpoints.
          </span>
        </div>
      )}

      {/* Sources — cards on mobile, table on desktop */}
      {data.sources.length === 0 ? (
        <div style={{ color: theme.textDim, fontSize: "0.8rem", padding: "0.5rem 0" }}>
          No sources loaded. Drop a bundle .zip above.
        </div>
      ) : isMobile ? (
        <div style={{ display: "flex", flexDirection: "column", gap: "0.6rem" }}>
          {data.sources.map((src) => (
            <div
              key={src.name}
              style={{
                border: `1px solid ${theme.border}`,
                borderRadius: 6,
                background: theme.surface2,
                padding: "0.7rem 0.75rem",
              }}
            >
              <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: "0.5rem" }}>
                <span style={{ display: "flex", alignItems: "center", gap: "0.45rem", flexWrap: "wrap" }}>
                  {src.is_moav && <MoavBadge />}
                  <span style={{ fontFamily: theme.mono, color: theme.green, fontWeight: 600, fontSize: "0.85rem" }}>
                    {src.name}
                  </span>
                </span>
                <button
                  onClick={() => removeSource(src.name)}
                  style={{
                    padding: "0.2rem 0.5rem",
                    background: "transparent",
                    border: `1px solid ${theme.border}`,
                    borderRadius: 4,
                    color: theme.red,
                    cursor: "pointer",
                    fontFamily: theme.mono,
                    fontSize: "0.7rem",
                    whiteSpace: "nowrap",
                  }}
                >
                  × remove
                </button>
              </div>
              {/* Show a remote URL (useful info) but not the local file path. */}
              {src.url && (
                <div style={{ fontFamily: theme.mono, color: theme.textDim, fontSize: "0.7rem", wordBreak: "break-all", marginTop: 4 }}>
                  {src.url}
                </div>
              )}
              <Tags tags={src.tags} />
              <div style={{ display: "flex", gap: "1.25rem", marginTop: 6, fontFamily: theme.mono, fontSize: "0.74rem" }}>
                <span style={{ color: theme.textDim }}>
                  endpoints <span style={{ color: theme.text }}>{src.endpoints}</span>
                </span>
                <span style={{ color: theme.textDim }}>
                  healthy <span style={{ color: src.healthy > 0 ? theme.green : theme.textDim }}>{src.healthy} / {src.endpoints}</span>
                </span>
              </div>
            </div>
          ))}
        </div>
      ) : (
        <div style={{ overflowX: "auto" }}>
          <table style={{ width: "100%", minWidth: 420, borderCollapse: "collapse" }}>
            <thead>
              <tr>
                {["Name", "Components", "Endpoints", "Healthy", ""].map((h) => (
                  <th key={h} style={th}>
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {data.sources.map((src) => (
                <tr key={src.name} style={{ borderTop: `1px solid ${theme.border}` }}>
                  <td style={{ ...td, fontFamily: theme.mono, color: theme.green, fontWeight: 600 }}>
                    <span style={{ display: "inline-flex", alignItems: "center", gap: "0.4rem" }}>
                      {src.is_moav && <MoavBadge />}
                      {src.name}
                    </span>
                  </td>
                  <td style={{ ...td, fontFamily: theme.mono, color: theme.textDim, fontSize: "0.72rem", wordBreak: "break-all" }}>
                    {src.url && <div>{src.url}</div>}
                    <Tags tags={src.tags} />
                  </td>
                  <td style={{ ...td, fontFamily: theme.mono }}>{src.endpoints}</td>
                  <td style={{ ...td, fontFamily: theme.mono, color: src.healthy > 0 ? theme.green : theme.textDim }}>
                    {src.healthy} / {src.endpoints}
                  </td>
                  <td style={{ ...td, textAlign: "right" }}>
                    <button
                      onClick={() => removeSource(src.name)}
                      style={{
                        padding: "0.2rem 0.5rem",
                        background: "transparent",
                        border: `1px solid ${theme.border}`,
                        borderRadius: 4,
                        color: theme.red,
                        cursor: "pointer",
                        fontFamily: theme.mono,
                        fontSize: "0.7rem",
                      }}
                    >
                      × remove
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

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

const chipBtn = (color: string): React.CSSProperties => ({
  padding: "0.45rem 0.75rem",
  background: "transparent",
  color,
  border: `1px solid ${color}55`,
  borderRadius: 6,
  cursor: "pointer",
  fontFamily: theme.mono,
  fontSize: "0.72rem",
  fontWeight: 600,
  whiteSpace: "nowrap",
});
