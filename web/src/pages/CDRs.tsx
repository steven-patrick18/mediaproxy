import { useCallback, useEffect, useState } from "react";
import { api, type CDR, type CDRStats, type Carrier, type Client } from "../api";
import { RefreshIcon, DownloadIcon } from "../components/Icons";
import SipTraceModal from "../components/SipTraceModal";
import ResetDataModal from "../components/ResetDataModal";

function fmtDur(sec: number | null | undefined): string {
  if (sec == null) return "—";
  const m = Math.floor(sec / 60);
  const s = sec % 60;
  return `${m}:${s.toString().padStart(2, "0")}`;
}
function dispoBadge(d: string | null | undefined) {
  if (!d) return null;
  const cls: Record<string, string> = {
    answered: "bg-emerald-100 text-emerald-800",
    busy: "bg-amber-100 text-amber-800",
    no_answer: "bg-amber-100 text-amber-800",
    failed: "bg-red-100 text-red-800",
    canceled: "bg-slate-200 text-slate-700",
  };
  return <span className={`rounded px-2 py-0.5 text-xs ${cls[d] ?? "bg-slate-200"}`}>{d}</span>;
}

export default function CDRs() {
  const [rows, setRows] = useState<CDR[]>([]);
  const [stats, setStats] = useState<CDRStats | null>(null);
  const [clients, setClients] = useState<Client[]>([]);
  const [carriers, setCarriers] = useState<Carrier[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [filters, setFilters] = useState({
    client_id: 0,
    carrier_id: 0,
    disposition: "",
    from: "",
    to: "",
    dnis: "",
  });
  const [traceFor, setTraceFor] = useState<{ call_id: string; started_at?: string | null } | null>(null);
  const [resetOpen, setResetOpen] = useState(false);

  function filterParts(): string[] {
    const parts: string[] = [];
    if (filters.client_id) parts.push(`client_id=${filters.client_id}`);
    if (filters.carrier_id) parts.push(`carrier_id=${filters.carrier_id}`);
    if (filters.disposition) parts.push(`disposition=${encodeURIComponent(filters.disposition)}`);
    if (filters.from) parts.push(`from=${encodeURIComponent(filters.from)}`);
    if (filters.to) parts.push(`to=${encodeURIComponent(filters.to)}`);
    if (filters.dnis) parts.push(`dnis=${encodeURIComponent(filters.dnis)}`);
    return parts;
  }

  const reload = useCallback(async () => {
    setBusy(true);
    setErr(null);
    try {
      const filters = filterParts();
      const listURL = `/api/v1/cdrs?${[...filters, "limit=200"].join("&")}`;
      const statsURL = `/api/v1/cdrs/stats${filters.length ? "?" + filters.join("&") : ""}`;
      const [r, s, c, ca] = await Promise.all([
        api.get<CDR[]>(listURL),
        api.get<CDRStats>(statsURL),
        api.get<Client[]>("/api/v1/clients"),
        api.get<Carrier[]>("/api/v1/carriers"),
      ]);
      setRows(r);
      setStats(s);
      setClients(c);
      setCarriers(ca);
    } catch (e) {
      setErr(e instanceof Error ? e.message : "load failed");
    } finally {
      setBusy(false);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filters]);
  useEffect(() => { reload(); }, [reload]);

  function exportCSV() {
    const headers = ["id","started_at","ani","dnis","duration_sec","disposition","sip_code","client_id","carrier_id","node_id","media_ip"];
    const lines = [headers.join(",")];
    for (const r of rows) {
      lines.push([
        r.id, r.started_at, r.ani ?? "", r.dnis ?? "",
        r.duration_sec ?? "", r.disposition ?? "", r.sip_code ?? "",
        r.client_id ?? "", r.carrier_id ?? "", r.node_id ?? "", r.media_ip ?? ""
      ].map((x) => `"${String(x).replace(/"/g,'""')}"`).join(","));
    }
    const blob = new Blob([lines.join("\n")], { type: "text/csv" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `cdrs-${new Date().toISOString()}.csv`;
    a.click();
    URL.revokeObjectURL(url);
  }

  return (
    <div className="space-y-4">
      <header className="flex items-start justify-between">
        <div>
          <h2 className="text-xl font-semibold tracking-tight text-slate-800">CDRs</h2>
          <p className="text-xs text-slate-500">Call detail records. Empty until Kamailio is in the call path.</p>
        </div>
        <div className="flex gap-2">
          <button onClick={reload} className="inline-flex items-center gap-1.5 rounded-md border border-slate-300 px-3 py-1.5 text-sm hover:bg-slate-50">
            <RefreshIcon /> Refresh
          </button>
          <button
            onClick={() => setResetOpen(true)}
            className="inline-flex items-center gap-1.5 rounded-md border border-rose-300 px-3 py-1.5 text-sm text-rose-700 hover:bg-rose-50"
            title="Wipe operational data (active calls, CDRs, metrics). Config not affected."
          >
            Clear data
          </button>
          <button onClick={exportCSV} disabled={rows.length === 0} className="inline-flex items-center gap-1.5 rounded-md border border-slate-300 px-3 py-1.5 text-sm hover:bg-slate-50 disabled:opacity-50">
            <DownloadIcon /> Export CSV
          </button>
        </div>
      </header>

      {err && <div className="rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{err}</div>}

      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <Stat label="Total calls" value={stats?.total ?? 0} />
        <Stat label="Answered" value={stats?.answered ?? 0} />
        <Stat label="ASR" value={`${(stats?.asr_pct ?? 0).toFixed(1)}%`} />
        <Stat label="ACD" value={stats?.acd_seconds ? `${stats.acd_seconds.toFixed(0)}s` : "—"} />
      </div>

      <div className="grid grid-cols-1 gap-3 rounded-lg border border-slate-200 bg-white p-3 shadow-sm sm:grid-cols-3 lg:grid-cols-6">
        <select value={filters.client_id} onChange={(e) => setFilters({ ...filters, client_id: Number(e.target.value) })} className="rounded border border-slate-300 px-2 py-1 text-sm">
          <option value={0}>any client</option>
          {clients.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
        </select>
        <select value={filters.carrier_id} onChange={(e) => setFilters({ ...filters, carrier_id: Number(e.target.value) })} className="rounded border border-slate-300 px-2 py-1 text-sm">
          <option value={0}>any carrier</option>
          {carriers.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
        </select>
        <select value={filters.disposition} onChange={(e) => setFilters({ ...filters, disposition: e.target.value })} className="rounded border border-slate-300 px-2 py-1 text-sm">
          <option value="">any disposition</option>
          <option value="answered">answered</option>
          <option value="busy">busy</option>
          <option value="no_answer">no_answer</option>
          <option value="failed">failed</option>
          <option value="canceled">canceled</option>
        </select>
        <input type="datetime-local" value={filters.from} onChange={(e) => setFilters({ ...filters, from: e.target.value })} className="rounded border border-slate-300 px-2 py-1 text-sm" />
        <input type="datetime-local" value={filters.to} onChange={(e) => setFilters({ ...filters, to: e.target.value })} className="rounded border border-slate-300 px-2 py-1 text-sm" />
        <input value={filters.dnis} onChange={(e) => setFilters({ ...filters, dnis: e.target.value })} placeholder="DNIS prefix" className="rounded border border-slate-300 px-2 py-1 text-sm font-mono" />
      </div>

      <div className="overflow-hidden rounded-lg border border-slate-200 bg-white shadow-sm">
        <table className="min-w-full divide-y divide-slate-200 text-sm">
          <thead className="bg-slate-50 text-left text-xs uppercase tracking-wide text-slate-500">
            <tr>
              <th className="px-3 py-2">Started</th>
              <th className="px-3 py-2">Dur</th>
              <th className="px-3 py-2">ANI</th>
              <th className="px-3 py-2">DNIS</th>
              <th className="px-3 py-2">Client</th>
              <th className="px-3 py-2">Carrier</th>
              <th className="px-3 py-2">Node</th>
              <th className="px-3 py-2">Disposition</th>
              <th className="px-3 py-2">SIP</th>
              <th className="px-3 py-2 text-right">Trace</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100">
            {rows.length === 0 && !busy && (
              <tr><td colSpan={10} className="px-3 py-8 text-center text-slate-400">No CDRs match.</td></tr>
            )}
            {rows.map((r) => (
              <tr key={r.id} className="hover:bg-slate-50">
                <td className="px-3 py-1.5 text-xs text-slate-500">{new Date(r.started_at).toLocaleString()}</td>
                <td className="px-3 py-1.5 font-mono">{fmtDur(r.duration_sec)}</td>
                <td className="px-3 py-1.5 font-mono text-xs">{r.ani ?? "—"}</td>
                <td className="px-3 py-1.5 font-mono text-xs">{r.dnis ?? "—"}</td>
                <td className="px-3 py-1.5">{r.client_id ?? "—"}</td>
                <td className="px-3 py-1.5">{r.carrier_id ?? "—"}</td>
                <td className="px-3 py-1.5">{r.node_id ?? "—"}</td>
                <td className="px-3 py-1.5">{dispoBadge(r.disposition)}</td>
                <td className="px-3 py-1.5 font-mono text-xs">{r.sip_code ?? "—"}</td>
                <td className="px-3 py-1.5 text-right">
                  <button
                    onClick={() =>
                      setTraceFor({
                        call_id: r.call_id,
                        started_at: r.started_at ?? null,
                      })
                    }
                    title="Open SIP ladder for this call"
                    className="rounded border border-slate-300 px-2 py-0.5 text-xs text-slate-600 hover:border-brand-400 hover:bg-brand-50 hover:text-brand-700"
                  >
                    SIP
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {traceFor && (
        <SipTraceModal
          callID={traceFor.call_id}
          startedAt={traceFor.started_at}
          onClose={() => setTraceFor(null)}
        />
      )}
      {resetOpen && (
        <ResetDataModal
          onClose={() => {
            setResetOpen(false);
            reload();
          }}
        />
      )}
    </div>
  );
}

function Stat({ label, value }: { label: string; value: number | string }) {
  return (
    <div className="rounded-lg border border-slate-200 bg-white p-3 shadow-sm">
      <div className="text-xs uppercase tracking-wide text-slate-500">{label}</div>
      <div className="mt-1 text-2xl font-semibold">{value}</div>
    </div>
  );
}
