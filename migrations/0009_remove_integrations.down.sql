BEGIN;
-- Re-create the integrations table if rolling back. Note: any data is lost.
CREATE TABLE external_integrations (
    id                BIGSERIAL  PRIMARY KEY,
    name              TEXT       NOT NULL,
    provider          TEXT       NOT NULL
                      CHECK (provider IN ('signalwire','freeswitch','twilio','other')),
    config            JSONB      NOT NULL DEFAULT '{}'::jsonb,
    status            TEXT       NOT NULL DEFAULT 'unverified'
                      CHECK (status IN ('unverified','verified','failed','disabled')),
    last_verified_at  TIMESTAMPTZ,
    last_error        TEXT,
    created_by        BIGINT     REFERENCES admin_users(id) ON DELETE SET NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE carriers
    ADD COLUMN integration_id BIGINT REFERENCES external_integrations(id) ON DELETE SET NULL;
CREATE INDEX idx_carriers_integration ON carriers(integration_id);
COMMIT;
