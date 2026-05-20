import { Navigate, Route, Routes } from "react-router-dom";
import Login from "./pages/Login";
import Dashboard from "./pages/Dashboard";
import Resellers from "./pages/Resellers";
import Clients from "./pages/Clients";
import ClientDetail from "./pages/ClientDetail";
import Nodes from "./pages/Nodes";
import IPPool from "./pages/IPPool";
import IPGroups from "./pages/IPGroups";
import SignalingIPs from "./pages/SignalingIPs";
import Carriers from "./pages/Carriers";
import RoutesPage from "./pages/Routes";
import Assignments from "./pages/Assignments";
import LiveCalls from "./pages/LiveCalls";
import CDRs from "./pages/CDRs";
import Reports from "./pages/Reports";
import AuditLog from "./pages/AuditLog";
import Users from "./pages/Users";
import Integrations from "./pages/Integrations";
import SettingsPage from "./pages/Settings";
import Layout from "./components/Layout";
import { useAuth } from "./auth";

function Protected({ children }: { children: JSX.Element }) {
  const { user, loading } = useAuth();
  if (loading) {
    return <div className="flex h-screen items-center justify-center text-slate-500">loading…</div>;
  }
  if (!user) return <Navigate to="/login" replace />;
  return children;
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route
        path="/"
        element={<Protected><Layout /></Protected>}
      >
        <Route index element={<Dashboard />} />
        <Route path="calls" element={<LiveCalls />} />
        <Route path="cdrs" element={<CDRs />} />
        <Route path="reports" element={<Reports />} />

        <Route path="resellers" element={<Resellers />} />
        <Route path="clients" element={<Clients />} />
        <Route path="clients/:id" element={<ClientDetail />} />

        <Route path="nodes" element={<Nodes />} />
        <Route path="ip-pool" element={<IPPool />} />
        <Route path="ip-groups" element={<IPGroups />} />
        <Route path="signaling-ips" element={<SignalingIPs />} />

        <Route path="carriers" element={<Carriers />} />
        <Route path="routes" element={<RoutesPage />} />
        <Route path="assignments" element={<Assignments />} />

        <Route path="users" element={<Users />} />
        <Route path="integrations" element={<Integrations />} />
        <Route path="audit" element={<AuditLog />} />
        <Route path="settings" element={<SettingsPage />} />
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
