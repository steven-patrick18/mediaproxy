import { NavLink, Outlet } from "react-router-dom";
import { useAuth } from "../auth";

const sections: { heading: string; items: { to: string; label: string; end?: boolean }[] }[] = [
  {
    heading: "Overview",
    items: [
      { to: "/", label: "Dashboard", end: true },
      { to: "/calls", label: "Live calls" },
      { to: "/cdrs", label: "CDRs" },
    ],
  },
  {
    heading: "Tenants",
    items: [
      { to: "/resellers", label: "Resellers" },
      { to: "/clients", label: "Clients" },
    ],
  },
  {
    heading: "Infrastructure",
    items: [
      { to: "/nodes", label: "Nodes" },
      { to: "/ip-pool", label: "IP Pool" },
      { to: "/ip-groups", label: "IP Groups" },
      { to: "/signaling-ips", label: "Signaling IPs" },
    ],
  },
  {
    heading: "Routing",
    items: [
      { to: "/carriers", label: "Carriers" },
      { to: "/routes", label: "Routes" },
      { to: "/assignments", label: "Assignments" },
    ],
  },
  {
    heading: "Operations",
    items: [{ to: "/audit", label: "Audit log" }],
  },
];

export default function Layout() {
  const { user, logout } = useAuth();

  const navItem = ({ isActive }: { isActive: boolean }) =>
    `block rounded px-3 py-1.5 text-sm transition ${
      isActive ? "bg-brand-600 text-white" : "text-slate-600 hover:bg-slate-200"
    }`;

  return (
    <div className="flex h-screen">
      <aside className="flex w-60 flex-col border-r border-slate-200 bg-white">
        <div className="border-b border-slate-200 px-4 py-4">
          <h1 className="text-lg font-semibold tracking-tight text-slate-900">mediaproxy</h1>
          <p className="text-xs text-slate-500">control plane</p>
        </div>
        <nav className="flex-1 overflow-y-auto p-3">
          {sections.map((s) => (
            <div key={s.heading} className="mb-4">
              <div className="mb-1 px-2 text-xs font-semibold uppercase tracking-wide text-slate-400">
                {s.heading}
              </div>
              <div className="space-y-0.5">
                {s.items.map((it) => (
                  <NavLink key={it.to} to={it.to} end={it.end} className={navItem}>
                    {it.label}
                  </NavLink>
                ))}
              </div>
            </div>
          ))}
        </nav>
        <div className="border-t border-slate-200 p-3 text-xs text-slate-500">
          <div className="mb-2 truncate">{user?.email}</div>
          <button
            onClick={logout}
            className="w-full rounded border border-slate-300 px-2 py-1 text-slate-700 hover:bg-slate-100"
          >
            Log out
          </button>
        </div>
      </aside>
      <main className="flex-1 overflow-auto p-8">
        <Outlet />
      </main>
    </div>
  );
}
