// MoaV admin-style dark palette.
// Pulled from shayanb/MoaV admin/templates/dashboard.html so the moav-client
// dashboard looks like a peer of the server-side admin UI.
export const theme = {
  bg: "#0a0e17",
  surface: "#111827",
  surface2: "#1a2233",
  border: "#1e2d3d",
  text: "#c9d1d9",
  textDim: "#6e7681",
  green: "#3fb950",
  greenDim: "rgba(63, 185, 80, 0.15)",
  blue: "#58a6ff",
  blueDim: "rgba(88, 166, 255, 0.1)",
  red: "#f85149",
  redDim: "rgba(248, 81, 73, 0.15)",
  yellow: "#d29922",
  yellowDim: "rgba(210, 153, 34, 0.15)",
  orange: "#db6d28",
  orangeDim: "rgba(219, 109, 40, 0.15)",
  mono: "'SF Mono', 'Cascadia Code', 'Fira Code', Consolas, monospace",
  sans:
    "-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif",
};

export const statusColor = (status: string): string => {
  if (status === "ok") return theme.green;
  if (status === "timeout" || status === "error") return theme.red;
  if (status === "unknown") return theme.textDim;
  return theme.textDim;
};

export const statusBg = (status: string): string => {
  if (status === "ok") return theme.greenDim;
  if (status === "timeout" || status === "error") return theme.redDim;
  return "rgba(110, 118, 129, 0.15)";
};
