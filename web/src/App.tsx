import { useEffect, useState } from "react";

type Snapshot = {
  host: string;
  ts: string;
  cpu_pct: number;
};

type Status = "connecting" | "connected" | "disconnected" | "error";

export default function App() {
  const [snapshots, setSnapshots] = useState<Snapshot[]>([]);
  const [status, setStatus] = useState<Status>("connecting");

  useEffect(() => {
    const scheme = window.location.protocol === "https:" ? "wss" : "ws";
    const url = `${scheme}://${window.location.host}/api/stream`;
    const ws = new WebSocket(url);

    ws.addEventListener("open", () => setStatus("connected"));
    ws.addEventListener("message", (e) => {
      try {
        const parsed = JSON.parse(e.data as string) as Snapshot[];
        setSnapshots(parsed ?? []);
      } catch {
        // ignore malformed frames
      }
    });
    ws.addEventListener("close", () => setStatus("disconnected"));
    ws.addEventListener("error", () => setStatus("error"));

    return () => ws.close();
  }, []);

  return (
    <main
      style={{
        fontFamily:
          "system-ui, -apple-system, Segoe UI, Roboto, sans-serif",
        padding: 24,
        maxWidth: 720,
        margin: "0 auto",
      }}
    >
      <h1 style={{ marginBottom: 4 }}>Lumen — live CPU</h1>
      <p style={{ color: "#888", marginTop: 0 }}>
        WebSocket: <strong>{status}</strong> · {snapshots.length} host
        {snapshots.length === 1 ? "" : "s"}
      </p>

      {snapshots.length === 0 ? (
        <p style={{ color: "#888" }}>
          Waiting for agent data… start <code>make dev-agent</code>.
        </p>
      ) : (
        <table style={{ borderCollapse: "collapse", width: "100%" }}>
          <thead>
            <tr style={{ textAlign: "left", borderBottom: "1px solid #ccc" }}>
              <th style={{ padding: "8px 4px" }}>Host</th>
              <th style={{ padding: "8px 4px" }}>CPU %</th>
              <th style={{ padding: "8px 4px" }}>Updated</th>
            </tr>
          </thead>
          <tbody>
            {snapshots.map((s) => (
              <tr key={s.host} style={{ borderBottom: "1px solid #eee" }}>
                <td style={{ padding: "8px 4px" }}>{s.host}</td>
                <td style={{ padding: "8px 4px" }}>{s.cpu_pct.toFixed(2)}</td>
                <td style={{ padding: "8px 4px", color: "#666" }}>
                  {new Date(s.ts).toLocaleTimeString()}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </main>
  );
}
