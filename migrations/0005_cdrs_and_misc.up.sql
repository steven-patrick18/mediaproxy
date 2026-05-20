BEGIN;

-- Mark which IPs the agent auto-discovered (vs admin pre-registered)
ALTER TABLE node_ips ADD COLUMN auto_discovered BOOLEAN NOT NULL DEFAULT false;

-- NOC role: read everything, take operational actions (drain, flag), no
-- billing or tenant-admin powers.
ALTER TABLE admin_users DROP CONSTRAINT IF EXISTS admin_users_role_check;
ALTER TABLE admin_users ADD CONSTRAINT admin_users_role_check
    CHECK (role IN ('admin','noc','reseller','viewer'));

-- Free-text 'notes' across editable entities so the panel can show + edit it.
ALTER TABLE resellers ADD COLUMN IF NOT EXISTS notes TEXT;
ALTER TABLE clients   ADD COLUMN IF NOT EXISTS notes TEXT;
ALTER TABLE carriers  ADD COLUMN IF NOT EXISTS notes TEXT;

-- Call detail records. Kamailio (or a future call-event consumer) will
-- write into this table at INVITE / 200OK / BYE. For now it's empty.
CREATE TABLE call_records (
    id              BIGSERIAL  PRIMARY KEY,
    call_id         TEXT       NOT NULL,
    client_id       BIGINT     REFERENCES clients(id)      ON DELETE SET NULL,
    carrier_id      BIGINT     REFERENCES carriers(id)     ON DELETE SET NULL,
    node_id         BIGINT     REFERENCES media_nodes(id)  ON DELETE SET NULL,
    media_ip        INET,
    signaling_from  INET,
    ani             TEXT,
    dnis            TEXT,
    started_at      TIMESTAMPTZ NOT NULL,
    answered_at     TIMESTAMPTZ,
    ended_at        TIMESTAMPTZ,
    duration_sec    INT,
    disposition     TEXT
                    CHECK (disposition IN ('answered','busy','no_answer','failed','canceled')),
    sip_code        INT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_cdr_started_at ON call_records(started_at DESC);
CREATE INDEX idx_cdr_client     ON call_records(client_id,  started_at DESC);
CREATE INDEX idx_cdr_carrier    ON call_records(carrier_id, started_at DESC);
CREATE INDEX idx_cdr_node       ON call_records(node_id,    started_at DESC);

-- Live call snapshot. Agents (or RTPEngine via the agent) push active calls
-- here; rows older than ~2 minutes are stale and treated as completed.
-- Keeping this separate from call_records avoids hot updates on the CDR table.
CREATE TABLE active_calls (
    id              BIGSERIAL PRIMARY KEY,
    call_id         TEXT      NOT NULL UNIQUE,
    client_id       BIGINT    REFERENCES clients(id)     ON DELETE SET NULL,
    carrier_id      BIGINT    REFERENCES carriers(id)    ON DELETE SET NULL,
    node_id         BIGINT    REFERENCES media_nodes(id) ON DELETE SET NULL,
    media_ip        INET,
    signaling_from  INET,
    ani             TEXT,
    dnis            TEXT,
    started_at      TIMESTAMPTZ NOT NULL,
    last_seen_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_active_calls_node ON active_calls(node_id);
CREATE INDEX idx_active_calls_seen ON active_calls(last_seen_at DESC);

COMMIT;
