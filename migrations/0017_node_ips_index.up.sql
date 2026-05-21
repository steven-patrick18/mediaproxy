-- Hot path: the router joins node_ips by (node_id, status='active') on every
-- /route call. Without this index Postgres does a full scan that scales with
-- total IP-pool size across the cluster. Partial index keeps it small.
CREATE INDEX IF NOT EXISTS idx_node_ips_node_status
    ON node_ips (node_id, status)
 WHERE status IN ('active', 'reserve');
