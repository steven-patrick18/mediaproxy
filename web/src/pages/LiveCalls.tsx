export default function LiveCalls() {
  return (
    <div className="space-y-4">
      <header>
        <h2 className="text-2xl font-semibold tracking-tight">Live calls</h2>
        <p className="text-sm text-slate-500">Real-time call list from RTPEngine.</p>
      </header>
      <div className="rounded-lg border border-dashed border-slate-300 bg-white p-8 text-center text-sm text-slate-500">
        No call data yet. Live calls will appear here once a media node and a SIP proxy are wired up
        and a dialer places a call. The agent feeds active-call counts via heartbeat; per-call
        records will be pushed via a websocket once RTPEngine is reporting.
      </div>
    </div>
  );
}
