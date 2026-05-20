BEGIN;
ALTER TABLE carriers DROP COLUMN IF EXISTS integration_id;
DROP TABLE IF EXISTS external_integrations;
COMMIT;
