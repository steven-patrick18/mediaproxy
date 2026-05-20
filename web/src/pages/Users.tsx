import { useEffect, useState } from "react";
import { api, type AdminUserRow } from "../api";
import { useAuth } from "../auth";
import Modal from "../components/Modal";
import Help from "../components/Help";
import { PencilIcon, PlusIcon, TrashIcon } from "../components/Icons";

const roles: Array<AdminUserRow["role"]> = ["admin", "noc", "reseller", "viewer"];

export default function Users() {
  const { user } = useAuth();
  const [rows, setRows] = useState<AdminUserRow[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<AdminUserRow | null>(null);
  const [form, setForm] = useState({
    email: "",
    password: "",
    role: "viewer" as AdminUserRow["role"],
    status: "active" as AdminUserRow["status"],
  });
  const [busy, setBusy] = useState(false);

  function reload() {
    api.get<AdminUserRow[]>("/api/v1/admin-users").then(setRows).catch((e) => setErr(e.message));
  }
  useEffect(reload, []);

  const canManage = user?.role === "admin";

  function openCreate() {
    setForm({ email: "", password: "", role: "viewer", status: "active" });
    setCreating(true);
  }
  function openEdit(u: AdminUserRow) {
    setForm({ email: u.email, password: "", role: u.role, status: u.status });
    setEditing(u);
  }
  async function save() {
    setErr(null);
    setBusy(true);
    try {
      if (editing) {
        const body: Record<string, unknown> = { role: form.role, status: form.status };
        if (form.password) body.password = form.password;
        await api.patch<void>(`/api/v1/admin-users/${editing.id}`, body);
      } else {
        await api.post("/api/v1/admin-users", form);
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
    if (!confirm("Delete this admin user?")) return;
    try {
      await api.del<void>(`/api/v1/admin-users/${id}`);
      reload();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "delete failed");
    }
  }

  return (
    <div className="space-y-4">
      <header className="flex items-center justify-between">
        <div>
          <h2 className="text-xl font-semibold tracking-tight text-slate-800">Admin users</h2>
          <p className="text-xs text-slate-500">
            Roles: <strong>admin</strong> manages everything,{" "}
            <strong>noc</strong> operates (drain/flag) but can't change billing,{" "}
            <strong>reseller</strong> sees their own tenant only,{" "}
            <strong>viewer</strong> is read-only.
          </p>
        </div>
        <button
          onClick={openCreate}
          disabled={!canManage}
          className="inline-flex items-center gap-1.5 rounded-md bg-brand-600 px-3 py-1.5 text-sm font-medium text-white shadow-sm hover:bg-brand-700 disabled:opacity-60"
          title={canManage ? "" : "Only admins can manage users"}
        >
          <PlusIcon /> Add user
        </button>
      </header>

      {err && <div className="rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{err}</div>}
      {!canManage && (
        <div className="rounded border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-800">
          You're signed in as <strong>{user?.role}</strong>. Only <code>admin</code> can create or edit users.
        </div>
      )}

      <div className="overflow-hidden rounded-lg border border-slate-200 bg-white shadow-sm">
        <table className="min-w-full divide-y divide-slate-200 text-sm">
          <thead className="bg-slate-50 text-left text-xs uppercase tracking-wide text-slate-500">
            <tr>
              <th className="px-4 py-2">ID</th>
              <th className="px-4 py-2">Email</th>
              <th className="px-4 py-2">Role</th>
              <th className="px-4 py-2">Status</th>
              <th className="px-4 py-2">MFA</th>
              <th className="px-4 py-2">Created</th>
              <th className="px-4 py-2 text-right">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100">
            {rows.map((u) => (
              <tr key={u.id} className="hover:bg-slate-50">
                <td className="px-4 py-2 font-mono text-slate-500">{u.id}</td>
                <td className="px-4 py-2 font-mono">{u.email}</td>
                <td className="px-4 py-2"><span className="rounded bg-brand-50 px-2 py-0.5 text-xs text-brand-700">{u.role}</span></td>
                <td className="px-4 py-2">{u.status}</td>
                <td className="px-4 py-2 text-xs">{u.has_mfa ? "on" : "off"}</td>
                <td className="px-4 py-2 text-slate-500">{new Date(u.created_at).toLocaleString()}</td>
                <td className="px-4 py-2">
                  <div className="flex justify-end gap-1">
                    <button onClick={() => openEdit(u)} disabled={!canManage} title="Edit" className="rounded p-1 text-slate-500 hover:bg-slate-100 hover:text-brand-600 disabled:opacity-30">
                      <PencilIcon />
                    </button>
                    <button onClick={() => del(u.id)} disabled={!canManage || u.id === user?.id} title="Delete" className="rounded p-1 text-slate-500 hover:bg-slate-100 hover:text-red-600 disabled:opacity-30">
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
        title={editing ? `Edit user ${editing.email}` : "Add admin user"}
        onClose={() => { setCreating(false); setEditing(null); }}
        footer={
          <>
            <button onClick={() => { setCreating(false); setEditing(null); }} className="rounded border border-slate-300 px-3 py-1.5 text-sm hover:bg-slate-100">Cancel</button>
            <button onClick={save} disabled={busy || (!editing && (!form.email || form.password.length < 8))} className="rounded bg-brand-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-brand-700 disabled:opacity-60">
              {busy ? "Saving…" : "Save"}
            </button>
          </>
        }
      >
        <div className="space-y-3">
          {!editing && (
            <div>
              <label className="block text-xs font-medium text-slate-500">Email</label>
              <input type="email" value={form.email} onChange={(e) => setForm({ ...form, email: e.target.value })} className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm" />
            </div>
          )}
          <div>
            <label className="block text-xs font-medium text-slate-500">
              Password {editing && <span className="text-slate-400">(leave blank to keep current)</span>}
            </label>
            <input type="password" value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm" />
          </div>
          <div>
            <label className="block text-xs font-medium text-slate-500">
              Role
              <Help>
                <strong>admin</strong> — full control over everything including admin users.
                <br /><strong>noc</strong> — operations: drain/flag/restart nodes, view all data, but cannot create/delete admin users or change billing.
                <br /><strong>reseller</strong> — tenant-scoped view (their own clients/CDRs only; UI scoping enforced when wired).
                <br /><strong>viewer</strong> — read-only across the panel.
              </Help>
            </label>
            <select value={form.role} onChange={(e) => setForm({ ...form, role: e.target.value as AdminUserRow["role"] })} className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm">
              {roles.map((r) => <option key={r} value={r}>{r}</option>)}
            </select>
          </div>
          {editing && (
            <div>
              <label className="block text-xs font-medium text-slate-500">Status</label>
              <select value={form.status} onChange={(e) => setForm({ ...form, status: e.target.value as AdminUserRow["status"] })} className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm">
                <option value="active">active</option>
                <option value="suspended">suspended</option>
              </select>
            </div>
          )}
        </div>
      </Modal>
    </div>
  );
}
