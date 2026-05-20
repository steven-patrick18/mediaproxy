import { useEffect, useState } from "react";
import { api, type MediaNode, type NodeIP } from "../api";

export default function IPPool() {
  const [rows, setRows] = useState<NodeIP[]>([]);
  const [nodes, setNodes] = useState<MediaNode[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [filter, setFilter] = useState(0);
  const [showAdd, setShowAdd] = useState(false);
  const [form, setForm] = useState({
    node_id: 0,
    mode: "single" as "single" | "bulk",
    ip_address: "",
    cidr: "",
    lease_block: "",
    purchased_from: "",
  });
  const [busy, setBusy] = useState(false);

  function reload() {
    Promise.all([
      api.get<NodeIP[]>(`/api/v1/node-ips${filter ? `?node_id=${filter}` : ""}`),
      api.get<MediaNode[]>("/api/v1/nodes"),
    ])
      .then(([ips, n]) => {
        setRows(ips);
        setNodes(n);
      })
      .catch((e) => setErr(e.message));
  }
  useEffect(reload, [filter]);

  const mediaNodes = nodes.filter((n) => n.role === "media");

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setErr(null);
    setBusy(true);
    try {
      if (form.mode === "single") {
        await api.post("/api/v1/node-ips", {
          node_id: form.node_id,
          ip_address: form.ip_address,
          lease_block: form.lease_block || undefined,
          purchased_from: form.purchased_from || undefined,
        });
      } else {
        const res = await api.post<{ created: number; skipped: number; total: number }>(
          "/api/v1/node-ips/bulk",
          {
            node_id: form.node_id,
            cidr: form.cidr,
            lease_block: form.lease_block || undefined,
            purchased_from: form.purchased_from || undefined,
          },
        );
        alert(`Bulk add: ${res.created} created, ${res.skipped} skipped (already existed)`);
      }
      setForm({
        node_id: form.node_id,
        mode: form.mode,
        ip_address: "",
        cidr: "",
        lease_block: form.lease_block,
        purchased_from: form.purchased_from,
      });
      reload();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "add failed");
    } finally {
      setBusy(false);
    }
  }
  async function patchStatus(id: number, status: string) {
    try {
      await api.patch<void>(`/api/v1/node-ips/${id}`, { status });
      reload();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "patch failed");
    }
  }
  async function del(id: number) {
    if (!confirm("Delete this IP from the pool?")) return;
    try {
      await api.del<void>(`/api/v1/node-ips/${id}`);
      reload();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "delete failed");
    }
  }
  const nodeName = (id: number) => nodes.find((n) => n.id === id)?.name ?? `#${id}`;

  return (
    <div className="space-y-4">
      <header className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-semibold tracking-tight">IP Pool</h2>
          <p className="text-sm text-slate-500">
            Leased IPs bound to media nodes. Use bulk mode to import a whole /24.
          </p>
        </div>
        <button
          onClick={() => setShowAdd((v) => !v)}
          disabled={mediaNodes.length === 0}
          className="rounded-md bg-brand-600 px-3 py-2 text-sm font-medium text-white shadow-sm hover:bg-brand-700 disabled:opacity-60"
        >
          {showAdd ? "Cancel" : "Add IPs"}
        </button>
      </header>

      {mediaNodes.length === 0 && (
        <div className="rounded border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-800">
          Create at least one node with role <code>media</code> first.
        </div>
      )}

      {err && (
        <div className="rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
          {err}
        </div>
      )}

      {showAdd && (
        <form
          onSubmit={submit}
          className="grid grid-cols-1 gap-3 rounded-lg border border-slate-200 bg-white p-4 shadow-sm sm:grid-cols-2"
        >
          <div>
            <label className="block text-xs font-medium uppercase tracking-wide text-slate-500">
              Media node
            </label>
            <select
              required
              value={form.node_id}
              onChange={(e) => setForm({ ...form, node_id: Number(e.target.value) })}
              className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm"
            >
              <option value={0}>— select —</option>
              {mediaNodes.map((n) => (
                <option key={n.id} value={n.id}>
                  {n.name} ({n.host_ip})
                </option>
              ))}
            </select>
          </div>
          <div>
            <label className="block text-xs font-medium uppercase tracking-wide text-slate-500">
              Mode
            </label>
            <select
              value={form.mode}
              onChange={(e) => setForm({ ...form, mode: e.target.value as "single" | "bulk" })}
              className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm"
            >
              <option value="single">Single IP</option>
              <option value="bulk">Bulk via CIDR (e.g. 192.0.2.0/24)</option>
            </select>
          </div>
          {form.mode === "single" ? (
            <div>
              <label className="block text-xs font-medium uppercase tracking-wide text-slate-500">
                IP
              </label>
              <input
                required
                value={form.ip_address}
                onChange={(e) => setForm({ ...form, ip_address: e.target.value })}
                placeholder="192.0.2.10"
                className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm font-mono"
              />
            </div>
          ) : (
            <div>
              <label className="block text-xs font-medium uppercase tracking-wide text-slate-500">
                CIDR
              </label>
              <input
                required
                value={form.cidr}
                onChange={(e) => setForm({ ...form, cidr: e.target.value })}
                placeholder="192.0.2.0/24"
                className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm font-mono"
              />
            </div>
          )}
          <div>
            <label className="block text-xs font-medium uppercase tracking-wide text-slate-500">
              Lease block (note)
            </label>
            <input
              value={form.lease_block}
              onChange={(e) => setForm({ ...form, lease_block: e.target.value })}
              placeholder="IPXO order #1234"
              className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm"
            />
          </div>
          <div>
            <label className="block text-xs font-medium uppercase tracking-wide text-slate-500">
              Purchased from
            </label>
            <input
              value={form.purchased_from}
              onChange={(e) => setForm({ ...form, purchased_from: e.target.value })}
              placeholder="IPXO / Heficed"
              className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm"
            />
          </div>
          <div className="sm:col-span-2">
            <button
              type="submit"
              disabled={busy || form.node_id === 0}
              className="rounded-md bg-brand-600 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-brand-700 disabled:opacity-60"
            >
              {busy ? "Saving…" : "Add"}
            </button>
          </div>
        </form>
      )}

      <div className="flex items-center gap-2 text-sm">
        <span className="text-slate-500">Filter by node:</span>
        <select
          value={filter}
          onChange={(e) => setFilter(Number(e.target.value))}
          className="rounded border border-slate-300 px-2 py-1"
        >
          <option value={0}>all</option>
          {mediaNodes.map((n) => (
            <option key={n.id} value={n.id}>
              {n.name}
            </option>
          ))}
        </select>
        <span className="ml-auto text-xs text-slate-400">{rows.length} IPs</span>
      </div>

      <div className="overflow-hidden rounded-lg border border-slate-200 bg-white shadow-sm">
        <table className="min-w-full divide-y divide-slate-200 text-sm">
          <thead className="bg-slate-50 text-left text-xs uppercase tracking-wide text-slate-500">
            <tr>
              <th className="px-4 py-2">ID</th>
              <th className="px-4 py-2">IP</th>
              <th className="px-4 py-2">Node</th>
              <th className="px-4 py-2">Status</th>
              <th className="px-4 py-2">Lease</th>
              <th className="px-4 py-2">Calls</th>
              <th className="px-4 py-2" />
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100">
            {rows.length === 0 && (
              <tr>
                <td colSpan={7} className="px-4 py-6 text-center text-slate-400">
                  No IPs in the pool yet.
                </td>
              </tr>
            )}
            {rows.map((r) => (
              <tr key={r.id}>
                <td className="px-4 py-2 font-mono text-slate-500">{r.id}</td>
                <td className="px-4 py-2 font-mono">{r.ip_address}</td>
                <td className="px-4 py-2">{nodeName(r.node_id)}</td>
                <td className="px-4 py-2">
                  <select
                    value={r.status}
                    onChange={(e) => patchStatus(r.id, e.target.value)}
                    className="rounded border border-slate-300 px-2 py-0.5 text-xs"
                  >
                    <option value="active">active</option>
                    <option value="reserve">reserve</option>
                    <option value="flagged">flagged</option>
                    <option value="disabled">disabled</option>
                  </select>
                </td>
                <td className="px-4 py-2 text-xs text-slate-500">{r.lease_block ?? "—"}</td>
                <td className="px-4 py-2 text-slate-600">{r.current_calls}</td>
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
