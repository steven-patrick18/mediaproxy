import { useEffect, useState } from "react";
import { api, type AuditEntry } from "../api";

export default function AuditLog() {
  const [rows, setRows] = useState<AuditEntry[]>([]);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    api
      .get<AuditEntry[]>("/api/v1/audit?limit=200")
      .then(setRows)
      .catch((e) => setErr(e.message));
  }, []);

  return (
    <div className="space-y-4">
      <header>
        <h2 className="text-2xl font-semibold tracking-tight">Audit log</h2>
        <p className="text-sm text-slate-500">Recent admin actions. (Auto-instrumentation is being added incrementally.)</p>
      </header>

      {err && (
        <div className="rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{err}</div>
      )}

      <div className="overflow-hidden rounded-lg border border-slate-200 bg-white shadow-sm">
        <table className="min-w-full divide-y divide-slate-200 text-sm">
          <thead className="bg-slate-50 text-left text-xs uppercase tracking-wide text-slate-500">
            <tr>
              <th className="px-4 py-2">When</th>
              <th className="px-4 py-2">Actor</th>
              <th className="px-4 py-2">Action</th>
              <th className="px-4 py-2">Target</th>
              <th className="px-4 py-2">IP</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100">
            {rows.length === 0 && (
              <tr><td colSpan={5} className="px-4 py-6 text-center text-slate-400">No audit events yet.</td></tr>
            )}
            {rows.map((e) => (
              <tr key={e.id}>
                <td className="px-4 py-2 text-xs text-slate-500">{new Date(e.ts).toLocaleString()}</td>
                <td className="px-4 py-2 font-mono text-xs">{e.actor_id ?? "system"}</td>
                <td className="px-4 py-2">{e.action}</td>
                <td className="px-4 py-2 text-xs text-slate-600">{e.target ?? "—"}</td>
                <td className="px-4 py-2 font-mono text-xs text-slate-500">{e.ip ?? ""}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
