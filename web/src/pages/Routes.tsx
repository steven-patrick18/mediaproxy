import { useEffect, useState } from "react";
import { api, type Carrier, type Client, type Route } from "../api";
import Help from "../components/Help";

export default function Routes() {
  const [rows, setRows] = useState<Route[]>([]);
  const [clients, setClients] = useState<Client[]>([]);
  const [carriers, setCarriers] = useState<Carrier[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [showForm, setShowForm] = useState(false);
  const [form, setForm] = useState({
    client_id: 0,
    carrier_id: 0,
    match_prefix: "",
    priority: 100,
  });
  const [busy, setBusy] = useState(false);

  function reload() {
    Promise.all([
      api.get<Route[]>("/api/v1/routes"),
      api.get<Client[]>("/api/v1/clients"),
      api.get<Carrier[]>("/api/v1/carriers"),
    ])
      .then(([r, c, ca]) => {
        setRows(r);
        setClients(c);
        setCarriers(ca);
      })
      .catch((e) => setErr(e.message));
  }
  useEffect(reload, []);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setErr(null);
    setBusy(true);
    try {
      await api.post("/api/v1/routes", {
        client_id: form.client_id,
        carrier_id: form.carrier_id,
        match_prefix: form.match_prefix || undefined,
        priority: form.priority,
      });
      setForm({ client_id: 0, carrier_id: 0, match_prefix: "", priority: 100 });
      setShowForm(false);
      reload();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "create failed");
    } finally {
      setBusy(false);
    }
  }
  async function del(id: number) {
    if (!confirm("Delete route?")) return;
    try {
      await api.del<void>(`/api/v1/routes/${id}`);
      reload();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "delete failed");
    }
  }
  const clientName = (id: number) => clients.find((c) => c.id === id)?.name ?? `#${id}`;
  const carrierName = (id: number) => carriers.find((c) => c.id === id)?.name ?? `#${id}`;

  return (
    <div className="space-y-4">
      <header className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-semibold tracking-tight">Routes</h2>
          <p className="text-sm text-slate-500">
            Per-client routing decisions: destination prefix → carrier.
          </p>
        </div>
        <button
          onClick={() => setShowForm((v) => !v)}
          disabled={clients.length === 0 || carriers.length === 0}
          className="rounded-md bg-brand-600 px-3 py-2 text-sm font-medium text-white shadow-sm hover:bg-brand-700 disabled:opacity-60"
        >
          {showForm ? "Cancel" : "Add route"}
        </button>
      </header>

      {(clients.length === 0 || carriers.length === 0) && (
        <div className="rounded border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-800">
          Need at least one client and one carrier before creating routes.
        </div>
      )}

      {err && (
        <div className="rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{err}</div>
      )}

      {showForm && (
        <form onSubmit={submit} className="grid grid-cols-1 gap-3 rounded-lg border border-slate-200 bg-white p-4 shadow-sm sm:grid-cols-4">
          <div>
            <label className="block text-xs font-medium uppercase tracking-wide text-slate-500">
              Client
              <Help>
                Which tenant this route belongs to. Calls coming from this client's
                whitelisted dialer IPs will be matched against this route.
              </Help>
            </label>
            <select required value={form.client_id} onChange={(e) => setForm({ ...form, client_id: Number(e.target.value) })} className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm">
              <option value={0}>— select —</option>
              {clients.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
            </select>
          </div>
          <div>
            <label className="block text-xs font-medium uppercase tracking-wide text-slate-500">
              Carrier
              <Help>
                Upstream SIP destination calls matching this route are sent to.
                Configure carriers under <strong>Routing → Carriers</strong> first.
                Each carrier maps to one or more media nodes that will carry the RTP.
              </Help>
            </label>
            <select required value={form.carrier_id} onChange={(e) => setForm({ ...form, carrier_id: Number(e.target.value) })} className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm">
              <option value={0}>— select —</option>
              {carriers.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
            </select>
          </div>
          <div>
            <label className="block text-xs font-medium uppercase tracking-wide text-slate-500">
              Prefix (E.164)
              <Help>
                <p className="mb-1 font-medium">Longest-prefix match on the dialed number.</p>
                Leave blank to match every number this client dials. Otherwise enter the
                leading digits in E.164 (no <code>+</code>):
                <ul className="ml-4 mt-1 list-disc">
                  <li><code>1</code> — all US/CA numbers</li>
                  <li><code>1212</code> — only New York</li>
                  <li><code>44</code> — all UK</li>
                  <li><code>800</code> — toll-free</li>
                </ul>
                Multiple routes can share a client; the longest matching prefix wins.
              </Help>
            </label>
            <input value={form.match_prefix} onChange={(e) => setForm({ ...form, match_prefix: e.target.value })} placeholder="1, 1212, 44" className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm font-mono" />
          </div>
          <div>
            <label className="block text-xs font-medium uppercase tracking-wide text-slate-500">
              Priority
              <Help>
                <p className="mb-1 font-medium">Lower number = tried first.</p>
                When multiple routes match the dialed number for this client, they're
                tried in ascending priority order. If a carrier returns 503/486/no-answer,
                the next priority is tried.
                <div className="mt-2 rounded bg-slate-50 p-2 font-mono text-[11px]">
                  Alfa · 1 → CarrierA · priority 100  ← primary<br/>
                  Alfa · 1 → CarrierB · priority 200  ← failover
                </div>
                Default <code>100</code> is fine for single-carrier setups.
              </Help>
            </label>
            <input type="number" value={form.priority} onChange={(e) => setForm({ ...form, priority: Number(e.target.value) })} className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm" />
          </div>
          <div className="sm:col-span-4">
            <button type="submit" disabled={busy || form.client_id === 0 || form.carrier_id === 0} className="rounded-md bg-brand-600 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-brand-700 disabled:opacity-60">
              {busy ? "Saving…" : "Create"}
            </button>
          </div>
        </form>
      )}

      <div className="overflow-hidden rounded-lg border border-slate-200 bg-white shadow-sm">
        <table className="min-w-full divide-y divide-slate-200 text-sm">
          <thead className="bg-slate-50 text-left text-xs uppercase tracking-wide text-slate-500">
            <tr>
              <th className="px-4 py-2">ID</th>
              <th className="px-4 py-2">Client</th>
              <th className="px-4 py-2">Prefix</th>
              <th className="px-4 py-2">Carrier</th>
              <th className="px-4 py-2">Priority</th>
              <th className="px-4 py-2">Status</th>
              <th className="px-4 py-2" />
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100">
            {rows.length === 0 && (
              <tr><td colSpan={7} className="px-4 py-6 text-center text-slate-400">No routes yet.</td></tr>
            )}
            {rows.map((r) => (
              <tr key={r.id}>
                <td className="px-4 py-2 font-mono text-slate-500">{r.id}</td>
                <td className="px-4 py-2">{clientName(r.client_id)}</td>
                <td className="px-4 py-2 font-mono">{r.match_prefix ?? "*"}</td>
                <td className="px-4 py-2">{carrierName(r.carrier_id)}</td>
                <td className="px-4 py-2">{r.priority}</td>
                <td className="px-4 py-2">{r.status}</td>
                <td className="px-4 py-2 text-right">
                  <button onClick={() => del(r.id)} className="text-xs text-red-600 hover:underline">Delete</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
