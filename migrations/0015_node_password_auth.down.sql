BEGIN;
ALTER TABLE media_nodes DROP COLUMN IF EXISTS password_auth_enabled;
COMMIT;
