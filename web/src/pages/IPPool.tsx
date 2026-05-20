import { useCallback, useEffect, useState } from "react";
import { api, type MediaNode, type NodeIP } from "../api";
import Modal from "../components/Modal";
import { PencilIcon, TrashIcon, RefreshIcon } from "../components/Icons";

export default function IPPool() {
  const [rows, setRows] = useState<NodeIP[]>([]);
  const [nodes, setNodes] = useState<MediaNode[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [filter, setFilter] = useState(0);
  const [editing, setEditing] = useState<NodeIP | null>(null);
  const [form, setForm] = useState({
    status: "active" as NodeIP["status"],
    purchased_from: "",
    lease_block: "",
    monthly_cost: "",
    rdns: "",
    max_calls: 0,
  });
  const [busy, setBusy] = useState(false);

  const reload = useCallback(() => {
    Promise.all([
      api.get<NodeIP[]>(`/api/v1/node-ips${filter ? `?node_id=${filter}` : ""}`),
      api.get<MediaNode[]>("/api/v1/nodes"),
    ])
      .then(([ips, n]) => {
        setRows(ips);
        setNodes(n);
      })
      .catch((e) => setErr(e.message));
  }, [filter]);
  useEffect(() => {
    reload();
    const t = setInterval(reload, 15000);
    return () => clearInterval(t);
  }, [reload]);

  const mediaNodes = nodes.filter((n) => n.role === "media");
  const nodeName = (id: number) => nodes.find((n) => n.id === id)?.name ?? `#${id}`;

  function openEdit(ip: NodeIP) {
    setForm({
      status: ip.status,
      purchased_from: ip.purchased_from ?? "",
      lease_block: ip.lease_block ?? "",
      monthly_cost: ip.monthly_cost != null ? String(ip.monthly_cost) : "",
      rdns: ip.rdns ?? "",
      max_calls: ip.max_calls ?? 0,
    });
    setEditing(ip);
  }
  async function save() {
    if (!editing) return;
    setErr(null);
    setBusy(true);
    try {
      const body: Record<string, unknown> = { status: form.status, max_calls: form.max_calls };
      if (form.purchased_from) body.purchased_from = form.purchased_from;
      if (form.lease_block) body.lease_block = form.lease_block;
      if (form.monthly_cost) body.monthly_cost = Number(form.monthly_cost);
      if (form.rdns) body.rdns = form.rdns;
      await api.patch<void>(`/api/v1/node-ips/${editing.id}`, body);
      setEditing(null);
      reload();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "save failed");
    } finally {
      setBusy(false);
    }
  }
  async function del(id: number) {
    if (!confirm("Delete this IP from the pool? The agent will re-add it on next heartbeat if it's still bound on the NIC.")) return;
    try {
      await api.del<void>(`/api/v1/node-ips/${id}`);
      reload();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "delete failed");
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

  return (
    <div className="space-y-4">
      <header className="flex items-start justify-between">
        <div>
          <h2 className="text-xl font-semibold tracking-tight text-slate-800">IP Pool</h2>
          <p className="text-xs text-slate-500">
            IPs auto-discovered from media-node agent heartbeats. Bind IPs at the OS / cloud-provider
            level on the node; they appear here automatically. Edit metadata (status, lease info, rDNS)
            from this page.
          </p>
        </div>
        <button onClick={reload} className="inline-flex items-center gap-1.5 rounded-md border border-slate-300 px-3 py-1.5 text-sm hover:bg-slate-50">
          <RefreshIcon /> Refresh
        </button>
      </header>

      {err && <div className="rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{err}</div>}

      <div className="flex items-center gap-2 text-sm">
        <span className="text-slate-500">Filter by node:</span>
        <select value={filter} onChange={(e) => setFilter(Number(e.target.value))} className="rounded border border-slate-300 px-2 py-1">
          <option value={0}>all</option>
          {mediaNodes.map((n) => <option key={n.id} value={n.id}>{n.name}</option>)}
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
              <th className="px-4 py-2">Source</th>
              <th className="px-4 py-2">Calls / cap</th>
              <th className="px-4 py-2 text-right">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100">
            {rows.length === 0 && (
              <tr>
                <td colSpan={8} className="px-4 py-6 text-center text-slate-400">
                  No IPs in the pool. Bind IPs on a media node and run the agent — they'll appear here.
                </td>
              </tr>
            )}
            {rows.map((r) => (
              <tr key={r.id} className="hover:bg-slate-50">
                <td className="px-4 py-2 font-mono text-slate-500">{r.id}</td>
                <td className="px-4 py-2 font-mono">{r.ip_address}</td>
                <td className="px-4 py-2">{nodeName(r.node_id)}</td>
                <td className="px-4 py-2">
                  <select value={r.status} onChange={(e) => patchStatus(r.id, e.target.value)} className="rounded border border-slate-300 px-2 py-0.5 text-xs">
                    <option value="active">active</option>
                    <option value="reserve">reserve</option>
                    <option value="flagged">flagged</option>
                    <option value="disabled">disabled</option>
                  </select>
                </td>
                <td className="px-4 py-2 text-xs text-slate-500">{r.lease_block ?? "—"}</td>
                <td className="px-4 py-2 text-xs">
                  {r.auto_discovered ? (
                    <span className="rounded bg-emerald-100 px-1.5 py-0.5 text-emerald-800">auto</span>
                  ) : (
                    <span className="rounded bg-slate-200 px-1.5 py-0.5 text-slate-700">manual</span>
                  )}
                </td>
                <td className="px-4 py-2 text-slate-600">
                  {r.current_calls}{" "}
                  <span className="text-xs text-slate-400">
                    / {r.max_calls ? r.max_calls : "∞"}
                  </span>
                </td>
                <td className="px-4 py-2">
                  <div className="flex justify-end gap-1">
                    <button onClick={() => openEdit(r)} title="Edit metadata" className="rounded p-1 text-slate-500 hover:bg-slate-100 hover:text-brand-600">
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
        open={editing !== null}
        title={editing ? `Edit IP ${editing.ip_address}` : ""}
        onClose={() => setEditing(null)}
        footer={
          <>
            <button onClick={() => setEditing(null)} className="rounded border border-slate-300 px-3 py-1.5 text-sm hover:bg-slate-100">Cancel</button>
            <button onClick={save} disabled={busy} className="rounded bg-brand-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-brand-700 disabled:opacity-60">
              {busy ? "Saving…" : "Save"}
            </button>
          </>
        }
      >
        <div className="space-y-3">
          <div>
            <label className="block text-xs font-medium text-slate-500">Status</label>
            <select value={form.status} onChange={(e) => setForm({ ...form, status: e.target.value as NodeIP["status"] })} className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm">
              <option value="active">active</option>
              <option value="reserve">reserve</option>
              <option value="flagged">flagged</option>
              <option value="disabled">disabled</option>
            </select>
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-xs font-medium text-slate-500">Purchased from</label>
              <input value={form.purchased_from} onChange={(e) => setForm({ ...form, purchased_from: e.target.value })} placeholder="IPXO" className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm" />
            </div>
            <div>
              <label className="block text-xs font-medium text-slate-500">Lease block</label>
              <input value={form.lease_block} onChange={(e) => setForm({ ...form, lease_block: e.target.value })} placeholder="192.0.2.0/24" className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm font-mono" />
            </div>
            <div>
              <label className="block text-xs font-medium text-slate-500">Monthly cost ($)</label>
              <input type="number" step="0.01" value={form.monthly_cost} onChange={(e) => setForm({ ...form, monthly_cost: e.target.value })} className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm" />
            </div>
            <div>
              <label className="block text-xs font-medium text-slate-500">rDNS</label>
              <input value={form.rdns} onChange={(e) => setForm({ ...form, rdns: e.target.value })} placeholder="ip1.example.com" className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm" />
            </div>
            <div className="col-span-2">
              <label className="block text-xs font-medium text-slate-500">
                Max concurrent calls (0 = unlimited, use node max)
              </label>
              <input
                type="number"
                min={0}
                value={form.max_calls}
                onChange={(e) => setForm({ ...form, max_calls: Number(e.target.value) })}
                className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm"
              />
              <p className="mt-1 text-xs text-slate-400">
                Rotation strategies will skip this IP once <code>current_calls</code> reaches the cap.
              </p>
            </div>
          </div>
        </div>
      </Modal>
    </div>
  );
}
