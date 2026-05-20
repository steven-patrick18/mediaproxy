import { useEffect, useState } from "react";
import { api, type IPGroup, type IPGroupMember, type NodeIP } from "../api";

export default function IPGroups() {
  const [groups, setGroups] = useState<IPGroup[]>([]);
  const [ips, setIps] = useState<NodeIP[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [showForm, setShowForm] = useState(false);
  const [form, setForm] = useState({ name: "", notes: "", ip_ids: [] as number[] });
  const [busy, setBusy] = useState(false);
  const [openGroup, setOpenGroup] = useState<number | null>(null);
  const [members, setMembers] = useState<IPGroupMember[]>([]);

  function reload() {
    Promise.all([
      api.get<IPGroup[]>("/api/v1/ip-groups"),
      api.get<NodeIP[]>("/api/v1/node-ips"),
    ])
      .then(([g, i]) => {
        setGroups(g);
        setIps(i);
      })
      .catch((e) => setErr(e.message));
  }
  useEffect(reload, []);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setErr(null);
    setBusy(true);
    try {
      await api.post("/api/v1/ip-groups", {
        name: form.name,
        notes: form.notes || undefined,
        ip_ids: form.ip_ids,
      });
      setForm({ name: "", notes: "", ip_ids: [] });
      setShowForm(false);
      reload();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "create failed");
    } finally {
      setBusy(false);
    }
  }
  async function openMembers(groupID: number) {
    try {
      const m = await api.get<IPGroupMember[]>(`/api/v1/ip-groups/${groupID}/members`);
      setMembers(m);
      setOpenGroup(groupID);
    } catch (e) {
      setErr(e instanceof Error ? e.message : "load members failed");
    }
  }
  async function addMember(groupID: number, ipID: number) {
    try {
      await api.post(`/api/v1/ip-groups/${groupID}/members`, { ip_id: ipID });
      openMembers(groupID);
      reload();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "add failed");
    }
  }
  async function removeMember(groupID: number, ipID: number) {
    if (!confirm("Remove this IP from the group?")) return;
    try {
      await api.del<void>(`/api/v1/ip-groups/${groupID}/members/${ipID}`);
      openMembers(groupID);
      reload();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "remove failed");
    }
  }
  async function del(id: number) {
    if (!confirm("Delete group?")) return;
    try {
      await api.del<void>(`/api/v1/ip-groups/${id}`);
      reload();
      if (openGroup === id) setOpenGroup(null);
    } catch (e) {
      setErr(e instanceof Error ? e.message : "delete failed");
    }
  }

  const availableIPs = ips.filter((ip) => ip.status === "active" || ip.status === "reserve");
  const inGroup = new Set(members.map((m) => m.ip_id));
  const addable = availableIPs.filter((ip) => !inGroup.has(ip.id));

  return (
    <div className="space-y-4">
      <header className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-semibold tracking-tight">IP Groups</h2>
          <p className="text-sm text-slate-500">
            Named subsets of the IP pool. Each IP can belong to only one active group at a time.
          </p>
        </div>
        <button onClick={() => setShowForm((v) => !v)} className="rounded-md bg-brand-600 px-3 py-2 text-sm font-medium text-white shadow-sm hover:bg-brand-700">
          {showForm ? "Cancel" : "Add group"}
        </button>
      </header>

      {err && (
        <div className="rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{err}</div>
      )}

      {showForm && (
        <form onSubmit={submit} className="grid grid-cols-1 gap-3 rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
          <div>
            <label className="block text-xs font-medium uppercase tracking-wide text-slate-500">Name</label>
            <input required value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm" />
          </div>
          <div>
            <label className="block text-xs font-medium uppercase tracking-wide text-slate-500">Notes</label>
            <input value={form.notes} onChange={(e) => setForm({ ...form, notes: e.target.value })} className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm" />
          </div>
          <div>
            <label className="block text-xs font-medium uppercase tracking-wide text-slate-500">
              Initial IPs ({form.ip_ids.length} selected)
            </label>
            <div className="mt-1 max-h-40 overflow-auto rounded border border-slate-300 p-2 text-sm">
              {availableIPs.length === 0 && <p className="text-slate-400">No IPs available — add some to the pool first.</p>}
              {availableIPs.map((ip) => (
                <label key={ip.id} className="block">
                  <input
                    type="checkbox"
                    checked={form.ip_ids.includes(ip.id)}
                    onChange={(e) => {
                      setForm((f) => ({
                        ...f,
                        ip_ids: e.target.checked ? [...f.ip_ids, ip.id] : f.ip_ids.filter((x) => x !== ip.id),
                      }));
                    }}
                    className="mr-2"
                  />
                  <span className="font-mono">{ip.ip_address}</span>{" "}
                  <span className="text-xs text-slate-500">(node #{ip.node_id})</span>
                </label>
              ))}
            </div>
          </div>
          <div>
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
              <th className="px-4 py-2">Status</th>
              <th className="px-4 py-2">IP count</th>
              <th className="px-4 py-2">Notes</th>
              <th className="px-4 py-2" />
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100">
            {groups.length === 0 && (
              <tr><td colSpan={6} className="px-4 py-6 text-center text-slate-400">No groups yet.</td></tr>
            )}
            {groups.map((g) => (
              <tr key={g.id}>
                <td className="px-4 py-2 font-mono text-slate-500">{g.id}</td>
                <td className="px-4 py-2 font-medium">{g.name}</td>
                <td className="px-4 py-2">{g.status}</td>
                <td className="px-4 py-2">{g.ip_count}</td>
                <td className="px-4 py-2 text-xs text-slate-500">{g.notes ?? ""}</td>
                <td className="px-4 py-2 text-right text-xs space-x-3">
                  <button onClick={() => openMembers(g.id)} className="text-slate-600 hover:underline">Members</button>
                  <button onClick={() => del(g.id)} className="text-red-600 hover:underline">Delete</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {openGroup !== null && (
        <div className="rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
          <div className="mb-3 flex items-center justify-between">
            <h3 className="text-sm font-semibold uppercase tracking-wide text-slate-500">
              Members of group #{openGroup}
            </h3>
            <button onClick={() => setOpenGroup(null)} className="text-xs text-slate-500 hover:underline">close</button>
          </div>

          <ul className="mb-3 divide-y divide-slate-100 text-sm">
            {members.length === 0 && <li className="py-3 text-slate-400">No members.</li>}
            {members.map((m) => (
              <li key={m.ip_id} className="flex items-center justify-between py-2">
                <code className="font-mono">{m.ip_address}</code>
                <button onClick={() => removeMember(openGroup, m.ip_id)} className="text-xs text-red-600 hover:underline">Remove</button>
              </li>
            ))}
          </ul>

          {addable.length > 0 && (
            <div className="text-sm">
              Add IP:{" "}
              <select
                defaultValue=""
                onChange={(e) => {
                  if (e.target.value) addMember(openGroup, Number(e.target.value));
                }}
                className="rounded border border-slate-300 px-2 py-1"
              >
                <option value="">— select —</option>
                {addable.map((ip) => (
                  <option key={ip.id} value={ip.id}>{ip.ip_address}</option>
                ))}
              </select>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
