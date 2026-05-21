import { useCallback, useEffect, useState } from "react";
import {
  api,
  type Client,
  type MediaNode,
  type Reseller,
  type SignalingIP,
  type NodeIP,
  type Carrier,
} from "../api";
import Bar from "../components/Bar";

export default function Dashboard() {
  const [resellers, setResellers] = useState<Reseller[]>([]);
  const [clients, setClients] = useState<Client[]>([]);
  const [nodes, setNodes] = useState<MediaNode[]>([]);
  const [sigs, setSigs] = useState<SignalingIP[]>([]);
  const [nodeIPs, setNodeIPs] = useState<NodeIP[]>([]);
  const [carriers, setCarriers] = useState<Carrier[]>([]);
  const [err, setErr] = useState<string | null>(null);

  const reload = useCallback(() => {
    Promise.all([
      api.get<Reseller[]>("/api/v1/resellers"),
      api.get<Client[]>("/api/v1/clients"),
      api.get<MediaNode[]>("/api/v1/nodes"),
      api.get<SignalingIP[]>("/api/v1/signaling-ips"),
      api.get<NodeIP[]>("/api/v1/node-ips"),
      api.get<Carrier[]>("/api/v1/carriers"),
    ])
      .then(([r, c, n, s, ip, ca]) => {
        setResellers(r);
        setClients(c);
        setNodes(n);
        setSigs(s);
        setNodeIPs(ip);
        setCarriers(ca);
      })
      .catch((e) => setErr(e.message));
  }, []);

  useEffect(() => {
    reload();
    const t = setInterval(reload, 5000);
    return () => clearInterval(t);
  }, [reload]);

  const onlineNodes = nodes.filter((n) => n.status === "online").length;
  const drainingNodes = nodes.filter((n) => n.status === "draining").length;
  // Each call has one signaling row (sip_proxy) and one media row
  // (media). Summing both double-counts every call. Use sip_proxy only
  // for the active count and media only for the capacity — media RTP
  // capacity is the real bottleneck.
  const totalCalls = nodes
    .filter((n) => n.role === "sip_proxy")
    .reduce((s, n) => s + (n.active_calls ?? 0), 0);
  const maxCalls = nodes
    .filter((n) => n.role === "media")
    .reduce((s, n) => s + n.max_calls, 0);
  const totalBw = nodes.reduce((s, n) => s + Number(n.net_in_mbps ?? 0) + Number(n.net_out_mbps ?? 0), 0);
  const ipsActive = nodeIPs.filter((ip) => ip.status === "active").length;
  const ipsFlagged = nodeIPs.filter((ip) => ip.status === "flagged").length;
  const sigsAssigned = sigs.filter((s) => s.status === "assigned").length;

  const cards = [
    { label: "Active calls", value: totalCalls.toLocaleString(), sub: `of ${maxCalls.toLocaleString()} max` },
    { label: "Online nodes", value: `${onlineNodes} / ${nodes.length}`, sub: drainingNodes > 0 ? `${drainingNodes} draining` : "" },
    { label: "Bandwidth", value: `${totalBw.toFixed(1)} Mbps`, sub: "in + out, cluster" },
    { label: "Pool IPs", value: `${ipsActive}`, sub: ipsFlagged > 0 ? `${ipsFlagged} flagged` : `of ${nodeIPs.length} total` },
    { label: "Signaling IPs", value: `${sigsAssigned} / ${sigs.length}`, sub: "assigned" },
    { label: "Resellers", value: resellers.length.toString(), sub: `${clients.length} clients · ${carriers.length} carriers` },
  ];

  return (
    <div className="space-y-6">
      <header>
        <h2 className="text-2xl font-semibold tracking-tight">Dashboard</h2>
        <p className="text-sm text-slate-500">Live cluster snapshot — refreshes every 5s.</p>
      </header>

      {err && (
        <div className="rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{err}</div>
      )}

      <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-6">
        {cards.map((c) => (
          <div key={c.label} className="rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
            <div className="text-xs uppercase tracking-wide text-slate-500">{c.label}</div>
            <div className="mt-2 text-2xl font-semibold tracking-tight">{c.value}</div>
            <div className="mt-0.5 text-xs text-slate-500">{c.sub}</div>
          </div>
        ))}
      </div>

      <section className="rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
        <h3 className="mb-3 text-sm font-semibold uppercase tracking-wide text-slate-500">
          Per-node load
        </h3>
        {nodes.length === 0 ? (
          <p className="py-4 text-center text-sm text-slate-400">No nodes yet.</p>
        ) : (
          <div className="space-y-3">
            {nodes.map((n) => {
              const callPct = n.max_calls ? Math.min(100, ((n.active_calls ?? 0) / n.max_calls) * 100) : 0;
              return (
                <div key={n.id} className="grid grid-cols-1 items-center gap-3 sm:grid-cols-[1fr_2fr_2fr_2fr]">
                  <div>
                    <div className="text-sm font-medium">{n.name}</div>
                    <div className="text-xs text-slate-500">
                      {n.role} · <span className={n.status === "online" ? "text-emerald-600" : n.status === "draining" ? "text-amber-700" : "text-slate-400"}>{n.status}</span>
                    </div>
                  </div>
                  <Bar
                    label="Calls"
                    pct={callPct}
                    value={`${n.active_calls ?? 0} / ${n.max_calls || "—"}`}
                    tone="info"
                  />
                  <Bar label="CPU" pct={Number(n.cpu_pct ?? 0)} value={`${Number(n.cpu_pct ?? 0).toFixed(0)}%`} />
                  <Bar label="RAM" pct={Number(n.ram_pct ?? 0)} value={`${Number(n.ram_pct ?? 0).toFixed(0)}%`} />
                </div>
              );
            })}
          </div>
        )}
      </section>
    </div>
  );
}
