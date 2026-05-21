# Scaling to 40,000 concurrent calls

This doc describes what changes when you outgrow a single SipProxy + MediaNode pair. **None of this is implemented today.** It's a roadmap, sized so you can plan effort and decide what to build first.

## Today's per-node ceiling (after the optimizations in this repo)

Roughly **10,000–15,000 concurrent calls** on a beefy box (32 vCPU, 64 GB RAM, 10 G NIC, properly tuned per [TUNING.md](./TUNING.md)). Above that you need a second node. See [TUNING.md](./TUNING.md) for the matrix.

## What 40,000 concurrent actually requires

Per-call resource cost (steady state):

| Resource | Per call | × 40,000 | Notes |
|---|---|---|---|
| RAM (Kamailio tm state + rtpengine session + active_calls row) | ~50 KB | ~2 GB | Easy on any modern box |
| UDP ports (rtpengine, 4 per call) | 4 | 160,000 | **No single box has 160k ports.** Hard wall at ~15k/node. |
| Bandwidth (PCMA, both directions) | 160 kbps | **6.4 Gbps** | Needs 10 G NIC per MediaNode; 4 nodes × 10 G works. |
| Postgres `active_calls` rows | 1 | 40,000 | Trivial for one Postgres instance. |
| HTTP req/s to baseapp (`/route` etc.) | ~5/call/lifecycle | varies with churn | At 1000 calls/s setup rate = 5000 req/s; with cache hit, ~500 req/s. |

So **4-6 MediaNode hosts** are mandatory (port range exhaustion). For SipProxy, throughput is mostly Kamailio worker count + `/route` cache hit rate; **3-4 SipProxy hosts** is a comfortable target.

## Target topology

```
                                  ┌─────────────────────────┐
   Dialer (Vicidial)               │  Base app (control      │
        │                          │  plane, single VPS)     │
        │                          │  - baseapp HTTP API     │
        │                          │  - Postgres 16 (primary)│
        │                          │  - HOMER stack          │
        │                          │  - Redis (shared state) │
        │                          └─────────────────────────┘
        ▼                                       ▲ heartbeats / call events
   ┌─────────────────────────────────────────────────────────┐
   │  SIP load balancer (option A: DNS round-robin           │
   │                     option B: kamailio dispatcher        │
   │                     option C: hardware/L4 LB)            │
   └─────────────────────────────────────────────────────────┘
        │                │                │              │
        ▼                ▼                ▼              ▼
   SipProxy-1       SipProxy-2       SipProxy-3     SipProxy-4
   (Kamailio)       (Kamailio)       (Kamailio)     (Kamailio)
   each: 32 vCPU, 16 workers, /route cache, ~10k cap
        │                │                │              │
        └─────┬──────────┴────────┬───────┴──────┬───────┘
              │                   │              │
              ▼                   ▼              ▼
       MediaNode-1         MediaNode-2     MediaNode-3   MediaNode-4
       (rtpengine)         (rtpengine)     (rtpengine)   (rtpengine)
       each: 32 vCPU, 10 G NIC, port range 1024-65535, ~10k cap

       └────────────── carrier(s) with 40k channel commit ──────────┘
```

## What needs to be built (the gaps)

### 1. SIP load balancing across multiple SipProxies — **MEDIUM effort (~1 week)**

Today: dialer points at ONE signaling IP, which lands on the single SipProxy. To scale, the dialer needs to spread calls across N SipProxies.

Options, ranked by simplicity:

- **DNS round-robin (A-records).** Dialer resolves `sip.example.com` → N IPs, picks one per call. Zero code changes. Downside: stale DNS cache during failover; no awareness of per-SipProxy load.
- **Multiple `client_ip → signaling_ip` mappings in the panel.** We already support this — a client can have several `signaling_ip` rows. The dialer is given the full list at provisioning time and round-robins client-side. Easiest if the dialer (Vicidial) supports it.
- **Kamailio dispatcher module on a "front" SipProxy.** A tiny SipProxy at the edge that does nothing but `ds_select_dst()` + `t_relay()` to one of the backend SipProxies. Adds a hop but gives smart load-aware routing.
- **L4 load balancer (HAProxy / dedicated hardware).** Most robust but adds operational complexity. Overkill for this scale.

Recommended: option 2 (panel-side multi-IP per client) for the first few thousand concurrent, then option 3 (dispatcher) when you outgrow that.

### 2. Shared call state across SipProxies — **MEDIUM effort (~3 days)**

Today: each SipProxy keeps in-memory state for its own calls (Kamailio tm transactions, the `call` htable, the `rcache`). When a call's INVITE lands on SipProxy-A but its BYE later lands on SipProxy-B (because the dialer's source port rotated, or DNS resolved differently), SipProxy-B has no record of the call and the BYE 481s.

Fix: ensure dialog persistence by hashing on Call-ID at the LB layer so all messages for a given call land on the same SipProxy. The dispatcher module + `dst_hash_size` modparam does this natively.

Alternative: replicate the `call` htable + `rcache` to Redis. Heavier, slower, but survives SipProxy failure mid-call.

### 3. MediaNode assignment + failover — **SMALL effort (~1 day)**

Today: `/route` picks a media_ip from a client's IP group via round_robin / least_used / etc. The IP group can already span multiple MediaNodes (the `node_ips` table joins to `media_nodes`).

What's missing:
- **Health-aware skip.** If MediaNode-2 goes offline, `/route` should automatically skip its IPs even before the operator marks the node `offline`. We have `media_nodes.last_seen_at` already — just add `WHERE last_seen_at > now() - 60s` to the `/route` query.
- **Media node draining.** Setting `status='draining'` should stop new call assignment but let in-flight calls complete naturally. We have the `draining` status; the `/route` query needs to honor it.

### 4. Postgres for high write throughput — **SMALL effort (~half day)**

At 40k concurrent + ~1000 calls/s setup rate, Postgres does roughly **10,000 writes/sec** (call-start INSERT, call-progress UPDATE, call-end DELETE+INSERT). Single Postgres can do this comfortably with:

- `wal_level=replica`, `synchronous_commit=off` (acceptable durability for CDR data — losing 100ms of writes on crash is fine)
- `max_connections = 300`
- Partition `active_calls` and `call_records` by day so old data doesn't slow current writes
- Move the database to a separate dedicated VPS (don't keep it on the Base App box)

### 5. Carrier connectivity — **OPERATIONAL, not code**

40k concurrent on one carrier account is unusual. Most wholesale carriers cap retail accounts at 100-2000 channels. To get 40k you need:

- A **carrier commit** for 40k channels (negotiated contract, not a credit-card signup)
- Often **multiple carrier interconnects** for redundancy (one carrier going down = 40k calls disconnected)
- **BGP or direct fiber** to the carrier's POP if you care about media quality at this scale

### 6. Observability — **MEDIUM effort (~1 week)**

At 40k concurrent, the page-by-page UI we have today won't scale. You'll want:

- **Prometheus exporters** on baseapp + each agent: req/s, /route p95, recv-q, active calls per node, MediaNode port utilization
- **Grafana dashboards** for at-a-glance cluster health
- **Alerting**: Slack/PagerDuty when ASR drops, when a node goes silent for >60s, when /route p95 > 100ms
- **Log aggregation**: Loki or similar, because grepping journalctl across 8-12 boxes during an incident is painful

## What to do first (effort vs benefit)

If you actually need to scale to 40k:

1. **Provision 4-6 MediaNode hosts** with the existing one-click flow. Expand port range to `1024-65535` on each. (≈ 1 day operational)
2. **Implement health-aware /route filter** so dead MediaNodes get skipped automatically. (≈ 4 hours)
3. **Provision 3-4 SipProxy hosts** the same way. (≈ 1 day operational)
4. **Set up DNS round-robin or multi-IP per client** so dialer spreads load. (≈ 1 day)
5. **Move Postgres to its own host**, tune `max_connections`, partition tables by day. (≈ half day)
6. **Build Prometheus/Grafana observability**. (≈ 1 week)
7. **Optionally** layer in Kamailio dispatcher for smart routing if the dialer-side LB is too crude.

Total: ~3 weeks of focused engineering work to get from current ~10k single-node to 40k multi-node. The carrier contract and BGP peering happen in parallel and are usually the long pole.
