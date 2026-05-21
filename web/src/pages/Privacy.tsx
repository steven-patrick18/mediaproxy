import { useCallback, useEffect, useMemo, useState } from "react";
import { api, type ActiveCallRow, type Carrier, type Client } from "../api";
import { RefreshIcon } from "../components/Icons";
import Help from "../components/Help";

// Call Privacy Monitor — shows which carriers' servers your calls are
// flowing through. The page is intentionally read-only and derives all
// numbers from the existing /calls/active + /carriers endpoints; no
// backend or kamailio changes have been made.
//
// Honest model:
//   - "Exposed" = the call's signaling+media path traverses a wholesale
//     carrier's network. That carrier *can* record audio.
//   - "Private" = a direct peer-to-peer media leg with no carrier middle-
//     box. Almost never exists in conventional SIP trunking.
//   - "Encryption known" requires SDP capture (RTP/AVP vs RTP/SAVP). That
//     capture isn't wired up yet, so we show "unknown" rather than guess.

type Severity = "exposed" | "private" | "unknown";

interface CarrierRow {
  id: number;
  name: string;
  status: string;
  count: number;
  severity: Severity;
}

export default function Privacy() {
  const [active, setActive] = useState<ActiveCallRow[]>([]);
  const [carriers, setCarriers] = useState<Carrier[]>([]);
  const [clients, setClients] = useState<Client[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [paused, setPaused] = useState(false);

  const reload = useCallback(() => {
    Promise.all([
      api.get<ActiveCallRow[]>("/api/v1/calls/active"),
      api.get<Carrier[]>("/api/v1/carriers"),
      api.get<Client[]>("/api/v1/clients"),
    ])
      .then(([a, c, cl]) => {
        setActive(a);
        setCarriers(c);
        setClients(cl);
        setErr(null);
      })
      .catch((e) => setErr(e.message));
  }, []);

  useEffect(() => {
    reload();
    if (paused) return;
    const t = setInterval(reload, 3000);
    return () => clearInterval(t);
  }, [reload, paused]);

  const clientName = (id: number | null | undefined) =>
    id ? clients.find((c) => c.id === id)?.name ?? `#${id}` : "—";
  const carrierName = (id: number | null | undefined) =>
    id ? carriers.find((c) => c.id === id)?.name ?? `#${id}` : "—";

  // Aggregate per carrier. Every active call that has a carrier_id is
  // considered "exposed" because, architecturally, its RTP flows through
  // that carrier's media gateway. Calls with no carrier_id are skipped
  // (these would be administrative / OPTIONS / etc).
  const perCarrier: CarrierRow[] = useMemo(() => {
    const counts = new Map<number, number>();
    for (const r of active) {
      if (r.carrier_id) counts.set(r.carrier_id, (counts.get(r.carrier_id) ?? 0) + 1);
    }
    const list: CarrierRow[] = carriers.map((c) => ({
      id: c.id,
      name: c.name,
      status: c.status,
      count: counts.get(c.id) ?? 0,
      severity: "exposed" as Severity,
    }));
    list.sort((a, b) => b.count - a.count);
    return list;
  }, [active, carriers]);

  const totalCalls = active.length;
  const exposedCalls = active.filter((r) => !!r.carrier_id).length;
  const privateCalls = 0; // Always 0 until media-bypass is implemented; honest 0 beats a fake number.
  const exposedPct = totalCalls > 0 ? Math.round((exposedCalls / totalCalls) * 100) : 0;

  const overallStatus =
    totalCalls === 0
      ? { dot: "⚪", label: "No active calls", tone: "text-slate-500" }
      : exposedPct >= 80
        ? { dot: "🔴", label: "Most calls are exposed", tone: "text-rose-700" }
        : exposedPct >= 40
          ? { dot: "🟠", label: "Mixed — some calls exposed", tone: "text-amber-700" }
          : { dot: "🟢", label: "Most calls private", tone: "text-emerald-700" };

  // Sort live feed by start time, newest first.
  const liveFeed = [...active]
    .sort((a, b) => new Date(b.started_at).getTime() - new Date(a.started_at).getTime())
    .slice(0, 12);

  function severityBadge(sev: Severity) {
    if (sev === "exposed") {
      return <span className="rounded bg-rose-100 px-2 py-0.5 text-xs font-medium text-rose-800">🔴 Exposed</span>;
    }
    if (sev === "private") {
      return <span className="rounded bg-emerald-100 px-2 py-0.5 text-xs font-medium text-emerald-800">🟢 Private</span>;
    }
    return <span className="rounded bg-slate-100 px-2 py-0.5 text-xs font-medium text-slate-700">⚪ Idle</span>;
  }

  return (
    <div className="space-y-4">
      <header className="flex items-start justify-between">
        <div>
          <h2 className="text-xl font-semibold tracking-tight text-slate-800">Call Privacy Monitor</h2>
          <p className="text-xs text-slate-500">
            Are your calls passing through other companies' servers? Refreshes every 3s. Shows
            ability to record (architectural exposure), not actual recording activity.
          </p>
        </div>
        <div className="flex gap-2">
          <button
            onClick={() => setPaused((p) => !p)}
            className={`rounded-md border px-3 py-1.5 text-sm ${
              paused
                ? "border-emerald-300 bg-emerald-50 text-emerald-800 hover:bg-emerald-100"
                : "border-slate-300 hover:bg-slate-50"
            }`}
          >
            {paused ? "▶ Resume" : "⏸ Pause"}
          </button>
          <button
            onClick={reload}
            className="inline-flex items-center gap-1.5 rounded-md border border-slate-300 px-3 py-1.5 text-sm hover:bg-slate-50"
          >
            <RefreshIcon /> Refresh
          </button>
        </div>
      </header>

      {err && (
        <div className="rounded border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">{err}</div>
      )}

      {/* Section 1 — Overall health */}
      <section className="rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
        <h3 className="text-sm font-semibold uppercase tracking-wide text-slate-500">Overall health</h3>
        <div className="mt-3 grid grid-cols-1 gap-4 sm:grid-cols-3">
          <div>
            <div className="text-xs uppercase tracking-wide text-slate-500">Total calls now</div>
            <div className="mt-1 text-3xl font-semibold text-slate-800">{totalCalls}</div>
          </div>
          <div>
            <div className="flex items-center gap-1 text-xs uppercase tracking-wide text-slate-500">
              Calls they can hear
              <Help>
                A wholesale carrier's media gateway sits in the audio path of every call routed
                through them. That gateway <strong>could</strong> record. This count includes any
                call assigned a carrier_id in the panel.
              </Help>
            </div>
            <div className="mt-1 flex items-baseline gap-2">
              <span className="text-3xl font-semibold text-rose-700">{exposedCalls}</span>
              <span className="text-sm text-slate-500">({exposedPct}%)</span>
            </div>
          </div>
          <div>
            <div className="flex items-center gap-1 text-xs uppercase tracking-wide text-slate-500">
              Private calls
              <Help>
                Direct peer-to-peer media with no carrier middlebox. Almost never exists in
                conventional SIP trunking; would require media-bypass / ICE-direct + DTLS-SRTP
                with verified fingerprints. Always <code>0</code> until that's implemented.
              </Help>
            </div>
            <div className="mt-1 flex items-baseline gap-2">
              <span className="text-3xl font-semibold text-emerald-700">{privateCalls}</span>
              <span className="text-sm text-slate-500">(0%)</span>
            </div>
          </div>
        </div>
        <div className={`mt-4 text-sm font-medium ${overallStatus.tone}`}>
          Overall status: {overallStatus.dot} {overallStatus.label}
        </div>
      </section>

      {/* Section 2 — By carrier */}
      <section className="rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
        <h3 className="text-sm font-semibold uppercase tracking-wide text-slate-500">By company (upstream)</h3>
        <p className="mt-1 text-xs text-slate-500">
          Per-carrier breakdown of currently-active calls.
        </p>
        <div className="mt-3 overflow-hidden rounded border border-slate-200">
          <table className="min-w-full divide-y divide-slate-200 text-sm">
            <thead className="bg-slate-50 text-left text-xs uppercase tracking-wide text-slate-500">
              <tr>
                <th className="px-3 py-2">Company</th>
                <th className="px-3 py-2 text-right">Calls</th>
                <th className="px-3 py-2">Can they hear?</th>
                <th className="px-3 py-2">Encrypted?</th>
                <th className="px-3 py-2">Status</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100">
              {perCarrier.length === 0 && (
                <tr>
                  <td colSpan={5} className="px-3 py-6 text-center text-slate-400">
                    No carriers defined yet.
                  </td>
                </tr>
              )}
              {perCarrier.map((c) => {
                const sev: Severity = c.count === 0 ? "unknown" : "exposed";
                return (
                  <tr key={c.id}>
                    <td className="px-3 py-2 font-medium">{c.name}</td>
                    <td className="px-3 py-2 text-right font-mono">{c.count}</td>
                    <td className="px-3 py-2 text-xs">
                      {c.count === 0 ? (
                        <span className="text-slate-400">—</span>
                      ) : (
                        <span className="text-rose-700">YES — all pass through them</span>
                      )}
                    </td>
                    <td className="px-3 py-2 text-xs">
                      <span className="text-slate-500" title="Encryption status requires SDP capture, not yet enabled">
                        Unknown
                      </span>
                    </td>
                    <td className="px-3 py-2">{severityBadge(sev)}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </section>

      {/* Section 3 — Live feed */}
      <section className="rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
        <h3 className="text-sm font-semibold uppercase tracking-wide text-slate-500">
          Live call feed{" "}
          <span className="ml-2 text-xs font-normal lowercase text-slate-400">newest on top</span>
        </h3>
        <div className="mt-3 overflow-hidden rounded border border-slate-200">
          <table className="min-w-full divide-y divide-slate-200 text-sm">
            <thead className="bg-slate-50 text-left text-xs uppercase tracking-wide text-slate-500">
              <tr>
                <th className="px-3 py-2">Time</th>
                <th className="px-3 py-2">Company</th>
                <th className="px-3 py-2">Client</th>
                <th className="px-3 py-2">From → To</th>
                <th className="px-3 py-2">Can hear?</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100">
              {liveFeed.length === 0 && (
                <tr>
                  <td colSpan={5} className="px-3 py-6 text-center text-slate-400">
                    No live calls right now.
                  </td>
                </tr>
              )}
              {liveFeed.map((r) => (
                <tr key={r.id}>
                  <td className="px-3 py-1.5 text-xs text-slate-500">
                    {new Date(r.started_at).toLocaleTimeString()}
                  </td>
                  <td className="px-3 py-1.5">{carrierName(r.carrier_id)}</td>
                  <td className="px-3 py-1.5">{clientName(r.client_id)}</td>
                  <td className="px-3 py-1.5 font-mono text-xs">
                    {r.ani ?? "?"} → {r.dnis ?? "?"}
                  </td>
                  <td className="px-3 py-1.5 text-xs">
                    {r.carrier_id ? (
                      <span className="text-rose-700">🔴 Yes — passes through carrier</span>
                    ) : (
                      <span className="text-slate-500">—</span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>

      {/* Section 4 — Alerts */}
      <section className="rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
        <h3 className="text-sm font-semibold uppercase tracking-wide text-slate-500">
          Watch out (alerts, last 15 min)
        </h3>
        <div className="mt-3 rounded border border-dashed border-slate-300 bg-slate-50 px-4 py-6 text-center text-sm text-slate-500">
          No alerts.
          <p className="mt-2 text-xs text-slate-400">
            Mid-call re-routing alerts and unencrypted-call warnings require SDP capture in the
            Kamailio call-start hook. Not enabled yet — when it is, alerts will surface here.
          </p>
        </div>
      </section>

      {/* Section 5 — What this means */}
      <section className="rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
        <h3 className="text-sm font-semibold uppercase tracking-wide text-slate-500">What this means</h3>
        <ul className="mt-2 space-y-2 text-sm text-slate-600">
          <li>
            <strong>"Can they hear"</strong> means the call's audio passes through that company's
            server, so they are <em>able</em> to listen or record it. It does NOT mean they are
            recording right now.
          </li>
          <li>
            <strong>No external tool can see inside a carrier's network.</strong> This page shows
            architectural <em>exposure</em>, never proof of actual recording.
          </li>
          <li>
            <strong>SRTP / encryption</strong> labels will be filled in once we capture SDP in the
            call-start hook. Note: SDES-SRTP (the common flavor in SIP trunks) exchanges keys in
            cleartext signaling — a carrier in the signaling path can still decrypt. Only
            DTLS-SRTP with verified fingerprints actually blinds the carrier.
          </li>
          <li>
            <strong>"Private" calls (peer-to-peer media bypass)</strong> are zero on a conventional
            wholesale SBC setup — by design. The wholesale carrier <em>is</em> the only path. To
            get real private calls you'd need direct peer interconnects with media-bypass + ICE +
            DTLS-SRTP.
          </li>
        </ul>
      </section>
    </div>
  );
}
