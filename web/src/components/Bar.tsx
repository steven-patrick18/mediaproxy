export type BarTone = "ok" | "warn" | "bad" | "info";

const colors: Record<BarTone, string> = {
  ok: "bg-emerald-500",
  warn: "bg-amber-500",
  bad: "bg-red-500",
  info: "bg-brand-600",
};

export default function Bar({
  pct,
  tone,
  label,
  value,
}: {
  pct: number;
  tone?: BarTone;
  label: string;
  value: string;
}) {
  const clamped = Math.max(0, Math.min(100, pct));
  let t: BarTone = tone ?? "info";
  if (!tone) {
    if (clamped >= 85) t = "bad";
    else if (clamped >= 65) t = "warn";
    else t = "ok";
  }
  return (
    <div>
      <div className="mb-0.5 flex items-center justify-between text-xs">
        <span className="text-slate-500">{label}</span>
        <span className="font-mono text-slate-700">{value}</span>
      </div>
      <div className="h-1.5 w-full overflow-hidden rounded bg-slate-200">
        <div
          className={`h-full rounded ${colors[t]} transition-all`}
          style={{ width: `${clamped}%` }}
        />
      </div>
    </div>
  );
}
