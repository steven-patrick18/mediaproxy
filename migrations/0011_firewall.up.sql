BEGIN;

CREATE TABLE firewall_rules (
    id              BIGSERIAL  PRIMARY KEY,
    name            TEXT       NOT NULL,
    action          TEXT       NOT NULL
                    CHECK (action IN ('allow','block','rate_limit')),
    source_cidr     CIDR,                                   -- NULL = any source
    dest_port_low   INT        CHECK (dest_port_low  IS NULL OR (dest_port_low  BETWEEN 1 AND 65535)),
    dest_port_high  INT        CHECK (dest_port_high IS NULL OR (dest_port_high BETWEEN 1 AND 65535)),
    proto           TEXT       NOT NULL DEFAULT 'any'
                    CHECK (proto IN ('any','tcp','udp')),
    node_id         BIGINT     REFERENCES media_nodes(id) ON DELETE CASCADE,  -- NULL = applies to all nodes
    rate_per_second INT        CHECK (rate_per_second IS NULL OR rate_per_second > 0),
    priority        INT        NOT NULL DEFAULT 100,
    enabled         BOOLEAN    NOT NULL DEFAULT true,
    notes           TEXT,
    created_by      BIGINT     REFERENCES admin_users(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_fw_node     ON firewall_rules(node_id);
CREATE INDEX idx_fw_enabled  ON firewall_rules(enabled) WHERE enabled = true;

COMMIT;
