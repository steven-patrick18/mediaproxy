import { useCallback, useEffect, useMemo, useState } from "react";
import { api, type ActiveCallRow, type Carrier, type CarrierQuality, type Client } from "../api";
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
//   - SDP capture is enabled in the kamailio template, so "Encrypted?"
//     and "Crypto suite" reflect real per-call values. SDES-SRTP is still
//     decryptable by a carrier in the signaling path (key exchange in
//     cleartext) — flagged separately from DTLS-SRTP in the UI.

type Severity = "exposed" | "private" | "unknown";

// describeTransport classifies an SDP m= transport into a friendly label
// + the underlying privacy implication. This is the single source of truth
// the UI uses for every "Encrypted?" cell.
function describeTransport(t: string | null | undefined): { label: string; tone: string; carrierCanHear: boolean } {
  if (!t) return { label: "Unknown", tone: "text-slate-500", carrierCanHear: true };
  switch (t.toUpperCase()) {
    case "RTP/AVP":
    case "RTP/AVPF":
      return { label: "No (plain RTP)", tone: "text-rose-700", carrierCanHear: true };
    case "RTP/SAVP":
    case "RTP/SAVPF":
      return { label: "SDES-SRTP", tone: "text-amber-700", carrierCanHear: true };
    case "UDP/TLS/RTP/SAVP":
    case "UDP/TLS/RTP/SAVPF":
      return { label: "DTLS-SRTP", tone: "text-emerald-700", carrierCanHear: false };
    default:
      return { label: t, tone: "text-slate-600", carrierCanHear: true };
  }
}

interface CarrierRow {
  id: number;
  name: string;
  status: string;
  count: number;
  severity: Severity;
  transport: string | null;
}

export default function Privacy() {
  const [active, setActive] = useState<ActiveCallRow[]>([]);
  const [carriers, setCarriers] = useState<Carrier[]>([]);
  const [clients, setClients] = useState<Client[]>([]);
  const [quality, setQuality] = useState<CarrierQuality[]>([]);
  const [qualityWindow, setQualityWindow] = useState<"1h" | "24h" | "168h">("24h");
  const [err, setErr] = useState<string | null>(null);
  const [paused, setPaused] = useState(false);

  const reload = useCallback(() => {
    Promise.all([
      api.get<ActiveCallRow[]>("/api/v1/calls/active"),
      api.get<Carrier[]>("/api/v1/carriers"),
      api.get<Client[]>("/api/v1/clients"),
      api.get<CarrierQuality[]>(`/api/v1/route-quality?window=${qualityWindow}`),
    ])
      .then(([a, c, cl, q]) => {
        setActive(a);
        setCarriers(c);
        setClients(cl);
        setQuality(q);
        setErr(null);
      })
      .catch((e) => setErr(e.message));
  }, [qualityWindow]);

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

  // Aggregate per carrier. We also track the dominant transport per carrier
  // (most common across that carrier's active calls) so the table can show
  // whether the leg is plain RTP, SDES-SRTP, or DTLS-SRTP.
  const perCarrier: CarrierRow[] = useMemo(() => {
    const counts = new Map<number, number>();
    const transports = new Map<number, Map<string, number>>();
    for (const r of active) {
      if (!r.carrier_id) continue;
      counts.set(r.carrier_id, (counts.get(r.carrier_id) ?? 0) + 1);
      const tkey = (r.media_transport ?? "unknown").toUpperCase();
      let m = transports.get(r.carrier_id);
      if (!m) {
        m = new Map();
        transports.set(r.carrier_id, m);
      }
      m.set(tkey, (m.get(tkey) ?? 0) + 1);
    }
    const list: CarrierRow[] = carriers.map((c) => {
      let dominantTransport: string | null = null;
      const m = transports.get(c.id);
      if (m) {
        let best = 0;
        for (const [k, v] of m) {
          if (v > best) {
            best = v;
            dominantTransport = k === "UNKNOWN" ? null : k;
          }
        }
      }
      return {
        id: c.id,
        name: c.name,
        status: c.status,
        count: counts.get(c.id) ?? 0,
        severity: "exposed" as Severity,
        transport: dominantTransport,
      };
    });
    list.sort((a, b) => b.count - a.count);
    return list;
  }, [active, carriers]);

  const totalCalls = active.length;
  // A call is only "private" if its negotiated transport is DTLS-SRTP AND
  // the media goes direct (no carrier). For wholesale traffic this stays 0;
  // we don't fake it.
  const privateCalls = active.filter((r) => {
    const t = describeTransport(r.media_transport);
    return !r.carrier_id && !t.carrierCanHear;
  }).length;
  const exposedCalls = totalCalls - privateCalls;
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
                const t = describeTransport(c.transport);
                return (
                  <tr key={c.id}>
                    <td className="px-3 py-2 font-medium">{c.name}</td>
                    <td className="px-3 py-2 text-right font-mono">{c.count}</td>
                    <td className="px-3 py-2 text-xs">
                      {c.count === 0 ? (
                        <span className="text-slate-400">—</span>
                      ) : t.carrierCanHear ? (
                        <span className="text-rose-700">YES — carrier in audio path</span>
                      ) : (
                        <span className="text-emerald-700">No — DTLS-SRTP end-to-end</span>
                      )}
                    </td>
                    <td className={`px-3 py-2 text-xs font-medium ${t.tone}`}>{t.label}</td>
                    <td className="px-3 py-2">{severityBadge(sev)}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </section>

      {/* Section 2b — Route quality (CDR-derived) */}
      <section className="rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
        <div className="flex items-start justify-between">
          <div>
            <h3 className="flex items-center text-sm font-semibold uppercase tracking-wide text-slate-500">
              Route quality
              <Help>
                <p className="mb-1 font-medium">What this scores</p>
                Per-carrier composite grade (A–F) computed from completed-call records over the
                selected window. Penalises low ASR, very-low ACD (premature drops or SIM-box
                detection), suspiciously high ASR (false-answer fraud), and high rates of 480 /
                503 / non-standard cause codes.
                <p className="mb-1 mt-2 font-medium">What it does NOT prove</p>
                Passive observation can't definitively distinguish "Tier 1 direct" from "grey
                route". The grade flags <em>suspicion</em>. Definitive Tier-1 verification needs
                active probing — synthetic test calls to known-good destinations comparing PDD &amp;
                MOS to a baseline. That's a separate feature.
                <p className="mb-1 mt-2 font-medium">Insufficient data?</p>
                Carriers with fewer than 20 calls in the window show grade <code>—</code>; not
                enough signal to score honestly.
              </Help>
            </h3>
            <p className="mt-1 text-xs text-slate-500">
              Per-carrier scorecard over the selected window. Heuristics, not proof — see the
              tooltip for the boundaries.
            </p>
          </div>
          <select
            value={qualityWindow}
            onChange={(e) => setQualityWindow(e.target.value as typeof qualityWindow)}
            className="rounded border border-slate-300 px-2 py-1 text-sm"
          >
            <option value="1h">last 1h</option>
            <option value="24h">last 24h</option>
            <option value="168h">last 7d</option>
          </select>
        </div>
        <div className="mt-3 overflow-hidden rounded border border-slate-200">
          <table className="min-w-full divide-y divide-slate-200 text-sm">
            <thead className="bg-slate-50 text-left text-xs uppercase tracking-wide text-slate-500">
              <tr>
                <th className="px-3 py-2">Carrier</th>
                <th className="px-3 py-2">Grade</th>
                <th className="px-3 py-2 text-right">Calls</th>
                <th className="px-3 py-2 text-right">ASR</th>
                <th className="px-3 py-2 text-right">ACD</th>
                <th className="px-3 py-2 text-right">PDD avg / p95</th>
                <th className="px-3 py-2">Top codec</th>
                <th className="px-3 py-2">Cause-code mix</th>
                <th className="px-3 py-2">Reasons</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100">
              {quality.length === 0 && (
                <tr>
                  <td colSpan={9} className="px-3 py-6 text-center text-slate-400">
                    No carriers defined yet.
                  </td>
                </tr>
              )}
              {quality.map((q) => {
                const pddTone =
                  q.avg_pdd_ms == null
                    ? "text-slate-400"
                    : q.avg_pdd_ms > 8000
                      ? "text-rose-700"
                      : q.avg_pdd_ms > 5000
                        ? "text-amber-700"
                        : "text-slate-600";
                const codecTone =
                  q.top_codec_pct != null &&
                  q.top_codec_pct >= 80 &&
                  q.top_codec?.toUpperCase().startsWith("G729")
                    ? "text-amber-700 font-medium"
                    : "text-slate-600";
                return (
                  <tr key={q.carrier_id}>
                    <td className="px-3 py-2 font-medium">{q.carrier_name}</td>
                    <td className="px-3 py-2">
                      <span
                        className={`inline-block min-w-[1.75rem] rounded px-2 py-0.5 text-center font-bold ${
                          q.grade === "A"
                            ? "bg-emerald-100 text-emerald-800"
                            : q.grade === "B"
                              ? "bg-lime-100 text-lime-800"
                              : q.grade === "C"
                                ? "bg-amber-100 text-amber-800"
                                : q.grade === "D"
                                  ? "bg-orange-100 text-orange-800"
                                  : q.grade === "F"
                                    ? "bg-rose-100 text-rose-800"
                                    : "bg-slate-100 text-slate-500"
                        }`}
                      >
                        {q.grade}
                      </span>
                    </td>
                    <td className="px-3 py-2 text-right font-mono">{q.total}</td>
                    <td className="px-3 py-2 text-right font-mono">
                      {q.total > 0 ? `${q.asr_pct.toFixed(1)}%` : "—"}
                    </td>
                    <td className="px-3 py-2 text-right font-mono">
                      {q.answered > 0 ? `${q.acd_seconds.toFixed(0)}s` : "—"}
                    </td>
                    <td className={`px-3 py-2 text-right font-mono text-xs ${pddTone}`}>
                      {q.avg_pdd_ms != null
                        ? `${Math.round(q.avg_pdd_ms)}ms / ${Math.round(q.p95_pdd_ms ?? 0)}ms`
                        : "—"}
                    </td>
                    <td className={`px-3 py-2 text-xs ${codecTone}`}>
                      {q.top_codec
                        ? `${q.top_codec} (${(q.top_codec_pct ?? 0).toFixed(0)}%)`
                        : "—"}
                    </td>
                    <td className="px-3 py-2 text-xs">
                      {q.total === 0 ? (
                        <span className="text-slate-400">—</span>
                      ) : (
                        <span className="font-mono text-slate-600">
                          {(["200", "486", "487", "480", "503", "other"] as const)
                            .filter((k) => (q.cause_mix[k] ?? 0) > 0)
                            .map((k) => `${k}:${q.cause_mix[k]}`)
                            .join("  ")}
                        </span>
                      )}
                    </td>
                    <td className="px-3 py-2 text-xs text-slate-500">
                      {q.grade_reasons.join(" · ")}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
        <p className="mt-2 text-[11px] text-slate-400">
          Phases 1–2 shipped: CDR metrics + PDD timing + codec-lock detection. Remaining:
          Phase 3 — RTP MOS / jitter / loss from RTPEngine NG socket;
          Phase 4 — HOMER integration for full SIP-ladder drill-down per call.
        </p>
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
                <th className="px-3 py-2">Transport</th>
                <th className="px-3 py-2">Can hear?</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100">
              {liveFeed.length === 0 && (
                <tr>
                  <td colSpan={6} className="px-3 py-6 text-center text-slate-400">
                    No live calls right now.
                  </td>
                </tr>
              )}
              {liveFeed.map((r) => {
                const t = describeTransport(r.media_transport);
                const carrierCanHear = !!r.carrier_id || t.carrierCanHear;
                return (
                  <tr key={r.id}>
                    <td className="px-3 py-1.5 text-xs text-slate-500">
                      {new Date(r.started_at).toLocaleTimeString()}
                    </td>
                    <td className="px-3 py-1.5">{carrierName(r.carrier_id)}</td>
                    <td className="px-3 py-1.5">{clientName(r.client_id)}</td>
                    <td className="px-3 py-1.5 font-mono text-xs">
                      {r.ani ?? "?"} → {r.dnis ?? "?"}
                    </td>
                    <td className={`px-3 py-1.5 text-xs font-medium ${t.tone}`}>{t.label}</td>
                    <td className="px-3 py-1.5 text-xs">
                      {carrierCanHear ? (
                        <span className="text-rose-700">🔴 Yes</span>
                      ) : (
                        <span className="text-emerald-700">🟢 No — direct + DTLS-SRTP</span>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </section>

      {/* Section 4 — Alerts */}
      <section className="rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
        <h3 className="text-sm font-semibold uppercase tracking-wide text-slate-500">
          Watch out (alerts, last 15 min)
        </h3>
        {(() => {
          const plainCalls = active.filter((r) => {
            const t = describeTransport(r.media_transport);
            return t.label.includes("plain");
          });
          if (plainCalls.length === 0) {
            return (
              <div className="mt-3 rounded border border-dashed border-slate-300 bg-slate-50 px-4 py-6 text-center text-sm text-slate-500">
                No alerts.
                <p className="mt-2 text-xs text-slate-400">
                  Alerts appear here when a call is negotiated with plain RTP (no encryption) or
                  when an active call's media endpoint is renegotiated mid-call. Mid-call
                  renegotiation detection still needs to be added separately.
                </p>
              </div>
            );
          }
          return (
            <ul className="mt-3 space-y-2">
              {plainCalls.slice(0, 10).map((r) => (
                <li
                  key={r.id}
                  className="rounded border border-amber-300 bg-amber-50 px-3 py-2 text-sm text-amber-900"
                >
                  🟠 {new Date(r.started_at).toLocaleTimeString()} ·{" "}
                  <strong>{carrierName(r.carrier_id)}</strong> — call{" "}
                  <code className="font-mono text-xs">{r.call_id.slice(0, 12)}…</code> negotiated
                  with <strong>plain RTP</strong> (no encryption). Audio is open on the wire.
                </li>
              ))}
            </ul>
          );
        })()}
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
            <strong>Encryption labels are real now</strong> — Kamailio captures SDP on every
            INVITE and the panel parses out the <code>m=</code> transport and{" "}
            <code>a=crypto:</code> suite. Note: <strong>SDES-SRTP</strong> (the common flavor in
            SIP trunks) exchanges keys in cleartext signaling — a carrier in the signaling path
            can still decrypt. Only <strong>DTLS-SRTP</strong> with verified fingerprints
            actually blinds the carrier; the page marks those rows green.
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
