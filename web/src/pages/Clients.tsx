import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api, type Client, type Reseller, type SignalingIP } from "../api";
import Modal from "../components/Modal";
import Help from "../components/Help";
import { PencilIcon, PlusIcon, TrashIcon } from "../components/Icons";

export default function Clients() {
  const [rows, setRows] = useState<Client[]>([]);
  const [resellers, setResellers] = useState<Reseller[]>([]);
  const [sigs, setSigs] = useState<SignalingIP[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<Client | null>(null);
  const [form, setForm] = useState({
    reseller_id: 0,
    name: "",
    status: "active",
    notes: "",
  });

  function reload() {
    Promise.all([
      api.get<Client[]>("/api/v1/clients"),
      api.get<Reseller[]>("/api/v1/resellers"),
      api.get<SignalingIP[]>("/api/v1/signaling-ips"),
    ])
      .then(([c, r, s]) => {
        setRows(c);
        setResellers(r);
        setSigs(s);
      })
      .catch((e) => setErr(e.message));
  }
  useEffect(reload, []);

  const sigForClient = (clientID: number) =>
    sigs.find((s) => s.assigned_client_id === clientID) ?? null;

  function openCreate() {
    setForm({ reseller_id: resellers[0]?.id ?? 0, name: "", status: "active", notes: "" });
    setCreating(true);
  }
  function openEdit(c: Client) {
    setForm({
      reseller_id: c.reseller_id,
      name: c.name,
      status: c.status,
      notes: (c as Client & { notes?: string }).notes ?? "",
    });
    setEditing(c);
  }

  async function save() {
    setErr(null);
    setBusy(true);
    try {
      if (editing) {
        await api.patch(`/api/v1/clients/${editing.id}`, {
          name: form.name,
          reseller_id: form.reseller_id,
          status: form.status,
          notes: form.notes,
        });
      } else {
        await api.post("/api/v1/clients", { reseller_id: form.reseller_id, name: form.name });
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
    if (!confirm("Delete client? Will also unassign their signaling IP.")) return;
    try {
      await api.del<void>(`/api/v1/clients/${id}`);
      reload();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "delete failed");
    }
  }

  const resellerName = (id: number) => resellers.find((r) => r.id === id)?.name ?? `#${id}`;

  return (
    <div className="space-y-4">
      <header className="flex items-center justify-between">
        <div>
          <h2 className="text-xl font-semibold tracking-tight text-slate-800">Clients</h2>
          <p className="text-xs text-slate-500">End customers. Click a row to manage signaling IP and dialer IPs.</p>
        </div>
        <button
          onClick={openCreate}
          disabled={resellers.length === 0}
          className="inline-flex items-center gap-1.5 rounded-md bg-brand-600 px-3 py-1.5 text-sm font-medium text-white shadow-sm hover:bg-brand-700 disabled:opacity-60"
        >
          <PlusIcon /> Add client
        </button>
      </header>

      {err && (
        <div className="rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{err}</div>
      )}

      <div className="overflow-hidden rounded-lg border border-slate-200 bg-white shadow-sm">
        <table className="min-w-full divide-y divide-slate-200 text-sm">
          <thead className="bg-slate-50 text-left text-xs uppercase tracking-wide text-slate-500">
            <tr>
              <th className="px-4 py-2">ID</th>
              <th className="px-4 py-2">Reseller</th>
              <th className="px-4 py-2">Name</th>
              <th className="px-4 py-2">Signaling IP</th>
              <th className="px-4 py-2">Status</th>
              <th className="px-4 py-2">Created</th>
              <th className="px-4 py-2 text-right">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100">
            {rows.length === 0 && (
              <tr>
                <td colSpan={7} className="px-4 py-6 text-center text-slate-400">No clients yet.</td>
              </tr>
            )}
            {rows.map((r) => (
              <tr key={r.id} className="hover:bg-slate-50">
                <td className="px-4 py-2 font-mono text-slate-500">
                  <Link to={`/clients/${r.id}`} className="hover:underline">{r.id}</Link>
                </td>
                <td className="px-4 py-2">{resellerName(r.reseller_id)}</td>
                <td className="px-4 py-2 font-medium">
                  <Link to={`/clients/${r.id}`} className="hover:underline">{r.name}</Link>
                </td>
                <td className="px-4 py-2">
                  {(() => {
                    const sig = sigForClient(r.id);
                    return sig ? (
                      <span className="font-mono text-sm">{sig.ip_address}</span>
                    ) : (
                      <span className="text-xs text-slate-400">— not assigned —</span>
                    );
                  })()}
                </td>
                <td className="px-4 py-2">{r.status}</td>
                <td className="px-4 py-2 text-slate-500">{new Date(r.created_at).toLocaleString()}</td>
                <td className="px-4 py-2">
                  <div className="flex justify-end gap-1">
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
        title={editing ? `Edit client #${editing.id}` : "Add client"}
        onClose={() => { setCreating(false); setEditing(null); }}
        footer={
          <>
            <button onClick={() => { setCreating(false); setEditing(null); }} className="rounded border border-slate-300 px-3 py-1.5 text-sm hover:bg-slate-100">Cancel</button>
            <button onClick={save} disabled={busy || !form.name || !form.reseller_id} className="rounded bg-brand-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-brand-700 disabled:opacity-60">
              {busy ? "Saving…" : "Save"}
            </button>
          </>
        }
      >
        <div className="space-y-3">
          <div>
            <label className="block text-xs font-medium text-slate-500">
              Reseller
              <Help>
                The top-level tenant that owns this client. Resellers can be billed
                separately and have their own clients. If you sell directly to end
                customers, put them all under one "Default" reseller.
              </Help>
            </label>
            <select
              value={form.reseller_id}
              onChange={(e) => setForm({ ...form, reseller_id: Number(e.target.value) })}
              className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm"
            >
              <option value={0}>— select —</option>
              {resellers.map((r) => <option key={r.id} value={r.id}>{r.name}</option>)}
            </select>
          </div>
          <div>
            <label className="block text-xs font-medium text-slate-500">
              Name
              <Help>End-customer name. Shown on CDRs, the live-calls list, etc.</Help>
            </label>
            <input
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
              className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm"
            />
          </div>
          {editing && (
            <>
              <div>
                <label className="block text-xs font-medium text-slate-500">
                  Status
                  <Help>
                    <strong>active</strong> — calls flow normally.
                    <br /><strong>suspended</strong> — Kamailio returns 403 to this client's
                    dialer IPs; signaling-IP cache is purged. Restore later by flipping back to active.
                    <br /><strong>deleted</strong> — soft-delete marker; kept for audit/CDR history.
                  </Help>
                </label>
                <select
                  value={form.status}
                  onChange={(e) => setForm({ ...form, status: e.target.value })}
                  className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm"
                >
                  <option value="active">active</option>
                  <option value="suspended">suspended</option>
                  <option value="deleted">deleted</option>
                </select>
              </div>
              <div>
                <label className="block text-xs font-medium text-slate-500">Notes</label>
                <textarea
                  value={form.notes}
                  onChange={(e) => setForm({ ...form, notes: e.target.value })}
                  rows={3}
                  className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm"
                />
              </div>
            </>
          )}
        </div>
      </Modal>
    </div>
  );
}
