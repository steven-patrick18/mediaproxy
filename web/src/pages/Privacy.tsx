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
  mediaProxied: number; // calls whose audio is rewritten through our MediaNode
  reinvites: number; // calls with at least one mid-call SDP change
}

// MediaNode IP prefixes — calls whose media_ip falls inside these subnets
// are "audio-proxied" (the carrier sees a MediaNode IP, not the dialer's
// real IP). Hardcoded heuristic for the current cluster; future work:
// derive from the /api/v1/nodes response so adding a new media node
// doesn't need a UI change.
const MEDIA_NODE_PREFIXES = ["67.215.233."];

function isMediaProxied(mediaIP?: string | null): boolean {
  if (!mediaIP) return false;
  return MEDIA_NODE_PREFIXES.some((p) => mediaIP.startsWith(p));
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

  // Aggregate per carrier: total calls, how many have audio rewritten
  // through our MediaNode (privacy preserved), how many had a mid-call
  // re-INVITE (possible bridge/fork). Replaced the old "transport" column
  // because every wholesale carrier is plain-RTP — that column had a
  // 100% noise rate and gave no real signal.
  const perCarrier: CarrierRow[] = useMemo(() => {
    const tally = new Map<number, { count: number; proxied: number; reinvites: number }>();
    for (const r of active) {
      if (!r.carrier_id) continue;
      const cur = tally.get(r.carrier_id) ?? { count: 0, proxied: 0, reinvites: 0 };
      cur.count++;
      if (isMediaProxied(r.media_ip)) cur.proxied++;
      if ((r.reinvite_count ?? 0) > 0) cur.reinvites++;
      tally.set(r.carrier_id, cur);
    }
    const list: CarrierRow[] = carriers.map((c) => {
      const t = tally.get(c.id) ?? { count: 0, proxied: 0, reinvites: 0 };
      return {
        id: c.id,
        name: c.name,
        status: c.status,
        count: t.count,
        severity: "exposed" as Severity,
        mediaProxied: t.proxied,
        reinvites: t.reinvites,
      };
    });
    list.sort((a, b) => b.count - a.count);
    return list;
  }, [active, carriers]);

  const totalCalls = active.length;
  // "Audio behind MediaNode" = the privacy guarantee actually offered by
  // this product. If the call's media_ip is one of our MediaNode pool IPs,
  // the carrier sees that IP — not the dialer's real address.
  const proxiedCalls = active.filter((r) => isMediaProxied(r.media_ip)).length;
  const proxiedPct = totalCalls > 0 ? Math.round((proxiedCalls / totalCalls) * 100) : 0;
  const leakingCalls = totalCalls - proxiedCalls;

  // Mid-call SDP change events — the strongest passively-observable signal
  // of a third-party bridge / fork / NAT roam. Count of currently-active
  // calls that have had at least one re-INVITE after the initial offer.
  const reinviteCalls = active.filter((r) => (r.reinvite_count ?? 0) > 0).length;

  const overallStatus =
    totalCalls === 0
      ? { dot: "⚪", label: "No active calls", tone: "text-slate-500" }
      : leakingCalls > 0
        ? {
            dot: "🔴",
            label: `${leakingCalls} call(s) bypassing MediaNode — dialer IP exposed`,
            tone: "text-rose-700",
          }
        : reinviteCalls > 0
          ? {
              dot: "🟠",
              label: `${reinviteCalls} call(s) renegotiated mid-stream`,
              tone: "text-amber-700",
            }
          : {
              dot: "🟢",
              label: "All active calls proxied through MediaNode, no mid-call changes",
              tone: "text-emerald-700",
            };

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
        <div className="mt-3 grid grid-cols-1 gap-4 sm:grid-cols-4">
          <div>
            <div className="text-xs uppercase tracking-wide text-slate-500">Active calls now</div>
            <div className="mt-1 text-3xl font-semibold text-slate-800">{totalCalls}</div>
          </div>
          <div>
            <div className="flex items-center gap-1 text-xs uppercase tracking-wide text-slate-500">
              Audio via MediaNode
              <Help>
                Calls whose RTP is rewritten through our MediaNode pool. The carrier sees a
                MediaNode IP, never the dialer's real address. This is the privacy guarantee the
                product actually delivers. Should be 100% if rotation + rtpengine are healthy.
              </Help>
            </div>
            <div className="mt-1 flex items-baseline gap-2">
              <span
                className={`text-3xl font-semibold ${proxiedPct >= 99 ? "text-emerald-700" : proxiedPct >= 80 ? "text-amber-700" : "text-rose-700"}`}
              >
                {proxiedCalls}
              </span>
              <span className="text-sm text-slate-500">
                / {totalCalls} ({proxiedPct}%)
              </span>
            </div>
          </div>
          <div>
            <div className="flex items-center gap-1 text-xs uppercase tracking-wide text-slate-500">
              Dialer IP leaked
              <Help>
                Calls whose media_ip is NOT a MediaNode IP — usually means rtpengine wasn't
                invoked and the SDP still carried the dialer's real audio endpoint. Carrier sees
                the dialer directly for these. Should be 0.
              </Help>
            </div>
            <div className="mt-1 flex items-baseline gap-2">
              <span
                className={`text-3xl font-semibold ${leakingCalls === 0 ? "text-emerald-700" : "text-rose-700"}`}
              >
                {leakingCalls}
              </span>
            </div>
          </div>
          <div>
            <div className="flex items-center gap-1 text-xs uppercase tracking-wide text-slate-500">
              Mid-call renegotiations
              <Help>
                Calls that received a mid-stream re-INVITE (SDP changed during the call). Strong
                passively-observable signal of a third-party bridge joining, a carrier-side fork,
                or a NAT roam. Investigate by opening the SIP ladder from CDRs.
              </Help>
            </div>
            <div className="mt-1 flex items-baseline gap-2">
              <span
                className={`text-3xl font-semibold ${reinviteCalls === 0 ? "text-slate-700" : "text-amber-700"}`}
              >
                {reinviteCalls}
              </span>
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
                <th className="px-3 py-2">
                  Audio via MediaNode
                  <Help>
                    Fraction of this carrier's active calls whose RTP is rewritten by our
                    MediaNode (dialer's real IP hidden). 100% = privacy guarantee holding.
                  </Help>
                </th>
                <th className="px-3 py-2">
                  Mid-call changes
                  <Help>
                    Number of currently-active calls with the carrier that received a re-INVITE
                    (SDP renegotiated mid-stream). Possible bridge / fork / NAT roam.
                  </Help>
                </th>
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
                const proxyPct =
                  c.count > 0 ? Math.round((c.mediaProxied / c.count) * 100) : 0;
                const proxyTone =
                  c.count === 0
                    ? "text-slate-400"
                    : proxyPct === 100
                      ? "text-emerald-700"
                      : proxyPct >= 80
                        ? "text-amber-700"
                        : "text-rose-700";
                const reinviteTone =
                  c.reinvites === 0
                    ? "text-slate-500"
                    : c.reinvites > 0 && c.count > 0 && c.reinvites / c.count > 0.05
                      ? "text-rose-700 font-medium"
                      : "text-amber-700";
                return (
                  <tr key={c.id}>
                    <td className="px-3 py-2 font-medium">{c.name}</td>
                    <td className="px-3 py-2 text-right font-mono">{c.count}</td>
                    <td className={`px-3 py-2 text-xs ${proxyTone}`}>
                      {c.count === 0 ? (
                        <span>—</span>
                      ) : (
                        <span>
                          {c.mediaProxied} / {c.count} ({proxyPct}%)
                        </span>
                      )}
                    </td>
                    <td className={`px-3 py-2 text-xs ${reinviteTone}`}>
                      {c.count === 0 ? <span className="text-slate-400">—</span> : c.reinvites}
                    </td>
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
                <th className="px-3 py-2">
                  Tier
                  <Help>
                    Route class inference (A=Tier-1 / direct, B=Tier-2, C=Tier-3 grey). Heuristic
                    from PDD, codec preservation, MOS, ASR/ACD shape, and cause-code purity.
                    Distinct from Grade — a Tier-1 route can have a bad day (Grade C) and a grey
                    route can score Grade A statistically. Hover/expand the reasons to see what
                    drove the call.
                  </Help>
                </th>
                <th className="px-3 py-2">Grade</th>
                <th className="px-3 py-2 text-right">Calls</th>
                <th className="px-3 py-2 text-right">ASR</th>
                <th className="px-3 py-2 text-right">ACD</th>
                <th className="px-3 py-2 text-right">PDD avg / p95</th>
                <th className="px-3 py-2">Top codec</th>
                <th className="px-3 py-2 text-right">MOS / loss</th>
                <th className="px-3 py-2">Cause-code mix</th>
                <th className="px-3 py-2">Reasons</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100">
              {quality.length === 0 && (
                <tr>
                  <td colSpan={11} className="px-3 py-6 text-center text-slate-400">
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
                    <td className="px-3 py-2" title={q.tier_reasons.join(" · ")}>
                      <span
                        className={`inline-block min-w-[1.75rem] rounded px-2 py-0.5 text-center font-bold ${
                          q.tier === "A"
                            ? "bg-emerald-100 text-emerald-800"
                            : q.tier === "B"
                              ? "bg-amber-100 text-amber-800"
                              : q.tier === "C"
                                ? "bg-rose-100 text-rose-800"
                                : "bg-slate-100 text-slate-500"
                        }`}
                      >
                        {q.tier}
                      </span>
                    </td>
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
                    <td className="px-3 py-2 text-right font-mono text-xs">
                      {q.avg_mos != null && q.rtp_samples >= 5 ? (
                        <span
                          className={
                            q.avg_mos < 3.0
                              ? "text-rose-700 font-medium"
                              : q.avg_mos < 3.5
                                ? "text-amber-700"
                                : q.avg_mos >= 4.0
                                  ? "text-emerald-700"
                                  : "text-slate-600"
                          }
                        >
                          {q.avg_mos.toFixed(2)} / {(q.avg_loss_pct ?? 0).toFixed(1)}%
                        </span>
                      ) : (
                        <span className="text-slate-400">—</span>
                      )}
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
          All 4 phases of route-quality monitoring shipped:
          Phase 1 CDR metrics, Phase 2 PDD &amp; codec-lock detection, Phase 3 RTP MOS / jitter
          / loss from RTPEngine NG socket, Phase 4 HOMER SIP-ladder drill-down (per-CDR "SIP"
          button on the CDRs page).
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
                <th className="px-3 py-2">Media via</th>
                <th className="px-3 py-2">Mid-call</th>
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
                const proxied = isMediaProxied(r.media_ip);
                const ri = r.reinvite_count ?? 0;
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
                    <td
                      className={`px-3 py-1.5 text-xs font-mono ${proxied ? "text-emerald-700" : "text-rose-700"}`}
                      title={proxied ? "Audio proxied — carrier sees MediaNode IP" : "Audio not proxied — carrier sees dialer IP"}
                    >
                      {r.media_ip ?? "—"}
                    </td>
                    <td className="px-3 py-1.5 text-xs">
                      {ri === 0 ? (
                        <span className="text-slate-400">—</span>
                      ) : (
                        <span className="text-amber-700">🟠 {ri}×</span>
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
          Watch out
        </h3>
        {(() => {
          const reinviteCalls = active.filter((r) => r.reinvite_count > 0);
          const plainCalls = active.filter((r) => {
            const t = describeTransport(r.media_transport);
            return t.label.includes("plain");
          });
          if (reinviteCalls.length === 0 && plainCalls.length === 0) {
            return (
              <div className="mt-3 rounded border border-dashed border-slate-300 bg-slate-50 px-4 py-6 text-center text-sm text-slate-500">
                No alerts.
                <p className="mt-2 text-xs text-slate-400">
                  Alerts appear here when (a) a call is negotiated with plain RTP, or (b) an
                  active call gets a mid-call re-INVITE — the most-detectable signal of a
                  third-party bridge, fork, or NAT roam.
                </p>
              </div>
            );
          }
          return (
            <ul className="mt-3 space-y-2">
              {reinviteCalls.slice(0, 10).map((r) => (
                <li
                  key={`ri-${r.id}`}
                  className="rounded border border-rose-300 bg-rose-50 px-3 py-2 text-sm text-rose-900"
                >
                  🔴 Mid-call renegotiation ({r.reinvite_count}×) ·{" "}
                  {r.last_reinvite_at && (
                    <>{new Date(r.last_reinvite_at).toLocaleTimeString()} · </>
                  )}
                  <strong>{carrierName(r.carrier_id)}</strong> · call{" "}
                  <code className="font-mono text-xs">{r.call_id.slice(0, 12)}…</code> · endpoint
                  was <code className="font-mono">{r.media_endpoint_ip ?? "?"}</code>
                  {r.last_reinvite_endpoint && r.last_reinvite_endpoint !== r.media_endpoint_ip && (
                    <>
                      {" "}
                      → now <code className="font-mono">{r.last_reinvite_endpoint}</code>
                    </>
                  )}
                  . Possible third-party bridge, fork, or NAT roam — investigate via HOMER SIP
                  ladder.
                </li>
              ))}
              {plainCalls.slice(0, 10).map((r) => (
                <li
                  key={`pl-${r.id}`}
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
        <h3 className="text-sm font-semibold uppercase tracking-wide text-slate-500">What this page measures</h3>
        <ul className="mt-2 space-y-2 text-sm text-slate-600">
          <li>
            <strong>Audio via MediaNode</strong> is the only privacy guarantee this product
            actually delivers against the carrier. When the call's media RTP is rewritten by our
            rtpengine, the carrier sees a MediaNode pool IP and cannot trivially correlate the
            audio with the dialer's real address. Should be 100%.
          </li>
          <li>
            <strong>Dialer IP leaked</strong> = the call's audio never made it through our
            MediaNode — usually means rtpengine_offer didn't fire (config drift, NG socket
            unreachable). The carrier sees the dialer's actual public IP in the SDP. This is the
            most actionable privacy alarm on the page.
          </li>
          <li>
            <strong>Mid-call renegotiations</strong> count re-INVITEs that arrived after the
            initial offer/answer was complete. The strongest passively-observable signal of a
            third-party joining (conference bridge, eavesdrop tap), a carrier-side fork to a
            recorder, or a NAT roam. Investigate per-call via the SIP ladder on CDRs.
          </li>
          <li>
            <strong>What this cannot prove:</strong> server-side recording at the carrier (e.g.
            sipREC mirror, lawful intercept) is invisible from outside their network. Encryption
            doesn't help here either — wholesale SIP is plain RTP industry-wide; SRTP isn't
            supported on the carrier interfaces we route through.
          </li>
        </ul>
      </section>
    </div>
  );
}
