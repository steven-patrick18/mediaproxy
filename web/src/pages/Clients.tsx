import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api, type Client, type Reseller } from "../api";

export default function Clients() {
  const [rows, setRows] = useState<Client[]>([]);
  const [resellers, setResellers] = useState<Reseller[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [showForm, setShowForm] = useState(false);
  const [form, setForm] = useState({ name: "", reseller_id: 0 });
  const [busy, setBusy] = useState(false);

  function reload() {
    Promise.all([
      api.get<Client[]>("/api/v1/clients"),
      api.get<Reseller[]>("/api/v1/resellers"),
    ])
      .then(([c, r]) => {
        setRows(c);
        setResellers(r);
      })
      .catch((e) => setErr(e.message));
  }
  useEffect(reload, []);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setErr(null);
    setBusy(true);
    try {
      await api.post("/api/v1/clients", form);
      setForm({ name: "", reseller_id: 0 });
      setShowForm(false);
      reload();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "create failed");
    } finally {
      setBusy(false);
    }
  }
  async function del(id: number) {
    if (!confirm("Delete this client? Will also unassign their signaling IP.")) return;
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
          <h2 className="text-2xl font-semibold tracking-tight">Clients</h2>
          <p className="text-sm text-slate-500">
            End customers. Click a row to manage their signaling IP and dialer IPs.
          </p>
        </div>
        <button
          onClick={() => setShowForm((v) => !v)}
          disabled={resellers.length === 0}
          className="rounded-md bg-brand-600 px-3 py-2 text-sm font-medium text-white shadow-sm hover:bg-brand-700 disabled:opacity-60"
        >
          {showForm ? "Cancel" : "Add client"}
        </button>
      </header>

      {resellers.length === 0 && (
        <div className="rounded border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-800">
          Create a reseller first.
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
              Reseller
            </label>
            <select
              required
              value={form.reseller_id}
              onChange={(e) => setForm({ ...form, reseller_id: Number(e.target.value) })}
              className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm"
            >
              <option value={0}>— select —</option>
              {resellers.map((r) => (
                <option key={r.id} value={r.id}>
                  {r.name}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label className="block text-xs font-medium uppercase tracking-wide text-slate-500">
              Name
            </label>
            <input
              required
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
              placeholder="Client name"
              className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm"
            />
          </div>
          <div className="sm:col-span-2">
            <button
              type="submit"
              disabled={busy || form.reseller_id === 0}
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
              <th className="px-4 py-2">Reseller</th>
              <th className="px-4 py-2">Name</th>
              <th className="px-4 py-2">Status</th>
              <th className="px-4 py-2">Created</th>
              <th className="px-4 py-2" />
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100">
            {rows.length === 0 && (
              <tr>
                <td colSpan={6} className="px-4 py-6 text-center text-slate-400">
                  No clients yet.
                </td>
              </tr>
            )}
            {rows.map((r) => (
              <tr key={r.id} className="hover:bg-slate-50">
                <td className="px-4 py-2 font-mono text-slate-500">
                  <Link to={`/clients/${r.id}`} className="hover:underline">
                    {r.id}
                  </Link>
                </td>
                <td className="px-4 py-2 text-slate-600">{resellerName(r.reseller_id)}</td>
                <td className="px-4 py-2 font-medium">
                  <Link to={`/clients/${r.id}`} className="hover:underline">
                    {r.name}
                  </Link>
                </td>
                <td className="px-4 py-2">{r.status}</td>
                <td className="px-4 py-2 text-slate-500">
                  {new Date(r.created_at).toLocaleString()}
                </td>
                <td className="px-4 py-2 text-right">
                  <button
                    onClick={() => del(r.id)}
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
