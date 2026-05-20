import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { api, type MediaNode, type MetricPoint } from "../api";
import Bar from "../components/Bar";
import Spark from "../components/Spark";

function pct(num: number | null | undefined, max: number) {
  if (!num || !max) return 0;
  return Math.max(0, Math.min(100, (num / max) * 100));
}
function ago(ts: string | null | undefined): string {
  if (!ts) return "never";
  const s = Math.max(0, Math.floor((Date.now() - new Date(ts).getTime()) / 1000));
  if (s < 60) return `${s}s ago`;
  if (s < 3600) return `${Math.floor(s / 60)}m ago`;
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`;
  return `${Math.floor(s / 86400)}d ago`;
}
function fmtUptime(secs: number | null | undefined): string {
  if (!secs) return "—";
  const d = Math.floor(secs / 86400);
  const h = Math.floor((secs % 86400) / 3600);
  const m = Math.floor((secs % 3600) / 60);
  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}

function StatusBadge({ status }: { status: string }) {
  const cls =
    status === "online"
      ? "bg-emerald-100 text-emerald-800"
      : status === "draining"
        ? "bg-amber-100 text-amber-800"
        : "bg-slate-200 text-slate-700";
  return <span className={`rounded px-2 py-0.5 text-xs font-medium ${cls}`}>{status}</span>;
}

function NodeCard({
  n,
  history,
  onAction,
  onError,
}: {
  n: MediaNode;
  history: MetricPoint[] | undefined;
  onAction: () => void;
  onError: (e: string) => void;
}) {
  const calls = n.active_calls ?? 0;
  const cpu = Number(n.cpu_pct ?? 0);
  const ram = Number(n.ram_pct ?? 0);
  const netIn = Number(n.net_in_mbps ?? 0);
  const netOut = Number(n.net_out_mbps ?? 0);
  const nicMbps = (n.role === "media" ? 10000 : 1000);
  const netPct = pct(netIn + netOut, nicMbps);

  async function drain() {
    if (!confirm(`Drain node ${n.name}? It will stop accepting new calls.`)) return;
    try {
      await api.post<void>(`/api/v1/nodes/${n.id}/drain`);
      onAction();
    } catch (e) {
      onError(e instanceof Error ? e.message : "drain failed");
    }
  }
  async function undrain() {
    try {
      await api.post<void>(`/api/v1/nodes/${n.id}/undrain`);
      onAction();
    } catch (e) {
      onError(e instanceof Error ? e.message : "undrain failed");
    }
  }
  async function del() {
    if (!confirm(`Delete node ${n.name}? IPs must be removed first.`)) return;
    try {
      await api.del<void>(`/api/v1/nodes/${n.id}`);
      onAction();
    } catch (e) {
      onError(e instanceof Error ? e.message : "delete failed");
    }
  }

  return (
    <div className="rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
      <header className="flex items-start justify-between">
        <div>
          <div className="flex items-center gap-2">
            <h3 className="text-sm font-semibold tracking-tight">{n.name}</h3>
            <StatusBadge status={n.status} />
            <span className="text-xs text-slate-400">#{n.id}</span>
          </div>
          <p className="mt-0.5 text-xs text-slate-500">
            <span className="font-mono">{n.host_ip}</span>
            {n.region ? <> · {n.region}</> : null}
            {" · "}role <code className="text-slate-600">{n.role}</code>
          </p>
        </div>
        <div className="text-right text-xs text-slate-500">
          <div>uptime {fmtUptime(n.uptime_seconds)}</div>
          <div>seen {ago(n.last_seen_at)}</div>
          {n.agent_version && (
            <div className="text-slate-400">agent v{n.agent_version}</div>
          )}
        </div>
      </header>

      <div className="mt-4 space-y-2">
        <Bar
          label="Active calls"
          pct={pct(calls, n.max_calls || 1)}
          value={`${calls} / ${n.max_calls || "—"}`}
          tone="info"
        />
        <Bar label="CPU" pct={cpu} value={`${cpu.toFixed(1)}%`} />
        <Bar label="RAM" pct={ram} value={`${ram.toFixed(1)}%`} />
        <Bar
          label="Network in+out"
          pct={netPct}
          value={`${netIn.toFixed(1)} ↓ / ${netOut.toFixed(1)} ↑ Mbps`}
        />
        <Bar
          label="IPs bound"
          pct={pct(n.ips_bound, n.ips_total || 1)}
          value={`${n.ips_bound} / ${n.ips_total}`}
          tone="info"
        />
      </div>

      <div className="mt-3 flex items-center justify-between border-t border-slate-100 pt-3 text-xs text-slate-500">
        <div className="flex items-center gap-2">
          <span className="text-slate-400">last hour CPU:</span>
          <span className="text-brand-600">
            <Spark data={(history ?? []).map((p) => Number(p.cpu_pct ?? 0))} width={100} height={20} />
          </span>
        </div>
        <div className="space-x-3">
          {n.status === "draining" ? (
            <button onClick={undrain} className="text-emerald-600 hover:underline">
              Undrain
            </button>
          ) : (
            <button onClick={drain} className="text-amber-700 hover:underline">
              Drain
            </button>
          )}
          <button onClick={del} className="text-red-600 hover:underline">
            Delete
          </button>
        </div>
      </div>
    </div>
  );
}

export default function Nodes() {
  const [nodes, setNodes] = useState<MediaNode[]>([]);
  const [historyByNode, setHistoryByNode] = useState<Record<number, MetricPoint[]>>({});
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
  const reloadingHistory = useRef(false);

  const reload = useCallback(async () => {
    try {
      const list = await api.get<MediaNode[]>("/api/v1/nodes");
      setNodes(list);
      if (!reloadingHistory.current) {
        reloadingHistory.current = true;
        const hist: Record<number, MetricPoint[]> = {};
        await Promise.all(
          list.map(async (n) => {
            try {
              hist[n.id] = await api.get<MetricPoint[]>(`/api/v1/nodes/${n.id}/metrics?minutes=60`);
            } catch {
              hist[n.id] = [];
            }
          }),
        );
        setHistoryByNode(hist);
        reloadingHistory.current = false;
      }
    } catch (e) {
      setErr(e instanceof Error ? e.message : "load failed");
    }
  }, []);

  useEffect(() => {
    reload();
    const t = setInterval(reload, 5000);
    return () => clearInterval(t);
  }, [reload]);

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

  const cluster = useMemo(() => {
    return nodes.reduce(
      (acc, n) => {
        acc.calls += n.active_calls ?? 0;
        acc.max += n.max_calls;
        if (n.status === "online") acc.online += 1;
        if (n.status === "draining") acc.draining += 1;
        if (n.status === "offline") acc.offline += 1;
        acc.netIn += Number(n.net_in_mbps ?? 0);
        acc.netOut += Number(n.net_out_mbps ?? 0);
        acc.ipsBound += n.ips_bound;
        acc.ipsTotal += n.ips_total;
        return acc;
      },
      { calls: 0, max: 0, online: 0, draining: 0, offline: 0, netIn: 0, netOut: 0, ipsBound: 0, ipsTotal: 0 },
    );
  }, [nodes]);

  return (
    <div className="space-y-4">
      <header className="flex items-start justify-between">
        <div>
          <h2 className="text-2xl font-semibold tracking-tight">Nodes</h2>
          <p className="text-sm text-slate-500">
            Media + SIP proxy hosts. Auto-refreshes every 5s. Nodes flip to <code>offline</code> if
            the agent doesn't heartbeat for 2 minutes.
          </p>
        </div>
        <button
          onClick={() => setShowForm((v) => !v)}
          className="rounded-md bg-brand-600 px-3 py-2 text-sm font-medium text-white shadow-sm hover:bg-brand-700"
        >
          {showForm ? "Cancel" : "Add node"}
        </button>
      </header>

      <div className="grid grid-cols-2 gap-3 rounded-lg border border-slate-200 bg-white p-4 shadow-sm sm:grid-cols-5">
        <div>
          <div className="text-xs uppercase tracking-wide text-slate-500">Cluster calls</div>
          <div className="text-2xl font-semibold">{cluster.calls.toLocaleString()}</div>
          <div className="text-xs text-slate-500">of {cluster.max.toLocaleString()} capacity</div>
        </div>
        <div>
          <div className="text-xs uppercase tracking-wide text-slate-500">Online</div>
          <div className="text-2xl font-semibold text-emerald-600">{cluster.online}</div>
          <div className="text-xs text-slate-500">{cluster.draining} drain · {cluster.offline} off</div>
        </div>
        <div>
          <div className="text-xs uppercase tracking-wide text-slate-500">Bandwidth</div>
          <div className="text-2xl font-semibold">
            {(cluster.netIn + cluster.netOut).toFixed(1)}
            <span className="ml-1 text-sm text-slate-500">Mbps</span>
          </div>
          <div className="text-xs text-slate-500">
            {cluster.netIn.toFixed(1)} ↓ / {cluster.netOut.toFixed(1)} ↑
          </div>
        </div>
        <div>
          <div className="text-xs uppercase tracking-wide text-slate-500">IPs bound</div>
          <div className="text-2xl font-semibold">{cluster.ipsBound}</div>
          <div className="text-xs text-slate-500">of {cluster.ipsTotal} expected</div>
        </div>
        <div>
          <div className="text-xs uppercase tracking-wide text-slate-500">Nodes</div>
          <div className="text-2xl font-semibold">{nodes.length}</div>
          <div className="text-xs text-slate-500">total</div>
        </div>
      </div>

      {err && (
        <div className="rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{err}</div>
      )}

      {created && (
        <div className="rounded border border-amber-300 bg-amber-50 p-4 text-sm">
          <div className="font-medium text-amber-900">
            Node "{created.name}" created. Save its agent token now — it won't be shown again:
          </div>
          <code className="mt-2 block break-all rounded bg-white p-2 font-mono text-xs">
            {created.agent_token}
          </code>
          <button onClick={() => setCreated(null)} className="mt-2 text-xs text-amber-900 underline">
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
            <label className="block text-xs font-medium uppercase tracking-wide text-slate-500">Name</label>
            <input required value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm" placeholder="media-node-1" />
          </div>
          <div>
            <label className="block text-xs font-medium uppercase tracking-wide text-slate-500">Role</label>
            <select value={form.role} onChange={(e) => setForm({ ...form, role: e.target.value as "media" | "sip_proxy" })} className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm">
              <option value="media">media (RTPEngine)</option>
              <option value="sip_proxy">sip_proxy (Kamailio)</option>
            </select>
          </div>
          <div>
            <label className="block text-xs font-medium uppercase tracking-wide text-slate-500">Host IP</label>
            <input required value={form.host_ip} onChange={(e) => setForm({ ...form, host_ip: e.target.value })} className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm" placeholder="45.77.156.60" />
          </div>
          <div>
            <label className="block text-xs font-medium uppercase tracking-wide text-slate-500">Region</label>
            <input value={form.region} onChange={(e) => setForm({ ...form, region: e.target.value })} className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm" placeholder="us-east" />
          </div>
          <div>
            <label className="block text-xs font-medium uppercase tracking-wide text-slate-500">Max calls</label>
            <input type="number" min={0} value={form.max_calls} onChange={(e) => setForm({ ...form, max_calls: Number(e.target.value) })} className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm" />
          </div>
          <div className="flex items-end sm:col-span-2">
            <button type="submit" disabled={busy} className="rounded-md bg-brand-600 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-brand-700 disabled:opacity-60">
              {busy ? "Saving…" : "Create node"}
            </button>
          </div>
        </form>
      )}

      {nodes.length === 0 ? (
        <div className="rounded-lg border border-dashed border-slate-300 bg-white p-8 text-center text-sm text-slate-500">
          No nodes yet.
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-3 lg:grid-cols-2 xl:grid-cols-3">
          {nodes.map((n) => (
            <NodeCard
              key={n.id}
              n={n}
              history={historyByNode[n.id]}
              onAction={reload}
              onError={setErr}
            />
          ))}
        </div>
      )}
    </div>
  );
}
