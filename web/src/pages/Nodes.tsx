import { useEffect, useState } from "react";
import { api, type MediaNode } from "../api";

export default function Nodes() {
  const [rows, setRows] = useState<MediaNode[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [showForm, setShowForm] = useState(false);
  const [form, setForm] = useState({
    name: "",
    role: "media" as "media" | "sip_proxy",
    host_ip: "",
    region: "",
    max_calls: 2500,
  });
  const [busy, setBusy] = useState(false);
  const [created, setCreated] = useState<MediaNode | null>(null);

  function reload() {
    api
      .get<MediaNode[]>("/api/v1/nodes")
      .then(setRows)
      .catch((e) => setErr(e.message));
  }

  useEffect(() => {
    reload();
  }, []);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setErr(null);
    setBusy(true);
    try {
      const node = await api.post<MediaNode>("/api/v1/nodes", form);
      setCreated(node);
      setShowForm(false);
      setForm({ name: "", role: "media", host_ip: "", region: "", max_calls: 2500 });
      reload();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "create failed");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="space-y-4">
      <header className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-semibold tracking-tight">Media Nodes</h2>
          <p className="text-sm text-slate-500">
            Hosts that run RTPEngine and carry media. SIP proxies live here too.
          </p>
        </div>
        <button
          onClick={() => setShowForm((v) => !v)}
          className="rounded-md bg-brand-600 px-3 py-2 text-sm font-medium text-white shadow-sm hover:bg-brand-700"
        >
          {showForm ? "Cancel" : "Add node"}
        </button>
      </header>

      {err && (
        <div className="rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
          {err}
        </div>
      )}

      {created && (
        <div className="rounded border border-amber-300 bg-amber-50 p-4 text-sm">
          <div className="font-medium text-amber-900">
            Node "{created.name}" created. Save its agent token now — it won't be shown again:
          </div>
          <code className="mt-2 block break-all rounded bg-white p-2 font-mono text-xs">
            {created.agent_token}
          </code>
          <button
            onClick={() => setCreated(null)}
            className="mt-2 text-xs text-amber-900 underline"
          >
            Dismiss
          </button>
        </div>
      )}

      {showForm && (
        <form
          onSubmit={submit}
          className="grid grid-cols-1 gap-3 rounded-lg border border-slate-200 bg-white p-4 shadow-sm sm:grid-cols-2"
        >
          <div>
            <label className="block text-xs font-medium uppercase tracking-wide text-slate-500">
              Name
            </label>
            <input
              required
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
              className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm"
              placeholder="media-node-1"
            />
          </div>
          <div>
            <label className="block text-xs font-medium uppercase tracking-wide text-slate-500">
              Role
            </label>
            <select
              value={form.role}
              onChange={(e) => setForm({ ...form, role: e.target.value as "media" | "sip_proxy" })}
              className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm"
            >
              <option value="media">media (RTPEngine)</option>
              <option value="sip_proxy">sip_proxy (Kamailio)</option>
            </select>
          </div>
          <div>
            <label className="block text-xs font-medium uppercase tracking-wide text-slate-500">
              Host IP
            </label>
            <input
              required
              value={form.host_ip}
              onChange={(e) => setForm({ ...form, host_ip: e.target.value })}
              className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm"
              placeholder="45.77.156.60"
            />
          </div>
          <div>
            <label className="block text-xs font-medium uppercase tracking-wide text-slate-500">
              Region
            </label>
            <input
              value={form.region}
              onChange={(e) => setForm({ ...form, region: e.target.value })}
              className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm"
              placeholder="us-east"
            />
          </div>
          <div>
            <label className="block text-xs font-medium uppercase tracking-wide text-slate-500">
              Max calls
            </label>
            <input
              type="number"
              min={0}
              value={form.max_calls}
              onChange={(e) => setForm({ ...form, max_calls: Number(e.target.value) })}
              className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm"
            />
          </div>
          <div className="flex items-end sm:col-span-2">
            <button
              type="submit"
              disabled={busy}
              className="rounded-md bg-brand-600 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-brand-700 disabled:opacity-60"
            >
              {busy ? "Saving…" : "Create node"}
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
              <th className="px-4 py-2">Role</th>
              <th className="px-4 py-2">Host IP</th>
              <th className="px-4 py-2">Region</th>
              <th className="px-4 py-2">Max calls</th>
              <th className="px-4 py-2">Status</th>
              <th className="px-4 py-2">Last seen</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100">
            {rows.length === 0 && (
              <tr>
                <td colSpan={8} className="px-4 py-6 text-center text-slate-400">
                  No nodes yet.
                </td>
              </tr>
            )}
            {rows.map((n) => (
              <tr key={n.id}>
                <td className="px-4 py-2 font-mono text-slate-500">{n.id}</td>
                <td className="px-4 py-2 font-medium">{n.name}</td>
                <td className="px-4 py-2 text-slate-600">{n.role}</td>
                <td className="px-4 py-2 font-mono text-slate-600">{n.host_ip}</td>
                <td className="px-4 py-2 text-slate-600">{n.region ?? "—"}</td>
                <td className="px-4 py-2 text-slate-600">{n.max_calls}</td>
                <td className="px-4 py-2">
                  <span
                    className={
                      "rounded px-2 py-0.5 text-xs " +
                      (n.status === "online"
                        ? "bg-green-100 text-green-800"
                        : n.status === "draining"
                          ? "bg-amber-100 text-amber-800"
                          : "bg-slate-200 text-slate-700")
                    }
                  >
                    {n.status}
                  </span>
                </td>
                <td className="px-4 py-2 text-slate-500">
                  {n.last_seen_at ? new Date(n.last_seen_at).toLocaleString() : "—"}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
