import { useEffect, useState } from "react";
import {
  api,
  type FirewallPreview,
  type FirewallRule,
  type MediaNode,
} from "../api";
import Modal from "../components/Modal";
import Help from "../components/Help";
import { PencilIcon, PlusIcon, TrashIcon, DownloadIcon, RefreshIcon } from "../components/Icons";

const blankForm = {
  name: "",
  action: "allow" as FirewallRule["action"],
  source_cidr: "",
  proto: "any" as FirewallRule["proto"],
  dest_port_low: "",
  dest_port_high: "",
  node_id: 0,
  rate_per_second: "",
  priority: 100,
  notes: "",
};

function ActionBadge({ action }: { action: FirewallRule["action"] }) {
  const cls =
    action === "allow"
      ? "bg-emerald-100 text-emerald-800"
      : action === "block"
        ? "bg-red-100 text-red-800"
        : "bg-amber-100 text-amber-800";
  return <span className={`rounded px-2 py-0.5 text-xs font-medium ${cls}`}>{action}</span>;
}

export default function Firewall() {
  const [rules, setRules] = useState<FirewallRule[]>([]);
  const [nodes, setNodes] = useState<MediaNode[]>([]);
  const [previewNode, setPreviewNode] = useState<number>(0);
  const [preview, setPreview] = useState<FirewallPreview | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<FirewallRule | null>(null);
  const [form, setForm] = useState(blankForm);

  function reload() {
    Promise.all([
      api.get<FirewallRule[]>("/api/v1/firewall/rules"),
      api.get<MediaNode[]>("/api/v1/nodes"),
    ])
      .then(([r, n]) => {
        setRules(r);
        setNodes(n);
        if (!previewNode && n.length > 0) setPreviewNode(n[0].id);
      })
      .catch((e) => setErr(e.message));
  }
  useEffect(reload, []);

  useEffect(() => {
    if (!previewNode) {
      setPreview(null);
      return;
    }
    api.get<FirewallPreview>(`/api/v1/firewall/preview/${previewNode}`)
      .then(setPreview)
      .catch((e) => setErr(e.message));
  }, [previewNode, rules.length]);

  const nodeName = (id: number | null | undefined) =>
    id ? nodes.find((n) => n.id === id)?.name ?? `#${id}` : "all nodes";

  function openCreate() {
    setForm(blankForm);
    setCreating(true);
  }
  function openEdit(r: FirewallRule) {
    setForm({
      name: r.name,
      action: r.action,
      source_cidr: r.source_cidr ?? "",
      proto: r.proto,
      dest_port_low: r.dest_port_low?.toString() ?? "",
      dest_port_high: r.dest_port_high?.toString() ?? "",
      node_id: r.node_id ?? 0,
      rate_per_second: r.rate_per_second?.toString() ?? "",
      priority: r.priority,
      notes: r.notes ?? "",
    });
    setEditing(r);
  }
  async function save() {
    setBusy(true);
    setErr(null);
    try {
      const body: Record<string, unknown> = {
        name: form.name,
        action: form.action,
        proto: form.proto,
        priority: form.priority,
        notes: form.notes || undefined,
        source_cidr: form.source_cidr || undefined,
      };
      if (form.dest_port_low) body.dest_port_low = Number(form.dest_port_low);
      if (form.dest_port_high) body.dest_port_high = Number(form.dest_port_high);
      if (form.node_id) body.node_id = form.node_id;
      if (form.rate_per_second) body.rate_per_second = Number(form.rate_per_second);

      if (editing) {
        delete body.action; // can't change after creation
        await api.patch(`/api/v1/firewall/rules/${editing.id}`, body);
      } else {
        await api.post("/api/v1/firewall/rules", body);
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
  async function toggle(r: FirewallRule) {
    try {
      await api.patch(`/api/v1/firewall/rules/${r.id}`, { enabled: !r.enabled });
      reload();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "toggle failed");
    }
  }
  async function del(id: number) {
    if (!confirm("Delete this firewall rule?")) return;
    try {
      await api.del<void>(`/api/v1/firewall/rules/${id}`);
      reload();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "delete failed");
    }
  }

  async function applyPreset(p: "rate_limit_sip" | "block_sipvicious") {
    try {
      if (p === "rate_limit_sip") {
        await api.post("/api/v1/firewall/rules", {
          name: "Rate-limit SIP per source",
          action: "rate_limit",
          proto: "udp",
          dest_port_low: 5060,
          dest_port_high: 5060,
          rate_per_second: 50,
          priority: 50,
          notes: "Caps how fast a single source IP can send SIP packets — protects against floods.",
        });
      } else if (p === "block_sipvicious") {
        // The renderer only does L3/L4 — User-Agent filtering is a Kamailio concern.
        // We pre-create a "known scanner" block rule the operator can extend with CIDRs.
        await api.post("/api/v1/firewall/rules", {
          name: "Block known SIP scanner ranges",
          action: "block",
          source_cidr: "192.0.2.0/24", // placeholder; edit after to taste
          priority: 10,
          notes: "Drops packets from a known scanner CIDR. Edit the CIDR to match your threat intel feed.",
        });
      }
      reload();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "preset failed");
    }
  }

  function downloadNFT() {
    if (!preview) return;
    const blob = new Blob([preview.nft_config], { type: "text/plain" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `firewall-${preview.node_name}.nft`;
    a.click();
    URL.revokeObjectURL(url);
  }

  return (
    <div className="space-y-6">
      <header className="flex items-start justify-between">
        <div>
          <h2 className="text-xl font-semibold tracking-tight text-slate-800">Firewall</h2>
          <p className="text-xs text-slate-500">
            Network-layer rules (nftables) per node. The base-app synthesizes a full ruleset
            from these custom rules plus auto-allows for every Carrier host and active Client
            dialer IP. <strong>Preview only</strong> in this iteration — paste/upload the
            generated config to the node manually until auto-apply is wired up.
          </p>
        </div>
        <div className="flex gap-2">
          <button onClick={reload} className="inline-flex items-center gap-1.5 rounded-md border border-slate-300 px-3 py-1.5 text-sm hover:bg-slate-50">
            <RefreshIcon /> Refresh
          </button>
          <button onClick={openCreate} className="inline-flex items-center gap-1.5 rounded-md bg-brand-600 px-3 py-1.5 text-sm font-medium text-white shadow-sm hover:bg-brand-700">
            <PlusIcon /> Add rule
          </button>
        </div>
      </header>

      {err && <div className="rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{err}</div>}

      {/* Presets */}
      <section className="rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
        <h3 className="text-sm font-semibold uppercase tracking-wide text-slate-500">
          One-click presets
          <Help>
            Quick-add common protection rules. You can edit them after — these are just
            templates with sensible defaults.
          </Help>
        </h3>
        <div className="mt-3 flex flex-wrap gap-2">
          <button
            onClick={() => applyPreset("rate_limit_sip")}
            className="rounded border border-slate-300 px-3 py-1.5 text-sm hover:bg-slate-50"
          >
            + Rate-limit SIP per source (50/s)
          </button>
          <button
            onClick={() => applyPreset("block_sipvicious")}
            className="rounded border border-slate-300 px-3 py-1.5 text-sm hover:bg-slate-50"
          >
            + Block scanner CIDR (template)
          </button>
        </div>
      </section>

      {/* Custom rules */}
      <section className="overflow-hidden rounded-lg border border-slate-200 bg-white shadow-sm">
        <table className="min-w-full divide-y divide-slate-200 text-sm">
          <thead className="bg-slate-50 text-left text-xs uppercase tracking-wide text-slate-500">
            <tr>
              <th className="px-4 py-2">Pri</th>
              <th className="px-4 py-2">Name</th>
              <th className="px-4 py-2">Action</th>
              <th className="px-4 py-2">Source</th>
              <th className="px-4 py-2">Proto / Port</th>
              <th className="px-4 py-2">Node</th>
              <th className="px-4 py-2">Enabled</th>
              <th className="px-4 py-2 text-right">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100">
            {rules.length === 0 && (
              <tr><td colSpan={8} className="px-4 py-6 text-center text-slate-400">
                No custom rules yet. Auto-allows from Carriers + Client dialer IPs are still applied — see Preview below.
              </td></tr>
            )}
            {rules.map((r) => (
              <tr key={r.id} className={r.enabled ? "" : "bg-slate-50 text-slate-400"}>
                <td className="px-4 py-2 font-mono text-slate-500">{r.priority}</td>
                <td className="px-4 py-2 font-medium">{r.name}</td>
                <td className="px-4 py-2"><ActionBadge action={r.action} /></td>
                <td className="px-4 py-2 font-mono text-xs">{r.source_cidr ?? "any"}</td>
                <td className="px-4 py-2 font-mono text-xs">
                  {r.proto === "any" ? "any" : r.proto}
                  {r.dest_port_low ? ` ${r.dest_port_low}${r.dest_port_high && r.dest_port_high !== r.dest_port_low ? "-" + r.dest_port_high : ""}` : ""}
                  {r.rate_per_second ? ` @ ${r.rate_per_second}/s` : ""}
                </td>
                <td className="px-4 py-2 text-xs">{nodeName(r.node_id)}</td>
                <td className="px-4 py-2">
                  <button onClick={() => toggle(r)} className={`rounded px-2 py-0.5 text-xs ${r.enabled ? "bg-emerald-100 text-emerald-800" : "bg-slate-200 text-slate-600"}`}>
                    {r.enabled ? "on" : "off"}
                  </button>
                </td>
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
      </section>

      {/* Per-node preview */}
      <section className="rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
        <div className="flex items-center justify-between">
          <h3 className="text-sm font-semibold uppercase tracking-wide text-slate-500">
            Per-node preview
            <Help>
              The synthesized nftables config for the selected node — custom rules + auto-allows
              for every active Carrier host and Client dialer IP. Copy or download and apply
              with <code>sudo nft -f firewall.nft</code> on the remote node.
            </Help>
          </h3>
          <div className="flex items-center gap-2">
            <select value={previewNode} onChange={(e) => setPreviewNode(Number(e.target.value))} className="rounded border border-slate-300 px-2 py-1 text-sm">
              <option value={0}>— select node —</option>
              {nodes.map((n) => <option key={n.id} value={n.id}>{n.name} ({n.role})</option>)}
            </select>
            <button onClick={downloadNFT} disabled={!preview} className="inline-flex items-center gap-1.5 rounded-md border border-slate-300 px-3 py-1.5 text-sm hover:bg-slate-50 disabled:opacity-50">
              <DownloadIcon /> Download .nft
            </button>
          </div>
        </div>

        {preview && (
          <>
            <div className="mt-3 grid grid-cols-2 gap-4">
              <div>
                <h4 className="mb-1 text-xs font-medium uppercase tracking-wide text-slate-500">
                  Auto-allowlist ({preview.auto_rules.length} entries)
                </h4>
                <ul className="max-h-48 overflow-auto rounded border border-slate-200 bg-slate-50 p-2 text-xs">
                  {preview.auto_rules.length === 0 && <li className="text-slate-400">none</li>}
                  {preview.auto_rules.map((a, i) => (
                    <li key={i} className="flex justify-between py-0.5">
                      <span className="font-mono">{a.CIDR}</span>
                      <span className="text-slate-500">{a.Kind === "carrier" ? "carrier" : "client"} · {a.Name}</span>
                    </li>
                  ))}
                </ul>
              </div>
              <div>
                <h4 className="mb-1 text-xs font-medium uppercase tracking-wide text-slate-500">
                  Custom rules applied ({preview.applied_rules.length})
                </h4>
                <ul className="max-h-48 overflow-auto rounded border border-slate-200 bg-slate-50 p-2 text-xs">
                  {preview.applied_rules.length === 0 && <li className="text-slate-400">none</li>}
                  {preview.applied_rules.map((r) => (
                    <li key={r.id} className="flex justify-between py-0.5">
                      <span><strong>{r.action}</strong> {r.name}</span>
                      <span className="font-mono text-slate-500">{r.source_cidr ?? "any"}</span>
                    </li>
                  ))}
                </ul>
              </div>
            </div>

            <h4 className="mt-4 mb-1 text-xs font-medium uppercase tracking-wide text-slate-500">
              Generated nftables config
            </h4>
            <pre className="max-h-96 overflow-auto rounded bg-slate-900 p-3 font-mono text-xs text-slate-100">
              {preview.nft_config}
            </pre>
          </>
        )}
      </section>

      <Modal
        open={creating || editing !== null}
        title={editing ? `Edit rule #${editing.id}` : "Add firewall rule"}
        width="max-w-xl"
        onClose={() => { setCreating(false); setEditing(null); }}
        footer={
          <>
            <button onClick={() => { setCreating(false); setEditing(null); }} className="rounded border border-slate-300 px-3 py-1.5 text-sm hover:bg-slate-100">Cancel</button>
            <button onClick={save} disabled={busy || !form.name} className="rounded bg-brand-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-brand-700 disabled:opacity-60">
              {busy ? "Saving…" : "Save"}
            </button>
          </>
        }
      >
        <div className="grid grid-cols-2 gap-3">
          <div className="col-span-2">
            <label className="block text-xs font-medium text-slate-500">
              Name
              <Help>Description shown on this page and as the comment on the generated nft rule.</Help>
            </label>
            <input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm" />
          </div>
          <div>
            <label className="block text-xs font-medium text-slate-500">
              Action
              <Help>
                <strong>allow</strong> — packets matching the filter pass.
                <br /><strong>block</strong> — packets matching the source CIDR are dropped before any other rule.
                <br /><strong>rate_limit</strong> — accepts but caps to N packets/second from any single source.
              </Help>
            </label>
            <select value={form.action} disabled={!!editing} onChange={(e) => setForm({ ...form, action: e.target.value as FirewallRule["action"] })} className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm disabled:bg-slate-100">
              <option value="allow">allow</option>
              <option value="block">block</option>
              <option value="rate_limit">rate_limit</option>
            </select>
          </div>
          <div>
            <label className="block text-xs font-medium text-slate-500">
              Priority
              <Help>Lower numbers are evaluated first in the chain. Use 10 for high-priority blocks, 100 for normal allows, 500+ for fallbacks.</Help>
            </label>
            <input type="number" value={form.priority} onChange={(e) => setForm({ ...form, priority: Number(e.target.value) })} className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm" />
          </div>
          <div className="col-span-2">
            <label className="block text-xs font-medium text-slate-500">
              Source CIDR
              <Help>
                Source IP or range. <code>1.2.3.4/32</code> for one IP, <code>10.0.0.0/8</code> for a range, blank for any source.
              </Help>
            </label>
            <input value={form.source_cidr} onChange={(e) => setForm({ ...form, source_cidr: e.target.value })} placeholder="1.2.3.4/32 or 10.0.0.0/8" className="mt-1 w-full rounded border border-slate-300 px-3 py-2 font-mono text-sm" />
          </div>
          <div>
            <label className="block text-xs font-medium text-slate-500">
              Proto
              <Help>Filter by L4 protocol. <code>any</code> = both TCP and UDP.</Help>
            </label>
            <select value={form.proto} onChange={(e) => setForm({ ...form, proto: e.target.value as FirewallRule["proto"] })} className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm">
              <option value="any">any</option>
              <option value="tcp">tcp</option>
              <option value="udp">udp</option>
            </select>
          </div>
          <div>
            <label className="block text-xs font-medium text-slate-500">Apply to node</label>
            <select value={form.node_id} onChange={(e) => setForm({ ...form, node_id: Number(e.target.value) })} className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm">
              <option value={0}>all nodes</option>
              {nodes.map((n) => <option key={n.id} value={n.id}>{n.name}</option>)}
            </select>
          </div>
          <div>
            <label className="block text-xs font-medium text-slate-500">
              Port low
              <Help>First port in the range. Leave blank to match any port.</Help>
            </label>
            <input type="number" min={1} max={65535} value={form.dest_port_low} onChange={(e) => setForm({ ...form, dest_port_low: e.target.value })} placeholder="5060" className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm" />
          </div>
          <div>
            <label className="block text-xs font-medium text-slate-500">
              Port high
              <Help>Last port in the range. Leave blank or equal to "Port low" for a single port.</Help>
            </label>
            <input type="number" min={1} max={65535} value={form.dest_port_high} onChange={(e) => setForm({ ...form, dest_port_high: e.target.value })} placeholder="5060" className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm" />
          </div>
          {form.action === "rate_limit" && (
            <div className="col-span-2">
              <label className="block text-xs font-medium text-slate-500">
                Rate (packets / second)
                <Help>Per-source-IP cap. 50/s is reasonable for SIP, 1000/s for higher-throughput flows.</Help>
              </label>
              <input type="number" min={1} value={form.rate_per_second} onChange={(e) => setForm({ ...form, rate_per_second: e.target.value })} className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm" />
            </div>
          )}
          <div className="col-span-2">
            <label className="block text-xs font-medium text-slate-500">Notes</label>
            <textarea value={form.notes} onChange={(e) => setForm({ ...form, notes: e.target.value })} rows={2} className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm" />
          </div>
        </div>
      </Modal>
    </div>
  );
}
