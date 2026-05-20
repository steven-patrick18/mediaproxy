import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { api, type MediaNode, type MetricPoint, type ProvisionResult } from "../api";
import Bar from "../components/Bar";
import Spark from "../components/Spark";
import Modal from "../components/Modal";
import Help from "../components/Help";
import { PlusIcon, RefreshIcon } from "../components/Icons";

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
  onProvision,
}: {
  n: MediaNode;
  history: MetricPoint[] | undefined;
  onAction: () => void;
  onError: (e: string) => void;
  onProvision: (n: MediaNode) => void;
}) {
  const calls = n.active_calls ?? 0;
  const cpu = Number(n.cpu_pct ?? 0);
  const ram = Number(n.ram_pct ?? 0);
  const netIn = Number(n.net_in_mbps ?? 0);
  const netOut = Number(n.net_out_mbps ?? 0);
  const nicGbps = n.nic_gbps && n.nic_gbps > 0 ? n.nic_gbps : 1; // default 1 Gbps
  const nicMbps = nicGbps * 1000;
  const netPct = pct(netIn + netOut, nicMbps);
  const neverSeen = !n.last_seen_at;

  async function cmd(type: string, label: string) {
    if (!confirm(`Send "${label}" to ${n.name}?`)) return;
    try {
      await api.post(`/api/v1/nodes/${n.id}/commands`, { type });
      onAction();
    } catch (e) {
      onError(e instanceof Error ? e.message : `${label} failed`);
    }
  }
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
          {n.agent_version && <div className="text-slate-400">agent v{n.agent_version}</div>}
        </div>
      </header>

      <div className="mt-4 space-y-2">
        <Bar
          label={n.role === "sip_proxy" ? "Active dialogs (signaling)" : "Active calls (media)"}
          pct={pct(calls, n.max_calls || 1)}
          value={`${calls} / ${n.max_calls || "—"}`}
          tone="info"
        />
        <Bar label="CPU" pct={cpu} value={`${cpu.toFixed(1)}%`} />
        <Bar label="RAM" pct={ram} value={`${ram.toFixed(1)}%`} />
        <Bar label={`Network (${nicGbps} Gbps NIC)`} pct={netPct} value={`${netIn.toFixed(1)} ↓ / ${netOut.toFixed(1)} ↑ Mbps`} />
        <Bar label="IPs bound" pct={pct(n.ips_bound, n.ips_total || 1)} value={`${n.ips_bound} / ${n.ips_total}`} tone="info" />
      </div>

      <div className="mt-3 flex items-center justify-between border-t border-slate-100 pt-3 text-xs text-slate-500">
        <div className="flex items-center gap-2">
          <span className="text-slate-400">last hour CPU:</span>
          <span className="text-brand-600">
            <Spark data={(history ?? []).map((p) => Number(p.cpu_pct ?? 0))} width={100} height={20} />
          </span>
        </div>
      </div>

      <div className="mt-3 flex flex-wrap gap-2 border-t border-slate-100 pt-3 text-xs">
        {neverSeen && (
          <button onClick={() => onProvision(n)} className="rounded bg-brand-600 px-2 py-1 font-medium text-white hover:bg-brand-700">
            Provision via SSH
          </button>
        )}
        <button onClick={() => cmd("apply", "Apply IPs now")} className="rounded border border-slate-300 px-2 py-1 hover:bg-slate-100">
          Apply
        </button>
        <button onClick={() => cmd("restart_rtpengine", "Restart RTPEngine")} disabled={n.role !== "media"} className="rounded border border-slate-300 px-2 py-1 hover:bg-slate-100 disabled:opacity-40">
          Restart RTPEngine
        </button>
        <button onClick={() => cmd("restart_kamailio", "Restart Kamailio")} disabled={n.role !== "sip_proxy"} className="rounded border border-slate-300 px-2 py-1 hover:bg-slate-100 disabled:opacity-40">
          Restart Kamailio
        </button>
        <button onClick={() => cmd("reboot", "Reboot")} className="rounded border border-red-300 px-2 py-1 text-red-700 hover:bg-red-50">
          Reboot
        </button>
        <span className="ml-auto">
          {n.status === "draining" ? (
            <button onClick={undrain} className="text-emerald-600 hover:underline">Undrain</button>
          ) : (
            <button onClick={drain} className="text-amber-700 hover:underline">Drain</button>
          )}
          <span className="mx-2 text-slate-300">·</span>
          <button onClick={del} className="text-red-600 hover:underline">Delete</button>
        </span>
      </div>
    </div>
  );
}

export default function Nodes() {
  const [nodes, setNodes] = useState<MediaNode[]>([]);
  const [historyByNode, setHistoryByNode] = useState<Record<number, MetricPoint[]>>({});
  const [err, setErr] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);
  const [form, setForm] = useState({
    name: "",
    role: "media" as "media" | "sip_proxy",
    host_ip: "",
    region: "",
    nic_gbps: 1,
    max_calls: 2500,
    ssh_user: "root",
    ssh_port: 22,
    ssh_password: "",
  });
  const [busy, setBusy] = useState(false);
  const [created, setCreated] = useState<MediaNode | null>(null);
  const [createLog, setCreateLog] = useState<string | null>(null);

  const [provisioning, setProvisioning] = useState<MediaNode | null>(null);
  const [provForm, setProvForm] = useState({
    ssh_host: "",
    ssh_port: 22,
    ssh_user: "root",
    ssh_password: "",
  });
  const [provResult, setProvResult] = useState<ProvisionResult | null>(null);
  const [provRunning, setProvRunning] = useState(false);

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
    setCreateLog(null);
    try {
      // 1) Create the node so we have an agent_token.
      const node = await api.post<MediaNode>("/api/v1/nodes", {
        name: form.name,
        role: form.role,
        host_ip: form.host_ip,
        region: form.region,
        nic_gbps: form.nic_gbps,
        max_calls: form.max_calls,
      });
      setCreated(node);

      // 2) If a password was given, install the agent right now.
      if (form.ssh_password) {
        setCreateLog("Connecting to " + form.host_ip + " over SSH...\n");
        const res = await api.post<ProvisionResult>(`/api/v1/nodes/${node.id}/provision`, {
          ssh_host: form.host_ip,
          ssh_port: form.ssh_port,
          ssh_user: form.ssh_user,
          ssh_password: form.ssh_password,
        });
        setCreateLog(res.log);
        if (!res.ok) {
          setErr("Node created but provisioning failed — fix the issue and click 'Provision via SSH' on the card to retry.");
        }
      } else {
        // No password supplied — close the modal as before.
        setCreating(false);
      }

      setForm({
        name: "",
        role: "media",
        host_ip: "",
        region: "",
        nic_gbps: 1,
        max_calls: 2500,
        ssh_user: "root",
        ssh_port: 22,
        ssh_password: "",
      });
      reload();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "create failed");
    } finally {
      setBusy(false);
    }
  }

  function openProvision(n: MediaNode) {
    setProvForm({ ssh_host: n.host_ip, ssh_port: 22, ssh_user: "root", ssh_password: "" });
    setProvResult(null);
    setProvisioning(n);
  }
  async function runProvision() {
    if (!provisioning) return;
    setProvRunning(true);
    setProvResult(null);
    try {
      const res = await api.post<ProvisionResult>(`/api/v1/nodes/${provisioning.id}/provision`, provForm);
      setProvResult(res);
      reload();
    } catch (e) {
      setProvResult({ ok: false, log: e instanceof Error ? e.message : "provision failed" });
    } finally {
      setProvRunning(false);
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
          <h2 className="text-xl font-semibold tracking-tight text-slate-800">Nodes</h2>
          <p className="text-xs text-slate-500">
            Media + SIP proxy hosts. Auto-refreshes every 5s. After creating a node, click
            "Provision via SSH" to install the agent on the remote host.
          </p>
        </div>
        <div className="flex gap-2">
          <button onClick={reload} className="inline-flex items-center gap-1.5 rounded-md border border-slate-300 px-3 py-1.5 text-sm hover:bg-slate-50">
            <RefreshIcon /> Refresh
          </button>
          <button onClick={() => setCreating(true)} className="inline-flex items-center gap-1.5 rounded-md bg-brand-600 px-3 py-1.5 text-sm font-medium text-white shadow-sm hover:bg-brand-700">
            <PlusIcon /> Add node
          </button>
        </div>
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
          <div className="text-xs text-slate-500">{cluster.netIn.toFixed(1)} ↓ / {cluster.netOut.toFixed(1)} ↑</div>
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

      {err && <div className="rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{err}</div>}

      {created && (
        <div className="rounded border border-amber-300 bg-amber-50 p-4 text-sm">
          <div className="font-medium text-amber-900">
            Node "{created.name}" created. Click "Provision via SSH" on its card to install the agent.
          </div>
          <details className="mt-2 text-xs">
            <summary className="cursor-pointer">Manual install token (in case SSH provisioning isn't an option)</summary>
            <code className="mt-2 block break-all rounded bg-white p-2 font-mono">{created.agent_token}</code>
          </details>
          <button onClick={() => setCreated(null)} className="mt-2 text-xs text-amber-900 underline">Dismiss</button>
        </div>
      )}

      {nodes.length === 0 ? (
        <div className="rounded-lg border border-dashed border-slate-300 bg-white p-8 text-center text-sm text-slate-500">No nodes yet.</div>
      ) : (
        <div className="grid grid-cols-1 gap-3 xl:grid-cols-2 2xl:grid-cols-3">
          {nodes.map((n) => (
            <NodeCard key={n.id} n={n} history={historyByNode[n.id]} onAction={reload} onError={setErr} onProvision={openProvision} />
          ))}
        </div>
      )}

      <Modal
        open={creating}
        title="Add node"
        onClose={() => { setCreating(false); setCreateLog(null); }}
        width="max-w-2xl"
        footer={
          createLog ? (
            <button onClick={() => { setCreating(false); setCreateLog(null); }} className="rounded bg-brand-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-brand-700">
              Close
            </button>
          ) : (
            <>
              <button onClick={() => setCreating(false)} className="rounded border border-slate-300 px-3 py-1.5 text-sm hover:bg-slate-100">Cancel</button>
              <button onClick={submit as unknown as () => void} disabled={busy || !form.name || !form.host_ip} className="rounded bg-brand-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-brand-700 disabled:opacity-60">
                {busy ? "Installing…" : (form.ssh_password ? "Create + install" : "Create")}
              </button>
            </>
          )
        }
      >
        {!createLog ? (
          <form onSubmit={submit} className="space-y-4">
            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className="block text-xs font-medium text-slate-500">
                  Name
                  <Help>Display name only — anything that helps you identify the host ("media-us-east-1", "kamailio-ny", etc.).</Help>
                </label>
                <input required value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="media-node-1" className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm" />
              </div>
              <div>
                <label className="block text-xs font-medium text-slate-500">
                  Role
                  <Help>
                    <p className="mb-1 font-medium">media</p>
                    Runs RTPEngine. Carries the actual RTP audio packets between dialer
                    and carrier; rotates the media IP per call from the IP Pool.
                    <p className="mb-1 mt-2 font-medium">sip_proxy</p>
                    Runs Kamailio. Handles SIP signaling — receives INVITEs from dialers,
                    looks up the right carrier, applies per-client signaling IP, forwards.
                    A single node can't be both; create separate nodes per role.
                  </Help>
                </label>
                <select
                  value={form.role}
                  onChange={(e) => {
                    const role = e.target.value as "media" | "sip_proxy";
                    // Sensible default: media nodes are RTP-capacity bound (~2500),
                    // sip_proxy nodes can hold many more concurrent dialogs.
                    setForm({ ...form, role, max_calls: role === "sip_proxy" ? 10000 : 2500 });
                  }}
                  className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm"
                >
                  <option value="media">media (RTPEngine)</option>
                  <option value="sip_proxy">sip_proxy (Kamailio)</option>
                </select>
              </div>
              <div className="col-span-2">
                <label className="block text-xs font-medium text-slate-500">
                  Host IP
                  <Help>
                    The management / SSH IP of the remote VPS. Used by the panel to reach
                    the agent (and SSH-install it). The agent's auto-discovered IPs include
                    this one — you can disable it via the Signaling IPs / IP Pool page if
                    you don't want it used for SIP/media.
                  </Help>
                </label>
                <input required value={form.host_ip} onChange={(e) => setForm({ ...form, host_ip: e.target.value })} placeholder="45.77.156.60" className="mt-1 w-full rounded border border-slate-300 px-3 py-2 font-mono text-sm" />
              </div>
              <div>
                <label className="block text-xs font-medium text-slate-500">
                  Region
                  <Help>Free-text geographic tag — "us-east", "ams1", "in-mumbai". Shown on the node card and used later for region-aware routing.</Help>
                </label>
                <input value={form.region} onChange={(e) => setForm({ ...form, region: e.target.value })} placeholder="us-east" className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm" />
              </div>
              <div>
                <label className="block text-xs font-medium text-slate-500">
                  NIC speed (Gbps)
                  <Help>
                    The node's physical NIC capacity. Used as the denominator on the
                    Network bar of the node card so utilization shows correctly. 1 Gbps
                    is the default for cloud-VPS plans; 10 Gbps for dedicated boxes.
                  </Help>
                </label>
                <select value={form.nic_gbps} onChange={(e) => setForm({ ...form, nic_gbps: Number(e.target.value) })} className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm">
                  <option value={1}>1 Gbps</option>
                  <option value={2}>2 Gbps</option>
                  <option value={5}>5 Gbps</option>
                  <option value={10}>10 Gbps</option>
                  <option value={25}>25 Gbps</option>
                  <option value={40}>40 Gbps</option>
                </select>
              </div>
              <div className="col-span-2">
                <label className="block text-xs font-medium text-slate-500">
                  {form.role === "sip_proxy" ? "Max concurrent dialogs (signaling)" : "Max concurrent calls (media)"}
                </label>
                <input
                  type="number"
                  min={0}
                  value={form.max_calls}
                  onChange={(e) => setForm({ ...form, max_calls: Number(e.target.value) })}
                  className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm"
                />
                <p className="mt-1 text-xs text-slate-400">
                  {form.role === "sip_proxy"
                    ? "Kamailio is light per dialog — 10K+ is fine on a modest VPS. Signaling is rarely the bottleneck."
                    : "RTPEngine's hard capacity for media streams on this node. Hit this and new calls fail over to another media node."}
                </p>
              </div>
            </div>

            <div className="rounded border border-slate-200 bg-slate-50 p-3">
              <div className="mb-2 flex items-baseline justify-between">
                <h4 className="text-sm font-medium text-slate-700">SSH auto-install</h4>
                <span className="text-xs text-slate-500">leave password blank to skip</span>
              </div>
              <p className="mb-3 text-xs text-slate-500">
                If you fill the password, the panel will SSH into the host as the user below, download
                the agent binary, write its config + systemd unit, and start it. The password is used
                once and never stored.
              </p>
              <div className="grid grid-cols-3 gap-3">
                <div>
                  <label className="block text-xs font-medium text-slate-500">SSH user</label>
                  <input value={form.ssh_user} onChange={(e) => setForm({ ...form, ssh_user: e.target.value })} className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm" />
                </div>
                <div>
                  <label className="block text-xs font-medium text-slate-500">SSH port</label>
                  <input type="number" value={form.ssh_port} onChange={(e) => setForm({ ...form, ssh_port: Number(e.target.value) })} className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm" />
                </div>
                <div>
                  <label className="block text-xs font-medium text-slate-500">SSH password</label>
                  <input type="password" autoComplete="new-password" value={form.ssh_password} onChange={(e) => setForm({ ...form, ssh_password: e.target.value })} placeholder="(optional)" className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm" />
                </div>
              </div>
            </div>
          </form>
        ) : (
          <div className="space-y-2">
            <div className="rounded bg-emerald-50 p-2 text-sm font-medium text-emerald-800">
              Install log
            </div>
            <pre className="max-h-96 overflow-auto rounded bg-slate-900 p-3 font-mono text-xs text-slate-100">{createLog}</pre>
            <p className="text-xs text-slate-500">
              The node will flip to <strong>online</strong> on its first heartbeat (within ~10s).
              You can close this — the Nodes page is auto-refreshing.
            </p>
          </div>
        )}
      </Modal>

      <Modal
        open={provisioning !== null}
        title={provisioning ? `Provision ${provisioning.name} via SSH` : ""}
        onClose={() => setProvisioning(null)}
        width="max-w-2xl"
        footer={
          !provRunning && (
            <>
              <button onClick={() => setProvisioning(null)} className="rounded border border-slate-300 px-3 py-1.5 text-sm hover:bg-slate-100">
                {provResult ? "Close" : "Cancel"}
              </button>
              {!provResult && (
                <button onClick={runProvision} disabled={!provForm.ssh_password} className="rounded bg-brand-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-brand-700 disabled:opacity-60">
                  Install agent
                </button>
              )}
            </>
          )
        }
      >
        {!provResult && !provRunning && (
          <div className="space-y-3">
            <p className="text-xs text-slate-500">
              The base-app will SSH in as the user below, download the agent binary from{" "}
              <code className="font-mono">/agent-binary</code>, write{" "}
              <code className="font-mono">/etc/node-agent/config.yaml</code> with this node's agent
              token, install a systemd unit, and start it. The password is used once and never stored.
            </p>
            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className="block text-xs font-medium text-slate-500">SSH host</label>
                <input value={provForm.ssh_host} onChange={(e) => setProvForm({ ...provForm, ssh_host: e.target.value })} className="mt-1 w-full rounded border border-slate-300 px-3 py-2 font-mono text-sm" />
              </div>
              <div>
                <label className="block text-xs font-medium text-slate-500">SSH port</label>
                <input type="number" value={provForm.ssh_port} onChange={(e) => setProvForm({ ...provForm, ssh_port: Number(e.target.value) })} className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm" />
              </div>
              <div>
                <label className="block text-xs font-medium text-slate-500">SSH user</label>
                <input value={provForm.ssh_user} onChange={(e) => setProvForm({ ...provForm, ssh_user: e.target.value })} className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm" />
              </div>
              <div>
                <label className="block text-xs font-medium text-slate-500">SSH password</label>
                <input type="password" value={provForm.ssh_password} onChange={(e) => setProvForm({ ...provForm, ssh_password: e.target.value })} className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm" />
              </div>
            </div>
          </div>
        )}
        {provRunning && (
          <div className="py-8 text-center text-sm text-slate-500">
            <div className="mx-auto mb-3 h-6 w-6 animate-spin rounded-full border-2 border-slate-200 border-t-brand-600" />
            Installing… this can take up to a minute.
          </div>
        )}
        {provResult && (
          <div className="space-y-2">
            <div className={`rounded p-2 text-sm font-medium ${provResult.ok ? "bg-emerald-50 text-emerald-800" : "bg-red-50 text-red-800"}`}>
              {provResult.ok ? "Provisioning complete." : "Provisioning failed."}
            </div>
            <pre className="max-h-96 overflow-auto rounded bg-slate-900 p-3 font-mono text-xs text-slate-100">
              {provResult.log}
            </pre>
          </div>
        )}
      </Modal>
    </div>
  );
}
