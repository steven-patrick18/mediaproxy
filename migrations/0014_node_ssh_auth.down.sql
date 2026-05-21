BEGIN;
ALTER TABLE media_nodes DROP COLUMN IF EXISTS ssh_auth_method;
COMMIT;
