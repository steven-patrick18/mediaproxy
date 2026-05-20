BEGIN;
DROP TABLE IF EXISTS node_metrics;
ALTER TABLE media_nodes
    DROP COLUMN IF EXISTS active_calls,
    DROP COLUMN IF EXISTS cpu_pct,
    DROP COLUMN IF EXISTS ram_pct,
    DROP COLUMN IF EXISTS net_in_mbps,
    DROP COLUMN IF EXISTS net_out_mbps,
    DROP COLUMN IF EXISTS packet_loss_pct,
    DROP COLUMN IF EXISTS uptime_seconds,
    DROP COLUMN IF EXISTS agent_version,
    DROP COLUMN IF EXISTS ips_bound;
COMMIT;
