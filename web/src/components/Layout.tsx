import { useEffect, useRef, useState } from "react";
import { Link, NavLink, Outlet, useLocation } from "react-router-dom";
import { useAuth } from "../auth";
import { ChevronDownIcon } from "./Icons";

type NavItem = { to: string; label: string };
type NavSection = { label: string; items: NavItem[] };

const nav: NavSection[] = [
  { label: "Dashboard", items: [{ to: "/", label: "Overview" }] },
  {
    label: "Calls",
    items: [
      { to: "/calls", label: "Live calls" },
      { to: "/privacy", label: "Privacy monitor" },
      { to: "/cdrs", label: "CDRs" },
      { to: "/reports", label: "Reports" },
    ],
  },
  {
    label: "Tenants",
    items: [
      { to: "/resellers", label: "Resellers" },
      { to: "/clients", label: "Clients" },
    ],
  },
  {
    label: "Infrastructure",
    items: [
      { to: "/nodes", label: "Nodes" },
      { to: "/ip-pool", label: "IP Pool" },
      { to: "/ip-groups", label: "IP Groups" },
      { to: "/signaling-ips", label: "Signaling IPs" },
    ],
  },
  {
    label: "Routing",
    items: [
      { to: "/carriers", label: "Carriers" },
      { to: "/routes", label: "Routes" },
      { to: "/assignments", label: "Assignments" },
    ],
  },
  {
    label: "System",
    items: [
      { to: "/users", label: "Admin users" },
      { to: "/firewall", label: "Firewall" },
      { to: "/audit", label: "Audit log" },
      { to: "/settings", label: "Settings" },
    ],
  },
];

function Dropdown({ section, currentPath }: { section: NavSection; currentPath: string }) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    function onClickOutside(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    }
    document.addEventListener("mousedown", onClickOutside);
    return () => document.removeEventListener("mousedown", onClickOutside);
  }, []);

  // Single-item sections (Dashboard) render as a direct link.
  if (section.items.length === 1) {
    const it = section.items[0];
    const active = currentPath === it.to;
    return (
      <Link
        to={it.to}
        className={`flex items-center rounded px-3 py-1.5 text-sm font-medium transition ${
          active ? "bg-slate-800 text-white" : "text-slate-300 hover:bg-slate-800 hover:text-white"
        }`}
      >
        {section.label}
      </Link>
    );
  }

  const anyActive = section.items.some((it) => currentPath === it.to || currentPath.startsWith(it.to + "/"));
  return (
    <div ref={ref} className="relative">
      <button
        onClick={() => setOpen((v) => !v)}
        className={`flex items-center gap-1 rounded px-3 py-1.5 text-sm font-medium transition ${
          anyActive ? "bg-slate-800 text-white" : "text-slate-300 hover:bg-slate-800 hover:text-white"
        }`}
      >
        {section.label}
        <ChevronDownIcon className="opacity-70" />
      </button>
      {open && (
        <div className="absolute left-0 top-full z-30 mt-1 w-48 overflow-hidden rounded-md border border-slate-200 bg-white shadow-lg">
          {section.items.map((it) => (
            <NavLink
              key={it.to}
              to={it.to}
              onClick={() => setOpen(false)}
              className={({ isActive }) =>
                `block px-3 py-2 text-sm ${
                  isActive ? "bg-brand-50 font-medium text-brand-700" : "text-slate-700 hover:bg-slate-100"
                }`
              }
            >
              {it.label}
            </NavLink>
          ))}
        </div>
      )}
    </div>
  );
}

function UserMenu() {
  const { user, logout } = useAuth();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    function on(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    }
    document.addEventListener("mousedown", on);
    return () => document.removeEventListener("mousedown", on);
  }, []);
  return (
    <div ref={ref} className="relative">
      <button
        onClick={() => setOpen((v) => !v)}
        className="flex items-center gap-2 rounded px-2 py-1 text-sm text-slate-200 hover:bg-slate-800 hover:text-white"
      >
        <div className="grid h-7 w-7 place-items-center rounded-full bg-brand-600 text-xs font-medium text-white">
          {user?.email?.[0]?.toUpperCase() ?? "?"}
        </div>
        <span className="hidden sm:inline">{user?.email}</span>
        <ChevronDownIcon className="opacity-70" />
      </button>
      {open && (
        <div className="absolute right-0 top-full z-30 mt-1 w-52 overflow-hidden rounded-md border border-slate-200 bg-white shadow-lg">
          <div className="border-b border-slate-100 px-3 py-2 text-xs text-slate-500">
            Signed in as<br />
            <span className="font-mono text-slate-700">{user?.email}</span>
            <span className="ml-2 rounded bg-brand-50 px-1.5 py-0.5 text-[10px] uppercase text-brand-700">
              {user?.role}
            </span>
          </div>
          <button
            onClick={logout}
            className="block w-full px-3 py-2 text-left text-sm text-slate-700 hover:bg-slate-100"
          >
            Log out
          </button>
        </div>
      )}
    </div>
  );
}

export default function Layout() {
  const loc = useLocation();
  return (
    <div className="flex h-screen flex-col bg-slate-100">
      <header className="border-b border-slate-700 bg-slate-900 text-slate-200">
        <div className="flex h-12 items-center px-4">
          <Link to="/" className="mr-6 flex items-center gap-2 text-white">
            <div className="grid h-7 w-7 place-items-center rounded bg-brand-600 text-xs font-bold">
              mp
            </div>
            <span className="font-semibold tracking-tight">mediaproxy</span>
          </Link>
          <nav className="flex flex-1 items-center gap-1">
            {nav.map((s) => (
              <Dropdown key={s.label} section={s} currentPath={loc.pathname} />
            ))}
          </nav>
          <UserMenu />
        </div>
      </header>

      <main className="flex-1 overflow-auto px-6 py-6">
        <Outlet />
      </main>
    </div>
  );
}
