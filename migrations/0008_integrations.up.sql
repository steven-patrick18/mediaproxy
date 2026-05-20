BEGIN;

-- External API integrations (SignalWire, generic FreeSWITCH PBX, Twilio, ...).
-- Each row holds credentials for one external system. The `config` JSONB shape
-- is provider-specific; for signalwire it's { space_url, project_id, api_token }.
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

-- Optional link from a carrier to the integration that backs it. A
-- SignalWire carrier, for example, would point to its SignalWire
-- integration so the panel can fetch number inventory, rates, etc.
ALTER TABLE carriers
    ADD COLUMN integration_id BIGINT REFERENCES external_integrations(id) ON DELETE SET NULL;

CREATE INDEX idx_carriers_integration ON carriers(integration_id);

COMMIT;
