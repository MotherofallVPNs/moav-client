import { useEffect, useState } from "react";
import { theme } from "../theme";
import { API_BASE } from "../apiBase";

// TODO: when the repo is made public, update GITHUB_URL to the canonical
// upstream — currently points at the private fork used during development.
const GITHUB_URL = "https://github.com/ibeezhan/moav-client";

interface VersionResp {
  version: string;
  commit: string;
  uptime_sec: number;
  public_ip: string;
}

function fmtUptime(sec: number): string {
  if (sec < 60) return `${sec}s`;
  if (sec < 3600) return `${Math.floor(sec / 60)}m`;
  if (sec < 86400) return `${Math.floor(sec / 3600)}h ${Math.floor((sec % 3600) / 60)}m`;
  return `${Math.floor(sec / 86400)}d ${Math.floor((sec % 86400) / 3600)}h`;
}

export default function Footer() {
  const [info, setInfo] = useState<VersionResp | null>(null);

  useEffect(() => {
    let cancelled = false;
    const fetchOnce = () => {
      fetch(`${API_BASE}/api/version`)
        .then((r) => r.json())
        .then((d) => {
          if (!cancelled) setInfo(d as VersionResp);
        })
        .catch(() => {});
    };
    fetchOnce();
    const id = setInterval(fetchOnce, 30000); // refresh uptime every 30s
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, []);

  const link: React.CSSProperties = {
    color: theme.blue,
    textDecoration: "none",
    fontFamily: theme.mono,
  };

  return (
    <footer
      style={{
        marginTop: "2rem",
        paddingTop: "1rem",
        borderTop: `1px solid ${theme.border}`,
        display: "flex",
        justifyContent: "space-between",
        alignItems: "center",
        flexWrap: "wrap",
        gap: "0.5rem",
        fontSize: "0.72rem",
        fontFamily: theme.mono,
        color: theme.textDim,
      }}
    >
      <div style={{ display: "flex", gap: "1rem", flexWrap: "wrap", alignItems: "center" }}>
        <span>
          MoaV-<span style={{ color: theme.green }}>client</span>{" "}
          <span style={{ color: theme.text }}>{info?.version ?? "—"}</span>
          {info?.commit && info.commit !== "dev" && (
            <span>
              {" "}
              · <span style={{ color: theme.textDim }}>{info.commit.slice(0, 7)}</span>
            </span>
          )}
        </span>
        {info?.public_ip && (
          <span>
            public ip <span style={{ color: theme.text }}>{info.public_ip}</span>
          </span>
        )}
        {info && (
          <span>
            up <span style={{ color: theme.text }}>{fmtUptime(info.uptime_sec)}</span>
          </span>
        )}
      </div>
      <div style={{ display: "flex", gap: "1rem", alignItems: "center" }}>
        <a href={GITHUB_URL} target="_blank" rel="noopener noreferrer" style={link}>
          ↗ github
        </a>
        <span style={{ color: theme.textDim }}>· mother of all VPNs</span>
      </div>
    </footer>
  );
}
