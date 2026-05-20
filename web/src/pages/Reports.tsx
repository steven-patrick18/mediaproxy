import { useEffect, useState } from "react";
import { api, type CDRStats } from "../api";

export default function Reports() {
  const [today, setToday] = useState<CDRStats | null>(null);
  const [yesterday, setYesterday] = useState<CDRStats | null>(null);
  const [week, setWeek] = useState<CDRStats | null>(null);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    const now = new Date();
    const t0 = new Date(now); t0.setHours(0, 0, 0, 0);
    const y0 = new Date(t0); y0.setDate(y0.getDate() - 1);
    const w0 = new Date(t0); w0.setDate(w0.getDate() - 7);
    Promise.all([
      api.get<CDRStats>(`/api/v1/cdrs/stats?from=${encodeURIComponent(t0.toISOString())}`),
      api.get<CDRStats>(`/api/v1/cdrs/stats?from=${encodeURIComponent(y0.toISOString())}&to=${encodeURIComponent(t0.toISOString())}`),
      api.get<CDRStats>(`/api/v1/cdrs/stats?from=${encodeURIComponent(w0.toISOString())}`),
    ])
      .then(([t, y, w]) => { setToday(t); setYesterday(y); setWeek(w); })
      .catch((e) => setErr(e.message));
  }, []);

  return (
    <div className="space-y-4">
      <header>
        <h2 className="text-xl font-semibold tracking-tight text-slate-800">Reports</h2>
        <p className="text-xs text-slate-500">Aggregate ASR/ACD/volume. Will deepen as CDR data accumulates.</p>
      </header>

      {err && <div className="rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{err}</div>}

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <Period title="Today" s={today} />
        <Period title="Yesterday" s={yesterday} />
        <Period title="Last 7 days" s={week} />
      </div>

      <div className="rounded-lg border border-dashed border-slate-300 bg-white p-6 text-center text-sm text-slate-500">
        Per-client / per-carrier / per-IP breakdowns will be added once there's CDR volume to show.
      </div>
    </div>
  );
}

function Period({ title, s }: { title: string; s: CDRStats | null }) {
  return (
    <div className="rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
      <h3 className="mb-3 text-sm font-semibold text-slate-700">{title}</h3>
      <dl className="grid grid-cols-2 gap-y-2 text-sm">
        <dt className="text-slate-500">Total</dt>
        <dd className="text-right font-mono">{s?.total ?? "—"}</dd>
        <dt className="text-slate-500">Answered</dt>
        <dd className="text-right font-mono">{s?.answered ?? "—"}</dd>
        <dt className="text-slate-500">ASR</dt>
        <dd className="text-right font-mono">{s ? `${s.asr_pct.toFixed(1)}%` : "—"}</dd>
        <dt className="text-slate-500">ACD</dt>
        <dd className="text-right font-mono">{s?.acd_seconds ? `${s.acd_seconds.toFixed(0)}s` : "—"}</dd>
      </dl>
    </div>
  );
}
