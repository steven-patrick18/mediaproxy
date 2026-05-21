# Tuning a single node for capacity

Every per-node knob is in `/etc/node-agent/config.yaml` on each box. Edit, then `systemctl restart node-agent` — the agent regenerates `kamailio.cfg` / `rtpengine.conf` on its next tick and reload-or-restarts the affected daemon.

## What each knob does

| Knob | Default | What it controls | Bump when |
|---|---|---|---|
| `kamailio_children` | 16 | UDP SIP worker count. Each worker handles one INVITE at a time. | INVITE/s > 100 |
| `kamailio_tcp_children` | 4 | TCP SIP workers. Rarely the bottleneck unless your carrier requires TCP. | Almost never |
| `route_cache_seconds` | 5 | TTL for `/route` lookup cache in Kamailio htable. Hit rate >95% for typical dialer traffic. | Default is good; set `-1` to disable for debugging. |
| `rtpengine_port_min` | 30000 | Lower bound of UDP port range rtpengine assigns to RTP. | Need >5k concurrent calls |
| `rtpengine_port_max` | 60000 | Upper bound. ~4 ports used per call. | Expand to `1024-65535` for ~15k concurrent ceiling |
| `rtpengine_ng_listen` | `127.0.0.1:2223` | Where rtpengine listens for NG control. Set to `0.0.0.0:2223` if a remote SipProxy will talk to it. | Split-host setup |
| `rtpengine_sock` | `udp:127.0.0.1:2223` | Where Kamailio sends NG commands. Set to `udp:<MediaNode-IP>:2223` for split-host. | Split-host setup |

## Capacity matrix (single node, well-tuned)

These are **realistic ceilings** measured with `/route` cache enabled, 100% media-proxied calls, and PCMA codec.

| Concurrent calls | INVITE/s peak | `kamailio_children` | `rtpengine_port_max` | Notes |
|---|---|---|---|---|
| ≤ 500 | 20 | 16 (default) | 60000 (default) | Stock config handles this without tuning. |
| 500-2,000 | 50 | 16 | 60000 | The defaults still cover this. |
| 2,000-5,000 | 200 | 32 | 60000 | Bump worker count. |
| 5,000-10,000 | 500 | 48 | 65535 | Expand port range. Need 16+ vCPU host. |
| 10,000-15,000 | 1000 | 64 | 65535 | Approaching single-node ceiling. 32+ vCPU + 10G NIC. |
| > 15,000 | > 1500 | — | — | Need multi-node. See [SCALING.md](./SCALING.md). |

## Beyond Kamailio / rtpengine

The control plane (baseapp + Postgres) is the other constraint. Defaults:

| Knob | Default | Where set | Bump when |
|---|---|---|---|
| pgxpool `MaxConns` | 100 | `internal/db/db.go` | > 5000 concurrent calls (each call hits the pool ~4× over its lifetime; pool keeps connections active <100 ms each, so 100 conns = ~1000 req/s ceiling). For >10k concurrent, bump to 200 and increase Postgres `max_connections` to match. |
| nginx `upstream baseapp { keepalive N }` | 128 | `deploy/nginx/mediaproxy.conf` | > 1000 req/s sustained on baseapp endpoints |
| `net.core.rmem_max` | 8 MB (set by provisioner) | `/etc/sysctl.d/99-mediaproxy.conf` | Already large enough for any single-node load. |

## Example configs

### Modest call center (~500 concurrent peak)
```yaml
# /etc/node-agent/config.yaml
# Leave everything at defaults; no edits needed.
```

### Medium operator (~5k concurrent peak)
```yaml
# On sip_proxy:
kamailio_children: 32

# On media:
rtpengine_port_min: 16384
rtpengine_port_max: 65535
```

### Large single-node (~10k concurrent peak — push the limits)
```yaml
# On sip_proxy:
kamailio_children: 48
kamailio_tcp_children: 8
route_cache_seconds: 5      # default, just confirming it's on

# On media:
rtpengine_port_min: 1024
rtpengine_port_max: 65535
```

Plus on Base app: edit `internal/db/db.go` MaxConns to 200, rebuild, restart baseapp. Restart Postgres with `max_connections=300`.

## How to know you're hitting a ceiling

```bash
# 1. Recv-q on Kamailio's UDP socket should stay near 0.
#    Sustained recv-q > 50 KB = workers can't keep up.
ssh root@sipproxy 'ss -lunp | grep kamailio | awk "{print \$2, \$5}"'

# 2. /route round-trip time. Should be < 5 ms with the cache enabled.
ssh root@sipproxy 'journalctl -u kamailio --since "1 minute ago" | \
   grep -c "mp-route-reply: http_ok=1"'   # how many calls hit /route
# divide by 60 to get per-second rate

# 3. pgx pool saturation. If 'active' close to MaxConns, pool is the cap.
ssh root@base 'psql -d mediaproxy -c "SELECT state, COUNT(*) FROM \
   pg_stat_activity WHERE datname=\"mediaproxy\" GROUP BY state"'

# 4. rtpengine port usage. If close to (port_max - port_min) / 4, you're
#    out of ports — new calls will fail to setup media.
ssh root@media 'ss -unlp | grep rtpengine | wc -l'   # rough current allocations
```

When any of these signals are pinned, you've hit the per-node limit. Time to add a second node — see [SCALING.md](./SCALING.md).
