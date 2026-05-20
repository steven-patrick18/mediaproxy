BEGIN;
ALTER TABLE signaling_ips DROP COLUMN IF EXISTS auto_discovered;
COMMIT;
