import { useAuth } from "../auth";

export default function Settings() {
  const { user } = useAuth();
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

      <section className="rounded-lg border border-dashed border-slate-300 bg-white p-6 text-center text-sm text-slate-500">
        API keys + alerting integrations will land in a future iteration.
      </section>
    </div>
  );
}
