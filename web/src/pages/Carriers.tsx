import { useEffect, useState } from "react";
import { api, type Carrier, type CarrierHistoryEntry, type MediaNode } from "../api";
import Modal from "../components/Modal";
import { PencilIcon, PlusIcon, TrashIcon } from "../components/Icons";

const blankForm = {
  name: "",
  host: "",
  port: 5060,
  transport: "udp" as "udp" | "tcp" | "tls",
  assigned_node_id: 0,
  codec_pref: "",
  status: "active",
  notes: "",
};

export default function Carriers() {
  const [rows, setRows] = useState<Carrier[]>([]);
  const [nodes, setNodes] = useState<MediaNode[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<Carrier | null>(null);
  const [form, setForm] = useState(blankForm);

  const [historyFor, setHistoryFor] = useState<Carrier | null>(null);
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
  const nodeName = (id: number | null | undefined) =>
    id ? nodes.find((n) => n.id === id)?.name ?? `#${id}` : "—";

  function openCreate() {
    setForm(blankForm);
    setCreating(true);
  }
  function openEdit(c: Carrier) {
    setForm({
      name: c.name,
      host: c.host,
      port: c.port,
      transport: c.transport,
      assigned_node_id: c.assigned_node_id ?? 0,
      codec_pref: c.codec_pref ?? "",
      status: c.status,
      notes: (c as Carrier & { notes?: string }).notes ?? "",
    });
    setEditing(c);
  }

  async function save() {
    setErr(null);
    setBusy(true);
    try {
      if (editing) {
        const body: Record<string, unknown> = {
          name: form.name,
          host: form.host,
          port: form.port,
          transport: form.transport,
          codec_pref: form.codec_pref || undefined,
          status: form.status,
          notes: form.notes,
        };
        if (form.assigned_node_id !== (editing.assigned_node_id ?? 0)) {
          body.assigned_node_id = form.assigned_node_id || undefined;
          body.reason = prompt("Reason for reassigning the node?") ?? "manual edit";
        }
        await api.patch(`/api/v1/carriers/${editing.id}`, body);
      } else {
        await api.post("/api/v1/carriers", {
          ...form,
          assigned_node_id: form.assigned_node_id || undefined,
          codec_pref: form.codec_pref || undefined,
        });
      }
      setCreating(false);
      setEditing(null);
      reload();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "save failed");
    } finally {
      setBusy(false);
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
  async function showHistory(c: Carrier) {
    try {
      const h = await api.get<CarrierHistoryEntry[]>(`/api/v1/carriers/${c.id}/node-history`);
      setHistory(h);
      setHistoryFor(c);
    } catch (e) {
      setErr(e instanceof Error ? e.message : "history failed");
    }
  }

  return (
    <div className="space-y-4">
      <header className="flex items-center justify-between">
        <div>
          <h2 className="text-xl font-semibold tracking-tight text-slate-800">Carriers</h2>
          <p className="text-xs text-slate-500">Upstream termination providers. Each maps to one media node at a time.</p>
        </div>
        <button onClick={openCreate} className="inline-flex items-center gap-1.5 rounded-md bg-brand-600 px-3 py-1.5 text-sm font-medium text-white shadow-sm hover:bg-brand-700">
          <PlusIcon /> Add carrier
        </button>
      </header>

      {err && <div className="rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{err}</div>}

      <div className="overflow-hidden rounded-lg border border-slate-200 bg-white shadow-sm">
        <table className="min-w-full divide-y divide-slate-200 text-sm">
          <thead className="bg-slate-50 text-left text-xs uppercase tracking-wide text-slate-500">
            <tr>
              <th className="px-4 py-2">ID</th>
              <th className="px-4 py-2">Name</th>
              <th className="px-4 py-2">Endpoint</th>
              <th className="px-4 py-2">Node</th>
              <th className="px-4 py-2">Status</th>
              <th className="px-4 py-2 text-right">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100">
            {rows.length === 0 && (
              <tr><td colSpan={6} className="px-4 py-6 text-center text-slate-400">No carriers yet.</td></tr>
            )}
            {rows.map((r) => (
              <tr key={r.id} className="hover:bg-slate-50">
                <td className="px-4 py-2 font-mono text-slate-500">{r.id}</td>
                <td className="px-4 py-2 font-medium">{r.name}</td>
                <td className="px-4 py-2 font-mono text-xs">{r.transport}://{r.host}:{r.port}</td>
                <td className="px-4 py-2">{nodeName(r.assigned_node_id)}</td>
                <td className="px-4 py-2">{r.status}</td>
                <td className="px-4 py-2">
                  <div className="flex justify-end gap-1">
                    <button onClick={() => showHistory(r)} title="History" className="rounded px-2 py-0.5 text-xs text-slate-600 hover:bg-slate-100">history</button>
                    <button onClick={() => openEdit(r)} title="Edit" className="rounded p-1 text-slate-500 hover:bg-slate-100 hover:text-brand-600">
                      <PencilIcon />
                    </button>
                    <button onClick={() => del(r.id)} title="Delete" className="rounded p-1 text-slate-500 hover:bg-slate-100 hover:text-red-600">
                      <TrashIcon />
                    </button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <Modal
        open={creating || editing !== null}
        title={editing ? `Edit carrier #${editing.id}` : "Add carrier"}
        width="max-w-lg"
        onClose={() => { setCreating(false); setEditing(null); }}
        footer={
          <>
            <button onClick={() => { setCreating(false); setEditing(null); }} className="rounded border border-slate-300 px-3 py-1.5 text-sm hover:bg-slate-100">Cancel</button>
            <button onClick={save} disabled={busy || !form.name || !form.host} className="rounded bg-brand-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-brand-700 disabled:opacity-60">
              {busy ? "Saving…" : "Save"}
            </button>
          </>
        }
      >
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="block text-xs font-medium text-slate-500">Name</label>
            <input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm" />
          </div>
          <div>
            <label className="block text-xs font-medium text-slate-500">Host</label>
            <input value={form.host} onChange={(e) => setForm({ ...form, host: e.target.value })} placeholder="sip.carrier.com" className="mt-1 w-full rounded border border-slate-300 px-3 py-2 font-mono text-sm" />
          </div>
          <div>
            <label className="block text-xs font-medium text-slate-500">Port</label>
            <input type="number" value={form.port} onChange={(e) => setForm({ ...form, port: Number(e.target.value) })} className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm" />
          </div>
          <div>
            <label className="block text-xs font-medium text-slate-500">Transport</label>
            <select value={form.transport} onChange={(e) => setForm({ ...form, transport: e.target.value as "udp" | "tcp" | "tls" })} className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm">
              <option value="udp">udp</option>
              <option value="tcp">tcp</option>
              <option value="tls">tls</option>
            </select>
          </div>
          <div>
            <label className="block text-xs font-medium text-slate-500">Media node</label>
            <select value={form.assigned_node_id} onChange={(e) => setForm({ ...form, assigned_node_id: Number(e.target.value) })} className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm">
              <option value={0}>— none —</option>
              {mediaNodes.map((n) => <option key={n.id} value={n.id}>{n.name}</option>)}
            </select>
          </div>
          <div>
            <label className="block text-xs font-medium text-slate-500">Codec pref</label>
            <input value={form.codec_pref} onChange={(e) => setForm({ ...form, codec_pref: e.target.value })} placeholder="g711a,g711u" className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm" />
          </div>
          {editing && (
            <div className="col-span-2">
              <label className="block text-xs font-medium text-slate-500">Status</label>
              <select value={form.status} onChange={(e) => setForm({ ...form, status: e.target.value })} className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm">
                <option value="active">active</option>
                <option value="paused">paused</option>
                <option value="disabled">disabled</option>
              </select>
            </div>
          )}
          <div className="col-span-2">
            <label className="block text-xs font-medium text-slate-500">Notes</label>
            <textarea value={form.notes} onChange={(e) => setForm({ ...form, notes: e.target.value })} rows={2} className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm" />
          </div>
        </div>
      </Modal>

      <Modal
        open={historyFor !== null}
        title={historyFor ? `Node assignment history — ${historyFor.name}` : ""}
        onClose={() => setHistoryFor(null)}
        width="max-w-xl"
      >
        <ul className="divide-y divide-slate-100">
          {history.length === 0 && <li className="py-3 text-center text-slate-400">No changes.</li>}
          {history.map((h) => (
            <li key={h.id} className="flex items-baseline justify-between py-2 text-sm">
              <span>
                <code className="text-xs text-slate-500">{new Date(h.changed_at).toLocaleString()}</code>{" "}
                {nodeName(h.old_node_id)} → <strong>{nodeName(h.new_node_id)}</strong>
              </span>
              <span className="text-xs text-slate-500">{h.reason ?? ""}</span>
            </li>
          ))}
        </ul>
      </Modal>
    </div>
  );
}
