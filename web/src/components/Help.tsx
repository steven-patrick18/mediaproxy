import { useEffect, useRef, useState, type ReactNode } from "react";

// A small "?" icon that opens an info popover on click. Used next to form
// labels and table headers to explain what a field/column actually means.
//
// Usage:
//   <label>Priority <Help>Lower = tried first. Default 100.</Help></label>
//
// The popover is positioned via absolute placement to the right of the
// trigger. Click anywhere outside (or press Escape) to close.
export default function Help({ children }: { children: ReactNode }) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLSpanElement>(null);

  useEffect(() => {
    if (!open) return;
    function onClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    }
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") setOpen(false);
    }
    document.addEventListener("mousedown", onClick);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onClick);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  return (
    <span ref={ref} className="relative ml-1 inline-block align-middle">
      <button
        type="button"
        onClick={(e) => {
          e.preventDefault();
          e.stopPropagation();
          setOpen((v) => !v);
        }}
        className={`inline-flex h-4 w-4 items-center justify-center rounded-full text-[10px] font-bold transition ${
          open
            ? "bg-brand-600 text-white"
            : "bg-slate-200 text-slate-600 hover:bg-brand-100 hover:text-brand-700"
        }`}
        aria-label="help"
      >
        ?
      </button>
      {open && (
        <div className="absolute left-5 top-0 z-50 w-80 rounded-lg border border-slate-200 bg-white p-3 text-xs leading-relaxed text-slate-700 shadow-lg">
          {children}
        </div>
      )}
    </span>
  );
}
