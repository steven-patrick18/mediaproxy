BEGIN;
-- External API integration feature removed. FreeSWITCH (and any SIP-PBX
-- upstream) is modeled as a plain Carrier row.
ALTER TABLE carriers DROP COLUMN IF EXISTS integration_id;
DROP TABLE IF EXISTS external_integrations;
COMMIT;
