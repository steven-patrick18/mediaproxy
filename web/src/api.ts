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

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
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
  created_at: string;
}
export interface Client {
  id: number;
  reseller_id: number;
  name: string;
  status: string;
  created_at: string;
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
}
export interface SignalingIP {
  id: number;
  ip_address: string;
  sip_proxy_node_id: number;
  status: "available" | "assigned" | "disabled";
  assigned_client_id?: number | null;
  created_at: string;
}
