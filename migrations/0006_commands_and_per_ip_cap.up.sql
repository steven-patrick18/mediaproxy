BEGIN;

-- Per-IP concurrent-call cap. 0 = no per-IP limit (use node max_calls only).
ALTER TABLE node_ips ADD COLUMN max_calls INT NOT NULL DEFAULT 0;

-- Commands the operator queues for an agent to execute on its next heartbeat.
-- Heartbeat returns rows where status='queued' and flips them to 'sent';
-- the agent ACKs via /agent/command-result which sets status='done'|'error'.
CREATE TABLE node_commands (
    id            BIGSERIAL  PRIMARY KEY,
    node_id       BIGINT     NOT NULL REFERENCES media_nodes(id) ON DELETE CASCADE,
    type          TEXT       NOT NULL,
    payload       JSONB      NOT NULL DEFAULT '{}'::jsonb,
    status        TEXT       NOT NULL DEFAULT 'queued'
                  CHECK (status IN ('queued','sent','done','error')),
    detail        TEXT,
    created_by    BIGINT     REFERENCES admin_users(id) ON DELETE SET NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    sent_at       TIMESTAMPTZ,
    completed_at  TIMESTAMPTZ
);
CREATE INDEX idx_node_cmd_node_status ON node_commands(node_id, status);

COMMIT;
