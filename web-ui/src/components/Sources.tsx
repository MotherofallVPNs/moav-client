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

const td: React.CSSProperties = { padding: "0.5rem 0.65rem", fontSize: "0.82rem", verticalAlign: "middle" };
const th: React.CSSProperties = {
  ...td,
  textAlign: "left",
  fontWeight: 500,
  color: theme.textDim,
  background: theme.surface2,
  fontFamily: theme.mono,
  fontSize: "0.7rem",
  letterSpacing: "0.04em",
  textTransform: "uppercase" as const,
  borderBottom: `1px solid ${theme.border}`,
};

export default function Sources({ refreshTick }: Props) {
  const [data, setData] = useState<SourcesResp>({ sources: [] });
  const [uploading, setUploading] = useState(false);
  const [dragOver, setDragOver] = useState(false);
  const [lastUpload, setLastUpload] = useState<UploadResult | null>(null);
  const [toast, setToast] = useState<{ msg: string; ok: boolean } | null>(null);
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
    setTimeout(() => setToast(null), 4000);
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
      flash(j.warning ?? j.note ?? "Bundle imported.", !j.warning);
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
      flash("Expected a .zip bundle file.", false);
      return;
    }
    upload(f);
  };

  return (
    <div>
      <div style={{ marginBottom: "1.25rem" }}>
        <h3 style={section()}>import a bundle</h3>
        <p style={{ ...blurb(), marginBottom: "0.65rem" }}>
          Drop a <code>.zip</code> exported from MoaV's per-user bundle directory (the
          same folder that contains <code>subscription.txt</code>, <code>wireguard.conf</code>,
          <code>amneziawg.conf</code>, <code>masterdns-instructions.txt</code>, etc).
          moav-client extracts it into <code>data/&lt;name&gt;/</code>, auto-detects all
          recognised files, and registers a new subscription source in
          <code>config.yaml</code>.
        </p>
        <div
          onDragOver={(e) => {
            e.preventDefault();
            setDragOver(true);
          }}
          onDragLeave={() => setDragOver(false)}
          onDrop={onDrop}
          onClick={() => fileRef.current?.click()}
          style={{
            border: `1.5px dashed ${dragOver ? theme.green : theme.border}`,
            background: dragOver ? theme.greenDim : theme.surface2,
            borderRadius: 8,
            padding: "1.5rem",
            textAlign: "center",
            cursor: uploading ? "wait" : "pointer",
            color: theme.text,
            fontFamily: theme.mono,
            fontSize: "0.85rem",
            transition: "all 0.15s ease",
          }}
        >
          {uploading ? (
            <span style={{ color: theme.blue }}>uploading…</span>
          ) : (
            <>
              <div style={{ marginBottom: 4 }}>
                <span style={{ color: theme.blue }}>drop a .zip here</span> or click to browse
              </div>
              <div style={{ color: theme.textDim, fontSize: "0.72rem" }}>
                64 MB max · scanned for subscription.txt / wireguard.conf / amneziawg.conf /
                trusttunnel.toml / masterdns-instructions.txt
              </div>
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

        {lastUpload?.result && (
          <div
            style={{
              marginTop: "0.6rem",
              padding: "0.75rem",
              border: `1px solid ${theme.green}`,
              borderRadius: 6,
              background: theme.greenDim,
              fontFamily: theme.mono,
              fontSize: "0.75rem",
            }}
          >
            <div style={{ marginBottom: 6, fontWeight: 600 }}>
              ✓ extracted as <span style={{ color: theme.green }}>{lastUpload.result.name}</span>
            </div>
            {lastUpload.result.subscription_path && (
              <Detected label="subscription" path={lastUpload.result.subscription_path} />
            )}
            {lastUpload.result.wireguard_conf && (
              <Detected label="wireguard.conf" path={lastUpload.result.wireguard_conf} />
            )}
            {lastUpload.result.amneziawg_conf && (
              <Detected label="amneziawg.conf" path={lastUpload.result.amneziawg_conf} />
            )}
            {lastUpload.result.trusttunnel_path && (
              <Detected label="trusttunnel.toml" path={lastUpload.result.trusttunnel_path} />
            )}
            {lastUpload.result.masterdns_domain && (
              <Detected
                label="masterdns"
                path={`domain=${lastUpload.result.masterdns_domain} (key auto-loaded)`}
              />
            )}
            <div style={{ color: theme.textDim, marginTop: 6 }}>
              {lastUpload.note}{" "}
              <strong style={{ color: theme.yellow }}>
                Run <code>docker compose restart proxy-core</code> to load the new endpoints.
              </strong>
            </div>
          </div>
        )}
      </div>

      <h3 style={section()}>active subscription sources</h3>
      <table style={{ width: "100%", borderCollapse: "collapse" }}>
        <thead>
          <tr>
            {["Name", "Source", "Endpoints", "Healthy"].map((h) => (
              <th key={h} style={th}>
                {h}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {data.sources.length === 0 ? (
            <tr>
              <td colSpan={4} style={{ ...td, color: theme.textDim }}>
                No sources loaded yet — import a bundle above.
              </td>
            </tr>
          ) : (
            data.sources.map((src, i) => (
              <tr key={src.name + i} style={{ borderTop: `1px solid ${theme.border}` }}>
                <td style={td}>
                  <span style={{ fontFamily: theme.mono, color: theme.green }}>{src.name}</span>
                </td>
                <td style={{ ...td, fontFamily: theme.mono, color: theme.textDim, fontSize: "0.75rem", wordBreak: "break-all" }}>
                  {src.file || src.url || "—"}
                  {src.wireguard_files && src.wireguard_files.length > 0 && (
                    <div style={{ marginTop: 2 }}>
                      + {src.wireguard_files.length} wireguard conf
                      {src.wireguard_files.length > 1 ? "s" : ""}
                    </div>
                  )}
                </td>
                <td style={{ ...td, fontFamily: theme.mono }}>{src.endpoints}</td>
                <td
                  style={{
                    ...td,
                    fontFamily: theme.mono,
                    color: src.healthy > 0 ? theme.green : theme.textDim,
                  }}
                >
                  {src.healthy}
                </td>
              </tr>
            ))
          )}
        </tbody>
      </table>

      {data.note && (
        <div style={{ marginTop: "0.5rem", fontSize: "0.72rem", color: theme.textDim, fontFamily: theme.mono }}>
          {data.note}
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

function Detected({ label, path }: { label: string; path: string }) {
  return (
    <div style={{ marginBottom: 2 }}>
      <span style={{ color: theme.textDim }}>{label}:</span>{" "}
      <span style={{ color: theme.blue, wordBreak: "break-all" }}>{path}</span>
    </div>
  );
}

const section = (): React.CSSProperties => ({
  margin: "0 0 0.5rem",
  fontFamily: theme.mono,
  fontSize: "0.78rem",
  color: theme.text,
  textTransform: "uppercase",
  letterSpacing: "0.04em",
});

const blurb = (): React.CSSProperties => ({
  margin: 0,
  color: theme.textDim,
  fontSize: "0.78rem",
  lineHeight: 1.5,
});
