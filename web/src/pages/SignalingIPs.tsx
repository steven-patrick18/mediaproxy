import { useEffect, useState } from "react";
import { api, type MediaNode, type SignalingIP } from "../api";

export default function SignalingIPs() {
  const [rows, setRows] = useState<SignalingIP[]>([]);
  const [nodes, setNodes] = useState<MediaNode[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [showForm, setShowForm] = useState(false);
  const [form, setForm] = useState({ ip_address: "", sip_proxy_node_id: 0 });
  const [busy, setBusy] = useState(false);

  function reload() {
    Promise.all([
      api.get<SignalingIP[]>("/api/v1/signaling-ips"),
      api.get<MediaNode[]>("/api/v1/nodes"),
    ])
      .then(([s, n]) => {
        setRows(s);
        setNodes(n);
      })
      .catch((e) => setErr(e.message));
  }
  useEffect(reload, []);

  const sipProxies = nodes.filter((n) => n.role === "sip_proxy");

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setErr(null);
    setBusy(true);
    try {
      await api.post("/api/v1/signaling-ips", form);
      setForm({ ip_address: "", sip_proxy_node_id: 0 });
      setShowForm(false);
      reload();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "create failed");
    } finally {
      setBusy(false);
    }
  }

  async function del(id: number) {
    if (!confirm("Delete this signaling IP? It will be unassigned from its client.")) return;
    try {
      await api.del<void>(`/api/v1/signaling-ips/${id}`);
      reload();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "delete failed");
    }
  }

  const nodeName = (id: number) => nodes.find((n) => n.id === id)?.name ?? `#${id}`;

  return (
    <div className="space-y-4">
      <header className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-semibold tracking-tight">Signaling IPs</h2>
          <p className="text-sm text-slate-500">
            Per-client SIP source IPs. IPs bound on a sip_proxy node's NIC are auto-discovered from
            agent heartbeats; you can also add them manually with the button on the right. Carriers
            whitelist these IPs on their side.
          </p>
        </div>
        <button
          onClick={() => setShowForm((v) => !v)}
          disabled={sipProxies.length === 0}
          className="rounded-md bg-brand-600 px-3 py-2 text-sm font-medium text-white shadow-sm hover:bg-brand-700 disabled:opacity-60"
        >
          {showForm ? "Cancel" : "Add signaling IP"}
        </button>
      </header>

      {sipProxies.length === 0 && (
        <div className="rounded border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-800">
          You need at least one node with role <code>sip_proxy</code> before you can add signaling
          IPs. Create one on the Nodes page.
        </div>
      )}

      {err && (
        <div className="rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
          {err}
        </div>
      )}

      {showForm && (
        <form
          onSubmit={submit}
          className="grid grid-cols-1 gap-3 rounded-lg border border-slate-200 bg-white p-4 shadow-sm sm:grid-cols-2"
        >
          <div>
            <label className="block text-xs font-medium uppercase tracking-wide text-slate-500">
              IP address
            </label>
            <input
              required
              value={form.ip_address}
              onChange={(e) => setForm({ ...form, ip_address: e.target.value })}
              placeholder="203.0.113.10"
              className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm font-mono"
            />
          </div>
          <div>
            <label className="block text-xs font-medium uppercase tracking-wide text-slate-500">
              SIP proxy node
            </label>
            <select
              required
              value={form.sip_proxy_node_id}
              onChange={(e) =>
                setForm({ ...form, sip_proxy_node_id: Number(e.target.value) })
              }
              className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm"
            >
              <option value={0}>— select —</option>
              {sipProxies.map((n) => (
                <option key={n.id} value={n.id}>
                  {n.name} ({n.host_ip})
                </option>
              ))}
            </select>
          </div>
          <div className="sm:col-span-2">
            <button
              type="submit"
              disabled={busy || form.sip_proxy_node_id === 0}
              className="rounded-md bg-brand-600 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-brand-700 disabled:opacity-60"
            >
              {busy ? "Saving…" : "Create"}
            </button>
          </div>
        </form>
      )}

      <div className="overflow-hidden rounded-lg border border-slate-200 bg-white shadow-sm">
        <table className="min-w-full divide-y divide-slate-200 text-sm">
          <thead className="bg-slate-50 text-left text-xs uppercase tracking-wide text-slate-500">
            <tr>
              <th className="px-4 py-2">ID</th>
              <th className="px-4 py-2">IP</th>
              <th className="px-4 py-2">SIP node</th>
              <th className="px-4 py-2">Status</th>
              <th className="px-4 py-2">Source</th>
              <th className="px-4 py-2">Assigned client</th>
              <th className="px-4 py-2" />
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100">
            {rows.length === 0 && (
              <tr>
                <td colSpan={7} className="px-4 py-6 text-center text-slate-400">
                  No signaling IPs yet.
                </td>
              </tr>
            )}
            {rows.map((s) => (
              <tr key={s.id}>
                <td className="px-4 py-2 font-mono text-slate-500">{s.id}</td>
                <td className="px-4 py-2 font-mono">{s.ip_address}</td>
                <td className="px-4 py-2">{nodeName(s.sip_proxy_node_id)}</td>
                <td className="px-4 py-2">
                  <span
                    className={
                      "rounded px-2 py-0.5 text-xs " +
                      (s.status === "available"
                        ? "bg-emerald-100 text-emerald-800"
                        : s.status === "assigned"
                          ? "bg-blue-100 text-blue-800"
                          : "bg-slate-200 text-slate-700")
                    }
                  >
                    {s.status}
                  </span>
                </td>
                <td className="px-4 py-2 text-xs">
                  {s.auto_discovered ? (
                    <span className="rounded bg-emerald-100 px-1.5 py-0.5 text-emerald-800">auto</span>
                  ) : (
                    <span className="rounded bg-slate-200 px-1.5 py-0.5 text-slate-700">manual</span>
                  )}
                </td>
                <td className="px-4 py-2 text-slate-600">
                  {s.assigned_client_id ?? "—"}
                </td>
                <td className="px-4 py-2 text-right">
                  <button
                    onClick={() => del(s.id)}
                    className="text-xs text-red-600 hover:underline"
                  >
                    Delete
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
