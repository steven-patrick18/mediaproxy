import { useEffect, useState } from "react";
import { api, type Reseller } from "../api";
import Modal from "../components/Modal";
import { PencilIcon, PlusIcon, TrashIcon } from "../components/Icons";

export default function Resellers() {
  const [rows, setRows] = useState<Reseller[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<Reseller | null>(null);
  const [form, setForm] = useState({ name: "", status: "active", notes: "" });

  function reload() {
    api
      .get<Reseller[]>("/api/v1/resellers")
      .then(setRows)
      .catch((e) => setErr(e.message));
  }
  useEffect(reload, []);

  function openCreate() {
    setForm({ name: "", status: "active", notes: "" });
    setCreating(true);
  }
  function openEdit(r: Reseller) {
    setForm({ name: r.name, status: r.status, notes: r.notes ?? "" });
    setEditing(r);
  }

  async function save() {
    setErr(null);
    setBusy(true);
    try {
      if (editing) {
        await api.patch(`/api/v1/resellers/${editing.id}`, {
          name: form.name,
          status: form.status,
          notes: form.notes,
        });
      } else {
        await api.post("/api/v1/resellers", { name: form.name });
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
    if (!confirm("Delete this reseller? Will fail if it still has clients.")) return;
    try {
      await api.del<void>(`/api/v1/resellers/${id}`);
      reload();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "delete failed");
    }
  }

  return (
    <div className="space-y-4">
      <header className="flex items-center justify-between">
        <div>
          <h2 className="text-xl font-semibold tracking-tight text-slate-800">Resellers</h2>
          <p className="text-xs text-slate-500">Top-level tenants.</p>
        </div>
        <button
          onClick={openCreate}
          className="inline-flex items-center gap-1.5 rounded-md bg-brand-600 px-3 py-1.5 text-sm font-medium text-white shadow-sm hover:bg-brand-700"
        >
          <PlusIcon /> Add reseller
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
              <th className="px-4 py-2">Name</th>
              <th className="px-4 py-2">Status</th>
              <th className="px-4 py-2">Notes</th>
              <th className="px-4 py-2">Created</th>
              <th className="px-4 py-2 text-right">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100">
            {rows.length === 0 && (
              <tr>
                <td colSpan={6} className="px-4 py-6 text-center text-slate-400">
                  No resellers yet.
                </td>
              </tr>
            )}
            {rows.map((r) => (
              <tr key={r.id} className="hover:bg-slate-50">
                <td className="px-4 py-2 font-mono text-slate-500">{r.id}</td>
                <td className="px-4 py-2 font-medium">{r.name}</td>
                <td className="px-4 py-2">{r.status}</td>
                <td className="px-4 py-2 text-xs text-slate-500">{r.notes ?? ""}</td>
                <td className="px-4 py-2 text-slate-500">{new Date(r.created_at).toLocaleString()}</td>
                <td className="px-4 py-2">
                  <div className="flex justify-end gap-1">
                    <button
                      onClick={() => openEdit(r)}
                      title="Edit"
                      className="rounded p-1 text-slate-500 hover:bg-slate-100 hover:text-brand-600"
                    >
                      <PencilIcon />
                    </button>
                    <button
                      onClick={() => del(r.id)}
                      title="Delete"
                      className="rounded p-1 text-slate-500 hover:bg-slate-100 hover:text-red-600"
                    >
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
        title={editing ? `Edit reseller #${editing.id}` : "Add reseller"}
        onClose={() => {
          setCreating(false);
          setEditing(null);
        }}
        footer={
          <>
            <button
              onClick={() => {
                setCreating(false);
                setEditing(null);
              }}
              className="rounded border border-slate-300 px-3 py-1.5 text-sm hover:bg-slate-100"
            >
              Cancel
            </button>
            <button
              onClick={save}
              disabled={busy || !form.name}
              className="rounded bg-brand-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-brand-700 disabled:opacity-60"
            >
              {busy ? "Saving…" : "Save"}
            </button>
          </>
        }
      >
        <div className="space-y-3">
          <div>
            <label className="block text-xs font-medium text-slate-500">Name</label>
            <input
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
              className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm"
            />
          </div>
          {editing && (
            <div>
              <label className="block text-xs font-medium text-slate-500">Status</label>
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
          )}
          {editing && (
            <div>
              <label className="block text-xs font-medium text-slate-500">Notes</label>
              <textarea
                value={form.notes}
                onChange={(e) => setForm({ ...form, notes: e.target.value })}
                rows={3}
                className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm"
              />
            </div>
          )}
        </div>
      </Modal>
    </div>
  );
}
