# mediaproxy REST API

Base URL: `https://mediaproxy.voipzap.com`

All `/api/v1/*` endpoints (except `auth/login` and `agent/*`) require a JSON Web Token in the `Authorization: Bearer <token>` header.

Get a token:
```http
POST /api/v1/auth/login
Content-Type: application/json

{ "email": "you@example.com", "password": "your-pass", "mfa_code": "123456" }
```

`mfa_code` is required only if your account has MFA enrolled. On first failed login because of MFA, the response is `401 {"error":"mfa_required","mfa_required":true}` — the UI uses that to show the TOTP prompt.

## Resource summary

| Resource | List | Create | Update | Delete | Notes |
|---|---|---|---|---|---|
| **Resellers** | `GET /resellers` | `POST /resellers` | `PATCH /resellers/:id` | `DELETE /resellers/:id` | top-level tenants |
| **Clients** | `GET /clients` | `POST /clients` | `PATCH /clients/:id` | `DELETE /clients/:id` | `GET /clients/:id` for detail |
| **Dialer IPs** | `GET /clients/:id/dialer-ips` | `POST /clients/:id/dialer-ips` | — | `DELETE /clients/:id/dialer-ips/:dialer_ip_id` | source-IP whitelist per client |
| **Signaling IPs** | `GET /signaling-ips` | `POST /signaling-ips` | `PATCH /signaling-ips/:id` | `DELETE /signaling-ips/:id` | auto-discovered from sip_proxy heartbeats too |
| **Client → Signaling** | — | `POST /clients/:id/signaling-ip` | — | `DELETE /clients/:id/signaling-ip` | assign / unassign |
| **Nodes** | `GET /nodes` | `POST /nodes` | `PATCH /nodes/:id` | `DELETE /nodes/:id` | `POST /nodes/:id/{drain,undrain,provision}`, `GET /nodes/:id/metrics?minutes=` |
| **Node Commands** | `GET /nodes/:id/commands` | `POST /nodes/:id/commands` | — | — | types: `apply`, `apply_firewall`, `reboot`, `restart_rtpengine`, `restart_kamailio` |
| **Node IPs (pool)** | `GET /node-ips` | `POST /node-ips`, `POST /node-ips/bulk` | `PATCH /node-ips/:id` | `DELETE /node-ips/:id` | auto-discovered from media heartbeats |
| **IP Groups** | `GET /ip-groups` | `POST /ip-groups` | `PATCH /ip-groups/:id` | `DELETE /ip-groups/:id` | `GET/POST/DELETE /ip-groups/:id/members` |
| **Carriers** | `GET /carriers` | `POST /carriers` | `PATCH /carriers/:id` | `DELETE /carriers/:id` | `GET /carriers/:id/node-history` |
| **Routes** | `GET /routes` | `POST /routes` | `PATCH /routes/:id` | `DELETE /routes/:id` | longest-prefix match, priority asc |
| **Assignments** | `GET /assignments` | `POST /assignments` | — | `DELETE /assignments/:id` | binds (client, carrier) → IP group + rotation strategy |
| **CDRs** | `GET /cdrs` (filters: client_id, carrier_id, node_id, disposition, from, to, dnis) | — | — | — | `GET /cdrs/stats` for ASR/ACD aggregates |
| **Active calls** | `GET /calls/active?node_id=` | — | — | — | rows < 2 min old |
| **Firewall** | `GET /firewall/rules` | `POST /firewall/rules` | `PATCH /firewall/rules/:id` | `DELETE /firewall/rules/:id` | `GET /firewall/preview/:node_id` for synthesized nft config |
| **Admin users** | `GET /admin-users` | `POST /admin-users` (admin) | `PATCH /admin-users/:id` (admin) | `DELETE /admin-users/:id` (admin) | roles: `admin`, `noc`, `reseller`, `viewer` |
| **MFA** | — | `POST /mfa/setup`, `POST /mfa/verify` | — | `POST /mfa/disable` | TOTP, returns QR-png-base64 on setup |
| **Webhooks** | `GET /webhooks` | `POST /webhooks` | `PATCH /webhooks/:id` | `DELETE /webhooks/:id` | `POST /webhooks/:id/test`, `GET /webhooks/:id/deliveries` |
| **Audit** | `GET /audit?limit=` | — | — | — | every mutation is logged automatically |
| **Routing diagnostic** | `GET /route?src_ip=&dnis=` | — | — | — | returns the full routing decision; what Kamailio sees per call |

## Agent endpoints (auth: `Authorization: Bearer <agent_token>`)

These are the URLs the node-agent and Kamailio talk to. Don't call them with a JWT.

| Endpoint | Method | Notes |
|---|---|---|
| `/api/v1/agent/register` | POST | first call on boot; returns `expected_ips` |
| `/api/v1/agent/heartbeat` | POST | every 10s; ships metrics + bound_ips; returns `expected_ips` + any queued commands |
| `/api/v1/agent/command-result` | POST | ack a command (status: ok / error) |
| `/api/v1/agent/firewall` | GET | returns synthesized nftables config for this node |
| `/api/v1/agent/firewall-applied` | POST | confirms successful apply (cancels the rollback `at` job) |
| `/api/v1/agent/call-start` | POST | Kamailio: a new call has begun |
| `/api/v1/agent/call-end` | POST | Kamailio: BYE / final non-2xx — writes a CDR |

## Webhook event payloads

All webhook POSTs include:
- `Content-Type: application/json`
- `X-Mediaproxy-Event: <event-name>`
- `X-Mediaproxy-Signature: hex(hmac_sha256(secret, body))` if `secret` is set on the subscription

Events currently emitted:

| Event | Payload |
|---|---|
| `node.offline` | `{event, node_id, node_name, role, timestamp, message}` — fires when `last_seen_at > 2 min` |
| `node.online` | `{event, node_id, node_name, role, timestamp}` — fires when an offline-flagged node heartbeats again |
| `test` | from `POST /webhooks/:id/test` — verifies URL + signing works |

## Routing — what `/route` returns

```http
GET /api/v1/route?src_ip=1.2.3.4&dnis=14155551212
Authorization: Bearer <jwt-or-agent-token>
```

```json
{
  "client_id": 1,
  "client_name": "Alfa",
  "signaling_ip": "45.76.7.40",
  "carrier_id": 3,
  "carrier_host": "sip.carrier.com",
  "carrier_port": 5060,
  "carrier_transport": "udp",
  "media_node_id": 7,
  "media_ip": "192.0.2.42",
  "rotation_strategy": "round_robin"
}
```

On failure, returns 200 with `{"error":"...","code":<sip-code>}` so Kamailio can map directly to the right SIP status (403 / 404 / 503).

## Heartbeat request body (agent → /heartbeat)

```json
{
  "bound_ips":      ["45.76.7.40", "216.155.135.216", "64.176.220.158"],
  "active_calls":   12,
  "cpu_pct":        4.2,
  "ram_pct":        18.5,
  "net_in_mbps":    0.4,
  "net_out_mbps":   0.7,
  "packet_loss_pct": 0,
  "uptime_seconds": 86412,
  "agent_version":  "0.1.0"
}
```

Response:
```json
{
  "expected_ips": ["45.76.7.40", "216.155.135.216", "64.176.220.158"],
  "commands": [
    { "id": "12", "type": "apply_firewall" }
  ]
}
```
