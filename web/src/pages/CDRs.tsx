export default function CDRs() {
  return (
    <div className="space-y-4">
      <header>
        <h2 className="text-2xl font-semibold tracking-tight">CDRs</h2>
        <p className="text-sm text-slate-500">Call detail records — historical search.</p>
      </header>
      <div className="rounded-lg border border-dashed border-slate-300 bg-white p-8 text-center text-sm text-slate-500">
        No CDRs yet. Records will populate after Kamailio is in the call path and writes BYE events
        to the <code className="font-mono">call_records</code> table. CDR search/filter/export will
        be added once data is flowing.
      </div>
    </div>
  );
}
