import { useEffect, useState } from "react";
import { api, type Carrier, type CarrierHistoryEntry, type MediaNode } from "../api";
import Modal from "../components/Modal";
import Help from "../components/Help";
import { PencilIcon, PlusIcon, TrashIcon } from "../components/Icons";

const blankForm = {
  name: "",
  host: "",
  port: 5060,
  transport: "udp" as "udp" | "tcp" | "tls",
  assigned_node_ids: [] as number[],
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
      assigned_node_ids: [...c.assigned_node_ids],
      codec_pref: c.codec_pref ?? "",
      status: c.status,
      notes: c.notes ?? "",
    });
    setEditing(c);
  }
  function toggleNode(id: number) {
    setForm((f) => {
      const has = f.assigned_node_ids.includes(id);
      return {
        ...f,
        assigned_node_ids: has
          ? f.assigned_node_ids.filter((x) => x !== id)
          : [...f.assigned_node_ids, id],
      };
    });
  }

  async function save() {
    setErr(null);
    setBusy(true);
    try {
      if (editing) {
        const sortedOld = [...editing.assigned_node_ids].sort((a, b) => a - b).join(",");
        const sortedNew = [...form.assigned_node_ids].sort((a, b) => a - b).join(",");
        const nodesChanged = sortedOld !== sortedNew;
        const body: Record<string, unknown> = {
          name: form.name,
          host: form.host,
          port: form.port,
          transport: form.transport,
          codec_pref: form.codec_pref || undefined,
          status: form.status,
          notes: form.notes,
        };
        if (nodesChanged) {
          body.assigned_node_ids = form.assigned_node_ids;
          const r = prompt("Reason for changing this carrier's media nodes?");
          if (r === null) {
            setBusy(false);
            return;
          }
          body.reason = r || "manual edit";
        }
        await api.patch(`/api/v1/carriers/${editing.id}`, body);
      } else {
        await api.post("/api/v1/carriers", {
          name: form.name,
          host: form.host,
          port: form.port,
          transport: form.transport,
          assigned_node_ids: form.assigned_node_ids,
          codec_pref: form.codec_pref || undefined,
          notes: form.notes || undefined,
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
          <p className="text-xs text-slate-500">
            Upstream termination providers. Each can be served by multiple media nodes — pick a set
            in the carrier's edit dialog.
          </p>
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
              <th className="px-4 py-2">Media nodes</th>
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
                <td className="px-4 py-2 text-xs">
                  {r.assigned_node_ids.length === 0 ? (
                    <span className="text-slate-400">— none —</span>
                  ) : (
                    <div className="flex flex-wrap gap-1">
                      {r.assigned_node_ids.map((id) => (
                        <span key={id} className="rounded bg-brand-50 px-1.5 py-0.5 text-brand-700">
                          {nodeName(id)}
                        </span>
                      ))}
                    </div>
                  )}
                </td>
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
        width="max-w-2xl"
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
            <label className="block text-xs font-medium text-slate-500">
              Name
              <Help>Display name only. Anything that's meaningful to you ("Telnyx-DE", "Twilio-prod", etc.).</Help>
            </label>
            <input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm" />
          </div>
          <div>
            <label className="block text-xs font-medium text-slate-500">
              Host
              <Help>
                The carrier's SIP endpoint — hostname or IP. Kamailio will send INVITEs here.
                For carriers with multiple IPs, use the hostname they give you so they can
                load-balance behind their own A-records.
              </Help>
            </label>
            <input value={form.host} onChange={(e) => setForm({ ...form, host: e.target.value })} placeholder="sip.carrier.com" className="mt-1 w-full rounded border border-slate-300 px-3 py-2 font-mono text-sm" />
          </div>
          <div>
            <label className="block text-xs font-medium text-slate-500">
              Port
              <Help>SIP signaling port on the carrier side. <strong>5060</strong> for UDP/TCP, <strong>5061</strong> for TLS.</Help>
            </label>
            <input type="number" value={form.port} onChange={(e) => setForm({ ...form, port: Number(e.target.value) })} className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm" />
          </div>
          <div>
            <label className="block text-xs font-medium text-slate-500">
              Transport
              <Help>
                Wire protocol for SIP signaling.
                <ul className="ml-4 mt-1 list-disc">
                  <li><code>udp</code> — most common, lowest latency</li>
                  <li><code>tcp</code> — preferred for large messages (lots of routing headers)</li>
                  <li><code>tls</code> — encrypted; required by some carriers</li>
                </ul>
              </Help>
            </label>
            <select value={form.transport} onChange={(e) => setForm({ ...form, transport: e.target.value as "udp" | "tcp" | "tls" })} className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm">
              <option value="udp">udp</option>
              <option value="tcp">tcp</option>
              <option value="tls">tls</option>
            </select>
          </div>
          <div className="col-span-2">
            <label className="block text-xs font-medium text-slate-500">
              Media nodes ({form.assigned_node_ids.length} selected)
              <Help>
                Which media nodes carry the RTP for calls to this carrier. Pick one or
                multiple — rotation strategies (set on the Assignment) decide which active
                node handles a given call, and if one is full or offline the next is used.
                You can change this set later from this dialog; every change is recorded
                in this carrier's history.
              </Help>
            </label>
            <p className="mt-1 text-xs text-slate-500">
              Routing will pick any active node when placing a call to this carrier. Selecting
              multiple gives you failover.
            </p>
            <div className="mt-2 max-h-40 overflow-auto rounded border border-slate-300 p-2 text-sm">
              {mediaNodes.length === 0 && (
                <p className="text-slate-400">No media nodes yet — add one in Infrastructure → Nodes first.</p>
              )}
              {mediaNodes.map((n) => (
                <label key={n.id} className="flex items-center gap-2 py-1">
                  <input
                    type="checkbox"
                    checked={form.assigned_node_ids.includes(n.id)}
                    onChange={() => toggleNode(n.id)}
                  />
                  <span className="font-medium">{n.name}</span>
                  <span className="text-xs text-slate-500">{n.host_ip}{n.region ? " · " + n.region : ""}</span>
                  <span
                    className={
                      "ml-auto rounded px-2 py-0.5 text-xs " +
                      (n.status === "online" ? "bg-emerald-100 text-emerald-800" : "bg-slate-200 text-slate-700")
                    }
                  >
                    {n.status}
                  </span>
                </label>
              ))}
            </div>
          </div>
          <div>
            <label className="block text-xs font-medium text-slate-500">Codec pref</label>
            <input value={form.codec_pref} onChange={(e) => setForm({ ...form, codec_pref: e.target.value })} placeholder="g711a,g711u" className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm" />
          </div>
          {editing && (
            <div>
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
          {history.map((h) => {
            const isAdd = !h.old_node_id && h.new_node_id;
            const isRemove = h.old_node_id && !h.new_node_id;
            return (
              <li key={h.id} className="flex items-baseline justify-between py-2 text-sm">
                <span>
                  <code className="text-xs text-slate-500">{new Date(h.changed_at).toLocaleString()}</code>{" "}
                  {isAdd && (<><span className="rounded bg-emerald-100 px-1.5 py-0.5 text-xs text-emerald-800">added</span> {nodeName(h.new_node_id)}</>)}
                  {isRemove && (<><span className="rounded bg-red-100 px-1.5 py-0.5 text-xs text-red-800">removed</span> {nodeName(h.old_node_id)}</>)}
                  {!isAdd && !isRemove && (<>{nodeName(h.old_node_id)} → <strong>{nodeName(h.new_node_id)}</strong></>)}
                </span>
                <span className="text-xs text-slate-500">{h.reason ?? ""}</span>
              </li>
            );
          })}
        </ul>
      </Modal>
    </div>
  );
}
