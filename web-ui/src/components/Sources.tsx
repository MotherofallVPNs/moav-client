import { useEffect, useRef, useState } from "react";
import { theme } from "../theme";
import { API_BASE } from "../apiBase";

interface Source {
  name: string;
  file?: string;
  url?: string;
  wireguard_files?: string[];
  endpoints: number;
  healthy: number;
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
  const [data, setData] = useState<SourcesResp>({ sources: [] });
  const [uploading, setUploading] = useState(false);
  const [dragOver, setDragOver] = useState(false);
  const [helpOpen, setHelpOpen] = useState(false);
  const [lastUpload, setLastUpload] = useState<UploadResult | null>(null);
  const [toast, setToast] = useState<{ msg: string; ok: boolean } | null>(null);
  const [reloading, setReloading] = useState(false);
  const fileRef = useRef<HTMLInputElement | null>(null);

  const refresh = async () => {
    try {
      const r = await fetch(`${API_BASE}/api/sources`);
      setData(await r.json());
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
      {/* Compact upload row */}
      <div style={{ display: "flex", gap: "0.75rem", alignItems: "stretch", marginBottom: "1rem" }}>
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
            alignItems: "center",
            justifyContent: "center",
            gap: "0.5rem",
            border: `1.5px dashed ${dragOver ? theme.green : theme.border}`,
            background: dragOver ? theme.greenDim : theme.surface2,
            borderRadius: 6,
            padding: "0.75rem 1rem",
            cursor: uploading ? "wait" : "pointer",
            color: theme.text,
            fontFamily: theme.mono,
            fontSize: "0.82rem",
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
        <button
          onClick={() => setHelpOpen((o) => !o)}
          style={chipBtn(theme.textDim)}
          title="What is a bundle?"
        >
          ?
        </button>
        <button
          onClick={reload}
          disabled={reloading}
          style={chipBtn(theme.blue)}
          title="Restart proxy-core to load any new/removed sources"
        >
          {reloading ? "reloading…" : "↻ reload"}
        </button>
      </div>

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

      {/* Sources table */}
      <table style={{ width: "100%", borderCollapse: "collapse" }}>
        <thead>
          <tr>
            {["Name", "Source", "Endpoints", "Healthy", ""].map((h) => (
              <th key={h} style={th}>
                {h}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {data.sources.length === 0 ? (
            <tr>
              <td colSpan={5} style={{ ...td, color: theme.textDim }}>
                No sources loaded. Drop a bundle .zip above.
              </td>
            </tr>
          ) : (
            data.sources.map((src) => (
              <tr key={src.name} style={{ borderTop: `1px solid ${theme.border}` }}>
                <td style={{ ...td, fontFamily: theme.mono, color: theme.green, fontWeight: 600 }}>
                  {src.name}
                </td>
                <td style={{ ...td, fontFamily: theme.mono, color: theme.textDim, fontSize: "0.72rem", wordBreak: "break-all" }}>
                  {src.file || src.url || "—"}
                  {src.wireguard_files && src.wireguard_files.length > 0 && (
                    <span style={{ color: theme.blue, marginLeft: 6 }}>
                      + {src.wireguard_files.length} wg
                    </span>
                  )}
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
            ))
          )}
        </tbody>
      </table>

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
