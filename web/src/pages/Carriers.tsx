import { useEffect, useState } from "react";
import { api, type Carrier, type CarrierHistoryEntry, type MediaNode } from "../api";

export default function Carriers() {
  const [rows, setRows] = useState<Carrier[]>([]);
  const [nodes, setNodes] = useState<MediaNode[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [showForm, setShowForm] = useState(false);
  const [form, setForm] = useState({
    name: "",
    host: "",
    port: 5060,
    transport: "udp",
    assigned_node_id: 0,
    codec_pref: "",
  });
  const [busy, setBusy] = useState(false);
  const [historyFor, setHistoryFor] = useState<number | null>(null);
  const [history, setHistory] = useState<CarrierHistoryEntry[]>([]);

  function reload() {
    Promise.all([
      api.get<Carrier[]>("/api/v1/carriers"),
      api.get<MediaNode[]>("/api/v1/nodes"),
    ])
      .then(([c, n]) => {
        setRows(c);
        setNodes(n);
      })
      .catch((e) => setErr(e.message));
  }
  useEffect(reload, []);

  const mediaNodes = nodes.filter((n) => n.role === "media");

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setErr(null);
    setBusy(true);
    try {
      await api.post("/api/v1/carriers", {
        ...form,
        assigned_node_id: form.assigned_node_id || undefined,
        codec_pref: form.codec_pref || undefined,
      });
      setForm({ name: "", host: "", port: 5060, transport: "udp", assigned_node_id: 0, codec_pref: "" });
      setShowForm(false);
      reload();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "create failed");
    } finally {
      setBusy(false);
    }
  }
  async function reassignNode(carrierID: number, nodeID: number) {
    const reason = prompt("Why are you reassigning this carrier?");
    if (reason === null) return;
    try {
      await api.patch<void>(`/api/v1/carriers/${carrierID}`, {
        assigned_node_id: nodeID,
        reason,
      });
      reload();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "reassign failed");
    }
  }
  async function setStatus(id: number, status: string) {
    try {
      await api.patch<void>(`/api/v1/carriers/${id}`, { status });
      reload();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "status change failed");
    }
  }
  async function del(id: number) {
    if (!confirm("Delete carrier?")) return;
    try {
      await api.del<void>(`/api/v1/carriers/${id}`);
      reload();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "delete failed");
    }
  }
  async function viewHistory(id: number) {
    try {
      const h = await api.get<CarrierHistoryEntry[]>(`/api/v1/carriers/${id}/node-history`);
      setHistory(h);
      setHistoryFor(id);
    } catch (e) {
      setErr(e instanceof Error ? e.message : "history fetch failed");
    }
  }

  const nodeName = (id: number | null | undefined) =>
    id ? nodes.find((n) => n.id === id)?.name ?? `#${id}` : "—";

  return (
    <div className="space-y-4">
      <header className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-semibold tracking-tight">Carriers</h2>
          <p className="text-sm text-slate-500">
            Upstream termination providers. Each maps to one media node at a time.
          </p>
        </div>
        <button
          onClick={() => setShowForm((v) => !v)}
          className="rounded-md bg-brand-600 px-3 py-2 text-sm font-medium text-white shadow-sm hover:bg-brand-700"
        >
          {showForm ? "Cancel" : "Add carrier"}
        </button>
      </header>

      {err && (
        <div className="rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
          {err}
        </div>
      )}

      {showForm && (
        <form
          onSubmit={submit}
          className="grid grid-cols-1 gap-3 rounded-lg border border-slate-200 bg-white p-4 shadow-sm sm:grid-cols-3"
        >
          <div>
            <label className="block text-xs font-medium uppercase tracking-wide text-slate-500">Name</label>
            <input required value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm" />
          </div>
          <div>
            <label className="block text-xs font-medium uppercase tracking-wide text-slate-500">Host</label>
            <input required value={form.host} onChange={(e) => setForm({ ...form, host: e.target.value })} placeholder="sip.carrier.com" className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm font-mono" />
          </div>
          <div>
            <label className="block text-xs font-medium uppercase tracking-wide text-slate-500">Port</label>
            <input type="number" value={form.port} onChange={(e) => setForm({ ...form, port: Number(e.target.value) })} className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm" />
          </div>
          <div>
            <label className="block text-xs font-medium uppercase tracking-wide text-slate-500">Transport</label>
            <select value={form.transport} onChange={(e) => setForm({ ...form, transport: e.target.value })} className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm">
              <option value="udp">udp</option>
              <option value="tcp">tcp</option>
              <option value="tls">tls</option>
            </select>
          </div>
          <div>
            <label className="block text-xs font-medium uppercase tracking-wide text-slate-500">Media node</label>
            <select value={form.assigned_node_id} onChange={(e) => setForm({ ...form, assigned_node_id: Number(e.target.value) })} className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm">
              <option value={0}>— none —</option>
              {mediaNodes.map((n) => <option key={n.id} value={n.id}>{n.name}</option>)}
            </select>
          </div>
          <div>
            <label className="block text-xs font-medium uppercase tracking-wide text-slate-500">Codec pref</label>
            <input value={form.codec_pref} onChange={(e) => setForm({ ...form, codec_pref: e.target.value })} placeholder="g711a,g711u" className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm" />
          </div>
          <div className="sm:col-span-3">
            <button type="submit" disabled={busy} className="rounded-md bg-brand-600 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-brand-700 disabled:opacity-60">
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
              <th className="px-4 py-2">Name</th>
              <th className="px-4 py-2">Endpoint</th>
              <th className="px-4 py-2">Node</th>
              <th className="px-4 py-2">Status</th>
              <th className="px-4 py-2" />
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100">
            {rows.length === 0 && (
              <tr><td colSpan={6} className="px-4 py-6 text-center text-slate-400">No carriers yet.</td></tr>
            )}
            {rows.map((r) => (
              <tr key={r.id}>
                <td className="px-4 py-2 font-mono text-slate-500">{r.id}</td>
                <td className="px-4 py-2 font-medium">{r.name}</td>
                <td className="px-4 py-2 font-mono text-xs">{r.transport}://{r.host}:{r.port}</td>
                <td className="px-4 py-2">
                  <select
                    value={r.assigned_node_id ?? 0}
                    onChange={(e) => reassignNode(r.id, Number(e.target.value))}
                    className="rounded border border-slate-300 px-2 py-0.5 text-xs"
                  >
                    <option value={0}>— none —</option>
                    {mediaNodes.map((n) => <option key={n.id} value={n.id}>{n.name}</option>)}
                  </select>
                </td>
                <td className="px-4 py-2">
                  <select value={r.status} onChange={(e) => setStatus(r.id, e.target.value)} className="rounded border border-slate-300 px-2 py-0.5 text-xs">
                    <option value="active">active</option>
                    <option value="paused">paused</option>
                    <option value="disabled">disabled</option>
                  </select>
                </td>
                <td className="px-4 py-2 text-right text-xs space-x-3">
                  <button onClick={() => viewHistory(r.id)} className="text-slate-600 hover:underline">History</button>
                  <button onClick={() => del(r.id)} className="text-red-600 hover:underline">Delete</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {historyFor !== null && (
        <div className="rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
          <div className="mb-2 flex items-center justify-between">
            <h3 className="text-sm font-semibold uppercase tracking-wide text-slate-500">
              Node reassignment history — carrier #{historyFor}
            </h3>
            <button onClick={() => setHistoryFor(null)} className="text-xs text-slate-500 hover:underline">close</button>
          </div>
          <ul className="divide-y divide-slate-100 text-sm">
            {history.length === 0 && <li className="py-3 text-slate-400">No changes recorded.</li>}
            {history.map((h) => (
              <li key={h.id} className="flex items-baseline justify-between py-2">
                <span>
                  <code className="text-xs text-slate-500">{new Date(h.changed_at).toLocaleString()}</code>{" "}
                  {nodeName(h.old_node_id)} → <strong>{nodeName(h.new_node_id)}</strong>
                </span>
                <span className="text-xs text-slate-500">{h.reason ?? ""}</span>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}
