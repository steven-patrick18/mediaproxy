import { useEffect, useState } from "react";
import { useParams, Link } from "react-router-dom";
import {
  api,
  type ClientDetail,
  type DialerIP,
  type SignalingIP,
} from "../api";
import Help from "../components/Help";

// Convert a window in seconds to {value, unit}. Picks the largest unit that
// divides evenly, so 3600s reads as "1 hour", 90s reads as "90 seconds".
function secondsToParts(secs: number): { value: number; unit: "second" | "minute" | "hour" | "day" } {
  if (secs <= 0) return { value: 0, unit: "minute" };
  if (secs % 86400 === 0) return { value: secs / 86400, unit: "day" };
  if (secs % 3600 === 0) return { value: secs / 3600, unit: "hour" };
  if (secs % 60 === 0) return { value: secs / 60, unit: "minute" };
  return { value: secs, unit: "second" };
}
function partsToSeconds(value: number, unit: "second" | "minute" | "hour" | "day"): number {
  switch (unit) {
    case "second": return value;
    case "minute": return value * 60;
    case "hour":   return value * 3600;
    case "day":    return value * 86400;
  }
}

export default function ClientDetailPage() {
  const { id } = useParams<{ id: string }>();
  const clientID = Number(id);
  const [client, setClient] = useState<ClientDetail | null>(null);
  const [dialers, setDialers] = useState<DialerIP[]>([]);
  const [sigs, setSigs] = useState<SignalingIP[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [newDialer, setNewDialer] = useState("");
  const [busy, setBusy] = useState(false);

  // Local edit state for the rate-limit form. Stays in sync with the loaded
  // client until the user edits it; then it diverges until Save is clicked.
  const [rlMax, setRlMax] = useState(0);
  const [rlWindow, setRlWindow] = useState(0);
  const [rlUnit, setRlUnit] = useState<"second" | "minute" | "hour" | "day">("minute");
  const [rlSaved, setRlSaved] = useState<string | null>(null);

  function reload() {
    Promise.all([
      api.get<ClientDetail>(`/api/v1/clients/${clientID}`),
      api.get<DialerIP[]>(`/api/v1/clients/${clientID}/dialer-ips`),
      api.get<SignalingIP[]>(`/api/v1/signaling-ips`),
    ])
      .then(([c, d, s]) => {
        setClient(c);
        setDialers(d);
        setSigs(s);
        const parts = secondsToParts(c.rate_limit_window_seconds);
        setRlMax(c.max_attempts_per_lead);
        setRlWindow(parts.value);
        setRlUnit(parts.unit);
      })
      .catch((e) => setErr(e.message));
  }
  useEffect(reload, [clientID]);

  async function saveRateLimit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setErr(null);
    setRlSaved(null);
    try {
      // 0 max OR 0 window both mean "disabled"; persist a clean (0, 0) pair
      // so the backend never sees a nonsensical "5 per 0 seconds".
      const maxAttempts = Math.max(0, Math.floor(rlMax));
      const windowSecs = maxAttempts === 0 ? 0 : Math.max(0, partsToSeconds(Math.floor(rlWindow), rlUnit));
      await api.patch(`/api/v1/clients/${clientID}`, {
        max_attempts_per_lead: maxAttempts,
        rate_limit_window_seconds: windowSecs,
      });
      setRlSaved("Saved.");
      setTimeout(() => setRlSaved(null), 2500);
      reload();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "save failed");
    } finally {
      setBusy(false);
    }
  }

  async function assignSig(sigID: number) {
    setBusy(true);
    setErr(null);
    try {
      await api.post(`/api/v1/clients/${clientID}/signaling-ip`, {
        signaling_ip_id: sigID,
      });
      reload();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "assign failed");
    } finally {
      setBusy(false);
    }
  }
  async function unassignSig() {
    setBusy(true);
    setErr(null);
    try {
      await api.del(`/api/v1/clients/${clientID}/signaling-ip`);
      reload();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "unassign failed");
    } finally {
      setBusy(false);
    }
  }
  async function addDialer(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setErr(null);
    try {
      await api.post(`/api/v1/clients/${clientID}/dialer-ips`, {
        ip_address: newDialer,
      });
      setNewDialer("");
      reload();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "add failed");
    } finally {
      setBusy(false);
    }
  }
  async function removeDialer(dialerID: number) {
    if (!confirm("Remove this dialer IP?")) return;
    setBusy(true);
    setErr(null);
    try {
      await api.del(`/api/v1/clients/${clientID}/dialer-ips/${dialerID}`);
      reload();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "remove failed");
    } finally {
      setBusy(false);
    }
  }

  if (!client) {
    return <div className="text-slate-500">Loading…</div>;
  }

  const availableSigs = sigs.filter(
    (s) => s.status === "available" && !s.assigned_client_id,
  );

  return (
    <div className="space-y-6">
      <div>
        <Link to="/clients" className="text-xs text-slate-500 hover:underline">
          ← back to clients
        </Link>
        <h2 className="mt-1 text-2xl font-semibold tracking-tight">
          {client.name}
        </h2>
        <p className="text-sm text-slate-500">
          client #{client.id} · reseller #{client.reseller_id} · {client.status}
        </p>
      </div>

      {err && (
        <div className="rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
          {err}
        </div>
      )}

      <section className="rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
        <h3 className="text-sm font-semibold uppercase tracking-wide text-slate-500">
          Signaling IP
        </h3>
        <p className="mt-1 text-xs text-slate-500">
          Carrier-facing SIP source IP. One per client. Selected from the pool of available
          signaling IPs.
        </p>
        <div className="mt-3 flex items-center gap-3">
          {client.signaling_ip ? (
            <>
              <code className="rounded bg-slate-100 px-2 py-1 font-mono text-sm">
                {client.signaling_ip}
              </code>
              <button
                onClick={unassignSig}
                disabled={busy}
                className="text-xs text-red-600 hover:underline disabled:opacity-60"
              >
                Unassign
              </button>
            </>
          ) : (
            <>
              <span className="text-sm text-slate-500">none assigned</span>
              {availableSigs.length === 0 ? (
                <span className="text-xs text-slate-400">
                  (no available signaling IPs in the pool)
                </span>
              ) : (
                <select
                  defaultValue=""
                  disabled={busy}
                  onChange={(e) => e.target.value && assignSig(Number(e.target.value))}
                  className="rounded border border-slate-300 px-2 py-1 text-sm"
                >
                  <option value="" disabled>
                    assign…
                  </option>
                  {availableSigs.map((s) => (
                    <option key={s.id} value={s.id}>
                      {s.ip_address}
                    </option>
                  ))}
                </select>
              )}
            </>
          )}
        </div>
      </section>

      <section className="rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
        <h3 className="flex items-center text-sm font-semibold uppercase tracking-wide text-slate-500">
          Per-lead call rate limit
          <Help>
            <p className="mb-1 font-medium">What this does</p>
            Caps how many times this client's dialer may attempt the same
            destination number (DNIS / "lead") within a sliding window. When
            the cap is hit, the panel replies <code className="font-mono">486 Busy Here</code>
            to Kamailio — Vicidial sees a real busy and won't immediately retry.
            <p className="mb-1 mt-2 font-medium">Why it matters</p>
            Dialers that retry unanswered leads aggressively look like spam to
            carriers and quickly burn the reputation of your media IPs. A
            conservative cap (e.g. <strong>1 attempt per 5 minutes</strong>)
            protects both.
            <p className="mb-1 mt-2 font-medium">How to disable</p>
            Set <strong>Max attempts</strong> to <code className="font-mono">0</code>.
            The window value is ignored when max is 0.
          </Help>
        </h3>
        <p className="mt-1 text-xs text-slate-500">
          Throttles repeated dials of the same destination number from this client.
          Counter is per (client, DNIS) and resets after the window elapses.
        </p>

        <form onSubmit={saveRateLimit} className="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-[1fr_2fr_auto] sm:items-end">
          <div>
            <label className="block text-xs font-medium text-slate-500">
              Max attempts per lead
              <Help>
                Number of times the same DNIS can be dialed within the window
                below. <code className="font-mono">0</code> = unlimited (rate
                limiting disabled for this client).
              </Help>
            </label>
            <input
              type="number"
              min={0}
              value={rlMax}
              onChange={(e) => setRlMax(Number(e.target.value))}
              className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm"
            />
          </div>
          <div>
            <label className="block text-xs font-medium text-slate-500">
              Window
              <Help>
                How far back the counter looks. Sliding (not aligned to clock
                minutes/hours). Common picks: <strong>1 per 5 min</strong> for
                aggressive dialers, <strong>3 per hour</strong> for normal
                outbound, <strong>5 per day</strong> for warm-list nurture.
              </Help>
            </label>
            <div className="mt-1 flex gap-2">
              <input
                type="number"
                min={0}
                value={rlWindow}
                onChange={(e) => setRlWindow(Number(e.target.value))}
                disabled={rlMax === 0}
                className="w-24 rounded border border-slate-300 px-3 py-2 text-sm disabled:bg-slate-100 disabled:text-slate-400"
              />
              <select
                value={rlUnit}
                onChange={(e) => setRlUnit(e.target.value as typeof rlUnit)}
                disabled={rlMax === 0}
                className="flex-1 rounded border border-slate-300 px-3 py-2 text-sm disabled:bg-slate-100 disabled:text-slate-400"
              >
                <option value="second">seconds</option>
                <option value="minute">minutes</option>
                <option value="hour">hours</option>
                <option value="day">days</option>
              </select>
            </div>
          </div>
          <div className="flex items-center gap-3">
            <button
              type="submit"
              disabled={busy}
              className="rounded-md bg-brand-600 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-brand-700 disabled:opacity-60"
            >
              Save
            </button>
            {rlSaved && <span className="text-xs text-emerald-600">{rlSaved}</span>}
          </div>
        </form>

        <p className="mt-3 text-xs text-slate-400">
          {client.max_attempts_per_lead === 0 || client.rate_limit_window_seconds === 0 ? (
            <>Currently: <strong className="text-slate-600">disabled</strong> — every call passes through.</>
          ) : (
            <>
              Currently enforcing: <strong className="text-slate-700">
                {client.max_attempts_per_lead} attempt{client.max_attempts_per_lead === 1 ? "" : "s"}
              </strong> per same DNIS per <strong className="text-slate-700">
                {(() => {
                  const p = secondsToParts(client.rate_limit_window_seconds);
                  return `${p.value} ${p.unit}${p.value === 1 ? "" : "s"}`;
                })()}
              </strong>.
            </>
          )}
        </p>
      </section>

      <section className="rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
        <h3 className="text-sm font-semibold uppercase tracking-wide text-slate-500">
          Dialer source IPs
        </h3>
        <p className="mt-1 text-xs text-slate-500">
          IPs the client's dialer sends from. Used to whitelist the client and resolve which
          signaling IP to use.
        </p>

        <form onSubmit={addDialer} className="mt-3 flex gap-2">
          <input
            required
            value={newDialer}
            onChange={(e) => setNewDialer(e.target.value)}
            placeholder="1.2.3.4"
            className="flex-1 rounded border border-slate-300 px-3 py-2 font-mono text-sm"
          />
          <button
            type="submit"
            disabled={busy}
            className="rounded-md bg-brand-600 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-brand-700 disabled:opacity-60"
          >
            Add
          </button>
        </form>

        <ul className="mt-3 divide-y divide-slate-100 rounded border border-slate-200">
          {dialers.length === 0 && (
            <li className="px-3 py-4 text-center text-sm text-slate-400">
              No dialer IPs yet.
            </li>
          )}
          {dialers.map((d) => (
            <li key={d.id} className="flex items-center justify-between px-3 py-2">
              <code className="font-mono text-sm">{d.ip_address}</code>
              <button
                onClick={() => removeDialer(d.id)}
                className="text-xs text-red-600 hover:underline"
              >
                Remove
              </button>
            </li>
          ))}
        </ul>
      </section>
    </div>
  );
}
