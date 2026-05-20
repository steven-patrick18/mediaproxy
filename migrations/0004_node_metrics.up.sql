BEGIN;

-- Denormalized "latest snapshot" on the node row, for fast list views.
ALTER TABLE media_nodes ADD COLUMN active_calls    INT;
ALTER TABLE media_nodes ADD COLUMN cpu_pct         NUMERIC(5,2);
ALTER TABLE media_nodes ADD COLUMN ram_pct         NUMERIC(5,2);
ALTER TABLE media_nodes ADD COLUMN net_in_mbps     NUMERIC(10,2);
ALTER TABLE media_nodes ADD COLUMN net_out_mbps    NUMERIC(10,2);
ALTER TABLE media_nodes ADD COLUMN packet_loss_pct NUMERIC(5,2);
ALTER TABLE media_nodes ADD COLUMN uptime_seconds  BIGINT;
ALTER TABLE media_nodes ADD COLUMN agent_version   TEXT;
ALTER TABLE media_nodes ADD COLUMN ips_bound       INT DEFAULT 0;

-- Per-heartbeat time series. Used for sparklines and trend charts.
-- At 1 row/min/node and ~20 nodes that's ~10M rows/year — manageable
-- without partitioning until call_records becomes the bottleneck instead.
CREATE TABLE node_metrics (
    id              BIGSERIAL   PRIMARY KEY,
    node_id         BIGINT      NOT NULL REFERENCES media_nodes(id) ON DELETE CASCADE,
    ts              TIMESTAMPTZ NOT NULL DEFAULT now(),
    active_calls    INT,
    cpu_pct         NUMERIC(5,2),
    ram_pct         NUMERIC(5,2),
    net_in_mbps     NUMERIC(10,2),
    net_out_mbps    NUMERIC(10,2),
    packet_loss_pct NUMERIC(5,2)
);
CREATE INDEX idx_node_metrics_node_ts ON node_metrics(node_id, ts DESC);

COMMIT;
