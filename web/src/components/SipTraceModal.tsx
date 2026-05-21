import { useEffect, useState } from "react";
import { api } from "../api";

interface SipMessage {
  id: number;
  timestamp_ms: number;
  src_ip: string;
  src_port: number;
  dst_ip: string;
  dst_port: number;
  method: string;
  method_text: string;
  ruri_preview: string;
  color: string;
}
interface SipEndpoint {
  ip: string;
  role: "dialer" | "sip_proxy" | "media" | "carrier" | "unknown";
  name: string;
  emoji: string;
}
interface SipTraceResp {
  call_id: string;
  messages: SipMessage[];
  endpoints: SipEndpoint[];
  from_ms: number;
  to_ms: number;
}

// SipTraceModal — fetches a call's SIP messages from HOMER via the
// /cdrs/:id/sip-trace proxy and renders a compact chronological list
// (sender → receiver, method/code, timestamp delta from first message).
// Inline modal so the operator never leaves the panel UX.
export default function SipTraceModal({
  callID,
  startedAt,
  onClose,
}: {
  callID: string;
  startedAt?: string | null;
  onClose: () => void;
}) {
  const [data, setData] = useState<SipTraceResp | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setErr(null);
    const qs = startedAt ? `?started_at=${new Date(startedAt).getTime()}` : "";
    api
      .get<SipTraceResp>(`/api/v1/cdrs/${encodeURIComponent(callID)}/sip-trace${qs}`)
      .then((r) => {
        if (!cancelled) setData(r);
      })
      .catch((e) => {
        if (!cancelled) setErr(e instanceof Error ? e.message : "load failed");
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [callID, startedAt]);

  // Close on Escape for keyboard ergonomics.
  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", h);
    return () => window.removeEventListener("keydown", h);
  }, [onClose]);

  const messages = data?.messages ?? [];
  const endpoints = data?.endpoints ?? [];
  const t0 = messages.length > 0 ? messages[0].timestamp_ms : 0;

  // Quick lookup by IP for the From/To cells.
  const lookup = new Map<string, SipEndpoint>();
  for (const ep of endpoints) lookup.set(ep.ip, ep);

  function endpointPill(ip: string, port: number) {
    const ep = lookup.get(ip);
    const tone =
      ep?.role === "dialer"
        ? "bg-sky-100 text-sky-800"
        : ep?.role === "sip_proxy"
          ? "bg-violet-100 text-violet-800"
          : ep?.role === "media"
            ? "bg-emerald-100 text-emerald-800"
            : ep?.role === "carrier"
              ? "bg-amber-100 text-amber-800"
              : "bg-slate-100 text-slate-700";
    const label = ep ? `${ep.emoji} ${ep.name}` : ip;
    return (
      <span className={`inline-flex flex-col rounded px-2 py-0.5 text-xs ${tone}`} title={ip}>
        <span className="font-medium">{label}</span>
        <span className="font-mono text-[10px] opacity-70">
          {ip}:{port}
        </span>
      </span>
    );
  }

  function methodTone(m: SipMessage): string {
    // Color the row based on SIP method/code: 2xx green, 3xx blue,
    // 4xx amber, 5xx/6xx red, requests neutral.
    const code = parseInt(m.method, 10);
    if (Number.isFinite(code)) {
      if (code >= 200 && code < 300) return "text-emerald-700";
      if (code >= 300 && code < 400) return "text-blue-700";
      if (code >= 400 && code < 500) return "text-amber-700";
      if (code >= 500) return "text-rose-700";
    }
    if (m.method === "INVITE") return "text-slate-900 font-medium";
    if (m.method === "BYE" || m.method === "CANCEL") return "text-slate-700";
    return "text-slate-600";
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/60 p-4"
      onClick={onClose}
    >
      <div
        className="flex max-h-[90vh] w-full max-w-5xl flex-col overflow-hidden rounded-lg bg-white shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <header className="flex items-center justify-between border-b border-slate-200 px-4 py-3">
          <div>
            <h3 className="text-lg font-semibold tracking-tight">SIP trace</h3>
            <div className="font-mono text-xs text-slate-500">{callID}</div>
          </div>
          <button
            onClick={onClose}
            className="rounded-md border border-slate-300 px-3 py-1 text-sm hover:bg-slate-50"
          >
            Close
          </button>
        </header>

        <div className="flex-1 overflow-auto p-4">
          {loading && (
            <div className="py-10 text-center text-slate-500">Loading SIP messages…</div>
          )}
          {err && (
            <div className="rounded border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">
              {err}
            </div>
          )}
          {!loading && !err && messages.length === 0 && (
            <div className="rounded border border-dashed border-slate-300 bg-slate-50 px-4 py-10 text-center text-sm text-slate-500">
              No SIP messages captured for this call.
              <p className="mt-2 text-xs text-slate-400">
                Either the call predates HOMER capture, or the search window (±15 min around
                start) doesn't cover it. HOMER stores last 30 days by default.
              </p>
            </div>
          )}

          {!loading && !err && messages.length > 0 && (
            <>
              {/* Endpoint legend — labels each IP with its role in the call */}
              <div className="mb-3">
                <div className="mb-1 text-xs uppercase tracking-wide text-slate-500">
                  Who's who in this call
                </div>
                <div className="flex flex-wrap gap-2">
                  {endpoints.map((ep) => (
                    <span key={ep.ip} className="inline-flex flex-col rounded border border-slate-200 bg-white px-2 py-1 text-xs">
                      <span className="font-medium text-slate-800">
                        {ep.emoji} {ep.name}
                      </span>
                      <span className="font-mono text-[10px] text-slate-500">{ep.ip}</span>
                    </span>
                  ))}
                </div>
              </div>

              <table className="min-w-full divide-y divide-slate-200 text-sm">
                <thead className="bg-slate-50 text-left text-xs uppercase tracking-wide text-slate-500">
                  <tr>
                    <th className="px-3 py-2">Δt (ms)</th>
                    <th className="px-3 py-2">From</th>
                    <th className="px-3 py-2"></th>
                    <th className="px-3 py-2">To</th>
                    <th className="px-3 py-2">Method / Code</th>
                    <th className="px-3 py-2">R-URI / status line</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-100">
                  {messages.map((m) => {
                    const dt = m.timestamp_ms - t0;
                    return (
                      <tr key={m.id} className="align-top">
                        <td className="px-3 py-1.5 font-mono text-xs text-slate-500">{dt}</td>
                        <td className="px-3 py-1.5">{endpointPill(m.src_ip, m.src_port)}</td>
                        <td className="px-3 py-1.5 font-mono text-xs text-slate-400">→</td>
                        <td className="px-3 py-1.5">{endpointPill(m.dst_ip, m.dst_port)}</td>
                        <td className={`px-3 py-1.5 text-xs ${methodTone(m)}`}>
                          {m.method_text || m.method}
                        </td>
                        <td className="max-w-[24rem] truncate px-3 py-1.5 font-mono text-xs text-slate-600">
                          {m.ruri_preview}
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>

              <p className="mt-4 text-[11px] text-slate-400">
                Times are relative to the first message. Need the full payload of any message?
                Open HOMER directly (we proxy at <code>/homer/</code>).
              </p>
            </>
          )}
        </div>
      </div>
    </div>
  );
}
