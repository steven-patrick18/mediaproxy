# VoIP Media IP Rotation Platform — Complete Architecture

> A multi-tenant SBC/media-relay platform. Dialers send SIP calls in; the platform
> rewrites the media (RTP) IP per call using a managed pool of leased IPs spread
> across media nodes, then forwards to carriers. Signaling stays on fixed IPs;
> only media IPs rotate.

---

## 1. SYSTEM OVERVIEW

```
+-------------------------------------------------------------------------+
|                          PLATFORM PURPOSE                               |
|                                                                         |
|  Dialer sends call  -->  Platform changes MEDIA IP  -->  Carrier        |
|                                                                         |
|  - Signaling (SIP)  : fixed platform IP (carrier-whitelisted)           |
|  - Media (RTP)       : rotated from a pool of leased IPs                 |
|  - Multi-tenant      : resellers -> clients -> IP groups -> carriers     |
+-------------------------------------------------------------------------+
```

### Core Concepts

| Concept       | Meaning                                                          |
|---------------|------------------------------------------------------------------|
| Base App      | Control plane: panel + API + DB. The brain. One source of truth. |
| SIP Proxy     | Kamailio. Handles signaling, whitelist, SDP rewrite.             |
| Media Node    | RTPEngine server holding many IPs. Relays RTP, changes media IP. |
| IP Pool       | All leased IPs across all nodes.                                 |
| IP Group      | A named subset of IPs (e.g. 60) assigned to a client+carrier.    |
| Agent Daemon  | Small program on each node, reports metrics + takes commands.    |
| Reseller      | Top-level tenant who owns clients.                               |
| Client        | End customer whose dialer IPs are whitelisted.                   |
| Carrier       | Upstream termination provider. Mapped to a media node.           |

---

## 2. HIGH-LEVEL TOPOLOGY

```
                              INTERNET
                                 |
        +------------------------+------------------------+
        |                        |                        |
   +---------+              +---------+              +-----------+
   | DIALER  |              | DIALER  |              |  CARRIER  |
   | Client A|              | Client B|              | A / B / C |
   +----+----+              +----+----+              +-----+-----+
        |                        |                         |
        +-----------+------------+                         |
                    |                                      |
                    v                                      |
         +----------------------+                          |
         |  DNS SRV / Floating  |                          |
         |  sip.yourdomain.com  |                          |
         +----------+-----------+                          |
                    |                                      |
       +------------+------------+                         |
       v            v            v                         |
   +-------+    +-------+    +-------+                      |
   | SIP   |    | SIP   |    | SIP   |   (Kamailio cluster) |
   | Proxy1|    | Proxy2|    | Proxy3|                      |
   +---+---+    +---+---+    +---+---+                      |
       |            |            |                          |
       +------+-----+-----+------+                          |
              |           |                                 |
              v           v                                 |
       +------------+ +------------+                        |
       | BASE APP   | | BASE APP   |                        |
       | PRIMARY    | | STANDBY    |                        |
       | Panel + DB | | DB replica |                        |
       +------------+ +------------+                        |
              |                                             |
              | proxies forward INVITE to carrier  ---------+
              | AND command media nodes for RTP
              v
   +-----------------------------------------------------------+
   |                    MEDIA NODE CLUSTER                     |
   |  (carries RTP only - no DB, no UI, no signaling logic)    |
   |                                                           |
   |  DC1 (US East)     DC2 (US West)     DC3 (US Central)     |
   |  Node 1..6         Node 7..12        Node 13..18          |
   |  ~250 IPs          ~250 IPs          ~250 IPs             |
   +-----------------------------------------------------------+
                                |
                                v
                  +-----------------------------+
                  |   Leased IP Inventory       |
                  |   4x /24 = ~1000 IPs        |
                  |   IPXO / Heficed            |
                  |   Routed via BGP per DC     |
                  +-----------------------------+
```

---

## 3. WHO SEES WHICH IP

```
   CLIENT DIALER                PLATFORM                   CARRIER
   1.2.3.4                                                 5.6.7.8

   Signaling -->  [ SIP Proxy IP: 203.0.113.10 ]  --> Signaling
   (fixed, both sides see this same IP - never rotates)

   Media     -->  [ Media Node IP: 192.0.2.X   ]  --> Media
   (rotates per call from the assigned IP group)

   Carrier sees:  signaling 203.0.113.10, media 192.0.2.X
   Client sees:   signaling 203.0.113.10, media 192.0.2.X
   Neither sees the other's true IP. Platform sits in the middle.
```

Rule: **only media IPs rotate**. Signaling IPs are fixed and carrier-whitelisted.

---

## 4. IP POOL -> GROUPS -> ASSIGNMENT

```
+-----------------------------------------------------------------+
|                     LEASED IP INVENTORY                         |
|  /24 Block A (IPXO)        /24 Block B (Heficed)                |
|  192.0.2.0/24              198.51.100.0/24                      |
|  -> routed to DC1          -> routed to DC2                     |
+------------------------------+----------------------------------+
                               |
                               v
+-----------------------------------------------------------------+
|                        GLOBAL IP POOL                           |
|  ip            node    status    ASR    reputation              |
|  192.0.2.1     Node1   active    42%    clean                   |
|  192.0.2.2     Node1   active    38%    clean                   |
|  192.0.2.3     Node1   flagged   8%     spamhaus-listed         |
|  ...                                                            |
+------------------------------+----------------------------------+
                               | admin builds groups
              +----------------+----------------+
              v                v                v
        +-----------+    +-----------+    +-----------+
        | GROUP A   |    | GROUP B   |    | GROUP C   |
        | 60 IPs    |    | 60 IPs    |    | 60 IPs    |
        | (Node 1)  |    | (Node 2)  |    | (Node 3)  |
        +-----+-----+    +-----+-----+    +-----+-----+
              v                v                v
        +-----------+    +-----------+    +-----------+
        |ASSIGNMENT1|    |ASSIGNMENT2|    |ASSIGNMENT3|
        |Client:ACME|    |Client:XYZ |    |Client:ACME|
        |Carrier: A |    |Carrier: B |    |Carrier: C |
        |Strat: RR  |    |Strat: LU  |    |Strat:stick|
        +-----------+    +-----------+    +-----------+

  RULES:
   - one IP can be in only ONE active group at a time
   - one carrier maps to ONE node at a time (history kept on change)
   - rotation strategies: round-robin | random | sticky | least-used | health-weighted
```

---

## 5. SINGLE CALL FLOW (the IP-change moment)

```
 DIALER (1.2.3.4)            PLATFORM                    CARRIER (5.6.7.8)
      |                                                       |
      | (1) INVITE  SDP media = 1.2.3.4:10000                 |
      |---------------------------->                          |
      |                  +-------------------+                |
      |                  |  KAMAILIO PROXY   |                |
      |                  | - whitelist check |                |
      |                  |   1.2.3.4 = ClientA ok            |
      |                  | - find route      |                |
      |                  |   ClientA+dest -> CarrierA        |
      |                  | - ask control plane for media IP  |
      |                  +---------+---------+                |
      |                            v                          |
      |                  +-------------------+                |
      |                  |  CONTROL PLANE    |                |
      |                  | ClientA+CarrierA  |                |
      |                  |   -> Group A      |                |
      |                  | Group A on Node1  |                |
      |                  | strategy=RR       |                |
      |                  | next IP=192.0.2.5 |                |
      |                  +---------+---------+                |
      |                            v                          |
      |                  +-------------------+                |
      |                  | NODE1 RTPENGINE   |                |
      |                  | allocate          |                |
      |                  | 192.0.2.5:30040   |                |
      |                  +---------+---------+                |
      |                            v                          |
      |          Kamailio rewrites SDP media=192.0.2.5:30040  |
      |                                                       |
      | (2) INVITE  SDP media = 192.0.2.5:30040               |
      |------------------------------------------------------>|
      | (3) 200 OK + carrier SDP                              |
      |<------------------------------------------------------|
      | (4) 200 OK relayed                                    |
      |<----------------------------                          |
      |                                                       |
  ====================== RTP MEDIA FLOWS ======================
      |  RTP            +----------------+           RTP       |
      |---------------->|  NODE 1        |------------------->|
      |                 |  RTPEngine     |                     |
      |<----------------|  192.0.2.5     |<-------------------|
      |  RTP            |  relays both   |           RTP       |
      |                 +----------------+                     |
      |                                                       |
      | (5) BYE                                               |
      |------------------------------------------------------>|
      |  RTPEngine releases 192.0.2.5:30040                   |
      |  CDR written, IP returns to rotation                  |
```

---

## 6. SERVER INVENTORY (40K concurrent target)

```
+----+------------------+-----+----------------------------+--------+
| Qty| Role             | IPs | Spec                       | $/mo   |
+----+------------------+-----+----------------------------+--------+
|  1 | Base App primary |  2  | 16c / 64GB / 1TB NVMe RAID1| 400    |
|  1 | Base App standby |  2  | 16c / 64GB / 1TB NVMe RAID1| 400    |
|  3 | SIP Proxy        |  1  | 8c  / 16GB / 240GB NVMe    | 100 ea |
| 18 | Media Node       | 50  | 16c / 32GB / 480 NVMe/10G  | 300 ea |
+----+------------------+-----+----------------------------+--------+
| IP leasing: ~1000 IPs (4x /24) @ ~$0.40/IP             ~ 410      |
| BGP announcement fees (3 DCs)                          ~ 300      |
| Backup + monitoring storage                            ~  50      |
+-----------------------------------------------------------+-------+
| TOTAL                                          ~ $7,300 - 11,000  |
+------------------------------------------------------------------+

Capacity check:
  - 18 nodes x 2,500 calls  = 45,000 concurrent (12% buffer over 40K)
  - 3 proxies x 5,000 CPS    = 15,000 CPS (need ~270 CPS -> huge headroom)
  - ~900 active IPs / 40K calls = ~44 calls per IP average
```

---

## 7. PER-MEDIA-NODE CAPACITY MODEL

```
Per call:
  - 2 RTP streams = ~100 packets/sec
  - G.711 bandwidth ~ 87 kbps (both directions)

At 2,500 calls per node:
  - PPS:        250,000 packets/sec   (kernel RTPEngine: 500k+ OK)
  - Bandwidth:  ~218 Mbps             (10 Gbps NIC: ~2% used)
  - CPU:        ~50% on 16 cores (kernel module ON)
  - RAM:        ~250 MB sessions + OS

Capacity formula (pick the LOWEST = real ceiling):
  max_calls = min(
     cores * 1500       (kernel mode, no transcoding),
     cores * 60         (if transcoding enabled),
     ram_mb / 50,
     nic_mbps * 1000 / 90
  )

WATCH OUT:
  - transcoding cuts capacity 5-10x  -> dedicate transcoding nodes
  - recording adds 30-50% CPU + disk
  - SRTP/DTLS adds 20-30% CPU
  - bump nf_conntrack_max, file descriptors, udp buffers (see sysctl below)
```

---

## 8. DATABASE SCHEMA (PostgreSQL)

```
resellers
  id, name, balance, settings_json, status, created_at

clients
  id, reseller_id (FK), name, status, created_at
  -> whitelisted via client_ips

client_ips
  id, client_id (FK), ip_address, port_range, status
  (source IPs of the dialer; used by Kamailio permissions)

media_nodes
  id, name, role(media|sip_proxy), host_ip, mgmt_ip, region,
  cpu_cores, ram_gb, nic_gbps, max_calls, soft_warn, hard_limit,
  transcoding_enabled, status(online|offline|draining), agent_token,
  rtpengine_version, last_seen_at, created_at

node_ips                          (the IP pool)
  id, node_id (FK), ip_address, status(active|disabled|flagged),
  purchased_from, lease_block, lease_expires, monthly_cost,
  rdns, reputation_score, last_health_check, current_calls

ip_groups
  id, name, status, created_by, notes, created_at

ip_group_members
  id, group_id (FK), ip_id (FK)
  UNIQUE(ip_id) WHERE active   -- one IP in one active group only

carriers
  id, name, host, port, transport(udp|tcp|tls),
  assigned_node_id (FK), codec_pref, status, created_at

carrier_node_history
  id, carrier_id (FK), old_node_id, new_node_id,
  changed_by, changed_at, reason, active_calls_at_switch

assignments
  id, group_id (FK), client_id (FK), carrier_id (FK),
  rotation_strategy, status(active|paused|ended),
  assigned_by, assigned_at

assignment_history
  id, assignment_id (FK), action(created|modified|ended),
  old_values_json, new_values_json, changed_by, changed_at

routes
  id, client_id (FK), match_prefix/dest_pattern, carrier_id (FK),
  priority, status

call_records  (CDR - partition monthly at scale)
  id, call_id, client_id, carrier_id, node_id, media_ip,
  signaling_from, ani, dnis, started_at, answered_at, ended_at,
  duration_sec, disposition, sip_code

ip_assignment_log
  id, call_id, ip_id (FK), assigned_at, released_at

node_metrics  (time-series; or push to Prometheus instead)
  id, node_id (FK), ts, active_calls, cpu_pct, ram_pct,
  net_in_mbps, net_out_mbps, pps, packet_loss_pct, ips_healthy

admin_users
  id, email, role(admin|reseller|viewer), password_hash, mfa, status

audit_log
  id, actor_id, action, target, before_json, after_json, ts, ip
```

Key constraints:
- one IP in one active group: partial unique index on `ip_group_members`.
- one carrier -> one node: `carriers.assigned_node_id`; every change logs a row in `carrier_node_history`.

---

## 9. AGENT DAEMON (runs on every node)

```
+--------------------------------------------------------------+
|  AGENT DAEMON (small Go binary on each node)                 |
|                                                              |
|  On startup:                                                 |
|    - register with base app (agent_token auth)               |
|    - report specs (cores, ram, nic, OS, rtpengine ver)       |
|    - discover bound IPs (ip -j addr show) -> report to pool  |
|                                                              |
|  Every 5-10 sec:                                             |
|    - push metrics: active_calls, cpu, ram, net, pps, loss    |
|                                                              |
|  On command from base app:                                   |
|    - add IP    (ip addr add + netplan persist)               |
|    - remove IP                                               |
|    - disable/enable IP in RTPEngine                          |
|    - drain node (stop accepting new calls)                   |
|    - restart RTPEngine                                       |
|                                                              |
|  Self-heal: reconnect if base app link drops                 |
+--------------------------------------------------------------+

  base app  <----HTTPS / mTLS---->  agent      (control + metrics)
  kamailio  <----NG protocol---->   rtpengine  (per-call media ctrl)
```

---

## 10. ADD-A-NODE FROM GUI (flow)

```
Admin clicks "Add Node"
        |
        v
+-------------------------------+
| Name, Host IP, SSH creds,     |
| Role (SIP proxy | Media),     |
| Region, Max calls             |
+---------------+---------------+
                |
                v
Base app SSHes into the node and runs installer:
  1. detect OS (Ubuntu 22.04/24.04)
  2. apt update/upgrade
  3. if media : install RTPEngine + kernel module + sysctl tuning
     if proxy : install Kamailio + modules
  4. install agent daemon (systemd service)
  5. agent registers -> appears in panel
  6. health check passes -> status = Online

Progress bar shows each step. No further SSH needed afterwards.
```

---

## 11. ADMIN PANEL STRUCTURE (UI)

```
+--------------------------------------------------------------+
|  YOUR PLATFORM                                  [Admin v]    |
+--------------------------------------------------------------+
|  SIDEBAR              |  PAGE CONTENT                        |
|  --------             |  ------------                        |
|  Dashboard            |  live calls, ASR, IP usage, node     |
|                       |  health, top carriers/clients        |
|  Resellers            |  list/create/edit, balance           |
|  Clients              |  whitelist IPs, status, assign group |
|  Nodes                |  add/remove, capacity bars, drain    |
|  IP Pool              |  all leased IPs, flag, refresh, rDNS  |
|  Groups               |  build groups from pool              |
|  Carriers             |  add, assign node, view history      |
|  Routes               |  client+dest -> carrier              |
|  Live Calls           |  real-time call list (websocket)     |
|  CDRs                 |  search/filter/export history        |
|  Reports              |  ASR/ACD per IP/carrier/client       |
|  Logs / Audit         |  admin actions trail                 |
|  Settings             |  global config, API keys, alerts     |
+--------------------------------------------------------------+
```

### Node card (capacity display)

```
+- Node 1 - LA Datacenter ------------------------------------+
| Status: ONLINE     Uptime: 47d                              |
| Active calls   [########............]  287 / 600            |
| CPU            [######..............]   32%                 |
| RAM            [####................]   24%                 |
| Network        [##..................]   18 Mbps             |
| IPs healthy    [###################.]   58 / 60             |
| Loss 0.01%  Jitter 6ms  ASR 41%                             |
| [View] [Drain] [Edit]                                       |
+-------------------------------------------------------------+
CLUSTER TOTAL: 854 / 1800 calls (47%)
```

### Carrier node-history page

```
Carrier A  >  View History
+-------------------------------------------------------------+
| Date        From    To     Changed By   Reason              |
| 2026-05-20  Node1   Node2  admin@you    IP reputation       |
| 2026-04-15  Node3   Node1  admin@you    Capacity            |
| 2026-03-02  --      Node3  admin@you    Initial setup       |
| Active calls during last switch: 87 (drained gracefully)    |
+-------------------------------------------------------------+
```

---

## 12. TECH STACK

```
+----------------+--------------------------+--------------------+
| Layer          | Tech                     | Runs on            |
+----------------+--------------------------+--------------------+
| Admin Panel    | React + Tailwind         | Base app           |
| REST API       | Go (Gin/Fiber) or Node   | Base app           |
| Database       | PostgreSQL 15            | Base app (+replica)|
| Cache/State    | Redis 7                  | Base app           |
| SIP Proxy      | Kamailio 5.7+            | SIP proxy nodes    |
| Media Relay    | RTPEngine + kernel mod   | Media nodes        |
| Node Agent     | Go binary                | Every node         |
| Monitoring     | Prometheus + Grafana     | Base app / sep.    |
| Logs           | Loki / ELK               | Base app / sep.    |
| Reverse proxy  | nginx + Let's Encrypt    | Base app           |
| Backups        | pg_dump + WAL -> S3/B2    | Base app           |
+----------------+--------------------------+--------------------+
```

---

## 13. KAMAILIO ROUTING LOGIC (pseudocode)

```
route {
    if (!is_method("INVITE|ACK|BYE|CANCEL|OPTIONS")) {
        sl_send_reply("405","Method Not Allowed"); exit;
    }

    # 1. whitelist
    if (!allow_address("1","$si","$sp")) {
        sl_send_reply("403","Forbidden"); exit;
    }

    # 2. identify client by source IP
    $var(client) = lookup_client_by_ip($si);

    # 3. find route -> carrier
    $var(route) = lookup_route($var(client), $rU);
    if ($var(route) == null) { sl_send_reply("404","No Route"); exit; }

    # 4. ask control plane for media IP (HTTP or Redis cache)
    $var(media_ip) = get_media_ip($var(client), $var(route.carrier));

    # 5. capacity guard
    if (node_full($var(media_ip))) { sl_send_reply("503","Busy"); exit; }

    # 6. RTPEngine: allocate + rewrite SDP to media IP
    rtpengine_manage("replace-origin replace-session-connection
                      out-iface=$var(media_ip)");

    # 7. forward to carrier
    $du = $var(route.carrier_host);
    t_relay();
}

onreply_route {
    if (status =~ "(200|486|487|603)") rtpengine_manage();
}
```

---

## 14. OS TUNING (media nodes) - /etc/sysctl.conf

```
net.core.rmem_max = 134217728
net.core.wmem_max = 134217728
net.core.netdev_max_backlog = 50000
net.ipv4.udp_mem = 102400 873800 16777216
net.ipv4.udp_rmem_min = 16384
net.ipv4.udp_wmem_min = 16384
net.ipv4.ip_local_port_range = 1024 65535
net.netfilter.nf_conntrack_max = 2097152
fs.file-max = 2097152
# ulimit -n 1000000  (systemd LimitNOFILE)

# RTPEngine
INTERFACES="ip1!pub1 ip2!pub2 ... ipN!pubN"
PORT_MIN=30000
PORT_MAX=60000
LISTEN_NG=127.0.0.1:2223
TOS=184
```

---

## 15. IP LEASING WORKFLOW

```
1. Register on IPXO / Heficed (company KYC)
2. Search marketplace -> select /24 blocks (separate /24 per DC!)
3. Lease subnet -> pay LOA fee (~$9 per ASN assignment)
4. Provide ASN (your own or your DC provider's)
5. Receive LOA + ROA documents
6. Send LOA to hosting provider (Performive/Psychz/etc.)
7. Provider announces block via BGP -> IPs route to your node
8. Agent binds IPs -> they appear in the pool (24-48h total)

NOTE: to split across DCs you must lease SEPARATE /24s
      (a /23 cannot have its halves announced in 2 places).

2026 pricing: ~$0.38-0.45/IP/mo (RIPE/ARIN), APNIC ~$0.60+
Build an "IP Inventory" page tracking block / source / DC / expiry.
```

---

## 16. HEALTH + AUTO-FLAG SYSTEM

```
Every 5 min per IP:
  - synthetic test call through the IP
  - track ASR (answer-seizure ratio) rolling window
  - check Spamhaus / AbuseIPDB / Talos
  - if ASR < threshold OR blacklisted:
        -> auto-flag IP
        -> remove from active rotation
        -> swap in a reserve IP
        -> alert admin
  - track per-carrier ASR per IP (IP can be good for A, bad for B)

At ~1000 IPs expect to lose 10-50/week -> automation is mandatory.
```

---

## 17. PHASED ROLLOUT

```
Phase 1  Month 1     Base app only                1 server   $400/mo   build panel+DB
Phase 2  Month 2     +1 SIP +1 Media +1 /24       3 servers  $1000/mo  2,500 cc, e2e call
Phase 3  Month 3-4   +2 Media +1 SIP +1 /24       6 servers  $2000/mo  7,500 cc, MVP
Phase 4  Month 5-7   +6 Media +standby +2 /24    15 servers  $5500/mo  22,500 cc, HA
Phase 5  Month 8-12  +9 Media, tuning, autoscale 23 servers  $7-11k/mo 45,000 cc

Validate each phase under real load before doubling.
```

---

## 18. BUILD ORDER (engineering)

```
1.  Control plane + DB + minimal admin UI
2.  One media node + RTPEngine + agent daemon
3.  Kamailio: whitelist + RTPEngine bridge
4.  End-to-end test call with media IP rewrite working
5.  IP pool + groups + assignment logic
6.  Carrier mgmt + node assignment + history
7.  Live calls page + CDRs + reports
8.  Health monitoring + auto-flag bad IPs
9.  Multi-node deploy + graceful drain logic
10. Reseller features + billing hooks + polish

MVP realistic in 6-8 weeks with: 1-2 backend, 1 frontend,
1 VoIP/network engineer (Kamailio/RTPEngine/BGP), 1 ops/SRE.
```

---

## 19. CRITICAL RISKS / GOTCHAS

```
- Sourcing ~1000 CLEAN IPs at sane price (check reputation before activating)
- Hosts that allow VoIP + large IP allocations (Performive/Psychz/Path.net/Hivelocity)
- Carrier whitelisting at scale (every IP must be declared; mostly manual tickets)
- IP reputation churn (auto-flag + reserve-swap is mandatory)
- CDR volume (~700M rows/yr at 40K) -> monthly partitions + cold storage
- DDoS targeting SIP proxies -> Path.net / Voxility / Cloudflare Spectrum
- Compliance: STIR/SHAKEN (US), DNC lists, recording disclosure, real entity
- Base app is single source of truth -> HA standby + tested restores
- Transcoding kills capacity -> isolate on dedicated nodes
- Split /24 per DC (cannot announce a /23 in two places)
```

---

## 20. PROVIDER MIX (avoid single-provider risk)

```
Base app primary   Hetzner dedicated (Falkenstein)
Base app standby   Hivelocity (Dallas)
SIP proxy 1        Vultr High Performance (NY)
SIP proxy 2        Hetzner Cloud CCX33 (Helsinki)
SIP proxy 3        DigitalOcean (NYC)
Media DC1 (6)      Performive (Ashburn)
Media DC2 (6)      Psychz (LA)
Media DC3 (6)      Hivelocity (Chicago)
IP leasing         IPXO + Heficed
```

---

*End of architecture document.*
