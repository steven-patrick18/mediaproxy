// Inline SVG icons — no icon library so the bundle stays small.

type P = { className?: string };

export const PencilIcon = (p: P) => (
  <svg viewBox="0 0 20 20" fill="currentColor" className={p.className} width={14} height={14}>
    <path d="M13.586 3.586a2 2 0 1 1 2.828 2.828l-.793.793-2.828-2.828.793-.793Zm-1.793 1.793-9 9V17.5h3.121l9-9-3.121-3.121Z" />
  </svg>
);

export const TrashIcon = (p: P) => (
  <svg viewBox="0 0 20 20" fill="currentColor" className={p.className} width={14} height={14}>
    <path d="M8 4V3a1 1 0 0 1 1-1h2a1 1 0 0 1 1 1v1h3a1 1 0 1 1 0 2h-.59l-.94 11.27a2 2 0 0 1-2 1.73H7.53a2 2 0 0 1-2-1.73L4.59 6H4a1 1 0 0 1 0-2h4Zm5 0V3h-2v1h2ZM6.6 6l.9 11h5l.9-11H6.6Z" />
  </svg>
);

export const ChevronDownIcon = (p: P) => (
  <svg viewBox="0 0 20 20" fill="currentColor" className={p.className} width={14} height={14}>
    <path d="M5.23 7.21a.75.75 0 0 1 1.06.02L10 11.04l3.71-3.81a.75.75 0 1 1 1.08 1.04l-4.25 4.36a.75.75 0 0 1-1.08 0L5.21 8.27a.75.75 0 0 1 .02-1.06Z" />
  </svg>
);

export const PlusIcon = (p: P) => (
  <svg viewBox="0 0 20 20" fill="currentColor" className={p.className} width={14} height={14}>
    <path d="M10 4a.75.75 0 0 1 .75.75v4.5h4.5a.75.75 0 0 1 0 1.5h-4.5v4.5a.75.75 0 0 1-1.5 0v-4.5h-4.5a.75.75 0 0 1 0-1.5h4.5v-4.5A.75.75 0 0 1 10 4Z" />
  </svg>
);

export const RefreshIcon = (p: P) => (
  <svg viewBox="0 0 20 20" fill="currentColor" className={p.className} width={14} height={14}>
    <path d="M4 10a6 6 0 0 1 10.243-4.243L13 7h4V3l-1.464 1.464A8 8 0 1 0 18 10h-2Z" />
  </svg>
);

export const DownloadIcon = (p: P) => (
  <svg viewBox="0 0 20 20" fill="currentColor" className={p.className} width={14} height={14}>
    <path d="M10 3a.75.75 0 0 1 .75.75v7.69l2.22-2.22a.75.75 0 0 1 1.06 1.06l-3.5 3.5a.75.75 0 0 1-1.06 0l-3.5-3.5a.75.75 0 1 1 1.06-1.06l2.22 2.22V3.75A.75.75 0 0 1 10 3ZM3 14a.75.75 0 0 1 .75.75V16h12.5v-1.25a.75.75 0 0 1 1.5 0v1.5A.75.75 0 0 1 17 17H3a.75.75 0 0 1-.75-.75v-1.5A.75.75 0 0 1 3 14Z" />
  </svg>
);
