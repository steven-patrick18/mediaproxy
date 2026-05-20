BEGIN;

-- Phase 1: rotation cursor per-assignment for round-robin / least-used strategies.
ALTER TABLE assignments ADD COLUMN rotation_cursor INT NOT NULL DEFAULT 0;

-- Phase 1: CDR / call_records gains some indices we'll need under load.
CREATE INDEX IF NOT EXISTS idx_cdr_ani  ON call_records(ani)  WHERE ani  IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_cdr_dnis ON call_records(dnis) WHERE dnis IS NOT NULL;

-- Phase 2: MFA / TOTP. mfa_secret column already exists; add an
-- "enrolled" flag separate from "mfa_secret IS NOT NULL" so a user
-- can scan a QR code, verify, then we mark it enrolled. Recovery
-- codes are JSONB-stored hashes; one-time use, removed when used.
ALTER TABLE admin_users
    ADD COLUMN mfa_enrolled BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN mfa_recovery_codes JSONB;

-- Phase 2: Webhook subscribers + event dispatch log.
CREATE TABLE webhooks (
    id         BIGSERIAL PRIMARY KEY,
    name       TEXT NOT NULL,
    url        TEXT NOT NULL,
    events     TEXT[] NOT NULL DEFAULT '{node.offline,asr.dropped}',
    secret     TEXT,                                   -- HMAC-SHA256 signing key
    enabled    BOOLEAN NOT NULL DEFAULT true,
    created_by BIGINT REFERENCES admin_users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE webhook_deliveries (
    id          BIGSERIAL PRIMARY KEY,
    webhook_id  BIGINT NOT NULL REFERENCES webhooks(id) ON DELETE CASCADE,
    event       TEXT   NOT NULL,
    payload     JSONB  NOT NULL,
    status      TEXT   NOT NULL DEFAULT 'pending'
                CHECK (status IN ('pending','success','failed','retrying')),
    attempts    INT    NOT NULL DEFAULT 0,
    last_error  TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    delivered_at TIMESTAMPTZ
);
CREATE INDEX idx_webhook_deliveries_status ON webhook_deliveries(status, created_at);

-- Track which alerts we've already fired (so we don't spam — one alert
-- per (node, condition) until the condition resolves).
CREATE TABLE alert_state (
    id         BIGSERIAL PRIMARY KEY,
    key        TEXT NOT NULL UNIQUE,    -- e.g. "node.offline:5"
    fired_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at TIMESTAMPTZ
);
CREATE INDEX idx_alert_state_open ON alert_state(key) WHERE resolved_at IS NULL;

COMMIT;
