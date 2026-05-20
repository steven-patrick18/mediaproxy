import { useEffect, useState } from "react";
import { useParams, Link } from "react-router-dom";
import {
  api,
  type ClientDetail,
  type DialerIP,
  type SignalingIP,
} from "../api";

export default function ClientDetailPage() {
  const { id } = useParams<{ id: string }>();
  const clientID = Number(id);
  const [client, setClient] = useState<ClientDetail | null>(null);
  const [dialers, setDialers] = useState<DialerIP[]>([]);
  const [sigs, setSigs] = useState<SignalingIP[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [newDialer, setNewDialer] = useState("");
  const [busy, setBusy] = useState(false);

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
      })
      .catch((e) => setErr(e.message));
  }
  useEffect(reload, [clientID]);

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
