import { useEffect, useState } from "react";
import { useAuth } from "../auth";
import { api, ApiError } from "../api";
import Help from "../components/Help";

interface SystemSSH {
  password_auth_enabled: boolean;
  config: string;
}

export default function Settings() {
  const { user } = useAuth();
  const isAdmin = user?.role === "admin";

  const [ssh, setSsh] = useState<SystemSSH | null>(null);
  const [sshErr, setSshErr] = useState<string | null>(null);
  const [sshBusy, setSshBusy] = useState(false);

  async function loadSSH() {
    setSshErr(null);
    try {
      const data = await api.get<SystemSSH>("/api/v1/system/ssh");
      setSsh(data);
    } catch (e) {
      setSshErr(e instanceof ApiError ? e.message : String(e));
      setSsh(null);
    }
  }

  useEffect(() => {
    if (isAdmin) loadSSH();
  }, [isAdmin]);

  async function toggleSSH() {
    if (!ssh) return;
    const turningOn = !ssh.password_auth_enabled;
    const warn = turningOn
      ? "Enable SSH password authentication on the BASE APP host?\n\n" +
        "This allows anyone with valid credentials to log in via password from anywhere on the internet. " +
        "Public-key auth stays enabled either way. PermitRootLogin remains hard-locked to NO.\n\n" +
        "Recommended only for short troubleshooting windows. Continue?"
      : "Disable SSH password authentication on the BASE APP host?\n\n" +
        "After this, the only way in is via SSH key. If you don't already have a working key on this host, " +
        "you'll lock yourself out. Continue?";
    if (!window.confirm(warn)) return;
    setSshBusy(true);
    setSshErr(null);
    try {
      const data = await api.post<SystemSSH>("/api/v1/system/ssh", { password_auth: turningOn });
      setSsh(data ?? null);
      await loadSSH();
    } catch (e) {
      setSshErr(e instanceof ApiError ? e.message : String(e));
    } finally {
      setSshBusy(false);
    }
  }

  return (
    <div className="space-y-4">
      <header>
        <h2 className="text-xl font-semibold tracking-tight text-slate-800">Settings</h2>
        <p className="text-xs text-slate-500">Global configuration. Most settings are env-driven on the server.</p>
      </header>

      <section className="rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
        <h3 className="mb-3 text-sm font-semibold uppercase tracking-wide text-slate-500">Your account</h3>
        <dl className="grid grid-cols-1 gap-y-2 text-sm sm:grid-cols-2">
          <dt className="text-slate-500">Email</dt>
          <dd className="font-mono">{user?.email}</dd>
          <dt className="text-slate-500">Role</dt>
          <dd>{user?.role}</dd>
        </dl>
      </section>

      <section className="rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
        <h3 className="mb-3 text-sm font-semibold uppercase tracking-wide text-slate-500">Server</h3>
        <dl className="grid grid-cols-1 gap-y-2 text-sm sm:grid-cols-2">
          <dt className="text-slate-500">API base</dt>
          <dd className="font-mono">{window.location.origin}</dd>
          <dt className="text-slate-500">Health</dt>
          <dd><a href="/healthz" target="_blank" rel="noreferrer" className="font-mono text-brand-600 hover:underline">/healthz</a></dd>
          <dt className="text-slate-500">Readiness</dt>
          <dd><a href="/readyz" target="_blank" rel="noreferrer" className="font-mono text-brand-600 hover:underline">/readyz</a></dd>
        </dl>
        <p className="mt-3 text-xs text-slate-400">
          DB URL, Redis address, JWT secret, log level are all env-driven on the base-app server
          (<code className="font-mono">/opt/mediaproxy/.env</code>) — change there and restart
          <code className="font-mono"> mediaproxy-baseapp</code>.
        </p>
      </section>

      {isAdmin && (
        <section className="rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
          <h3 className="mb-3 flex items-center gap-2 text-sm font-semibold uppercase tracking-wide text-slate-500">
            Base-app SSH
            <Help>
              Controls SSH password authentication on THIS host (the base app itself). Public-key auth always stays
              on, and PermitRootLogin is permanently locked to "no". Changes apply immediately via
              <code className="font-mono"> sshd -t</code> validation, then
              <code className="font-mono"> systemctl reload ssh</code>.
            </Help>
          </h3>
          {sshErr && (
            <div className="mb-3 rounded border border-rose-300 bg-rose-50 px-3 py-2 text-xs text-rose-700">
              {sshErr}
            </div>
          )}
          {!ssh && !sshErr && (
            <p className="text-xs text-slate-400">Loading SSH state…</p>
          )}
          {ssh && (
            <div className="space-y-3">
              <div className="flex items-center justify-between gap-3 rounded border border-slate-200 bg-slate-50 px-3 py-2">
                <div className="text-sm">
                  <div className="font-medium text-slate-700">Password authentication</div>
                  <div className="text-xs text-slate-500">
                    Public-key login stays on either way. Root login over SSH is always disabled.
                  </div>
                </div>
                <button
                  type="button"
                  onClick={toggleSSH}
                  disabled={sshBusy}
                  className={
                    ssh.password_auth_enabled
                      ? "rounded border border-amber-300 bg-amber-50 px-3 py-1 text-xs font-medium text-amber-800 hover:bg-amber-100 disabled:opacity-50"
                      : "rounded border border-emerald-300 bg-emerald-50 px-3 py-1 text-xs font-medium text-emerald-800 hover:bg-emerald-100 disabled:opacity-50"
                  }
                >
                  {sshBusy
                    ? "Applying…"
                    : ssh.password_auth_enabled
                    ? "Password: ON — click to disable"
                    : "Password: OFF — click to enable"}
                </button>
              </div>
              <details className="text-xs">
                <summary className="cursor-pointer text-slate-500 hover:text-slate-700">
                  Effective config (/etc/ssh/sshd_config.d/99-mediaproxy.conf)
                </summary>
                <pre className="mt-2 max-h-64 overflow-auto rounded border border-slate-200 bg-slate-900 p-3 font-mono text-[11px] text-slate-100">
{ssh.config}
                </pre>
              </details>
              <p className="text-[11px] text-slate-400">
                Recommendation: keep password auth <strong>OFF</strong> on the base app. Only enable it as a temporary
                break-glass and turn it back off once you're done.
              </p>
            </div>
          )}
        </section>
      )}

      <section className="rounded-lg border border-dashed border-slate-300 bg-white p-6 text-center text-sm text-slate-500">
        API keys + alerting integrations will land in a future iteration.
      </section>
    </div>
  );
}
