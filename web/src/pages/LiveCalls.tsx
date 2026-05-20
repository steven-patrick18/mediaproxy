import { useCallback, useEffect, useState } from "react";
import { api, type ActiveCallRow, type Carrier, type Client, type MediaNode } from "../api";
import { RefreshIcon } from "../components/Icons";

function fmtDur(sec: number): string {
  const m = Math.floor(sec / 60);
  const s = sec % 60;
  return `${m}:${s.toString().padStart(2, "0")}`;
}

export default function LiveCalls() {
  const [rows, setRows] = useState<ActiveCallRow[]>([]);
  const [nodes, setNodes] = useState<MediaNode[]>([]);
  const [clients, setClients] = useState<Client[]>([]);
  const [carriers, setCarriers] = useState<Carrier[]>([]);
  const [nodeFilter, setNodeFilter] = useState(0);
  const [err, setErr] = useState<string | null>(null);

  const reload = useCallback(() => {
    Promise.all([
      api.get<ActiveCallRow[]>(`/api/v1/calls/active${nodeFilter ? `?node_id=${nodeFilter}` : ""}`),
      api.get<MediaNode[]>("/api/v1/nodes"),
      api.get<Client[]>("/api/v1/clients"),
      api.get<Carrier[]>("/api/v1/carriers"),
    ])
      .then(([a, n, c, ca]) => {
        setRows(a);
        setNodes(n);
        setClients(c);
        setCarriers(ca);
      })
      .catch((e) => setErr(e.message));
  }, [nodeFilter]);
  useEffect(() => {
    reload();
    const t = setInterval(reload, 3000);
    return () => clearInterval(t);
  }, [reload]);

  const nodeName = (id: number | null | undefined) => (id ? nodes.find((n) => n.id === id)?.name ?? `#${id}` : "—");
  const clientName = (id: number | null | undefined) => (id ? clients.find((c) => c.id === id)?.name ?? `#${id}` : "—");
  const carrierName = (id: number | null | undefined) => (id ? carriers.find((c) => c.id === id)?.name ?? `#${id}` : "—");

  // Group counts per node from media_nodes.active_calls snapshot (always
  // available) so the per-node bar still works when no detailed call data
  // is flowing yet.
  const perNode = nodes
    .filter((n) => n.role === "media")
    .map((n) => ({ name: n.name, id: n.id, count: n.active_calls ?? 0, max: n.max_calls }));

  return (
    <div className="space-y-4">
      <header className="flex items-start justify-between">
        <div>
          <h2 className="text-xl font-semibold tracking-tight text-slate-800">Live calls</h2>
          <p className="text-xs text-slate-500">
            Active calls per media node, refreshing every 3s. Detailed per-call rows appear once
            Kamailio + RTPEngine are pushing call events.
          </p>
        </div>
        <button onClick={reload} className="inline-flex items-center gap-1.5 rounded-md border border-slate-300 px-3 py-1.5 text-sm hover:bg-slate-50">
          <RefreshIcon /> Refresh
        </button>
      </header>

      {err && <div className="rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{err}</div>}

      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
        {perNode.length === 0 && (
          <div className="col-span-full rounded-lg border border-dashed border-slate-300 bg-white p-6 text-center text-sm text-slate-500">
            No media nodes yet.
          </div>
        )}
        {perNode.map((n) => {
          const pct = n.max ? Math.min(100, (n.count / n.max) * 100) : 0;
          return (
            <button
              key={n.id}
              onClick={() => setNodeFilter(n.id === nodeFilter ? 0 : n.id)}
              className={`rounded-lg border bg-white p-4 text-left shadow-sm transition ${
                nodeFilter === n.id ? "border-brand-500 ring-2 ring-brand-200" : "border-slate-200 hover:border-slate-300"
              }`}
            >
              <div className="text-xs uppercase tracking-wide text-slate-500">{n.name}</div>
              <div className="mt-1 flex items-baseline gap-1">
                <span className="text-2xl font-semibold">{n.count}</span>
                <span className="text-sm text-slate-500">/ {n.max || "—"}</span>
              </div>
              <div className="mt-2 h-1.5 w-full overflow-hidden rounded bg-slate-200">
                <div className="h-full bg-brand-600" style={{ width: `${pct}%` }} />
              </div>
            </button>
          );
        })}
      </div>

      <div className="flex items-center gap-2 text-sm">
        <span className="text-slate-500">Filter:</span>
        <select value={nodeFilter} onChange={(e) => setNodeFilter(Number(e.target.value))} className="rounded border border-slate-300 px-2 py-1">
          <option value={0}>all nodes</option>
          {nodes.map((n) => <option key={n.id} value={n.id}>{n.name}</option>)}
        </select>
        <span className="ml-auto text-xs text-slate-400">{rows.length} call(s) tracked</span>
      </div>

      <div className="overflow-hidden rounded-lg border border-slate-200 bg-white shadow-sm">
        <table className="min-w-full divide-y divide-slate-200 text-sm">
          <thead className="bg-slate-50 text-left text-xs uppercase tracking-wide text-slate-500">
            <tr>
              <th className="px-3 py-2">Started</th>
              <th className="px-3 py-2">Dur</th>
              <th className="px-3 py-2">ANI</th>
              <th className="px-3 py-2">DNIS</th>
              <th className="px-3 py-2">Client</th>
              <th className="px-3 py-2">Carrier</th>
              <th className="px-3 py-2">Node</th>
              <th className="px-3 py-2">Media IP</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100">
            {rows.length === 0 && (
              <tr>
                <td colSpan={8} className="px-3 py-6 text-center text-slate-400">
                  No live call detail. Cards above show per-node counters from heartbeats.
                </td>
              </tr>
            )}
            {rows.map((r) => (
              <tr key={r.id}>
                <td className="px-3 py-1.5 text-xs text-slate-500">{new Date(r.started_at).toLocaleTimeString()}</td>
                <td className="px-3 py-1.5 font-mono">{fmtDur(r.duration_sec)}</td>
                <td className="px-3 py-1.5 font-mono text-xs">{r.ani ?? "—"}</td>
                <td className="px-3 py-1.5 font-mono text-xs">{r.dnis ?? "—"}</td>
                <td className="px-3 py-1.5">{clientName(r.client_id)}</td>
                <td className="px-3 py-1.5">{carrierName(r.carrier_id)}</td>
                <td className="px-3 py-1.5">{nodeName(r.node_id)}</td>
                <td className="px-3 py-1.5 font-mono text-xs">{r.media_ip ?? "—"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
