import { useEffect, useState } from "react";
import { api, type Client, type MediaNode, type Reseller } from "../api";

export default function Dashboard() {
  const [resellers, setResellers] = useState<Reseller[]>([]);
  const [clients, setClients] = useState<Client[]>([]);
  const [nodes, setNodes] = useState<MediaNode[]>([]);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    Promise.all([
      api.get<Reseller[]>("/api/v1/resellers"),
      api.get<Client[]>("/api/v1/clients"),
      api.get<MediaNode[]>("/api/v1/nodes"),
    ])
      .then(([r, c, n]) => {
        setResellers(r);
        setClients(c);
        setNodes(n);
      })
      .catch((e) => setErr(e.message));
  }, []);

  const onlineNodes = nodes.filter((n) => n.status === "online").length;

  const cards = [
    { label: "Resellers", value: resellers.length },
    { label: "Clients", value: clients.length },
    { label: "Media nodes", value: `${onlineNodes} / ${nodes.length}` },
    { label: "Active calls", value: 0 },
  ];

  return (
    <div className="space-y-6">
      <header>
        <h2 className="text-2xl font-semibold tracking-tight">Dashboard</h2>
        <p className="text-sm text-slate-500">
          Phase 1 — control plane is online.
        </p>
      </header>

      {err && (
        <div className="rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
          {err}
        </div>
      )}

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {cards.map((c) => (
          <div
            key={c.label}
            className="rounded-lg border border-slate-200 bg-white p-4 shadow-sm"
          >
            <div className="text-xs uppercase tracking-wide text-slate-500">
              {c.label}
            </div>
            <div className="mt-2 text-3xl font-semibold tracking-tight">
              {c.value}
            </div>
          </div>
        ))}
      </div>

      <div className="rounded-lg border border-slate-200 bg-white p-4 text-sm text-slate-600 shadow-sm">
        <p>Next up on the build plan:</p>
        <ul className="ml-5 mt-2 list-disc space-y-1">
          <li>Agent daemon on a media node (Go binary, reports + commands)</li>
          <li>RTPEngine install + kernel module + UDP port range</li>
          <li>Kamailio install + whitelist + SDP rewrite via RTPEngine</li>
          <li>End-to-end test call with media IP rotation</li>
        </ul>
      </div>
    </div>
  );
}
