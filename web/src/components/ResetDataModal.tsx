import { useEffect, useState } from "react";
import { api } from "../api";

// ResetDataModal — destructive data wipe with a typed-confirmation guard.
// Operator picks which scopes to clear, types "RESET" verbatim, and only
// then can submit. All three scopes are *operational* data; configuration
// (clients, carriers, nodes, IPs) is never touched.
type Scope = "active_calls" | "cdrs" | "metrics";

const scopeMeta: Record<Scope, { label: string; help: string }> = {
  active_calls: {
    label: "In-flight calls + counters",
    help: "Drops every row in active_calls and resets node_ips.current_calls to 0. Use this when the live calls list shows stuck/ghost rows.",
  },
  cdrs: {
    label: "CDR history",
    help: "Deletes every row from call_records. The Calls→CDRs page becomes empty. Reports for past windows lose their data.",
  },
  metrics: {
    label: "Node metrics history (CPU/RAM/net)",
    help: "Deletes per-heartbeat history that powers the sparklines on the Nodes page. Latest values stay (refilled on next heartbeat).",
  },
};

export default function ResetDataModal({ onClose }: { onClose: () => void }) {
  const [scopes, setScopes] = useState<Record<Scope, boolean>>({
    active_calls: false,
    cdrs: false,
    metrics: false,
  });
  const [confirm, setConfirm] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [result, setResult] = useState<Record<string, number> | null>(null);

  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      if (e.key === "Escape" && !busy) onClose();
    };
    window.addEventListener("keydown", h);
    return () => window.removeEventListener("keydown", h);
  }, [busy, onClose]);

  const selected = (Object.keys(scopes) as Scope[]).filter((k) => scopes[k]);
  const canSubmit = selected.length > 0 && confirm === "RESET" && !busy;

  async function submit() {
    setBusy(true);
    setErr(null);
    setResult(null);
    try {
      const r = await api.post<{ ok: boolean; result: Record<string, number> }>(
        "/api/v1/admin/reset",
        { scopes: selected, confirm },
      );
      setResult(r.result);
    } catch (e) {
      setErr(e instanceof Error ? e.message : "reset failed");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/60 p-4"
      onClick={busy ? undefined : onClose}
    >
      <div
        className="w-full max-w-lg rounded-lg bg-white shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <header className="flex items-center justify-between border-b border-slate-200 px-4 py-3">
          <div>
            <h3 className="text-lg font-semibold tracking-tight text-rose-700">Clear call data</h3>
            <p className="text-xs text-slate-500">
              Destructive · operational data only · config not affected
            </p>
          </div>
          <button
            onClick={onClose}
            disabled={busy}
            className="rounded-md border border-slate-300 px-3 py-1 text-sm hover:bg-slate-50 disabled:opacity-50"
          >
            Close
          </button>
        </header>

        <div className="space-y-4 p-4">
          {!result ? (
            <>
              <div>
                <p className="mb-2 text-sm font-medium text-slate-700">Pick scope:</p>
                <ul className="space-y-2">
                  {(Object.keys(scopeMeta) as Scope[]).map((k) => (
                    <li
                      key={k}
                      className="rounded border border-slate-200 p-3 hover:border-slate-300"
                    >
                      <label className="flex items-start gap-2">
                        <input
                          type="checkbox"
                          className="mt-1"
                          checked={scopes[k]}
                          onChange={(e) =>
                            setScopes((s) => ({ ...s, [k]: e.target.checked }))
                          }
                        />
                        <div>
                          <div className="text-sm font-medium text-slate-800">
                            {scopeMeta[k].label}
                          </div>
                          <div className="text-xs text-slate-500">{scopeMeta[k].help}</div>
                        </div>
                      </label>
                    </li>
                  ))}
                </ul>
              </div>

              <div className="rounded border border-rose-200 bg-rose-50 p-3 text-sm text-rose-800">
                <p className="font-medium">This cannot be undone.</p>
                <p className="mt-1 text-xs">
                  Type <code className="rounded bg-white px-1 font-mono">RESET</code> (uppercase)
                  to enable the button.
                </p>
                <input
                  type="text"
                  value={confirm}
                  onChange={(e) => setConfirm(e.target.value)}
                  placeholder="RESET"
                  autoComplete="off"
                  className="mt-2 w-full rounded border border-rose-300 bg-white px-3 py-1.5 text-sm font-mono focus:border-rose-500 focus:outline-none"
                />
              </div>

              {err && (
                <div className="rounded border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">
                  {err}
                </div>
              )}

              <div className="flex justify-end gap-2 border-t border-slate-100 pt-3">
                <button
                  onClick={onClose}
                  disabled={busy}
                  className="rounded-md border border-slate-300 px-3 py-1.5 text-sm hover:bg-slate-50 disabled:opacity-50"
                >
                  Cancel
                </button>
                <button
                  onClick={submit}
                  disabled={!canSubmit}
                  className="rounded-md bg-rose-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-rose-700 disabled:opacity-50"
                >
                  {busy ? "Wiping…" : "Reset selected scopes"}
                </button>
              </div>
            </>
          ) : (
            <>
              <div className="rounded border border-emerald-200 bg-emerald-50 p-3 text-sm">
                <p className="font-medium text-emerald-800">Reset complete.</p>
                <ul className="mt-2 space-y-1 font-mono text-xs text-slate-700">
                  {Object.entries(result).map(([k, v]) => (
                    <li key={k}>
                      {k}: <span className="font-medium">{v.toLocaleString()}</span> row
                      {v === 1 ? "" : "s"}
                    </li>
                  ))}
                </ul>
              </div>
              <div className="flex justify-end pt-2">
                <button
                  onClick={onClose}
                  className="rounded-md bg-slate-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-slate-700"
                >
                  Done
                </button>
              </div>
            </>
          )}
        </div>
      </div>
    </div>
  );
}
