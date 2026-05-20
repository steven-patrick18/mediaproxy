// Tiny dependency-free sparkline. Renders a polyline scaled to the
// extents of `data`; gaps (null) split the line.
export default function Spark({
  data,
  width = 120,
  height = 28,
  stroke = "currentColor",
}: {
  data: (number | null | undefined)[];
  width?: number;
  height?: number;
  stroke?: string;
}) {
  const vals = data.filter((v): v is number => typeof v === "number" && !Number.isNaN(v));
  if (vals.length < 2) {
    return <svg width={width} height={height} className="text-slate-400" />;
  }
  const min = Math.min(...vals);
  const max = Math.max(...vals, min + 0.001);
  const dx = width / Math.max(1, data.length - 1);
  let d = "";
  let pen = "M";
  data.forEach((v, i) => {
    if (typeof v !== "number" || Number.isNaN(v)) {
      pen = "M";
      return;
    }
    const x = i * dx;
    const y = height - ((v - min) / (max - min)) * height;
    d += `${pen}${x.toFixed(1)},${y.toFixed(1)} `;
    pen = "L";
  });
  return (
    <svg width={width} height={height} viewBox={`0 0 ${width} ${height}`} className="overflow-visible">
      <path d={d.trim()} fill="none" stroke={stroke} strokeWidth={1.5} strokeLinejoin="round" strokeLinecap="round" />
    </svg>
  );
}
