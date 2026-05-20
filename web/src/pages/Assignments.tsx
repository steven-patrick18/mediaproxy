import { useEffect, useState } from "react";
import {
  api,
  type Assignment,
  type Carrier,
  type Client,
  type IPGroup,
} from "../api";

export default function Assignments() {
  const [rows, setRows] = useState<Assignment[]>([]);
  const [clients, setClients] = useState<Client[]>([]);
  const [carriers, setCarriers] = useState<Carrier[]>([]);
  const [groups, setGroups] = useState<IPGroup[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [showForm, setShowForm] = useState(false);
  const [form, setForm] = useState({
    group_id: 0,
    client_id: 0,
    carrier_id: 0,
    rotation_strategy: "round_robin",
  });
  const [busy, setBusy] = useState(false);

  function reload() {
    Promise.all([
      api.get<Assignment[]>("/api/v1/assignments"),
      api.get<Client[]>("/api/v1/clients"),
      api.get<Carrier[]>("/api/v1/carriers"),
      api.get<IPGroup[]>("/api/v1/ip-groups"),
    ])
      .then(([a, c, ca, g]) => {
        setRows(a);
        setClients(c);
        setCarriers(ca);
        setGroups(g);
      })
      .catch((e) => setErr(e.message));
  }
  useEffect(reload, []);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setErr(null);
    setBusy(true);
    try {
      await api.post("/api/v1/assignments", form);
      setForm({ group_id: 0, client_id: 0, carrier_id: 0, rotation_strategy: "round_robin" });
      setShowForm(false);
      reload();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "create failed");
    } finally {
      setBusy(false);
    }
  }
  async function endIt(id: number) {
    if (!confirm("End this assignment?")) return;
    try {
      await api.del<void>(`/api/v1/assignments/${id}`);
      reload();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "end failed");
    }
  }
  const clientName = (id: number) => clients.find((c) => c.id === id)?.name ?? `#${id}`;
  const carrierName = (id: number) => carriers.find((c) => c.id === id)?.name ?? `#${id}`;
  const groupName = (id: number) => groups.find((g) => g.id === id)?.name ?? `#${id}`;

  const canCreate = clients.length > 0 && carriers.length > 0 && groups.length > 0;

  return (
    <div className="space-y-4">
      <header className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-semibold tracking-tight">Assignments</h2>
          <p className="text-sm text-slate-500">
            Bind an IP group to a (client, carrier) pair with a rotation strategy.
          </p>
        </div>
        <button
          onClick={() => setShowForm((v) => !v)}
          disabled={!canCreate}
          className="rounded-md bg-brand-600 px-3 py-2 text-sm font-medium text-white shadow-sm hover:bg-brand-700 disabled:opacity-60"
        >
          {showForm ? "Cancel" : "Add assignment"}
        </button>
      </header>

      {!canCreate && (
        <div className="rounded border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-800">
          Need at least one client, one carrier, and one IP group before creating assignments.
        </div>
      )}

      {err && (
        <div className="rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{err}</div>
      )}

      {showForm && (
        <form onSubmit={submit} className="grid grid-cols-1 gap-3 rounded-lg border border-slate-200 bg-white p-4 shadow-sm sm:grid-cols-4">
          <div>
            <label className="block text-xs font-medium uppercase tracking-wide text-slate-500">Client</label>
            <select required value={form.client_id} onChange={(e) => setForm({ ...form, client_id: Number(e.target.value) })} className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm">
              <option value={0}>— select —</option>
              {clients.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
            </select>
          </div>
          <div>
            <label className="block text-xs font-medium uppercase tracking-wide text-slate-500">Carrier</label>
            <select required value={form.carrier_id} onChange={(e) => setForm({ ...form, carrier_id: Number(e.target.value) })} className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm">
              <option value={0}>— select —</option>
              {carriers.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
            </select>
          </div>
          <div>
            <label className="block text-xs font-medium uppercase tracking-wide text-slate-500">IP group</label>
            <select required value={form.group_id} onChange={(e) => setForm({ ...form, group_id: Number(e.target.value) })} className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm">
              <option value={0}>— select —</option>
              {groups.map((g) => <option key={g.id} value={g.id}>{g.name} ({g.ip_count})</option>)}
            </select>
          </div>
          <div>
            <label className="block text-xs font-medium uppercase tracking-wide text-slate-500">Strategy</label>
            <select value={form.rotation_strategy} onChange={(e) => setForm({ ...form, rotation_strategy: e.target.value })} className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm">
              <option value="round_robin">round robin</option>
              <option value="random">random</option>
              <option value="sticky">sticky</option>
              <option value="least_used">least used</option>
              <option value="health_weighted">health weighted</option>
            </select>
          </div>
          <div className="sm:col-span-4">
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
              <th className="px-4 py-2">Client</th>
              <th className="px-4 py-2">Carrier</th>
              <th className="px-4 py-2">Group</th>
              <th className="px-4 py-2">Strategy</th>
              <th className="px-4 py-2">Status</th>
              <th className="px-4 py-2">When</th>
              <th className="px-4 py-2" />
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100">
            {rows.length === 0 && (
              <tr><td colSpan={8} className="px-4 py-6 text-center text-slate-400">No assignments yet.</td></tr>
            )}
            {rows.map((a) => (
              <tr key={a.id}>
                <td className="px-4 py-2 font-mono text-slate-500">{a.id}</td>
                <td className="px-4 py-2">{clientName(a.client_id)}</td>
                <td className="px-4 py-2">{carrierName(a.carrier_id)}</td>
                <td className="px-4 py-2">{groupName(a.group_id)}</td>
                <td className="px-4 py-2 text-xs">{a.rotation_strategy}</td>
                <td className="px-4 py-2">{a.status}</td>
                <td className="px-4 py-2 text-xs text-slate-500">{new Date(a.assigned_at).toLocaleString()}</td>
                <td className="px-4 py-2 text-right">
                  {a.status === "active" && (
                    <button onClick={() => endIt(a.id)} className="text-xs text-red-600 hover:underline">End</button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
