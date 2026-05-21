import { useCallback, useEffect, useState } from "react";
import { api, type MediaNode, type NodeIP } from "../api";
import Modal from "../components/Modal";
import Help from "../components/Help";
import { PencilIcon, TrashIcon, RefreshIcon, PlusIcon } from "../components/Icons";

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

  // Bulk-add modal state
  const [bulkOpen, setBulkOpen] = useState(false);
  const [bulkForm, setBulkForm] = useState({
    node_id: 0,
    mode: "cidr" as "cidr" | "list",
    cidr: "",
    ips: "",
    purchased_from: "",
    lease_block: "",
  });
  const [bulkResult, setBulkResult] = useState<string | null>(null);

  // Bulk-apply modal state (sets max_calls / status across many IPs at once)
  const [applyOpen, setApplyOpen] = useState(false);
  const [applyForm, setApplyForm] = useState({
    node_id: 0,
    status_filter: "active" as "active" | "reserve" | "flagged" | "disabled" | "all",
    set_max_calls: true,
    max_calls: 30,
    set_status: false,
    new_status: "active" as NodeIP["status"],
  });
  const [applyResult, setApplyResult] = useState<string | null>(null);

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

  function openApply() {
    setApplyForm({
      node_id: filter || mediaNodes[0]?.id || 0,
      status_filter: "active",
      set_max_calls: true,
      max_calls: 30,
      set_status: false,
      new_status: "active",
    });
    setApplyResult(null);
    setApplyOpen(true);
  }
  async function submitApply() {
    if (!applyForm.node_id) {
      setErr("Pick a media node first.");
      return;
    }
    if (!applyForm.set_max_calls && !applyForm.set_status) {
      setErr("Tick at least one of 'set max_calls' or 'set status'.");
      return;
    }
    setErr(null);
    setBusy(true);
    setApplyResult(null);
    try {
      const body: Record<string, unknown> = { node_id: applyForm.node_id };
      if (applyForm.status_filter !== "all") body.status_filter = applyForm.status_filter;
      if (applyForm.set_max_calls) body.max_calls = applyForm.max_calls;
      if (applyForm.set_status) body.new_status = applyForm.new_status;
      const res = await api.post<{ updated: number }>("/api/v1/node-ips/bulk-update", body);
      setApplyResult(`Updated ${res.updated} IP${res.updated === 1 ? "" : "s"}.`);
      reload();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "bulk apply failed");
    } finally {
      setBusy(false);
    }
  }

  function openBulk() {
    setBulkForm({
      node_id: mediaNodes[0]?.id ?? 0,
      mode: "cidr",
      cidr: "",
      ips: "",
      purchased_from: "",
      lease_block: "",
    });
    setBulkResult(null);
    setBulkOpen(true);
  }
  async function submitBulk() {
    if (!bulkForm.node_id) {
      setErr("Pick a media node first.");
      return;
    }
    setErr(null);
    setBusy(true);
    setBulkResult(null);
    try {
      const body: Record<string, unknown> = { node_id: bulkForm.node_id };
      if (bulkForm.mode === "cidr") {
        if (!bulkForm.cidr.trim()) {
          throw new Error("CIDR is required (e.g. 67.215.233.64/26)");
        }
        body.cidr = bulkForm.cidr.trim();
      } else {
        // Split on whitespace, comma, semicolon — accept anything reasonable.
        const list = bulkForm.ips
          .split(/[\s,;]+/)
          .map((s) => s.trim())
          .filter(Boolean);
        if (list.length === 0) {
          throw new Error("Paste at least one IP address.");
        }
        body.ips = list;
      }
      if (bulkForm.purchased_from) body.purchased_from = bulkForm.purchased_from;
      if (bulkForm.lease_block) body.lease_block = bulkForm.lease_block;
      const res = await api.post<{ created: number; skipped: number; total: number }>(
        "/api/v1/node-ips/bulk",
        body,
      );
      setBulkResult(
        `Imported ${res.created} new IP${res.created === 1 ? "" : "s"}` +
          (res.skipped > 0 ? `, skipped ${res.skipped} duplicate${res.skipped === 1 ? "" : "s"}` : "") +
          ". Agent will bind them on its next heartbeat (~10s).",
      );
      reload();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "bulk add failed");
    } finally {
      setBusy(false);
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
        <div className="flex gap-2">
          <button onClick={reload} className="inline-flex items-center gap-1.5 rounded-md border border-slate-300 px-3 py-1.5 text-sm hover:bg-slate-50">
            <RefreshIcon /> Refresh
          </button>
          <button
            onClick={openApply}
            disabled={mediaNodes.length === 0 || rows.length === 0}
            className="inline-flex items-center gap-1.5 rounded-md border border-brand-300 px-3 py-1.5 text-sm font-medium text-brand-700 hover:bg-brand-50 disabled:opacity-50"
            title="Apply max_calls or status to many IPs at once"
          >
            Bulk apply
          </button>
          <button
            onClick={openBulk}
            disabled={mediaNodes.length === 0}
            className="inline-flex items-center gap-1.5 rounded-md bg-brand-600 px-3 py-1.5 text-sm font-medium text-white shadow-sm hover:bg-brand-700 disabled:opacity-50"
            title={mediaNodes.length === 0 ? "Add a media node first" : "Bulk-add IPs to a media node"}
          >
            <PlusIcon /> Bulk add IPs
          </button>
        </div>
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
            <label className="block text-xs font-medium text-slate-500">
              Status
              <Help>
                <strong>active</strong> — eligible for rotation.
                <br /><strong>reserve</strong> — held back, brought in if active pool gets thin.
                <br /><strong>flagged</strong> — bad reputation (Spamhaus / low ASR); skipped automatically.
                <br /><strong>disabled</strong> — admin-locked; never used.
              </Help>
            </label>
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
                <Help>
                  Hard cap on how many calls this individual IP can carry at once.
                  When <code>current_calls</code> reaches the cap, rotation strategies
                  skip this IP and try another from the same group. Useful for paid IPs
                  with strict per-IP limits or for spreading load evenly. <code>0</code>
                  means "no per-IP limit; let the node cap kick in instead".
                </Help>
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

      <Modal
        open={bulkOpen}
        title="Bulk add IPs to a media node"
        onClose={() => setBulkOpen(false)}
        width="max-w-2xl"
        footer={
          <>
            <button onClick={() => setBulkOpen(false)} className="rounded border border-slate-300 px-3 py-1.5 text-sm hover:bg-slate-100">
              {bulkResult ? "Close" : "Cancel"}
            </button>
            {!bulkResult && (
              <button onClick={submitBulk} disabled={busy} className="rounded bg-brand-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-brand-700 disabled:opacity-60">
                {busy ? "Importing…" : "Import"}
              </button>
            )}
          </>
        }
      >
        <div className="space-y-4">
          <p className="text-xs text-slate-500">
            Register IPs your cloud provider has allocated to a media node. The agent
            on that node will call <code className="font-mono">ip addr add</code> for
            every new IP on its primary interface and write a managed netplan file so
            they persist across reboots. No SSH required — happens on the next heartbeat
            (~10 seconds).
          </p>

          <div>
            <label className="block text-xs font-medium text-slate-500">
              Target media node
              <Help>
                IPs are registered to a single node. The agent on that node binds them.
                Only nodes with <code>role=media</code> appear here.
              </Help>
            </label>
            <select
              value={bulkForm.node_id}
              onChange={(e) => setBulkForm({ ...bulkForm, node_id: Number(e.target.value) })}
              className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm"
            >
              {mediaNodes.length === 0 && <option value={0}>no media nodes</option>}
              {mediaNodes.map((n) => (
                <option key={n.id} value={n.id}>
                  {n.name} ({n.host_ip})
                </option>
              ))}
            </select>
          </div>

          <div>
            <label className="block text-xs font-medium text-slate-500">
              Input mode
              <Help>
                <strong>CIDR</strong> — expand a whole block (e.g.
                <code className="font-mono"> 67.215.233.64/26</code> = 62 usable IPs).
                Use this when your provider gave you a contiguous block.
                <br /><br />
                <strong>List</strong> — paste a list of individual IPs
                (one per line, or comma/space separated). Use this for scattered IPs.
              </Help>
            </label>
            <div className="mt-1 flex gap-2">
              <button
                type="button"
                onClick={() => setBulkForm({ ...bulkForm, mode: "cidr" })}
                className={`flex-1 rounded border px-3 py-1.5 text-sm ${bulkForm.mode === "cidr" ? "border-brand-500 bg-brand-50 text-brand-700" : "border-slate-300 text-slate-600 hover:bg-slate-50"}`}
              >
                CIDR block
              </button>
              <button
                type="button"
                onClick={() => setBulkForm({ ...bulkForm, mode: "list" })}
                className={`flex-1 rounded border px-3 py-1.5 text-sm ${bulkForm.mode === "list" ? "border-brand-500 bg-brand-50 text-brand-700" : "border-slate-300 text-slate-600 hover:bg-slate-50"}`}
              >
                Paste IP list
              </button>
            </div>
          </div>

          {bulkForm.mode === "cidr" ? (
            <div>
              <label className="block text-xs font-medium text-slate-500">
                CIDR block
                <Help>
                  Standard notation. The handler skips network + broadcast addresses
                  automatically, so a <code>/26</code> gives you 62 usable hosts.
                  Examples: <code>67.215.233.64/26</code>, <code>192.0.2.0/28</code>.
                </Help>
              </label>
              <input
                value={bulkForm.cidr}
                onChange={(e) => setBulkForm({ ...bulkForm, cidr: e.target.value })}
                placeholder="67.215.233.64/26"
                className="mt-1 w-full rounded border border-slate-300 px-3 py-2 font-mono text-sm"
              />
            </div>
          ) : (
            <div>
              <label className="block text-xs font-medium text-slate-500">
                IP addresses
              </label>
              <textarea
                rows={8}
                value={bulkForm.ips}
                onChange={(e) => setBulkForm({ ...bulkForm, ips: e.target.value })}
                placeholder={"67.215.233.65\n67.215.233.66\n67.215.233.67\n..."}
                className="mt-1 w-full rounded border border-slate-300 px-3 py-2 font-mono text-xs"
              />
              <p className="mt-1 text-xs text-slate-400">
                One per line, or comma/space separated. Whitespace ignored.
              </p>
            </div>
          )}

          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-xs font-medium text-slate-500">
                Purchased from (optional)
                <Help>Free-text source tag. E.g. "Vultr", "IPXO", "AWS BYO".</Help>
              </label>
              <input
                value={bulkForm.purchased_from}
                onChange={(e) => setBulkForm({ ...bulkForm, purchased_from: e.target.value })}
                placeholder="Vultr"
                className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm"
              />
            </div>
            <div>
              <label className="block text-xs font-medium text-slate-500">
                Lease block (optional)
              </label>
              <input
                value={bulkForm.lease_block}
                onChange={(e) => setBulkForm({ ...bulkForm, lease_block: e.target.value })}
                placeholder="67.215.233.64/26"
                className="mt-1 w-full rounded border border-slate-300 px-3 py-2 font-mono text-sm"
              />
            </div>
          </div>

          {bulkResult && (
            <div className="rounded border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-800">
              {bulkResult}
            </div>
          )}
        </div>
      </Modal>

      <Modal
        open={applyOpen}
        title="Bulk apply settings to many IPs"
        onClose={() => setApplyOpen(false)}
        width="max-w-2xl"
        footer={
          <>
            <button onClick={() => setApplyOpen(false)} className="rounded border border-slate-300 px-3 py-1.5 text-sm hover:bg-slate-100">
              {applyResult ? "Close" : "Cancel"}
            </button>
            {!applyResult && (
              <button onClick={submitApply} disabled={busy} className="rounded bg-brand-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-brand-700 disabled:opacity-60">
                {busy ? "Applying…" : "Apply to selection"}
              </button>
            )}
          </>
        }
      >
        <div className="space-y-4">
          <p className="text-xs text-slate-500">
            Update <code>max_calls</code> and/or <code>status</code> across many IPs at once.
            Saves a lot of clicking when a freshly-imported block needs a per-IP cap.
          </p>

          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-xs font-medium text-slate-500">
                Target media node
              </label>
              <select
                value={applyForm.node_id}
                onChange={(e) => setApplyForm({ ...applyForm, node_id: Number(e.target.value) })}
                className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm"
              >
                {mediaNodes.length === 0 && <option value={0}>no media nodes</option>}
                {mediaNodes.map((n) => (
                  <option key={n.id} value={n.id}>
                    {n.name} ({n.host_ip})
                  </option>
                ))}
              </select>
            </div>
            <div>
              <label className="block text-xs font-medium text-slate-500">
                Apply only to IPs in status
                <Help>
                  Restrict the update to IPs in a specific state. "All states" hits every IP
                  on that node regardless. Pick <strong>active</strong> for the most common
                  case (set the cap on your usable pool).
                </Help>
              </label>
              <select
                value={applyForm.status_filter}
                onChange={(e) => setApplyForm({ ...applyForm, status_filter: e.target.value as typeof applyForm.status_filter })}
                className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm"
              >
                <option value="all">all states</option>
                <option value="active">active</option>
                <option value="reserve">reserve</option>
                <option value="flagged">flagged</option>
                <option value="disabled">disabled</option>
              </select>
            </div>
          </div>

          <div className="space-y-2 rounded border border-slate-200 bg-slate-50 p-3">
            <label className="flex items-start gap-2 text-sm">
              <input
                type="checkbox"
                checked={applyForm.set_max_calls}
                onChange={(e) => setApplyForm({ ...applyForm, set_max_calls: e.target.checked })}
                className="mt-1"
              />
              <div className="flex-1">
                <div className="font-medium text-slate-700">
                  Set max calls per IP
                  <Help>
                    Hard cap on simultaneous calls per IP. Common picks:
                    <strong> 30</strong> for G.711 media (most setups),
                    <strong> 20</strong> for transcoded G.729,
                    <strong> 0</strong> for "no per-IP cap" (the node's <code>max_calls</code>
                    is then the only ceiling).
                  </Help>
                </div>
                <input
                  type="number"
                  min={0}
                  value={applyForm.max_calls}
                  onChange={(e) => setApplyForm({ ...applyForm, max_calls: Number(e.target.value) })}
                  disabled={!applyForm.set_max_calls}
                  className="mt-1 w-full rounded border border-slate-300 px-3 py-1.5 text-sm disabled:bg-slate-100"
                />
              </div>
            </label>

            <label className="flex items-start gap-2 text-sm">
              <input
                type="checkbox"
                checked={applyForm.set_status}
                onChange={(e) => setApplyForm({ ...applyForm, set_status: e.target.checked })}
                className="mt-1"
              />
              <div className="flex-1">
                <div className="font-medium text-slate-700">Change status</div>
                <select
                  value={applyForm.new_status}
                  onChange={(e) => setApplyForm({ ...applyForm, new_status: e.target.value as NodeIP["status"] })}
                  disabled={!applyForm.set_status}
                  className="mt-1 w-full rounded border border-slate-300 px-3 py-1.5 text-sm disabled:bg-slate-100"
                >
                  <option value="active">active</option>
                  <option value="reserve">reserve</option>
                  <option value="flagged">flagged</option>
                  <option value="disabled">disabled</option>
                </select>
              </div>
            </label>
          </div>

          {applyResult && (
            <div className="rounded border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-800">
              {applyResult}
            </div>
          )}
        </div>
      </Modal>
    </div>
  );
}
