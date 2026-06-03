import { useEffect, useState } from "react";
import { theme } from "../theme";
import { API_BASE } from "../apiBase";

// TODO: when the repo is made public, update GITHUB_URL to the canonical
// upstream — currently points at the private fork used during development.
const GITHUB_URL = "https://github.com/MotherofallVPNs/moav-client";

interface VersionResp {
  version: string;
  commit: string;
  uptime_sec: number;
  public_ip: string;        // kept for back-compat with older builds
  direct_ip: string;
  direct_country: string;
  proxy_ip: string;
  proxy_country: string;
}

function fmtUptime(sec: number): string {
  if (sec < 60) return `${sec}s`;
  if (sec < 3600) return `${Math.floor(sec / 60)}m`;
  if (sec < 86400) return `${Math.floor(sec / 3600)}h ${Math.floor((sec % 3600) / 60)}m`;
  return `${Math.floor(sec / 86400)}d ${Math.floor((sec % 86400) / 3600)}h`;
}

// Two-letter ISO country code → 🇫🇷 etc. Browsers without regional indicator
// support render the letters in a box; that's still readable.
function flag(cc?: string): string {
  if (!cc || cc.length !== 2) return "";
  const base = 0x1f1e6 - "A".charCodeAt(0);
  const a = cc.toUpperCase().charCodeAt(0) + base;
  const b = cc.toUpperCase().charCodeAt(1) + base;
  try {
    return String.fromCodePoint(a, b);
  } catch {
    return "";
  }
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
    const id = setInterval(fetchOnce, 60000); // hourly is fine; lookup is rate-limited upstream
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

  const ipChip = (label: string, ip?: string, cc?: string) =>
    ip ? (
      <span title={`${label} egress · ${cc || "??"}`}>
        {label} <span style={{ color: theme.text }}>{ip}</span>
        {cc && <span style={{ marginLeft: 4 }}>{flag(cc)}</span>}
      </span>
    ) : (
      <span style={{ opacity: 0.5 }}>{label} —</span>
    );

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
        {ipChip("direct", info?.direct_ip ?? info?.public_ip, info?.direct_country)}
        {ipChip("proxy", info?.proxy_ip, info?.proxy_country)}
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
