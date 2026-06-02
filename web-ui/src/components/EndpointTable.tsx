// TODO Phase 4: fetch from GET /api/endpoints and render live data.

interface Endpoint {
  id: string;
  address: string;
  protocol: string;
  latencyMs: number | null;
  alive: boolean;
}

const placeholder: Endpoint[] = [
  { id: "ep-1", address: "example.com:443", protocol: "vless", latencyMs: null, alive: false },
];

export default function EndpointTable() {
  return (
    <table style={{ width: "100%", borderCollapse: "collapse", fontSize: "0.875rem" }}>
      <thead>
        <tr style={{ background: "#f8fafc" }}>
          {["ID", "Address", "Protocol", "Latency", "Status"].map((h) => (
            <th
              key={h}
              style={{ textAlign: "left", padding: "0.5rem 0.75rem", fontWeight: 500, color: "#475569" }}
            >
              {h}
            </th>
          ))}
        </tr>
      </thead>
      <tbody>
        {placeholder.map((ep) => (
          <tr key={ep.id} style={{ borderTop: "1px solid #e2e8f0" }}>
            <td style={{ padding: "0.5rem 0.75rem", color: "#94a3b8" }}>{ep.id}</td>
            <td style={{ padding: "0.5rem 0.75rem" }}>{ep.address}</td>
            <td style={{ padding: "0.5rem 0.75rem" }}>{ep.protocol}</td>
            <td style={{ padding: "0.5rem 0.75rem" }}>
              {ep.latencyMs !== null ? `${ep.latencyMs} ms` : "—"}
            </td>
            <td style={{ padding: "0.5rem 0.75rem" }}>
              <span style={{ color: ep.alive ? "#16a34a" : "#dc2626" }}>
                {ep.alive ? "alive" : "down"}
              </span>
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
