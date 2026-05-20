const TOKEN_KEY = "mp.token";

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY);
}
export function setToken(t: string | null) {
  if (t === null) localStorage.removeItem(TOKEN_KEY);
  else localStorage.setItem(TOKEN_KEY, t);
}

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const headers: Record<string, string> = {};
  if (body !== undefined) headers["Content-Type"] = "application/json";
  const tok = getToken();
  if (tok) headers["Authorization"] = `Bearer ${tok}`;
  const res = await fetch(path, {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  const text = await res.text();
  const data = text ? JSON.parse(text) : null;
  if (!res.ok) {
    const msg = (data && (data.error || data.message)) || res.statusText;
    throw new ApiError(res.status, msg);
  }
  return data as T;
}

export const api = {
  get: <T>(path: string) => request<T>("GET", path),
  post: <T>(path: string, body?: unknown) => request<T>("POST", path, body),
  patch: <T>(path: string, body?: unknown) => request<T>("PATCH", path, body),
  del: <T>(path: string) => request<T>("DELETE", path),
};

export interface User {
  id: number;
  email: string;
  role: string;
}
export interface Reseller {
  id: number;
  name: string;
  status: string;
  notes?: string | null;
  created_at: string;
}
export interface Client {
  id: number;
  reseller_id: number;
  name: string;
  status: string;
  notes?: string | null;
  created_at: string;
}
export interface AdminUserRow {
  id: number;
  email: string;
  role: "admin" | "noc" | "reseller" | "viewer";
  status: "active" | "suspended";
  has_mfa: boolean;
  created_at: string;
}
export interface CDR {
  id: number;
  call_id: string;
  client_id?: number | null;
  carrier_id?: number | null;
  node_id?: number | null;
  media_ip?: string | null;
  signaling_from?: string | null;
  ani?: string | null;
  dnis?: string | null;
  started_at: string;
  answered_at?: string | null;
  ended_at?: string | null;
  duration_sec?: number | null;
  disposition?: string | null;
  sip_code?: number | null;
}
export interface CDRStats {
  total: number;
  answered: number;
  asr_pct: number;
  acd_seconds?: number | null;
}
export interface ActiveCallRow {
  id: number;
  call_id: string;
  client_id?: number | null;
  carrier_id?: number | null;
  node_id?: number | null;
  media_ip?: string | null;
  signaling_from?: string | null;
  ani?: string | null;
  dnis?: string | null;
  started_at: string;
  last_seen_at: string;
  duration_sec: number;
}
export interface ClientDetail extends Client {
  signaling_ip_id: number | null;
  signaling_ip: string | null;
}
export interface DialerIP {
  id: number;
  client_id: number;
  ip_address: string;
  status: string;
  created_at: string;
}
export interface MediaNode {
  id: number;
  name: string;
  role: "media" | "sip_proxy";
  host_ip: string;
  region: string | null;
  max_calls: number;
  transcoding_enabled: boolean;
  status: "online" | "offline" | "draining";
  agent_token?: string;
  last_seen_at: string | null;
  created_at: string;
  active_calls?: number | null;
  cpu_pct?: number | null;
  ram_pct?: number | null;
  net_in_mbps?: number | null;
  net_out_mbps?: number | null;
  packet_loss_pct?: number | null;
  uptime_seconds?: number | null;
  agent_version?: string | null;
  ips_bound: number;
  ips_total: number;
}
export interface MetricPoint {
  ts: string;
  active_calls?: number | null;
  cpu_pct?: number | null;
  ram_pct?: number | null;
  net_in_mbps?: number | null;
  net_out_mbps?: number | null;
  packet_loss_pct?: number | null;
}
export interface NodeIP {
  id: number;
  node_id: number;
  ip_address: string;
  status: "active" | "disabled" | "flagged" | "reserve";
  purchased_from?: string | null;
  lease_block?: string | null;
  monthly_cost?: number | null;
  rdns?: string | null;
  reputation_score?: number | null;
  current_calls: number;
  auto_discovered?: boolean;
  created_at: string;
}
export interface SignalingIP {
  id: number;
  ip_address: string;
  sip_proxy_node_id: number;
  status: "available" | "assigned" | "disabled";
  assigned_client_id?: number | null;
  created_at: string;
}
export interface Carrier {
  id: number;
  name: string;
  host: string;
  port: number;
  transport: "udp" | "tcp" | "tls";
  assigned_node_id?: number | null;
  codec_pref?: string | null;
  status: "active" | "paused" | "disabled";
  created_at: string;
}
export interface CarrierHistoryEntry {
  id: number;
  old_node_id?: number | null;
  new_node_id?: number | null;
  changed_by?: number | null;
  changed_at: string;
  reason?: string | null;
}
export interface IPGroup {
  id: number;
  name: string;
  status: string;
  notes?: string | null;
  created_by?: number | null;
  created_at: string;
  ip_count: number;
}
export interface IPGroupMember {
  ip_id: number;
  ip_address: string;
  node_id: number;
  active: boolean;
}
export interface Route {
  id: number;
  client_id: number;
  match_prefix?: string | null;
  carrier_id: number;
  priority: number;
  status: string;
}
export interface Assignment {
  id: number;
  group_id: number;
  client_id: number;
  carrier_id: number;
  rotation_strategy: string;
  status: string;
  assigned_by?: number | null;
  assigned_at: string;
}
export interface AuditEntry {
  id: number;
  actor_id?: number | null;
  action: string;
  target?: string | null;
  before?: unknown;
  after?: unknown;
  ip?: string | null;
  ts: string;
}
