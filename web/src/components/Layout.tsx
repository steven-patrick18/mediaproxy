import { NavLink, Outlet } from "react-router-dom";
import { useAuth } from "../auth";

export default function Layout() {
  const { user, logout } = useAuth();

  const navItem = ({ isActive }: { isActive: boolean }) =>
    `block rounded px-3 py-2 text-sm transition ${
      isActive
        ? "bg-brand-600 text-white"
        : "text-slate-600 hover:bg-slate-200"
    }`;

  return (
    <div className="flex h-screen">
      <aside className="flex w-60 flex-col border-r border-slate-200 bg-white">
        <div className="border-b border-slate-200 px-4 py-4">
          <h1 className="text-lg font-semibold tracking-tight text-slate-900">
            mediaproxy
          </h1>
          <p className="text-xs text-slate-500">control plane</p>
        </div>
        <nav className="flex-1 space-y-1 p-3">
          <NavLink to="/" end className={navItem}>
            Dashboard
          </NavLink>
          <NavLink to="/nodes" className={navItem}>
            Nodes
          </NavLink>
          <NavLink to="/signaling-ips" className={navItem}>
            Signaling IPs
          </NavLink>
          <NavLink to="/resellers" className={navItem}>
            Resellers
          </NavLink>
          <NavLink to="/clients" className={navItem}>
            Clients
          </NavLink>
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
