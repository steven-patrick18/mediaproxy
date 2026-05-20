import { useEffect, useState } from "react";
import { api, type Integration, type VerifyResponse } from "../api";
import Modal from "../components/Modal";
import { PencilIcon, PlusIcon, TrashIcon, RefreshIcon } from "../components/Icons";

type ProviderKey = Integration["provider"];

const providerLabel: Record<ProviderKey, string> = {
  signalwire: "SignalWire",
  freeswitch: "FreeSWITCH (self-hosted)",
  twilio: "Twilio",
  other: "Other / custom",
};

// Fields each provider asks for. Secret fields are typed=password so the
// browser doesn't autocomplete them with login creds.
type FieldSpec = { key: string; label: string; secret?: boolean; placeholder?: string; help?: string };
const fieldsByProvider: Record<ProviderKey, FieldSpec[]> = {
  signalwire: [
    { key: "space_url", label: "Space URL", placeholder: "yourspace.signalwire.com",
      help: "Just the host — no https://, no trailing slash." },
    { key: "project_id", label: "Project ID", placeholder: "abcd1234-…" },
    { key: "api_token", label: "API token", secret: true, placeholder: "PT…",
      help: "From SignalWire Dashboard → API → Tokens." },
  ],
  freeswitch: [
    { key: "host", label: "ESL host", placeholder: "10.0.0.10" },
    { key: "esl_port", label: "ESL port", placeholder: "8021" },
    { key: "esl_password", label: "ESL password", secret: true },
  ],
  twilio: [
    { key: "account_sid", label: "Account SID", placeholder: "AC…" },
    { key: "auth_token", label: "Auth token", secret: true },
  ],
  other: [
    { key: "base_url", label: "Base URL", placeholder: "https://api.example.com" },
    { key: "api_key", label: "API key", secret: true },
  ],
};

function StatusBadge({ status }: { status: Integration["status"] }) {
  const cls =
    status === "verified" ? "bg-emerald-100 text-emerald-800"
    : status === "failed" ? "bg-red-100 text-red-800"
    : status === "disabled" ? "bg-slate-200 text-slate-700"
    : "bg-amber-100 text-amber-800";
  return <span className={`rounded px-2 py-0.5 text-xs font-medium ${cls}`}>{status}</span>;
}

export default function Integrations() {
  const [rows, setRows] = useState<Integration[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<Integration | null>(null);
  const [form, setForm] = useState({ name: "", provider: "signalwire" as ProviderKey, config: {} as Record<string, string> });

  const [verifyOpen, setVerifyOpen] = useState<{ id: number; res: VerifyResponse | null } | null>(null);

  function reload() {
    api.get<Integration[]>("/api/v1/integrations").then(setRows).catch((e) => setErr(e.message));
  }
  useEffect(reload, []);

  function openCreate() {
    setForm({ name: "", provider: "signalwire", config: {} });
    setCreating(true);
  }
  function openEdit(it: Integration) {
    const cfg: Record<string, string> = {};
    for (const [k, v] of Object.entries(it.config ?? {})) cfg[k] = String(v ?? "");
    setForm({ name: it.name, provider: it.provider, config: cfg });
    setEditing(it);
  }

  async function save() {
    setErr(null);
    setBusy(true);
    try {
      const payload: Record<string, unknown> = { name: form.name };
      const cfg: Record<string, unknown> = {};
      for (const f of fieldsByProvider[form.provider]) {
        const v = form.config[f.key] ?? "";
        if (v !== "") cfg[f.key] = v;
      }
      if (editing) {
        payload.config = cfg;
        await api.patch(`/api/v1/integrations/${editing.id}`, payload);
      } else {
        payload.provider = form.provider;
        payload.config = cfg;
        await api.post("/api/v1/integrations", payload);
      }
      setCreating(false);
      setEditing(null);
      reload();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "save failed");
    } finally {
      setBusy(false);
    }
  }
  async function del(id: number) {
    if (!confirm("Delete this integration? Carriers pointing at it will be detached.")) return;
    try {
      await api.del<void>(`/api/v1/integrations/${id}`);
      reload();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "delete failed");
    }
  }
  async function verify(id: number) {
    setVerifyOpen({ id, res: null });
    try {
      const res = await api.post<VerifyResponse>(`/api/v1/integrations/${id}/verify`);
      setVerifyOpen({ id, res });
    } catch (e) {
      setVerifyOpen({ id, res: { ok: false, status: "failed", error: e instanceof Error ? e.message : "verify failed" } });
    }
    reload();
  }

  return (
    <div className="space-y-4">
      <header className="flex items-center justify-between">
        <div>
          <h2 className="text-xl font-semibold tracking-tight text-slate-800">Integrations</h2>
          <p className="text-xs text-slate-500">
            External APIs (SignalWire, FreeSWITCH, Twilio, …) — store credentials and verify them.
            Secrets are write-only: once saved they show masked, you can only replace them.
          </p>
        </div>
        <button onClick={openCreate} className="inline-flex items-center gap-1.5 rounded-md bg-brand-600 px-3 py-1.5 text-sm font-medium text-white shadow-sm hover:bg-brand-700">
          <PlusIcon /> Add integration
        </button>
      </header>

      {err && <div className="rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{err}</div>}

      <div className="overflow-hidden rounded-lg border border-slate-200 bg-white shadow-sm">
        <table className="min-w-full divide-y divide-slate-200 text-sm">
          <thead className="bg-slate-50 text-left text-xs uppercase tracking-wide text-slate-500">
            <tr>
              <th className="px-4 py-2">ID</th>
              <th className="px-4 py-2">Name</th>
              <th className="px-4 py-2">Provider</th>
              <th className="px-4 py-2">Status</th>
              <th className="px-4 py-2">Last verified</th>
              <th className="px-4 py-2 text-right">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100">
            {rows.length === 0 && (
              <tr><td colSpan={6} className="px-4 py-6 text-center text-slate-400">No integrations yet.</td></tr>
            )}
            {rows.map((r) => (
              <tr key={r.id} className="hover:bg-slate-50">
                <td className="px-4 py-2 font-mono text-slate-500">{r.id}</td>
                <td className="px-4 py-2 font-medium">{r.name}</td>
                <td className="px-4 py-2">{providerLabel[r.provider]}</td>
                <td className="px-4 py-2"><StatusBadge status={r.status} /></td>
                <td className="px-4 py-2 text-xs text-slate-500">
                  {r.last_verified_at ? new Date(r.last_verified_at).toLocaleString() : "—"}
                  {r.last_error && <div className="text-red-600">{r.last_error}</div>}
                </td>
                <td className="px-4 py-2">
                  <div className="flex justify-end gap-1">
                    <button onClick={() => verify(r.id)} title="Verify credentials" className="rounded p-1 text-slate-500 hover:bg-slate-100 hover:text-emerald-600">
                      <RefreshIcon />
                    </button>
                    <button onClick={() => openEdit(r)} title="Edit" className="rounded p-1 text-slate-500 hover:bg-slate-100 hover:text-brand-600">
                      <PencilIcon />
                    </button>
                    <button onClick={() => del(r.id)} title="Delete" className="rounded p-1 text-slate-500 hover:bg-slate-100 hover:text-red-600">
                      <TrashIcon />
                    </button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <Modal
        open={creating || editing !== null}
        title={editing ? `Edit integration #${editing.id}` : "Add integration"}
        width="max-w-lg"
        onClose={() => { setCreating(false); setEditing(null); }}
        footer={
          <>
            <button onClick={() => { setCreating(false); setEditing(null); }} className="rounded border border-slate-300 px-3 py-1.5 text-sm hover:bg-slate-100">Cancel</button>
            <button onClick={save} disabled={busy || !form.name} className="rounded bg-brand-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-brand-700 disabled:opacity-60">
              {busy ? "Saving…" : "Save"}
            </button>
          </>
        }
      >
        <div className="space-y-3">
          <div>
            <label className="block text-xs font-medium text-slate-500">Name</label>
            <input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="My SignalWire" className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm" />
          </div>
          <div>
            <label className="block text-xs font-medium text-slate-500">Provider</label>
            <select
              value={form.provider}
              disabled={!!editing}
              onChange={(e) => setForm({ ...form, provider: e.target.value as ProviderKey, config: {} })}
              className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm disabled:bg-slate-100"
            >
              {Object.entries(providerLabel).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
            </select>
            {editing && <p className="mt-1 text-xs text-slate-400">Provider can't change after creation.</p>}
          </div>

          <div className="rounded border border-slate-200 bg-slate-50 p-3">
            <h4 className="mb-2 text-sm font-medium text-slate-700">Credentials</h4>
            <div className="space-y-2">
              {fieldsByProvider[form.provider].map((f) => (
                <div key={f.key}>
                  <label className="block text-xs font-medium text-slate-500">{f.label}</label>
                  <input
                    type={f.secret ? "password" : "text"}
                    autoComplete={f.secret ? "new-password" : "off"}
                    value={form.config[f.key] ?? ""}
                    onChange={(e) => setForm({ ...form, config: { ...form.config, [f.key]: e.target.value } })}
                    placeholder={f.placeholder ?? ""}
                    className="mt-1 w-full rounded border border-slate-300 px-3 py-2 font-mono text-sm"
                  />
                  {f.help && <p className="mt-1 text-xs text-slate-500">{f.help}</p>}
                </div>
              ))}
            </div>
            {editing && (
              <p className="mt-2 text-xs text-slate-500">
                Existing secret fields are stored masked. Leave them empty to keep the stored value;
                fill in a new value to replace.
              </p>
            )}
          </div>

          {form.provider === "signalwire" && (
            <div className="rounded border border-blue-200 bg-blue-50 p-3 text-xs text-blue-900">
              After saving, click <strong>Verify</strong> (↻ icon) to call SignalWire's LaML API
              and confirm the credentials work. Verified integrations can be linked from Carriers.
            </div>
          )}
        </div>
      </Modal>

      <Modal
        open={verifyOpen !== null}
        title={verifyOpen ? `Verify integration #${verifyOpen.id}` : ""}
        onClose={() => setVerifyOpen(null)}
        width="max-w-md"
        footer={
          <button onClick={() => setVerifyOpen(null)} className="rounded bg-brand-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-brand-700">
            Close
          </button>
        }
      >
        {!verifyOpen?.res ? (
          <div className="py-6 text-center text-sm text-slate-500">
            <div className="mx-auto mb-3 h-5 w-5 animate-spin rounded-full border-2 border-slate-200 border-t-brand-600" />
            Calling provider API…
          </div>
        ) : verifyOpen.res.ok ? (
          <div className="space-y-2 text-sm">
            <div className="rounded bg-emerald-50 p-3 font-medium text-emerald-800">
              ✓ Credentials are valid (HTTP {verifyOpen.res.status_code})
            </div>
            <p className="text-xs text-slate-500">
              The provider returned a successful response. Status is now <strong>verified</strong>.
            </p>
          </div>
        ) : (
          <div className="space-y-2 text-sm">
            <div className="rounded bg-red-50 p-3 font-medium text-red-800">
              ✗ Verification failed{verifyOpen.res.status_code ? ` (HTTP ${verifyOpen.res.status_code})` : ""}
            </div>
            <p className="text-xs text-red-600">{verifyOpen.res.error}</p>
            <p className="text-xs text-slate-500">
              Double-check the credential fields and retry. Status is now <strong>failed</strong>.
            </p>
          </div>
        )}
      </Modal>
    </div>
  );
}
