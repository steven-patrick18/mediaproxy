BEGIN;
ALTER TABLE media_nodes DROP COLUMN IF EXISTS firewall_applied_at;
COMMIT;
